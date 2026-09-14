package gate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestReauthorizationPreviewDoesNotApprove(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                         string
		approved, self, failApproval bool
		writes                       int32
	}{
		{name: "approval needed", writes: 1},
		{name: "already approved", approved: true},
		{name: "self approval forbidden", self: true},
		{name: "approval write fails", failApproval: true, writes: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newReauthorizationPreviewHarness(t, test.approved, test.self, test.failApproval)
			check, err := harness.gate.checkPendingCIReauthorization(t.Context(), harness.candidate, harness.signal)
			if err != nil || check.allowed != !test.self || check.approvalRequired != (!test.approved && !test.self) {
				t.Fatalf("preview = %+v, %v", check, err)
			}
			if harness.approvals.Load() != 0 {
				t.Fatal("preview wrote an approval")
			}
			allowed, err := harness.gate.preparePendingCIReauthorization(t.Context(), harness.candidate, harness.signal)
			if (err != nil) != test.failApproval || allowed != (!test.self && !test.failApproval) {
				t.Fatalf("execution allowed = %v, error = %v", allowed, err)
			}

			if harness.approvals.Load() != test.writes {
				t.Fatalf("approval writes = %d, want %d", harness.approvals.Load(), test.writes)
			}
		})
	}
}

func TestReauthorizationExecutionRechecksPreviewPermission(t *testing.T) {
	t.Parallel()
	harness := newReauthorizationPreviewHarness(t, false, false, false)
	check, err := harness.gate.checkPendingCIReauthorization(t.Context(), harness.candidate, harness.signal)
	if err != nil || !check.allowed {
		t.Fatalf("preview = %+v, %v", check, err)
	}
	harness.denied.Store(true)
	allowed, err := harness.gate.preparePendingCIReauthorization(t.Context(), harness.candidate, harness.signal)
	if err != nil || allowed || harness.approvals.Load() != 0 {
		t.Fatalf("revoked execution = %v, %v, approvals = %d", allowed, err, harness.approvals.Load())
	}
}

type reauthorizationPreviewHarness struct {
	gate      *Gate
	candidate reauthorizationCandidate
	signal    pendingci.Signal
	approvals atomic.Int32
	denied    atomic.Bool
}

func newReauthorizationPreviewHarness(t *testing.T, approved, self, failApproval bool) *reauthorizationPreviewHarness {
	t.Helper()
	h := &reauthorizationPreviewHarness{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repository/pulls/42/reviews" {
			h.approvals.Add(1)
			if failApproval {
				w.WriteHeader(http.StatusUnprocessableEntity)
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if r.Method != http.MethodGet && r.URL.Path != "/graphql" {
			t.Errorf("unexpected write %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/repos/owner/repository/contents/.github/CODEOWNERS":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/owner/repository/collaborators/maintainer/permission":
			permission := "write"
			if h.denied.Load() {
				permission = "read"
			}
			_, _ = fmt.Fprintf(w, `{"permission":%q}`, permission)
		case "/repos/owner/repository/pulls/42":
			author := "contributor"
			if self {
				author = "maintainer"
			}
			_, _ = fmt.Fprintf(w, `{"state":"open","head":{"sha":"new-head"},"base":{"ref":"main"},"user":{"login":%q}}`, author)
		case "/repos/owner/repository/pulls/42/reviews":
			writeReauthorizationReviews(w, approved)
		case "/graphql":
			_, _ = w.Write([]byte(`{"data":{"repository":{"mergeQueue":null}}}`))
		case "/repos/owner/repository/branches/main/protection/required_status_checks":
			_, _ = w.Write([]byte(`{"checks":[{"context":"Smyklot","app_id":17}]}`))
		case "/repos/owner/repository/rules/branches/main":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected read %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	h.gate = &Gate{config: func(context.Context, *github.Client, string, string, string, string) (*config.Config, error) {
		return &config.Config{}, nil
	}}
	h.candidate = reauthorizationCandidate{slot: pendingci.CheckSlot{PullRequest: 42, Name: "Smyklot", AppID: 17}, request: pendingci.Request{CandidateBaseBranch: "main"}, client: client, owner: "owner", repository: "repository"}
	h.signal = pendingci.Signal{Actor: "maintainer", HeadSHA: "new-head"}
	return h
}

func writeReauthorizationReviews(w http.ResponseWriter, approved bool) {
	body := `[]`
	if approved {
		body = `[{"state":"APPROVED","user":{"login":"maintainer"}}]`
	}
	_, _ = w.Write([]byte(body))
}
