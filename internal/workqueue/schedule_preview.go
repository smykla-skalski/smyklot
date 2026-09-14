package workqueue

import (
	"fmt"
	"time"
)

// DatePreview describes configured hours on a profile-local calendar date.
// It uses the same exception precedence and clock-change rules as execution.
type DatePreview struct {
	Date     string          `json:"date"`
	Timezone string          `json:"timezone"`
	Windows  []WindowPreview `json:"windows"`
}

// WindowPreview retains requested boundaries even when a clock change removes
// their entire interval. Unavailable intervals have no opening or closing instant.
type WindowPreview struct {
	StartMinute int    `json:"start_minute"`
	EndMinute   int    `json:"end_minute"`
	Available   bool   `json:"available"`
	OpensAt     string `json:"opens_at,omitempty"`
	ClosesAt    string `json:"closes_at,omitempty"`
}

// PreviewDate resolves a date without persisting the profile or scheduling work.
func PreviewDate(profile Profile, dateText string) (DatePreview, error) {
	if err := ValidateProfile(profile); err != nil {
		return DatePreview{}, fmt.Errorf("validate preview profile: %w", err)
	}
	date, err := time.Parse(time.DateOnly, dateText)
	if err != nil {
		return DatePreview{}, fmt.Errorf("parse preview date: %w", err)
	}
	location, _ := time.LoadLocation(profile.Timezone)
	preview := DatePreview{Date: dateText, Timezone: profile.Timezone, Windows: []WindowPreview{}}
	for _, window := range windowsForDate(profile, date) {
		start, end, available := windowInstantRange(date, window, location)
		item := WindowPreview{StartMinute: window.Start, EndMinute: window.End, Available: available}
		if available {
			item.OpensAt = start.In(location).Format(time.RFC3339)
			item.ClosesAt = end.In(location).Format(time.RFC3339)
		}
		preview.Windows = append(preview.Windows, item)
	}
	return preview, nil
}
