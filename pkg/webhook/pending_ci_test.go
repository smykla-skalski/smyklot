package webhook_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const pendingCICommon = `
"repository": {
  "id": 9001,
  "name": "smyklot",
  "full_name": "smykla-skalski/smyklot",
  "owner": {"login": "smykla-skalski"}
},
"installation": {"id": 77}`

var _ = Describe("pending CI webhook notifications [Unit]", func() {
	It("maps check runs to their pull requests and a stable event key", func() {
		body := []byte(`{
"action": "completed",
"check_run": {
  "id": 501,
  "head_sha": "abc123",
  "status": "completed",
  "conclusion": "success",
  "updated_at": "2026-08-15T12:00:00Z",
  "pull_requests": [{"number": 198}, {"number": 199}]
},` + pendingCICommon + `}`)
		notification, err := webhook.ParsePendingCINotification(webhook.EventCheckRun, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(notification.Metadata.RepositoryID).To(Equal(int64(9001)))
		Expect(notification.Metadata.InstallationID).To(Equal(int64(77)))
		Expect(notification.Signals).To(HaveLen(2))
		Expect(notification.Signals[0]).To(Equal(webhook.PendingCISignal{
			Kind:        webhook.SignalWakePullRequest,
			PullRequest: 198,
			HeadSHA:     "abc123",
			MatchHead:   true,
			EventKey:    notification.Key,
		}))

		redelivery, err := webhook.ParsePendingCINotification(webhook.EventCheckRun, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(redelivery.Key).To(Equal(notification.Key))
	})

	It("correlates a requested reauthorization action to the exact check run", func() {
		body := []byte(`{
"action": "requested_action",
"check_run": {
  "id": 701,
  "name": "Smyklot / merge after CI",
  "external_id": "smyklot:merge-after-ci:9001:new-head",
  "head_sha": "new-head",
  "app": {"id": 17},
  "pull_requests": [{"number": 198}]
},
"requested_action": {"identifier": "reauthorize"},
"sender": {"login": "maintainer"},` + pendingCICommon + `}`)

		notification, err := webhook.ParsePendingCINotification(webhook.EventCheckRun, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(notification.Action).To(Equal("requested_action"))
		Expect(notification.Signals).To(ConsistOf(webhook.PendingCISignal{
			Kind: webhook.SignalReauthorize, PullRequest: 198, HeadSHA: "new-head",
			EventKey: notification.Key, Actor: "maintainer", CheckRunID: 701,
			CheckName:  "Smyklot / merge after CI",
			ExternalID: "smyklot:merge-after-ci:9001:new-head",
			AppID:      17, ActionID: "reauthorize",
		}))
	})

	It("falls back to matching a check suite by head SHA", func() {
		body := []byte(`{
"action": "completed",
"check_suite": {
  "id": 601,
  "head_sha": "fork-head",
  "status": "completed",
  "conclusion": "failure",
  "updated_at": "2026-08-15T12:01:00Z",
  "pull_requests": []
},` + pendingCICommon + `}`)
		notification, err := webhook.ParsePendingCINotification(webhook.EventCheckSuite, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(notification.Signals).To(ConsistOf(webhook.PendingCISignal{
			Kind: webhook.SignalWakeHead, HeadSHA: "fork-head", EventKey: notification.Key,
		}))
	})

	It("distinguishes check run and suite rerequests for durable repair", func() {
		checkRun := []byte(`{
"action": "rerequested",
"check_run": {
  "id": 701,
  "name": "Smyklot / merge after CI",
  "external_id": "smyklot:merge-after-ci:9001:head",
  "head_sha": "head",
  "app": {"id": 17},
  "pull_requests": [{"number": 198}]
},` + pendingCICommon + `}`)
		runNotification, err := webhook.ParsePendingCINotification(
			webhook.EventCheckRun, checkRun,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(runNotification.Signals).To(ConsistOf(webhook.PendingCISignal{
			Kind: webhook.SignalRerequestCheck, PullRequest: 198,
			HeadSHA: "head", MatchHead: true, EventKey: runNotification.Key,
			CheckRunID: 701, CheckName: "Smyklot / merge after CI",
			ExternalID: "smyklot:merge-after-ci:9001:head", AppID: 17,
		}))

		checkSuite := []byte(`{
"action": "rerequested",
"check_suite": {
  "id": 801,
  "head_sha": "head",
  "app": {"id": 17},
  "pull_requests": []
},` + pendingCICommon + `}`)
		suiteNotification, err := webhook.ParsePendingCINotification(
			webhook.EventCheckSuite, checkSuite,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(suiteNotification.Signals).To(ConsistOf(webhook.PendingCISignal{
			Kind: webhook.SignalRerequestCheck, HeadSHA: "head",
			EventKey: suiteNotification.Key, CheckRunID: 801, AppID: 17,
		}))
	})

	It("maps legacy commit statuses by head SHA", func() {
		body := []byte(`{
"sha": "legacy-head",
"context": "ci/build",
"state": "success",
"updated_at": "2026-08-15T12:02:00Z",` + pendingCICommon + `}`)
		notification, err := webhook.ParsePendingCINotification(webhook.EventStatus, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(notification.Action).To(Equal("success"))
		Expect(notification.Signals).To(ConsistOf(webhook.PendingCISignal{
			Kind: webhook.SignalWakeHead, HeadSHA: "legacy-head", EventKey: notification.Key,
		}))
	})

	It("distinguishes pull request wakes, completion, and label removal", func() {
		payload := func(action, label string, merged bool) []byte {
			return []byte(`{
"action": "` + action + `",
"number": 198,
"pull_request": {
  "merged": ` + fmt.Sprint(merged) + `,
  "updated_at": "2026-08-15T12:03:00Z",
  "head": {"sha": "new-head"}
},
"label": {"name": "` + label + `"},` + pendingCICommon + `}`)
		}

		synchronized, err := webhook.ParsePendingCINotification(
			webhook.EventPullRequest,
			payload("synchronize", "", false),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(synchronized.Signals[0].Kind).To(Equal(webhook.SignalWakePullRequest))
		opened, err := webhook.ParsePendingCINotification(
			webhook.EventPullRequest,
			payload("opened", "", false),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(opened.Signals[0].Kind).To(Equal(webhook.SignalWakePullRequest))

		closed, err := webhook.ParsePendingCINotification(
			webhook.EventPullRequest,
			payload("closed", "", true),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(closed.Signals[0].Kind).To(Equal(webhook.SignalPullRequestDone))
		Expect(closed.Signals[0].Merged).To(BeTrue())

		unlabeled, err := webhook.ParsePendingCINotification(
			webhook.EventPullRequest,
			payload("unlabeled", "smyklot:pending:ci:squash", false),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(unlabeled.Signals[0].Kind).To(Equal(webhook.SignalLabelRemoved))
		Expect(unlabeled.Signals[0].Label).To(Equal("smyklot:pending:ci:squash"))
	})

	It("rejects missing event identity and common metadata", func() {
		_, err := webhook.ParsePendingCINotification(
			webhook.EventStatus,
			[]byte(`{"sha":"abc","context":"build","state":"success"}`),
		)
		Expect(err).To(MatchError(webhook.ErrNoInstallation))

		_, err = webhook.ParsePendingCINotification(
			webhook.EventCheckRun,
			[]byte(`{"action":"completed",`+pendingCICommon+`}`),
		)
		Expect(err).To(MatchError(ContainSubstring("missing action, id, or head SHA")))
	})
})
