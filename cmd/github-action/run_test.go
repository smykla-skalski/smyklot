package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// envDisableDeletedComments is the viper-bound form of the disable flag
const envDisableDeletedComments = "SMYKLOT_DISABLE_DELETED_COMMENTS"

// envRunner is the viper-bound form of the runner setting
const envRunner = "SMYKLOT_RUNNER"

// runEnv lists every variable run() reads, so a spec starts from a known state
// whatever the developer's shell or the CI job already exports
var runEnv = []string{
	envGitHubToken,
	envCommentBody,
	envCommentID,
	envCommentAction,
	envPRNumber,
	envRepoOwner,
	envRepoName,
	envCommentAuthor,
	envGitHubAppPrivateKey,
	envGitHubAppClientID,
	envGitHubAppID,
	envInstallationID,
	envBotUsername,
	envAPIBaseURL,
	envStepSummary,
	envDisableDeletedComments,
	envRunner,
	config.EnvConfig,
}

// postedComment records one comment the bot posted during a run
type postedComment struct {
	Path string
	Body string
}

// commentRecorder is a stand-in GitHub API that captures posted comments and
// flags any other call, so a spec can assert on everything run() did
type commentRecorder struct {
	// repoConfig is the repository's .github/smyklot.yaml, or empty for a
	// repository that has none
	repoConfig string

	mu       sync.Mutex
	comments []postedComment
	other    []string
}

func (r *commentRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/comments") {
		raw, _ := io.ReadAll(req.Body)

		var payload struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(raw, &payload)

		r.comments = append(r.comments, postedComment{Path: req.URL.Path, Body: payload.Body})

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))

		return
	}

	// Both entry points look for the repository's own configuration before
	// doing anything else, so this is expected traffic rather than a surprise
	if strings.Contains(req.URL.Path, "/contents/.github/smyklot.") {
		if r.repoConfig == "" || !strings.HasSuffix(req.URL.Path, ".yaml") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))

			return
		}

		payload, _ := json.Marshal(map[string]string{
			"content": base64.StdEncoding.EncodeToString([]byte(r.repoConfig)),
		})
		_, _ = w.Write(payload)

		return
	}

	r.other = append(r.other, req.Method+" "+req.URL.Path)

	// GitHub answers a create with the thing it created, and a list with a
	// list. Answering everything with `[]` was survivable only while the client
	// discarded the bodies it did not need; it is a lie either way, and the
	// kind that makes a spec pass against an endpoint nobody modelled.
	if req.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))

		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":1}`))
}

func (r *commentRecorder) posted() []postedComment {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]postedComment(nil), r.comments...)
}

func (r *commentRecorder) unexpected() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.other...)
}

// runWithComment drives run() end to end against a test server, as if GitHub
// had delivered commentAction on a comment whose body is commentBody, and
// asserts run() succeeded without calling anything but the comments endpoint
func runWithComment(commentAction, commentBody string, env map[string]string) []postedComment {
	GinkgoHelper()

	recorder, err := runComment(commentAction, commentBody, env)
	Expect(err).NotTo(HaveOccurred())
	Expect(recorder.unexpected()).To(BeEmpty(), "run() called an endpoint the spec did not expect")

	return recorder.posted()
}

// runComment drives run() and hands back everything it did, including the error
func runComment(commentAction, commentBody string, env map[string]string) (*commentRecorder, error) {
	GinkgoHelper()

	return runCommentOn(&commentRecorder{}, commentAction, commentBody, env)
}

// runCommentOn drives run() against a recorder the caller has set up, so a spec
// can decide what the repository looks like
func runCommentOn(
	recorder *commentRecorder,
	commentAction, commentBody string,
	env map[string]string,
) (*commentRecorder, error) {
	GinkgoHelper()

	server := httptest.NewServer(recorder)
	DeferCleanup(server.Close)

	for _, name := range runEnv {
		GinkgoT().Setenv(name, "")
	}

	settings := map[string]string{
		envGitHubToken:   "test-token",
		envAPIBaseURL:    server.URL,
		envCommentAction: commentAction,
		envCommentBody:   commentBody,
		envCommentID:     "555",
		envPRNumber:      "42",
		envRepoOwner:     "smykla-skalski",
		envRepoName:      "smyklot",
		envCommentAuthor: "someone",

		// These specs exercise the Action, which stands down by default now
		// that the service handles a repository that says nothing
		envRunner: string(config.RunnerAction),
	}
	for key, value := range env {
		settings[key] = value
	}

	for key, value := range settings {
		GinkgoT().Setenv(key, value)
	}

	cmd := &cobra.Command{}
	registerRunFlags(cmd)
	cmd.SetContext(context.Background())

	return recorder, run(cmd, nil)
}

var _ = Describe("Deleted comment handling [Unit]", func() {
	Context("when the deleted comment carried a command", func() {
		It("should post a notice naming the command", func() {
			posted := runWithComment("deleted", "/approve", nil)

			Expect(posted).To(HaveLen(1))
			Expect(posted[0].Path).To(Equal("/repos/smykla-skalski/smyklot/issues/42/comments"))
			Expect(posted[0].Body).To(ContainSubstring("Command Comment Deleted"))
			Expect(posted[0].Body).To(ContainSubstring("`approve`"))
			Expect(posted[0].Body).To(ContainSubstring("someone"))
		})

		// Guards the ordering in run(): moving the parse-error check back above
		// the deleted branch drops this notice entirely
		It("should post a notice when the command combination was rejected", func() {
			posted := runWithComment("deleted", "/approve /unapprove", nil)

			Expect(posted).To(HaveLen(1))
			Expect(posted[0].Body).To(ContainSubstring("`approve`, `unapprove`"))
		})
	})

	Context("when the deleted comment carried no command", func() {
		DescribeTable("should stay silent",
			func(commentBody string) {
				Expect(runWithComment("deleted", commentBody, nil)).To(BeEmpty())
			},
			Entry("plain discussion comment", "I am not sure this is the right approach here"),
			Entry("cleanup command", "/cleanup"),
		)

		// An empty body never reaches the deleted branch - validateConfig
		// rejects it first - so the silence there proves nothing about this code
		It("should reject an empty comment body before deciding anything", func() {
			_, err := runComment("deleted", "", nil)

			Expect(err).To(MatchError(ContainSubstring(envCommentBody)))
		})
	})

	// The deleted branch must return rather than fall through: a deleted /merge
	// comment executing the merge again is the regression here
	It("should not execute the command when deleted comment handling is disabled", func() {
		posted := runWithComment("deleted", "/merge", map[string]string{
			envDisableDeletedComments: "true",
		})

		Expect(posted).To(BeEmpty())
	})
})

var _ = Describe("Repository configuration [Unit]", func() {
	// The service reads this file because it cannot see a workflow's repository
	// variables. The Action reads it too, or the same comment would get
	// different treatment depending on which one handled it
	It("should honour .github/smyklot.yaml", func() {
		recorder := &commentRecorder{repoConfig: "disable_deleted_comments: true\n"}

		_, err := runCommentOn(recorder, "deleted", "/approve", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.posted()).To(BeEmpty())
	})

	It("should keep the environment's settings for anything the file omits", func() {
		recorder := &commentRecorder{repoConfig: "quiet_success: true\n"}

		_, err := runCommentOn(recorder, "deleted", "/approve", map[string]string{
			envDisableDeletedComments: "true",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.posted()).To(BeEmpty())
	})

	It("should act normally for a repository without the file", func() {
		recorder := &commentRecorder{}

		_, err := runCommentOn(recorder, "deleted", "/approve", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.posted()).To(HaveLen(1))
	})

	// A broken file used to abort before any feedback, leaving the reason
	// visible only in the workflow log that PR authors do not read
	Context("when the file cannot be parsed", func() {
		It("should say so on the pull request", func() {
			recorder := &commentRecorder{repoConfig: "- a list, not a mapping\n"}

			_, err := runCommentOn(recorder, "created", "/approve", nil)
			Expect(err).To(MatchError(ErrRepoConfigInvalid))

			posted := recorder.posted()
			Expect(posted).To(HaveLen(1))
			Expect(posted[0].Body).To(ContainSubstring("Invalid Configuration File"))
			Expect(posted[0].Body).To(ContainSubstring("smyklot.yaml"))
		})

		It("should stay silent on a comment that asked for nothing", func() {
			recorder := &commentRecorder{repoConfig: "- a list, not a mapping\n"}

			_, err := runCommentOn(recorder, "created", "just thinking out loud", nil)
			Expect(err).To(MatchError(ErrRepoConfigInvalid))
			Expect(recorder.posted()).To(BeEmpty())
		})
	})
})
