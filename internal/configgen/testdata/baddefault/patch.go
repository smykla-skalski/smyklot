package fixture

type Patch struct {
	// QuietSuccess carries a default that is not a boolean, which would render
	// as a bare identifier.
	QuietSuccess *bool `json:"quiet_success" default:"maybe"`
}
