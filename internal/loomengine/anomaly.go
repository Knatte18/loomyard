// anomaly.go classifies the structural anomalies a tapestry run can show.
// Those are a crash-resume seen at entry and the halt kinds seen in a run's final state.
// It performs no reads, spawns nothing, and derives no paths, so anomaly_test.go table-tests it without a build tag.
// The classifier feeds the friction notes that the Tier 2 reflection reads; it files no note itself.

package loomengine

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// AnomalyKind is the type of a structural anomaly; a friction note carries its value as the kind token.
type AnomalyKind string

// The anomaly kinds, each with the exact wire spelling a friction note carries as its kind token.
const (
	// AnomalyCrashResume marks an entry observation with a running state, a live history, and no held run lock.
	// Such an observation is a driver process that died mid-run, not one that exited cleanly.
	// A held run lock rules it out, since it means a live driver.
	AnomalyCrashResume AnomalyKind = "crash-resume"
	// AnomalyEscalation marks a blocked halt with no OnStuck target, which requires a human.
	AnomalyEscalation AnomalyKind = "escalation-to-human"
	// AnomalyBudgetExhausted marks a blocked halt after a producer exhausted its bounce budget.
	// Its persisted reason starts with shedengine.ReasonBounceBudgetExhausted.
	AnomalyBudgetExhausted AnomalyKind = "bounce-budget-exhausted"
	// AnomalyProducerFailure marks a failed halt, an engine-level producer error.
	AnomalyProducerFailure AnomalyKind = "producer-hard-failure"
	// AnomalyInterrupted marks a non-blocked halt of a step that ended without an envelope, from outside its producer.
	// That covers a killed or crashed step, a stalled one the watchdog killed, or a loop that died under it.
	AnomalyInterrupted AnomalyKind = "interrupted-stop"
)

// EntryObservation is the entry-time observation of a task, gathered by the caller before any read.
// It is data only, so the crash-resume check can run on the cancelled-context path with no read of its own.
// Observed is false when the entry step was skipped or its read failed.
// RunLockHeld is the result of a non-blocking run-lock probe taken before the read.
// A held lock means a live driver, never a crash.
type EntryObservation struct {
	Observed        bool
	RunLockHeld     bool
	State           shedengine.State
	CurrentProducer string
	HistoryLength   int
	Slug            string
	Parent          string
	// Vouched is true when the observation matches the handoff voucher that `lym loom step` or `lym loom start` left.
	// Such a task is an operator's handoff to a driver, and it is byte-identical on disk to a mid-run driver death.
	// Crash-resume detection excludes it, and the caller consumes the voucher and reports the match here.
	Vouched bool
	// VoucherFound is true when a handoff voucher existed at entry, whether or not it matched.
	// VoucherHistoryLength and VoucherState are the values it recorded, meaningful only when VoucherFound is true.
	// They are carried into the crash-resume note, so a reader can compare the recorded reading with the observed one.
	// The crash-resume check reads Vouched, not these two fields.
	VoucherFound         bool
	VoucherHistoryLength int
	VoucherState         shedengine.State
	// History is every history entry observed at entry.
	// It is carried into the crash-resume note, so the reflection sees the whole of what the run had done.
	History []shedengine.HistoryEntry
}

// DetectCrashResume reports whether entry describes a driver that died mid-run rather than exiting cleanly.
// It reports true when, and only when, entry.Observed is true and entry.RunLockHeld is false.
// It also requires entry.State to be shedengine.StateRunning, entry.HistoryLength above zero, and entry.Vouched false.
// A held run lock means a live driver, never a crash.
// An empty history is a fresh seed at the run's start, indistinguishable from a crash in the first row.
// It is not reported, since that would flag every ordinary first drive of every task.
// A vouched entry is excluded for a second reason.
// A completed `lym loom step` also leaves the state running with a live history and no lock held.
// Without its voucher, every operator handing a supervised task to a driver would read as a crash-resume.
// A paused, blocked, failed, or awaiting entry state is an ordinary human resume, not a crash.
func DetectCrashResume(entry EntryObservation) bool {
	return entry.Observed && !entry.RunLockHeld && entry.State == shedengine.StateRunning && entry.HistoryLength > 0 && !entry.Vouched
}

// ClassifyHalt names the kind of a run's final halt, and reports whether one applies.
// It reads the final state, the reason the state was persisted with, and whether the stop was interrupted.
// At most one kind fires, since the cases partition the state.
// A blocked state with shedengine.ReasonBounceBudgetExhausted is budget-exhausted.
// Any other blocked state is an escalation, and the interrupted flag changes no blocked kind.
// Otherwise an interrupted stop is AnomalyInterrupted, and a failed state is AnomalyProducerFailure.
// Any other state reports no kind.
// An interrupted step can leave any state its failed write could reach.
// For example, a paused run died before its resume write, or a running run was found busy by the failed write.
// Such a step still classifies as an interrupted stop.
// shedengine persists StateBlocked from exactly two arms.
// Blocked and not budget-exhausted is therefore always an escalation, whatever reason its producer supplied.
// The budget prefix is the one text still matched, with strings.HasPrefix.
// The persisted reason carries the exhausted row and the way forward after it.
// A producer reason starting with the prefix would read as budget exhaustion, and no shipped producer returns one.
func ClassifyHalt(state shedengine.State, reason string, interrupted bool) (AnomalyKind, bool) {
	switch {
	case state == shedengine.StateBlocked && strings.HasPrefix(reason, shedengine.ReasonBounceBudgetExhausted):
		return AnomalyBudgetExhausted, true
	case state == shedengine.StateBlocked:
		return AnomalyEscalation, true
	case interrupted:
		return AnomalyInterrupted, true
	case state == shedengine.StateFailed:
		return AnomalyProducerFailure, true
	default:
		return "", false
	}
}
