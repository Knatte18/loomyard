// anomaly.go implements the pure, in-memory, no-I/O, spawn-free classifier that names the structural anomalies a loom run can show: a crash-resume seen at entry, and the halt kinds seen in a run's final state.
// It performs no reads and derives no paths, so it is exhaustively table-tested untagged (anomaly_test.go), exactly the shape coherence.go already established in this package.
// The classifier feeds friction notes that the Tier 2 reflection reads; it files nothing itself.

package loomengine

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// AnomalyKind identifies which of the four triggers classified an anomaly.
type AnomalyKind string

// The four exported anomaly kinds and their exact wire spellings, each the kind token a friction note carries.
const (
	// AnomalyCrashResume is trigger 1: an entry observation showing a running state, a live
	// history, and no held run lock -- a driver process that died mid-run rather than exiting
	// cleanly.
	AnomalyCrashResume AnomalyKind = "crash-resume"
	// AnomalyEscalation is trigger 2: a blocked halt with no OnStuck target, requiring a human.
	AnomalyEscalation AnomalyKind = "escalation-to-human"
	// AnomalyBudgetExhausted is trigger 3: a blocked halt after exhausting a producer's bounce
	// budget.
	AnomalyBudgetExhausted AnomalyKind = "bounce-budget-exhausted"
	// AnomalyProducerFailure is trigger 4: a failed halt, an engine-level producer error.
	AnomalyProducerFailure AnomalyKind = "producer-hard-failure"
)

// EntryObservation carries the whole entry-time observation as data, told by the caller before
// any read, so trigger 1 can be evaluated on the cancelled-context path with no read of its own.
// Observed is false when the entry step was skipped or its read failed. RunLockHeld is the result
// of the non-blocking run-lock probe, taken before the read: a held lock means a live driver, never
// a crash.
type EntryObservation struct {
	Observed        bool
	RunLockHeld     bool
	State           shedengine.State
	CurrentProducer string
	HistoryLength   int
	Slug            string
	Parent          string
	// Vouched is true when the observation matches the handoff voucher a completed `lyx loom step` or `lyx loom start` recorded (same history length, same state).
	// Such a task is an operator handing it from the supervised step loop to a driver, or resuming it deliberately, byte-identical on disk to a mid-run driver death and excluded from trigger 1 for that reason.
	// The caller consumes the voucher and reports the match here; this package performs no read of its own.
	Vouched bool
	// VoucherFound is true when a handoff voucher existed at entry, whether or not it matched.
	// VoucherHistoryLength and VoucherState are the values it recorded, meaningful only when VoucherFound is true.
	// They are carried into the crash-resume note so the reader can compare the recorded reading with the observed one; DetectCrashResume reads Vouched only.
	VoucherFound         bool
	VoucherHistoryLength int
	VoucherState         shedengine.State
	// History is every history entry observed at entry.
	// It is carried into the crash-resume note so the reflection sees what the run had done, whole.
	History []shedengine.HistoryEntry
}

// DetectCrashResume reports trigger 1: whether entry describes a driver that died mid-run rather than exiting cleanly.
// It reports true when, and only when, entry.Observed is true, entry.RunLockHeld is false, entry.State is shedengine.StateRunning, entry.HistoryLength is greater than zero, and entry.Vouched is false.
// A held run lock means a live driver, never a crash.
// An empty history is a fresh seed at Preflight, byte-identical on disk to a crash at Preflight, so it is deliberately not reported -- the alternative would report a crash-resume for every ordinary first drive of every task.
// A vouched entry is that exclusion's sibling one row further along:
// a completed `lyx loom step` also leaves state running with a live history and no lock held, so without its voucher every operator handing a supervised task to a driver would read as a crash-resume.
// A paused, blocked, failed, or awaiting entry state is an ordinary human resume, not a crash.
func DetectCrashResume(entry EntryObservation) bool {
	return entry.Observed && !entry.RunLockHeld && entry.State == shedengine.StateRunning && entry.HistoryLength > 0 && !entry.Vouched
}

// ClassifyHalt evaluates the three halt-kind triggers (escalation, budget-exhausted,
// producer-hard-failure) against a run's final state and the reason it persisted.
// At most one fires, since the three triggers partition state:
// blocked with shedengine.ReasonBounceBudgetExhausted, blocked with anything else, or failed.
// shedengine persists StateBlocked from exactly two arms, so "blocked and not budget-exhausted" is exactly the
// escalation arm whatever reason its producer supplied.
// The one text the partition still matches is the budget prefix, matched with strings.HasPrefix because the persisted reason carries the exhausted row and a goto way forward after it:
// a producer reason starting with it would read as budget exhaustion, and no shipped producer returns one.
// Any other state reports no kind.
func ClassifyHalt(state shedengine.State, reason string) (AnomalyKind, bool) {
	switch {
	case state == shedengine.StateBlocked && strings.HasPrefix(reason, shedengine.ReasonBounceBudgetExhausted):
		return AnomalyBudgetExhausted, true
	case state == shedengine.StateBlocked:
		return AnomalyEscalation, true
	case state == shedengine.StateFailed:
		return AnomalyProducerFailure, true
	default:
		return "", false
	}
}
