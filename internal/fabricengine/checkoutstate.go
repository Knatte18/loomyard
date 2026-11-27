// checkoutstate.go holds the read-only readings of the two checkouts of a pair:
// CheckoutStates reads each checkout's HEAD, changed tracked paths and untracked paths.
// RecordsPathsChangedSince lists the weft paths committed since a revision.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// CheckoutStates reads the state of the warp (code) checkout and of the weft (records) worktree paired with l.
// It reads only; a path ignored by either checkout's rules is in neither list.
func CheckoutStates(l *lymcwd.Location) (code, records fabriclayout.CheckoutState, err error) {
	code, err = readCheckoutState(l.WorktreePath())
	if err != nil {
		return fabriclayout.CheckoutState{}, fabriclayout.CheckoutState{}, err
	}
	records, err = readCheckoutState(RecordsWorktree(l))
	if err != nil {
		return fabriclayout.CheckoutState{}, fabriclayout.CheckoutState{}, err
	}
	return code, records, nil
}

// readCheckoutState reads the HEAD, changed tracked paths and untracked paths of the checkout at root.
func readCheckoutState(root string) (fabriclayout.CheckoutState, error) {
	repo := gitrepo.New(root)
	state := fabriclayout.CheckoutState{Root: root}

	head, err := repo.CurrentSHA()
	if err != nil && !errors.Is(err, gitrepo.ErrNoCommits) {
		return fabriclayout.CheckoutState{}, fmt.Errorf("fabricengine: read HEAD of %s: %w", root, err)
	}
	state.Head = head

	dirty, err := trackedDirtyPaths(root)
	if err != nil {
		return fabriclayout.CheckoutState{}, err
	}
	state.Changed = make([]string, 0, len(dirty))
	for _, path := range dirty {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fabriclayout.CheckoutState{}, fmt.Errorf("fabricengine: relate %s to %s: %w", path, root, err)
		}
		state.Changed = append(state.Changed, filepath.ToSlash(rel))
	}
	sort.Strings(state.Changed)

	untracked, err := repo.UntrackedFiles()
	if err != nil {
		return fabriclayout.CheckoutState{}, fmt.Errorf("fabricengine: list untracked paths of %s: %w", root, err)
	}
	state.Untracked = append([]string{}, untracked...)
	sort.Strings(state.Untracked)
	return state, nil
}

// RecordsPathsChangedSince returns the weft paths whose committed content differs between rev and the weft HEAD.
// The paths are slash-separated, relative to the weft worktree's root, sorted and never nil.
func RecordsPathsChangedSince(l *lymcwd.Location, rev string) ([]string, error) {
	changed, err := gitrepo.New(RecordsWorktree(l)).ChangedFilesSince(rev)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list weft paths changed since %s: %w", rev, err)
	}
	paths := append([]string{}, changed...)
	sort.Strings(paths)
	return paths, nil
}
