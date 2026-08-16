package fixture

type Patch struct {
	// Runner defaults to a value its own enum does not offer.
	Runner *string `json:"runner" enum:"service,action" default:"workflow"`
}
