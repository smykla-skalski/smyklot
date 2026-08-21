package storagetest

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// declareOrgSyncSpecs covers what org sync needs from a database, on both
// engines. The invariants below are ones a second engine could plausibly fail
// on its own: a partial unique index, a read-then-write that has to hold under
// a connection pool, and an invalidation that has to share its transaction.
func declareOrgSyncSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	const (
		target = "github:installation:100"
		repoA  = "github:repository:1"
		repoB  = "github:repository:2"
	)

	// seed puts one installation with two repositories behind the port, which
	// is what every sync row references.
	seed := func(ctx context.Context, store storage.Store, now time.Time) storage.Account {
		GinkgoHelper()

		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{
			testInstallation(account, now, []storage.RepositorySnapshot{
				testRepository(repoA, "smykla-skalski/one", false),
				testRepository(repoB, "smykla-skalski/two", false),
			}),
		})).To(Succeed())

		return account
	}

	writeConfig := func(
		ctx context.Context, store storage.Store, actor string, now time.Time,
		document string, revision int64,
	) orgsync.Config {
		GinkgoHelper()

		config, err := store.SetSyncConfig(ctx, orgsync.ConfigChange{
			TargetID: target, Kind: orgsync.KindLabels, Enabled: true,
			Document: []byte(document), ActorID: actor, Now: now, Revision: revision,
		})
		Expect(err).NotTo(HaveOccurred())

		return config
	}

	planFor := func(
		ctx context.Context, store storage.Store, id, actor, digest string, now time.Time,
		actions []orgsync.Action,
	) orgsync.Plan {
		GinkgoHelper()

		plan, err := store.CreateSyncPlan(ctx, orgsync.PlanCreate{
			ID: id, TargetID: target, Trigger: orgsync.TriggerManual, ActorID: actor,
			Digest: digest, Actions: actions, Now: now, ExpiresAt: now.Add(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		return plan
	}

	// approveAndLease puts a plan into the state an executor holds it in, which
	// is the only state it may be closed from.
	approveAndLease := func(
		ctx context.Context, store storage.Store, actor, planID, digest string, now time.Time,
	) orgsync.PlanLease {
		GinkgoHelper()

		_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
			TargetID: target, PlanID: planID, Digest: digest, ActorID: actor, Now: now,
		})
		Expect(err).NotTo(HaveOccurred())

		lease, err := store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Found).To(BeTrue())

		return lease
	}

	action := func(repo string, operation orgsync.Operation, subject string) orgsync.Action {
		return orgsync.Action{
			RepositoryID: repo, Kind: orgsync.KindLabels,
			Operation: operation, Subject: subject, After: subject,
			Payload: []byte(`{"name":"` + subject + `","color":"d73a4a"}`),
		}
	}

	It("keeps a configuration, its fingerprint and its revision", func() {
		ctx, store, now := runtime()
		account := seed(ctx, store, now)

		written := writeConfig(ctx, store, account.ID, now, `{"labels":[]}`, 0)
		Expect(written.Revision).To(Equal(int64(1)))
		Expect(written.Digest).To(Equal(orgsync.DigestConfig(true, []byte(`{"labels":[]}`))))

		read, err := store.GetSyncConfig(ctx, target, orgsync.KindLabels)
		Expect(err).NotTo(HaveOccurred())
		Expect(read.Digest).To(Equal(written.Digest))

		// Byte for byte, on either engine. The fingerprint is taken from what
		// somebody saved, and a copy between engines moves the document and the
		// fingerprint independently - so a column that re-rendered its contents
		// would hand back a document its own fingerprint no longer describes.
		// PostgreSQL's JSONB does exactly that, which is why this column is not
		// one.
		Expect(string(read.Document)).To(Equal(`{"labels":[]}`))
		Expect(read.Enabled).To(BeTrue())
		Expect(read.UpdatedBy).To(Equal(account.ID))
		Expect(read.UpdatedAt).To(BeTemporally("==", now))

		listed, err := store.ListSyncConfigs(ctx, target)
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(HaveLen(1))
	})

	// A kind nobody has configured is not a kind configured and switched off,
	// and the fingerprint has to tell those apart
	It("reports a kind nobody has configured as absent", func() {
		ctx, store, now := runtime()
		seed(ctx, store, now)

		_, err := store.GetSyncConfig(ctx, target, orgsync.KindSettings)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	})

	// Two people editing the same label set from two tabs is the ordinary
	// case, and the one who saved second should be told rather than win
	It("refuses a write against a revision that has moved", func() {
		ctx, store, now := runtime()
		account := seed(ctx, store, now)
		writeConfig(ctx, store, account.ID, now, `{"labels":[]}`, 0)

		_, err := store.SetSyncConfig(ctx, orgsync.ConfigChange{
			TargetID: target, Kind: orgsync.KindLabels, Enabled: true,
			Document: []byte(`{"labels":[{}]}`), ActorID: account.ID, Now: now, Revision: 0,
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	})

	Describe("repository overrides", func() {
		It("keeps a repository's own answer, and tells it from inheriting", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			no := false

			written, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: repoA, Kind: orgsync.KindLabels, Enabled: &no,
					ActorID: account.ID, Now: now,
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(written.Enabled).To(HaveValue(BeFalse()))

			listed, err := store.ListSyncRepositoryOverrides(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed).To(HaveLen(1))
			Expect(listed[0].RepositoryID).To(Equal(repoA))
			Expect(listed[0].Enabled).To(HaveValue(BeFalse()))

			// Cleared back to inheriting, which is a third state and not the
			// same as saying no
			cleared, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: repoA, Kind: orgsync.KindLabels, Enabled: nil,
					ActorID: account.ID, Now: now, Revision: written.Revision,
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(cleared.Enabled).To(BeNil())

			listed, err = store.ListSyncRepositoryOverrides(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed[0].Enabled).To(BeNil())
		})

		It("keeps what a repository adjusts, byte for byte", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)

			// Spaced and ordered the way somebody typed it. The bytes are what
			// the digest beside them is taken from, so an engine that
			// re-rendered the document would disagree with the other about
			// whether a repository has settled.
			document := []byte(`{"merges":[{"path":"renovate.json",` +
				`"overrides":{"timezone":"Europe/Warsaw"}}]}`)

			written, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: repoA, Kind: orgsync.KindFiles, Document: document,
					ActorID: account.ID, Now: now,
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written.Document)).To(Equal(string(document)))

			listed, err := store.ListSyncRepositoryOverrides(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed).To(HaveLen(1))
			Expect(string(listed[0].Document)).To(Equal(string(document)))
			Expect(listed[0].Enabled).To(BeNil())
		})

		It("reads a repository that adjusts nothing as adjusting nothing", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			yes := true

			written, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: repoA, Kind: orgsync.KindFiles, Enabled: &yes,
					ActorID: account.ID, Now: now,
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written.Document)).To(Equal("{}"))

			listed, err := store.ListSyncRepositoryOverrides(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(listed[0].Document)).To(Equal("{}"))
		})

		It("reads one repository's answer without reading the installation's", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			no := false

			_, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: repoA, Kind: orgsync.KindLabels, Enabled: &no,
					ActorID: account.ID, Now: now,
				})
			Expect(err).NotTo(HaveOccurred())

			read, err := store.GetSyncRepositoryOverride(
				ctx, target, repoA, orgsync.KindLabels)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.RepositoryID).To(Equal(repoA))
			Expect(read.Kind).To(Equal(orgsync.KindLabels))
			Expect(read.Enabled).To(HaveValue(BeFalse()))

			// A kind this repository has said nothing about, and a repository
			// that has said nothing at all. Both are inheriting, which the
			// caller renders rather than reports.
			_, err = store.GetSyncRepositoryOverride(
				ctx, target, repoA, orgsync.KindFiles)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

			_, err = store.GetSyncRepositoryOverride(
				ctx, target, repoB, orgsync.KindLabels)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		// The row is keyed by repository, and the installation is reached
		// through the catalog. An identifier from one installation naming a
		// repository in another has to answer nothing rather than answer.
		It("will not read an override through the wrong installation", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			no := false

			_, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: repoA, Kind: orgsync.KindLabels, Enabled: &no,
					ActorID: account.ID, Now: now,
				})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.GetSyncRepositoryOverride(
				ctx, "github:test:target:absent", repoA, orgsync.KindLabels)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		It("refuses an override for a repository nothing knows about", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)
			yes := true

			_, err := store.SetSyncRepositoryOverride(
				ctx, orgsync.RepositoryOverrideChange{
					RepositoryID: "github:repository:404", Kind: orgsync.KindLabels,
					Enabled: &yes, ActorID: testAccount(now).ID, Now: now,
				})
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})
	})

	Describe("plans", func() {
		It("records a plan with its actions and counts", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)

			plan := planFor(ctx, store, "plan-1", account.ID, "digest-1", now, []orgsync.Action{
				action(repoA, orgsync.OperationCreate, "bug"),
				action(repoA, orgsync.OperationUpdate, "chore"),
				action(repoB, orgsync.OperationDelete, "wontfix"),
			})

			Expect(plan.Counts).To(Equal(orgsync.Counts{Create: 1, Update: 1, Delete: 1}))
			Expect(plan.State).To(Equal(orgsync.PlanComputed))

			read, actions, err := store.GetSyncPlan(ctx, target, plan.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.Counts).To(Equal(plan.Counts))
			Expect(read.ComputedAt).To(BeTemporally("==", now))
			Expect(actions).To(HaveLen(3))
			Expect(actions[0].State).To(Equal(orgsync.ActionPending))
			Expect(actions[0].RepositoryID).To(Equal(repoA))

			// The payload is what the executor applies, so it has to survive
			// storage exactly. An action that lost it would be an action the
			// executor could only guess at, and guessing applies something
			// nobody approved
			label, err := orgsync.DecodeLabel(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(label.Color).To(Equal("d73a4a"))

			live, _, err := store.GetLiveSyncPlan(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(live.ID).To(Equal(plan.ID))
		})

		// The partial unique index makes this a fact the database holds rather
		// than a convention its callers keep, so the reconcile loop cannot race
		// the panel and pressing "sync now" twice is idempotent
		It("allows one live plan per installation", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, err := store.CreateSyncPlan(ctx, orgsync.PlanCreate{
				ID: "plan-2", TargetID: target, Trigger: orgsync.TriggerReconcile,
				ActorID: account.ID, Digest: "digest-1", Now: now,
				ExpiresAt: now.Add(time.Hour),
			})
			Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		})

		It("frees the slot once a plan is finished", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)
			approveAndLease(ctx, store, account.ID, "plan-1", "digest-1", now)

			Expect(store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-1", State: orgsync.PlanApplied, Now: now,
			})).To(Succeed())

			planFor(ctx, store, "plan-2", account.ID, "digest-1", now, nil)
		})

		// Only the executor holding it may close a plan. Anything else is a
		// caller finishing work it never started
		It("refuses to close a plan nobody is applying", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			err := store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-1", State: orgsync.PlanApplied, Now: now,
			})
			Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		})

		It("discards a plan somebody declined", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			discarded, err := store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{
				TargetID: target, PlanID: "plan-1", ActorID: account.ID, Now: now,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(discarded.State).To(Equal(orgsync.PlanDiscarded))
			Expect(discarded.FinishedAt).To(HaveValue(BeTemporally("==", now)))

			// The slot is free again: a discarded plan is not live, so the next
			// sweep may compute a fresh one.
			_, _, err = store.GetLiveSyncPlan(ctx, target)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		// The same scoping approval carries, for the same reason: the plan
		// identifier alone would let somebody with rights over one installation
		// retire another's work
		It("refuses a discard that names another installation's plan", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, err := store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{
				TargetID: "github:installation:999", PlanID: "plan-1",
				ActorID: account.ID, Now: now,
			})
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanComputed))
		})

		// A plan an executor holds may be half applied; "discarded" would say
		// it never ran
		It("refuses to discard a plan that has left the reader's hands", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: target, PlanID: "plan-1", Digest: "digest-1",
				ActorID: account.ID, Now: now,
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())

			_, err = store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{
				TargetID: target, PlanID: "plan-1", ActorID: account.ID, Now: now,
			})
			Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		})

		It("approves a plan whose fingerprint still matches", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			approved, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: target, PlanID: "plan-1", Digest: "digest-1",
				ActorID: account.ID, Now: now,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(approved.State).To(Equal(orgsync.PlanApproved))
			Expect(approved.ApprovedAt).To(HaveValue(BeTemporally("==", now)))
		})

		// A plan identifier is a name for something the caller may never have
		// been authorized against. The panel checks "may you write to this
		// installation" and then names a plan; without the installation in the
		// query, somebody with rights over their own installation approves
		// another's work and it runs against that other's repositories
		It("refuses an approval that names another installation's plan", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: "github:installation:999", PlanID: "plan-1",
				Digest: "digest-1", ActorID: account.ID, Now: now,
			})
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

			// And the plan is untouched, rather than approved and merely
			// reported as missing
			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanComputed))
		})

		// The same second identifier, on the read side. Answering this would
		// hand one installation's repository names and labels to somebody with
		// no rights over it at all
		It("refuses to read another installation's plan", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, _, err := store.GetSyncPlan(ctx, "github:installation:999", "plan-1")
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		// The fingerprint is the only thing standing between what somebody
		// read and what runs
		It("refuses an approval carrying a fingerprint that has moved", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: target, PlanID: "plan-1", Digest: "digest-stale",
				ActorID: account.ID, Now: now,
			})
			Expect(errors.Is(err, orgsync.ErrStalePlan)).To(BeTrue())
		})

		// Checked in the approval itself rather than left to the sweeper, so
		// correctness never depends on the sweeper having run
		It("refuses an approval of a plan that outlived its window", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: target, PlanID: "plan-1", Digest: "digest-1",
				ActorID: account.ID, Now: now.Add(2 * time.Hour),
			})
			Expect(errors.Is(err, orgsync.ErrStalePlan)).To(BeTrue())
		})

		It("retires a plan nobody acted on, and frees the slot", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			Expect(store.ExpireSyncPlans(ctx, now.Add(2*time.Hour))).To(Succeed())

			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanExpired))

			_, _, err = store.GetLiveSyncPlan(ctx, target)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		// A plan an executor holds is not abandoned because its expiry passed.
		// It is being applied, and its lease is what says whether that is true
		It("leaves a plan being applied alone when its expiry passes", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)
			_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: target, PlanID: "plan-1", Digest: "digest-1",
				ActorID: account.ID, Now: now,
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())

			Expect(store.ExpireSyncPlans(ctx, now.Add(2*time.Hour))).To(Succeed())

			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanApplying))
		})
	})

	Describe("invalidation", func() {
		// Saving a label colour while a plan is on screen has to invalidate
		// that plan in the same transaction, or the plan stays approvable and
		// applies work nobody agreed to
		It("marks a live plan stale when the configuration changes", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			config := writeConfig(ctx, store, account.ID, now, `{"labels":[]}`, 0)
			planFor(ctx, store, "plan-1", account.ID, config.Digest, now, nil)

			writeConfig(ctx, store, account.ID, now.Add(time.Minute),
				`{"labels":[{"name":"bug"}]}`, config.Revision)

			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanStale))

			// And the slot is free, so the next planner can compute against
			// what the configuration now says
			planFor(ctx, store, "plan-2", account.ID, "digest-2", now, nil)
		})

		// The dangerous direction: a plan that went stale while an executor was
		// applying it must not go on to record what each repository now has.
		// Those digests are what the next reconcile trusts, and they would
		// describe a scope that has since moved - so every repository needing
		// exactly the change nobody applied would be skipped for ever.
		It("refuses to record a scope for a plan that went stale mid-flight", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			config := writeConfig(ctx, store, account.ID, now, `{"labels":[]}`, 0)
			planFor(ctx, store, "plan-1", account.ID, config.Digest, now, nil)
			approveAndLease(ctx, store, account.ID, "plan-1", config.Digest, now)

			// Somebody saves while the work is running.
			writeConfig(ctx, store, account.ID, now.Add(time.Minute),
				`{"labels":[{"name":"bug"}]}`, config.Revision)

			err := store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-1", State: orgsync.PlanApplied, Now: now,
				Applied: []orgsync.RepositoryState{{
					RepositoryID: repoA, Kind: orgsync.KindLabels,
					AppliedDigest: "digest-from-the-old-scope", AppliedAt: now,
				}},
			})
			Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

			// The plan stays stale rather than being reported as applied, and
			// the repository is left looking un-synchronised, which it is
			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanStale))

			state, err := store.ListSyncRepositoryState(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(BeEmpty())
		})

		// Turning a kind off for one repository removes its actions just as
		// surely as deleting a label does
		It("marks a live plan stale when a repository override changes", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)
			no := false

			_, err := store.SetSyncRepositoryOverride(ctx, orgsync.RepositoryOverrideChange{
				RepositoryID: repoA, Kind: orgsync.KindLabels, Enabled: &no,
				ActorID: account.ID, Now: now,
			})
			Expect(err).NotTo(HaveOccurred())

			read, _, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(read.State).To(Equal(orgsync.PlanStale))
		})
	})

	Describe("applying", func() {
		leaseOne := func(
			ctx context.Context, store storage.Store, account string, now time.Time,
			actions []orgsync.Action,
		) orgsync.PlanLease {
			GinkgoHelper()

			planFor(ctx, store, "plan-1", account, "digest-1", now, actions)
			_, err := store.ApproveSyncPlan(ctx, orgsync.PlanApproval{
				TargetID: target, PlanID: "plan-1", Digest: "digest-1",
				ActorID: account, Now: now,
			})
			Expect(err).NotTo(HaveOccurred())

			lease, err := store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())

			return lease
		}

		It("leases an approved plan with the work still to do", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)

			lease := leaseOne(ctx, store, account.ID, now, []orgsync.Action{
				action(repoA, orgsync.OperationCreate, "bug"),
			})

			Expect(lease.Found).To(BeTrue())
			Expect(lease.Plan.State).To(Equal(orgsync.PlanApplying))
			Expect(lease.Plan.Attempt).To(Equal(1))
			Expect(lease.Actions).To(HaveLen(1))
			Expect(lease.Actions[0].Subject).To(Equal("bug"))
		})

		// Nothing due is the ordinary answer on most ticks, not a failure
		It("reports nothing due without an error", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			lease, err := store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.Found).To(BeFalse())
		})

		// An executor that dies leaves work whose lease runs out, rather than a
		// plan stuck in applying for as long as nobody notices
		It("offers a plan again once its lease runs out", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			leaseOne(ctx, store, account.ID, now, nil)

			again, err := store.LeaseSyncPlan(ctx, now.Add(time.Minute), now.Add(2*time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(again.Found).To(BeTrue())
			Expect(again.Plan.Attempt).To(Equal(2))
		})

		It("does not offer a plan whose lease is still held", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			leaseOne(ctx, store, account.ID, now, nil)

			again, err := store.LeaseSyncPlan(ctx, now.Add(time.Second), now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(again.Found).To(BeFalse())
		})

		It("records what became of each action, and skips name the blocker", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			lease := leaseOne(ctx, store, account.ID, now, []orgsync.Action{
				action(repoA, orgsync.OperationCreate, "bug"),
				action(repoA, orgsync.OperationCreate, "chore"),
			})

			Expect(store.RecordSyncActionOutcome(ctx, orgsync.ActionOutcome{
				ActionID: lease.Actions[0].ID, State: orgsync.ActionFailed,
				Error: "422 invalid color",
			})).To(Succeed())
			Expect(store.RecordSyncActionOutcome(ctx, orgsync.ActionOutcome{
				ActionID: lease.Actions[1].ID, State: orgsync.ActionSkipped,
				Blocker: orgsync.KindLabels,
			})).To(Succeed())

			_, actions, err := store.GetSyncPlan(ctx, target, "plan-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(actions[0].State).To(Equal(orgsync.ActionFailed))
			Expect(actions[0].Error).To(Equal("422 invalid color"))
			Expect(actions[1].State).To(Equal(orgsync.ActionSkipped))
			Expect(actions[1].Blocker).To(Equal(orgsync.KindLabels))
		})

		// A retry sees everything, with what already happened recorded on it.
		//
		// Two things need that. It must not do finished work again - recreating
		// a label GitHub already made is a 422 that fails a repository for
		// having succeeded - and it must still be able to record the digest for
		// a kind that completed, which it can only do if that kind is still in
		// the work. Leasing only the pending actions dropped the second, so an
		// interrupted plan left the repository looking permanently
		// unsynchronised and re-read from GitHub on every tick after
		It("leases every action, carrying what an earlier attempt settled", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			lease := leaseOne(ctx, store, account.ID, now, []orgsync.Action{
				action(repoA, orgsync.OperationCreate, "bug"),
				action(repoA, orgsync.OperationCreate, "chore"),
			})
			Expect(store.RecordSyncActionOutcome(ctx, orgsync.ActionOutcome{
				ActionID: lease.Actions[0].ID, State: orgsync.ActionApplied,
			})).To(Succeed())

			again, err := store.LeaseSyncPlan(ctx, now.Add(time.Minute), now.Add(2*time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(again.Actions).To(HaveLen(2))

			bySubject := map[string]orgsync.ActionState{}
			for _, action := range again.Actions {
				bySubject[action.Subject] = action.State
			}
			Expect(bySubject["bug"]).To(Equal(orgsync.ActionApplied))
			Expect(bySubject["chore"]).To(Equal(orgsync.ActionPending))
		})

		// The digests are what the next reconcile trusts, so they are written
		// with the plan's own state rather than beside it
		It("records what a repository now has when the plan finishes", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			leaseOne(ctx, store, account.ID, now, nil)

			Expect(store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-1", State: orgsync.PlanApplied, Now: now,
				Applied: []orgsync.RepositoryState{{
					RepositoryID: repoA, Kind: orgsync.KindLabels,
					AppliedDigest: "digest-1", AppliedAt: now,
				}},
			})).To(Succeed())

			state, err := store.ListSyncRepositoryState(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].RepositoryID).To(Equal(repoA))
			Expect(state[0].AppliedDigest).To(Equal("digest-1"))
			Expect(state[0].AppliedAt).To(BeTemporally("==", now))
		})

		It("replaces what a repository had rather than adding beside it", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			leaseOne(ctx, store, account.ID, now, nil)
			applied := []orgsync.RepositoryState{{
				RepositoryID: repoA, Kind: orgsync.KindLabels,
				AppliedDigest: "digest-1", AppliedAt: now,
			}}
			Expect(store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-1", State: orgsync.PlanApplied, Now: now, Applied: applied,
			})).To(Succeed())

			later := now.Add(time.Hour)
			planFor(ctx, store, "plan-2", account.ID, "digest-2", later, nil)
			approveAndLease(ctx, store, account.ID, "plan-2", "digest-2", later)
			applied[0].AppliedDigest = "digest-2"
			applied[0].AppliedAt = later
			Expect(store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-2", State: orgsync.PlanApplied, Now: later, Applied: applied,
			})).To(Succeed())

			state, err := store.ListSyncRepositoryState(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].AppliedDigest).To(Equal("digest-2"))
		})
	})

	// What is known about one repository for one kind, which is either what it
	// has had applied or why nothing could be. One row holds both, so the two
	// cannot be true at once.
	Describe("repository paths", func() {
		It("keeps one list per repository and reads them back per installation", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target,
				Paths: []string{"README.md", ".github/workflows/test.yaml"}, ObservedAt: now,
				HeadSHA: "aaaa1111", Partial: true,
			})).To(Succeed())
			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoB, TargetID: target,
				Paths: []string{"README.md"}, ObservedAt: now,
			})).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(HaveLen(2))
			Expect(read[0].Paths).To(Equal([]string{"README.md", ".github/workflows/test.yaml"}))
			Expect(read[0].ObservedAt).To(BeTemporally("==", now))
			// The commit the list was read at, which is what lets a refresh
			// skip the tree. A row written without one reads back empty rather
			// than as a commit nothing can match.
			Expect(read[0].HeadSHA).To(Equal("aaaa1111"))
			// GitHub having declined to list one repository whole. Nothing
			// drops a path on purpose, so this is the only way a list is short.
			Expect(read[0].Partial).To(BeTrue())
			Expect(read[1].Paths).To(Equal([]string{"README.md"}))
			Expect(read[1].HeadSHA).To(BeEmpty())
			Expect(read[1].Partial).To(BeFalse())
		})

		// A picture of what a repository held, so a file somebody deleted stops
		// being offered. Merging would remember it for ever.
		It("replaces a list rather than merging into it", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target,
				Paths: []string{"gone.md", "README.md"}, ObservedAt: now,
			})).To(Succeed())
			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target,
				Paths: []string{"README.md"}, ObservedAt: now.Add(time.Minute),
			})).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(HaveLen(1))
			Expect(read[0].Paths).To(Equal([]string{"README.md"}))
		})

		// A repository read and holding nothing is not a repository nobody has
		// read, and a list that came back as one empty string would offer the
		// finder a path called "".
		It("reads an empty list back as no paths at all", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target, ObservedAt: now,
			})).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(HaveLen(1))
			Expect(read[0].Paths).To(BeEmpty())
		})

		// git permits a newline in a filename, and the list used to be stored
		// as one string with a newline between every path - so one such file
		// came back as two paths that do not exist, both offered to somebody
		// typing in the finder.
		It("keeps a path that holds a newline whole", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			awkward := []string{"docs/a\nb.md", "plain.md", `quote"and\backslash.md`}

			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target, Paths: awkward, ObservedAt: now,
			})).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(HaveLen(1))
			Expect(read[0].Paths).To(Equal(awkward))
		})

		// What a refresh actually asks of a stored row, without reading the one
		// column that can hold fifty thousand strings.
		It("describes a stored list without reading it", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target,
				Paths:      []string{"a.md", "b.md"},
				ObservedAt: now, HeadSHA: "abc123", Partial: true,
			})).To(Succeed())

			scans, err := store.ListSyncRepositoryPathScans(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(scans).To(HaveLen(1))
			Expect(scans[0].RepositoryID).To(Equal(repoA))
			Expect(scans[0].HeadSHA).To(Equal("abc123"))
			Expect(scans[0].Partial).To(BeTrue())
			Expect(scans[0].ObservedAt).To(BeTemporally("~", now, time.Second))
		})

		// The common branch of a sweep: the branch has not moved, so the list
		// this row holds is still the list, and the only thing to record is
		// that it was looked at. Writing that through the replace re-encoded
		// every path to move one column.
		It("records a list as current without rewriting it", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			paths := []string{"a.md", "b.md"}
			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target, Paths: paths,
				ObservedAt: now.Add(-time.Hour), HeadSHA: "abc123",
			})).To(Succeed())

			Expect(store.TouchSyncRepositoryPaths(ctx, repoA, now)).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(HaveLen(1))
			// The moment moved and nothing else did.
			Expect(read[0].ObservedAt).To(BeTemporally("~", now, time.Second))
			Expect(read[0].Paths).To(Equal(paths))
			Expect(read[0].HeadSHA).To(Equal("abc123"))
		})

		// A repository nothing has scanned yet has no row to bring forward, and
		// that is not a failure - the scan that follows writes one.
		It("says nothing about a repository with no list yet", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.TouchSyncRepositoryPaths(ctx, repoA, now)).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(BeEmpty())
		})

		// Nothing else removes these. The sweep writes a list per repository it
		// reads, and a repository that left the installation, or was archived,
		// or whose access was withdrawn is one it does not read - so its paths
		// stayed, and the finder went on offering files nobody can configure a
		// template at.
		It("drops the lists of repositories the installation no longer holds", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			for _, id := range []string{repoA, repoB} {
				Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
					RepositoryID: id, TargetID: target,
					Paths: []string{"README.md"}, ObservedAt: now,
				})).To(Succeed())
			}

			// One of them stops being synchronized, said the way the product
			// says it: the installation is reconciled without it, which is
			// what an archived repository and a withdrawn access both are.
			account := testAccount(now)
			Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{
				testInstallation(account, now, []storage.RepositorySnapshot{
					testRepository(repoA, "smykla-skalski/one", false),
				}),
			})).To(Succeed())

			dropped, err := store.PruneSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(dropped).To(BeNumerically("==", 1))

			read, err := store.ListSyncRepositoryPaths(ctx, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(HaveLen(1))
			Expect(read[0].RepositoryID).To(Equal(repoA))
		})

		// The scope of an installation is the catalog's, like every other read
		// of these tables.
		It("answers nothing for an installation that owns none of it", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
				RepositoryID: repoA, TargetID: target,
				Paths: []string{"README.md"}, ObservedAt: now,
			})).To(Succeed())

			read, err := store.ListSyncRepositoryPaths(ctx, "github:installation:999")
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(BeEmpty())
		})
	})

	Describe("repository state", func() {
		It("keeps why a repository could not be synced", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.RecordSyncRepositoryState(ctx, []orgsync.RepositoryState{{
				RepositoryID: repoA, Kind: orgsync.KindFiles, AppliedAt: now,
				Problem: "these files cannot be composed: docs is not a directory here",
			}})).To(Succeed())

			read, err := store.GetSyncRepositoryState(ctx, target, repoA, orgsync.KindFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.Problem).To(Equal(
				"these files cannot be composed: docs is not a directory here"))
			Expect(read.AppliedAt).To(BeTemporally("==", now))

			// And with no digest, which is what keeps a refusal from reading as
			// a repository that matches: the planner compares the stored digest
			// against the configured one, and an empty one matches nothing
			Expect(read.AppliedDigest).To(BeEmpty())
		})

		// A repository nothing has looked at is not a repository with a
		// problem, and a page that read the two the same way would report a
		// refusal on every repository in a fresh installation
		It("answers not-found where nothing has looked yet", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			_, err := store.GetSyncRepositoryState(ctx, target, repoA, orgsync.KindFiles)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		// A repository identifier names something the caller may never have
		// been authorized against, so the installation is a parameter here
		// rather than a check somebody remembers to make first. Asked with a
		// row that exists and an installation that does not own it, because the
		// suite seeds one installation and every other read in it would pass
		// with the scoping deleted.
		It("does not read a repository through another installation", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.RecordSyncRepositoryState(ctx, []orgsync.RepositoryState{{
				RepositoryID: repoA, Kind: orgsync.KindFiles, AppliedAt: now,
				Problem: "these files cannot be composed",
			}})).To(Succeed())

			_, err := store.GetSyncRepositoryState(
				ctx, "github:installation:999", repoA, orgsync.KindFiles)
			Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		})

		// The one that matters most: a repository that was refused and then
		// settles must not keep the refusal. It is the same row, so writing the
		// digest is what clears it - nothing has to remember to
		It("clears a refusal when the repository settles", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.RecordSyncRepositoryState(ctx, []orgsync.RepositoryState{{
				RepositoryID: repoA, Kind: orgsync.KindFiles, AppliedAt: now,
				Problem: "the adjustments saved for this repository cannot be used",
			}})).To(Succeed())

			later := now.Add(time.Hour)
			Expect(store.RecordSyncRepositoryState(ctx, []orgsync.RepositoryState{{
				RepositoryID: repoA, Kind: orgsync.KindFiles,
				AppliedDigest: "digest-1", AppliedAt: later,
			}})).To(Succeed())

			read, err := store.GetSyncRepositoryState(ctx, target, repoA, orgsync.KindFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.Problem).To(BeEmpty())
			Expect(read.AppliedDigest).To(Equal("digest-1"))
		})

		// A repository decides each kind on its own, and so does a refusal.
		// Reading one kind's row for another is how a repository whose files
		// cannot be composed would be reported as refusing its labels too
		It("keeps one kind's refusal out of another's", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			Expect(store.RecordSyncRepositoryState(ctx, []orgsync.RepositoryState{
				{
					RepositoryID: repoA, Kind: orgsync.KindFiles, AppliedAt: now,
					Problem: "these files cannot be composed",
				},
				{
					RepositoryID: repoA, Kind: orgsync.KindLabels,
					AppliedDigest: "digest-1", AppliedAt: now,
				},
			})).To(Succeed())

			labels, err := store.GetSyncRepositoryState(ctx, target, repoA, orgsync.KindLabels)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.Problem).To(BeEmpty())
			Expect(labels.AppliedDigest).To(Equal("digest-1"))

			files, err := store.GetSyncRepositoryState(ctx, target, repoA, orgsync.KindFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(files.Problem).To(Equal("these files cannot be composed"))
		})

		// Applying is the other way a repository settles, and it has to clear a
		// refusal too - otherwise a repository whose pull request landed would
		// go on saying its files could not be composed
		It("clears a refusal when a plan finishes against it", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)

			Expect(store.RecordSyncRepositoryState(ctx, []orgsync.RepositoryState{{
				RepositoryID: repoA, Kind: orgsync.KindLabels, AppliedAt: now,
				Problem: "something stopped it",
			}})).To(Succeed())

			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)
			approveAndLease(ctx, store, account.ID, "plan-1", "digest-1", now)

			Expect(store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
				PlanID: "plan-1", State: orgsync.PlanApplied, Now: now,
				Applied: []orgsync.RepositoryState{{
					RepositoryID: repoA, Kind: orgsync.KindLabels,
					AppliedDigest: "digest-1", AppliedAt: now,
				}},
			})).To(Succeed())

			read, err := store.GetSyncRepositoryState(ctx, target, repoA, orgsync.KindLabels)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.Problem).To(BeEmpty())
		})
	})

	Describe("audit", func() {
		// A sync entry has to reach the trunk as well as its own table, or the
		// history page cannot see it. Every other detail table does the same,
		// in the same transaction, for the same reason
		It("mirrors an entry into the audit trunk", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			Expect(store.RecordSyncAudit(ctx, orgsync.AuditEntry{
				TargetID: target, PlanID: "plan-1", ActorID: account.ID,
				Action: orgsync.AuditPlanned, Summary: "3 to add, 0 to change, 0 to remove",
				Counts: orgsync.Counts{Create: 3}, Now: now,
			})).To(Succeed())

			page, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
				Categories:         []storage.AuditCategory{storage.AuditCategorySync},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items).To(HaveLen(1))
			Expect(page.Items[0].Action).To(Equal(string(orgsync.AuditPlanned)))
			Expect(page.Items[0].Summary).To(ContainSubstring("3 to add"))
			Expect(page.Items[0].Category).To(Equal(storage.AuditCategorySync))
		})

		// The widened CHECK is the whole point of the migration that rebuilt
		// the largest table in the schema, so it is worth asserting rather than
		// assuming
		It("accepts the sync category the trunk was widened for", func() {
			ctx, store, now := runtime()
			account := seed(ctx, store, now)
			planFor(ctx, store, "plan-1", account.ID, "digest-1", now, nil)

			Expect(store.RecordSyncAudit(ctx, orgsync.AuditEntry{
				TargetID: target, PlanID: "plan-1", ActorID: account.ID,
				Action: orgsync.AuditFinished, Summary: "3 applied, 0 failed", Now: now,
			})).To(Succeed())
		})
	})

	Describe("refusals", func() {
		DescribeTable("refuses a plan that could not be applied safely",
			func(mutate func(*orgsync.PlanCreate)) {
				ctx, store, now := runtime()
				account := seed(ctx, store, now)

				create := orgsync.PlanCreate{
					ID: "plan-1", TargetID: target, Trigger: orgsync.TriggerManual,
					ActorID: account.ID, Digest: "digest-1", Now: now,
					ExpiresAt: now.Add(time.Hour),
				}
				mutate(&create)

				_, err := store.CreateSyncPlan(ctx, create)
				Expect(errors.Is(err, orgsync.ErrInvalidPlan)).To(BeTrue())
			},
			// A plan with no fingerprint could never be approved safely: there
			// would be nothing for the browser's copy to be checked against
			Entry("without a fingerprint", func(c *orgsync.PlanCreate) { c.Digest = "" }),
			Entry("without an actor", func(c *orgsync.PlanCreate) { c.ActorID = "" }),
			Entry("without an expiry", func(c *orgsync.PlanCreate) { c.ExpiresAt = time.Time{} }),
			Entry("with an unknown trigger", func(c *orgsync.PlanCreate) { c.Trigger = "cron" }),
			Entry("with an action naming no kind", func(c *orgsync.PlanCreate) {
				c.Actions = []orgsync.Action{{RepositoryID: repoA, Subject: "bug"}}
			}),
			Entry("with an action naming no subject", func(c *orgsync.PlanCreate) {
				c.Actions = []orgsync.Action{{RepositoryID: repoA, Kind: orgsync.KindLabels}}
			}),
		)

		It("refuses a configuration for a kind it does not know", func() {
			ctx, store, now := runtime()
			seed(ctx, store, now)

			_, err := store.SetSyncConfig(ctx, orgsync.ConfigChange{
				TargetID: target, Kind: "workflows", ActorID: testAccount(now).ID, Now: now,
			})
			Expect(err).To(HaveOccurred())
		})
	})
}
