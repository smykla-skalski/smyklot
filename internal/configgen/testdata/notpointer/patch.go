package fixture

type Patch struct {
	// QuietSuccess is not sparse, so an omitted setting and an explicit false
	// would be the same value.
	QuietSuccess bool `json:"quiet_success"`
}
