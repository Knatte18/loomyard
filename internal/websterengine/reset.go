// reset.go decides, read-only, what `lyx webster reset` may do.
// PlanReset resolves the commit a reset moves the task branch to and the paths the run itself wrote, and refuses with a way forward before fabric is ever called.
// Webster runs no mutating git: every probe here is a read, and the reset itself belongs to fabric.

package websterengine

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ResetTarget names the commit a reset moves the task branch to.
// No raw SHA is accepted: the target is always one the run recorded.
type ResetTarget string

const (
	// ResetToStart is the run's start commit.
	ResetToStart ResetTarget = "start"
	// ResetToPreFix is the HEAD the verify gate started its fixes from.
	ResetToPreFix ResetTarget = "pre-fix"
)

// ResetDeps is what PlanReset reads.
type ResetDeps struct {
	// Geom is the told geometry.
	Geom Geometry
	// State is the loaded state.json, nil when the run has none.
	State *State
	// Engine audits the run's sessions for the writes the dirt check exempts.
	Engine shuttleengine.Engine
	// ParentBranch names the branch the run merges its parent in from; nil or an error leaves the parent-branch refusal unchecked.
	ParentBranch ParentBranchFunc
	// Branch returns the branch checked out in the task worktree as fabric reads it, and errors on a detached HEAD or a branch that is not the pair's own.
	Branch func() (string, error)
}

// ResetPlan is what a reset may do.
type ResetPlan struct {
	// Target is the target the plan resolved.
	Target ResetTarget
	// SHA is the commit the reset moves the task branch to.
	SHA string
	// OwnPaths are the worktree-relative, slash-separated tracked paths the run wrote successfully, exempt from the dirt check.
	OwnPaths []string
}

// wayForwardSteps renders an ordered step list: `way forward: 1) <a>; 2) <b>` for two or more steps, `way forward: <a>` for one, and "" for none.
// It is the one renderer of an ordered step list, so each step is a distinct action and none restates another.
func wayForwardSteps(steps ...string) string {
	switch len(steps) {
	case 0:
		return ""
	case 1:
		return "way forward: " + steps[0]
	}
	numbered := make([]string, len(steps))
	for i, s := range steps {
		numbered[i] = fmt.Sprintf("%d) %s", i+1, s)
	}
	return "way forward: " + strings.Join(numbered, "; ")
}

// resetRefusal builds a refusal of the reset to target, ending in its way forward.
func resetRefusal(to ResetTarget, reason, wayForward string) error {
	return fmt.Errorf("webster: reset --to %s refused: %s; %s", to, reason, wayForward)
}

// PlanReset resolves to against the run's record and refuses, read-only, when a reset would be unsafe.
// The refusals run in this order: run lock held, no state, merge in progress, a checked-out branch that is not the task branch,
// no recorded target, a recorded commit missing from the repository, a target that is not an ancestor of HEAD, and a dirty tracked path outside the run's own writes.
// It plans only a run-recorded commit that is an ancestor of HEAD, exempts only paths the run's own transcripts record a successful write to, and never reads a force flag.
func PlanReset(deps ResetDeps, to ResetTarget) (ResetPlan, error) {
	geom := deps.Geom
	rerun := fmt.Sprintf("re-run `lyx webster reset --to %s`", to)

	active, err := RunActive(geom.ScratchDir)
	if err != nil {
		return ResetPlan{}, err
	}
	if active {
		return ResetPlan{}, fmt.Errorf("%w: %q (run.lock held); %s", ErrRunBusy, geom.ScratchDir,
			wayForwardSteps("wait for it to finish, or check `lyx webster status`"))
	}
	if deps.State == nil {
		return ResetPlan{}, resetRefusal(to, "the run has no state.json", wayForwardSteps("run `lyx webster run` first"))
	}
	if err := refuseMidMerge(geom.git(), geom.WorktreeRoot); err != nil {
		return ResetPlan{}, err
	}
	if err := refuseForeignBranch(deps, to, rerun); err != nil {
		return ResetPlan{}, err
	}

	bases, err := runEvidenceBases(geom, deps.State)
	if err != nil {
		return ResetPlan{}, err
	}
	switch to {
	case ResetToStart:
		if len(bases.Starts) == 0 {
			return ResetPlan{}, resetRefusal(to, "no batch recorded a start commit", wayForwardSteps("run `lyx webster run --fresh`"))
		}
	case ResetToPreFix:
		if deps.State.PreFixHead == "" {
			return ResetPlan{}, resetRefusal(to, "the verify gate recorded no pre-fix head", wayForwardSteps("run `lyx webster run`"))
		}
	default:
		return ResetPlan{}, fmt.Errorf("webster: reset target %q is not %q or %q", to, ResetToStart, ResetToPreFix)
	}

	missing := bases.Missing
	if to == ResetToPreFix {
		missing = nil
		if !geom.git().SHAExists(geom.WorktreeRoot, deps.State.PreFixHead) {
			missing = []string{deps.State.PreFixHead}
		}
	}
	if len(missing) > 0 {
		return ResetPlan{}, fmt.Errorf("webster: reset --to %s refused: %s", to, missingCommitsClause(missing))
	}

	sha, err := resolveResetSHA(geom.WorktreeRoot, deps.State, bases, to)
	if err != nil {
		return ResetPlan{}, err
	}
	head, err := geom.git().HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return ResetPlan{}, err
	}
	reachable, err := geom.git().IsAncestor(geom.WorktreeRoot, sha, head)
	if err != nil {
		return ResetPlan{}, err
	}
	if !reachable {
		fallback := "run `lyx webster reset --to start`"
		if to == ResetToStart {
			fallback = "run `lyx webster run --fresh`"
		}
		return ResetPlan{}, resetRefusal(to, fmt.Sprintf("the recorded commit %s is not an ancestor of HEAD %s, so the branch was rewritten under the run", sha, head),
			wayForwardSteps(fallback))
	}

	own, err := ownTrackedPaths(deps)
	if err != nil {
		return ResetPlan{}, err
	}
	dirty, err := dirtyTrackedPaths(geom.WorktreeRoot)
	if err != nil {
		return ResetPlan{}, err
	}
	var foreign []string
	for _, p := range dirty {
		if !slices.Contains(own, p) {
			foreign = append(foreign, p)
		}
	}
	if len(foreign) > 0 {
		return ResetPlan{}, resetRefusal(to, "tracked changes outside the run's own writes would be lost: "+strings.Join(foreign, ", "),
			wayForwardSteps("commit them on the task branch, or restore each with `git checkout -- <path>`", rerun))
	}
	return ResetPlan{Target: to, SHA: sha, OwnPaths: own}, nil
}

// refuseForeignBranch refuses when the checked-out branch is detached, the parent branch, or one the caller reports is not the pair's own.
func refuseForeignBranch(deps ResetDeps, to ResetTarget, rerun string) error {
	steps := wayForwardSteps("check out the task worktree's own branch with `git switch <task-branch>`", rerun)
	if deps.Branch == nil {
		return resetRefusal(to, "the checked-out branch is unknown", steps)
	}
	branch, err := deps.Branch()
	if err != nil {
		return resetRefusal(to, fmt.Sprintf("the worktree is not on the task branch (%v)", err), steps)
	}
	if deps.ParentBranch != nil {
		parent, perr := deps.ParentBranch()
		if perr == nil && parent != "" && parent == branch {
			return resetRefusal(to, fmt.Sprintf("the worktree is on the parent branch %q, not the task branch", branch), steps)
		}
	}
	return nil
}

// resolveResetSHA returns the commit to reset to.
// The start target is the oldest recorded start; when starts are recorded but none is the oldest of all, it is their octopus merge-base.
func resolveResetSHA(worktree string, st *State, bases evidenceBases, to ResetTarget) (string, error) {
	if to == ResetToPreFix {
		return st.PreFixHead, nil
	}
	if bases.Start != "" {
		return bases.Start, nil
	}
	base, err := octopusMergeBase(worktree, bases.Starts)
	if err != nil {
		return "", resetRefusal(to, fmt.Sprintf("the recorded start commits %s share no single oldest commit and no common ancestor (%v)", strings.Join(bases.Starts, ", "), err),
			wayForwardSteps("run `lyx webster run --fresh`"))
	}
	return base, nil
}

// ownTrackedPaths returns the sorted, deduplicated worktree-relative tracked paths the run wrote successfully.
// A failed write is not evidence: the exemption is by path, not content.
func ownTrackedPaths(deps ResetDeps) ([]string, error) {
	if deps.Engine == nil {
		return nil, nil
	}
	writes, err := loadRunWrites(deps.Engine, deps.State, deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	var own []string
	for _, ev := range slices.Concat(writes.Master, writes.Forks) {
		if !ev.Succeeded {
			continue
		}
		rel, ok, err := trackedRel(deps.Geom.git(), deps.Geom.WorktreeRoot, ev.Path)
		if err != nil {
			return nil, err
		}
		if ok && !slices.Contains(own, rel) {
			own = append(own, rel)
		}
	}
	sort.Strings(own)
	return own, nil
}
