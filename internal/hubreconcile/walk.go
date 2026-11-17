// walk.go walks a hub's worktrees under the hub lock.
// It reconciles each worktree's config, commits what was written, and stamps the build.

package hubreconcile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// walkHooks are test seams run at fixed points of the walk; a nil hook is skipped.
type walkHooks struct {
	// afterEnumerate runs after each listing of the hub's code worktrees.
	afterEnumerate func()
	// beforeCommit runs between a worktree's config write and its commit, with the worktree root.
	beforeCommit func(worktreePath string)
	// afterHubWideWrite runs in the board write once the hub-wide reconcile has written the files and the board marker.
	// A non-nil error fails the write as a commit failure would.
	afterHubWideWrite func() error
}

// ensure brings the hub's config up to the running build, under the hub lock.
// It returns at once when the build stamp is current, except for a pair call, which reconciles only that pair.
func ensure(geom Geometry, opts Options, hooks walkHooks) error {
	running := runningStamp()
	pairCall := opts.Pair != ""

	if !pairCall {
		if _, current := readCurrentStamp(geom.StampPath(), running.BuildKey); current && !repoDefaultAbsent(geom.BoardDir) {
			return nil
		}
	}

	wait := opts.LockWait
	if wait == 0 {
		wait = DefaultLockWait
	}
	if err := os.MkdirAll(filepath.Dir(geom.LockPath()), 0o755); err != nil {
		return fmt.Errorf("hubreconcile: create lock dir: %w", err)
	}
	hubLock, acquired, err := lock.AcquireWriteLockWithin(geom.LockPath(), wait)
	if err != nil {
		return fmt.Errorf("hubreconcile: %w", err)
	}
	if !acquired {
		return &LockTimeoutError{Path: geom.LockPath(), Wait: wait}
	}
	defer func() { _ = hubLock.Release() }()

	revision := running.Identity.Label()

	if pairCall {
		return reconcilePair(geom, filepath.Clean(opts.Pair), revision, hooks)
	}

	previous, current := readCurrentStamp(geom.StampPath(), running.BuildKey)
	if current && !repoDefaultAbsent(geom.BoardDir) {
		return nil
	}
	logger.Info("hubreconcile: reconciling hub config for a new build", "previous_revision", previous.Revision, "running_revision", running.Revision)

	skipped, err := walkHub(geom, revision, hooks)
	if err != nil {
		return err
	}
	if skipped {
		logger.Info("hubreconcile: the hub-wide config was not committed and pushed or a worktree is mid-merge, so the build stamp stays stale and the next start verb retries")
		return nil
	}
	return writeStamp(geom.StampPath(), running)
}

// repoDefaultAbsent reports whether boardDir lacks the base-sparse marker or a per-worktree module's repo default.
// A true result marks the build stamp stale, so the next start verb reseeds a default deleted after a walk.
// A board not yet stripped to sparse base files, or whose marker was removed, is walked again.
func repoDefaultAbsent(boardDir string) bool {
	if !fileExists(configengine.BaseSparseMarker(boardDir)) {
		return true
	}
	for _, module := range configreg.Modules() {
		if module.HubWide {
			continue
		}
		if _, err := os.Stat(configengine.ConfigFile(boardDir, module.Name)); errors.Is(err, fs.ErrNotExist) {
			return true
		}
	}
	return false
}

// readCurrentStamp returns the stamp at path and whether it names key.
// An unreadable or unparseable stamp is logged and counts as stale.
func readCurrentStamp(path, key string) (s stamp, current bool) {
	s, found, err := readStamp(path)
	if err != nil {
		logger.Warn("hubreconcile: ignoring unreadable build stamp", "path", path, "error", err)
		return stamp{}, false
	}
	return s, found && s.BuildKey == key
}

// walkHub reconciles the hub-wide config, the prime and then every pair, re-listing until no unwalked pair remains.
// It reports whether the hub-wide write was skipped because the board could not be brought up to date,
// or any worktree was skipped as mid-merge.
func walkHub(geom Geometry, revision string, hooks walkHooks) (skipped bool, err error) {
	worktrees, err := fabricengine.CodeWorktrees(geom.WorktreePath)
	if err != nil {
		return false, fmt.Errorf("hubreconcile: list code worktrees: %w", err)
	}
	if hooks.afterEnumerate != nil {
		hooks.afterEnumerate()
	}
	prime, found := findPrime(worktrees)
	if !found {
		return false, fmt.Errorf("hubreconcile: no main code worktree among the hub's worktrees listed from %s", geom.WorktreePath)
	}

	hubWideSkipped, err := reconcileHubWide(geom.BoardDir, prime, fabricengine.BinaryLabel(), hooks)
	if err != nil {
		return false, err
	}
	walked := map[string]bool{prime.Path: true}
	primeSkipped, err := reconcileWorktree(geom.BoardDir, prime, revision, hooks)
	if err != nil {
		return false, err
	}
	skipped = hubWideSkipped || primeSkipped

	for {
		worktrees, err := fabricengine.CodeWorktrees(geom.WorktreePath)
		if err != nil {
			return false, fmt.Errorf("hubreconcile: list code worktrees: %w", err)
		}
		if hooks.afterEnumerate != nil {
			hooks.afterEnumerate()
		}
		foundNew := false
		for _, w := range worktrees {
			if walked[w.Path] {
				continue
			}
			foundNew = true
			walked[w.Path] = true
			pairSkipped, err := reconcileWorktree(geom.BoardDir, w, revision, hooks)
			if err != nil {
				return false, err
			}
			skipped = skipped || pairSkipped
		}
		if !foundNew {
			return skipped, nil
		}
	}
}

// reconcilePair reconciles the one pair whose root is pairPath, whatever the stamp says.
func reconcilePair(geom Geometry, pairPath, revision string, hooks walkHooks) error {
	worktrees, err := fabricengine.CodeWorktrees(geom.WorktreePath)
	if err != nil {
		return fmt.Errorf("hubreconcile: list code worktrees: %w", err)
	}
	for _, w := range worktrees {
		if w.Path == pairPath {
			_, err := reconcileWorktree(geom.BoardDir, w, revision, hooks)
			return err
		}
	}
	return fmt.Errorf("hubreconcile: %s is not one of the hub's code worktrees", pairPath)
}

// findPrime returns the main code worktree of a listing.
func findPrime(worktrees []fabricengine.CodeWorktree) (fabricengine.CodeWorktree, bool) {
	for _, w := range worktrees {
		if w.Main {
			return w, true
		}
	}
	return fabricengine.CodeWorktree{}, false
}

// spawnBoardSync starts the detached board sync of a board dir.
// It is a variable so a test can record the call.
var spawnBoardSync = boardengine.SpawnSync

// reconcileHubWide reconciles the board's hub-wide config, seeded from the prime's anchor, and commits what it wrote.
// While the base-sparse marker is absent, the pass strips each present base file once.
// The marker it then writes joins the commit.
// A seed-only module's template base that the pass wrote joins the commit beside its file.
// A registered reinterpretation whose board marker is absent is judged against the prime and applied.
// Its marker joins the commit.
// The heddle overview is rendered afresh from what heddle holds, and joins the commit when its bytes changed.
// The board is pulled first: when it cannot be brought up to date, nothing is written and skipped is true.
// A reconcile or commit failure restores the board's config files and the overview to their prior bytes.
// It also removes a marker or template base the pass wrote.
// Once the commit lands, the hub's orch is mailed what the reinterpretations rewrote or flagged.
// It is also mailed what the seed-only files took from their template or kept against it.
// The detached board sync runs whether or not this run committed.
// So a hub-wide commit an earlier run left unpushed is pushed too.
// The walk never waits on that push.
// A refusal of it reaches the orch as a failed-push record; a spawn failure is only logged.
func reconcileHubWide(boardDir string, prime fabricengine.CodeWorktree, label string, hooks walkHooks) (skipped bool, err error) {
	bolt := fabricengine.NewBolt(boardDir)
	var prior map[string][]byte
	var notices, followed []string
	write := fabricengine.BoltWrite{
		Message: "lym: reconcile hub-wide config for build " + label,
		Write: func() ([]string, error) {
			var err error
			prior, err = snapshotConfig(boardDir)
			if err != nil {
				return nil, err
			}
			if err := snapshotFile(prior, filepath.Join(boardDir, heddleengine.OverviewFileName)); err != nil {
				return nil, err
			}
			markedBefore := fileExists(configengine.BaseSparseMarker(boardDir))
			results, err := configsync.ReconcileHubWideAt(boardDir, prime.Anchor, configsync.HubWideOptions{Apply: true, SeedDefaults: true, StripBase: true})
			if err != nil {
				return nil, err
			}
			var written []string
			followed = nil
			for _, result := range results {
				if len(result.Unsettled) > 0 {
					logger.Warn("hubreconcile: a seed-only config file holds a pair to settle, and reconcile never changes it", "module", result.Module, "unsettled", result.Unsettled,
						"way_forward", fmt.Sprintf("in \"lym config %s\" from the hub's prime worktree, keep one block of each pair that differs", result.Module))
				}
				followed = append(followed, followedLines(result)...)
				files := result.ChangedFilesRel()
				if len(files) == 0 {
					continue
				}
				logResult(boardDir, result)
				written = append(written, files...)
			}
			if !markedBefore && fileExists(configengine.BaseSparseMarker(boardDir)) {
				written = append(written, configengine.BaseSparseMarkerRel())
			}
			reinterpreted, lines, err := applyReinterpretations(boardDir, prime.Path, prime.Anchor)
			if err != nil {
				return nil, err
			}
			for _, rel := range reinterpreted {
				if !slices.Contains(written, rel) {
					written = append(written, rel)
				}
			}
			notices = lines
			if renderHeddleOverview(boardDir) {
				written = append(written, heddleengine.OverviewFileName)
			}
			if hooks.afterHubWideWrite != nil {
				if err := hooks.afterHubWideWrite(); err != nil {
					return nil, err
				}
			}
			return written, nil
		},
	}
	res, err := bolt.PullThenCommitWritten([]fabricengine.BoltWrite{write}, fabricengine.NewMutations(filepath.Dir(boardDir)))
	if err != nil {
		return false, failRestoringConfig(prior, boardErrorFor(boardDir, err))
	}
	if res.Skipped != "" {
		return true, nil
	}
	notifyReinterpretations(boardDir, label, notices)
	notifyTemplateFollowed(boardDir, label, followed)
	if err := spawnBoardSync(boardDir); err != nil {
		logger.Warn("hubreconcile: the detached board sync did not start, so the board's commits wait for the board's next push", "board", boardDir, "committed", res.Committed, "error", err)
	}
	return false, nil
}

// renderHeddleOverview renders the heddle overview in boardDir afresh and reports whether its bytes changed.
// The shipped entries it lists change with a lym build while heddle.json stays as it is,
// so the walk a new build runs is what renews it.
// A failed render is logged and leaves the file as it was,
// so a heddle store that does not load never blocks the config reconcile.
func renderHeddleOverview(boardDir string) bool {
	changed, err := heddleengine.RenderOverview(boardDir)
	if err != nil {
		logger.Warn("hubreconcile: the heddle overview was not rendered and stays as it was", "board", boardDir, "file", heddleengine.OverviewFileName, "error", err,
			"way_forward", "fix the cause the error names; the next `lym heddle set` or `lym heddle remove` renders it")
		return false
	}
	return changed
}

// reconcileWorktree reconciles and commits one code worktree's config.
// It reports skipped when a merge in progress keeps the worktree from being written or committed.
// A skipped worktree leaves the build stamp stale.
// A pair that is incomplete or has been removed is passed over without being reported skipped.
// A failure after the write restores the config files to their prior bytes, so nothing written is left uncommitted;
// a commit that landed before a later step of it failed keeps them.
func reconcileWorktree(boardDir string, w fabricengine.CodeWorktree, revision string, hooks walkHooks) (skipped bool, err error) {
	if !w.Main {
		complete, reason, err := fabricengine.PairCompleteAt(w.Path)
		if err != nil {
			return passOverOrFail(w.Path, err)
		}
		if !complete {
			logger.Info("hubreconcile: skipping an incomplete pair", "worktree", w.Path, "reason", reason)
			return false, nil
		}
	}

	f, err := fabricengine.OpenCodeWorktree(w.Path)
	if err != nil {
		return passOverOrFail(w.Path, err)
	}
	blocked, err := f.MergeBlocked()
	if err != nil {
		return false, &WorktreeError{Worktree: w.Path, Err: err}
	}
	if blocked {
		logger.Info("hubreconcile: skipping a worktree that is mid-merge", "worktree", w.Path)
		return true, nil
	}

	prior, err := snapshotConfig(w.Anchor)
	if err != nil {
		return false, &WorktreeError{Worktree: w.Path, Err: err}
	}
	markedBefore := fileExists(configengine.SparseMarker(w.Anchor))
	results, err := configsync.ReconcileAll(w.Anchor, boardDir, configsync.Options{Apply: true, Migrate: true})
	if err != nil {
		if _, statErr := os.Stat(w.Path); errors.Is(statErr, fs.ErrNotExist) {
			logger.Info("hubreconcile: skipping a worktree removed during the walk", "worktree", w.Path)
			return false, nil
		}
		return false, failRestoringConfig(prior, worktreeErrorFor(w.Path, err))
	}

	anchorRel, err := filepath.Rel(w.Path, w.Anchor)
	if err != nil {
		return false, failRestoringConfig(prior, &WorktreeError{Worktree: w.Path, Err: err})
	}
	var files []string
	for _, result := range results {
		if !result.Applied {
			continue
		}
		logResult(w.Path, result)
		if result.Layer != configengine.LayerWorktree {
			continue
		}
		files = append(files, filepath.Join(anchorRel, configengine.ConfigFileRel(result.Module)))
		for _, legacy := range result.MigratedFrom {
			files = append(files, filepath.Join(anchorRel, configengine.ConfigFileRel(legacy)))
		}
	}
	if !markedBefore && fileExists(configengine.SparseMarker(w.Anchor)) {
		files = append(files, filepath.Join(anchorRel, configengine.SparseMarkerRel()))
	}

	if hooks.beforeCommit != nil {
		hooks.beforeCommit(w.Path)
	}
	if len(files) == 0 {
		return false, nil
	}
	res, err := f.Commit(files, "lym: reconcile config for build "+revision, nil, fabricengine.SyncOptions{})
	if err != nil {
		// A commit that landed before a later step failed already holds the written bytes.
		// A restore would dirty the tree against that commit.
		if res.Committed() {
			return false, &WorktreeError{Worktree: w.Path, Err: err}
		}
		var mergeInProgress *fabricengine.ErrMergeInProgress
		var foreignMergeState *fabricengine.ErrForeignMergeState
		if errors.As(err, &mergeInProgress) || errors.As(err, &foreignMergeState) {
			if restoreErr := restoreConfig(prior); restoreErr != nil {
				return false, &WorktreeError{Worktree: w.Path, Err: restoreErr}
			}
			logger.Info("hubreconcile: a merge began before the commit, so the config was restored", "worktree", w.Path)
			return true, nil
		}
		return false, failRestoringConfig(prior, &WorktreeError{Worktree: w.Path, Err: err})
	}
	return false, nil
}

// failRestoringConfig puts a snapshot back after a failure that followed the config write, and returns failure.
// A written file left uncommitted would match its template on the retry, so the retry would never commit it.
// A failed restore is joined into failure's cause.
func failRestoringConfig(prior map[string][]byte, failure *WorktreeError) *WorktreeError {
	if err := restoreConfig(prior); err != nil {
		failure.Err = errors.Join(failure.Err, fmt.Errorf("restore config: %w", err))
	}
	return failure
}

// passOverOrFail maps an error from opening a worktree.
// A path that no longer exists is passed over; any other error fails the walk.
func passOverOrFail(worktreePath string, err error) (skipped bool, failure error) {
	var missing *fabricengine.ErrMissingPath
	if errors.As(err, &missing) {
		logger.Info("hubreconcile: skipping a worktree that no longer exists", "worktree", worktreePath)
		return false, nil
	}
	return false, &WorktreeError{Worktree: worktreePath, Err: err}
}

// worktreeErrorFor wraps a reconcile failure, taking the file and module from a *configsync.FileError.
// A template base is not a file the module's editor repairs.
// A failure naming one carries no module, so its way forward is the file alone.
func worktreeErrorFor(worktree string, err error) *WorktreeError {
	var fileErr *configsync.FileError
	if errors.As(err, &fileErr) {
		module := fileErr.Module
		if configengine.IsTemplateBaseFile(fileErr.Path) {
			module = ""
		}
		return &WorktreeError{Worktree: worktree, File: fileErr.Path, Module: module, Err: fileErr.Err}
	}
	return &WorktreeError{Worktree: worktree, Err: err}
}

// boardErrorFor wraps a failure at the board dir, and marks the error so its way forward names the board.
func boardErrorFor(boardDir string, err error) *WorktreeError {
	failure := worktreeErrorFor(boardDir, err)
	failure.Board = true
	return failure
}

// followedLines renders one line per change a board result carried from the template into a present seed-only file.
// Each operator value kept against the template gets a line too, naming the module's file.
// Any other result renders none.
func followedLines(result configsync.Result) []string {
	module, ok := configreg.Lookup(result.Module)
	if !ok || !module.SeedOnly || result.Seed != configsync.SeedHub {
		return nil
	}
	file := result.Module + ".yaml"
	var lines []string
	for _, path := range result.Added {
		lines = append(lines, file+" added "+path)
	}
	for _, change := range result.Updated {
		lines = append(lines, file+" updated "+change)
	}
	for _, path := range result.Removed {
		lines = append(lines, file+" removed "+path)
	}
	for _, kept := range result.Differs {
		lines = append(lines, file+" kept the hub's "+kept)
	}
	return lines
}

// notifyTemplateFollowed logs what the walk carried into the hub's seed-only files and kept against the template.
// It mails the same, as one line, to the hub's orch through the installed notifier.
// With no lines, it sends nothing.
func notifyTemplateFollowed(boardDir, label string, lines []string) {
	if len(lines) == 0 {
		return
	}
	line := fmt.Sprintf("%s lym build %s carried template changes into the hub's operator-owned config files, keeping every value the operator set: %s; each kept value is the operator's to settle with lym config <module>", noticePrefix, label, strings.Join(lines, "; "))
	logger.Info("hubreconcile: a seed-only config file followed its template", "board", boardDir, "lines", lines)
	mailOrch(boardDir, line)
}

// logResult logs one changed module.
func logResult(worktree string, result configsync.Result) {
	logger.Info("hubreconcile: config changed", "worktree", worktree, "module", result.Module,
		"added", result.Added, "removed", result.Removed, "updated", result.Updated, "differs", result.Differs, "templateBase", result.WroteTemplateBase, "migrated", result.Migrated,
		"stripped", result.Stripped, "inherited", result.Inherited, "preserved", result.Preserved,
		"moved", result.Moved, "keptTarget", result.KeptTarget, "keptExtra", result.KeptExtra, "dropped", result.Dropped, "overwritten", result.Overwritten, "copied", result.Copied)
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// snapshotConfig reads every file that a reconcile under anchor may write, and maps an absent one to nil.
// It covers every registry module's config and local override file and each seed-only module's template base.
// It also covers the sparse, base-sparse and reinterpretation markers under anchor.
// Every file in the config dir is covered too, so a legacy file that a migration removes is restored.
// Any read or list failure other than absence is returned naming the path.
// A snapshot missing a file could not restore it.
func snapshotConfig(anchor string) (map[string][]byte, error) {
	prior := make(map[string][]byte)
	configDir := configengine.ConfigDir(anchor)
	entries, err := os.ReadDir(configDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("snapshot config dir %s: %w", configDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(configDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", path, err)
		}
		prior[path] = data
	}
	// The markers are snapshotted too, so a restore removes one a failed walk wrote.
	// The worktree or board then stays unmarked for the next walk.
	paths := []string{configengine.SparseMarker(anchor), configengine.BaseSparseMarker(anchor)}
	for _, module := range configreg.Modules() {
		paths = append(paths, configengine.ConfigFile(anchor, module.Name), configengine.LocalFile(anchor, module.Name))
		if module.SeedOnly {
			paths = append(paths, configengine.TemplateBaseFile(anchor, module.Name))
		}
		for _, change := range module.Reinterpretations {
			paths = append(paths, configengine.ReinterpretedMarker(anchor, change.ID))
		}
	}
	for _, path := range paths {
		if err := snapshotFile(prior, path); err != nil {
			return nil, err
		}
	}
	return prior, nil
}

// snapshotFile records in prior the bytes of the file at path, or nil when it is absent.
// Any read failure other than absence is returned naming the path.
func snapshotFile(prior map[string][]byte, path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		prior[path] = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("snapshot %s: %w", path, err)
	}
	prior[path] = data
	return nil
}

// restoreConfig writes every file of a snapshot back to its prior bytes, removing the files it recorded as absent.
// It attempts every file and returns the failures joined, so one failed write does not skip the rest.
func restoreConfig(prior map[string][]byte) error {
	var errs []error
	for path, data := range prior {
		if data == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		if err := fsx.AtomicWriteBytes(path, data); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
