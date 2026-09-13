package orgsync

import "time"

// CheckDetails is retained on one queue occurrence. Evidence rows are stored
// separately so queue lists remain bounded independently of repository count.
type CheckDetails struct {
	ResultPlanID string        `json:"result_plan_id,omitempty"`
	Outcome      *CheckOutcome `json:"outcome,omitempty"`
}

type CheckOutcome struct {
	// BlockingPlanID names earlier work, never a result produced by this check.
	BlockingPlanID     string              `json:"blocking_plan_id,omitempty"`
	CompletedAt        time.Time           `json:"completed_at"`
	Disposition        string              `json:"disposition"`
	Summary            string              `json:"summary"`
	Counts             map[Observation]int `json:"counts"`
	Cached             int                 `json:"cached"`
	MissingPermissions []Kind              `json:"missing_permissions"`
}

// CheckObservation preserves the repository name, input identity and observation
// time as they were at this check, including the older time of reused evidence.
type CheckObservation struct {
	RepositoryID string      `json:"repository_id"`
	Repository   string      `json:"repository"`
	Kind         Kind        `json:"kind"`
	Outcome      Observation `json:"outcome"`
	ObservedAt   time.Time   `json:"observed_at"`
	InputDigest  string      `json:"input_digest"`
	Reason       string      `json:"reason,omitempty"`
	ProposalURL  string      `json:"proposal_url,omitempty"`
	Cached       bool        `json:"cached"`
}

type CheckResult struct {
	Outcome      CheckOutcome
	Observations []CheckObservation
}

type CheckResultCreate struct {
	Check    CheckReference
	TargetID string
	Result   CheckResult
	Now      time.Time
}

type CheckObservationPage struct {
	Items []CheckObservation
	Total int
	Next  *int
}
