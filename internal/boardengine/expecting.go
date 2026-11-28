// expecting.go makes a write conditional on the entry state its caller read.
// A caller that showed an entry to someone sends that entry's revision back with the write,
// and the board's write section refuses the write when the entry has moved on since.

package boardengine

import (
	"fmt"
	"slices"
)

// StaleEntryError is the refusal of a write whose expected entry state no longer holds.
// Nothing was written.
type StaleEntryError struct {
	// Slug names the entry the write expected something of.
	Slug string
	// Expected is the revision the write expected, empty for a write that expected no such entry.
	Expected string
	// Removed reports that the expected entry is gone.
	Removed bool
	// Fresh is the entry as the board holds it now, nil when it is gone or has no entry projection.
	Fresh *ViewEntry
}

// Error implements error, naming the entry and the way forward.
func (e *StaleEntryError) Error() string {
	switch {
	case e.Removed:
		return fmt.Sprintf("entry %q was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now", e.Slug)
	case e.Expected == "":
		return fmt.Sprintf("entry %q already exists, so nothing was written: way forward: pick another slug, or edit the existing entry", e.Slug)
	}
	return fmt.Sprintf("entry %q changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again", e.Slug)
}

// Expecting returns a copy of the board whose writes go through only while the entry slug names is at revision.
// An empty revision means the write expects no entry of that slug, as a create does.
// A write through the copy that finds otherwise returns a *StaleEntryError and writes nothing.
// Only the named entry is checked, and the receiver is unchanged.
func (b *Board) Expecting(slug, revision string) *Board {
	expecting := *b
	expecting.expected = &struct{ slug, revision string }{slug: slug, revision: revision}
	return &expecting
}

// checkExpected refuses with a *StaleEntryError when the loaded entries do not hold what the board expects.
// A board that expects nothing passes.
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
		return &StaleEntryError{Slug: slug, Expected: revision, Removed: true}
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

	view, err := buildView(loaded, withMissingRecordsUndiscussed(b.boardPath, loaded), primeOwner, b.out, b.now())
	if err != nil {
		return err
	}
	refusal := &StaleEntryError{Slug: slug, Expected: revision}
	if i := slices.IndexFunc(view.Entries, func(e ViewEntry) bool { return e.Slug == slug }); i >= 0 {
		refusal.Fresh = &view.Entries[i]
	}
	return refusal
}
