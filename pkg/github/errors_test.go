package github_test

import (
	"errors"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GitHub API errors [Unit]", func() {
	DescribeTable("classifies retryable provider outcomes",
		func(status int, detail string, expected bool) {
			apiErr := github.NewAPIError(
				github.ErrAPIRequest, status, http.MethodGet, "/repos", errors.New(detail),
			)
			var classified interface{ Retryable() bool }
			Expect(errors.As(apiErr, &classified)).To(BeTrue())
			Expect(classified.Retryable()).To(Equal(expected))
		},
		Entry("transport failure", 0, "connection reset", true),
		Entry("request timeout", http.StatusRequestTimeout, "timeout", true),
		Entry("head conflict", http.StatusConflict, "conflict", true),
		Entry("rate limit", http.StatusTooManyRequests, "rate limit", true),
		Entry("server failure", http.StatusBadGateway, "unavailable", true),
		Entry("secondary rate limit", http.StatusForbidden, "secondary rate limit", true),
		Entry("permanent denial", http.StatusForbidden, "forbidden", false),
		Entry("invalid request", http.StatusUnprocessableEntity, "invalid", false),
	)
})
