// finalize_land.go implements Finalize's land check, which halts the landing of a beam while an open member or a pair forked from it remains, and words the ways forward itself.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// LandHaltPrefix opens the reason of every halt Finalize's land check makes, so a reader of a halt note or a test tells a land halt from any other stop.
const LandHaltPrefix = "land check: "

// LandBlocker is one thing that still works under a beam: a local pair forked from it, or an open member entry.
// Pair marks a forked pair; a member carries whether this machine holds its pair and whether a run is in flight on its board entry.
type LandBlocker struct {
	Slug    string
	Pair    bool
	HasPair bool
	Running bool
}

// landCheck halts the landing of a beam while a blocker remains, which backstops an open member the run's earlier re-check missed, such as one a stale board on another machine added.
// stop is false when the landing may go on: with no LandBlockers seam, with no blocker, and for a Finalize already past its parent-side merge, which finishes whatever remains under the beam.
// Otherwise the other values are what Call returns at once: a halt behind LandHaltPrefix naming each blocker's way forward, or the read failure and the re-run.
func (fz *Finalize) landCheck(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if fz.deps.LandBlockers == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	if fz.deps.ParentHoldsTaskWork != nil {
		held, err := fz.deps.ParentHoldsTaskWork()
		if err != nil {
			return fz.landHalt(ctx, fmt.Sprintf("could not read whether the parent branch already holds the beam's work: %v; %s", err, landRerun(fz.deps.Slug)))
		}
		if held {
			return "", shedengine.OutputPointer{}, nil, false
		}
	}
	blockers, err := fz.deps.LandBlockers()
	if err != nil {
		return fz.landHalt(ctx, fmt.Sprintf("could not read what still works under the beam %s: %v; %s", fz.deps.Slug, err, landRerun(fz.deps.Slug)))
	}
	if len(blockers) == 0 {
		return "", shedengine.OutputPointer{}, nil, false
	}
	return fz.landHalt(ctx, landHaltReason(fz.deps.Slug, fz.deps.ParentWorktree, blockers))
}

// landHalt returns the halting Stuck of the land check with reason behind LandHaltPrefix, which writes the loom halt note.
func (fz *Finalize) landHalt(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	outcome, out, err := stuckVerdict(ctx, finalizeName, LandHaltPrefix+reason, true)
	return outcome, out, err, true
}

// landRerun is the way forward of a land check that could not be judged: the run's re-entry from the beam's worktree once the cause is gone.
func landRerun(beam string) string {
	return fmt.Sprintf("run `lym loom resume` in the beam's worktree %s", beam)
}

// landHaltReason words the reason of a land halt for the beam, whose parent worktree is parentWorktree.
// A forked pair and a member with a pair or a run in flight are cancelled, in the parent worktree or on the machine holding the pair of a run in flight with no pair here, and a member is then moved off the beam, since the cancel leaves it a draft member, which still blocks the landing.
// A member with no run status is moved alone.
// It never names the landing of a member with `lym batten run`, which the landing hold refuses.
func landHaltReason(beam, parentWorktree string, blockers []LandBlocker) string {
	moveOff := "move its `project` off the beam with `lym board upsert`"
	var ways []string
	for _, blocker := range blockers {
		switch {
		case blocker.Pair:
			ways = append(ways, fmt.Sprintf("the pair %s was forked from the beam: end it with `lym batten cancel %s --remove`, run in the parent worktree %s", blocker.Slug, blocker.Slug, parentWorktree))
		case blocker.HasPair:
			ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam: end it with `lym batten cancel %s --remove`, run in the parent worktree %s, then %s, since the cancel leaves a `draft` member, which still blocks the landing", blocker.Slug, blocker.Slug, parentWorktree, moveOff))
		case blocker.Running:
			ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam with a run in flight and no pair on this machine: end it with `lym batten cancel %s --remove` on the machine holding its pair, which `lym batten status %s` there shows, then %s, since the cancel leaves a `draft` member, which still blocks the landing", blocker.Slug, blocker.Slug, blocker.Slug, moveOff))
		default:
			ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam with no run status: %s", blocker.Slug, moveOff))
		}
	}
	return fmt.Sprintf("the beam %s cannot land while work remains under it; way forward: %s; then %s", beam, strings.Join(ways, "; "), landRerun(beam))
}
