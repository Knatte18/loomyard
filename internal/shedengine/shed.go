// shed.go declares the Shed engine struct, the run-level result types RunOutcome and Result,
// and the default bounce budget, used when neither ProducerDef nor Shed sets MaxBounces.

package shedengine

// Shed walks one flat, ordered producer list.
// It honors resume, crash-recovery, and pause uniformly, at producer granularity.
// It is a plain exported-field struct on purpose:
// there is no New constructor, which would leave a bare struct literal as a second, unvalidated door.
// Run validates every field below before it touches anything.
type Shed struct {
	Producers []ProducerDef
	// StatusPath is the durable status file; the caller supplies it, and it is never derived.
	StatusPath string
	// LockPath is the run lock, held non-blocking for the whole of one Run.
	LockPath string
	// StatusLockPath is the lock internal/state takes itself. It must name a file other than LockPath.
	// internal/state acquires it in blocking form,
	// so a shared path would hang on the first persist rather than fail.
	StatusLockPath string
	// MaxBounces is the default that a ProducerDef.MaxBounces of 0 inherits.
	// A 0 here falls back to defaultMaxBounces.
	// The budget is per producer and per episode, counted from the persisted history.
	//
	// The count is persisted rather than held in memory,
	// so that a crash-restart loop cannot be handed a fresh budget on each restart.
	//
	// A run-wide counter held in memory would reset on every Run call and so lose this bound.
	// The persisted per-producer count is kept instead.
	MaxBounces int
	// CommitStatus is the injected closure that persist calls after every successful status-file write.
	// Nil means commit nothing, so a product that wires no seam commits nothing.
	// It receives the transition's current_producer and state as plain strings, not State.
	// The owner can therefore build a per-transition commit message.
	// A caller in another package thereby never has to import this engine's enum type.
	//
	// It is called outside internal/state's write lock, never inside the mutate callback.
	// A synchronous network push inside that lock would block every reader of the status file.
	// Such a push would stall "lym loom status --watch" for its duration.
	CommitStatus func(producer, state string) error
	// Transient classifies a producer's returned error into the transient mark it records.
	// The caller supplies it, so this package never imports the packages whose failures it classifies.
	// Nil means no error is ever classified.
	// It runs only on a producer's returned error, never on a verdict.
	Transient func(error) TransientClass
	// RunID is the run-id that the verbs address this run by.
	// Every way-forward text naming a shed verb names it, so a slug-addressed run stays reachable.
	// The caller supplies it;
	// the empty string names the run-id-less form, which addresses self.
	RunID string
	// MissingStatusWayForward is the trailing "way forward:" clause for a missing status file.
	// The caller supplies it;
	// the empty string falls back to a generic clause.
	MissingStatusWayForward string
	// PauseWayForward is the command a busy refusal names to stop the run at a producer boundary.
	// The caller supplies it;
	// the empty string falls back to `lym shed pause <run-id> --after <current_producer>`.
	PauseWayForward string
	// RerouteCapWayForward is the trailing "way forward:" clause of a reroute-cap halt.
	// The caller supplies it;
	// the empty string falls back to the run verb that resumes the row.
	RerouteCapWayForward string
	// BudgetWayForward is the trailing "way forward:" clause of a bounce-budget-exhausted halt.
	// The caller supplies it;
	// the empty string falls back to the run verb that resumes the row and grants it one more budget.
	BudgetWayForward string
	// ResumedBy is the caller label of the process stepping this run, recorded when the step resumes a halted run.
	// The caller supplies it, for reports only; the empty string records none.
	ResumedBy string
	// LockedCheck is called once Run or Step holds the run lock, before the status file is read or written.
	// A non-nil error refuses the drive and is returned wrapped by ErrLockedCheckRefused.
	// The refusal calls no producer and leaves the status file untouched.
	// It can only stop a drive, never route one.
	// The error's own trailing "way forward:" clause stays last after the wrap.
	// The caller supplies it; nil means no check.
	LockedCheck func() error
	// Machine is the name of the machine driving this run, stamped on every history entry the shed appends.
	// The caller supplies it and this package never reads it; the empty string stamps nothing.
	Machine string
	// Retired maps each row name the recipe dropped to the row a run on it moves to.
	// A step whose status names a key of it, and no row of Producers, moves the run onto the target and pauses it.
	// A current row that is a row of Producers never consults it.
	// The caller supplies it; nil means no row is retired.
	Retired map[string]string
	// RetiredWayForward is the way-forward command that a move off a retired row names in its pause reason and error.
	// The caller supplies it;
	// the empty string falls back to the run verb that resumes the run.
	RetiredWayForward string
	// RetiredMove is the recipe wiring's side effects of a move off a retired row.
	// It is called with the retired row, its target, and whether the status was not running.
	// A non-nil error refuses the move and persists nothing, so the next step calls it again;
	// it must be idempotent. The caller supplies it; nil does nothing.
	RetiredMove func(from, to string, resumed bool) error
}

// RunOutcome is the whole run's terminal classification.
type RunOutcome string

// The legal RunOutcome values.
// The first four have the same string values as State's four clean-exit values.
// Mapping between the two is therefore the identity, never a lookup table.
// RunYielded has no State twin: the run ended on a step that yielded, and it stays at the same row, still running.
const (
	RunDone     RunOutcome = "done"
	RunBlocked  RunOutcome = "blocked"
	RunPaused   RunOutcome = "paused"
	RunAwaiting RunOutcome = "awaiting"
	RunYielded  RunOutcome = "yielded"
)

// Result is what Run reports on a clean exit.
// A caller must branch on Outcome before reading Reason, which is populated only alongside
// RunBlocked and RunAwaiting.
// Result is meaningless unless the returned error is nil.
// Every hard-error path returns an unpopulated Result alongside its error.
// The zero RunOutcome is the empty string, which is not one of the legal values.
type Result struct {
	Outcome RunOutcome
	// HaltedProducer is the producer current_producer named when Run returned.
	HaltedProducer string
	// Reason is set only alongside RunBlocked and RunAwaiting.
	Reason string
	// History is the full persisted history as it stands when Run returns.
	// It includes the entries that earlier invocations appended, not only this invocation's.
	History []HistoryEntry
}

// defaultMaxBounces is the budget used when both ProducerDef.MaxBounces and Shed.MaxBounces are 0.
const defaultMaxBounces = 10
