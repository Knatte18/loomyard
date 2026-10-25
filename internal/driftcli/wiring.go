// wiring.go — resolves the worktree a drift verb runs in and builds the seams the driftengine functions are told: the run status, the git facts and the gate slots.

package driftcli

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// worktree is the task worktree a drift verb runs in, with its drift paths.
type worktree struct {
	location *lyxcwd.Location
	paths    driftengine.Paths
}

// openWorktree resolves the worktree of the command's cwd.
func openWorktree(cmd *cobra.Command) (*worktree, error) {
	cwd, err := lyxcwd.CwdFrom(cmd.Context())
	if err != nil {
		return nil, err
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	return &worktree{location: location, paths: driftengine.NewPaths(location.AnchorPath())}, nil
}

// root returns the worktree root a drift command's relative dir is joined onto.
func (w *worktree) root() string {
	return w.location.WorktreePath()
}

// config loads the hub's drift config strictly.
func (w *worktree) config() (driftengine.Config, error) {
	return driftengine.LoadConfig(fabricengine.ConfigDirs(w.location))
}

// verbStatus reads the run status under its lock, the recipe from the run's seed, the capture lock and the agent-session mark.
func (w *worktree) verbStatus() (driftengine.VerbStatus, error) {
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](shedrun.StatusFile(w.location, shedrun.SelfRunID), shedrun.StatusLock(w.location, shedrun.SelfRunID))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return driftengine.VerbStatus{}, err
	}
	if !found {
		return driftengine.VerbStatus{}, errors.New("drift: this worktree has no run status; way forward: lyx drift verbs run inside a loom or darn run's worktree")
	}
	seed, seedFound, err := shedrun.ReadSeed(w.location, shedrun.SelfRunID)
	if err != nil {
		return driftengine.VerbStatus{}, err
	}
	if !seedFound {
		return driftengine.VerbStatus{}, errors.New("drift: this worktree's run has no seed; way forward: lyx drift verbs run inside a loom or darn run's worktree")
	}
	captureHeld, err := driftengine.ProbeLock(w.paths.CaptureLock)
	if err != nil {
		return driftengine.VerbStatus{}, err
	}
	agentSession, err := driftengine.AgentSessionOpen(w.paths)
	if err != nil {
		return driftengine.VerbStatus{}, err
	}
	return driftengine.VerbStatus{
		Status:       status,
		AwaitRow:     seed.Recipe == shedrun.RecipeLoom,
		CaptureHeld:  captureHeld,
		AgentSession: agentSession,
	}, nil
}

// gitFacts reads HEAD, the branch point and the tracked dirty paths of the code checkout.
// The branch point is the merge base of HEAD with the parent branch the pair's origin record names.
func (w *worktree) gitFacts() (head, branchPoint string, dirty []string, err error) {
	fabric, err := fabricengine.Open(w.location)
	if err != nil {
		return "", "", nil, err
	}
	if head, err = fabric.HeadSHA(); err != nil {
		return "", "", nil, err
	}
	origin, found, err := fabricengine.ReadOrigin(w.location)
	if err != nil {
		return "", "", nil, err
	}
	if !found {
		return "", "", nil, errors.New("drift: this worktree has no origin record, so its branch point is unknown; way forward: lyx fabric reconcile")
	}
	if branchPoint, err = fabric.MergeBase(origin.ParentBranch); err != nil {
		return "", "", nil, fmt.Errorf("drift: find the branch point against %s: %w", origin.ParentBranch, err)
	}
	if _, dirty, err = fabric.CodeDirtyPaths(); err != nil {
		return "", "", nil, err
	}
	return head, branchPoint, dirty, nil
}

// slots returns the hub's gate-slot pool and the directory a step's wait record goes to.
func (w *worktree) slots() (*gateslot.Pool, string) {
	return hubgeom.GateSlots(w.location), gateslot.WaitDir(w.location.AnchorPath())
}

// stepTimeout returns how long a declared step may run.
func stepTimeout(step driftengine.CommandStep) time.Duration {
	return time.Duration(step.TimeoutMin) * time.Minute
}
