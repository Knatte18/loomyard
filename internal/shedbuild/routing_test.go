// routing_test.go covers RoutingOf: the projection's routing fields equal those NewShed builds for
// the same recipe, row for row, and a parse failure surfaces unwrapped.

package shedbuild

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

func TestRoutingOf_MatchesNewShedRowForRow(t *testing.T) {
	t.Parallel()

	const recipeYAML = `
version: 1
entry: row1
terminals: [row3]
producers:
  - name: row1
    engine: Stub
    on_done: row2
    on_stuck: row2
    segment: seg
    max_bounces: 4
  - name: row2
    engine: Stub
    on_done: row3
    on_stuck: row1
    segment: seg
  - name: row3
    engine: Stub
`
	env := envkit.FullEnv(t)
	shed, err := NewShed([]byte(recipeYAML), env, ShedPaths{MaxBounces: 9})
	if err != nil {
		t.Fatalf("NewShed() = _, %v; want nil", err)
	}

	routing, err := RoutingOf([]byte(recipeYAML))
	if err != nil {
		t.Fatalf("RoutingOf() = _, %v; want nil", err)
	}

	if routing.Entry != "row1" {
		t.Errorf("routing.Entry = %q; want %q", routing.Entry, "row1")
	}
	if routing.MaxBounces != 0 {
		t.Errorf("routing.MaxBounces = %d; want 0 (the shed-level default travels in ShedPaths)", routing.MaxBounces)
	}
	if len(routing.Producers) != len(shed.Producers) {
		t.Fatalf("len(routing.Producers) = %d; want %d", len(routing.Producers), len(shed.Producers))
	}
	for i, got := range routing.Producers {
		want := shed.Producers[i]
		if got.Producer != nil {
			t.Errorf("row %d (%s): Producer = %v; want nil", i, got.Name, got.Producer)
		}
		if got.Name != want.Name || got.OnDone != want.OnDone || got.OnStuck != want.OnStuck ||
			got.Segment != want.Segment || got.MaxBounces != want.MaxBounces {
			t.Errorf("row %d: projected {%s %s %s %s %d}; NewShed built {%s %s %s %s %d}", i,
				got.Name, got.OnDone, got.OnStuck, got.Segment, got.MaxBounces,
				want.Name, want.OnDone, want.OnStuck, want.Segment, want.MaxBounces)
		}
	}
}

// TestRoutingOf_ParseErrorSurfaces asserts a malformed recipe returns Parse's error.
func TestRoutingOf_ParseErrorSurfaces(t *testing.T) {
	t.Parallel()

	if _, err := RoutingOf([]byte("version: [")); err == nil {
		t.Fatal("RoutingOf(malformed) = _, nil; want an error")
	}
}
