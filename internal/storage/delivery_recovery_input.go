package storage

// DeliveryRecoveryInput is private server input for workload eligibility checks.
// It is an observation, not a lease or authorization. The recovery transaction
// must still recheck the run, revision, payload and actor's current access.
type DeliveryRecoveryInput struct {
	RunID              int64
	Revision           int64
	SourceOrder        int64
	ClaimKey           string
	TargetID           string
	RepositoryID       *string
	RepositoryFullName string
	Event              string
	Payload            []byte `json:"-"`
}
