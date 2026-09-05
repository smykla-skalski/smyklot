package storagetest

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareServiceSampleSpecs(runtime queueRuntime) {
	It("folds repeated observations into one hour and keeps the worst", func() {
		ctx, store, now := runtime()
		hour := now.UTC().Truncate(time.Hour)

		Expect(store.RecordServiceSamples(ctx, []storage.ServiceSample{{
			Metric: storage.SampleQuery, Label: "Store.ListWorkQueue",
			SampledAt:    hour.Add(11 * time.Minute),
			Observations: 3, Failures: 1,
			Total: 30 * time.Millisecond, Max: 20 * time.Millisecond,
		}})).To(Succeed())

		Expect(store.RecordServiceSamples(ctx, []storage.ServiceSample{{
			Metric: storage.SampleQuery, Label: "Store.ListWorkQueue",
			SampledAt:    hour.Add(47 * time.Minute),
			Observations: 2, Failures: 0,
			Total: 10 * time.Millisecond, Max: 8 * time.Millisecond,
		}})).To(Succeed())

		samples, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleQuery,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(samples).To(HaveLen(1))
		Expect(samples[0].SampledAt).To(BeTemporally("==", hour))
		Expect(samples[0].Observations).To(Equal(int64(5)))
		Expect(samples[0].Failures).To(Equal(int64(1)))
		Expect(samples[0].Total).To(Equal(40 * time.Millisecond))
		Expect(samples[0].Max).To(Equal(20 * time.Millisecond))
		Expect(samples[0].Mean()).To(Equal(8 * time.Millisecond))
	})

	It("keeps an hour's highest gauge rather than its last", func() {
		ctx, store, now := runtime()
		hour := now.UTC().Truncate(time.Hour)
		gauge := func(value float64) []storage.ServiceSample {
			return []storage.ServiceSample{{
				Metric: storage.SampleLane, Label: "maintenance",
				SampledAt: hour, Value: value,
			}}
		}

		Expect(store.RecordServiceSamples(ctx, gauge(12))).To(Succeed())
		Expect(store.RecordServiceSamples(ctx, gauge(5000))).To(Succeed())
		Expect(store.RecordServiceSamples(ctx, gauge(0))).To(Succeed())

		samples, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleLane,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(samples).To(HaveLen(1))
		Expect(samples[0].Value).To(Equal(5000.0))
	})

	It("adds up a count of what happened rather than keeping its highest", func() {
		ctx, store, now := runtime()
		hour := now.UTC().Truncate(time.Hour)
		counted := func(value float64) []storage.ServiceSample {
			return []storage.ServiceSample{{
				Metric: storage.SampleDatabase, Label: "pool_waits",
				SampledAt: hour.Add(time.Duration(value) * time.Minute),
				Value:     value, Cumulative: true,
			}}
		}

		Expect(store.RecordServiceSamples(ctx, counted(10))).To(Succeed())
		Expect(store.RecordServiceSamples(ctx, counted(20))).To(Succeed())
		Expect(store.RecordServiceSamples(ctx, counted(5))).To(Succeed())

		samples, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleDatabase,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(samples).To(HaveLen(1))
		Expect(samples[0].SampledAt).To(BeTemporally("==", hour))
		Expect(samples[0].Value).To(Equal(35.0))
	})

	It("reads one metric's window, oldest first, and prunes behind it", func() {
		ctx, store, now := runtime()
		hour := now.UTC().Truncate(time.Hour)
		samples := make([]storage.ServiceSample, 0, 8)
		for age := range 4 {
			samples = append(samples,
				storage.ServiceSample{
					Metric: storage.SampleLedger, Label: "reaction_scan",
					SampledAt: hour.Add(-time.Duration(age) * time.Hour),
					Value:     float64(100 * age),
				},
				storage.ServiceSample{
					Metric: storage.SampleLedger, Label: "config_migration",
					SampledAt: hour.Add(-time.Duration(age) * time.Hour),
					Value:     float64(10 * age),
				},
			)
		}
		Expect(store.RecordServiceSamples(ctx, samples)).To(Succeed())

		window, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleLedger,
			Since:  hour.Add(-2 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(window).To(HaveLen(6))
		Expect(window[0].SampledAt).To(BeTemporally("==", hour.Add(-2*time.Hour)))
		Expect(window[5].SampledAt).To(BeTemporally("==", hour))
		Expect(window[0].Label).To(Equal("config_migration"))

		removed, err := store.PruneServiceSamples(ctx, hour.Add(-time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(4)))
		remaining, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleLedger,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining).To(HaveLen(4))
	})

	declareMeasurementSpecs(runtime)
}

func declareMeasurementSpecs(runtime queueRuntime) {
	It("counts how many rows each workload holds", func() {
		ctx, store, now := runtime()
		finished := now.Add(-time.Hour)
		for _, fixture := range []struct {
			id   string
			done *time.Time
		}{{id: "ledger:open"}, {id: "ledger:done", done: &finished}} {
			_, err := store.CreateQueueItem(ctx, workqueue.Item{
				ID: fixture.id, Kind: workqueue.KindReactionScan,
				Lane: workqueue.LaneMaintenance, Title: "Scan for new commands",
				State:      workqueue.StateScheduled,
				Priority:   workqueue.PriorityNormal,
				WindowMode: workqueue.WindowRespect,
				ProfileID:  pointer(workqueue.AlwaysOpenProfileID),
				NotBefore:  finished, EligibleAt: finished, FinishedAt: fixture.done,
				CreatedAt: finished, UpdatedAt: finished,
			})
			Expect(err).NotTo(HaveOccurred())
		}

		sizes, err := store.LedgerSizes(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sizes).To(ContainElement(And(
			HaveField("Kind", string(workqueue.KindReactionScan)),
			HaveField("Finished", int64(1)),
		)))
	})

	It("reads only the loudest series when it is given a limit", func() {
		ctx, store, now := runtime()
		hour := now.UTC().Truncate(time.Hour)
		samples := make([]storage.ServiceSample, 0, 13)
		for series := range 4 {
			for age := range 3 {
				samples = append(samples, storage.ServiceSample{
					Metric: storage.SampleQuery, Label: "statement-" + string(rune('a'+series)),
					SampledAt:    hour.Add(-time.Duration(age) * time.Hour),
					Observations: 200,
					Total:        time.Duration(series+1) * 100 * time.Millisecond,
					Max:          time.Duration(series+1) * time.Millisecond,
				})
			}
		}
		samples = append(samples, storage.ServiceSample{
			Metric: storage.SampleQuery, Label: "statement-spike",
			SampledAt: hour, Observations: 1,
			Total: 200 * time.Millisecond, Max: 200 * time.Millisecond,
		})
		Expect(store.RecordServiceSamples(ctx, samples)).To(Succeed())

		loudest, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleQuery, Limit: 2,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(loudest).To(HaveLen(6))
		labels := map[string]bool{}
		for _, sample := range loudest {
			labels[sample.Label] = true
		}
		Expect(labels).To(HaveLen(2))
		Expect(labels).To(HaveKey("statement-d"))
		Expect(labels).To(HaveKey("statement-c"))
		Expect(labels).NotTo(HaveKey("statement-spike"))

		windowed, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleQuery,
			Since:  hour.Add(-time.Hour), Until: hour,
			Limit: 2,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(windowed).To(HaveLen(4))
		for _, sample := range windowed {
			Expect(sample.Label).To(BeElementOf("statement-c", "statement-d"))
			Expect(sample.SampledAt).To(BeTemporally(">=", hour.Add(-time.Hour)))
			Expect(sample.SampledAt).To(BeTemporally("<=", hour))
		}

		beyond, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleQuery, Limit: 99,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(beyond).To(HaveLen(13))
	})

	It("counts what each lane has waiting and how long the oldest has waited", func() {
		ctx, store, now := runtime()
		created := now.Add(-25 * time.Minute)
		for _, fixture := range []struct {
			id    string
			state workqueue.State
		}{
			{id: "lane:waiting", state: workqueue.StateScheduled},
			{id: "lane:ready", state: workqueue.StateReady},
			{id: "lane:done", state: workqueue.StateSucceeded},
		} {
			_, err := store.CreateQueueItem(ctx, workqueue.Item{
				ID: fixture.id, Kind: workqueue.KindReactionScan,
				Lane: workqueue.LaneMaintenance, Title: "Scan for new commands",
				State:      fixture.state,
				Priority:   workqueue.PriorityNormal,
				WindowMode: workqueue.WindowRespect,
				ProfileID:  pointer(workqueue.AlwaysOpenProfileID),
				NotBefore:  created, EligibleAt: created,
				CreatedAt: created, UpdatedAt: created,
			})
			Expect(err).NotTo(HaveOccurred())
		}

		backlogs, err := store.LaneBacklogs(ctx, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(backlogs).To(ContainElement(And(
			HaveField("Lane", string(workqueue.LaneMaintenance)),
			HaveField("Depth", int64(2)),
			HaveField("Oldest", 25*time.Minute),
		)))
	})

	It("counts the statements it ran and starts again when they are read", func() {
		ctx, store, _ := runtime()

		_, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleQuery,
		})
		Expect(err).NotTo(HaveOccurred())

		stats := store.DrainQueryStats()
		Expect(stats).NotTo(BeEmpty())
		named := map[string]storage.QueryStats{}
		for _, stat := range stats {
			named[stat.Name] = stat
		}
		Expect(named).To(HaveKey("Store.ListServiceSamples"))
		Expect(named["Store.ListServiceSamples"].Observations).To(BeNumerically(">=", 1))
		Expect(named["Store.ListServiceSamples"].Total).To(BeNumerically(">", 0))

		_, err = store.ListAudit(ctx, "installation", storage.AuditPageRequest{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.ListRootAudit(ctx, storage.RootAuditPageRequest{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		shared := []string{}
		for _, stat := range store.DrainQueryStats() {
			if strings.HasSuffix(stat.Name, ".countHistory") {
				shared = append(shared, stat.Name)
			}
		}
		Expect(shared).To(HaveLen(2))

		Expect(store.DrainQueryStats()).To(BeEmpty())
	})

	declareStatementNamingSpecs(runtime)
}

func declareStatementNamingSpecs(runtime queueRuntime) {
	It("names a generic statement after the query rather than after its type shape", func() {
		ctx, store, now := runtime()
		store.DrainQueryStats()

		Expect(store.RecordServiceSamples(ctx, []storage.ServiceSample{{
			Metric: storage.SampleLedger, Label: "reaction_scan",
			SampledAt: now, Value: 1,
		}})).To(Succeed())

		stats := store.DrainQueryStats()
		Expect(stats).NotTo(BeEmpty())
		for _, stat := range stats {
			Expect(stat.Name).NotTo(ContainSubstring("["))
			Expect(stat.Name).NotTo(ContainSubstring("go.shape"))
			Expect(stat.Name).NotTo(ContainSubstring(" "))
		}
	})
}
