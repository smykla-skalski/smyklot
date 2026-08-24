package workqueue

import (
	"testing"
	"time"
)

func TestNextEligibleAlwaysOpen(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 24, 8, 42, 17, 0, time.UTC)
	profile := AlwaysOpenProfile(now)

	eligible, err := NextEligible(profile, now)
	if err != nil {
		t.Fatalf("resolve always-open profile: %v", err)
	}
	if !eligible.Equal(now) {
		t.Fatalf("eligible at %v, want %v", eligible, now)
	}
}

func TestNextEligibleUsesProfileTimezone(t *testing.T) {
	t.Parallel()

	profile := Profile{
		ID:       "warsaw-office",
		Name:     "Warsaw office hours",
		Timezone: "Europe/Warsaw",
		Windows: []Window{{
			Weekday: time.Monday,
			Start:   9 * 60,
			End:     17 * 60,
		}},
	}
	notBefore := time.Date(2026, time.August, 24, 6, 30, 0, 0, time.UTC)

	eligible, err := NextEligible(profile, notBefore)
	if err != nil {
		t.Fatalf("resolve Warsaw window: %v", err)
	}
	want := time.Date(2026, time.August, 24, 7, 0, 0, 0, time.UTC)
	if !eligible.Equal(want) {
		t.Fatalf("eligible at %v, want %v", eligible, want)
	}
}

func TestNextEligibleHonorsClosedAndOpenExceptions(t *testing.T) {
	t.Parallel()

	profile := Profile{
		ID:       "office",
		Name:     "Office hours",
		Timezone: "UTC",
		Windows: []Window{
			{Weekday: time.Monday, Start: 9 * 60, End: 17 * 60},
			{Weekday: time.Tuesday, Start: 9 * 60, End: 17 * 60},
		},
		Exceptions: []Exception{
			{Date: "2026-08-24", Closed: true},
			{Date: "2026-08-25", Start: 7 * 60, End: 8 * 60},
		},
	}
	notBefore := time.Date(2026, time.August, 24, 8, 0, 0, 0, time.UTC)

	eligible, err := NextEligible(profile, notBefore)
	if err != nil {
		t.Fatalf("resolve exception: %v", err)
	}
	want := time.Date(2026, time.August, 25, 7, 0, 0, 0, time.UTC)
	if !eligible.Equal(want) {
		t.Fatalf("eligible at %v, want %v", eligible, want)
	}
}

func TestNextEligibleHandlesSpringForwardGap(t *testing.T) {
	t.Parallel()

	profile := Profile{
		ID:       "new-york-gap",
		Name:     "New York gap",
		Timezone: "America/New_York",
		Windows: []Window{{
			Weekday: time.Sunday,
			Start:   2*60 + 30,
			End:     4 * 60,
		}},
	}
	notBefore := time.Date(2026, time.March, 8, 6, 0, 0, 0, time.UTC)

	eligible, err := NextEligible(profile, notBefore)
	if err != nil {
		t.Fatalf("resolve spring-forward window: %v", err)
	}
	// 02:30 does not exist. The first valid local instant in the window is 03:00 EDT.
	want := time.Date(2026, time.March, 8, 7, 0, 0, 0, time.UTC)
	if !eligible.Equal(want) {
		t.Fatalf("eligible at %v, want %v", eligible, want)
	}
}

func TestNextEligibleMergesRepeatedHourIntoOneOpening(t *testing.T) {
	t.Parallel()

	profile := Profile{
		ID:       "new-york-repeat",
		Name:     "New York repeated hour",
		Timezone: "America/New_York",
		Windows: []Window{{
			Weekday: time.Sunday,
			Start:   30,
			End:     90,
		}},
	}
	// 01:45 EDT is later than the first 01:30 closing, but the clock will
	// repeat 01:00. The profile remains continuously open until 01:30 EST.
	notBefore := time.Date(2026, time.November, 1, 5, 45, 0, 0, time.UTC)

	eligible, err := NextEligible(profile, notBefore)
	if err != nil {
		t.Fatalf("resolve repeated-hour window: %v", err)
	}
	if !eligible.Equal(notBefore) {
		t.Fatalf("eligible at %v, want continuous opening at %v", eligible, notBefore)
	}
}

func TestValidateProfileRejectsOverlappingWindows(t *testing.T) {
	t.Parallel()

	err := ValidateProfile(Profile{
		ID:       "overlap",
		Name:     "Overlap",
		Timezone: "UTC",
		Windows: []Window{
			{Weekday: time.Monday, Start: 9 * 60, End: 12 * 60},
			{Weekday: time.Monday, Start: 11 * 60, End: 13 * 60},
		},
	})
	if err == nil {
		t.Fatal("overlapping windows were accepted")
	}
}
