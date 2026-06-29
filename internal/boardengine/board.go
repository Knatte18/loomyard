// Package boardengine provides a one-shot, daemonless file-locked task tracker.
// Board is the only entry point callers use.
//
// Board sequences all mutating operations with a file lock: lock → load →
// mutate → render → write files → save. After each write, a detached background
// sync process (see sync.go) is launched to commit and push changes to the
// remote. The write returns immediately without waiting for the sync. Read
// methods (Get/List) bypass the lock and load directly from disk.

package boardengine

import (
	"fmt"
	"os"
	"path/filepath"

	flock "github.com/Knatte18/loomyard/internal/lock"
)

// Board is the high-level facade over a board directory.
// Every mutating method acquires an exclusive file lock, mutates the store, and
// renders output files; the slow remote backup (commit + push) is handed off to
// a detached sync process so the write returns immediately.
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

// writeOp runs the locked, file-only write sequence: lock → load → mutate →
// render → write files → save. The remote backup is not done here; on success it
// launches a detached `lyx board sync` (unless `b.skipGit` is set) and returns
// without waiting. The second argument is ignored — the commit message is fixed
// in the pusher (batched "board sync" commits), not per-write.
func (b *Board) writeOp(mutate func(*Store) (any, error), _ string) (any, error) {
	// (0) Ensure board directory exists before acquiring lock
	// (the lock file lives inside the board dir)
	if err := os.MkdirAll(b.boardPath, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir board: %w", err)
	}

	// (1) Acquire write lock
	lock, err := flock.AcquireWriteLock(filepath.Join(b.boardPath, writeLockFile))
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()

	// (2) Load store
	store := NewStore(filepath.Join(b.boardPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	// (3) Call mutate
	result, err := mutate(store)
	if err != nil {
		return nil, err
	}

	// (4) Save the store first — tasks.json is the source of truth, persisted
	// before the derived .md view (so a crash never leaves .md ahead of the data).
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}

	// (5) Render the readable .md files (render.go owns all markdown output and
	// orphan cleanup; board.go only deals with tasks.json).
	if err := RenderToDisk(b.boardPath, store.Tasks(), b.out); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	// (6) Hand the remote backup to a detached sync process and return. The data
	// is already durable on disk; a failed spawn just defers backup to the next
	// write, since git push is cumulative.
	if !b.skipGit {
		_ = spawnSync(b.boardPath)
	}

	return result, nil
}

func (b *Board) UpsertTask(fields map[string]any) (Task, error) {
	// Extract slug for message
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}
	slugStr, ok := slugVal.(string)
	if !ok {
		slugStr = fmt.Sprintf("%v", slugVal)
	}

	result, err := b.writeOp(func(s *Store) (any, error) {
		return s.UpsertTask(fields)
	}, slugStr)

	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

// SetStatus sets or clears the status field of the task identified by idOrSlug.
// It acquires the write lock, mutates the store, and triggers a render.
func (b *Board) SetStatus(idOrSlug any, status *string) error {
	slugForMsg := fmt.Sprintf("%v", idOrSlug)
	_, err := b.writeOp(func(s *Store) (any, error) {
		return nil, s.SetStatus(idOrSlug, status)
	}, slugForMsg)
	return err
}

func (b *Board) RemoveTask(idOrSlug any) error {
	slugForMsg := fmt.Sprintf("%v", idOrSlug)
	_, err := b.writeOp(func(s *Store) (any, error) {
		return nil, s.RemoveTask(idOrSlug)
	}, slugForMsg)
	return err
}

// MergeTasks atomically removes slugs, upserts one task, and optionally applies a
// status update. setStatus carries the pre-resolved task selector and status value;
// pass nil to skip the status step. A status update that targets a missing task
// causes the entire merge to fail (writeOp discards the in-memory mutation).
func (b *Board) MergeTasks(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, error) {
	// Extract slug for the commit message; the store validates the full field set.
	slugVal, hasSlug := upsert["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}
	slugStr, ok := slugVal.(string)
	if !ok {
		slugStr = fmt.Sprintf("%v", slugVal)
	}

	result, err := b.writeOp(func(s *Store) (any, error) {
		return s.MergeTasks(removeSlugs, upsert, setStatus)
	}, slugStr)

	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (b *Board) SetDeps(slug string, dependsOn []string) error {
	_, err := b.writeOp(func(s *Store) (any, error) {
		return nil, s.SetDeps(slug, dependsOn)
	}, slug)
	return err
}

func (b *Board) UpsertTasksBatch(tasks []map[string]any) error {
	_, err := b.writeOp(func(s *Store) (any, error) {
		return nil, s.UpsertTasksBatch(tasks)
	}, "batch")
	return err
}

func (b *Board) Rerender() error {
	_, err := b.writeOp(func(s *Store) (any, error) {
		return nil, nil
	}, "rerender")
	return err
}

// Sync backs up pending local changes to the remote (commit + push), looping
// until nothing is left. It is what the detached `lyx board sync` process runs;
// it can also be called directly to force a synchronous backup.
func (b *Board) Sync() error {
	return Sync(b.boardPath, b.skipGit, b.skipPush)
}

// HealthCheck verifies the board directory and tasks.json file exist and are readable.
// Returns nil if the board is healthy, or an error if the board dir is absent,
// tasks.json is absent/unreadable, or any other I/O error occurs.
// Syntactically corrupt but readable files pass the health check.
func (b *Board) HealthCheck() error {
	// Check if board directory exists
	if _, err := os.Stat(b.boardPath); err != nil {
		return err
	}

	// Check if tasks.json exists and is readable
	tasksPath := filepath.Join(b.boardPath, "tasks.json")
	_, err := os.ReadFile(tasksPath)
	return err
}

func (b *Board) GetTask(idOrSlug any) (Task, bool, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return Task{}, false, nil
	}

	store := NewStore(filepath.Join(b.boardPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return Task{}, false, fmt.Errorf("load store: %w", err)
	}

	task, found := store.GetTask(idOrSlug)
	return task, found, nil
}

func (b *Board) ListTasksBrief() ([]BriefTask, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store := NewStore(filepath.Join(b.boardPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksBrief(), nil
}

func (b *Board) ListTasksFull() ([]Task, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store := NewStore(filepath.Join(b.boardPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksFull(), nil
}
