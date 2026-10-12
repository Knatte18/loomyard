// cleanup.go implements "lyx batten cleanup": the dry-run-by-default removal of finished runs' records and scratch from the hub's prime worktree.
//
// Cleanup is the one function the verb and "lyx cleanup" both call; every refusal and skip lives in cleanup, so neither caller can remove more than cleanup decides.
// It deletes only a done slug's records run directory and the scratch contents beside its lock files, or an aged orphan's scratch contents.
// It never touches a pair, a branch, a lock file or another slug, and it pushes the records side only.

package battencli

import (
	"bytes"
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// orphanScratchAge is how long an orphan scratch directory must go unmodified before cleanup empties it.
const orphanScratchAge = 24 * time.Hour

// lockFileSuffix ends the name of every lock file a scratch directory holds; emptyScratch keeps them all.
const lockFileSuffix = ".lock"

// CleanupSkip names a slug cleanup left alone and why.
type CleanupSkip struct {
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// CleanupReport is what one cleanup pass did, or with Applied false what --apply would do.
type CleanupReport struct {
	// Applied reports whether the pass changed anything.
	Applied bool
	// Removed lists the slugs whose records run directory was removed and committed.
	Removed []string
	// Emptied lists the slugs whose scratch contents were emptied, beside their lock files.
	Emptied []string
	// Skipped lists every slug left alone, with its reason.
	Skipped []CleanupSkip
	// PushError is the records push failure, empty when the push succeeded or none was due.
	// The commits stay for the next records sync.
	PushError string
}

// cleanupDeps carries the fabric calls and the clock cleanup drives, as function values, so a Tier 1 test drives every branch with stubs.
type cleanupDeps struct {
	// Committed reads an anchor-relative file at the records HEAD; found is false when it is absent there.
	Committed func(relPath string) (data []byte, found bool, err error)
	// Commit commits the anchor-relative paths on the records side with msg and reports whether a commit was made.
	Commit func(relPaths []string, msg string) (committed bool, err error)
	// Push pushes the records side.
	Push func() error
	// Now reads the clock the orphan age is measured against.
	Now func() time.Time
}

// fabricCleanupDeps builds the cleanupDeps that drive the real records side of location's hub.
func fabricCleanupDeps(location *lyxcwd.Location) cleanupDeps {
	return cleanupDeps{
		Committed: func(relPath string) ([]byte, bool, error) {
			return fabricengine.CommittedAnchoredFile(location, relPath)
		},
		Commit: func(relPaths []string, msg string) (bool, error) {
			_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, relPaths, msg, fabricengine.EnvSyncOptions())
			return committed, err
		},
		Push: func() error {
			_, err := fabricengine.PushAnchored(location, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait)
			return err
		},
		Now: time.Now,
	}
}

// Cleanup removes, from the hub's prime worktree, the records run directory and scratch contents of every done run, and the scratch contents of every aged orphan.
// With apply false it changes nothing and reports exactly what apply true removes.
// It refuses outside the prime worktree.
// A records commit error stops the pass with that error; a push error is reported on the result.
func Cleanup(location *lyxcwd.Location, apply bool) (CleanupReport, error) {
	if err := RefuseUnlessPrime(location); err != nil {
		return CleanupReport{}, err
	}
	return cleanup(location, apply, fabricCleanupDeps(location))
}

// cleanup is Cleanup over injected deps.
// Each slug is judged under its run lock and status lock, so a run started meanwhile blocks or is skipped rather than racing the removal.
func cleanup(location *lyxcwd.Location, apply bool, deps cleanupDeps) (CleanupReport, error) {
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
		if err := deps.Push(); err != nil {
			report.PushError = err.Error()
		}
	}
	return report, nil
}

// cleanupSlugs returns the sorted union of the directory names under the records root and the scratch root.
func cleanupSlugs(location *lyxcwd.Location) ([]string, error) {
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
func cleanupSlug(location *lyxcwd.Location, slug string, apply bool, deps cleanupDeps, report *CleanupReport) (committed bool, err error) {
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

	// Read before any lock file is created: taking a lock in an empty scratch directory would otherwise make an orphan look fresh.
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

	candidate, tracked, reason := judgeRunDirectory(location, slug, deps)
	if reason != "" {
		skip(reason)
		return false, nil
	}

	if candidate {
		if apply && !tracked {
			if err := os.RemoveAll(shedrun.RunDir(location, slug)); err != nil {
				skip("remove run directory: " + err.Error())
				return false, nil
			}
		}
		if apply {
			committed, err = deps.Commit([]string{shedrun.RunDirRel(location, slug)}, "batten cleanup: remove the run directory of "+slug)
			if err != nil {
				return committed, fmt.Errorf("battencli: commit the removal of %s: %w", slug, err)
			}
		}
		report.Removed = append(report.Removed, slug)
	} else {
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
	}

	if err := emptyScratch(scratchDir, apply); err != nil {
		skip("empty scratch directory: " + err.Error())
		return committed, nil
	}
	if scratchErr == nil {
		report.Emptied = append(report.Emptied, slug)
	}
	return committed, nil
}

// judgeRunDirectory decides what the records run directory of slug makes the slug.
// candidate is true for a done run; tracked is true when its directory is already gone from disk and only the records commit is outstanding.
// A non-empty reason skips the slug.
// With candidate and reason both clear, the slug is an orphan.
func judgeRunDirectory(location *lyxcwd.Location, slug string, deps cleanupDeps) (candidate, tracked bool, reason string) {
	_, statErr := os.Stat(shedrun.RunDir(location, slug))
	switch {
	case statErr == nil:
		data, err := os.ReadFile(shedrun.StatusFile(location, slug))
		if err != nil {
			return false, false, "read status: " + err.Error()
		}
		return judgeStatus(data, "")
	case !errors.Is(statErr, fs.ErrNotExist):
		return false, false, "stat run directory: " + statErr.Error()
	}

	data, found, err := deps.Committed(shedrun.StatusRel(location, slug))
	if err != nil {
		return false, false, "read committed status: " + err.Error()
	}
	if found {
		return judgeStatus(data, "committed ")
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

// judgeStatus decodes a status file and answers whether the run is done.
// A done run is a candidate; every other state and an undecodable status is a skip reason, worded with the given prefix before "status".
// tracked is set when the prefix is non-empty, which marks a status read from the records HEAD rather than the disk.
func judgeStatus(data []byte, prefix string) (candidate, tracked bool, reason string) {
	var st shedengine.Status
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&st); err != nil {
		return false, false, "decode " + prefix + "status: " + err.Error()
	}
	if st.State != shedengine.StateDone {
		return false, false, fmt.Sprintf("%srun is %s", prefix, st.State)
	}
	return true, prefix != "", ""
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

// releaseLock releases a lock takeLock returned, which may be nil.
func releaseLock(held *lock.FileLock) {
	if held != nil {
		_ = held.Release()
	}
}

// emptyScratch removes every file under dir whose name does not end in ".lock", then every directory left empty below it.
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

// cleanupCmd builds "lyx batten cleanup [--apply]", the prime-only dry-run-by-default removal of finished runs' records and scratch.
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
pushes the records. A run that is running, paused, blocked, awaiting or failed, or whose run
lock is held, is skipped and reported with its reason. It never touches a pair, a branch, a
lock file or another run.

Output: {"applied", "removed", "emptied", "skipped", "push_error"}.

Example:
  lyx batten cleanup
  lyx batten cleanup --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lyxcwd.Resolve(cwd)
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
				"applied":    report.Applied,
				"removed":    report.Removed,
				"emptied":    report.Emptied,
				"skipped":    report.Skipped,
				"push_error": report.PushError,
			}))
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "remove what the dry run lists (default is dry-run/report)")
	return cmd
}
