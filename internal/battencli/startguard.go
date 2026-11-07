// startguard.go holds batten's pre-run guard.
// It refuses a run a cancel is ending, a start over a dirty source, and a start over the left parts of an earlier pair.
// They are all raised before the pre-run writes anything.
// It also judges a raddle entry's start over the done motif run under its own slug.
// It treats that start as the fresh start the run's removal makes.
// It removes the run only once the start's owner check has passed.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// startGuard carries the local reads the pre-run's guard refuses on, as function values.
// A nil field checks nothing, which only a hand-populated receiver has.
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
	// Promoted reports whether the board holds the slug as a task whose local pair carries the beam mark.
	// Only a promote stopped before its board write leaves that state.
	// A project entry reads false, since a beam project is judged on its own.
	// A slug the board no longer holds also reads false, such as a beam project that its own landing pruned.
	Promoted func() (bool, error)
	// ReachedParent reports whether the run's work reached its parent, given batten's status, as a cancel judges it.
	// A nil ReachedParent reads every run as not reached.
	ReachedParent func(status shedengine.Status, found bool) (bool, error)
	// Project reads whether the slug's board entry is a project, and whether its beam flag is set.
	// A nil Project reads every entry as a task.
	Project func() (project, beam bool, err error)
	// PairMark reads the origin record of the slug's local pair.
	// found is false when the pair has no readable record; marked says whether it carries the beam mark.
	PairMark func() (marked, found bool, err error)
	// Unreleased reads the board status of the slug when it is a task no run starts:
	// boardengine.StatusDraft or boardengine.LegacyStatusDiscussed, and "" for any other entry.
	// A nil Unreleased reads every entry as released.
	Unreleased func() (string, error)
	// Raddle judges the start of a raddle entry.
	// It returns a raddleRefusal when the entry cannot start;
	// any other error is a failed probe, and nil means an entry of another recipe.
	// A nil Raddle checks nothing.
	Raddle func() error
	// ClearsMotifRun reports whether the slug's done run is the motif run of the raddle entry about to start.
	// That start would remove the run; this reports without removing anything.
	// A nil ClearsMotifRun reads every done run as one the start does not clear.
	ClearsMotifRun func() (bool, error)
	// ClearMotifRun removes the slug's done run when it is the motif run of the raddle entry about to start.
	// It reports whether it removed the run.
	// A nil ClearMotifRun removes nothing.
	ClearMotifRun func() (bool, error)
}

// raddleRefusal reports that a raddle entry cannot start, as opposed to a failed probe.
// The guard returns it as it stands, and wraps any other probe error.
type raddleRefusal struct {
	message string
}

// Error returns the refusal's message.
func (r raddleRefusal) Error() string { return r.message }

// hubStartGuard builds the startGuard over the hub of location:
// the cancel marker, the prime's tracked dirty paths, the slug's pair parts and its beam.
// All these are local reads with no network call.
func hubStartGuard(location *lymcwd.Location, slug string) startGuard {
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
			if entry, held := owners.Entries[slug]; !held || !entry.IsTask {
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
		ReachedParent: hubChildReachedParent(location, slug),
		Unreleased: func() (string, error) {
			tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return "", err
			}
			for _, task := range tasks {
				if task.Slug != slug || task.Status == nil {
					continue
				}
				if *task.Status == boardengine.StatusDraft || *task.Status == boardengine.LegacyStatusDiscussed {
					return *task.Status, nil
				}
			}
			return "", nil
		},
		PairParts: func() (fabricengine.PairParts, error) {
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return fabricengine.PairParts{}, err
			}
			return fabricengine.NewTopology(cfg).PairParts(location, slug)
		},
		Raddle:         func() error { return readRaddleStartRefusal(location, slug) },
		ClearsMotifRun: func() (bool, error) { return clearsMotifRunOfRaddleEntry(location, slug) },
		ClearMotifRun:  func() (bool, error) { return ClearMotifRun(location, slug) },
	}
}

// startClearsMotifRun reports whether a start of task clears the done run under its slug instead of refusing it.
// The hub board resolves the task's recipe to raddle, its own or the board's default, and that run's child ran motif.
// Promoting a Sketch keeps the slug of the motif run that made it, so the raddle run always finds that done record.
// A task with its own recipe field decides without opening the board.
// Only an entry with no recipe, or a project, reads the board.
// It removes nothing.
func startClearsMotifRun(location *lymcwd.Location, task boardengine.Task) (bool, error) {
	recipe := task.Recipe
	if task.Kind != boardengine.KindTask || recipe == "" {
		board, err := boardengine.OpenHub(location.HubPath)
		if err != nil {
			return false, err
		}
		recipe = boardRecipe(board, task)
	}
	if recipe != shedrun.RecipeRaddle {
		return false, nil
	}
	childRecipe, err := shedrun.ReadChildRecipe(location, task.Slug)
	return childRecipe == shedrun.RecipeMotif, err
}

// clearsMotifRunOfRaddleEntry reports whether a start of slug clears its done motif run.
// The board's entry under slug decides; an absent entry clears nothing.
// It removes nothing.
func clearsMotifRunOfRaddleEntry(location *lymcwd.Location, slug string) (bool, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return false, err
	}
	index := slices.IndexFunc(tasks, func(task boardengine.Task) bool { return task.Slug == slug })
	if index < 0 {
		return false, nil
	}
	return startClearsMotifRun(location, tasks[index])
}

// readRaddleStartRefusal reads the slug's board entry and, when its recipe is raddle, judges its start.
// An absent entry and an entry of another recipe pass.
func readRaddleStartRefusal(location *lymcwd.Location, slug string) error {
	boardDir := fabricengine.BoardDir(location.HubPath)
	tasks, err := boardengine.ReadTasks(boardDir)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(tasks, func(task boardengine.Task) bool { return task.Slug == slug })
	if index < 0 {
		return nil
	}
	entry := tasks[index]
	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		return err
	}
	if boardRecipe(board, entry) != shedrun.RecipeRaddle {
		return nil
	}
	recordStored := false
	if entry.Discussed {
		_, statErr := os.Stat(filepath.Join(boardDir, boardengine.DecisionFileName(slug)))
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		recordStored = statErr == nil
	}
	store, err := boardengine.ReadRaddleStore(boardDir)
	if err != nil {
		return err
	}
	return raddleStartRefusal(entry, recordStored, store)
}

// raddleStartRefusal judges the start of a raddle entry.
// recordStored reports whether the entry is flagged discussed with its decision record in the board directory.
// It refuses an entry with no stored record, which the raddle recipe has nothing to split.
// It refuses an entry whose slug the store already holds, since the run's filing would collide with the store.
// That covers another Sketch's slug, the store's own Sketch under another created stamp,
// and a slug a stored proposal filed.
// The entry that is the store's own Sketch under its own created stamp passes,
// so a fresh start after a filing that kept the store is not refused.
func raddleStartRefusal(entry boardengine.Task, recordStored bool, store boardengine.RaddleStore) error {
	slug := entry.Slug
	if !recordStored {
		return raddleRefusal{fmt.Sprintf("%s runs the raddle recipe, which splits a Sketch's stored decision record, and it holds none; way forward: run its Discussion first with a motif run: `lym board upsert '{\"slug\":\"%s\",\"recipe\":\"motif\"}'`, release it with `lym board set-status '{\"slug\":\"%s\",\"status\":null}'`, and once the motif run has made it a Sketch, `%s`", slug, slug, slug, boardengine.SketchPromoteCommand(slug))}
	}
	held, isHeld := store.Holds(slug)
	if !isHeld || (held.Sketch == slug && held.Created.Equal(entry.Created)) {
		return nil
	}
	free := slug + "-2"
	for suffix := 3; ; suffix++ {
		if _, taken := store.Holds(free); !taken {
			break
		}
		free = fmt.Sprintf("%s-%d", slug, suffix)
	}
	return raddleRefusal{fmt.Sprintf("the raddle store holds %s as a slug of the Sketch %s, so a run of %s would collide with what that Sketch filed; way forward: run `lym board rename %s %s`, then `lym batten run %s`", slug, held.Sketch, slug, slug, free, free)}
}

// refuse judges verb's start or resume of slug, in order.
// A present cancel marker refuses run, step and goto.
// A run or step of a task whose pair carries the beam mark refuses, with or without a status file.
// Only a stopped promote leaves the mark behind.
// A run whose work reached its parent passes that check, since a cancel refuses it and only the run finishes it.
// A run or step with no status file refuses on tracked dirty paths of the source worktree.
// So does one whose status is at Worktree-Create while the task worktree is absent.
// A task that forks from a beam skips that check:
// its pair forks from the beam's last commit, not from the source worktree.
// A run or step of a project entry is judged by the project check.
// A resume past Worktree-Create passes over all the checks.
// A marked beam pair passes over the dirty-source and left-pair checks.
// A beam project with no marked pair here is left to the checks of a task.
// A run or step with no status file refuses a task whose board status is draft or discussed.
// The refusal names that status and the release that makes it startable.
// A beam project skips that check.
// A beam's cancel leaves its entry draft, and `lym beam create` starts its run again from there.
// A run or step with no status file of a raddle entry refuses when the entry holds no stored record.
// It also refuses when the entry's slug collides with the raddle store.
// A run or step with no status file refuses on a leftover task worktree, records worktree or local records branch.
// A probe error refuses naming the probe and the verb's re-run.
// found and status describe the run's status file.
// taskWorktreePresent reports whether the task worktree is there, which only a status at Worktree-Create reads.
func (g startGuard) refuse(slug, verb string, status shedengine.Status, found, taskWorktreePresent bool) error {
	rerun := fmt.Sprintf(`"lym batten %s %s"`, verb, slug)

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
		if promoted && g.ReachedParent != nil {
			reached, err := g.ReachedParent(status, found)
			if err != nil {
				return probeRefusal("whether the work reached its parent", slug, rerun, err)
			}
			promoted = !reached
		}
		if promoted {
			return fmt.Errorf("the pair of %s carries the beam mark, so an interrupted promote of an older build left it and a run would restart the ended run inside a beam; way forward: run \"lym batten cancel %s --remove\" from the prime, then the release and the run again", slug, slug)
		}
	}

	beamProject := false
	if g.Project != nil {
		var judged bool
		var err error
		beamProject, judged, err = g.refuseProject(slug, rerun, status, found)
		if judged || err != nil {
			return err
		}
	}

	if !found && !beamProject && g.Unreleased != nil {
		unreleased, err := g.Unreleased()
		if err != nil {
			return probeRefusal("the board entry's status", slug, rerun, err)
		}
		if unreleased != "" {
			return fmt.Errorf("%s is a %s task, which no run starts; way forward: run `%s` to release it, then `lym batten %s %s`", slug, unreleased, releaseCommandText(slug), verb, slug)
		}
	}

	if !found && g.Raddle != nil {
		if err := g.Raddle(); err != nil {
			var refusal raddleRefusal
			if errors.As(err, &refusal) {
				return err
			}
			return probeRefusal("the raddle entry's start", slug, rerun, err)
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
			return fmt.Errorf("%s left a pair from an earlier run (%s); run \"lym batten cancel %s --remove\" from the prime, which removes it, then %s and %s",
				slug, strings.Join(left, ", "), slug, releaseCommand(slug), rerun)
		}
	}
	return nil
}

// refuseProject judges a run or step of a project entry, which only a beam project's pawl run makes claimable.
// beam reports whether the entry is a beam project.
// judged is false for an entry that is not a project, which the task checks then judge.
// It is also false for a beam project with no marked pair here, which they judge as a fresh start.
// A grouping project is refused naming the create that makes it a beam.
// A resume past Worktree-Create passes.
// A beam pair here with no readable origin record is refused.
// A marked one passes the dirty-source and left-pair checks, as the legacy beam's or the resumed run's own pair.
func (g startGuard) refuseProject(slug, rerun string, status shedengine.Status, found bool) (beam, judged bool, err error) {
	project, beamFlag, err := g.Project()
	if err != nil {
		return false, true, probeRefusal("the board entry", slug, rerun, err)
	}
	if !project {
		return false, false, nil
	}
	if !beamFlag {
		return false, true, fmt.Errorf("%s is a grouping project, and only a beam project runs; way forward: run \"lym beam create %s\" to make it a beam, then %s", slug, slug, rerun)
	}
	if found && status.CurrentProducer != battenrecipe.NameWorktreeCreate {
		return true, true, nil
	}
	if g.PairParts == nil || g.PairMark == nil {
		return true, false, nil
	}

	parts, err := g.PairParts()
	if err != nil {
		return true, true, probeRefusal("the pair's parts", slug, rerun, err)
	}
	if !parts.TaskWorktree {
		return true, false, nil
	}
	marked, readable, err := g.PairMark()
	if err != nil {
		return true, true, probeRefusal("the beam pair's origin record", slug, rerun, err)
	}
	if !readable {
		return true, true, fmt.Errorf("the pair of the beam %s has no readable origin record; way forward: run \"lym fabric reconcile\" to restore its records worktree, then %s", slug, rerun)
	}
	return true, marked, nil
}

// forksFromPrime reports whether the slug's pair forks from the prime.
// With no parent-beam probe set, every task forks from the prime.
func (g startGuard) forksFromPrime() (bool, error) {
	if g.ParentBeam == nil {
		return true, nil
	}
	beam, err := g.ParentBeam()
	return beam == "", err
}

// leftPartNames names the parts of a pair that make a start refuse:
// the task worktree, the records worktree and the local records branch.
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

// releaseCommandText words the command that releases the draft or discussed task slug.
// The slug is filled in, so a way forward that names the command runs as typed.
func releaseCommandText(slug string) string {
	return fmt.Sprintf(`lym board set-status '{"slug":"%s","status":null}'`, slug)
}

// releaseCommand returns the release command for slug in double quotes, for a way forward that names it inline.
func releaseCommand(slug string) string {
	return `"` + releaseCommandText(slug) + `"`
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
// A run or step over a done status asks the guard's `ClearsMotifRun`, which removes nothing.
// A start it says clears the run is judged as the fresh start it will be, and its removal waits for `claimStart`.
// Verbs other than run, step and goto are not guarded.
// Its reads create nothing except the status lock's ephemeral directory.
func (c *battenCLI) refuseAtStart(verb string) error {
	if verb != "run" && verb != "step" && verb != "goto" {
		return nil
	}
	statusPath := StatusFile(c.location, c.slug)
	statusLockPath := shedrun.StatusLock(c.location, c.slug)
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		return err
	}
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		return fmt.Errorf("battencli: read status file %s: %w", statusPath, err)
	}

	if found && status.State == shedengine.StateDone && verb != "goto" {
		clears, err := c.clearsMotifRun(verb)
		if err != nil {
			return err
		}
		if clears {
			found, status = false, shedengine.Status{}
			c.motifRunClearPending = true
		}
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

// claimStart runs the owner check for a run or step that starts a fresh run.
// It also removes the done motif run that the start replaces.
// A start with no status file, or one replacing a done motif run, is refused when another machine owns the task.
// Nothing is written or removed before that check.
// The owner check pulls the board, so the start is judged as replacing the run once more before the removal.
// A start that the pulled board no longer shows as replacing the run removes nothing.
// A failed removal refuses, naming the re-run, and leaves the removal pending.
func (c *battenCLI) claimStart(verb string) error {
	if verb != "run" && verb != "step" {
		return nil
	}
	_, statErr := os.Stat(StatusFile(c.location, c.slug))
	if c.motifRunClearPending || errors.Is(statErr, fs.ErrNotExist) {
		if err := c.refuseStartOwnedElsewhere(); err != nil {
			return err
		}
	}
	if !c.motifRunClearPending || c.guard.ClearMotifRun == nil {
		return nil
	}
	clears, err := c.clearsMotifRun(verb)
	if err != nil {
		return err
	}
	if !clears {
		c.motifRunClearPending = false
		return nil
	}
	if _, err := c.guard.ClearMotifRun(); err != nil {
		return fmt.Errorf("battencli: remove the done motif run of %s: %w; way forward: fix the cause it names, then run \"lym batten %s %s\" again", c.slug, err, verb, c.slug)
	}
	c.motifRunClearPending = false
	return nil
}

// clearsMotifRun reports whether the start of verb replaces the slug's done motif run.
// A nil `ClearsMotifRun` on the guard reads as false.
// A failed read refuses, naming the re-run of verb.
func (c *battenCLI) clearsMotifRun(verb string) (bool, error) {
	if c.guard.ClearsMotifRun == nil {
		return false, nil
	}
	clears, err := c.guard.ClearsMotifRun()
	if err != nil {
		return false, probeRefusal("whether the done run is a motif run the start clears", c.slug, fmt.Sprintf(`"lym batten %s %s"`, verb, c.slug), err)
	}
	return clears, nil
}
