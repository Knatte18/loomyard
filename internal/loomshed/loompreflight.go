// loompreflight.go wires loomengine.CheckSeed in behind the internal constructor, so this task has
// a Loom-Preflight row something inside this package can construct.

package loomshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// formatSeedFailures renders report's determined failures as a single "check: reason" list,
// semicolon separated, for the log line that surfaces them.
// Like Preflight's own copy, it exists because this row carries no OnStuck and a human is its only
// recovery, so the Failures slice is the only account of why the seed was refused.
func formatSeedFailures(report loomengine.Report) string {
	parts := make([]string, len(report.Failures))
	for i, f := range report.Failures {
		parts[i] = string(f.Check) + ": " + f.Reason
	}
	return strings.Join(parts, "; ")
}

// loomPreflightProducer is the Loom-Preflight producer: it validates that loom's own status file is
// a coherent fresh seed at the told paths, mapping loomengine.CheckSeed's determined Report onto
// shedengine's contract.
type loomPreflightProducer struct {
	name           string
	statusPath     string
	statusLockPath string
}

var _ shedengine.ShedProducer = (*loomPreflightProducer)(nil)

// NewLoomPreflight returns a *loomPreflightProducer named name, checking the status file at
// statusPath (guarded by the lock at statusLockPath) via loomengine.CheckSeed. The return type is
// shedengine.ShedProducer, the seam interface, so the internal/shedrecipe registry can call this
// constructor from outside this package while loomPreflightProducer itself stays unexported.
//
// The constructor is exported for the internal/shedrecipe registry: row 2 spawns nothing and reads
// one JSON file under a caller-supplied path, so it is not a row a tier-1 test substitutes a fake
// for -- unlike row 1, which the moved tests substitute post-build (see the
// row1-substitution-is-a-seam-not-a-fixed-fake Shared Decision).
//
// This wires the production import of internal/loomengine that the Told-Geometry guard test in
// seam_enforcement_test.go already allowlists -- that import does not compromise this package's
// Told-Geometry position, since the invariant's membership predicate is about a direct production
// import of internal/lyxcwd and transitive is explicitly fine.
func NewLoomPreflight(name, statusPath, statusLockPath string) shedengine.ShedProducer {
	return &loomPreflightProducer{name: name, statusPath: statusPath, statusLockPath: statusLockPath}
}

// Call implements shedengine.ShedProducer: it invokes loomengine.CheckSeed(p.statusPath,
// p.statusLockPath, NameLoomPreflight, []string{NamePreflight, NameLoomPreflight}) and maps its
// result -- a Report with OK true to shedengine.Done with an empty pointer, a Report with OK false
// to shedengine.Stuck with an empty Path and the cause on Reason, and a non-nil error to a returned
// error. That mapping is the whole producer -- CheckSeed reports a determined verdict rather than
// erroring on anything short of an infra failure, so its OK false is a verdict to route and its
// error is an undetermined failure to escalate.
//
// NameLoomPreflight and NamePreflight are passed as the expected name and the tolerated history set
// directly, never p.name -- see the told-names-never-come-from-the-producer-name-field Shared
// Decision: the two told names are the row's own durable on-disk identity and the set of history
// producers a resumable blocked run may legitimately have left behind.
func (p *loomPreflightProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	report, err := loomengine.CheckSeed(p.statusPath, p.statusLockPath, NameLoomPreflight, []string{NamePreflight, NameLoomPreflight})
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}

	if !report.OK {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		// Surfaced rather than discarded, for the same reason Preflight surfaces its own: this row
		// carries no OnStuck, so its Stuck halts the run for a human. The cause is returned as the
		// row's reason, which reaches the persisted error and activity.wait, and also logged.
		if p.waivesHalfFinished(report) {
			return shedengine.Done, shedengine.OutputPointer{}, nil
		}
		failures := formatSeedFailures(report)
		logger.Warn("loomshed: seed is not a coherent fresh start", "producer", p.name, "statusPath", p.statusPath, "failures", failures)
		reason := "seed is not a coherent fresh start: " + failures
		for _, f := range report.Failures {
			if f.Check == loomengine.CheckHalfFinished {
				reason += fmt.Sprintf("; way forward: seed a new run, or \"lyx loom goto --to %s\" accepts this run as a deliberate re-entry", NameLoomPreflight)
				break
			}
		}
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// waivesHalfFinished reports whether report's only failures are CheckHalfFinished ones on a deliberate goto re-entry.
// A run an operator moved back onto Preflight or Loom-Preflight is not a fresh start this check protects, so those failures are waived and logged.
// Any other failure, or a history that is not a re-entry, leaves the report to stand.
func (p *loomPreflightProducer) waivesHalfFinished(report loomengine.Report) bool {
	halfFinished := false
	for _, f := range report.Failures {
		if f.Check != loomengine.CheckHalfFinished {
			return false
		}
		halfFinished = true
	}
	if !halfFinished {
		return false
	}
	st, found, err := state.ReadJSONStrict[shedengine.Status](p.statusPath, p.statusLockPath)
	if err != nil || !found || !isGotoReentry(st.History) {
		return false
	}
	logger.Warn("loomshed: waiving half-finished failures on a deliberate goto re-entry", "producer", p.name, "statusPath", p.statusPath, "failures", formatSeedFailures(report))
	return true
}

// isGotoReentry reports whether the latest goto entry in history targets Preflight or Loom-Preflight and every entry after it names only those two rows.
func isGotoReentry(history []shedengine.HistoryEntry) bool {
	tolerated := func(name string) bool { return name == NamePreflight || name == NameLoomPreflight }
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Outcome != shedengine.OutcomeGoto {
			continue
		}
		if !tolerated(history[i].Producer) {
			return false
		}
		for _, e := range history[i+1:] {
			if !tolerated(e.Producer) {
				return false
			}
		}
		return true
	}
	return false
}
