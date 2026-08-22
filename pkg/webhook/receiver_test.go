package webhook_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type recordingObserver struct {
	received []string
}

func (r *recordingObserver) Received(_, outcome string) {
	r.received = append(r.received, outcome)
}

type erroringInbox struct{ webhook.Inbox }

func (erroringInbox) Claim(context.Context, webhook.Claim) (webhook.ClaimResult, error) {
	return webhook.ClaimResult{}, errors.New("database unavailable")
}

var _ = Describe("Receiver [Unit]", func() {
	var (
		observed *recordingObserver
		inbox    *webhook.MemoryInbox
		handled  chan webhook.Delivery
		screen   webhook.Screen
		attrs    int
	)

	BeforeEach(func() {
		observed = &recordingObserver{}
		inbox = webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
		handled = make(chan webhook.Delivery, 4)
		screen = nil
		attrs = 0
	})

	build := func(store webhook.Inbox) *webhook.Pipeline {
		GinkgoHelper()
		pipeline, err := webhook.New(
			[]byte(testSecret), store,
			func(_ context.Context, delivery webhook.Delivery) error {
				handled <- delivery

				return nil
			},
			webhook.Options{
				Events: []string{webhook.EventIssueComment},
				Screen: screen,
				Attrs: func(webhook.Delivery) []slog.Attr {
					attrs++

					return nil
				},
				Observer: webhook.Observer{Received: observed.Received},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		return pipeline
	}

	serve := func(request *http.Request) *httptest.ResponseRecorder {
		GinkgoHelper()
		response := httptest.NewRecorder()
		build(inbox).Receiver().ServeHTTP(response, request)

		return response
	}

	It("should answer a ping without reading anything", func() {
		response := serve(signed(webhook.EventPing, "d1", []byte(`{"zen":"hi"}`)))

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeIgnored}))
	})

	It("should refuse an event outside its list before parsing it", func() {
		response := serve(signed(webhook.EventCheckRun, "d1", []byte("not json")))

		Expect(response.Code).To(Equal(http.StatusNoContent))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeIgnored}))
	})

	It("should reject a delivery with no signature", func() {
		request := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		request.Header.Set(webhook.EventHeader, webhook.EventIssueComment)

		response := serve(request)

		Expect(response.Code).To(Equal(http.StatusUnauthorized))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeUnsigned}))
	})

	It("should reject a delivery signed with the wrong secret", func() {
		request := signed(webhook.EventIssueComment, "d1", comment("/approve"))
		request.Header.Set(webhook.SignatureHeader, "sha256=deadbeef")

		Expect(serve(request).Code).To(Equal(http.StatusUnauthorized))
	})

	It("should reject a body that is not JSON", func() {
		response := serve(signed(webhook.EventIssueComment, "d1", []byte("not json")))

		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeInvalid}))
	})

	It("should reject a payload carrying no installation", func() {
		body := []byte(`{"action":"created","repository":{"id":1,"name":"r","owner":{"login":"o"}}}`)

		Expect(serve(signed(webhook.EventIssueComment, "d1", body)).Code).
			To(Equal(http.StatusBadRequest))
	})

	It("should ignore a delivery the screen does not want", func() {
		screen = func(webhook.Delivery) (bool, error) { return false, nil }

		response := serve(signed(webhook.EventIssueComment, "d1", comment("hello")))

		Expect(response.Code).To(Equal(http.StatusNoContent))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeIgnored}))

		Expect(attrs).To(BeZero())
	})

	It("should reject a delivery the screen could not read", func() {
		screen = func(webhook.Delivery) (bool, error) { return false, errors.New("unreadable") }

		response := serve(signed(webhook.EventIssueComment, "d1", comment("hello")))

		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeInvalid}))
	})

	It("should accept a delivery and claim it exactly once", func() {
		pipeline := build(inbox)

		first := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(
			first, signed(webhook.EventIssueComment, "d1", comment("/approve")),
		)
		Expect(first.Code).To(Equal(http.StatusAccepted))

		second := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(
			second, signed(webhook.EventIssueComment, "d1", comment("/approve")),
		)
		Expect(second.Code).To(Equal(http.StatusAccepted))
		Expect(observed.received).
			To(Equal([]string{webhook.OutcomeAccepted, webhook.OutcomeDuplicate}))
	})

	It("should answer 503 when the inbox will not answer", func() {
		response := serve(signed(webhook.EventIssueComment, "d1", comment("/approve")))
		Expect(response.Code).To(Equal(http.StatusAccepted))

		observed.received = nil
		refused := httptest.NewRecorder()
		build(erroringInbox{}).Receiver().ServeHTTP(
			refused, signed(webhook.EventIssueComment, "d2", comment("/approve")),
		)

		Expect(refused.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(observed.received).To(Equal([]string{webhook.OutcomeRefused}))
	})

	It("should count an unknown event as other", func() {
		counted := make([]string, 0, 1)
		pipeline, err := webhook.New(
			[]byte(testSecret), inbox,
			func(context.Context, webhook.Delivery) error { return nil },
			webhook.Options{
				Events: []string{webhook.EventIssueComment},
				Observer: webhook.Observer{Received: func(event, _ string) {
					counted = append(counted, event)
				}},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		response := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(
			response, signed("<script>alert(1)</script>", "d1", []byte(`{}`)),
		)

		Expect(counted).To(Equal([]string{"other"}))
	})

	It("should deduplicate a header-less delivery by its body", func() {
		pipeline := build(inbox)
		body := comment("/approve")

		first := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(first, signed(webhook.EventIssueComment, "", body))
		second := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(second, signed(webhook.EventIssueComment, "", body))

		Expect(first.Code).To(Equal(http.StatusAccepted))
		Expect(second.Code).To(Equal(http.StatusAccepted))
		Expect(observed.received).
			To(Equal([]string{webhook.OutcomeAccepted, webhook.OutcomeDuplicate}))
	})

	It("should scrub the delivery header before it is used", func() {
		pipeline := build(inbox)
		pipeline.Start(GinkgoT().Context())
		defer func() { Expect(pipeline.Shutdown(GinkgoT().Context())).To(Succeed()) }()

		response := httptest.NewRecorder()
		pipeline.Receiver().ServeHTTP(
			response,
			signed(webhook.EventIssueComment, "abc\ndef ghi", comment("/approve")),
		)
		Expect(response.Code).To(Equal(http.StatusAccepted))

		Eventually(handled).Should(Receive(HaveField("ID", Equal("abcdefghi"))))
	})
})
