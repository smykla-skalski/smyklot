package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GitHub CI status [Unit]", func() {
	It("aggregates Check Runs and the latest legacy commit status", func() {
		server := newCIServer(
			[]map[string]interface{}{
				checkRun("build", 1, "completed", "success"),
				checkRun("test", 1, "in_progress", nil),
			},
			[]map[string]interface{}{
				{"context": "deploy", "state": "success"},
				{"context": "deploy", "state": "failure"},
			},
		)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatus(context.Background(), "owner", "repo", "abc123", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStatePending))
		Expect(status.Total).To(Equal(3))
		Expect(status.Passed).To(Equal(2))
		Expect(status.InProgress).To(Equal(1))
	})

	It("never treats unknown conclusions as green", func() {
		server := newCIServer(
			[]map[string]interface{}{checkRun("build", 1, "completed", "mystery")},
			nil,
		)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatus(context.Background(), "owner", "repo", "abc123", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStateIndeterminate))
		Expect(status.AllPassing).To(BeFalse())
		Expect(status.Unknown).To(Equal(1))
	})

	It("reports no checks instead of passing an empty commit", func() {
		server := newCIServer(nil, nil)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatus(context.Background(), "owner", "repo", "abc123", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStateNoChecks))
		Expect(status.AllPassing).To(BeFalse())
	})

	It("matches app-bound required checks and marks absent requirements missing", func() {
		server := newCIServer(
			[]map[string]interface{}{
				checkRun("build", 7, "completed", "success"),
				checkRun("build", 8, "completed", "failure"),
			},
			nil,
		)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		appID := int64(7)
		required := []github.RequiredCheck{
			{Context: "build", AppID: &appID},
			{Context: "test", AppID: &appID},
		}
		status, err := client.GetCheckStatus(context.Background(), "owner", "repo", "abc123", required)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStatePending))
		Expect(status.Total).To(Equal(2))
		Expect(status.Passed).To(Equal(1))
		Expect(status.Missing).To(Equal(1))
	})

	It("uses the safest result when an app-free requirement has multiple producers", func() {
		server := newCIServer(
			[]map[string]interface{}{checkRun("build", 7, "completed", "success")},
			[]map[string]interface{}{{"context": "build", "state": "failure"}},
		)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatus(
			context.Background(),
			"owner",
			"repo",
			"abc123",
			[]github.RequiredCheck{{Context: "build"}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStateFailing))
		Expect(status.Failed).To(Equal(1))
	})

	It("paginates Check Runs and commit statuses", func() {
		server := paginatedCIServer()
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatus(context.Background(), "owner", "repo", "abc123", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStatePassing))
		Expect(status.Total).To(Equal(202))
	})

	It("preserves app ids and removes legacy duplicates in branch requirements", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"contexts": []string{"build", "legacy"},
				"checks": []map[string]interface{}{
					{"context": "build", "app_id": 7},
				},
			})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		required, err := client.GetRequiredStatusChecks(context.Background(), "owner", "repo", "main")
		Expect(err).NotTo(HaveOccurred())
		Expect(required).To(HaveLen(2))
		Expect(required[0].Context).To(Equal("build"))
		Expect(required[0].AppID).NotTo(BeNil())
		Expect(*required[0].AppID).To(Equal(int64(7)))
		Expect(required[1]).To(Equal(github.RequiredCheck{Context: "legacy"}))
	})

	It("treats GitHub's app id minus one as any status producer", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"checks": []map[string]interface{}{{"context": "build", "app_id": -1}},
			})
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		required, err := client.GetRequiredStatusChecks(context.Background(), "owner", "repo", "main")
		Expect(err).NotTo(HaveOccurred())
		Expect(required).To(Equal([]github.RequiredCheck{{Context: "build"}}))
	})
})

func newCIServer(checkRuns, statuses []map[string]interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.Header.Get("Authorization")).To(Equal("token test-token"))
		switch r.URL.Path {
		case "/repos/owner/repo/commits/abc123/check-runs":
			Expect(r.URL.Query().Get("filter")).To(Equal("latest"))
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total_count": len(checkRuns),
				"check_runs":  checkRuns,
			})
		case "/repos/owner/repo/commits/abc123/status":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total_count": len(statuses),
				"statuses":    statuses,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func checkRun(name string, appID int64, status string, conclusion interface{}) map[string]interface{} {
	return map[string]interface{}{
		"name":       name,
		"status":     status,
		"conclusion": conclusion,
		"app":        map[string]interface{}{"id": appID},
	}
}

func paginatedCIServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		Expect(err).NotTo(HaveOccurred())
		switch r.URL.Path {
		case "/repos/owner/repo/commits/abc123/check-runs":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total_count": 101,
				"check_runs":  passingCheckRuns(page),
			})
		case "/repos/owner/repo/commits/abc123/status":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total_count": 101,
				"statuses":    passingCommitStatuses(page),
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func passingCheckRuns(page int) []map[string]interface{} {
	count := 100
	if page == 2 {
		count = 1
	}
	runs := make([]map[string]interface{}, 0, count)
	for index := 0; index < count; index++ {
		runs = append(runs, checkRun("check-"+strconv.Itoa((page-1)*100+index), 1, "completed", "success"))
	}

	return runs
}

func passingCommitStatuses(page int) []map[string]interface{} {
	count := 100
	if page == 2 {
		count = 1
	}
	statuses := make([]map[string]interface{}, 0, count)
	for index := 0; index < count; index++ {
		statuses = append(statuses, map[string]interface{}{
			"context": "status-" + strconv.Itoa((page-1)*100+index),
			"state":   "success",
		})
	}

	return statuses
}
