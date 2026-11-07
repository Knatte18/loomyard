// preflight.go holds the general Preflight producer.
// It is a content-free shedengine.ShedProducer that wraps internal/preflight.Check.
// NewPreflight constructs it from a told name and cwd, and NewPreflightWithChecks also takes optional checks.

package preflightshed

import (
	"context"
	"errors"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// formatFailures renders the failures in report as a "check: reason" list, separated by semicolons, for the log.
// The failures are the only account of why this row refused.
// This row carries no `OnStuck`, so a human is the only recovery,
// and the driver log is the only place they can read the failures.
func formatFailures(report preflight.Report) string {
	parts := make([]string, len(report.Failures))
	for i, f := range report.Failures {
		parts[i] = string(f.Check) + ": " + f.Reason
	}
	return strings.Join(parts, "; ")
}

// wayForward returns the trailing "way forward" clause for the failures in report.
// It joins each distinct fix that a failed check calls for, and returns "" when none does.
// A failed geometry check has no operator-runnable fix.
// A junction failure from an unreadable fabric.yaml has none either.
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
			add("commit or stash the code changes with git, and commit the _lym changes with `lym fabric commit`, then re-step")
		case preflight.CheckFabricSync:
			add("`lym fabric checkout` re-checks out the current branch and re-syncs the _lym side, then re-step")
		case preflight.CheckFabricReady, preflight.CheckJunction:
			// An unreadable fabric.yaml is a config decode error reconcile cannot repair.
			if strings.Contains(f.Reason, "cannot load fabric.yaml") {
				continue
			}
			add("`lym fabric reconcile` recreates a missing _lym worktree and re-points broken junctions, then re-step")
		}
	}
	if len(steps) == 0 {
		return ""
	}
	return "; way forward: " + strings.Join(steps, "; ") + "; a session lym refuses the verb from reports status: FAILED and the orch runs it"
}

// Checks holds the optional checks the Preflight row runs after the hub reconcile and before preflight.Check.
// They run in field order, and a nil field runs no check.
type Checks struct {
	// Batcher loads batcher.yaml.
	// A failure makes the row stuck, with the batchifier reason and the way forward its error calls for.
	Batcher func() error
	// Stencils composes the recipe's seat tables.
	// It refuses a stencil marker that no value fills, or a table that cannot be built.
	// A failure makes the row stuck, with the stencils reason and the way forward its error carries.
	Stencils func() error
	// Drift loads drift.yaml and refuses a command a run would be refused at a Drift row.
	// A failure makes the row stuck, with the drift reason and the way forward its error carries.
	Drift func() error
}

// preflightProducer is the general Preflight producer.
// It wraps preflight.Check and maps its determined Report onto shedengine's contract.
type preflightProducer struct {
	name   string
	cwd    string
	opts   hubreconcile.Options
	checks Checks
}

var _ shedengine.ShedProducer = (*preflightProducer)(nil)

// batchifierReasonPrefix leads the stuck reason of a failed batcher check; the error text follows.
const batchifierReasonPrefix = "active batchifier did not resolve: "

// batchifierWayForward closes the batchifier reason;
// the fault is in batcher.yaml, so the operator fixes it there.
const batchifierWayForward = "; way forward: fix batcher.yaml (its active: key or the profile it names), then re-step"

// retiredKeyWayForward completes the way forward that a retired-key error already ends with.
// It tells the operator to run `lym config reconcile --apply`, which migrates a worktree or local file.
// For the repo default, it tells the operator to run `lym fabric reconcile`.
const retiredKeyWayForward = ", then re-step"

// stencilsReasonPrefix leads the stuck reason of a failed stencils check;
// the error text, which carries its own way forward, follows.
const stencilsReasonPrefix = "seat stencils check failed: "

// driftReasonPrefix leads the stuck reason of a failed drift check;
// the error text, which carries its own way forward, follows.
const driftReasonPrefix = "drift config check failed: "

// NewPreflight returns a shedengine.ShedProducer that validates cwd via preflight.Check.
//
// name is told rather than hardcoded, because other products name this row independently.
// The name is used only as a log field and in error text, never compared, parsed, or used for control flow.
func NewPreflight(name, cwd string) shedengine.ShedProducer {
	return NewPreflightWithChecks(name, cwd, Checks{})
}

// NewPreflightWithChecks returns the Preflight producer with the optional checks.
// They run after the hub reconcile and before preflight.Check,
// so a failure stops the run before any agent spends tokens.
// The checks catch a broken batcher.yaml and a stencil the running binary cannot fill.
// They also catch a drift command that a Drift row would refuse.
func NewPreflightWithChecks(name, cwd string, checks Checks) shedengine.ShedProducer {
	return &preflightProducer{name: name, cwd: cwd, checks: checks}
}

// newPreflightWith returns the producer for name and cwd, with opts for the hub config reconcile.
// That reconcile runs ahead of the check.
func newPreflightWith(name, cwd string, opts hubreconcile.Options) shedengine.ShedProducer {
	return &preflightProducer{name: name, cwd: cwd, opts: opts}
}

// batcherFailure runs the told batcher check and returns the stuck reason for its failure.
// It returns "" when the check passed or none is told.
// A retired-key error closes with the reconcile way forward its own text already carries;
// any other error closes with the fix-batcher.yaml way forward.
// A cancelled context wins over the failure and comes back as the error.
func (p *preflightProducer) batcherFailure(ctx context.Context) (string, error) {
	if p.checks.Batcher == nil {
		return "", nil
	}
	berr := p.checks.Batcher()
	if berr == nil {
		return "", nil
	}
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", cerr
	}
	logger.Warn("preflightshed: batcher.yaml did not load", "producer", p.name, "cwd", p.cwd, "cause", berr)
	wayForward := batchifierWayForward
	if errors.Is(berr, batcher.ErrRetiredKey) {
		wayForward = retiredKeyWayForward
	}
	return batchifierReasonPrefix + berr.Error() + wayForward, nil
}

// stencilsFailure runs the told stencils check and returns the stuck reason for its failure.
// It returns "" when the check passed or none is told.
// A cancelled context wins over the failure and comes back as the error.
func (p *preflightProducer) stencilsFailure(ctx context.Context) (string, error) {
	return p.prefixedFailure(ctx, p.checks.Stencils, stencilsReasonPrefix)
}

// driftFailure runs the told drift check and returns the stuck reason for its failure.
// It returns "" when the check passed or none is told.
// A cancelled context wins over the failure and comes back as the error.
func (p *preflightProducer) driftFailure(ctx context.Context) (string, error) {
	return p.prefixedFailure(ctx, p.checks.Drift, driftReasonPrefix)
}

// prefixedFailure runs check, when told, and logs its failure.
// On failure it returns prefix followed by the error's text, which carries its own way forward.
// It returns "" when the check passed or none is told.
// A cancelled context wins over the failure and comes back as the error.
func (p *preflightProducer) prefixedFailure(ctx context.Context, check func() error, prefix string) (string, error) {
	if check == nil {
		return "", nil
	}
	failure := check()
	if failure == nil {
		return "", nil
	}
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", cerr
	}
	logger.Warn("preflightshed: a check failed", "producer", p.name, "cwd", p.cwd, "check", strings.TrimSuffix(prefix, ": "), "cause", failure)
	return prefix + failure.Error(), nil
}

// reconcileHub reconciles the hub's config when the build stamp is stale.
// It does nothing for a repository outside a hub.
// A failed reconcile is warned about and the row goes on.
func (p *preflightProducer) reconcileHub() {
	location, inHub := preflight.HubPresent(p.cwd)
	if !inHub {
		return
	}
	geometry, inHub := hubgeom.ReconcileGeometry(location)
	if !inHub {
		return
	}
	hubreconcile.EnsureOrWarn(geometry, p.opts)
}

// Call implements shedengine.ShedProducer by running preflight.Check on p.cwd.
// A Report with OK true maps to shedengine.Done with an empty pointer.
// A Report with OK false maps to shedengine.Stuck with an empty Path and the failures on Reason.
// A non-nil error is returned.
// That mapping is the whole producer.
// Check reports a determined verdict rather than erroring on anything short of an infra failure.
// So OK false is a verdict to route, and its error is an undetermined failure to escalate.
// The resolved location that Check also returns is discarded, since this package never touches it.
func (p *preflightProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// The reconcile runs before the check so its committed config leaves the tree clean for the worktree-clean check.
	p.reconcileHub()
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}

	for _, check := range []func(context.Context) (string, error){p.batcherFailure, p.stencilsFailure, p.driftFailure} {
		reason, err := check(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if reason != "" {
			return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
		}
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
		// The determined failures are surfaced rather than discarded.
		// This row carries no OnStuck by design:
		// nothing in the producer list produces what it gates, so its Stuck halts the whole run for a human.
		// The failures are returned as the row's reason, which reaches the persisted error and activity.wait,
		// and they are also logged.
		// The pointer's Path stays empty: this is a gate signal, not an artifact.
		failures := formatFailures(report)
		logger.Warn("preflightshed: preconditions not met", "producer", p.name, "cwd", p.cwd, "failures", failures)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: "preconditions not met: " + failures + wayForward(report)}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{}, nil
}
