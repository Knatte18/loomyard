// amendduringwebster_test.go pins a plan amendment made while Webster holds done batches.
// It drives the amendment through to the plan rework's gate.

package loomcli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestAmendsPlanDuringWebster_KeepsDoneBatches pins that an amendment made while Webster holds done batches keeps them.
// Webster has recorded four batches done, each card's Create landed in the tree, and a fifth card is pending.
// The parent records a decision with --amends-plan and moves the run back to Plan-Burler-Review.
// Webster's run record then holds the same batches the record held before.
// That holds across decisionVerb over its test seams and the engine's goto, not loom's goto hooks.
// The Plan-Burler-Fix plan gate passes without a create-already-exists finding on the landed cards.
func TestAmendsPlanDuringWebster_KeepsDoneBatches(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const doneCards = 4
	landed := "package sub\n"
	plan := plankit.Plan{Approved: true, Language: "go", Framing: "Framing."}
	batches := map[int]*websterengine.BatchState{}
	for n := 1; n <= doneCards+1; n++ {
		slug := fmt.Sprintf("card-%d", n)
		plan.Cards = append(plan.Cards, plankit.Card{
			Number:  n,
			Slug:    slug,
			Summary: "placeholder card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{fmt.Sprintf("sub#Foo%d", n)}}},
			Intent:  "placeholder card.",
		})
		if n > doneCards {
			continue
		}
		landed += fmt.Sprintf("\nfunc Foo%d() {}\n", n)
		batches[n] = &websterengine.BatchState{Slug: slug, Cards: []string{fmt.Sprintf("%02d-%s", n, slug)}, Terminal: true, Status: websterengine.DigestStatusDone}
	}
	worktreeRoot := plankit.Repo(t, map[string]string{"sub/a.go": landed})
	anchorPath := t.TempDir()
	plankit.Write(t, planparser.PlanDir(anchorPath), plan)
	if err := websterengine.SaveState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath), &websterengine.State{State: runplan.State{Batches: batches}}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	recorded, err := websterengine.LoadState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath))
	if err != nil {
		t.Fatalf("LoadState() before the amendment: %v", err)
	}

	f := newDecisionFake(t)
	f.implementerSeam = weaverengine.ActiveDeps{AnchorRoot: anchorPath}
	f.amendmentPath = filepath.Join(anchorPath, "amendment.yaml")
	f.found = true
	f.status = shedengine.Status{CurrentProducer: loomshed.NameWebster, State: shedengine.StateBlocked}
	deps := f.deps()
	deps.approvedPlan = planApprovedReader(anchorPath)
	input := goodDecisionInput()
	input.amendsPlan = true
	var out bytes.Buffer
	if code := decisionVerb(&out, "task-a", deps, input); code != 0 {
		t.Fatalf("decisionVerb() exit = %d, out %s; want 0", code, out.String())
	}

	runDir := t.TempDir()
	statusPath := filepath.Join(runDir, "status.json")
	statusLockPath := filepath.Join(runDir, "status.json.lock")
	writeStatusFixture(t, statusPath, statusLockPath, shedengine.Status{
		CurrentProducer: loomshed.NameWebster,
		State:           shedengine.StateBlocked,
		History: []shedengine.HistoryEntry{
			{Producer: loomshed.NamePlanBurlerReview, Outcome: shedengine.Stuck, At: "2026-01-01T00:00:00Z"},
			{Producer: loomshed.NamePlanBurlerFix, Outcome: shedengine.Stuck, At: "2026-01-01T00:00:01Z"},
			{Producer: loomshed.NamePlanBouncer, Outcome: shedengine.Done, At: "2026-01-01T00:00:02Z"},
			{Producer: loomshed.NameWebster, Outcome: shedengine.Stuck, At: "2026-01-01T00:00:03Z"},
		},
		Product: productJSON(t, loomengine.Status{Slug: "task-a", Parent: "main"}),
	})
	if _, err := shedengine.Goto(shedengine.GotoRequest{
		StatusPath:     statusPath,
		LockPath:       filepath.Join(runDir, "run.lock"),
		StatusLockPath: statusLockPath,
		Producers: []shedengine.ProducerDef{
			{Name: loomshed.NamePlanBouncer, OnStuck: loomshed.NamePlanBurlerReview, OnDone: loomshed.NameWebster},
			{Name: loomshed.NamePlanBurlerReview, OnStuck: loomshed.NamePlanBurlerFix},
			{Name: loomshed.NamePlanBurlerFix, OnStuck: loomshed.NamePlanBouncer},
			{Name: loomshed.NameWebster},
		},
		Target: loomshed.NamePlanBurlerReview,
	}); err != nil {
		t.Fatalf("shedengine.Goto() error = %v", err)
	}

	after, err := websterengine.LoadState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath))
	if err != nil {
		t.Fatalf("LoadState() after the goto: %v", err)
	}
	if after == nil || !reflect.DeepEqual(after, recorded) {
		t.Errorf("LoadState() after the goto = %+v; want %+v", after, recorded)
	}

	result, err := loomshed.NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), loomshed.PlanContext{})()
	if err != nil {
		t.Fatalf("plan gate error = %v; want nil", err)
	}
	if !result.Passed || strings.Contains(result.Findings, "create-already-exists") {
		t.Errorf("plan gate = %+v; want a pass with no create-already-exists on the done cards", result)
	}
}
