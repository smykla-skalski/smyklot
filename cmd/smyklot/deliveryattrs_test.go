package main

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

var _ = Describe("Delivery pull request attribution [Unit]", func() {
	attribute := func(payload []byte) int {
		GinkgoHelper()
		source, err := webhook.ParseSource(payload)
		Expect(err).NotTo(HaveOccurred())

		return deliveryPullRequest(webhook.Delivery{
			Event: webhook.EventCheckRun, Source: source, Payload: payload,
		})
	}

	It("should name the pull request a check run belongs to", func() {
		Expect(attribute(checkRunDelivery())).To(Equal(42))
	})

	It("should name none when the check run covers two pull requests", func() {
		two := strings.Replace(
			string(checkRunDelivery()),
			`"pull_requests": [{"number": 42}]`,
			`"pull_requests": [{"number": 42}, {"number": 43}]`,
			1,
		)
		Expect(two).To(ContainSubstring(`{"number": 43}`))

		Expect(attribute([]byte(two))).To(BeZero())
	})

	It("should name none when a comment payload cannot be read", func() {
		Expect(deliveryPullRequest(webhook.Delivery{
			Event: webhook.EventIssueComment, Payload: []byte("not json"),
		})).To(BeZero())
	})
})
