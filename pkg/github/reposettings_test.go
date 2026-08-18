package github_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Repository settings [Unit]", func() {
	var (
		server  *httptest.Server
		request *http.Request
		body    string
	)

	// Reset between specs. Without this the recorded request survives into the
	// next one, and a spec asserting that no request was made passes on the
	// previous spec's - which is the shape of a test that cannot fail.
	BeforeEach(func() {
		request = nil
		body = ""
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	serve := func(status int, answer string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			request = r
			body = string(raw)

			w.WriteHeader(status)
			_, _ = io.WriteString(w, answer)
		}))
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	Describe("reading", func() {
		It("reads the settings sync compares against", func() {
			server = serve(http.StatusOK, `{
				"allow_merge_commit": false, "allow_squash_merge": true,
				"allow_rebase_merge": false, "allow_auto_merge": true,
				"delete_branch_on_merge": true, "allow_update_branch": true,
				"squash_merge_commit_title": "PR_TITLE",
				"squash_merge_commit_message": "BLANK",
				"merge_commit_title": "MERGE_MESSAGE",
				"merge_commit_message": "PR_BODY",
				"has_issues": true, "has_projects": false,
				"has_wiki": false, "has_discussions": true
			}`)

			settings, err := client().GetRepositorySettings(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())

			Expect(settings).To(Equal(github.RepositorySettings{
				AllowSquashMerge: true, AllowAutoMerge: true,
				DeleteBranchOnMerge: true, AllowUpdateBranch: true,
				SquashMergeCommitTitle:   "PR_TITLE",
				SquashMergeCommitMessage: "BLANK",
				MergeCommitTitle:         "MERGE_MESSAGE",
				MergeCommitMessage:       "PR_BODY",
				HasIssues:                true, HasDiscussions: true,
			}))
			Expect(request.URL.Path).To(Equal("/repos/acme/web"))
		})

		// A feature the repository cannot have is left out of the answer
		// rather than reported off, and that difference is what stops a sync
		// asking for it on every run and being refused on every run
		It("tells a security feature that is off from one that is absent", func() {
			server = serve(http.StatusOK, `{"security_and_analysis":{
				"secret_scanning": {"status": "enabled"},
				"secret_scanning_push_protection": {"status": "disabled"}
			}}`)

			settings, err := client().GetRepositorySettings(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())

			Expect(settings.Security.SecretScanning.On()).To(BeTrue())
			Expect(settings.Security.SecretScanningPushProtection).NotTo(BeNil())
			Expect(settings.Security.SecretScanningPushProtection.On()).To(BeFalse())

			// Never mentioned, which is not the same as mentioned and off
			Expect(settings.Security.AdvancedSecurity).To(BeNil())
		})

		// The whole object is absent for a repository whose features nobody can
		// see, and a nil there must answer like every other absence rather than
		// panicking on the way past
		It("reads a repository that reports no security features at all", func() {
			server = serve(http.StatusOK, `{"has_wiki": true}`)

			settings, err := client().GetRepositorySettings(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())

			Expect(settings.Security.SecretScanning).To(BeNil())
			Expect(settings.Security.SecretScanning.On()).To(BeFalse())
		})

		// A setting GitHub reports as false has to read as false, not as
		// missing. The whole diff turns on telling those apart
		It("reads a setting that is off as off", func() {
			server = serve(http.StatusOK, `{"has_wiki": false, "has_issues": true}`)

			settings, err := client().GetRepositorySettings(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(settings.HasWiki).To(BeFalse())
			Expect(settings.HasIssues).To(BeTrue())
		})
	})

	Describe("updating", func() {
		// The body goes on the wire exactly as the plan decided it. This
		// endpoint replaces what it is sent, so which keys are present is the
		// whole instruction, and rebuilding it here would put a second author
		// between what somebody approved and what happens
		It("sends exactly the keys it was given", func() {
			server = serve(http.StatusOK, `{}`)

			Expect(client().UpdateRepositorySettings(
				context.Background(), "acme", "web",
				map[string]any{"has_wiki": false, "delete_branch_on_merge": true},
			)).To(Succeed())

			var sent map[string]any
			Expect(json.Unmarshal([]byte(body), &sent)).To(Succeed())
			Expect(sent).To(HaveLen(2))
			Expect(sent).To(HaveKeyWithValue("has_wiki", false))
			Expect(sent).To(HaveKeyWithValue("delete_branch_on_merge", true))

			Expect(request.Method).To(Equal(http.MethodPatch))
			Expect(request.URL.Path).To(Equal("/repos/acme/web"))
		})

		// A false has to survive to the wire. A body that dropped it would
		// leave the setting on, which is the direction that does damage
		It("sends a setting being turned off", func() {
			server = serve(http.StatusOK, `{}`)

			Expect(client().UpdateRepositorySettings(
				context.Background(), "acme", "web", map[string]any{"has_wiki": false},
			)).To(Succeed())

			Expect(body).To(ContainSubstring(`"has_wiki":false`))
		})

		// An empty PATCH can only fail or do nothing, so a caller that built
		// one has a bug worth naming rather than a request worth making
		It("refuses a change that changes nothing", func() {
			server = serve(http.StatusOK, `{}`)

			err := client().UpdateRepositorySettings(
				context.Background(), "acme", "web", nil)
			Expect(err).To(HaveOccurred())
			Expect(request).To(BeNil())
		})

		It("reports a refusal rather than swallowing it", func() {
			server = serve(http.StatusForbidden,
				`{"message":"Resource not accessible by integration"}`)

			err := client().UpdateRepositorySettings(
				context.Background(), "acme", "web", map[string]any{"has_wiki": false})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("403"))
		})
	})

	// Reported with the security features and changed nowhere near them. The
	// settings endpoint takes no key for it, so sending one would be a change
	// GitHub ignores and this records as made
	Describe("Dependabot security updates", func() {
		It("reads it beside the features it is reported with", func() {
			server = serve(http.StatusOK, `{"security_and_analysis":{
				"dependabot_security_updates": {"status": "enabled"}
			}}`)

			settings, err := client().GetRepositorySettings(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(settings.Security.DependabotSecurityUpdates.On()).To(BeTrue())
		})

		It("reads a repository that does not mention it as absent", func() {
			server = serve(http.StatusOK, `{"security_and_analysis":{
				"secret_scanning": {"status": "enabled"}
			}}`)

			settings, err := client().GetRepositorySettings(
				context.Background(), "acme", "web")
			Expect(err).NotTo(HaveOccurred())
			Expect(settings.Security.DependabotSecurityUpdates).To(BeNil())
		})

		// The verb is the instruction. Both directions are asserted because a
		// method that always sent one of them would satisfy the other spec
		DescribeTable("puts it where GitHub takes it",
			func(enable bool, method string) {
				server = serve(http.StatusNoContent, "")

				Expect(client().SetAutomatedSecurityFixes(
					context.Background(), "acme", "web", enable)).To(Succeed())

				Expect(request.Method).To(Equal(method))
				Expect(request.URL.Path).To(
					Equal("/repos/acme/web/automated-security-fixes"))
				Expect(body).To(BeEmpty())
			},
			Entry("switching it on", true, http.MethodPut),
			Entry("switching it off", false, http.MethodDelete),
		)

		It("reports a refusal rather than swallowing it", func() {
			server = serve(http.StatusUnprocessableEntity,
				`{"message":"Dependabot alerts are disabled"}`)

			err := client().SetAutomatedSecurityFixes(
				context.Background(), "acme", "web", true)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("422"))
		})
	})
})
