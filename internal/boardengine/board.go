// Package boardengine provides a one-shot, daemonless file-locked task tracker.
// Board is the only entry point callers use.
//
// Board sequences all mutating operations with a file lock: lock → load both
// stores → mutate → render → write files → save. After each write, a detached
// background sync process (see sync.go) is launched to commit and push
// changes to the remote. The write returns immediately without waiting for
// the sync. Read methods (Get/List) bypass the lock and load directly from
// disk.
//
// The detached sync path talks to git through fabricengine.CommitWeftAt and
// fabricengine.PushWeftAt, never hand-rolled gitexec calls, under board's own
// board.lock/board.push.lock write and push locks.

package boardengine

import (
	"fmt"
	"os"
	"path/filepath"

	flock "github.com/Knatte18/loomyard/internal/lock"
)

// tasksFile and notesFile name the two JSON stores a board directory holds:
// tasks.json is the canonical task store; notes.json is the parallel,
// independently-slugged staging store PromoteNote moves entries out of.
const (
	tasksFile = "tasks.json"
	notesFile = "notes.json"
)

// Board is the high-level facade over a board directory.
// Every mutating method acquires an exclusive file lock, mutates one or both
// stores, and renders output files; the slow remote backup (commit + push) is
// handed off to a detached sync process so the write returns immediately.
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

// storeTarget selects which of the two stores a writeOp-driven mutation acts
// on as its primary target; the other store is still loaded (for cross-store
// checks and rendering) but not necessarily saved.
type storeTarget int

const (
	tasksTarget storeTarget = iota
	notesTarget
)

// boardCriticalSection runs the locked, dual-store scaffolding shared by
// every mutating Board method: lock → load both stores → fn → render from
// both stores' current state → spawn detached sync. It never calls
// Store.Save itself — fn is responsible for saving whichever store(s) it
// mutated, in whatever order its own crash-safety story requires (see
// writeOp for the common single-store case and PromoteNote for the dual-save
// case). This is the concrete mechanism behind discussion.md's combined-lock
// decision: one lock/render/sync path shared by every writer, rather than two
// separate per-file locks with an ordering rule.
func (b *Board) boardCriticalSection(fn func(tasksStore, notesStore *Store) (any, error)) (any, error) {
	// Guard before any disk mutation: a Board built without output filenames
	// (the --board-path shape, which carries only the data dir) can sync and
	// read but must not write — otherwise a store would be saved and the
	// render would then fail on an empty filename, leaving a half-applied,
	// never-synced write behind.
	if b.out.Readme == "" {
		return nil, fmt.Errorf("board outputs not configured; write commands require board config (not --board-path)")
	}

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

	// (2) Load both stores — every writer needs both, either as its own
	// mutation target or as the "other" store a cross-store check reads.
	tasksStore := NewStore(filepath.Join(b.boardPath, tasksFile))
	if err := tasksStore.Load(); err != nil {
		return nil, fmt.Errorf("load tasks store: %w", err)
	}
	notesStore := NewStore(filepath.Join(b.boardPath, notesFile))
	if err := notesStore.Load(); err != nil {
		return nil, fmt.Errorf("load notes store: %w", err)
	}

	// (3) Call fn. fn saves whichever store(s) it mutated itself.
	result, err := fn(tasksStore, notesStore)
	if err != nil {
		return nil, err
	}

	// (4) Render the readable README (render.go owns all markdown output and
	// orphan cleanup; board.go only deals with the two JSON stores) from both
	// stores' current, just-saved state.
	if err := RenderToDisk(b.boardPath, tasksStore.Tasks(), notesStore.Tasks(), b.out); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	// (5) Hand the remote backup to a detached sync process and return. The data
	// is already durable on disk; a failed spawn just defers backup to the next
	// write, since git push is cumulative.
	if !b.skipGit {
		_ = spawnSync(b.boardPath)
	}

	return result, nil
}

// writeOp runs the common single-store write shape: pick target/other from
// (tasksStore, notesStore) per target, call mutate, and on success save only
// the target store. It is a thin wrapper over boardCriticalSection for the
// seven existing task-mutating methods, none of which need to save the other
// store.
func (b *Board) writeOp(target storeTarget, mutate func(target, other *Store) (any, error)) (any, error) {
	return b.boardCriticalSection(func(tasksStore, notesStore *Store) (any, error) {
		targetStore, otherStore := tasksStore, notesStore
		if target == notesTarget {
			targetStore, otherStore = notesStore, tasksStore
		}

		result, err := mutate(targetStore, otherStore)
		if err != nil {
			return nil, err
		}

		// Save the target store — tasks.json/notes.json is the source of truth,
		// persisted before the derived .md view (so a crash never leaves .md
		// ahead of the data).
		if err := targetStore.Save(); err != nil {
			return nil, fmt.Errorf("save store: %w", err)
		}

		return result, nil
	})
}

// checkCrossStoreSlugAvailable rejects an upsert whose slug already exists in
// the OTHER store: tasks.json and notes.json share one global slug
// namespace, so a slug introduced in one store must never silently shadow an
// entry already living in the other. An in-place update of a slug already
// present in target is not a new-slug introduction and passes through
// unchecked. A missing or non-string slug is left for the store-level check
// (UpsertTask/ApplyPatch's own validation) to report with a clearer message.
func checkCrossStoreSlugAvailable(fields map[string]any, target, other *Store) error {
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return nil
	}
	slugStr, ok := slugVal.(string)
	if !ok || slugStr == "" {
		return nil
	}
	if _, exists := target.slugIndex()[slugStr]; exists {
		return nil
	}
	if _, exists := other.slugIndex()[slugStr]; exists {
		return fmt.Errorf("slug %q already exists in the other store", slugStr)
	}
	return nil
}

// checkCrossStoreSlugsAvailable applies checkCrossStoreSlugAvailable to every
// entry of a batch upsert, returning the first error encountered so a batch
// with one colliding slug fails fast before any of it is applied.
func checkCrossStoreSlugsAvailable(entries []map[string]any, target, other *Store) error {
	for _, fields := range entries {
		if err := checkCrossStoreSlugAvailable(fields, target, other); err != nil {
			return err
		}
	}
	return nil
}

// UpsertTask creates or updates the task identified by fields["slug"] under the
// write lock; field validation (allowlist, slug shape) is the store's job.
func (b *Board) UpsertTask(fields map[string]any) (Task, error) {
	result, err := b.writeOp(tasksTarget, func(target, other *Store) (any, error) {
		if err := checkCrossStoreSlugAvailable(fields, target, other); err != nil {
			return nil, err
		}
		return target.UpsertTask(fields)
	})
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

// SetStatus sets or clears the status field of the task identified by idOrSlug.
// It acquires the write lock, mutates the store, and triggers a render.
func (b *Board) SetStatus(idOrSlug any, status *string) error {
	_, err := b.writeOp(tasksTarget, func(target, _ *Store) (any, error) {
		return nil, target.SetStatus(idOrSlug, status)
	})
	return err
}

func (b *Board) RemoveTask(idOrSlug any) error {
	_, err := b.writeOp(tasksTarget, func(target, _ *Store) (any, error) {
		return nil, target.RemoveTask(idOrSlug)
	})
	return err
}

// MergeTasks atomically removes slugs, upserts one task, and optionally applies a
// status update. setStatus carries the pre-resolved task selector and status value;
// pass nil to skip the status step. A status update that targets a missing task
// causes the entire merge to fail (writeOp discards the in-memory mutation).
func (b *Board) MergeTasks(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, error) {
	result, err := b.writeOp(tasksTarget, func(target, other *Store) (any, error) {
		if err := checkCrossStoreSlugAvailable(upsert, target, other); err != nil {
			return nil, err
		}
		return target.MergeTasks(removeSlugs, upsert, setStatus)
	})
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (b *Board) SetDeps(slug string, dependsOn []string) error {
	_, err := b.writeOp(tasksTarget, func(target, _ *Store) (any, error) {
		return nil, target.SetDeps(slug, dependsOn)
	})
	return err
}

func (b *Board) UpsertTasksBatch(tasks []map[string]any) error {
	_, err := b.writeOp(tasksTarget, func(target, other *Store) (any, error) {
		if err := checkCrossStoreSlugsAvailable(tasks, target, other); err != nil {
			return nil, err
		}
		return nil, target.UpsertTasksBatch(tasks)
	})
	return err
}

func (b *Board) Rerender() error {
	_, err := b.writeOp(tasksTarget, func(target, _ *Store) (any, error) {
		return nil, nil
	})
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
	tasksPath := filepath.Join(b.boardPath, tasksFile)
	_, err := os.ReadFile(tasksPath)
	return err
}

func (b *Board) GetTask(idOrSlug any) (Task, bool, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return Task{}, false, nil
	}

	store := NewStore(filepath.Join(b.boardPath, tasksFile))
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

	store := NewStore(filepath.Join(b.boardPath, tasksFile))
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

	store := NewStore(filepath.Join(b.boardPath, tasksFile))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksFull(), nil
}

// UpsertNote creates or updates the note identified by fields["slug"] under the
// write lock; field validation (allowlist, slug shape) is the store's job.
// Mirrors UpsertTask, targeting notesFile instead of tasksFile.
func (b *Board) UpsertNote(fields map[string]any) (Task, error) {
	result, err := b.writeOp(notesTarget, func(target, other *Store) (any, error) {
		if err := checkCrossStoreSlugAvailable(fields, target, other); err != nil {
			return nil, err
		}
		return target.UpsertTask(fields)
	})
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

// SetNoteStatus sets or clears the status field of the note identified by
// idOrSlug. Mirrors SetStatus, targeting notesFile instead of tasksFile.
func (b *Board) SetNoteStatus(idOrSlug any, status *string) error {
	_, err := b.writeOp(notesTarget, func(target, _ *Store) (any, error) {
		return nil, target.SetStatus(idOrSlug, status)
	})
	return err
}

// RemoveNote deletes the note by ID or slug. Mirrors RemoveTask, targeting
// notesFile instead of tasksFile.
func (b *Board) RemoveNote(idOrSlug any) error {
	_, err := b.writeOp(notesTarget, func(target, _ *Store) (any, error) {
		return nil, target.RemoveTask(idOrSlug)
	})
	return err
}

// MergeNotes atomically removes slugs, upserts one note, and optionally
// applies a status update. Mirrors MergeTasks, targeting notesFile instead of
// tasksFile.
func (b *Board) MergeNotes(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, error) {
	result, err := b.writeOp(notesTarget, func(target, other *Store) (any, error) {
		if err := checkCrossStoreSlugAvailable(upsert, target, other); err != nil {
			return nil, err
		}
		return target.MergeTasks(removeSlugs, upsert, setStatus)
	})
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

// SetNoteDeps replaces the depends_on list for a note. Mirrors SetDeps,
// targeting notesFile instead of tasksFile.
func (b *Board) SetNoteDeps(slug string, dependsOn []string) error {
	_, err := b.writeOp(notesTarget, func(target, _ *Store) (any, error) {
		return nil, target.SetDeps(slug, dependsOn)
	})
	return err
}

// UpsertNotesBatch applies multiple note upserts atomically. Mirrors
// UpsertTasksBatch, targeting notesFile instead of tasksFile.
func (b *Board) UpsertNotesBatch(notes []map[string]any) error {
	_, err := b.writeOp(notesTarget, func(target, other *Store) (any, error) {
		if err := checkCrossStoreSlugsAvailable(notes, target, other); err != nil {
			return nil, err
		}
		return nil, target.UpsertTasksBatch(notes)
	})
	return err
}

// GetNote looks up a note by integer ID or slug string. Mirrors GetTask,
// targeting notesFile instead of tasksFile.
func (b *Board) GetNote(idOrSlug any) (Task, bool, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return Task{}, false, nil
	}

	store := NewStore(filepath.Join(b.boardPath, notesFile))
	if err := store.Load(); err != nil {
		return Task{}, false, fmt.Errorf("load store: %w", err)
	}

	note, found := store.GetTask(idOrSlug)
	return note, found, nil
}

// ListNotesBrief returns all notes enriched with computed Layer/HasProposal
// fields. Mirrors ListTasksBrief, targeting notesFile instead of tasksFile.
func (b *Board) ListNotesBrief() ([]BriefTask, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store := NewStore(filepath.Join(b.boardPath, notesFile))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksBrief(), nil
}

// ListNotesFull returns a copy of the raw note list with no enrichment.
// Mirrors ListTasksFull, targeting notesFile instead of tasksFile.
func (b *Board) ListNotesFull() ([]Task, error) {
	// Short-circuit if board dir does not exist
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store := NewStore(filepath.Join(b.boardPath, notesFile))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksFull(), nil
}

// taskToUpsertFields builds an upsert field map from an existing Task's
// fields, explicitly — NOT a JSON round-trip of the whole Task, since the
// struct's id field is not in upsertAllowedKeys and would be rejected as an
// unknown field if included. Used by PromoteNote to hand a note's content to
// the tasks store's UpsertTask as a fresh upsert payload.
func taskToUpsertFields(t Task) map[string]any {
	fields := map[string]any{
		"slug":       t.Slug,
		"title":      t.Title,
		"depends_on": t.DependsOn,
		"isolated":   t.Isolated,
		"deferred":   t.Deferred,
		"brief":      t.Brief,
		"body":       t.Body,
	}
	if t.Status != nil {
		fields["status"] = *t.Status
	}
	if t.ShortName != "" {
		fields["short_name"] = t.ShortName
	}
	return fields
}

// PromoteNote moves the note identified by idOrSlug from notes.json into
// tasks.json: it upserts the note's content into the tasks store, saves it,
// then removes the entry from the notes store and saves that too. Built
// directly on boardCriticalSection (not writeOp, which only saves one store)
// because PromoteNote must save both stores, in this order.
//
// Save ordering is the crash-safety contract this method exists to uphold:
// tasksStore.Save() happens FIRST, notesStore.Save() SECOND. A crash between
// the two saves leaves the entry present in BOTH files — recoverable, never
// silently lost. A retry after such a crash is a plain idempotent call:
// notesStore.GetTask still finds the note (removal never ran), and
// tasksStore.UpsertTask finds the slug already present and takes the
// ApplyPatch in-place-update branch (not NewTask), converging to the same
// result; notesStore.RemoveTask then completes on the retry.
//
// A note whose depends_on still names a notes.json-only, not-yet-promoted
// slug is rejected by tasksStore.UpsertTask's own validateWrite
// dangling-dependency check with no new code needed here — this is intended
// behavior (promote dependencies first), not a gap.
func (b *Board) PromoteNote(idOrSlug any) (Task, error) {
	result, err := b.boardCriticalSection(func(tasksStore, notesStore *Store) (any, error) {
		note, found := notesStore.GetTask(idOrSlug)
		if !found {
			return nil, fmt.Errorf("note not found: %v", idOrSlug)
		}

		// UpsertTask is called DIRECTLY (bypassing checkCrossStoreSlugAvailable
		// and Board.UpsertTask entirely): this is the one legitimate path where
		// the slug is expected to be transiently present in both stores, both
		// on a genuine first call (it exists in notesStore and is about to be
		// added to tasksStore) and on a crash-recovery retry (it may already
		// exist in both).
		upserted, err := tasksStore.UpsertTask(taskToUpsertFields(note))
		if err != nil {
			return nil, err
		}
		if err := tasksStore.Save(); err != nil {
			return nil, fmt.Errorf("save tasks store: %w", err)
		}

		if err := notesStore.RemoveTask(note.Slug); err != nil {
			return nil, err
		}
		if err := notesStore.Save(); err != nil {
			return nil, fmt.Errorf("save notes store: %w", err)
		}

		return upserted, nil
	})
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}
