// shed.go defines the Shed engine struct and the run-level result vocabulary: RunOutcome, Result,
// and the two-level default the bounce budget falls back to when a ProducerDef and then Shed
// itself leave MaxBounces unset.

package shedengine

// Shed walks one flat, ordered producer list, honoring resume, crash-recovery, and pause
// uniformly at producer granularity. It is a plain exported-field struct on purpose: there is no
// New constructor, which would leave a bare struct literal as a second, unvalidated door.
// Run validates every field below before it touches anything.
type Shed struct {
	Producers []ProducerDef
	// StatusPath is the status file; it is told and never derived.
	StatusPath string
	// LockPath is the run lock, held non-blocking for the whole of one Run.
	LockPath string
	// StatusLockPath is the lock internal/state itself takes. It must name a different file from
	// LockPath, because internal/state acquires it with the blocking form, so one shared path
	// would hang on the first persist rather than failing.
	StatusLockPath string
	// MaxBounces is the default a ProducerDef.MaxBounces of 0 inherits; 0 here in turn falls back
	// to the internal default of ten. The budget it seeds is per-producer and episode-scoped,
	// counted from the persisted history[] rather than held in memory.
	//
	// This is an inversion from an earlier design, worth naming explicitly: the budget used to
	// be run-wide, held in an in-memory counter that was deliberately reset on every new Run
	// call. That reset made a crash-restart loop unbounded -- each restart handed the loop a
	// fresh budget -- which is why this task overturned it in favor of the persisted,
	// per-producer count above. Read the missing reset as intentional, not an omission.
	MaxBounces int
}

// RunOutcome is the whole run's terminal classification.
type RunOutcome string

// The three legal RunOutcome values. Their string values are deliberately identical to State's
// three clean-exit values, so mapping between the two is identity, never a lookup table.
const (
	RunDone    RunOutcome = "done"
	RunBlocked RunOutcome = "blocked"
	RunPaused  RunOutcome = "paused"
)

// Result is what Run reports on a clean exit.
// A caller must branch on Outcome before reading Reason, which is populated only alongside
// RunBlocked.
// Result is meaningless unless the returned error is nil: RunOutcome's zero value is the empty
// string, not one of the three legal constants above, and every hard-error path returns an
// unpopulated Result alongside its error.
type Result struct {
	Outcome RunOutcome
	// HaltedProducer is the producer current_producer named when Run returned.
	HaltedProducer string
	// Reason is set only alongside RunBlocked.
	Reason string
	// History is the full persisted history as it stands when Run returns, not only the entries
	// this invocation appended.
	History []HistoryEntry
}

// defaultMaxBounces is the default an unset budget resolves to at the second level: when both a
// ProducerDef.MaxBounces and Shed.MaxBounces are 0.
const defaultMaxBounces = 10
