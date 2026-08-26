// revalidate_test.go's single subject: the Plan-Revalidate row is what catches a format regression
// a fixer round introduced after the mechanical validator already ran, and routes it back to
// Plan-Write instead of letting it reach Webster.
package loomrecipe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestSequence_PlanRevalidateCatchesPostSegmentRegression scripts the Plan-Review segment's one
// review round to leave the plan present and parseable but failing planparser's own
// plan-unapproved check -- via fakeLoomBurler.corruptPlanOverview, set to the fixture's own plan
// overview path before New is called -- and asserts the run bounces from Plan-Revalidate to
// Plan-Write rather than continuing on to Batchifier.
//
// Deliberately asserts nothing about what happens after that bounce: a re-entered Plan-Bouncer now
// archives its run directory and re-judges from a fresh round 1 rather than replaying the settled
// verdict its run directory still holds. This test's single subject stays that Plan-Revalidate
// routes a post-segment format regression back to Plan-Write rather than letting it reach Webster --
// it asserts nothing about the post-bounce behaviour either way.
func TestSequence_PlanRevalidateCatchesPostSegmentRegression(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)

	planOverviewPath := filepath.Join(env.AnchorPath, lyxdirs.LyxDirName, "plan", "00-overview.md")
	env.Burler.(*fakeLoomBurler).corruptPlanOverview = planOverviewPath

	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	revalidateIdx := -1
	for i, e := range result.History {
		if e.Producer == loomshed.NamePlanRevalidate && e.Outcome == shedengine.Stuck {
			revalidateIdx = i
			break
		}
	}
	if revalidateIdx == -1 {
		t.Fatalf("Run() History has no %s Stuck entry: %+v", loomshed.NamePlanRevalidate, result.History)
	}
	if revalidateIdx+1 >= len(result.History) {
		t.Fatalf("Run() History ends at the %s Stuck entry; want a following entry naming the bounce target %q", loomshed.NamePlanRevalidate, loomshed.NamePlanWrite)
	}
	if got := result.History[revalidateIdx+1].Producer; got != loomshed.NamePlanWrite {
		t.Errorf("History[%d].Producer (following the %s Stuck entry) = %q; want the declared bounce target %q", revalidateIdx+1, loomshed.NamePlanRevalidate, got, loomshed.NamePlanWrite)
	}
}
