package workqueue

import (
	"fmt"
	"sort"
	"time"
)

const localMinuteLayout = "2006-01-02T15:04"

// LocalTimeResolution keeps missing and repeated wall times explicit. Callers must
// select an option before dispatch rather than letting time.Date choose for them.
type LocalTimeResolution struct {
	Timezone  string            `json:"timezone"`
	LocalTime string            `json:"local_time"`
	Options   []TimezonePreview `json:"options"`
}

// ResolveLocalTime returns every instant matching a minute in the scheduler's
// timezone database. A skipped clock time has no options; a repeated one has two.
func ResolveLocalTime(name, value string) (LocalTimeResolution, error) {
	wall, err := time.Parse(localMinuteLayout, value)
	if err != nil || wall.Format(localMinuteLayout) != value {
		return LocalTimeResolution{}, fmt.Errorf("invalid local date and time %q", value)
	}
	if name == "" || name == "Local" {
		return LocalTimeResolution{}, fmt.Errorf("an explicit timezone is required")
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return LocalTimeResolution{}, fmt.Errorf("load timezone: %w", err)
	}
	result := LocalTimeResolution{Timezone: name, LocalTime: value, Options: []TimezonePreview{}}
	// IANA offsets are less than 24 hours. Traverse every zone interval within a
	// wider envelope, including date-line jumps and historical second offsets.
	// ZoneBounds avoids sampling that could miss a short-lived offset interval.
	end := wall.Add(48 * time.Hour)
	offsets := map[int]bool{}
	for cursor := wall.Add(-48 * time.Hour); cursor.Before(end); {
		local := cursor.In(location)
		_, offset := local.Zone()
		offsets[offset] = true
		_, next := local.ZoneBounds()
		if next.IsZero() || !next.After(cursor) {
			break
		}
		cursor = next
	}
	for offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(location)
		if local.Format(localMinuteLayout) != value || local.Second() != 0 {
			continue
		}
		preview, err := PreviewTimezone(name, candidate)
		if err != nil {
			return LocalTimeResolution{}, err
		}
		result.Options = append(result.Options, preview)
	}
	sort.Slice(result.Options, func(i, j int) bool { return result.Options[i].At < result.Options[j].At })
	return result, nil
}
