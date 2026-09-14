package workqueue

import "time"

// TimezonePreview describes an instant using the scheduler's timezone database.
// Offsets belong to the instant, not to the zone for all dates.
type TimezonePreview struct {
	Timezone      string `json:"timezone"`
	At            string `json:"at"`
	LocalTime     string `json:"local_time"`
	Abbreviation  string `json:"abbreviation"`
	OffsetSeconds int    `json:"offset_seconds"`
}

// PreviewTimezone uses the same location loader as profile validation and execution.
func PreviewTimezone(name string, at time.Time) (TimezonePreview, error) {
	location, err := time.LoadLocation(name)
	if err != nil {
		return TimezonePreview{}, err
	}
	local := at.In(location)
	abbreviation, offset := local.Zone()
	return TimezonePreview{
		Timezone: name, At: at.UTC().Format(time.RFC3339Nano),
		LocalTime: local.Format(time.RFC3339Nano), Abbreviation: abbreviation,
		OffsetSeconds: offset,
	}, nil
}
