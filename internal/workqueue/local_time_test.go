package workqueue

import (
	"reflect"
	"testing"
)

func TestResolveLocalTime(t *testing.T) {
	cases := []struct {
		name, zone, wall string
		instants         []string
	}{
		{"UTC", "UTC", "2026-10-25T02:30", []string{"2026-10-25T02:30:00Z"}},
		{"Tokyo", "Asia/Tokyo", "2026-10-25T02:30", []string{"2026-10-24T17:30:00Z"}},
		{"Warsaw repeated hour", "Europe/Warsaw", "2026-10-25T02:30", []string{"2026-10-25T00:30:00Z", "2026-10-25T01:30:00Z"}},
		{"Warsaw skipped hour", "Europe/Warsaw", "2026-03-29T02:30", []string{}},
		{"New York repeated hour", "America/New_York", "2026-11-01T01:30", []string{"2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"}},
		{"half-hour repeat", "Australia/Lord_Howe", "2026-04-05T01:45", []string{"2026-04-04T14:45:00Z", "2026-04-04T15:15:00Z"}},
		{"half-hour gap", "Australia/Lord_Howe", "2026-10-04T02:15", []string{}},
		{"skipped date", "Pacific/Apia", "2011-12-30T12:00", []string{}},
		{"historical seconds", "Europe/Paris", "1900-01-01T12:00", []string{"1900-01-01T11:50:39Z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ResolveLocalTime(tc.zone, tc.wall)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, option := range result.Options {
				got = append(got, option.At)
			}
			if !reflect.DeepEqual(got, tc.instants) {
				t.Fatalf("got %v, want %v", got, tc.instants)
			}
			if result.Timezone != tc.zone || result.LocalTime != tc.wall {
				t.Fatalf("lost input identity: %+v", result)
			}
		})
	}
}

func TestResolveLocalTimeRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"", "2026-02-30T12:00", "2026-01-01T24:00", "2026-01-01T12:00Z", "2026-01-01T12:00:30", "2026-1-1T12:00"} {
		if _, err := ResolveLocalTime("UTC", value); err == nil {
			t.Errorf("accepted invalid wall time %q", value)
		}
	}
	for _, zone := range []string{"", "Local", "Mars/Olympus"} {
		if _, err := ResolveLocalTime(zone, "2026-01-01T12:00"); err == nil {
			t.Errorf("accepted invalid zone %q", zone)
		}
	}
}
