package workqueue

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrNoEligibleWindow = errors.New("schedule profile has no eligible window")

func AlwaysOpenProfile(now time.Time) Profile {
	windows := make([]Window, 0, 7)
	for day := time.Sunday; day <= time.Saturday; day++ {
		windows = append(windows, Window{Weekday: day, Start: 0, End: 24 * 60})
	}

	return Profile{
		ID: AlwaysOpenProfileID, Name: "Always Open", Timezone: "UTC",
		System: true, Revision: 1, Windows: windows,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
}

func ValidateProfile(profile Profile) error {
	if strings.TrimSpace(profile.ID) == "" || strings.TrimSpace(profile.Name) == "" {
		return errors.New("schedule profile id and name are required")
	}
	if _, err := time.LoadLocation(profile.Timezone); err != nil {
		return fmt.Errorf("load schedule timezone: %w", err)
	}
	if len(profile.Windows) == 0 && len(profile.Exceptions) == 0 {
		return errors.New("schedule profile needs an open window")
	}
	if err := validateWindows(profile.Windows); err != nil {
		return err
	}

	return validateExceptions(profile.Exceptions)
}

func validateWindows(windows []Window) error {
	byDay := map[time.Weekday][]Window{}
	for _, window := range windows {
		if window.Weekday < time.Sunday || window.Weekday > time.Saturday {
			return errors.New("schedule window weekday is invalid")
		}
		if err := validateMinuteRange(window.Start, window.End); err != nil {
			return err
		}
		byDay[window.Weekday] = append(byDay[window.Weekday], window)
	}
	for _, day := range byDay {
		if rangesOverlap(day) {
			return errors.New("schedule windows overlap")
		}
	}

	return nil
}

func validateExceptions(exceptions []Exception) error {
	byDate := map[string][]Exception{}
	for _, exception := range exceptions {
		if _, err := time.Parse(time.DateOnly, exception.Date); err != nil {
			return errors.New("schedule exception date is invalid")
		}
		if !exception.Closed {
			if err := validateMinuteRange(exception.Start, exception.End); err != nil {
				return err
			}
		}
		byDate[exception.Date] = append(byDate[exception.Date], exception)
	}
	for _, entries := range byDate {
		if exceptionSetInvalid(entries) {
			return errors.New("schedule exceptions overlap or mix open and closed rules")
		}
	}

	return nil
}

func validateMinuteRange(start, end int) error {
	if start < 0 || start >= 24*60 || end <= 0 || end > 24*60 || start >= end {
		return errors.New("schedule window minutes are invalid")
	}

	return nil
}

func exceptionSetInvalid(entries []Exception) bool {
	closed := false
	windows := make([]Window, 0, len(entries))
	for _, entry := range entries {
		closed = closed || entry.Closed
		if !entry.Closed {
			windows = append(windows, Window{Start: entry.Start, End: entry.End})
		}
	}

	return closed && len(entries) > 1 || rangesOverlap(windows)
}

func rangesOverlap(windows []Window) bool {
	sort.Slice(windows, func(i, j int) bool { return windows[i].Start < windows[j].Start })
	for index := 1; index < len(windows); index++ {
		if windows[index].Start < windows[index-1].End {
			return true
		}
	}

	return false
}

func NextEligible(profile Profile, notBefore time.Time) (time.Time, error) {
	if err := ValidateProfile(profile); err != nil {
		return time.Time{}, err
	}
	location, _ := time.LoadLocation(profile.Timezone)
	if profileOpenAt(profile, notBefore, location) {
		return notBefore, nil
	}

	localStart := notBefore.In(location)
	for day := 0; day <= searchDays(profile, localStart); day++ {
		date := localDate(localStart.AddDate(0, 0, day))
		for _, window := range windowsForDate(profile, date) {
			start, _, ok := windowInstantRange(date, window, location)
			if ok && !start.Before(notBefore) {
				return start.UTC(), nil
			}
		}
	}

	return time.Time{}, ErrNoEligibleWindow
}

func profileOpenAt(profile Profile, instant time.Time, location *time.Location) bool {
	date := localDate(instant.In(location))
	for _, window := range windowsForDate(profile, date) {
		start, end, ok := windowInstantRange(date, window, location)
		if ok && !instant.Before(start) && instant.Before(end) {
			return true
		}
	}

	return false
}

func windowsForDate(profile Profile, date time.Time) []Window {
	dateText := date.Format(time.DateOnly)
	var exceptions []Window
	found := false
	for _, exception := range profile.Exceptions {
		if exception.Date != dateText {
			continue
		}
		found = true
		if !exception.Closed {
			exceptions = append(exceptions, Window{Start: exception.Start, End: exception.End})
		}
	}
	if found {
		return sortedWindows(exceptions)
	}
	var windows []Window
	for _, window := range profile.Windows {
		if window.Weekday == date.Weekday() {
			windows = append(windows, window)
		}
	}

	return sortedWindows(windows)
}

func sortedWindows(windows []Window) []Window {
	result := append([]Window(nil), windows...)
	sort.Slice(result, func(i, j int) bool { return result[i].Start < result[j].Start })

	return result
}

func windowInstantRange(
	date time.Time,
	window Window,
	location *time.Location,
) (time.Time, time.Time, bool) {
	start, startOK := wallBoundaryInstant(date, window.Start, location, false)
	end, endOK := wallBoundaryInstant(date, window.End, location, true)
	if !startOK || !endOK || !end.After(start) {
		return time.Time{}, time.Time{}, false
	}

	return start, end, true
}

// wallBoundaryInstant maps a profile-local wall-clock boundary to an instant.
// A missing boundary moves to the first valid wall-clock instant after the DST
// gap. For a repeated boundary, starts use the first occurrence and ends use
// the last occurrence, making a repeated-hour window one continuous opening.
func wallBoundaryInstant(
	date time.Time,
	minute int,
	location *time.Location,
	latest bool,
) (time.Time, bool) {
	targetDate := date
	if minute == 24*60 {
		targetDate = localDate(date.AddDate(0, 0, 1))
		minute = 0
	}
	approximate := time.Date(
		targetDate.Year(), targetDate.Month(), targetDate.Day(),
		minute/60, minute%60, 0, 0, location,
	)
	searchStart, searchEnd := approximate.Add(-4*time.Hour), approximate.Add(4*time.Hour)
	var exact, after *time.Time
	for candidate := searchStart; !candidate.After(searchEnd); candidate = candidate.Add(time.Minute) {
		local := candidate.In(location)
		if !sameDate(local, targetDate) {
			continue
		}
		wallMinute := local.Hour()*60 + local.Minute()
		if wallMinute == minute {
			copy := candidate
			if exact == nil || latest && copy.After(*exact) || !latest && copy.Before(*exact) {
				exact = &copy
			}
			continue
		}
		if wallMinute > minute && (after == nil || candidate.Before(*after)) {
			copy := candidate
			after = &copy
		}
	}
	if exact != nil {
		return *exact, true
	}
	if after != nil {
		return *after, true
	}

	return time.Time{}, false
}

func searchDays(profile Profile, start time.Time) int {
	days := 14
	for _, exception := range profile.Exceptions {
		date, err := time.ParseInLocation(time.DateOnly, exception.Date, start.Location())
		if err == nil && date.After(start) {
			days = max(days, int(date.Sub(localDate(start)).Hours()/24)+14)
		}
	}

	return days
}

func localDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func sameDate(left, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}
