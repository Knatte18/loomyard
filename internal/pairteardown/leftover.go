// leftover.go removes two kinds of pair the dobby finds left over that no run's own teardown takes:
// the local pair of a run this machine handed over, and an orphan pair no board entry or batten record names.

package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// RemoveHandedOverPair removes the local pair of slug, a run this machine handed over to the machine that owns it now.
// It ends this machine's processes for the run first, then the pair's session.
// That session stays live when the hand-over ran inside the pair.
// The removal is local: it takes no origin step and writes no board or batten record, so a hold does not stop it.
// The hand-over archived the pair's tips, so the removal answers dirtiness with force.
func RemoveHandedOverPair(prime *lymcwd.Location, slug string) (Result, error) {
	if err := EndSubjectProcesses(prime, slug, false); err != nil {
		return Result{}, fmt.Errorf("pairteardown: remove the handed-over pair %q: end its processes: %w", slug, err)
	}
	td, err := New(prime)
	if err != nil {
		return Result{}, err
	}
	request := Request{Slug: slug, Local: true, Force: true, Verdict: fabricengine.EntryOwnedElsewhere}
	session, err := td.EndSession(context.Background(), request)
	if err != nil {
		return Result{Session: session}, err
	}
	removal, err := td.RemovePair(request)
	return Result{Session: session, Removal: removal}, err
}

// RemoveOrphanPair tears down the pair of slug, which neither a board entry nor a batten record names.
// It first archives the pair's code and records tips that origin lacks under owner archive tags pushed to origin.
// It commits the pair's uncommitted files onto the archived tip.
// It then ends the pair's session and removes the pair, leaving its branches on origin and settling no board claim.
// A failed archive removes nothing.
// It first reads again that the pair is still an orphan.
// It returns an error naming the reason, with nothing touched, when a board entry names the slug.
// It does the same when a batten run directory names the slug or a process registered under it lives.
func RemoveOrphanPair(ctx context.Context, prime *lymcwd.Location, slug string) (Result, error) {
	if err := requireStillOrphan(prime, slug); err != nil {
		return Result{}, err
	}
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return Result{}, fmt.Errorf("pairteardown: locate the orphan pair %q: %w", slug, err)
	}
	archive := []fabricengine.OwnerArchivePair{{Slug: slug, Pair: pair, CommitUncommitted: true}}
	if _, err := fabricengine.ArchiveOwnerTips(prime.HubPath, archive); err != nil {
		return Result{}, fmt.Errorf("pairteardown: archive the tips of the orphan pair %q: %w", slug, err)
	}
	td, err := New(prime)
	if err != nil {
		return Result{}, err
	}
	return td.Run(ctx, Request{Slug: slug, Force: true, LeaveClaim: true, Verdict: fabricengine.EntryEnded})
}

// requireStillOrphan returns an error when the board, the batten run directory or a registered live process names slug.
func requireStillOrphan(prime *lymcwd.Location, slug string) error {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return fmt.Errorf("pairteardown: read the board before removing the orphan pair %q: %w", slug, err)
	}
	if slices.ContainsFunc(tasks, func(task boardengine.Task) bool { return task.Slug == slug }) {
		return fmt.Errorf("pairteardown: %q is no orphan pair: a board entry names it", slug)
	}
	if _, err := os.Stat(shedrun.RunDir(prime, slug)); err == nil {
		return fmt.Errorf("pairteardown: %q is no orphan pair: a batten run directory names it", slug)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("pairteardown: read the batten run directory of %q: %w", slug, err)
	}
	live, err := runproc.List(fabricengine.RunProcDir(prime.HubPath), slug)
	if err != nil {
		return fmt.Errorf("pairteardown: read the processes of %q: %w", slug, err)
	}
	if len(live) > 0 {
		return fmt.Errorf("pairteardown: %q is no idle orphan pair: a process registered under it lives", slug)
	}
	return nil
}
