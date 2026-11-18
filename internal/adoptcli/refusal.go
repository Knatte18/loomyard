// refusal.go holds the check that runs before adopt's first write:
// the refusal of a standing anchor change.

package adoptcli

import (
	"fmt"
)

// refuseStandingMove refuses an adopt while an anchor change stands in a worktree it would act under.
// slugs lists the adopted beams and the runs moved with them.
// It checks the prime's worktree and the parent worktree of each slug.
// It also checks each slug's own worktree when this machine holds its pair.
// The refusal carries the way forward the standing record names, then the re-run.
// Nothing runs under a moving anchor: no hold is written, no pair added and no run taken over.
// A read error refuses, naming the entry or worktree whose read failed,
// and a nil `MoveInProgress` reads as no record standing.
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
