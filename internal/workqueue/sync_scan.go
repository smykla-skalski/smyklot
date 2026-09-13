package workqueue

// SyncScanDetails belongs to one occurrence, independently of the current live
// sync plan. Plan creation writes this link in the same transaction as the plan.
type SyncScanDetails struct {
	ResultPlanID string `json:"result_plan_id,omitempty"`
}
