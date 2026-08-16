package fixture

type Patch struct {
	// AllowedCommands is a list, which cannot carry a scalar default.
	AllowedCommands *[]string `json:"allowed_commands" default:"approve"`
}
