// leftover.go removes three kinds of pair that no run's own pair removal takes:
// the local pair of a run this machine handed over, an orphan pair no board entry or batten record names,
// and the bench pairs a run's darn writer made, which the run's teardown removes.

package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
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
	request := Request{Slug: slug, Local: true, Force: true, Verdict: fabriclayout.EntryOwnedElsewhere}
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
	return td.Run(ctx, Request{Slug: slug, Force: true, LeaveClaim: true, Verdict: fabriclayout.EntryEnded})
}

// RemoveBenchPairs tears down every bench pair of the task taskSlug in the hub and returns their slugs.
// A darn writer makes such pairs for its own bench, forked from its task worktree, and none may outlive its run.
// Each pair's live processes are ended first, so a driver still running in it does not get the removal refused.
// Its tips that origin lacks are then archived under owner archive tags pushed to origin.
// Its uncommitted files are committed onto the archived tip.
// The pair is then torn down with force.
// Its branches stay on origin and no board claim is settled.
// A pair whose slug a board entry or a batten run directory names is another run's and is left standing.
// A half-removed pair, one git lists as prunable or whose worktree does not resolve, is left with a warning.
// The first failure returns with the slugs removed before it; a re-run removes the rest.
func RemoveBenchPairs(ctx context.Context, prime *lymcwd.Location, taskSlug string) ([]string, error) {
	worktrees, err := fabricengine.List(prime.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("pairteardown: list the hub's pairs for the bench pairs of %q: %w", taskSlug, err)
	}
	var removed []string
	for _, worktree := range worktrees {
		if worktree.Main {
			continue
		}
		name := filepath.Base(worktree.Path)
		if worktree.Prunable {
			warnHalfRemovedBenchPair(taskSlug, name, "git lists it as prunable")
			continue
		}
		pair, err := lymcwd.ResolveWorktree(worktree.Path)
		if err != nil {
			warnHalfRemovedBenchPair(taskSlug, name, err.Error())
			continue
		}
		if !darnengine.IsBenchPairSlug(taskSlug, pair.WorktreeName) {
			continue
		}
		slug := pair.WorktreeName
		reason, err := runNamingReason(prime, slug)
		if err != nil {
			return removed, err
		}
		if reason != "" {
			logger.Info("pairteardown: a pair with a bench pair slug belongs to another run and stays", "slug", slug, "task", taskSlug, "reason", reason)
			continue
		}
		td, err := New(prime)
		if err != nil {
			return removed, err
		}
		// The processes end before the archive, so nothing they commit is left out of it.
		if _, err := td.endOwned(slug); err != nil {
			return removed, fmt.Errorf("pairteardown: end the processes of the bench pair %q: %w", slug, err)
		}
		archive := []fabricengine.OwnerArchivePair{{Slug: slug, Pair: pair, CommitUncommitted: true}}
		if _, err := fabricengine.ArchiveOwnerTips(prime.HubPath, archive); err != nil {
			return removed, fmt.Errorf("pairteardown: archive the tips of the bench pair %q: %w", slug, err)
		}
		if _, err := td.Run(ctx, Request{Slug: slug, Force: true, LeaveClaim: true, Verdict: fabriclayout.EntryEnded}); err != nil {
			return removed, fmt.Errorf("pairteardown: remove the bench pair %q: %w", slug, err)
		}
		removed = append(removed, slug)
	}
	return removed, nil
}

// warnHalfRemovedBenchPair warns that the worktree name stays, when name is a bench pair slug of the task taskSlug.
func warnHalfRemovedBenchPair(taskSlug, name, reason string) {
	if !darnengine.IsBenchPairSlug(taskSlug, name) {
		return
	}
	logger.Warn("pairteardown: a half-removed bench pair is left; run `lym fabric prune --apply` to clear it", "slug", name, "task", taskSlug, "reason", reason)
}

// runNamingReason returns why slug is a run's own pair: a board entry or a batten run directory names it.
// It returns "" when neither does.
func runNamingReason(prime *lymcwd.Location, slug string) (string, error) {
	tasks, err := boardengine.ReadTasks(fabriclayout.BoardDir(prime.HubPath))
	if err != nil {
		return "", fmt.Errorf("pairteardown: read the board before removing the pair %q: %w", slug, err)
	}
	if slices.ContainsFunc(tasks, func(task boardengine.Task) bool { return task.Slug == slug }) {
		return "a board entry names it", nil
	}
	if _, err := os.Stat(shedrun.RunDir(prime, slug)); err == nil {
		return "a batten run directory names it", nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("pairteardown: read the batten run directory of %q: %w", slug, err)
	}
	return "", nil
}

// requireStillOrphan returns an error when the board, the batten run directory or a registered live process names slug.
func requireStillOrphan(prime *lymcwd.Location, slug string) error {
	reason, err := runNamingReason(prime, slug)
	if err != nil {
		return err
	}
	if reason != "" {
		return fmt.Errorf("pairteardown: %q is no orphan pair: %s", slug, reason)
	}
	live, err := runproc.List(fabriclayout.RunProcDir(prime.HubPath), slug)
	if err != nil {
		return fmt.Errorf("pairteardown: read the processes of %q: %w", slug, err)
	}
	if len(live) > 0 {
		return fmt.Errorf("pairteardown: %q is no idle orphan pair: a process registered under it lives", slug)
	}
	return nil
}
