package workqueue

import (
	"testing"
	"time"
)

func TestPreviewDateUsesExecutionBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, zone, date string
		start, end       int
		opens, closes    string
	}{
		{"spring gap", "Europe/Warsaw", "2026-03-29", 150, 240, "2026-03-29T03:00:00+02:00", "2026-03-29T04:00:00+02:00"},
		{"repeated hour", "Europe/Warsaw", "2026-10-25", 135, 165, "2026-10-25T02:15:00+02:00", "2026-10-25T02:45:00+01:00"},
		{"removed window", "Europe/Warsaw", "2026-03-29", 135, 165, "", ""},
		{"end of day", "Europe/Warsaw", "2026-03-29", 0, 1440, "2026-03-29T00:00:00+01:00", "2026-03-30T00:00:00+02:00"},
		{"fractional offset", "Asia/Kathmandu", "2026-01-01", 540, 1020, "2026-01-01T09:00:00+05:45", "2026-01-01T17:00:00+05:45"},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := Profile{
				ID: "preview", Name: "Preview", Timezone: test.zone,
				Exceptions: []Exception{{Date: test.date, Start: test.start, End: test.end}},
			}
			preview, err := PreviewDate(profile, test.date)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Windows) != 1 {
				t.Fatalf("unexpected windows: %+v", preview)
			}
			got := preview.Windows[0]
			want := WindowPreview{StartMinute: test.start, EndMinute: test.end, OpensAt: test.opens, ClosesAt: test.closes, Available: test.opens != ""}
			if got != want {
				t.Fatalf("unexpected boundary preview: %+v", got)
			}
			verifyPreviewOpensForExecution(t, profile, got)
		})
	}
}

func TestPreviewDateHonorsExceptionsAndRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	profile := AlwaysOpenProfile(time.Now())
	profile.Exceptions = []Exception{{Date: "2026-12-25", Closed: true}}
	preview, err := PreviewDate(profile, "2026-12-25")
	if err != nil || preview.Windows == nil || len(preview.Windows) != 0 {
		t.Fatalf("closed date: %+v, %v", preview, err)
	}
	preview, err = PreviewDate(profile, "2026-12-26")
	if err != nil || len(preview.Windows) != 1 {
		t.Fatalf("weekly fallback: %+v, %v", preview, err)
	}
	if _, err := PreviewDate(profile, "2026-02-30"); err == nil {
		t.Fatal("accepted impossible date")
	}
	profile.Timezone = "Mars/Olympus"
	if _, err := PreviewDate(profile, "2026-12-26"); err == nil {
		t.Fatal("accepted invalid profile")
	}
}

func verifyPreviewOpensForExecution(t *testing.T, profile Profile, preview WindowPreview) {
	t.Helper()
	if !preview.Available {
		return
	}
	instant, err := time.Parse(time.RFC3339, preview.OpensAt)
	if err != nil {
		t.Fatal(err)
	}
	next, err := NextEligible(profile, instant)
	if err != nil || !next.Equal(instant) {
		t.Fatalf("preview disagrees with execution: %v, %v", next, err)
	}
}
