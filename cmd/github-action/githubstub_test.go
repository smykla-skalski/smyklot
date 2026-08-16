package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// githubStub is a stand-in GitHub API for the service specs.
//
// It answers the handful of endpoints a command touches and records every call,
// so a spec can assert on what the service did rather than on what it logged.
type githubStub struct {
	// codeowners is the repository's CODEOWNERS, or empty for a repository
	// without one
	codeowners string

	// branchPRs answers a pull request listing filtered by head branch, which
	// is how the configuration migration asks what became of its proposal.
	branchPRs string

	// migrationRef is the migration branch, empty until something pushes it.
	migrationRef string

	// migrationTip is what the commit on the migration branch says. Smyklot's
	// own, unless a spec describes somebody else having pushed to it.
	migrationTip string

	// forcedPushes counts the times the branch was replaced rather than
	// created, which must only happen when nothing is open against it.
	forcedPushes int

	// refuseBranchPush is an App that was never granted write access here.
	refuseBranchPush bool

	// busyOnBranchPush is GitHub rate-limiting the push, which is the same
	// request worth making again rather than a permission that will not come.
	busyOnBranchPush bool

	// createdPRs and createdTrees are what the migration sent, because a pull
	// request nobody asked for is judged entirely on what it contains.
	createdPRs   []string
	createdTrees []string
	createdBlobs []string

	// repoConfigTOML is the repository's .smyklot.toml, or empty for a
	// repository that has not migrated. Stocking both is how a spec describes
	// a repository carrying two configuration files
	repoConfigTOML string

	// repoConfig is the repository's .github/smyklot.yaml, or empty for a
	// repository without one
	repoConfig string

	// prAuthor is who opened the pull request, which decides whether a command
	// counts as self-approval
	prAuthor   string
	prLabels   string
	prHead     string
	prComments string

	issueComments    map[int64]issueCommentRecord
	commentReactions map[int64]string
	prReactions      string

	// installations is what GET /app/installations reports, and repos what
	// each installation can reach. Both are empty unless a spec sweeps
	installations string
	repos         string
	members       string
	membersStatus int

	// openPRs is what a repository's pull request list reports
	openPRs string

	// brokenRepo names a repository every request for fails, standing in for
	// one the App has lost access to
	brokenRepo string

	// probeStatus is what GET /app answers, which is the one call the readiness
	// probe makes. Set it before the service starts, never while it is running
	probeStatus int

	installationsStarted chan struct{}
	installationsRelease chan struct{}
	installationsBlock   sync.Once

	mu    sync.Mutex
	calls []string
}

func newGitHubStub() *githubStub {
	return &githubStub{
		codeowners:   "* @someone\n",
		migrationTip: migrationCommit,

		prAuthor:   "author",
		prLabels:   `[]`,
		prHead:     "command-head",
		prComments: `[{"id":555}]`,
		issueComments: map[int64]issueCommentRecord{
			githubtest.DefaultCommentID: {
				exists: true, body: "/approve", updatedAt: githubtest.DefaultUpdatedAt,
				author: githubtest.DefaultAuthor, authorType: githubtest.DefaultAuthorTypeVal,
			},
		},
		commentReactions: map[int64]string{},
		prReactions:      `[]`,
		installations:    `[]`,
		repos:            `{"total_count": 0, "repositories": []}`,
		members:          `[]`,
		membersStatus:    http.StatusOK,
		openPRs:          `[]`,
		probeStatus:      http.StatusOK,
	}
}

type issueCommentRecord struct {
	exists     bool
	body       string
	updatedAt  string
	author     string
	authorType string
}

func (s *githubStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	s.mu.Unlock()

	switch {
	case s.brokenRepo != "" && strings.Contains(r.URL.Path, "/"+s.brokenRepo+"/"):
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message": "Resource not accessible by integration"}`))

	case r.URL.Path == "/app/installations":
		s.mu.Lock()
		installations := s.installations
		s.mu.Unlock()
		if s.installationsStarted != nil {
			s.installationsBlock.Do(func() {
				close(s.installationsStarted)
				<-s.installationsRelease
			})
		}
		_, _ = io.WriteString(w, installations)

	// The readiness probe's endpoint. It must be matched after
	// /app/installations, which would otherwise never be reached
	case r.URL.Path == "/app":
		if s.probeStatus != http.StatusOK {
			w.WriteHeader(s.probeStatus)
			_, _ = w.Write([]byte(`{"message": "unavailable"}`))

			return
		}

		_, _ = w.Write([]byte(`{"id": 1197525, "slug": "smyklot"}`))

	// GitHub answers this 401 for an App JWT, which is what the probe carries.
	// The stub answering it too would hide a probe pointed back at it
	case r.URL.Path == "/rate_limit":
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))

	case r.URL.Path == "/installation/repositories":
		_, _ = io.WriteString(w, s.repos)

	case strings.HasPrefix(r.URL.Path, "/orgs/") && strings.HasSuffix(r.URL.Path, "/members"):
		if s.membersStatus != http.StatusOK {
			w.WriteHeader(s.membersStatus)
		}
		_, _ = io.WriteString(w, s.members)

	case strings.HasSuffix(r.URL.Path, "/pulls"):
		s.servePulls(w, r)

	// Git data, enough of it to let the configuration migration run end to
	// end. It routes in its own function because this switch is already at the
	// complexity the linter allows.
	case strings.Contains(r.URL.Path, "/git/"):
		s.serveGitData(w, r)

	case strings.HasSuffix(r.URL.Path, "/labels"):
		_, _ = w.Write([]byte(`[]`))

	// CI signals. These answer "nothing has reported" rather than being left to
	// a permissive default: the specs that watch a pull request settle on
	// no_checks are asserting exactly this, and leaving it implicit meant they
	// passed against an endpoint nobody had stubbed.
	case strings.HasSuffix(r.URL.Path, "/check-runs"):
		_, _ = w.Write([]byte(`{"total_count": 0, "check_runs": []}`))

	case strings.HasSuffix(r.URL.Path, "/status"):
		_, _ = w.Write([]byte(`{"total_count": 0, "statuses": []}`))

	// An unprotected branch, which is what these repositories are.
	case strings.HasSuffix(r.URL.Path, "/protection/required_status_checks"):
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Branch not protected"}`))

	case strings.HasSuffix(r.URL.Path, "/access_tokens"):
		expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"token": "installation-token", "expires_at": %q}`, expiry)

	case strings.HasSuffix(r.URL.Path, "/contents/.github/CODEOWNERS"):
		s.writeFile(w, s.codeowners)

	// The head of the default branch, which the service reads as the validator
	// for its configuration cache. A fixed SHA means a spec that reads twice
	// gets the second answer from cache, which is what production does.
	case strings.HasSuffix(r.URL.Path, "/commits"):
		_, _ = io.WriteString(w, `[{"sha":"0000000000000000000000000000000000000000"}]`)

	// A repository's own configuration, at any of the paths it may live at.
	// Only the legacy one is stocked, so a spec that sets repoConfig still
	// describes a repository configured the way it was before TOML.
	case strings.Contains(r.URL.Path, "/contents/"):
		s.serveRepoConfig(w, r)

	case strings.HasSuffix(r.URL.Path, "/reviews"):
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1, "state": "APPROVED"}`))

	case strings.Contains(r.URL.Path, "/issues/comments/") &&
		strings.Contains(r.URL.Path, "/reactions"):
		s.writeCommentReactions(w, r)

	case strings.Contains(r.URL.Path, "/issues/") &&
		strings.Contains(r.URL.Path, "/reactions"):
		s.writePullRequestReactions(w, r)

	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/issues/comments/"):
		s.writeIssueComment(w, r.URL.Path)

	case strings.HasSuffix(r.URL.Path, "/comments"):
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, s.prComments)

			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 1}`))

	case strings.Contains(r.URL.Path, "/pulls/"):
		_, _ = fmt.Fprintf(w, `{
			"number": 42,
			"state": "open",
			"mergeable": true,
			"mergeable_state": "clean",
			"title": "a change",
			"user": {"login": %q},
			"base": {"ref": "main"},
			"head": {"sha": %q},
			"labels": %s
		}`, s.prAuthor, s.prHead, s.prLabels)

	default:
		// A path this stub does not know about is a gap in the stub, and it
		// has to look like one. Answering 200 {} made a list decode to an
		// empty slice and an object to a zero struct, so a spec exercising an
		// endpoint nobody had stubbed passed while proving nothing. The 404
		// names the path so the fix is obvious rather than a hunt.
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(
			w,
			`{"message":"githubstub has no route for %s %s"}`,
			r.Method,
			r.URL.Path,
		)
	}
}

func (s *githubStub) observeIssueComment(payload []byte) {
	event, err := webhook.ParseIssueComment(payload)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.issueComments[event.Comment.ID] = issueCommentRecord{
		exists: event.Action != webhook.ActionDeleted,
		body:   event.Comment.Body, updatedAt: event.Comment.UpdatedAt,
		author: event.Comment.User.Login, authorType: event.Comment.User.Type,
	}
}

func (s *githubStub) writeIssueComment(w http.ResponseWriter, path string) {
	commentID, err := strconv.ParseInt(path[strings.LastIndex(path, "/")+1:], 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	comment, found := s.issueComments[commentID]
	if !found || !comment.exists {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))

		return
	}
	_, _ = fmt.Fprintf(w, `{
		"id":%d,"body":%q,"updated_at":%q,
		"user":{"login":%q,"type":%q}
	}`,
		commentID, comment.body, comment.updatedAt,
		comment.author, comment.authorType,
	)
}

func (s *githubStub) writeCommentReactions(w http.ResponseWriter, r *http.Request) {
	commentID, err := reactionCommentID(r.URL.Path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)

		return
	}
	s.mu.Lock()
	comment, found := s.issueComments[commentID]
	reactions := s.commentReactions[commentID]
	s.mu.Unlock()
	if !found || !comment.exists {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))

		return
	}
	switch r.Method {
	case http.MethodGet:
		if reactions == "" {
			reactions = `[]`
		}
		_, _ = io.WriteString(w, reactions)
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *githubStub) writePullRequestReactions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	reactions := s.prReactions
	s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		_, _ = io.WriteString(w, reactions)
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func reactionCommentID(path string) (int64, error) {
	const marker = "/issues/comments/"
	start := strings.Index(path, marker)
	if start < 0 {
		return 0, errors.New("comment path is missing")
	}
	value := path[start+len(marker):]
	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}

	return strconv.ParseInt(value, 10, 64)
}

func (s *githubStub) setInstallations(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.installations = value
}

// setRepoConfig changes the repository's file while the service is running, the
// way a commit to the default branch does
func (s *githubStub) setRepoConfig(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.repoConfig = content
}

// currentRepoConfig reads the file under the lock, because a spec can change it
// while a worker is serving a delivery
func (s *githubStub) currentRepoConfig() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repoConfig
}

// writeFile answers the contents API, treating empty content as a missing file
// servePulls answers both things the pull request endpoint is asked: opening
// one, and reporting what became of the one opened from a named branch.
func (s *githubStub) servePulls(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.record(&s.createdPRs, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"number":77,"state":"open"}`)

		return
	}

	// A listing filtered by head branch is asking about one proposal, not
	// about the repository's open pull requests
	if r.URL.Query().Get("head") != "" {
		_, _ = io.WriteString(w, orEmptyList(s.branchPRs))

		return
	}

	_, _ = io.WriteString(w, s.openPRs)
}

// serveRepoConfig stocks the two configuration paths a spec can set, and 404s
// the rest - which is what a repository that only has one of them looks like.
func (s *githubStub) serveRepoConfig(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/contents/.github/smyklot.yaml"):
		s.writeFile(w, s.currentRepoConfig())

	case strings.HasSuffix(r.URL.Path, "/contents/.smyklot.toml"):
		s.writeFile(w, s.repoConfigTOML)

	default:
		s.writeFile(w, "")
	}
}

// serveGitData answers the object endpoints the configuration migration
// builds a commit from.
//
// Each one answers with a fixed sha: what the migration builds out of them is
// the thing under test, not what git would have made of it.
func (s *githubStub) serveGitData(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/git/ref/heads/"+migrationBranch):
		s.mu.Lock()
		sha := s.migrationRef
		s.mu.Unlock()

		if sha == "" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message": "Not Found"}`)

			return
		}

		_, _ = fmt.Fprintf(w, `{"object":{"sha":%q}}`, sha)

	case strings.Contains(r.URL.Path, "/git/ref/"):
		_, _ = io.WriteString(w, `{"object":{"sha":"basecommit"}}`)

	// A commit and the tree it records are different objects, and the tree is
	// what a new one is built from
	case strings.HasSuffix(r.URL.Path, "/git/commits/basecommit"):
		_, _ = io.WriteString(w, `{"sha":"basecommit","tree":{"sha":"basetree"}}`)

	// The tip of the migration branch, whose message is what says whether the
	// branch is still Smyklot's to rebuild.
	case strings.Contains(r.URL.Path, "/git/commits/"):
		s.mu.Lock()
		message := s.migrationTip
		s.mu.Unlock()

		_, _ = fmt.Fprintf(
			w, `{"sha":"commitsha","tree":{"sha":"treesha"},"message":%q}`, message,
		)

	case strings.HasSuffix(r.URL.Path, "/git/blobs"):
		s.record(&s.createdBlobs, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"sha":"blobsha"}`)

	case strings.HasSuffix(r.URL.Path, "/git/trees"):
		s.record(&s.createdTrees, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"sha":"treesha"}`)

	case strings.HasSuffix(r.URL.Path, "/git/commits"):
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"sha":"commitsha"}`)

	case strings.Contains(r.URL.Path, "/git/refs/heads/"):
		s.mu.Lock()
		s.migrationRef = "commitsha"
		s.forcedPushes++
		s.mu.Unlock()
		_, _ = io.WriteString(w, `{"object":{"sha":"commitsha"}}`)

	case strings.HasSuffix(r.URL.Path, "/git/refs"):
		if s.refuseBranchPush {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w,
				`{"message": "Resource not accessible by integration"}`)

			return
		}
		if s.busyOnBranchPush {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message": "API rate limit exceeded"}`)

			return
		}

		s.mu.Lock()
		s.migrationRef = "commitsha"
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"object":{"sha":"commitsha"}}`)

	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"message": "unstubbed git path %s"}`, r.URL.Path)
	}
}

// record keeps a request body for a spec to assert on.
func (s *githubStub) record(into *[]string, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	*into = append(*into, readBody(r))
}

// readBody reads a request body for a spec to assert on, and never fails: a
// stub that could not read one has nothing useful to say about it either.
func readBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}

	return string(body)
}

// orEmptyList keeps an unset listing decoding as an empty list rather than as
// a parse failure, which is what a stub answering nothing would produce.
func orEmptyList(body string) string {
	if body == "" {
		return "[]"
	}

	return body
}

func (s *githubStub) writeFile(w http.ResponseWriter, content string) {
	if content == "" {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))

		return
	}

	_, _ = io.WriteString(w, githubtest.ContentsResponse(content))
}

// countCalls reports how many recorded calls match method and path suffix
func (s *githubStub) countCalls(method, pathSuffix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0

	for _, call := range s.calls {
		if strings.HasPrefix(call, method+" ") && strings.HasSuffix(call, pathSuffix) {
			count++
		}
	}

	return count
}

func (s *githubStub) recordedCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.calls...)
}

// total reports how many calls the service made in all
func (s *githubStub) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.calls)
}
