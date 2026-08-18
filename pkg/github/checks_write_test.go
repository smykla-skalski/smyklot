package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("GitHub Check Run writes [Unit]", func() {
	It("creates a durable external check identity", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal("/repos/owner/repo/check-runs"))
			var payload map[string]any
			Expect(json.NewDecoder(r.Body).Decode(&payload)).To(Succeed())
			Expect(payload).To(HaveKeyWithValue("head_sha", "head"))
			Expect(payload).To(HaveKeyWithValue("external_id", "owned"))
			Expect(payload["actions"]).To(HaveLen(1))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 71, "name": "Smyklot / merge after CI", "head_sha": "head",
				"external_id": "owned", "status": "in_progress",
				"html_url": "https://github.example/checks/71", "app": map[string]any{"id": 17},
			})
		}))
		defer server.Close()
		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		run, err := client.CreateCheckRun(context.Background(), "owner", "repo", github.CheckRunWrite{
			Name: "Smyklot / merge after CI", HeadSHA: "head", ExternalID: "owned",
			Status: "in_progress", StartedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC),
			Output: github.CheckRunOutput{Title: "Merge authorized", Summary: "Waiting for CI"},
			Actions: []github.CheckRunAction{{
				Label: "Reauthorize", Description: "Authorize this revision", Identifier: "reauthorize",
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(run.ID).To(Equal(int64(71)))
		Expect(run.ExternalID).To(Equal("owned"))
		Expect(run.AppID).To(Equal(int64(17)))
	})

	It("clears stale requested actions when updating a run", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPatch))
			Expect(r.URL.Path).To(Equal("/repos/owner/repo/check-runs/71"))
			var payload map[string]any
			Expect(json.NewDecoder(r.Body).Decode(&payload)).To(Succeed())
			Expect(payload).NotTo(HaveKey("head_sha"))
			Expect(payload["actions"]).To(Equal([]any{}))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 71, "name": "Smyklot / merge after CI", "head_sha": "head",
				"external_id": "owned", "status": "in_progress", "app": map[string]any{"id": 17},
			})
		}))
		defer server.Close()
		client, err := github.NewClient("test-token", server.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.UpdateCheckRun(context.Background(), "owner", "repo", 71, github.CheckRunWrite{
			Name: "Smyklot / merge after CI", ExternalID: "owned", Status: "in_progress",
			Output: github.CheckRunOutput{Title: "Merge authorized", Summary: "Waiting for CI"},
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
