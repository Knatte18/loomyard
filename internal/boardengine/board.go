// Package boardengine provides a one-shot, daemonless file-locked task tracker.
// Board is the only entry point callers use.
// Anyone adds notes, the orchestrator curates.
//
// Board holds one store, board.json, whose entries carry a tier and a type.
// A board directory that still holds the pre-upgrade tasks.json and notes.json migrates in memory on load and persists to board.json on the first write,
// and a pre-upgrade binary's later done marks are folded into the store, so a long-running old driver keeps working until the legacy files are retired.
//
// Board sequences all mutating operations with a file lock: lock → load → mutate → save board.json → render → write files.
// After each write, a detached background sync process (see sync.go) is launched to commit and push
// changes to the remote.
// The write returns immediately without waiting for the sync.
// Read methods (Get/List) bypass the lock and load directly from disk, persisting nothing.
//
// The detached sync path talks to git through fabricengine.Bolt, never hand-rolled gitexec calls,
// under board's own board.lock/board.push.lock write and push locks.
//
// Storage: board lives at weft:main, never a separate repo.
// fabricengine enforces one uniform branch-naming scheme with no exceptions: a warp branch <branch>
// is always paired with weft branch <branch>-weft.
// That means no task's weft branch can ever be named exactly the warp's own default branch (every
// paired weft branch carries the -weft suffix) — which is what makes the unsuffixed name
// permanently unclaimed by the pairing convention and reserved exclusively for board.
// This repo's earlier design considered and rejected two alternatives before landing here: a
// separate third repo for board is extra git-identity overhead for something that doesn't need its
// own identity;
// and GitHub wiki rendering (an intermediate idea) requires whichever repo holds the wiki to be
// public on GitHub's free tier — in the old separate-repo model that meant board's own repo, never
// the warp/warp repo — disqualifying for private consulting work, where the warp repo's
// wiki-serving repo would have had to go public just to render board's front page.
//
// The long-lived "prime" worktree is the only worktree with a reason to check out two weft branches
// simultaneously: its own ordinary <name>-weft companion (the standard pairing rule, unchanged),
// plus weft:main for board access — never paired with any warp branch.
// No other worktree checks out weft:main directly.
//
// Consequence for fabric: weft:main has no corresponding warp branch, so the Warp-SHA trailer /
// correspondence-index machinery (fabricengine.RecordCorrespondence / WeftSHAForWarpSHA) does not
// apply to it — board's reads/writes to weft:main are a standalone concern, not routed through
// fabric.Commit.
//
// The board is the roadmap: it carries the planned work, the next-up work and the someday work.

package boardengine

import (
	"fmt"
	"os"
	"path/filepath"

	flock "github.com/Knatte18/loomyard/internal/lock"
)

// Board is the high-level facade over a board directory.
// Every mutating method acquires an exclusive file lock, mutates the store, and renders output files;
// the remote backup (commit + push) is detached.
type Board struct {
	boardPath string
	out       Outputs
	skipGit   bool
	skipPush  bool
}

// New returns a Board operating with the given config.
func New(cfg Config) *Board {
	return &Board{
		boardPath: cfg.Path,
		out:       cfg.Outputs(),
		skipGit:   cfg.SkipGit,
		skipPush:  cfg.SkipPush,
	}
}

// noWrite is returned as a mutation's result to end boardCriticalSection without a save, render or sync;
// result is what the caller receives.
type noWrite struct{ result any }

// boardCriticalSection runs the locked write scaffolding shared by every mutating Board method:
// it acquires the lock, loads the store, calls fn to mutate it, saves board.json, runs afterSave when non-nil, renders outputs, and spawns a detached sync.
// afterSave runs under the lock after the save and before the render.
// fn returning a noWrite skips every step after the mutation.
func (b *Board) boardCriticalSection(fn func(store *Store) (any, error), afterSave func() error) (any, error) {
	// Guard before any disk mutation: a Board built without output filenames
	// (the --board-path shape, which carries only the data dir) can sync and
	// read but must not write — otherwise the store would be saved and the
	// render would then fail on an empty filename, leaving a half-applied,
	// never-synced write behind.
	if b.out.Readme == "" {
		return nil, fmt.Errorf("board outputs not configured; write commands require board config (not --board-path)")
	}

	if err := os.MkdirAll(b.boardPath, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir board: %w", err)
	}

	lock, err := flock.AcquireWriteLock(filepath.Join(b.boardPath, writeLockFile))
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()

	store := NewStore(b.boardPath)
	if err := store.Load(); err != nil {
		return nil, err
	}

	result, err := fn(store)
	if err != nil {
		return nil, err
	}
	if skipped, ok := result.(noWrite); ok {
		return skipped.result, nil
	}

	// Save before the derived .md view (JSON is authoritative).
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}

	if afterSave != nil {
		if err := afterSave(); err != nil {
			return nil, err
		}
	}

	if err := RenderToDisk(b.boardPath, store.Tasks(), b.out); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	if !b.skipGit {
		_ = spawnSync(b.boardPath)
	}

	return result, nil
}

// UpsertTask creates or updates the task identified by fields["slug"] under the write lock;
// field validation (allowlist, slug shape) is the store's job.
func (b *Board) UpsertTask(fields map[string]any) (Task, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return store.UpsertTask(fields)
	}, nil)
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

// SetStatus sets or clears the status field of the task identified by idOrSlug.
// It acquires the write lock, mutates the store, and triggers a render.
func (b *Board) SetStatus(idOrSlug any, status *string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return nil, store.SetStatus(idOrSlug, status)
	}, nil)
	return err
}

func (b *Board) RemoveTask(idOrSlug any) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return nil, store.RemoveTask(idOrSlug)
	}, nil)
	return err
}

// MergeTasks atomically removes slugs, upserts one task, and optionally applies a status update.
// setStatus carries the pre-resolved task selector and status value;
// pass nil to skip the status step.
// A status update that targets a missing task causes the entire merge to fail (boardCriticalSection discards the in-memory mutation).
func (b *Board) MergeTasks(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return store.MergeTasks(removeSlugs, upsert, setStatus)
	}, nil)
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (b *Board) SetDeps(slug string, dependsOn []string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return nil, store.SetDeps(slug, dependsOn)
	}, nil)
	return err
}

func (b *Board) UpsertTasksBatch(tasks []map[string]any) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return nil, store.UpsertTasksBatch(tasks)
	}, nil)
	return err
}

func (b *Board) Rerender() error {
	_, err := b.boardCriticalSection(func(*Store) (any, error) {
		return nil, nil
	}, nil)
	return err
}

// Sync backs up pending local changes to the remote (commit + push), looping until nothing is left.
// It is what the detached `lyx board sync` process runs;
// it can also be called directly to force a synchronous backup.
func (b *Board) Sync() error {
	return Sync(b.boardPath, b.skipGit, b.skipPush)
}

// HealthCheck verifies the board directory exists and holds a readable board.json or a readable legacy file.
// Syntactically corrupt but readable files pass the health check.
func (b *Board) HealthCheck() error {
	if _, err := os.Stat(b.boardPath); err != nil {
		return err
	}

	var firstErr error
	for _, name := range []string{boardFile, legacyTasksFile, legacyNotesFile} {
		_, err := os.ReadFile(filepath.Join(b.boardPath, name))
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// loadStore loads the one store for a read, persisting nothing.
func (b *Board) loadStore() (*Store, error) {
	store := NewStore(b.boardPath)
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (b *Board) GetTask(idOrSlug any) (Task, bool, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return Task{}, false, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return Task{}, false, err
	}

	task, found := store.GetTask(idOrSlug)
	return task, found, nil
}

func (b *Board) ListTasksBrief() ([]BriefTask, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}

	return store.ListTasksBrief(), nil
}

func (b *Board) ListTasksFull() ([]Task, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}

	return store.ListTasksFull(), nil
}

// Promote moves the entry identified by slug to a lower tier number under the write lock;
// a nil target means one tier lower.
func (b *Board) Promote(slug string, target *int) (Task, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return store.Promote(slug, target)
	}, nil)
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

// Prune removes every done entry under the write lock and returns the removed slugs;
// the render that follows drops their design docs.
func (b *Board) Prune() ([]string, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return store.Prune(), nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// Find returns the entries whose slug, title, brief or body contains text, persisting nothing.
func (b *Board) Find(text string) ([]BriefTask, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}
	return store.Find(text), nil
}

// RetireLegacy ends the pre-upgrade compatibility window under the write lock.
// It refuses when neither legacy file exists;
// otherwise the load folds the last done marks and board.json is saved with legacy_done still in place.
// The legacy files and their swap locks are then deleted, and only after that is legacy_done dropped and board.json saved again.
// A crash or a failed deletion therefore leaves the surviving legacy files beside an intact legacy_done, so a reload folds no mark twice and retire-legacy can be rerun.
func (b *Board) RetireLegacy() error {
	legacyPaths := []string{
		filepath.Join(b.boardPath, legacyTasksFile),
		filepath.Join(b.boardPath, legacyNotesFile),
	}
	var retired *Store
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		if !fileExists(legacyPaths[0]) && !fileExists(legacyPaths[1]) {
			return nil, fmt.Errorf("nothing to retire: neither %s nor %s exists in the board directory", legacyTasksFile, legacyNotesFile)
		}
		retired = store
		return nil, nil
	}, func() error {
		for _, path := range legacyPaths {
			for _, p := range []string{path, path + swapLockSuffix} {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove legacy file: %w", err)
				}
			}
		}
		retired.legacyDone = nil
		if err := retired.Save(); err != nil {
			return fmt.Errorf("save store: %w", err)
		}
		return nil
	})
	return err
}

// PromoteNote moves the entry identified by idOrSlug to tier 1 under the write lock.
// An entry already at tier 1 is returned unchanged without a write.
func (b *Board) PromoteNote(idOrSlug any) (Task, error) {
	target := MinTier
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		current, found := store.GetTask(idOrSlug)
		if !found {
			return nil, fmt.Errorf("task not found: %v", idOrSlug)
		}
		if current.Tier <= MinTier {
			return noWrite{result: current}, nil
		}
		return store.Promote(idOrSlug, &target)
	}, nil)
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}
