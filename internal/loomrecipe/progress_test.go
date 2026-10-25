//go:build tier0

// progress_test.go runs shedengine.Routing.ProgressAt over loom's real recipe: every row maps to a
// step, a review segment's three rows share one step, Friction-Reflect is the last step, and a
// Burler current producer reports its segment's step.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
)

// TestRouting_ProgressAt asserts, over loom's real recipe, that every row maps to a step, the three rows of each review segment map to the same step and a Burler row reports its segment's step name, Friction-Reflect is the last main-line step, and the rows after Webster follow the recipe's tail order and reroute edges.
func TestRouting_ProgressAt(t *testing.T) {
	t.Parallel()
	routing, err := Routing(map[string]int{"Discussion-Review": 5, "Plan-Review": 5, "Code-Review": 5})
	if err != nil {
		t.Fatalf("Routing() = _, %v; want nil", err)
	}

	t.Run("every row maps to a step", func(t *testing.T) {
		t.Parallel()
		if len(routing.Producers) == 0 {
			t.Fatal("Routing().Producers is empty")
		}
		for _, def := range routing.Producers {
			if p := routing.ProgressAt(def.Name); p.Step < 1 || p.Step > p.Steps {
				t.Errorf("ProgressAt(%q).Step = %d of %d; want a mapped step", def.Name, p.Step, p.Steps)
			}
		}
	})

	t.Run("review segments share a step", func(t *testing.T) {
		t.Parallel()
		for _, seg := range []struct{ bouncer, review, fix, name string }{
			{"Discussion-Bouncer", "Discussion-Burler-Review", "Discussion-Burler-Fix", "Discussion-Review"},
			{"Plan-Bouncer", "Plan-Burler-Review", "Plan-Burler-Fix", "Plan-Review"},
			{"Code-Bouncer", "Code-Burler-Review", "Code-Burler-Fix", "Code-Review"},
		} {
			a := routing.ProgressAt(seg.bouncer)
			for _, row := range []string{seg.review, seg.fix} {
				b := routing.ProgressAt(row)
				if a.Step != b.Step || a.Step == 0 {
					t.Errorf("%s step = %d, %s step = %d; want the same nonzero step", seg.bouncer, a.Step, row, b.Step)
				}
				if b.Name != seg.name {
					t.Errorf("ProgressAt(%q).Name = %q; want %q", row, b.Name, seg.name)
				}
			}
		}
	})

	t.Run("the tail after Webster runs Merge-In, the Code-Review perch, Describe, PR-Review and Finalize", func(t *testing.T) {
		t.Parallel()
		rows := map[string]struct{ done, stuck, reroute string }{}
		for _, def := range routing.Producers {
			rows[def.Name] = struct{ done, stuck, reroute string }{def.OnDone, def.OnStuck, def.OnReroute}
		}
		for _, want := range []struct{ row, done, stuck, reroute string }{
			{loomshed.NameWebster, loomshed.NameParentAlign, "", loomshed.NamePlanBurlerReview},
			{loomshed.NameParentAlign, loomshed.NameCodeBouncer, "", ""},
			{loomshed.NameCodeBouncer, loomshed.NameDescribe, loomshed.NameCodeBurlerReview, ""},
			{loomshed.NameDescribe, loomshed.NamePRReview, "", ""},
			{loomshed.NamePRReview, loomshed.NameFinalize, "", ""},
			{loomshed.NameFinalize, loomshed.NameFrictionReflect, "", loomshed.NameParentAlign},
		} {
			got, ok := rows[want.row]
			if !ok {
				t.Errorf("Routing() has no row %q", want.row)
				continue
			}
			if got.done != want.done || got.stuck != want.stuck || got.reroute != want.reroute {
				t.Errorf("%s OnDone/OnStuck/OnReroute = %q/%q/%q; want %q/%q/%q", want.row, got.done, got.stuck, got.reroute, want.done, want.stuck, want.reroute)
			}
		}
	})

	t.Run("Friction-Reflect is last", func(t *testing.T) {
		t.Parallel()
		p := routing.ProgressAt("Friction-Reflect")
		if p.Step != p.Steps || len(p.Remaining) != 0 {
			t.Errorf("ProgressAt(Friction-Reflect) = step %d of %d, remaining %v; want the last step", p.Step, p.Steps, p.Remaining)
		}
	})
}
