package storage

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// DeliveryOperation connects a retained historical run to the current execution.
// It contains no replay payload. Availability does not grant recovery permission.
// Revision is zero and Current is nil when the operation cursor is not retained.
type DeliveryOperation struct {
	TargetID    string
	Revision    int64
	SourceOrder int64
	Current     *DeliveryRun
}

// DeliveryRun describes current execution independently of a historical failure.
// Queue is absent when queue retention removed its record. Status remains usable.
type DeliveryRun struct {
	ID               int64
	Status           DeliveryStatus
	Event            string
	PayloadAvailable bool
	Queue            *DeliveryRunQueue
}

// DeliveryRunQueue distinguishes scheduled eligibility from active execution.
type DeliveryRunQueue struct {
	ID         string
	State      workqueue.State
	EligibleAt time.Time
}
