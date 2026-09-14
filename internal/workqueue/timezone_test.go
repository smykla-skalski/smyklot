package workqueue

import (
	"testing"
	"time"
)

func TestTimezonePreviewUsesOffsetAtRequestedInstant(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		zone, at, local string
		offset          int
	}{
		{"Europe/Warsaw", "2026-03-29T00:59:00Z", "2026-03-29T01:59:00+01:00", 3600},
		{"Europe/Warsaw", "2026-03-29T01:00:00Z", "2026-03-29T03:00:00+02:00", 7200},
		{"Europe/Warsaw", "2026-10-25T00:30:00Z", "2026-10-25T02:30:00+02:00", 7200},
		{"Europe/Warsaw", "2026-10-25T01:30:00Z", "2026-10-25T02:30:00+01:00", 3600},
		{"Asia/Kathmandu", "2026-01-01T00:00:00Z", "2026-01-01T05:45:00+05:45", 20700},
		{"UTC", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", 0},
	} {
		t.Run(test.zone+"/"+test.at, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, test.at)
			if err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewTimezone(test.zone, at)
			if err != nil {
				t.Fatal(err)
			}
			if preview.LocalTime != test.local || preview.OffsetSeconds != test.offset || preview.At != test.at {
				t.Fatalf("unexpected preview: %+v", preview)
			}
		})
	}
}

func TestTimezonePreviewRejectsUnknownZone(t *testing.T) {
	t.Parallel()
	if _, err := PreviewTimezone("Mars/Olympus", time.Now()); err == nil {
		t.Fatal("unknown timezone was accepted")
	}
}
