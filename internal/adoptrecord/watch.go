// watch.go holds the under-adopt accessor: the read-only judgment of whether an adopt in flight covers a worktree.

package adoptrecord

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Watch reads whether an adopt in flight on this machine covers a worktree of its hub.
// A worker that reads its worktree owned by another machine asks it,
// so a takeover running here neither stops that worker nor makes it hand the worktree over.
// It never writes and takes no lock.
// It is not safe for concurrent use.
type Watch struct {
	hubPath  string
	owners   func() (boardengine.Owners, error)
	parentOf func(slug string) (string, error)
	failures logger.Episode
}

// NewWatch returns the accessor over the adopt record of the hub at hubPath.
// owners reads the hub's local board, and parentOf names the slug an entry was forked from, empty for the prime.
func NewWatch(hubPath string, owners func() (boardengine.Owners, error), parentOf func(slug string) (string, error)) *Watch {
	return &Watch{hubPath: hubPath, owners: owners, parentOf: parentOf}
}

// Covers reports whether an adopt in flight covers the worktree named slug, empty for the prime.
// The worktree's subject is the entry whose owner it reads:
// the slug itself for a beam project with its own owner recorded or one the record lists.
// Otherwise it is its parent's subject, and `root` above the top.
// So a beam with no owner of its own is covered by the adopt that lists it.
// Failing that, the adopt of the subject above it covers it, `root` included.
// The record must be unfinished, list that subject and record no owner write of it that stood.
// The record is read before the board, so a hub with no unfinished record reads no board.
// A record or board that does not read counts as no adopt, logged once per episode.
func (w *Watch) Covers(slug string) bool {
	covered, err := w.covers(slug)
	if err != nil {
		w.failures.Fail("adoptrecord: the adopt record or the owners it is judged against do not read; they count as no adopt", "hub", w.hubPath, "error", err)
		return false
	}
	w.failures.Recover("adoptrecord: the adopt record and the owners read again", "hub", w.hubPath)
	return covered
}

// covers reads the record and, when it is unfinished, the owners, and judges slug's subject against the record.
func (w *Watch) covers(slug string) (bool, error) {
	record, found, err := Read(w.hubPath)
	if err != nil || !found || !record.Unfinished() {
		return false, err
	}
	owners, err := w.owners()
	if err != nil {
		return false, fmt.Errorf("read the board's owners: %w", err)
	}
	listed := func(subject string) bool { return slices.Contains(record.Subjects, subject) }
	subject, err := ownerSubject(owners, slug, listed, w.parentOf)
	if err != nil {
		return false, err
	}
	return record.Covers(subject), nil
}

// ownerSubject returns the subject whose owner the worktree slug reads, `root` for the prime's empty slug.
// It is the slug itself for an entry the board does not hold.
// It is also the slug itself for one the adopt record lists and for a beam project with its own owner recorded.
// Otherwise it is the subject of the entry's parent, `root` above the top.
func ownerSubject(
	owners boardengine.Owners, slug string, listed func(subject string) bool, parentOf func(slug string) (string, error),
) (string, error) {
	if slug == "" {
		return boardengine.RootSubject, nil
	}
	entry, held := owners.Entries[slug]
	if !held || listed(slug) || (entry.IsProject && entry.Beam && entry.Owner != "") {
		return slug, nil
	}
	parent, err := parentOf(slug)
	if err != nil {
		return "", fmt.Errorf("resolve the parent of %q: %w", slug, err)
	}
	return ownerSubject(owners, parent, listed, parentOf)
}
