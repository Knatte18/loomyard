// expected.go holds a write to the revision of the entry its caller read, and the refusal of a stale one.

package boardengine

import (
	"fmt"
	"slices"
)

// expectedRevision is the entry state one write expects to find.
type expectedRevision struct {
	slug string
	// revision is empty when the write expects no entry of slug.
	revision string
}

// StaleEntryError refuses a write whose expected entry state no longer holds; nothing was written.
type StaleEntryError struct {
	Slug string
	// Revision is the revision the write expected, empty for a write that expected no entry.
	Revision string
	// Removed reports that the expected entry is no longer on the board.
	Removed bool
	// Fresh is the entry as the board holds it now, nil for a removed entry and for a project.
	Fresh *ViewEntry
}

// Error names what changed and the way forward.
func (e *StaleEntryError) Error() string {
	switch {
	case e.Revision == "":
		return fmt.Sprintf("entry %q already exists, so nothing was written: way forward: pick another slug, or edit the existing entry", e.Slug)
	case e.Removed:
		return fmt.Sprintf("entry %q was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now", e.Slug)
	}
	return fmt.Sprintf("entry %q changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again", e.Slug)
}

// Expecting returns a copy of the board whose writes go through only while the entry slug stands at revision.
// An empty revision expects no entry of that slug, as a create does.
// A write that finds otherwise returns a *StaleEntryError and changes nothing.
// Only the named entry is checked, not a sibling the same write changes beside it.
// The receiver is left unchanged.
func (b *Board) Expecting(slug, revision string) *Board {
	expecting := *b
	expecting.expected = &expectedRevision{slug: slug, revision: revision}
	return &expecting
}

// checkExpected judges the loaded entries against the board's expected entry state, and passes a board with none.
func (b *Board) checkExpected(store *Store) error {
	if b.expected == nil {
		return nil
	}
	slug, revision := b.expected.slug, b.expected.revision
	loaded := store.Tasks()
	index := slices.IndexFunc(loaded, func(task Task) bool { return task.Slug == slug })
	if index < 0 {
		if revision == "" {
			return nil
		}
		return &StaleEntryError{Slug: slug, Revision: revision, Removed: true}
	}
	if revision != "" {
		current, err := EntryRevision(loaded[index])
		if err != nil {
			return err
		}
		if current == revision {
			return nil
		}
	}

	view, err := buildView(loaded, withMissingRecordsUndiscussed(b.boardPath, loaded), store.primeOwner, b.out, b.now())
	if err != nil {
		return err
	}
	stale := &StaleEntryError{Slug: slug, Revision: revision}
	if i := slices.IndexFunc(view.Entries, func(entry ViewEntry) bool { return entry.Slug == slug }); i >= 0 {
		stale.Fresh = &view.Entries[i]
	}
	return stale
}
