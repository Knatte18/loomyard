// refusal.go holds the checks that run before adopt's first mutation: the refusal of a standing anchor change, and the read-only answer to whether an entry's takeover would stop.

package adoptcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// refuseStandingMove refuses an adopt while an anchor change stands in a worktree it would act under: the prime, each adopted beam and moved run's own worktree when this machine holds its pair, and each one's parent worktree.
// slugs lists the adopted beams and the runs moved with them.
// The refusal carries the way forward the standing record names, then the re-run, so no pair is added, no run resumed and no gate suite started under a moving anchor.
// A read error refuses naming it, and nil MoveInProgress reads as no record standing.
func refuseStandingMove(deps adoptDeps, slugs []string) error {
	if deps.MoveInProgress == nil {
		return nil
	}
	// The empty name is the prime's worktree.
	subjects := []string{""}
	for _, slug := range slugs {
		if deps.LocalPair(slug) {
			subjects = append(subjects, slug)
		}
		parent, err := deps.ParentOf(slug)
		if err != nil {
			return fmt.Errorf("lym adopt: read the parent of %s, so nothing ran: %w; %s", slug, err, reentry)
		}
		subjects = append(subjects, parent)
	}
	checked := map[string]bool{}
	for _, subject := range subjects {
		if checked[subject] {
			continue
		}
		checked[subject] = true
		wayForward, standing, err := deps.MoveInProgress(subject)
		if err != nil {
			return fmt.Errorf("lym adopt: read whether an anchor change stands in %q, so nothing ran: %w; %s", subject, err, reentry)
		}
		if standing {
			return fmt.Errorf("lym adopt: an anchor change stands, so nothing ran: %s; %s", wayForward, reentry)
		}
	}
	return nil
}

// wouldStop answers, from the read-only part of step 1 alone, whether takeOverSlug would stop slug there: its pair holds local commits origin lacks or is live, or --drop-local meets a live pair.
// An entry no other machine owns, one with no pair here and any entry under --keep-local pass step 1.
func wouldStop(deps adoptDeps, req request, report fabricengine.OriginReport, slug string) (bool, error) {
	if !ownedElsewhere(report, slug) || !deps.LocalPair(slug) || req.KeepLocal {
		return false, nil
	}
	live, err := deps.LivePair(slug)
	if err != nil {
		return false, err
	}
	if req.DropLocal {
		return live, nil
	}
	return live || len(stoppedBranches(report, slug)) > 0, nil
}
