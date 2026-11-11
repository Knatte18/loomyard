// ownerarchive.go archives this machine's unpushed tips under a worktree another machine now owns.
// The dobby's hand-over and `lym beam start`'s refusal of such a worktree both run it.

package beamcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
)

// archiveOwnerTips archives every local tip under the worktree named worktree that origin lacks.
// It archives through fabric's owner archive.
// The pairs are the worktree's own, named `root` for the prime.
// They also include the pair of each batten run whose parent the worktree is.
// A run whose task worktree is gone archives nothing.
func archiveOwnerTips(prime *lymcwd.Location, worktree string) error {
	geom, err := hubgeom.DobbyGeometry(prime, worktree)
	if err != nil {
		return err
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return err
	}
	runs, err := battencli.DobbyRuns(prime, geom, battencli.NewDobbyParents(prime), pause.RecordPath)
	if err != nil {
		return fmt.Errorf("list the runs under %s: %w", worktree, err)
	}
	own := fabricengine.OwnerArchivePair{Slug: boardengine.RootSubject, Pair: prime}
	if geom.Slug != "" {
		pair, err := pairLocation(prime, geom.Slug)
		if err != nil {
			return err
		}
		own = fabricengine.OwnerArchivePair{Slug: geom.Slug, Pair: pair}
	}
	pairs := []fabricengine.OwnerArchivePair{own}
	for _, run := range runs {
		pair, err := pairLocation(prime, run.Slug)
		if err != nil {
			return err
		}
		pairs = append(pairs, fabricengine.OwnerArchivePair{Slug: run.Slug, Pair: pair})
	}
	tags, err := fabricengine.ArchiveOwnerTips(prime.HubPath, pairs)
	if len(tags) > 0 {
		logger.Info("beamcli: archived this machine's unpushed tips for the worktree's new owner", "worktree", worktree, "tags", tags)
	}
	return err
}

// ownerArchiveBeforeRefusal returns the archive `lym beam start` runs for target.
// The verb runs it before refusing target for another machine's ownership.
// The archive covers this machine's unpushed tips under the worktree.
// It archives nothing while an adopt in flight here covers the worktree.
func ownerArchiveBeforeRefusal(target beamTarget) func() error {
	return func() error {
		root, err := resolveBeamTarget(target.Location, rootSlug)
		if err != nil {
			return err
		}
		prime := root.Location
		worktree := target.Location.WorktreeName
		slug := worktree
		if worktree == prime.WorktreeName {
			slug = ""
		}
		if orchcli.UnderAdoptWatch(prime).Covers(slug) {
			return nil
		}
		return archiveOwnerTips(prime, worktree)
	}
}
