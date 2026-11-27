// expect.go lets a write expect the revision of one entry, and refuses the write when the stored entry no longer has it.

package boardengine

import (
	"fmt"
	"slices"
)

// expectedRevision is the entry a write expects: a slug and the revision its entry must have, "" for no entry at all.
type expectedRevision struct {
	slug     string
	revision string
}

// StaleEntryError refuses a write whose entry is not the one the writer expected.
// Nothing of the refused write is saved, rendered or synced.
type StaleEntryError struct {
	Slug string
	// Create is true when the writer expected no entry of the slug and one exists.
	Create bool
	// Removed is true when the writer expected an entry and the slug has none.
	Removed bool
	// Fresh is the stored entry as the view projection shows it, nil when there is no such task or note.
	Fresh *ViewEntry
}

// Error returns the refusal with the way forward.
func (e *StaleEntryError) Error() string {
	switch {
	case e.Create:
		return fmt.Sprintf("entry %q already exists, so nothing was written: way forward: pick another slug, or edit the existing entry", e.Slug)
	case e.Removed:
		return fmt.Sprintf("entry %q was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now", e.Slug)
	}
	return fmt.Sprintf("entry %q changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again", e.Slug)
}

// Expecting returns a copy of the board whose writes go through only while the entry slug has revision.
// An empty revision expects that no entry of the slug exists, as a create does.
// Any other write is refused with a *StaleEntryError.
// Only the named entry is judged, never an entry the write changes as a side effect.
// The receiver is unchanged.
func (b *Board) Expecting(slug, revision string) *Board {
	expecting := *b
	expecting.expected = &expectedRevision{slug: slug, revision: revision}
	return &expecting
}

// checkExpected judges the expected revision against store, the entries as loaded and migrated under the write lock.
func (b *Board) checkExpected(store *Store) error {
	if b.expected == nil {
		return nil
	}
	slug, revision := b.expected.slug, b.expected.revision
	task, found := store.GetTask(slug)
	if !found {
		if revision == "" {
			return nil
		}
		return &StaleEntryError{Slug: slug, Removed: true}
	}
	current, err := EntryRevision(task)
	if err != nil {
		return err
	}
	if current == revision {
		return nil
	}

	tasks := store.Tasks()
	view, err := buildView(tasks, withMissingRecordsUndiscussed(b.boardPath, tasks), store.primeOwner, b.out, b.now())
	if err != nil {
		return err
	}
	stale := &StaleEntryError{Slug: slug, Create: revision == ""}
	if index := slices.IndexFunc(view.Entries, func(e ViewEntry) bool { return e.Slug == slug }); index >= 0 {
		stale.Fresh = &view.Entries[index]
	}
	return stale
}
