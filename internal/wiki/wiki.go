// wiki.go — the Wiki facade, the only entry point the CLI uses.
//
// writeOp sequences a write: lock → load → mutate → render → write files →
// save, then launches a detached background sync (see sync.go) to back the
// change up to the remote. It never waits on git. Every mutating method
// delegates to it; read methods (Get/List) bypass it and load straight from disk.

package wiki

import (
	"fmt"
	"os"
	"path/filepath"
)

// Wiki is the high-level facade over a wiki directory.
// Every mutating method acquires an exclusive file lock, mutates the store, and
// renders output files; the slow remote backup (commit + push) is handed off to
// a detached sync process so the write returns immediately.
type Wiki struct {
	wikiPath string
}

// New returns a Wiki operating on wikiPath.
func New(wikiPath string) *Wiki {
	return &Wiki{
		wikiPath: wikiPath,
	}
}

// writeOp runs the locked, file-only write sequence: lock → load → mutate →
// render → write files → save. The remote backup is not done here; on success it
// launches a detached `mhgo wiki sync` (unless WIKI_SKIP_GIT=1) and returns
// without waiting. The second argument is ignored — the commit message is fixed
// in the pusher (batched "wiki sync" commits), not per-write.
func (w *Wiki) writeOp(mutate func(*Store) (any, error), _ string) (any, error) {
	// (1) Acquire write lock
	lock, err := AcquireWriteLock(filepath.Join(w.wikiPath, writeLockFile))
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()

	// (2) Load store
	store := NewStore(filepath.Join(w.wikiPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	// (3) Call mutate
	result, err := mutate(store)
	if err != nil {
		return nil, err
	}

	// (4) Render
	renderMap, err := Render(store.Tasks())
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	// (5) Write render outputs
	for relPath, content := range renderMap {
		if err := AtomicWrite(w.wikiPath, relPath, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	// (6) Delete orphan proposal-*.md files
	existingProposals, err := filepath.Glob(filepath.Join(w.wikiPath, "proposal-*.md"))
	if err == nil {
		for _, existingPath := range existingProposals {
			relPath := filepath.Base(existingPath)
			if _, exists := renderMap[relPath]; !exists {
				os.Remove(existingPath)
			}
		}
	}

	// (7) Save store
	if err := store.Save(w.wikiPath, "tasks.json"); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}

	// (8) Hand the remote backup to a detached sync process and return. The data
	// is already durable on disk; a failed spawn just defers backup to the next
	// write, since git push is cumulative.
	if os.Getenv("WIKI_SKIP_GIT") != "1" {
		_ = spawnSync(w.wikiPath)
	}

	return result, nil
}

func (w *Wiki) UpsertTask(fields map[string]any) (Task, error) {
	// Extract slug for message
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}
	slugStr, ok := slugVal.(string)
	if !ok {
		slugStr = fmt.Sprintf("%v", slugVal)
	}

	result, err := w.writeOp(func(s *Store) (any, error) {
		return s.UpsertTask(fields)
	}, slugStr)

	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (w *Wiki) SetPhase(idOrSlug any, phase *string) error {
	slugForMsg := fmt.Sprintf("%v", idOrSlug)
	_, err := w.writeOp(func(s *Store) (any, error) {
		return nil, s.SetPhase(idOrSlug, phase)
	}, slugForMsg)
	return err
}

func (w *Wiki) RemoveTask(idOrSlug any) error {
	slugForMsg := fmt.Sprintf("%v", idOrSlug)
	_, err := w.writeOp(func(s *Store) (any, error) {
		return nil, s.RemoveTask(idOrSlug)
	}, slugForMsg)
	return err
}

func (w *Wiki) MergeTasks(removeSlugs []string, upsert map[string]any, setPhase *[2]any) (Task, error) {
	// Extract slug for message
	slugVal, hasSlug := upsert["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}
	slugStr, ok := slugVal.(string)
	if !ok {
		slugStr = fmt.Sprintf("%v", slugVal)
	}

	result, err := w.writeOp(func(s *Store) (any, error) {
		return s.MergeTasks(removeSlugs, upsert, setPhase)
	}, slugStr)

	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (w *Wiki) SetDeps(slug string, dependsOn []string) error {
	_, err := w.writeOp(func(s *Store) (any, error) {
		return nil, s.SetDeps(slug, dependsOn)
	}, slug)
	return err
}

func (w *Wiki) UpsertTasksBatch(tasks []map[string]any) error {
	_, err := w.writeOp(func(s *Store) (any, error) {
		return nil, s.UpsertTasksBatch(tasks)
	}, "batch")
	return err
}

func (w *Wiki) Rerender() error {
	_, err := w.writeOp(func(s *Store) (any, error) {
		return nil, nil
	}, "rerender")
	return err
}

// Sync backs up pending local changes to the remote (commit + push), looping
// until nothing is left. It is what the detached `mhgo wiki sync` process runs;
// it can also be called directly to force a synchronous backup.
func (w *Wiki) Sync() error {
	return Sync(w.wikiPath)
}

func (w *Wiki) GetTask(idOrSlug any) (Task, bool, error) {
	store := NewStore(filepath.Join(w.wikiPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return Task{}, false, fmt.Errorf("load store: %w", err)
	}

	task, found := store.GetTask(idOrSlug)
	return task, found, nil
}

func (w *Wiki) ListTasksBrief() ([]BriefTask, error) {
	store := NewStore(filepath.Join(w.wikiPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksBrief(), nil
}

func (w *Wiki) ListTasksFull() ([]Task, error) {
	store := NewStore(filepath.Join(w.wikiPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	return store.ListTasksFull(), nil
}
