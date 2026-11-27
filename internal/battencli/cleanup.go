// cleanup.go implements `lym batten cleanup`, which removes finished runs' records and scratch from the prime worktree.
// Cleanup decides every slug's removal, so no caller removes more than Cleanup decides.
// The file also holds ClearDoneRun and ClearMotifRun, which remove one slug's done run,
// so the next run under the slug can start.
// ClearDoneRun removes the run's task branch too.

package battencli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// orphanScratchAge is how long an orphan scratch directory must go unmodified before cleanup empties it.
const orphanScratchAge = 24 * time.Hour

// lockFileSuffix ends the name of every lock file in a scratch directory; emptying the directory keeps those files.
const lockFileSuffix = ".lock"

// CleanupSkip names a slug cleanup left alone and why.
type CleanupSkip struct {
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// CleanupReport is what one cleanup pass did, or with Applied false what --apply would do.
type CleanupReport struct {
	// Applied reports whether the pass ran with apply true; false marks a dry run.
	Applied bool
	// Removed lists the slugs whose records run directory was removed.
	Removed []string
	// Emptied lists the slugs whose scratch contents were emptied, beside their lock files.
	Emptied []string
	// Skipped lists every slug left alone, with its reason.
	Skipped []CleanupSkip
	// PushSpawned reports whether the detached push of the records side was started.
	// A spawned push has not landed: an origin refusal of it is read from the hub's failed-push records.
	PushSpawned bool
	// PushError is the records push spawn failure, empty when the push was spawned or none was due.
	// The commits stay for the next records sync.
	PushError string
}

// cleanupDeps carries the fabric calls and the clock that cleanup drives, as function values.
// Tests can replace each with a stub.
type cleanupDeps struct {
	// Committed reads an anchor-relative file at the records HEAD; found is false when it is absent there.
	Committed func(relPath string) (data []byte, found bool, err error)
	// Commit commits the anchor-relative paths on the records side with msg and reports whether a commit was made.
	Commit func(relPaths []string, msg string) (committed bool, err error)
	// Spawn starts the detached push of the records side.
	Spawn func() error
	// Now reads the clock the orphan age is measured against.
	Now func() time.Time
	// PairStands reports whether the slug's pair is on this machine, and whether its task branch is on origin.
	// A cancelled run keeps its record while its pair stands on either.
	PairStands func(slug string) (local, onOrigin bool, err error)
}

// fabricCleanupDeps builds the cleanupDeps that drive the real records side of location's hub.
func fabricCleanupDeps(location *lymcwd.Location) cleanupDeps {
	return cleanupDeps{
		Committed: func(relPath string) ([]byte, bool, error) {
			return fabricengine.CommittedAnchoredFile(location, relPath)
		},
		Commit: func(relPaths []string, msg string) (bool, error) {
			_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, relPaths, msg, fabricengine.EnvSyncOptions())
			return committed, err
		},
		Spawn: func() error {
			_, err := spawnStatusPush(location, "")
			return err
		},
		Now: time.Now,
		PairStands: func(slug string) (bool, bool, error) {
			topology, err := hubTopology(location)
			if err != nil {
				return false, false, err
			}
			parts, err := topology.PairParts(location, slug)
			if err != nil {
				return false, false, err
			}
			onOrigin, err := topology.TaskBranchOnOrigin(location, slug)
			if err != nil {
				return false, false, err
			}
			return parts.TaskWorktree || parts.RecordsWorktree, onOrigin, nil
		},
	}
}

// Cleanup removes, from the hub's prime worktree, the records run directory and scratch contents of every done run.
// It also empties the scratch contents of every aged orphan.
// It never touches a pair, a branch, a lock file or another slug, and it spawns the push of the records side only.
// With apply false it changes nothing and reports what apply true would remove.
// It runs from the prime worktree and from a beam, acting on the prime's location in both, and refuses elsewhere.
// A records commit error stops the pass with that error; a push spawn error is reported on the result.
func Cleanup(location *lymcwd.Location, apply bool) (CleanupReport, error) {
	prime, err := ResolveBattenPrime(location)
	if err != nil {
		return CleanupReport{}, err
	}
	return cleanup(prime, apply, fabricCleanupDeps(prime))
}

// ClearMotifRun removes the run directory and scratch contents of slug's run when it is a done motif or arras run.
// It reports whether it removed them.
// A motif run is the planned first step of a project split or a raddle run;
// an arras run that filed a family left its slug to the family's project, which a beam's run takes next.
// The done record of either must not block the next run under the same slug.
// The removal takes the run's filing mark with it, so the next run's status writes reach the entry again.
// It runs from the prime worktree and from a beam, acting on the prime's location, and refuses elsewhere.
// A run is left alone with cleared false when its run lock is held or it is not done.
// A run whose child-recipe record names another recipe or none is left alone too.
// When a removal fails before its records commit, the status, child-recipe and filing-mark records are written back,
// and the error is returned, so a re-run finds the run again.
// A failed push spawn or scratch emptying only warns, since the done record is gone either way.
func ClearMotifRun(location *lymcwd.Location, slug string) (cleared bool, err error) {
	prime, err := ResolveBattenPrime(location)
	if err != nil {
		return false, err
	}
	return clearDoneRun(prime, slug, fabricCleanupDeps(prime), admitsMotifRun, nil)
}

// ClearDoneRun removes the run directory, scratch contents and task branch of slug's run when it is done.
// It does so whatever the run's recipe.
// It reports whether it removed them.
// A batten run is done once its Worktree-Teardown row finished, so nothing of it is left to drive,
// and its record or its leftover task branch would refuse every later start under the slug.
// The task branch is archived under a tag and deleted locally and on origin, when either holds it.
// A failed branch removal leaves the run's records in place and returns the error, so a re-run finds the run again.
// It runs from the prime worktree and from a beam, acting on the prime's location, and refuses elsewhere.
// A run is left alone with cleared false when its run lock is held or it is not done.
// A records removal that fails before its records commit writes the status, child-recipe and filing-mark records back.
// It then returns the error, so a re-run finds the run again.
// A failed push spawn or scratch emptying only warns, since the done record is gone either way.
func ClearDoneRun(location *lymcwd.Location, slug string) (cleared bool, err error) {
	prime, err := ResolveBattenPrime(location)
	if err != nil {
		return false, err
	}
	topology, err := hubTopology(prime)
	if err != nil {
		return false, fmt.Errorf("battencli: load the fabric config to clear %s: %w", slug, err)
	}
	discardBranch := func(slug string) error { return discardTaskBranchIfPresent(prime, topology, slug) }
	return clearDoneRun(prime, slug, fabricCleanupDeps(prime), admitsAnyRun, discardBranch)
}

// discardTaskBranchIfPresent archives and deletes slug's task branch, locally and on origin, when either holds it.
// It does nothing when neither does.
// The origin half of the discard can be handed to another process and fail there unreported,
// so it reads origin again afterwards and returns an error while the branch is still there.
func discardTaskBranchIfPresent(location *lymcwd.Location, topology *fabricengine.Topology, slug string) error {
	parts, err := topology.PairParts(location, slug)
	if err != nil {
		return fmt.Errorf("read the pair's parts: %w", err)
	}
	onOrigin, err := topology.TaskBranchOnOrigin(location, slug)
	if err != nil {
		return fmt.Errorf("read whether the task branch is on origin: %w", err)
	}
	if !parts.TaskBranch && !onOrigin {
		return nil
	}
	// The dobby's next tick runs this discard again, so no command is named for its re-run.
	if _, err := topology.DiscardTaskBranch(location, slug, ""); err != nil {
		return err
	}
	stillOnOrigin, err := topology.TaskBranchOnOrigin(location, slug)
	if err != nil {
		return fmt.Errorf("read whether the task branch is still on origin: %w", err)
	}
	if stillOnOrigin {
		return fmt.Errorf("the task branch of %s is still on origin after its discard", slug)
	}
	return nil
}

// admitsMotifRun reports whether a done run whose child-recipe record reads recipe is a motif or arras run.
func admitsMotifRun(recipe string) bool {
	return recipe == shedrun.RecipeMotif || recipe == shedrun.RecipeArras
}

// admitsAnyRun admits a done run whatever its child-recipe record reads, none included.
func admitsAnyRun(string) bool {
	return true
}

// clearDoneRun removes slug's done run when admits accepts its child-recipe record, and reports whether it removed it.
// Its records calls and push spawn come from deps, so tests can stub them.
// A non-nil discardBranch runs after the run is judged and before any record is removed;
// its error is returned with the records untouched.
// It judges and removes while holding the slug's run lock and status lock, so a run started meanwhile is never removed.
func clearDoneRun(location *lymcwd.Location, slug string, deps cleanupDeps, admits func(recipe string) bool, discardBranch func(slug string) error) (cleared bool, err error) {
	scratchDir := shedrun.ScratchDir(location, slug)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return false, fmt.Errorf("battencli: create the scratch directory of %s: %w", slug, err)
	}
	runLock, free, err := lock.TryAcquireWriteLock(shedrun.RunLock(location, slug))
	if err != nil {
		return false, fmt.Errorf("battencli: take the run lock of %s: %w", slug, err)
	}
	if !free {
		return false, nil
	}
	defer releaseLock(runLock)
	statusLock, err := lock.AcquireWriteLock(shedrun.StatusLock(location, slug))
	if err != nil {
		return false, fmt.Errorf("battencli: take the status lock of %s: %w", slug, err)
	}
	defer releaseLock(statusLock)

	data, err := os.ReadFile(shedrun.StatusFile(location, slug))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("battencli: read the status of %s: %w", slug, err)
	}
	if done, _, _, _ := judgeStatus(data, "", slug); !done {
		return false, nil
	}
	recipe, err := shedrun.ReadChildRecipe(location, slug)
	if err != nil {
		return false, fmt.Errorf("battencli: read the child recipe of %s: %w", slug, err)
	}
	if !admits(recipe) {
		return false, nil
	}
	filingMarked, err := shedrun.FilingMarked(location, slug)
	if err != nil {
		return false, fmt.Errorf("battencli: read the filing mark of %s: %w", slug, err)
	}

	if discardBranch != nil {
		if err := discardBranch(slug); err != nil {
			return false, fmt.Errorf("battencli: discard the task branch of %s: %w", slug, err)
		}
	}

	run := "run"
	if recipe != "" {
		run = recipe + " run"
	}
	committed, err := removeRunRecords(location, slug, "batten: remove the done "+run+" of "+slug, deps)
	var emptyErr scratchEmptyError
	switch {
	case errors.As(err, &emptyErr):
		logger.Warn("battencli: the done run's records are removed but its scratch is not emptied", "slug", slug, "recipe", recipe, "error", err.Error())
	case err != nil:
		return false, errors.Join(err, restoreDoneRunRecords(location, slug, data, recipe, filingMarked))
	}
	if committed {
		if err := deps.Spawn(); err != nil {
			logger.Warn("battencli: the push of the done run's removal did not spawn; the next records sync carries it", "slug", slug, "recipe", recipe, "error", err.Error())
		}
	}
	return true, nil
}

// restoreDoneRunRecords writes back the status record of slug's run.
// It also writes back the child-recipe record when recipe is not empty, and the filing mark when filingMarked is true.
// It runs when a removal fails before its records commit, so the run reads as the same done run again.
// A re-run of the removal then finishes it.
// The caller holds the slug's status lock.
func restoreDoneRunRecords(location *lymcwd.Location, slug string, status []byte, recipe string, filingMarked bool) error {
	statusFile := shedrun.StatusFile(location, slug)
	if err := os.MkdirAll(filepath.Dir(statusFile), 0o755); err != nil {
		return fmt.Errorf("battencli: restore the status of %s: %w", slug, err)
	}
	if err := os.WriteFile(statusFile, status, 0o644); err != nil {
		return fmt.Errorf("battencli: restore the status of %s: %w", slug, err)
	}
	if recipe != "" {
		if err := shedrun.WriteChildRecipe(location, slug, recipe); err != nil {
			return fmt.Errorf("battencli: restore the child recipe of %s: %w", slug, err)
		}
	}
	if filingMarked {
		if err := shedrun.WriteFilingMark(location, slug); err != nil {
			return fmt.Errorf("battencli: restore the filing mark of %s: %w", slug, err)
		}
	}
	return nil
}

// cleanup runs one pass over every slug and reports what it did, or with apply false what it would do.
// Its records calls, push spawn and clock come from deps, so tests can stub them.
// Each slug is judged under its run lock and status lock.
// A run started meanwhile blocks or is skipped rather than racing the removal.
func cleanup(location *lymcwd.Location, apply bool, deps cleanupDeps) (CleanupReport, error) {
	report := CleanupReport{Applied: apply, Removed: []string{}, Emptied: []string{}, Skipped: []CleanupSkip{}}

	slugs, err := cleanupSlugs(location)
	if err != nil {
		return report, err
	}

	committedAny := false
	for _, slug := range slugs {
		committed, err := cleanupSlug(location, slug, apply, deps, &report)
		committedAny = committedAny || committed
		if err != nil {
			return report, err
		}
	}

	if committedAny {
		if err := deps.Spawn(); err != nil {
			report.PushError = err.Error()
		} else {
			report.PushSpawned = true
		}
	}
	return report, nil
}

// cleanupSlugs returns the sorted union of the directory names under the records root and the scratch root.
func cleanupSlugs(location *lymcwd.Location) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range []string{shedrun.RunsRoot(location), shedrun.ScratchRoot(location)} {
		entries, err := os.ReadDir(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("battencli: list %s: %w", root, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				seen[entry.Name()] = true
			}
		}
	}
	slugs := make([]string, 0, len(seen))
	for slug := range seen {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs, nil
}

// cleanupSlug judges one slug and, with apply, removes what it decided.
// It reports whether it made a records commit; a non-nil error is a commit failure, which stops the pass.
func cleanupSlug(location *lymcwd.Location, slug string, apply bool, deps cleanupDeps, report *CleanupReport) (committed bool, err error) {
	skip := func(reason string) {
		report.Skipped = append(report.Skipped, CleanupSkip{Slug: slug, Reason: reason})
	}

	if err := shedrun.ValidateRunID(slug); err != nil {
		skip(err.Error())
		return false, nil
	}
	if shedrun.IsReserved(slug) {
		skip("reserved run-id")
		return false, nil
	}
	if slug == location.WorktreeName {
		skip("the prime worktree's own run")
		return false, nil
	}

	// Read before any lock file is created:
	// taking a lock in an empty scratch directory would otherwise make an orphan look fresh.
	scratchDir := shedrun.ScratchDir(location, slug)
	scratchNewest, scratchErr := fsx.NewestModTime(scratchDir)

	if apply {
		if err := os.MkdirAll(scratchDir, 0o755); err != nil {
			skip("create scratch directory: " + err.Error())
			return false, nil
		}
	}

	runLock, free, err := takeLock(shedrun.RunLock(location, slug), apply, lock.TryAcquireWriteLock)
	if err != nil {
		skip("run lock: " + err.Error())
		return false, nil
	}
	if !free {
		skip("run lock held")
		return false, nil
	}
	defer releaseLock(runLock)

	statusLock, _, err := takeLock(shedrun.StatusLock(location, slug), apply, func(path string) (*lock.FileLock, bool, error) {
		held, err := lock.AcquireWriteLock(path)
		return held, err == nil, err
	})
	if err != nil {
		skip("status lock: " + err.Error())
		return false, nil
	}
	defer releaseLock(statusLock)

	candidate, _, reason := judgeRunDirectory(location, slug, deps)
	if reason != "" {
		skip(reason)
		return false, nil
	}

	if candidate {
		if apply {
			committed, err = removeRunRecords(location, slug, "batten cleanup: remove the run directory of "+slug, deps)
			var commitErr removalCommitError
			var emptyErr scratchEmptyError
			switch {
			case errors.As(err, &commitErr):
				return committed, err
			case errors.As(err, &emptyErr):
				report.Removed = append(report.Removed, slug)
				skip(err.Error())
				return committed, nil
			case err != nil:
				skip(err.Error())
				return false, nil
			}
		}
		report.Removed = append(report.Removed, slug)
		if scratchErr == nil {
			report.Emptied = append(report.Emptied, slug)
		}
		return committed, nil
	}

	if errors.Is(scratchErr, fs.ErrNotExist) {
		// Nothing to empty: the slug was listed from the records root alone.
		return false, nil
	}
	if scratchErr != nil {
		skip("read scratch age: " + scratchErr.Error())
		return false, nil
	}
	if age := deps.Now().Sub(scratchNewest); age <= orphanScratchAge {
		skip(fmt.Sprintf("orphan scratch modified %s ago, newer than %s", age.Round(time.Minute), orphanScratchAge))
		return false, nil
	}
	if err := emptyScratch(scratchDir, apply); err != nil {
		skip("empty scratch directory: " + err.Error())
		return false, nil
	}
	report.Emptied = append(report.Emptied, slug)
	return false, nil
}

// runRecordsTracked reports whether the records HEAD holds the status or the seed of slug's run.
// It reads both through committed.
func runRecordsTracked(location *lymcwd.Location, slug string, committed func(relPath string) ([]byte, bool, error)) (bool, error) {
	for _, rel := range []string{shedrun.StatusRel(location, slug), shedrun.SeedRel(location, slug)} {
		_, tracked, err := committed(rel)
		if err != nil || tracked {
			return tracked, err
		}
	}
	return false, nil
}

// removalCommitError marks a failure to commit a run's removal to the records side, which stops a cleanup pass.
type removalCommitError struct{ error }

// Unwrap returns the commit failure.
func (e removalCommitError) Unwrap() error { return e.error }

// scratchEmptyError marks a failure to empty a run's scratch contents, after the run's records are removed.
type scratchEmptyError struct{ error }

// Unwrap returns the emptying failure.
func (e scratchEmptyError) Unwrap() error { return e.error }

// removeRunRecords removes slug's records run directory when it is on disk.
// When the records HEAD tracks the run, it commits that removal on the records side with msg.
// It then empties the slug's scratch contents, keeping its lock files.
// It takes no lock and judges nothing; the caller holds the slug's locks and has decided the removal.
// An absent run directory that the records HEAD still tracks commits,
// so a removal whose commit failed earlier finishes on a re-run.
// A run the records HEAD no longer tracks commits nothing and succeeds,
// so a removal that already finished succeeds again.
// committed reports whether a records commit was made.
// A failed commit is a removalCommitError; a failed emptying is a scratchEmptyError.
func removeRunRecords(location *lymcwd.Location, slug, msg string, deps cleanupDeps) (committed bool, err error) {
	if err := os.RemoveAll(shedrun.RunDir(location, slug)); err != nil {
		return false, fmt.Errorf("remove run directory: %w", err)
	}
	tracked, err := runRecordsTracked(location, slug, deps.Committed)
	if err != nil {
		return false, fmt.Errorf("read whether the records HEAD tracks the run of %s: %w", slug, err)
	}
	if tracked {
		committed, err = deps.Commit([]string{shedrun.RunDirRel(location, slug)}, msg)
		if err != nil {
			return committed, removalCommitError{fmt.Errorf("battencli: commit the removal of %s: %w", slug, err)}
		}
	}
	if err := emptyScratch(shedrun.ScratchDir(location, slug), true); err != nil {
		return committed, scratchEmptyError{fmt.Errorf("empty scratch directory: %w", err)}
	}
	return committed, nil
}

// judgeRunDirectory classifies slug by its records run directory, or by the records HEAD when the directory is gone.
// candidate is true for a done run.
// tracked is true when the run directory is already gone from disk, so only the records commit is outstanding.
// A cancelled run is a candidate once neither its pair nor its task branch on origin stands.
// It is a skip while either does.
// A non-empty reason skips the slug.
// A slug with no candidate and no reason is an orphan.
func judgeRunDirectory(location *lymcwd.Location, slug string, deps cleanupDeps) (candidate, tracked bool, reason string) {
	_, statErr := os.Stat(shedrun.RunDir(location, slug))
	switch {
	case statErr == nil:
		data, err := os.ReadFile(shedrun.StatusFile(location, slug))
		if err != nil {
			return false, false, "read status: " + err.Error()
		}
		return judgeRecord(slug, data, "", deps)
	case !errors.Is(statErr, fs.ErrNotExist):
		return false, false, "stat run directory: " + statErr.Error()
	}

	data, found, err := deps.Committed(shedrun.StatusRel(location, slug))
	if err != nil {
		return false, false, "read committed status: " + err.Error()
	}
	if found {
		return judgeRecord(slug, data, "committed ", deps)
	}
	_, seedFound, err := deps.Committed(shedrun.SeedRel(location, slug))
	if err != nil {
		return false, false, "read committed seed: " + err.Error()
	}
	if seedFound {
		return false, false, "tracked run directory with no committed status"
	}
	return false, false, ""
}

// judgeRecord judges the status data of slug's run, read from the disk or, with a prefix, from the records HEAD.
// A cancelled run asks whether its pair stands.
// With neither the pair here nor its task branch on origin, it is a candidate.
// A pair that stands, or a probe that fails, skips the run with a reason naming the step that goes on.
func judgeRecord(slug string, data []byte, prefix string, deps cleanupDeps) (candidate, tracked bool, reason string) {
	candidate, tracked, cancelled, reason := judgeStatus(data, prefix, slug)
	if !cancelled {
		return candidate, tracked, reason
	}
	local, onOrigin, err := deps.PairStands(slug)
	switch {
	case err != nil:
		return false, false, prefix + "run is cancelled and whether its pair stands cannot be read: " + err.Error()
	case local:
		return false, false, fmt.Sprintf("%srun is cancelled and its pair stands; way forward: \"lym fabric remove %s\" removes the pair and the record", prefix, slug)
	case onOrigin:
		return false, false, fmt.Sprintf("%srun is cancelled and its pair stands on another machine; way forward: \"lym batten cancel %s --remove\" on the machine that owns it removes the pair, its task branch and the record", prefix, slug)
	}
	return true, prefix != "", ""
}

// judgeStatus decodes a status file and reports whether the run is done or cancelled.
// A done run is a candidate.
// A cancelled run is neither a candidate nor a skip: cancelled is set, and the caller judges its pair.
// Every other state and an undecodable status is a skip reason, worded with the given prefix.
// A run that is not done is skipped with the cancel of slug as its way forward.
// tracked is set when the prefix is non-empty, which marks a status read from the records HEAD rather than the disk.
func judgeStatus(data []byte, prefix, slug string) (candidate, tracked, cancelled bool, reason string) {
	var st shedengine.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return false, false, false, "decode " + prefix + "status: " + err.Error()
	}
	switch st.State {
	case shedengine.StateDone:
		return true, prefix != "", false, ""
	case shedengine.StateCancelled:
		return false, false, true, ""
	}
	return false, false, false, fmt.Sprintf("%srun is %s; way forward: \"lym batten cancel %s\" ends it, and a later cleanup removes it", prefix, st.State, slug)
}

// takeLock takes the lock at path through acquire.
// Without apply a lock whose file is absent counts as free and is not taken, so a dry run creates no lock file.
// It returns a nil lock when none was taken; free is false when the lock is held by another process.
func takeLock(path string, apply bool, acquire func(path string) (*lock.FileLock, bool, error)) (held *lock.FileLock, free bool, err error) {
	if !apply {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			return nil, true, nil
		}
	}
	held, acquired, err := acquire(path)
	if err != nil {
		return nil, false, err
	}
	return held, acquired, nil
}

// releaseLock releases held, and does nothing when held is nil because no lock was taken.
func releaseLock(held *lock.FileLock) {
	if held != nil {
		_ = held.Release()
	}
}

// emptyScratch removes every file under dir whose name does not end in ".lock".
// It then removes every directory left empty below dir.
// Lock files, their directories and dir itself stay, so no lock is deleted while held or recreated beside a held one.
// With apply false, or when dir is absent, it removes nothing.
func emptyScratch(dir string, apply bool) error {
	if !apply {
		return nil
	}
	var directories []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != dir {
				directories = append(directories, path)
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), lockFileSuffix) {
			return nil
		}
		return os.Remove(path)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(directories[i])
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(directories[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// cleanupCmd builds "lym batten cleanup [--apply]", which removes finished runs' records and scratch.
// It is a dry run unless --apply is given, and it runs from the prime or a beam.
func (c *battenCLI) cleanupCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:         "cleanup",
		Short:       "remove done runs' records and their scratch, and aged orphan scratch",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Without --apply, cleanup is a read-only dry run: it lists what --apply would remove and changes nothing.
With --apply it removes the records run directory of every done run and commits the removal
on the records side, empties that run's scratch beside its lock files, empties the scratch of
any orphan (a scratch directory with no records run directory, untouched for a day), and then
spawns the push of the records. A cancelled run is removed the same way once its pair is gone,
here and on origin; while its pair stands it is skipped, naming the step that removes the pair.
A run that is running, paused, blocked, awaiting or failed, or whose run
lock is held, is skipped and reported with its reason. It never touches a pair, a branch, a
lock file or another run. The dobby of a run's parent worktree removes a done run
itself once it has counted its landing and decided its landing notice, owes no run
watch line for it and has read the run's beam.run_watch; a done motif run is also
cleared by "lym beam create" and by the start of a raddle run under its slug.

Output: {"applied", "removed", "emptied", "skipped", "push_spawned", "push_error"}.
push_spawned says the push was started, not that origin took it; "lym fabric status" lists a push origin refused under failed_pushes.

Example:
  lym batten cleanup
  lym batten cleanup --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			report, err := Cleanup(location, apply)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"applied":      report.Applied,
				"removed":      report.Removed,
				"emptied":      report.Emptied,
				"skipped":      report.Skipped,
				"push_spawned": report.PushSpawned,
				"push_error":   report.PushError,
			}))
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "remove what the dry run lists (default is dry-run/report)")
	return cmd
}
