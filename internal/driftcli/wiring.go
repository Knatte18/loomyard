// wiring.go — resolves the worktree a drift verb runs in and builds the seams the drift engine's functions take.
// The seams cover the run status, the git facts and the gate slots.

package driftcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// worktree is the task worktree a drift verb runs in, with its drift paths.
type worktree struct {
	location *lymcwd.Location
	paths    driftengine.Paths
}

// openWorktree resolves the worktree of the command's cwd.
func openWorktree(cmd *cobra.Command) (*worktree, error) {
	cwd, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return nil, err
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	return &worktree{location: location, paths: driftengine.NewPaths(location.AnchorPath())}, nil
}

// openSlugWorktree resolves the worktree of the board slug in the hub the command's cwd is in,
// so a verb acts on a run from any of the hub's worktrees.
// It resolves the slug's sibling worktree at the cwd's own anchor.
// A slug that names no worktree is refused, with a pointer to `lym board list`.
func openSlugWorktree(cmd *cobra.Command, slug string) (*worktree, error) {
	cwd, err := lymcwd.CwdFrom(cmd.Context())
	if err != nil {
		return nil, err
	}
	here, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	root := fabricengine.WorktreePath(here, slug)
	info, statErr := os.Stat(root)
	if slug != filepath.Base(slug) || slug == "." || slug == ".." || errors.Is(statErr, fs.ErrNotExist) || (statErr == nil && !info.IsDir()) {
		return nil, fmt.Errorf("drift: the hub has no worktree %q; way forward: lym board list names the slug of each task", slug)
	}
	if statErr != nil {
		return nil, statErr
	}
	location, err := lymcwd.Resolve(filepath.Join(root, here.AnchorRel))
	if err != nil {
		return nil, err
	}
	return &worktree{location: location, paths: driftengine.NewPaths(location.AnchorPath())}, nil
}

// root returns the worktree root, which names the capture's job; a drift command's relative dir is joined onto it.
func (w *worktree) root() string {
	return w.location.WorktreePath()
}

// anchor returns the anchored directory the capture child starts in and a drift command with no dir runs in.
func (w *worktree) anchor() string {
	return w.location.AnchorPath()
}

// configDirs returns the directories the hub's config modules load under, with this machine's override.
func (w *worktree) configDirs() configengine.Dirs {
	return fabricengine.ConfigDirs(w.location)
}

// config loads the hub's drift config strictly.
func (w *worktree) config() (driftengine.Config, error) {
	return driftengine.LoadConfig(w.configDirs())
}

// verbStatus reads the run status under its lock and whether the capture lock is held.
func (w *worktree) verbStatus() (driftengine.VerbStatus, error) {
	status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(w.location, shedrun.SelfRunID), shedrun.StatusLock(w.location, shedrun.SelfRunID))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return driftengine.VerbStatus{}, err
	}
	if !found {
		return driftengine.VerbStatus{}, errors.New("drift: this worktree has no run status; way forward: lym drift verbs run inside a tapestry or darn run's worktree")
	}
	captureHeld, err := driftengine.ProbeLock(w.paths.CaptureLock)
	if err != nil {
		return driftengine.VerbStatus{}, err
	}
	return driftengine.VerbStatus{Status: status, CaptureHeld: captureHeld}, nil
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
		return "", "", nil, errors.New("drift: this worktree has no origin record, so its branch point is unknown; way forward: lym fabric reconcile")
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

// captureRequest describes a capture of step in this worktree.
// The caller completes the request with its precondition and lock bound.
// A relative step dir is joined onto the worktree root, and a step with no dir runs in the anchored directory.
func (w *worktree) captureRequest(step driftengine.CommandStep) driftengine.CaptureRequest {
	pool, waitDir := w.slots()
	return driftengine.CaptureRequest{
		Paths:       w.paths,
		WorktreeDir: w.root(),
		AnchorDir:   w.anchor(),
		Step:        step,
		Pool:        pool,
		WaitDir:     waitDir,
		Timeout:     stepTimeout(step),
		Now:         time.Now,
	}
}

// compareRequest describes a compare of step in this worktree, which the caller completes with its lock bound.
// A relative step dir is joined onto the worktree root, and a step with no dir runs in the anchored directory.
func (w *worktree) compareRequest(step driftengine.CommandStep) driftengine.CompareRequest {
	pool, waitDir := w.slots()
	return driftengine.CompareRequest{
		Paths:       w.paths,
		WorktreeDir: w.root(),
		AnchorDir:   w.anchor(),
		Step:        step,
		Pool:        pool,
		WaitDir:     waitDir,
		Timeout:     stepTimeout(step),
		Now:         time.Now,
	}
}

// checkDeps builds the seams the drift check reads over this worktree.
// They cover the records and seal reads, the run status, and the row order of the recipe the run's seed names.
// They also cover the current capture and the compare.
// The current capture runs in the gate slot the process's own gate holds, when it runs inside one.
func (w *worktree) checkDeps() (driftengine.Deps, error) {
	seed, found, err := shedrun.ReadSeed(w.location, shedrun.SelfRunID)
	if err != nil {
		return driftengine.Deps{}, err
	}
	if !found {
		return driftengine.Deps{}, errors.New("drift: this worktree has no run seed; way forward: lym drift check runs inside a tapestry or darn run's worktree")
	}
	rows, err := loomrecipe.RowNames(seed.Recipe)
	if err != nil {
		return driftengine.Deps{}, err
	}
	stepOf := func(name string) (driftengine.CommandStep, error) {
		cfg, err := w.config()
		if err != nil {
			return driftengine.CommandStep{}, err
		}
		return cfg.StepWithHubEnv(name, w.configDirs())
	}

	return driftengine.Deps{
		Paths:        w.paths,
		WorktreeRoot: w.root(),
		Config:       w.config,
		Status: func() (shedengine.Status, error) {
			verb, err := w.verbStatus()
			return verb.Status, err
		},
		Rows: rows,
		CommitWithTrailers: func(relPaths []string, trailers []fabricengine.Trailer) error {
			message := fmt.Sprintf("loom: drift records for %s", w.location.WorktreeName)
			_, _, err := fabricengine.CommitAnchoredPathsWithTrailers(fabricengine.NewMutations(""), w.location, relPaths, message, trailers, fabricengine.EnvSyncOptions())
			return err
		},
		TrailerCommit: func(key string, newest bool) (fabricengine.TrailerCommit, bool, error) {
			return fabricengine.FindTrailerCommit(w.location, key, newest)
		},
		FileAt: func(rev, relPath string) ([]byte, bool, error) {
			return fabricengine.AnchoredFileAt(w.location, rev, relPath)
		},
		CaptureCurrent: func(ctx context.Context) (driftengine.CurrentCapture, error) {
			step, err := stepOf("capture")
			if err != nil {
				return driftengine.CurrentCapture{}, err
			}
			request := w.captureRequest(step)
			request.InheritedSlot = os.Getenv(gateslot.InheritEnv)
			return driftengine.CaptureCurrent(ctx, request)
		},
		Compare: func(ctx context.Context) (driftengine.CompareResult, error) {
			step, err := stepOf("compare")
			if err != nil {
				return driftengine.CompareResult{}, err
			}
			request := w.compareRequest(step)
			request.LockRetry = compareLockRetry
			return driftengine.RunCompare(ctx, request)
		},
	}, nil
}

// stepTimeout returns how long a declared step may run.
func stepTimeout(step driftengine.CommandStep) time.Duration {
	return time.Duration(step.TimeoutMin) * time.Minute
}
