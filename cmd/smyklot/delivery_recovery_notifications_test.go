package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func runRecoveredNotification(f recoveryWorkerFixture) {
	GinkgoHelper()
	f.service.deliveries.Start(GinkgoT().Context())
	DeferCleanup(func() { Expect(f.service.deliveries.Shutdown(context.Background())).To(Succeed()) })
	Eventually(func() storage.DeliveryStatus {
		op, err := f.service.store.GetDeliveryOperation(GinkgoT().Context(), f.target, f.original)
		Expect(err).NotTo(HaveOccurred())
		Expect(op.Current.ID).NotTo(Equal(f.original))
		return op.Current.Status
	}).Within(eventuallyWindow).Should(Equal(storage.DeliverySucceeded))
	original, err := f.service.store.GetQueueItem(GinkgoT().Context(), fmt.Sprintf("delivery:%d", f.original))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(original.State)).To(Equal("failed"))
}

var _ = Describe("Recovered notification worker [Unit]", func() {
	It("wakes the matching CI request without merging from the saved check payload", func() {
		f := newRecoveryEventFixture(webhook.EventCheckRun, checkRunDelivery())
		armed := armWebhookTestRequest(f.service)
		lease, err := f.service.store.LeaseDue(GinkgoT().Context(), time.Now(), time.Now().Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Request.ID).To(Equal(armed.ID))
		preview := f.call(http.MethodGet)
		Expect(preview.Code).To(Equal(http.StatusOK), preview.Body.String())
		Expect(preview.Body.String()).To(ContainSubstring(`"available":true`))
		accepted := f.call(http.MethodPost)
		Expect(accepted.Code).To(Equal(http.StatusAccepted), accepted.Body.String())
		before, err := f.service.store.Get(GinkgoT().Context(), armed.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(before.LeaseExpiresAt).NotTo(BeNil(), "inspection/submission must not process the check event")
		runRecoveredNotification(f)
		after, err := f.service.store.Get(GinkgoT().Context(), armed.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.LastEventKey).To(ContainSubstring("check_run"))
		Expect(after.LeaseExpiresAt).To(BeNil())
		Expect(f.stub.recordedCalls()).NotTo(ContainElement(MatchRegexp(`^(POST|PUT|PATCH|DELETE) /repos/`)))
		Expect(f.call(http.MethodPost).Code).To(Equal(http.StatusOK))
	})

	It("refuses configuration recovery while its file connection is disabled", func() {
		payload := []byte(strings.ReplaceAll(string(configFilePushPayload("refs/heads/main", "main", false)), ".github", "smyklot"))
		f := newRecoveryEventFixture(webhook.EventPush, payload)
		preview := f.call(http.MethodGet)
		Expect(preview.Code).To(Equal(http.StatusOK), preview.Body.String())
		Expect(preview.Body.String()).To(ContainSubstring(`"available":false`))
		Expect(preview.Body.String()).To(ContainSubstring(`"reason":"configuration_disconnected"`))
		refused := f.call(http.MethodPost)
		Expect(refused.Code).To(Equal(http.StatusConflict), refused.Body.String())
		op, err := f.service.store.GetDeliveryOperation(GinkgoT().Context(), f.target, f.original)
		Expect(err).NotTo(HaveOccurred())
		Expect(op.Current.ID).To(Equal(f.original))
		Expect(op.Current.Status).To(Equal(storage.DeliveryFailed))
	})

	DescribeTable("queues a fresh configuration read subject to current connection state", func(disconnect bool) {
		payload := []byte(strings.ReplaceAll(string(configFilePushPayload("refs/heads/main", "main", false)), ".github", "smyklot"))
		f := newRecoveryEventFixture(webhook.EventPush, payload)
		setConnection := func(enabled bool) {
			repo, err := f.service.store.GetRepository(GinkgoT().Context(), f.target, storage.RepositoryID(123456))
			Expect(err).NotTo(HaveOccurred())
			target, err := f.service.store.GetTarget(GinkgoT().Context(), f.target)
			Expect(err).NotTo(HaveOccurred())
			_, err = f.service.store.SaveInstallationSettings(GinkgoT().Context(), storage.SaveInstallationSettingsRequest{
				TargetID: f.target, ActorAccountID: target.Account.ID, ChangedAt: time.Now(),
				Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: repo.ID, ConfigFileSyncEnabled: enabled, ExpectedRevision: repo.Revision}},
			})
			Expect(err).NotTo(HaveOccurred())
		}
		setConnection(true)
		count, err := f.service.store.DispatchConfigFileNotifications(GinkgoT().Context(), time.Now())
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1), "drain the activation notification before examining recovery")
		preview := f.call(http.MethodGet)
		Expect(preview.Code).To(Equal(http.StatusOK), preview.Body.String())
		Expect(preview.Body.String()).To(ContainSubstring(`"available":true`))
		accepted := f.call(http.MethodPost)
		Expect(accepted.Code).To(Equal(http.StatusAccepted), accepted.Body.String())
		count, err = f.service.store.DispatchConfigFileNotifications(GinkgoT().Context(), time.Now())
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeZero(), "inspection/submission must not process the push")
		if disconnect {
			setConnection(false)
		}
		runRecoveredNotification(f)
		count, err = f.service.store.DispatchConfigFileNotifications(GinkgoT().Context(), time.Now())
		Expect(err).NotTo(HaveOccurred())
		if disconnect {
			Expect(count).To(BeZero())
		} else {
			Expect(count).To(Equal(1))
		}
		Expect(f.stub.recordedCalls()).NotTo(ContainElement(MatchRegexp(`^(POST|PUT|PATCH|DELETE) /repos/`)))
		Expect(f.call(http.MethodPost).Code).To(Equal(http.StatusOK))
	}, Entry("connection still enabled", false), Entry("connection disabled after submission", true))
})
