// expect.go implements the entry revision check a page's write carries into the board's write section.
// Board.Expecting makes a board that checks one entry's revision, and StaleEntryError is the refusal.

package boardengine

import (
	"fmt"
	"slices"
)

// expectedRevision is the revision a write expects of one entry; an empty revision expects the entry to be absent.
type expectedRevision struct {
	slug     string
	revision string
}

// StaleEntryError refuses a write that expected another state of an entry than the board holds.
// Nothing was written, saved, rendered or synced.
type StaleEntryError struct {
	Slug string
	// Fresh is the entry as the board holds it now, nil when the board holds no task or note of that slug.
	Fresh   *ViewEntry
	message string
}

// Error returns the refusal's message, which names the entry and the way forward.
func (e *StaleEntryError) Error() string { return e.message }

// Expecting returns a copy of the board whose writes first check, under the write lock, the revision of the entry slug.
// An empty revision expects no entry of that slug, a create.
// The receiver is unchanged, so a board that never calls Expecting writes as it always did.
func (b *Board) Expecting(slug, revision string) *Board {
	expecting := *b
	expecting.expected = &expectedRevision{slug: slug, revision: revision}
	return &expecting
}

// checkExpected refuses with a *StaleEntryError when the loaded entries do not hold the entry b expects.
// loaded is the entries as the write section loaded them, so the revision is the one Board.View gives.
// A board with no expectation passes.
func (b *Board) checkExpected(loaded []Task, primeOwner string) error {
	if b.expected == nil {
		return nil
	}
	slug, revision := b.expected.slug, b.expected.revision
	index := slices.IndexFunc(loaded, func(t Task) bool { return t.Slug == slug })
	if index < 0 {
		if revision == "" {
			return nil
		}
		return &StaleEntryError{Slug: slug, message: fmt.Sprintf("entry %q was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now", slug)}
	}
	if revision != "" && revision == EntryRevision(loaded[index]) {
		return nil
	}
	// A project is no view entry, so a stale refusal for it would carry no fresh entry and read as a removal.
	if revision == "" && loaded[index].Kind == KindProject {
		return fmt.Errorf("slug %q is taken by a project, so nothing was written: way forward: pick another slug", slug)
	}

	view, err := buildView(loaded, withMissingRecordsUndiscussed(b.boardPath, loaded), primeOwner, b.out, b.now())
	if err != nil {
		return fmt.Errorf("project entry %q for the stale refusal: %w", slug, err)
	}
	stale := &StaleEntryError{Slug: slug}
	if entry := slices.IndexFunc(view.Entries, func(e ViewEntry) bool { return e.Slug == slug }); entry >= 0 {
		stale.Fresh = &view.Entries[entry]
	}
	if revision == "" {
		stale.message = fmt.Sprintf("entry %q already exists, so nothing was written: way forward: pick another slug, or edit the existing entry", slug)
	} else {
		stale.message = fmt.Sprintf("entry %q changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again", slug)
	}
	return stale
}
