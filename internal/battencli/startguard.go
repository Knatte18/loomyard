// startguard.go holds batten's pre-run guard: the refusals of a run a cancel is ending, of a start over a dirty source and of a start over the left parts of an earlier pair, all raised before the pre-run writes anything.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
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
	// ParentBeam reads the beam the slug's pair forks from over the local board, "" meaning the prime.
	// A nil ParentBeam reads every task as forked from the prime.
	ParentBeam func() (string, error)
	// Promoted reports whether the slug is a task whose local pair carries the beam mark, which only a promote stopped before its board write leaves.
	// A project entry reads false, since a beam project is judged on its own.
	Promoted func() (bool, error)
	// Project reads whether the slug's board entry is a project, and whether its beam flag is set.
	// A nil Project reads every entry as a task.
	Project func() (project, beam bool, err error)
	// PairMark reads the origin record of the slug's local pair: found is false when the pair has no readable record, and marked says whether it carries the beam mark.
	PairMark func() (marked, found bool, err error)
	// LandRefusal refuses the landing of the slug as a beam while work remains under it.
	LandRefusal func() error
}

// hubStartGuard builds the startGuard over the hub of location: the cancel marker, the prime's tracked dirty paths, the slug's pair parts and its beam, all local reads with no network call.
func hubStartGuard(location *lyxcwd.Location, slug string) startGuard {
	return startGuard{
		Marker:      func() (cancelMarkerReading, error) { return readCancelMarker(location, slug) },
		SourceDirty: func() ([]string, error) { return fabricengine.SourceDirtyPaths(location) },
		ParentBeam: func() (string, error) {
			tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return "", err
			}
			return boardengine.ParentBeam(tasks, slug), nil
		},
		Promoted: func() (bool, error) {
			owners, err := boardengine.ReadOwners(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return false, err
			}
			if entry, held := owners.Entries[slug]; held && entry.IsProject {
				return false, nil
			}
			origin, found, err := fabricengine.ReadOriginFor(location, slug)
			if err != nil {
				return false, err
			}
			return found && origin.Beam, nil
		},
		Project: func() (bool, bool, error) {
			owners, err := boardengine.ReadOwners(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return false, false, err
			}
			entry := owners.Entries[slug]
			return entry.IsProject, entry.Beam, nil
		},
		PairMark: func() (bool, bool, error) {
			origin, found, err := fabricengine.ReadOriginFor(location, slug)
			if err != nil {
				return false, false, err
			}
			return origin.Beam, found, nil
		},
		LandRefusal: func() error { return BeamLandRefusal(location, slug) },
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
// A run or step of a task whose pair carries the beam mark refuses, with or without a status file, since only a stopped promote leaves one.
// A run or step with no status file, or one whose status is at Worktree-Create while the task worktree is absent, refuses on tracked dirty paths of the source worktree, unless the task forks from a beam, whose pair forks from the beam's last commit and not from the source worktree.
// A run or step of a project entry is judged by refuseProject alone, which refuses a grouping project and a beam that cannot be landed, and passes a marked beam pair over the next two checks.
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

	if g.Promoted != nil {
		promoted, err := g.Promoted()
		if err != nil {
			return probeRefusal("the pair's origin record", slug, rerun, err)
		}
		if promoted {
			return fmt.Errorf("the pair of %s carries the beam mark, so a promote of it stopped before it finished and a run would restart the ended run inside a beam; way forward: run \"lyx beam promote %s\" to finish the promote", slug, slug)
		}
	}

	if g.Project != nil {
		judged, err := g.refuseProject(slug, rerun, status, found)
		if judged || err != nil {
			return err
		}
	}

	atWorktreeCreate := found && status.CurrentProducer == battenrecipe.NameWorktreeCreate && !taskWorktreePresent
	if (!found || atWorktreeCreate) && g.SourceDirty != nil {
		fromPrime, err := g.forksFromPrime()
		if err != nil {
			return probeRefusal("the task's beam", slug, rerun, err)
		}
		if fromPrime {
			paths, err := g.SourceDirty()
			if err != nil {
				return probeRefusal("the source worktree's tracked changes", slug, rerun, err)
			}
			if len(paths) > 0 {
				return fmt.Errorf("the source worktree has uncommitted tracked changes: %s; commit or revert them in the source worktree, then run %s again", strings.Join(paths, ", "), rerun)
			}
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

// refuseProject judges a run or step of a project entry, which only a beam's landing makes claimable.
// judged is false for an entry that is not a project, which the task checks then judge.
// A grouping project is refused naming the create that makes it a beam.
// A beam's start, or its resume at Worktree-Create, is refused when the beam has no local pair, when the pair has no readable origin record or lacks the beam mark, and while work remains under the beam.
// A marked beam pair passes the dirty-source and left-pair checks, since a landing forks no pair and Finalize guards the worktree it merges into.
// A resume past Worktree-Create passes, since the landing's own rows judge it.
func (g startGuard) refuseProject(slug, rerun string, status shedengine.Status, found bool) (judged bool, err error) {
	project, beam, err := g.Project()
	if err != nil {
		return true, probeRefusal("the board entry", slug, rerun, err)
	}
	if !project {
		return false, nil
	}
	if !beam {
		return true, fmt.Errorf("%s is a grouping project, and only a beam's landing runs a project; way forward: run \"lyx beam create %s\" to make it a beam, then %s", slug, slug, rerun)
	}
	if found && status.CurrentProducer != battenrecipe.NameWorktreeCreate {
		return true, nil
	}

	if g.PairParts != nil {
		parts, err := g.PairParts()
		if err != nil {
			return true, probeRefusal("the pair's parts", slug, rerun, err)
		}
		if !parts.TaskWorktree {
			return true, fmt.Errorf("the beam %s has no pair on this machine; way forward: run \"lyx beam create %s\" to finish the create, or \"lyx adopt %s\" when the beam's branch is on origin, then %s", slug, slug, slug, rerun)
		}
	}
	if g.PairMark != nil {
		marked, readable, err := g.PairMark()
		if err != nil {
			return true, probeRefusal("the beam pair's origin record", slug, rerun, err)
		}
		if !readable {
			return true, fmt.Errorf("the pair of the beam %s has no readable origin record; way forward: run \"lyx fabric reconcile\" to restore its records worktree, then %s", slug, rerun)
		}
		if !marked {
			return true, fmt.Errorf("the pair of the beam %s does not carry the beam mark; way forward: run \"lyx beam create %s\", which marks it, then %s", slug, slug, rerun)
		}
	}
	if g.LandRefusal != nil {
		return true, g.LandRefusal()
	}
	return true, nil
}

// forksFromPrime reports whether the slug's pair forks from the prime, which a nil ParentBeam seam reads every task as.
func (g startGuard) forksFromPrime() (bool, error) {
	if g.ParentBeam == nil {
		return true, nil
	}
	beam, err := g.ParentBeam()
	return beam == "", err
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
