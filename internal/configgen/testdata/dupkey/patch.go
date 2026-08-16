package fixture

type Patch struct {
	// QuietSuccess is addressed by quiet_success.
	QuietSuccess *bool `json:"quiet_success"`

	// AlsoQuiet is addressed by the same key, so one of them would never
	// resolve.
	AlsoQuiet *bool `json:"quiet_success"`
}
