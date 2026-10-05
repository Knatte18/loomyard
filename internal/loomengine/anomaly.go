// anomaly.go implements the pure, in-memory, no-I/O, spawn-free detector that finds Tier-1
// structural anomalies over told inputs: an entry-time observation, the run's final
// shedengine.Status and the loom product it carried. It performs no reads and derives no paths, so
// it is exhaustively table-tested in Tier 1 (anomaly_test.go), exactly the shape coherence.go
// already established in this package.

package loomengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// AnomalyKind identifies which of the four Tier-1 triggers detected an anomaly.
type AnomalyKind string

// The four exported anomaly kinds and their exact wire spellings, each the <kind> token embedded
// verbatim in that anomaly's Title.
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
	// CleanStepHandoff is true when the observation matches the clean-handoff marker the last
	// completed `lyx loom step` recorded (same history length, same state) -- a task an operator is
	// handing from the supervised step loop to a driver, byte-identical on disk to a mid-run driver
	// death and excluded from trigger 1 for that reason. The caller consumes the marker and reports
	// the match here; this package performs no read of its own.
	CleanStepHandoff bool
}

// Anomaly carries everything the title and body need with no further lookup.
type Anomaly struct {
	Kind            AnomalyKind
	Title           string
	Slug            string
	Parent          string
	State           shedengine.State
	CurrentProducer string
	Error           string
	History         []shedengine.HistoryEntry
}

// DetectCrashResume reports trigger 1 alone: whether entry describes a driver that died mid-run
// rather than exiting cleanly. It is its own exported function, not merely an internal branch of
// DetectAnomalies, because the caller must be able to reach it on a cancelled context with no
// final status in hand.
// It takes no slug parameter: entry already carries Slug and Parent, and reading them off it is
// exactly what makes the cancelled-context call site work, since that branch has neither a final
// status nor a decoded product in hand.
// It reports an anomaly when, and only when, entry.Observed is true, entry.RunLockHeld is false,
// entry.State is shedengine.StateRunning, entry.HistoryLength is greater than zero, and
// entry.CleanStepHandoff is false. A held run lock means a live driver, never a crash. An empty
// history is a fresh seed at Preflight, byte-identical on disk to a crash at Preflight, so it is
// deliberately not reported -- the alternative would file a crash-resume for every ordinary first
// drive of every task. A clean step handoff is that exclusion's sibling one row further along: a
// completed `lyx loom step` also leaves state running with a live history and no lock held, so
// without its marker every operator handing a supervised task to a driver would file a spurious
// crash-resume (crucible round 2, R2-F1). A paused, blocked, failed, or awaiting entry state is an
// ordinary human resume, not a crash.
func DetectCrashResume(entry EntryObservation) (Anomaly, bool) {
	if !entry.Observed || entry.RunLockHeld || entry.State != shedengine.StateRunning || entry.HistoryLength <= 0 || entry.CleanStepHandoff {
		return Anomaly{}, false
	}
	return Anomaly{
		Kind:            AnomalyCrashResume,
		Title:           renderCrashResumeTitle(entry),
		Slug:            entry.Slug,
		Parent:          entry.Parent,
		State:           entry.State,
		CurrentProducer: entry.CurrentProducer,
	}, true
}

// DetectAnomalies returns every anomaly detected across entry, final and product, in one
// deterministic order: the crash-resume first when present, then the one halt-kind anomaly derived
// from final when present. That order is pinned here because an unordered result cannot be
// table-asserted.
// product is required because every title embeds the slug and shedengine.Status does not carry
// one; Slug and Parent on each returned Anomaly come from product, except on the crash-resume,
// which takes them from entry so the cancelled-context path stays read-free.
// DetectAnomalies calls DetectCrashResume internally rather than reimplementing it.
func DetectAnomalies(entry EntryObservation, final shedengine.Status, product Status) []Anomaly {
	var anomalies []Anomaly

	if crash, ok := DetectCrashResume(entry); ok {
		anomalies = append(anomalies, crash)
	}

	if halt, ok := detectHaltAnomaly(final, product); ok {
		anomalies = append(anomalies, halt)
	}

	return anomalies
}

// detectHaltAnomaly evaluates the three halt-kind triggers (escalation, budget-exhausted,
// producer-hard-failure) against final, all read straight off it. At most one fires, since the
// three triggers partition final.State: blocked with shedengine.ReasonBounceBudgetExhausted,
// blocked with anything else, or failed matching on state alone. shedengine persists
// StateBlocked from exactly two arms, so "blocked and not budget-exhausted" is exactly the
// escalation arm whatever reason its producer supplied.
// The one text the partition still matches is the budget prefix, matched with strings.HasPrefix because the persisted reason carries the exhausted row and a goto way forward after it:
// a producer reason starting with it would read as budget exhaustion, and no shipped producer returns one.
func detectHaltAnomaly(final shedengine.Status, product Status) (Anomaly, bool) {
	var kind AnomalyKind
	switch {
	case final.State == shedengine.StateBlocked && strings.HasPrefix(final.Error, shedengine.ReasonBounceBudgetExhausted):
		kind = AnomalyBudgetExhausted
	case final.State == shedengine.StateBlocked:
		kind = AnomalyEscalation
	case final.State == shedengine.StateFailed:
		kind = AnomalyProducerFailure
	default:
		return Anomaly{}, false
	}

	return Anomaly{
		Kind:            kind,
		Title:           renderHaltTitle(kind, final, product),
		Slug:            product.Slug,
		Parent:          product.Parent,
		State:           final.State,
		CurrentProducer: final.CurrentProducer,
		Error:           final.Error,
		History:         final.History,
	}, true
}

// countDoneEntries returns the number of entries in history whose Producer equals name and whose
// Outcome is shedengine.Done -- that is, how many times this row had previously succeeded. A count
// is required rather than a history length because a blocked run's history grows on every resume:
// a resume of an unfixed escalation re-calls the halting producer and appends another stuck entry,
// so a length-keyed title would mint a fresh title -- and therefore a fresh issue -- on every
// single resume, which is the exact failure the marker exists to prevent. Counting that producer's
// own done entries is invariant under precisely that append and moves only when the row genuinely
// succeeds.
func countDoneEntries(history []shedengine.HistoryEntry, name string) int {
	count := 0
	for _, h := range history {
		if h.Producer == name && h.Outcome == shedengine.Done {
			count++
		}
	}
	return count
}

// renderHaltTitle renders one of the three halt kinds' titles:
// "loom anomaly: <kind> — <slug> — <producer>#<success-count>", where <producer> is
// final.CurrentProducer and <success-count> is countDoneEntries(final.History, <producer>).
func renderHaltTitle(kind AnomalyKind, final shedengine.Status, product Status) string {
	successCount := countDoneEntries(final.History, final.CurrentProducer)
	return fmt.Sprintf("loom anomaly: %s — %s — %s#%d", kind, product.Slug, final.CurrentProducer, successCount)
}

// renderCrashResumeTitle renders the crash-resume title:
// "loom anomaly: crash-resume — <slug> — <producer>@<history-length>", using entry.CurrentProducer
// and entry.HistoryLength, because a crash-resume is observed at most once and needs no stability
// under re-observation, only the finer distinctness that separates a crash at one point in the run
// from a crash at another.
func renderCrashResumeTitle(entry EntryObservation) string {
	return fmt.Sprintf("loom anomaly: %s — %s — %s@%d", AnomalyCrashResume, entry.Slug, entry.CurrentProducer, entry.HistoryLength)
}
