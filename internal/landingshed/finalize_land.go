// finalize_land.go implements the land check of Finalize.
// The check halts the landing of a beam while an open member or a pair forked from it remains,
// and it words the ways forward of that halt.

package landingshed

import (
	"context"
	"fmt"
	"strings"
)

// LandHaltPrefix opens the reason of every land halt,
// so a reader of a halt note or a test tells a land halt from any other stop.
const LandHaltPrefix = "land check: "

// LandBlocker is one thing that still works under a beam: a local pair forked from it, or an open member entry.
// Pair reports whether the blocker is a forked pair.
// For a member, HasPair reports whether this machine holds its pair.
// Running reports whether a run is in flight on the member's board entry.
type LandBlocker struct {
	Slug    string
	Pair    bool
	HasPair bool
	Running bool
}

// landCheck halts the landing of a beam while a blocker remains.
// It backstops an open member the run's earlier re-check missed, such as one a stale board on another machine added.
// It returns nil when the landing may go on: with no LandBlockers seam, or with no blocker.
// It also returns nil for a Finalize already past its parent-side merge, as held reports.
// Such a Finalize finishes whatever remains under the beam.
// Otherwise it returns the halt, whose reason opens with LandHaltPrefix.
// Its reason names each blocker's way forward, or, when a read failed, the re-run.
func (fz *Finalize) landCheck(ctx context.Context, held bool) *ending {
	if fz.deps.LandBlockers == nil || held {
		return nil
	}
	blockers, err := fz.deps.LandBlockers()
	if err != nil {
		return halt(ctx, finalizeName, LandHaltPrefix+fmt.Sprintf("could not read what still works under the beam %s: %v; %s", fz.deps.Slug, err, landRerun(fz.deps.Slug)))
	}
	if len(blockers) == 0 {
		return nil
	}
	return halt(ctx, finalizeName, LandHaltPrefix+landHaltReason(fz.deps.Slug, fz.deps.ParentWorktree, blockers))
}

// landRerun is the way forward of a land check that could not be judged:
// the run's re-entry from the beam's worktree once the cause is gone.
func landRerun(beam string) string {
	return fmt.Sprintf("run `lym loom resume` in the beam's worktree %s", beam)
}

// landHaltReason words the reason of a land halt for the beam, whose parent worktree is parentWorktree.
// A forked pair is ended with `lym batten cancel --remove` in the parent worktree.
// A member with a pair here is ended the same way, then moved off the beam:
// a cancel leaves a `draft` member, which still blocks the landing.
// A member with a run in flight and no pair here is ended on the machine holding its pair;
// `lym batten status` shows that machine, and the member is then moved off the beam.
// A member with no run status is moved off the beam alone.
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
