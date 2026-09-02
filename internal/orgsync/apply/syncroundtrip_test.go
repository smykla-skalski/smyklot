package apply

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

// WHAT THE PLANNER WRITES IS WHAT THE EXECUTOR READS.
//
// Every other spec in this package hands `applyX` a payload written by hand, so
// they prove the executor reads the shape the SPEC believes in. Nothing proved
// the planner produces that shape - and it did not have to, while a payload was
// the applied object and nothing else.
//
// It is not any more. A settings payload carries the request body beside what a
// reader has to be told about sending it, and a label payload carries the label
// it replaces beside the one to write, so both are wrappers the executor has to
// open. Get either unwrapping wrong and every sync of that kind fails at its
// last step, in production, with every unit spec in this package still green.
//
// So these plan the change and apply the action the planner returned, asserting
// on what reached GitHub. The two halves cannot drift without this failing.
var _ = Describe("A plan applied as the planner wrote it [Unit]", func() {
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

	serve := func() {
		server = httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests = append(requests, request{
					method: r.Method, path: r.URL.Path, body: string(body),
				})

				if r.Method == http.MethodDelete {
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

	describes := func(value string) *string { return &value }

	Describe("settings", func() {
		It("sends the body the planner put in the payload, and nothing beside it", func() {
			serve()

			actions := orgsync.PlanSettings("repository-1",
				orgsync.SettingsConfig{HasWiki: new(bool)},
				orgsync.CurrentSettings{HasWiki: true},
			)
			Expect(actions).To(HaveLen(1))

			// The payload is a wrapper, and the reader's half of it must not
			// reach GitHub: `{"body":{...},"changes":[...]}` sent to the
			// settings endpoint is a request that changes nothing and reports
			// success.
			plan, err := orgsync.DecodeSettingsPlan(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Changes).NotTo(BeEmpty())

			Expect(applySettingsAction(
				GinkgoT().Context(), client(), "acme", "web", actions[0],
			)).To(Succeed())

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].method).To(Equal(http.MethodPatch))
			Expect(requests[0].path).To(Equal("/repos/acme/web"))

			var sent map[string]any
			Expect(json.Unmarshal([]byte(requests[0].body), &sent)).To(Succeed())
			Expect(sent).To(Equal(map[string]any{"has_wiki": false}))
		})
	})

	Describe("labels", func() {
		engine := &Engine{}

		It("creates the label the planner resolved", func() {
			serve()

			actions := orgsync.PlanLabels("repository-1",
				orgsync.LabelConfig{Labels: []orgsync.Label{
					{Name: "bug", Color: "D73A4A", Description: describes("Something is broken")},
				}},
				nil,
				orgsync.Excludes{},
			)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))

			Expect(engine.applyLabelAction(
				GinkgoT().Context(), client(), "acme", "web", actions[0],
			)).To(Succeed())

			Expect(requests).To(HaveLen(1))
			var sent map[string]any
			Expect(json.Unmarshal([]byte(requests[0].body), &sent)).To(Succeed())
			Expect(sent).To(Equal(map[string]any{
				"name": "bug", "color": "d73a4a", "description": "Something is broken",
			}))
		})

		// The update payload carries the label it replaces as well. Only one of
		// the two may be sent, and it has to be the wanted one.
		It("sends only the wanted side of a change", func() {
			serve()

			actions := orgsync.PlanLabels("repository-1",
				orgsync.LabelConfig{Labels: []orgsync.Label{
					{Name: "bug", Color: "d73a4a", Description: describes("Something is broken")},
				}},
				[]orgsync.CurrentLabel{
					{Name: "bug", Color: "ff8800", Description: "Something is broke"},
				},
				orgsync.Excludes{},
			)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))

			plan, err := orgsync.DecodeLabelPlan(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Previous).NotTo(BeNil())
			Expect(plan.Previous.Color).To(Equal("ff8800"))

			Expect(engine.applyLabelAction(
				GinkgoT().Context(), client(), "acme", "web", actions[0],
			)).To(Succeed())

			Expect(requests).To(HaveLen(1))
			var sent map[string]any
			Expect(json.Unmarshal([]byte(requests[0].body), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("color", "d73a4a"))
			Expect(sent).NotTo(HaveKey("previous"))
			Expect(sent).NotTo(HaveKey("label"))
		})

		// A deletion carries its label now, for a reader that draws what is
		// going. Apply answers from the subject and must not start reading it.
		It("removes by subject, whatever the payload holds", func() {
			serve()

			actions := orgsync.PlanLabels("repository-1",
				orgsync.LabelConfig{AllowRemoval: true},
				[]orgsync.CurrentLabel{{Name: "wontfix", Color: "ffffff"}},
				orgsync.Excludes{},
			)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationDelete))
			Expect(actions[0].Payload).NotTo(BeEmpty())

			Expect(engine.applyLabelAction(
				GinkgoT().Context(), client(), "acme", "web", actions[0],
			)).To(Succeed())

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].method).To(Equal(http.MethodDelete))
			Expect(requests[0].path).To(Equal("/repos/acme/web/labels/wontfix"))
			Expect(requests[0].body).To(BeEmpty())
		})
	})

	// The straddle: a plan computed before the payloads became wrappers is in
	// the store for hours after a deploy, and still has to apply.
	Describe("a payload written by the version before this one", func() {
		It("applies a bare settings body", func() {
			serve()

			Expect(applySettingsAction(GinkgoT().Context(), client(), "acme", "web",
				orgsync.Action{
					Kind:      orgsync.KindSettings,
					Operation: orgsync.OperationUpdate,
					Subject:   orgsync.SettingsSubject,
					Payload:   []byte(`{"has_wiki":false,"allow_auto_merge":true}`),
				})).To(Succeed())

			Expect(requests).To(HaveLen(1))
			var sent map[string]any
			Expect(json.Unmarshal([]byte(requests[0].body), &sent)).To(Succeed())
			Expect(sent).To(Equal(map[string]any{
				"has_wiki": false, "allow_auto_merge": true,
			}))
		})

		It("applies a bare label", func() {
			serve()

			engine := &Engine{}
			Expect(engine.applyLabelAction(GinkgoT().Context(), client(), "acme", "web",
				orgsync.Action{
					Kind:      orgsync.KindLabels,
					Operation: orgsync.OperationCreate,
					Subject:   "bug",
					Payload:   []byte(`{"name":"bug","color":"d73a4a","description":"Broken"}`),
				})).To(Succeed())

			Expect(requests).To(HaveLen(1))
			var sent map[string]any
			Expect(json.Unmarshal([]byte(requests[0].body), &sent)).To(Succeed())
			Expect(sent).To(Equal(map[string]any{
				"name": "bug", "color": "d73a4a", "description": "Broken",
			}))
		})
	})
})
