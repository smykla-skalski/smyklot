package apply

import "github.com/smykla-skalski/smyklot/internal/orgsync"

// kindObservation carries evidence and its destination from one GitHub operation.
// An empty result means no fresh observation was established by this attempt.
type kindObservation struct {
	state       orgsync.Observation
	proposalURL string
}
