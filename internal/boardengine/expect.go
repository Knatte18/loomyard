// expect.go holds the revision check of the board's write section:
// a write made against an entry another write has since changed, removed or created is refused.

package boardengine

import (
	"fmt"
	"slices"
)

// expectation is the state of one entry a write expects to find.
type expectation struct {
	slug string
	// revision is the entry's revision, empty when the write expects no entry of slug.
	revision string
}

// StaleEntryError is the refusal of a write made against an entry that is no longer as the writer read it.
// Nothing is written.
type StaleEntryError struct {
	Slug string
	// Revision is the revision the write expected, empty for a write that expected no entry.
	Revision string
	// Removed reports that the expected entry no longer exists.
	Removed bool
	// Fresh is the entry as it stands, nil when it was removed or is not a task or note.
	Fresh *ViewEntry
}

// Error names the entry, what happened to it and the way forward.
func (e *StaleEntryError) Error() string {
	switch {
	case e.Removed:
		return fmt.Sprintf("entry %q was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now", e.Slug)
	case e.Revision == "":
		return fmt.Sprintf("entry %q already exists, so nothing was written: way forward: pick another slug, or edit the existing entry", e.Slug)
	}
	return fmt.Sprintf("entry %q changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again", e.Slug)
}

// Expecting returns a copy of the board that checks the entry slug names under the write lock.
// Its writes are refused with a *StaleEntryError unless that entry still has the given revision.
// An empty revision expects that no entry of slug exists, as a create does.
// Only that entry is checked, not a sibling a write changes as a side effect.
func (b *Board) Expecting(slug, revision string) *Board {
	expecting := *b
	expecting.expected = &expectation{slug: slug, revision: revision}
	return &expecting
}

// checkExpected judges the loaded and migrated store against what the board expects, if anything.
func (b *Board) checkExpected(store *Store) error {
	if b.expected == nil {
		return nil
	}
	slug, revision := b.expected.slug, b.expected.revision
	current, found := store.GetTask(slug)
	if !found {
		if revision == "" {
			return nil
		}
		return &StaleEntryError{Slug: slug, Revision: revision, Removed: true}
	}
	if revision != "" {
		got, err := EntryRevision(current)
		if err != nil {
			return err
		}
		if got == revision {
			return nil
		}
	}
	tasks := store.Tasks()
	view, err := buildView(tasks, withMissingRecordsUndiscussed(b.boardPath, tasks), store.primeOwner, b.out, b.now())
	if err != nil {
		return err
	}
	stale := &StaleEntryError{Slug: slug, Revision: revision}
	if i := slices.IndexFunc(view.Entries, func(entry ViewEntry) bool { return entry.Slug == slug }); i >= 0 {
		stale.Fresh = &view.Entries[i]
	}
	return stale
}
