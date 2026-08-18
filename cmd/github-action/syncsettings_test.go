package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// The settings kind is the only one whose actions are told apart by their
// subject rather than by their operation: a repository's settings and its
// Dependabot security updates are two requests to two endpoints, and the plan
// says which is which.
//
// So these specs are mostly about the subject. Sending one to the other's
// endpoint is a change GitHub ignores and this records as made, which is the
// failure the whole plan-then-apply split exists to prevent.
var _ = Describe("Applying a settings action [Unit]", func() {
	// Recorded in parts rather than as one line, so a spec asserts on the part
	// it is about: which endpoint, which verb, and - where there is one - what
	// was in the body.
	type request struct {
		method string
		path   string
		body   string
	}

	var (
		server   *httptest.Server
		requests []request
	)

	BeforeEach(func() { requests = nil })

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// Records every request, which for these two endpoints is the whole of what
	// happened: one answers with settings nobody reads back and the other
	// answers with nothing at all.
	serve := func() {
		server = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests = append(requests, request{
					method: r.Method, path: r.URL.Path, body: string(body),
				})

				if r.Method == http.MethodPut || r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)

					return
				}

				_, _ = io.WriteString(w, `{}`)
			}))
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	apply := func(action orgsync.Action) error {
		return applySettingsAction(GinkgoT().Context(), client(), "acme", "web", action)
	}

	It("sends the settings the plan named to the settings endpoint", func() {
		serve()

		Expect(apply(orgsync.Action{
			Kind:      orgsync.KindSettings,
			Operation: orgsync.OperationUpdate,
			Subject:   orgsync.SettingsSubject,
			Payload:   []byte(`{"has_wiki":false}`),
		})).To(Succeed())

		// Decoded rather than compared as text. The body is built by
		// go-github's encoder, which ends it with a newline, and pinning that
		// here would tie this spec to how a dependency serializes rather than
		// to which endpoint got which settings.
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].method).To(Equal(http.MethodPatch))
		Expect(requests[0].path).To(Equal("/repos/acme/web"))

		var sent map[string]any
		Expect(json.Unmarshal([]byte(requests[0].body), &sent)).To(Succeed())
		Expect(sent).To(Equal(map[string]any{"has_wiki": false}))
	})

	DescribeTable("switches Dependabot with the verb GitHub reads as the instruction",
		func(payload string, method string) {
			serve()

			Expect(apply(orgsync.Action{
				Kind:      orgsync.KindSettings,
				Operation: orgsync.OperationUpdate,
				Subject:   orgsync.DependabotSubject,
				Payload:   []byte(payload),
			})).To(Succeed())

			// No body at all: the verb is the whole instruction, and a request
			// carrying one would mean the client had invented a shape GitHub
			// does not read.
			Expect(requests).To(ConsistOf(request{
				method: method, path: "/repos/acme/web/automated-security-fixes",
			}))
		},
		Entry("switching them on", `{"enabled":true}`, http.MethodPut),
		Entry("switching them off", `{"enabled":false}`, http.MethodDelete),
	)

	// A subject this version does not know is refused rather than falling
	// through to whichever branch happens to be last. It cannot come from a plan
	// this version computed - which is exactly why the branch that would send it
	// to the settings endpoint is the one nobody would notice.
	It("refuses a subject it does not know", func() {
		serve()

		err := apply(orgsync.Action{
			Kind:      orgsync.KindSettings,
			Operation: orgsync.OperationUpdate,
			Subject:   "vulnerability_alerts",
			Payload:   []byte(`{"enabled":true}`),
		})

		Expect(err).To(MatchError(errSyncSubjectUnknown))
		Expect(requests).To(BeEmpty())
	})

	It("refuses an operation it does not know", func() {
		serve()

		err := apply(orgsync.Action{
			Kind:      orgsync.KindSettings,
			Operation: orgsync.OperationDelete,
			Subject:   orgsync.SettingsSubject,
			Payload:   []byte(`{"has_wiki":false}`),
		})

		Expect(err).To(MatchError(errSyncOperationUnknown))
		Expect(requests).To(BeEmpty())
	})

	It("refuses an action that says nothing at all", func() {
		serve()

		err := apply(orgsync.Action{
			Kind:      orgsync.KindSettings,
			Operation: orgsync.OperationUpdate,
			Subject:   orgsync.SettingsSubject,
		})

		Expect(err).To(MatchError(errSyncPayloadMissing))
		Expect(requests).To(BeEmpty())
	})
})
