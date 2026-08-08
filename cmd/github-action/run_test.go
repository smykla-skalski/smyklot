package main

import (
	"context"
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

	r.other = append(r.other, req.Method+" "+req.URL.Path)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`[]`))
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

	recorder := &commentRecorder{}
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
