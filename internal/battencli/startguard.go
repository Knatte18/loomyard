// startguard.go holds batten's pre-run guard: the refusals of a run a cancel is ending, of a start over a dirty source and of a start over the left parts of an earlier pair, all raised before the pre-run writes anything.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// startGuard carries the local reads the pre-run's guard refuses on, as function values.
// A nil seam checks nothing, which only a hand-populated receiver has.
type startGuard struct {
	// Marker reads the slug's cancel marker and the liveness of its writer.
	Marker func() (cancelMarkerReading, error)
	// SourceDirty lists the tracked uncommitted paths of the source worktree.
	SourceDirty func() ([]string, error)
	// PairParts reads which parts of the slug's pair are present locally.
	PairParts func() (fabricengine.PairParts, error)
}

// hubStartGuard builds the startGuard over the hub of location: the cancel marker, the prime's tracked dirty paths and the slug's pair parts, all local reads with no network call.
func hubStartGuard(location *lyxcwd.Location, slug string) startGuard {
	return startGuard{
		Marker:      func() (cancelMarkerReading, error) { return readCancelMarker(location, slug) },
		SourceDirty: func() ([]string, error) { return fabricengine.SourceDirtyPaths(location) },
		PairParts: func() (fabricengine.PairParts, error) {
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return fabricengine.PairParts{}, err
			}
			return fabricengine.NewTopology(cfg).PairParts(location, slug)
		},
	}
}

// refuse judges verb's start or resume of slug, in order.
// A present cancel marker refuses run, step and goto.
// A run or step with no status file, or one whose status is at Worktree-Create while the task worktree is absent, refuses on tracked dirty paths of the source worktree.
// A run or step with no status file refuses when a task worktree, records worktree or local records branch is left from an earlier run.
// A probe error refuses naming the probe and the verb's re-run.
// found and status describe the run's status file, and taskWorktreePresent whether the task worktree is there, which only a status at Worktree-Create reads.
func (g startGuard) refuse(slug, verb string, status shedengine.Status, found, taskWorktreePresent bool) error {
	rerun := fmt.Sprintf(`"lyx batten %s %s"`, verb, slug)

	if (verb == "run" || verb == "step" || verb == "goto") && g.Marker != nil {
		reading, err := g.Marker()
		if err != nil {
			return probeRefusal("the cancel marker", slug, rerun, err)
		}
		if reading.Present {
			return cancelMarkerRefusal(slug, reading)
		}
	}
	if verb != "run" && verb != "step" {
		return nil
	}

	atWorktreeCreate := found && status.CurrentProducer == battenrecipe.NameWorktreeCreate && !taskWorktreePresent
	if (!found || atWorktreeCreate) && g.SourceDirty != nil {
		paths, err := g.SourceDirty()
		if err != nil {
			return probeRefusal("the source worktree's tracked changes", slug, rerun, err)
		}
		if len(paths) > 0 {
			return fmt.Errorf("the source worktree has uncommitted tracked changes: %s; commit or revert them in the source worktree, then run %s again", strings.Join(paths, ", "), rerun)
		}
	}

	if !found && g.PairParts != nil {
		parts, err := g.PairParts()
		if err != nil {
			return probeRefusal("the pair's parts", slug, rerun, err)
		}
		if left := leftPartNames(parts); len(left) > 0 {
			return fmt.Errorf("%s left a pair from an earlier run (%s); run \"lyx batten cancel %s --remove\" from the prime, which removes it, then %s",
				slug, strings.Join(left, ", "), slug, rerun)
		}
	}
	return nil
}

// leftPartNames names the parts of a pair that make a start refuse: the task worktree, the records worktree and the local records branch.
// A task branch left alone is not named, since the add refuses it and Worktree-Create halts on it.
func leftPartNames(parts fabricengine.PairParts) []string {
	var names []string
	if parts.TaskWorktree {
		names = append(names, "task worktree")
	}
	if parts.RecordsWorktree {
		names = append(names, "records worktree")
	}
	if parts.RecordsBranch {
		names = append(names, "records branch")
	}
	return names
}

// probeRefusal words the refusal of a guard probe that failed with err, naming the probe and the step to run again.
func probeRefusal(probe, slug, rerun string, err error) error {
	return fmt.Errorf("battencli: could not read %s of %s: %w; way forward: fix the cause, then run %s again", probe, slug, err, rerun)
}

// refuseCancelMarker refuses when a cancel of the slug is under way or was interrupted.
// A receiver with no guard seam checks nothing.
func (c *battenCLI) refuseCancelMarker() error {
	if c.guard.Marker == nil {
		return nil
	}
	reading, err := c.guard.Marker()
	if err != nil {
		return probeRefusal("the cancel marker", c.slug, "the verb", err)
	}
	if reading.Present {
		return cancelMarkerRefusal(c.slug, reading)
	}
	return nil
}

// refuseAtStart reads the run's status file and applies the guard to verb's start or resume.
// Verbs other than run, step and goto are not guarded.
// Its reads create nothing but the status lock's ephemeral directory, which checkRunStatus creates the same way.
func (c *battenCLI) refuseAtStart(verb string) error {
	if verb != "run" && verb != "step" && verb != "goto" {
		return nil
	}
	statusPath := StatusFile(c.location, c.slug)
	statusLockPath := shedrun.StatusLock(c.location, c.slug)
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		return err
	}
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		return fmt.Errorf("battencli: read status file %s: %w", statusPath, err)
	}

	var taskPresent bool
	if found && status.CurrentProducer == battenrecipe.NameWorktreeCreate {
		taskPresent, err = taskWorktreePresent(c.location, c.slug)
		if err != nil {
			return err
		}
	}
	return c.guard.refuse(c.slug, verb, status, found, taskPresent)
}
