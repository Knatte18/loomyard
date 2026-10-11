// progress_test.go runs shedengine.Routing.ProgressAt over loom's real recipe: every row maps to a
// step, a review segment's three rows share one step, Friction-Reflect is the last step, and a
// Burler current producer reports its segment's step.

package loomrecipe

import "testing"

// TestRouting_ProgressAt asserts, over loom's real recipe, that every row maps to a step, the three rows of each review segment map to the same step and a Burler row reports its segment's step name, and Friction-Reflect is the last main-line step.
func TestRouting_ProgressAt(t *testing.T) {
	t.Parallel()
	routing, err := Routing(5)
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
			{"Webster-Bouncer", "Webster-Burler-Review", "Webster-Burler-Fix", "Webster-Review"},
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

	t.Run("Friction-Reflect is last", func(t *testing.T) {
		t.Parallel()
		p := routing.ProgressAt("Friction-Reflect")
		if p.Step != p.Steps || len(p.Remaining) != 0 {
			t.Errorf("ProgressAt(Friction-Reflect) = step %d of %d, remaining %v; want the last step", p.Step, p.Steps, p.Remaining)
		}
	})
}
