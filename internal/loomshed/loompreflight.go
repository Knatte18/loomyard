// loompreflight.go wires loomengine.CheckSeed in behind the internal constructor, so this task has
// a Loom-Preflight row something inside this package can construct.

package loomshed

import (
	"context"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
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
// to shedengine.Stuck with an empty pointer, and a non-nil error to a returned error. That mapping
// is the whole producer -- CheckSeed reports a determined verdict rather than erroring on anything
// short of an infra failure, so its OK false is a verdict to route and its error is an undetermined
// failure to escalate.
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
		// carries no OnStuck, so its Stuck halts the run for a human who would otherwise be told only
		// Shed's generic "stuck with no OnStuck target".
		logger.Warn("loomshed: seed is not a coherent fresh start", "producer", p.name, "statusPath", p.statusPath, "failures", formatSeedFailures(report))
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{}, nil
}
