package wiki

import (
	"fmt"
	"os"
	"path/filepath"
)

type Wiki struct {
	wikiPath string
}

func New(wikiPath string) *Wiki {
	return &Wiki{
		wikiPath: wikiPath,
	}
}

func (w *Wiki) writeOp(mutate func(*Store) (interface{}, error), slugForMsg string) (interface{}, error) {
	// (1) Acquire write lock
	lockPath := filepath.Join(w.wikiPath, "tasks.json.lock")
	lock, err := AcquireWriteLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()

	// (2) Pull if not skipped
	if os.Getenv("WIKI_SKIP_GIT") != "1" {
		_, _ = Pull(w.wikiPath) // Ignore errors
	}

	// (3) Load store
	store := NewStore(filepath.Join(w.wikiPath, "tasks.json"))
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}

	// (4) Call mutate
	result, err := mutate(store)
	if err != nil {
		return nil, err
	}

	// (5) Render
	renderMap, err := Render(store.Tasks())
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	// (6) Write render outputs
	for relPath, content := range renderMap {
		if err := AtomicWrite(w.wikiPath, relPath, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	// (7) Delete orphan proposal-*.md files
	existingProposals, err := filepath.Glob(filepath.Join(w.wikiPath, "proposal-*.md"))
	if err == nil {
		for _, existingPath := range existingProposals {
			relPath := filepath.Base(existingPath)
			if _, exists := renderMap[relPath]; !exists {
				os.Remove(existingPath)
			}
		}
	}

	// (8) Save store
	if err := store.Save(w.wikiPath, "tasks.json"); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}

	// (9) Commit and push if not skipped
	if os.Getenv("WIKI_SKIP_GIT") != "1" {
		renderKeys := make([]string, 0, len(renderMap))
		for k := range renderMap {
			renderKeys = append(renderKeys, k)
		}
		paths := append(renderKeys, "tasks.json")

		// Add deleted orphans
		for _, existingPath := range existingProposals {
			relPath := filepath.Base(existingPath)
			if _, exists := renderMap[relPath]; !exists {
				paths = append(paths, relPath)
			}
		}

		message := "wiki: " + slugForMsg
		if err := CommitPush(w.wikiPath, paths, message); err != nil {
			return nil, fmt.Errorf("commit push: %w", err)
		}
	}

	return result, nil
}

func (w *Wiki) UpsertTask(fields map[string]interface{}) (Task, error) {
	// Extract slug for message
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}
	slugStr, ok := slugVal.(string)
	if !ok {
		slugStr = fmt.Sprintf("%v", slugVal)
	}

	result, err := w.writeOp(func(s *Store) (interface{}, error) {
		return s.UpsertTask(fields)
	}, slugStr)

	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (w *Wiki) SetPhase(idOrSlug interface{}, phase *string) error {
	slugForMsg := fmt.Sprintf("%v", idOrSlug)
	_, err := w.writeOp(func(s *Store) (interface{}, error) {
		return nil, s.SetPhase(idOrSlug, phase)
	}, slugForMsg)
	return err
}

func (w *Wiki) RemoveTask(idOrSlug interface{}) error {
	slugForMsg := fmt.Sprintf("%v", idOrSlug)
	_, err := w.writeOp(func(s *Store) (interface{}, error) {
		return nil, s.RemoveTask(idOrSlug)
	}, slugForMsg)
	return err
}

func (w *Wiki) MergeTasks(removeSlugs []string, upsert map[string]interface{}, setPhase *[2]interface{}) (Task, error) {
	// Extract slug for message
	slugVal, hasSlug := upsert["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}
	slugStr, ok := slugVal.(string)
	if !ok {
		slugStr = fmt.Sprintf("%v", slugVal)
	}

	result, err := w.writeOp(func(s *Store) (interface{}, error) {
		return s.MergeTasks(removeSlugs, upsert, setPhase)
	}, slugStr)

	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}

func (w *Wiki) SetDeps(slug string, dependsOn []string) error {
	_, err := w.writeOp(func(s *Store) (interface{}, error) {
		return nil, s.SetDeps(slug, dependsOn)
	}, slug)
	return err
}

func (w *Wiki) UpsertTasksBatch(tasks []map[string]interface{}) error {
	_, err := w.writeOp(func(s *Store) (interface{}, error) {
		return nil, s.UpsertTasksBatch(tasks)
	}, "batch")
	return err
}

func (w *Wiki) Rerender() error {
	_, err := w.writeOp(func(s *Store) (interface{}, error) {
		return nil, nil
	}, "rerender")
	return err
}

func (w *Wiki) GetTask(idOrSlug interface{}) (Task, bool, error) {
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
