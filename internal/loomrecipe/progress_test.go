// progress_test.go runs shedengine.Routing.ProgressAt over loom's real recipe: every row maps to a
// step, a review segment's two rows share one step, Friction-Reflect is the last step, and a
// Burler current producer reports its segment's step.

package loomrecipe

import "testing"

// TestRouting_ProgressAtEveryRowMapsToAStep asserts every row of the real recipe maps to a step.
func TestRouting_ProgressAtEveryRowMapsToAStep(t *testing.T) {
	routing, err := Routing(5)
	if err != nil {
		t.Fatalf("Routing() = _, %v; want nil", err)
	}
	if len(routing.Producers) == 0 {
		t.Fatal("Routing().Producers is empty")
	}
	for _, def := range routing.Producers {
		if p := routing.ProgressAt(def.Name); p.Step < 1 || p.Step > p.Steps {
			t.Errorf("ProgressAt(%q).Step = %d of %d; want a mapped step", def.Name, p.Step, p.Steps)
		}
	}
}

// TestRouting_ProgressAtReviewSegmentsShareAStep asserts both rows of each review segment map to
// the same step, and a Burler reports its segment's step name.
func TestRouting_ProgressAtReviewSegmentsShareAStep(t *testing.T) {
	routing, err := Routing(5)
	if err != nil {
		t.Fatalf("Routing() = _, %v; want nil", err)
	}
	for _, seg := range []struct{ bouncer, burler, name string }{
		{"Discussion-Bouncer", "Discussion-Burler", "Discussion-Review"},
		{"Plan-Bouncer", "Plan-Burler", "Plan-Review"},
		{"Webster-Bouncer", "Webster-Burler", "Webster-Review"},
	} {
		a, b := routing.ProgressAt(seg.bouncer), routing.ProgressAt(seg.burler)
		if a.Step != b.Step || a.Step == 0 {
			t.Errorf("%s step = %d, %s step = %d; want the same nonzero step", seg.bouncer, a.Step, seg.burler, b.Step)
		}
		if b.Name != seg.name {
			t.Errorf("ProgressAt(%q).Name = %q; want %q", seg.burler, b.Name, seg.name)
		}
	}
}

// TestRouting_ProgressAtFrictionReflectIsLast asserts Friction-Reflect is the last main-line step.
func TestRouting_ProgressAtFrictionReflectIsLast(t *testing.T) {
	routing, err := Routing(5)
	if err != nil {
		t.Fatalf("Routing() = _, %v; want nil", err)
	}
	p := routing.ProgressAt("Friction-Reflect")
	if p.Step != p.Steps || len(p.Remaining) != 0 {
		t.Errorf("ProgressAt(Friction-Reflect) = step %d of %d, remaining %v; want the last step", p.Step, p.Steps, p.Remaining)
	}
}
