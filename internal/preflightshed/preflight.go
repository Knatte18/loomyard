// preflight.go implements NewPreflight, the general Preflight producer: a content-free
// shedengine.ShedProducer wrapping internal/preflight.Check, constructed with a told name and cwd.

package preflightshed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// formatFailures renders report's determined failures as a single "check: reason" list, semicolon
// separated, for the log line that surfaces them.
// It exists because a Report's Failures slice is the only account of WHY this row refused, and this
// row carries no OnStuck: a human is the only recovery, and the driver log is the only place they
// can read it.
func formatFailures(report preflight.Report) string {
	parts := make([]string, len(report.Failures))
	for i, f := range report.Failures {
		parts[i] = string(f.Check) + ": " + f.Reason
	}
	return strings.Join(parts, "; ")
}

// wayForward returns the trailing "way forward" clause for report's failures, one fix per failed check that has one, or "" when none does.
// A failed geometry check, and a junction failure from an unreadable fabric.yaml, have no operator-runnable fix and stay bare.
func wayForward(report preflight.Report) string {
	var steps []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			steps = append(steps, s)
		}
	}
	for _, f := range report.Failures {
		switch f.Check {
		case preflight.CheckWorktreeClean:
			add("commit or stash the code changes with git, and commit the _lyx changes with `lyx fabric commit`, then re-step")
		case preflight.CheckFabricSync:
			add("`lyx fabric checkout` re-checks out the current branch and re-syncs the _lyx side, then re-step")
		case preflight.CheckFabricReady, preflight.CheckJunction:
			// An unreadable fabric.yaml is a config decode error reconcile cannot repair.
			if strings.Contains(f.Reason, "cannot load fabric.yaml") {
				continue
			}
			add("`lyx fabric reconcile` recreates a missing _lyx worktree and re-points broken junctions, then re-step")
		}
	}
	if len(steps) == 0 {
		return ""
	}
	return "; way forward: " + strings.Join(steps, "; ") + "; a session lyx refuses the verb from reports status: FAILED and the orch runs it"
}

// preflightProducer is the general Preflight producer: it wraps preflight.Check, mapping its
// determined Report onto shedengine's contract.
type preflightProducer struct {
	name string
	cwd  string
	opts hubreconcile.Options
}

var _ shedengine.ShedProducer = (*preflightProducer)(nil)

// NewPreflight returns a shedengine.ShedProducer that validates cwd via preflight.Check.
//
// name is told rather than hardcoded because a second product names this row independently.
// Per internal/shedadapters' own package doc, the name is used only as a log field and in error
// text -- never compared, parsed, or used for control flow.
func NewPreflight(name, cwd string) shedengine.ShedProducer {
	return newPreflightWith(name, cwd, hubreconcile.Options{})
}

// newPreflightWith is NewPreflight with told options for the hub config reconcile that runs ahead of the check.
func newPreflightWith(name, cwd string, opts hubreconcile.Options) shedengine.ShedProducer {
	return &preflightProducer{name: name, cwd: cwd, opts: opts}
}

// reconcileHub reconciles the hub's config when the build stamp is stale, and does nothing for a repository outside a hub.
func (p *preflightProducer) reconcileHub() error {
	location, inHub := preflight.HubPresent(p.cwd)
	if !inHub {
		return nil
	}
	geometry, inHub := hubgeom.ReconcileGeometry(location)
	if !inHub {
		return nil
	}
	return hubreconcile.Ensure(geometry, p.opts)
}

// reconcileRefusal renders a reconcile failure as the row's Stuck reason, ending with a way forward that names the loom verb to re-enter with.
func reconcileRefusal(err error) string {
	const resume = `run "lyx loom resume" in the task worktree`

	var worktreeErr *hubreconcile.WorktreeError
	if errors.As(err, &worktreeErr) {
		if worktreeErr.File == "" {
			return fmt.Sprintf("hub config reconcile failed in %s: %v; way forward: fix the cause named above, then %s", worktreeErr.Worktree, worktreeErr.Err, resume)
		}
		return fmt.Sprintf("hub config reconcile failed in %s: %s: %v; way forward: fix %s (or run \"lyx config %s\" on it), then %s", worktreeErr.Worktree, worktreeErr.File, worktreeErr.Err, worktreeErr.File, worktreeErr.Module, resume)
	}

	var lockErr *hubreconcile.LockTimeoutError
	if errors.As(err, &lockErr) {
		return fmt.Sprintf("another lyx command holds the hub's reconcile lock %s; way forward: %s once the other reconcile ends", lockErr.Path, resume)
	}

	return fmt.Sprintf("hub config reconcile failed: %v; way forward: fix the cause named above, then %s", err, resume)
}

// Call implements shedengine.ShedProducer: it invokes preflight.Check(p.cwd) and maps its result --
// a Report with OK true to shedengine.Done with an empty pointer, a Report with OK false to
// shedengine.Stuck with an empty Path and the failures on Reason, and a non-nil error to a returned
// error. That mapping is the whole producer -- Check reports a determined verdict rather than
// erroring on anything short of an infra failure, so its OK false is a verdict to route and its
// error is an undetermined failure to escalate. The resolved *lyxcwd.Location Check also returns is
// discarded: this package never touches the resolved location.
func (p *preflightProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// The reconcile runs before the check so its committed config leaves the tree clean for the worktree-clean check.
	if err := p.reconcileHub(); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("preflightshed: hub config reconcile failed", "producer", p.name, "cwd", p.cwd, "error", err)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reconcileRefusal(err)}, nil
	}

	report, _, err := preflight.Check(p.cwd)
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
		// The determined failures are surfaced rather than discarded: this row carries no OnStuck --
		// by design, since nothing in the producer list produces what it gates -- so its Stuck halts
		// the whole run for a human. The failures are returned as the row's reason, which reaches
		// the persisted error and activity.wait, and are also logged. The pointer's Path stays
		// empty: this is a gate signal, not an artifact.
		failures := formatFailures(report)
		logger.Warn("preflightshed: preconditions not met", "producer", p.name, "cwd", p.cwd, "failures", failures)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: "preconditions not met: " + failures + wayForward(report)}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{}, nil
}
