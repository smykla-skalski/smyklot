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

	It("excludes only the exact owned check run", func() {
		owned := checkRun("Smyklot / merge after CI", 17, "in_progress", nil)
		owned["external_id"] = "owned"
		server := newCIServer(
			[]map[string]interface{}{
				owned,
				checkRun("build", 7, "completed", "success"),
			},
			nil,
		)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		appID := int64(17)
		status, err := client.GetCheckStatusExcludingCheck(
			context.Background(), "owner", "repo", "abc123",
			[]github.RequiredCheck{
				{Context: "Smyklot / merge after CI", AppID: &appID},
				{Context: "build"},
			},
			github.OwnedCheck{
				Name: "Smyklot / merge after CI", AppID: appID, ExternalID: "owned",
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStatePassing))
		Expect(status.Total).To(Equal(1))
	})

	It("blocks when another run uses the owned check context", func() {
		owned := checkRun("Smyklot / merge after CI", 17, "in_progress", nil)
		owned["external_id"] = "owned"
		conflict := checkRun("Smyklot / merge after CI", 17, "completed", "success")
		conflict["external_id"] = "not-owned"
		server := newCIServer([]map[string]interface{}{owned, conflict}, nil)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatusExcludingCheck(
			context.Background(), "owner", "repo", "abc123", nil,
			github.OwnedCheck{
				Name: "Smyklot / merge after CI", AppID: 17, ExternalID: "owned",
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStatePending))
		Expect(status.AllPassing).To(BeFalse())
	})

	It("aggregates latest runs while proving exact owned identity from all runs", func() {
		owned := checkRun("Smyklot / merge after CI", 17, "in_progress", nil)
		owned["external_id"] = "owned:g2"
		oldOwned := checkRun("Smyklot / merge after CI", 17, "completed", "success")
		oldOwned["external_id"] = "owned"
		latestBuild := checkRun("build", 7, "completed", "success")
		failedBuild := checkRun("build", 7, "completed", "failure")
		server := newFilteredCIServer(
			[]map[string]interface{}{owned, oldOwned, latestBuild},
			[]map[string]interface{}{owned, oldOwned, latestBuild, failedBuild},
		)
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		status, err := client.GetCheckStatusExcludingCheck(
			context.Background(), "owner", "repo", "abc123", nil,
			github.OwnedCheck{
				Name: "Smyklot / merge after CI", AppID: 17, ExternalID: "owned:g2",
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(github.CIStatePassing))
		Expect(status.Total).To(Equal(1))
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

	It("combines classic and active ruleset requirements", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/owner/repo/branches/main/protection/required_status_checks":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"contexts": []string{"build", "legacy"},
					"checks":   []map[string]interface{}{{"context": "build", "app_id": 7}},
				})
			case "/repos/owner/repo/rules/branches/main":
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
					"type": "required_status_checks",
					"parameters": map[string]interface{}{
						"required_status_checks": []map[string]interface{}{
							{"context": "rules-build", "integration_id": 9},
						},
					},
				}})
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		required, err := client.GetRequiredStatusChecks(context.Background(), "owner", "repo", "main")
		Expect(err).NotTo(HaveOccurred())
		Expect(required).To(HaveLen(3))
		Expect(required[0].Context).To(Equal("build"))
		Expect(required[0].AppID).NotTo(BeNil())
		Expect(*required[0].AppID).To(Equal(int64(7)))
		Expect(required[1]).To(Equal(github.RequiredCheck{Context: "legacy"}))
		Expect(required[2].Context).To(Equal("rules-build"))
		Expect(required[2].AppID).NotTo(BeNil())
		Expect(*required[2].AppID).To(Equal(int64(9)))
	})

	It("reports required workflows separately from status contexts", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/owner/repo/branches/main/protection/required_status_checks":
				w.WriteHeader(http.StatusNotFound)
			case "/repos/owner/repo/rules/branches/main":
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{
					{"type": "workflows", "parameters": map[string]interface{}{
						"workflows": []map[string]interface{}{{
							"repository_id": 7, "path": ".github/workflows/ci.yml",
						}},
					}},
					{"type": "required_status_checks", "parameters": map[string]interface{}{
						"required_status_checks": []map[string]interface{}{{"context": "build"}},
					}},
				})
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		requirements, err := client.GetRequiredCIRequirements(
			context.Background(), "owner", "repo", "main",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(requirements.RequiredWorkflow).To(BeTrue())
		Expect(requirements.StatusChecks).To(Equal([]github.RequiredCheck{{Context: "build"}}))
	})

	It("does not retry an ambiguous Check Run creation", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"response lost"}`))
		}))
		defer server.Close()

		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.CreateCheckRun(context.Background(), "owner", "repo", github.CheckRunWrite{
			Name: "Smyklot / merge after CI", HeadSHA: "abc123", ExternalID: "owned",
			Status: "in_progress", Output: github.CheckRunOutput{Title: "Waiting", Summary: "Waiting"},
		})
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(1))
	})

	It("treats GitHub's app id minus one as any status producer", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/repos/owner/repo/rules/branches/main" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
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
			Expect(r.URL.Query().Get("filter")).To(Or(Equal("latest"), Equal("all")))
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

func newFilteredCIServer(
	latest, all []map[string]interface{},
) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/commits/abc123/check-runs":
			runs := latest
			if r.URL.Query().Get("filter") == "all" {
				runs = all
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total_count": len(runs), "check_runs": runs,
			})
		case "/repos/owner/repo/commits/abc123/status":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"total_count": 0, "statuses": []map[string]interface{}{},
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
