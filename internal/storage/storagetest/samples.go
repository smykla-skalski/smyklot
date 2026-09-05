package storagetest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// declareServiceSampleSpecs runs the measurement store's conformance suite on
// one engine.
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

		/* The same hour, seen again five minutes later: counts add and the worst
		   case is the worse of the two, or a sampler that visits an hour twelve
		   times reports the last five minutes as the whole hour. */
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

	It("replaces a gauge rather than adding to it", func() {
		ctx, store, now := runtime()
		hour := now.UTC().Truncate(time.Hour)
		gauge := func(value float64) []storage.ServiceSample {
			return []storage.ServiceSample{{
				Metric: storage.SampleLedger, Label: "reaction_scan",
				SampledAt: hour, Value: value,
			}}
		}

		Expect(store.RecordServiceSamples(ctx, gauge(4282))).To(Succeed())
		Expect(store.RecordServiceSamples(ctx, gauge(4310))).To(Succeed())

		samples, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleLedger,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(samples).To(HaveLen(1))
		Expect(samples[0].Value).To(Equal(4310.0))
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
			Labels: []string{"reaction_scan"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(window).To(HaveLen(3))
		Expect(window[0].SampledAt).To(BeTemporally("==", hour.Add(-2*time.Hour)))
		Expect(window[2].SampledAt).To(BeTemporally("==", hour))
		Expect(window[0].Label).To(Equal("reaction_scan"))

		removed, err := store.PruneServiceSamples(ctx, hour.Add(-time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(4)))
		remaining, err := store.ListServiceSamples(ctx, storage.ServiceSampleQuery{
			Metric: storage.SampleLedger,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining).To(HaveLen(4))
	})

	It("counts the statements it ran and starts again when they are read", func() {
		ctx, store, _ := runtime()

		/* Any read will do: what is being proven is that the store measures its
		   own work without a caller naming anything. */
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

		/* Draining resets, so each sample is its own hour rather than a running
		   total every reader would have to difference. */
		Expect(store.DrainQueryStats()).To(BeEmpty())
	})
}
