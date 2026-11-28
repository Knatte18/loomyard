// live.go declares the reads other modules make of the hub's orchs:
// HubOrchs lists the worktrees an orch may run in, and OrchLive reports whether one worktree's orch is live.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchroute"
)

// HubOrchs lists the worktrees of the hub that location belongs to in which an orch may run:
// the prime's location first, then each local pair whose origin record carries the beam mark.
// A pair whose origin record cannot be read is skipped with a warning.
// A reader that must keep working under a pause cannot stop on one half-present pair.
// A pair still being created, in its staging directory or without its _lym junction yet, is skipped silently.
// Nothing is read through such a pair.
func HubOrchs(location *lymcwd.Location) ([]*lymcwd.Location, error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return nil, err
	}
	worktrees, err := fabricengine.List(prime.WorktreePath())
	if err != nil {
		return nil, err
	}
	orchs := []*lymcwd.Location{prime}
	for _, worktree := range worktrees {
		// A worktree still being created is skipped before anything reads through it: it has no origin yet,
		// and its wiring refuses a real _lym a read could leave behind.
		if worktree.Main || worktree.Prunable || fabricengine.InWorktreeStaging(worktree.Path) {
			continue
		}
		pair, err := lymcwd.ResolveWorktree(worktree.Path)
		if err != nil {
			logger.Warn("orch: skipping a worktree that does not resolve while listing the hub's orchs", "worktree", worktree.Path, "error", err)
			continue
		}
		wired, err := fabricengine.LymJunctionWiredHere(pair)
		if err != nil {
			logger.Warn("orch: skipping a pair whose _lym junction cannot be read while listing the hub's orchs", "worktree", pair.WorktreeName, "error", err)
			continue
		}
		if !wired {
			continue
		}
		origin, found, err := fabricengine.ReadOrigin(pair)
		if err != nil {
			logger.Warn("orch: skipping a pair whose origin record cannot be read while listing the hub's orchs", "worktree", pair.WorktreeName, "error", err)
			continue
		}
		if found && origin.Beam {
			orchs = append(orchs, pair)
		}
	}
	return orchs, nil
}

// OrchLive reports whether the orch of worktree is live:
// its orch state records a strand that reed tracks and reports live.
// A worktree whose orch never started, or whose reed session is gone, is not live.
// Nor is one whose recorded strand reed no longer tracks.
func OrchLive(worktree *lymcwd.Location) (bool, error) {
	return orchroute.Live(worktree)
}
