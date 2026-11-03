// routing_test.go covers RoutingOf and RoutingFrom: the projection's routing fields equal those NewShed builds for
// the same recipe, row for row, and a parse failure or an unexpanded block row surfaces unwrapped.

package shedbuild

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRoutingOf_MatchesNewShedRowForRow(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const plainYAML = `
version: 1
entry: row1
terminals: [row3]
producers:
  - name: row1
    engine: Stub
    on_done: row2
    on_stuck: row2
    on_reroute: row3
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
	const blockParentYAML = `
version: 1
entry: row1
terminals: [row3]
producers:
  - name: row1
    engine: Stub
    on_done: Blk
  - name: Blk
    block: sub
    block_gate: always
    on_done: row3
  - name: row3
    engine: Stub
`
	const blockChildYAML = `
version: 1
entry: inner1
terminals: [inner2]
producers:
  - name: inner1
    engine: Stub
    on_done: inner2
  - name: inner2
    engine: Stub
`
	resolve := func(name string) ([]byte, error) {
		if name != "sub" {
			return nil, fmt.Errorf("no recipe %q", name)
		}
		return []byte(blockChildYAML), nil
	}

	tests := []struct {
		name      string
		wantEntry string
		// shedAndRouting builds the shed and projects the routing of the same recipe.
		shedAndRouting func(t *testing.T) (*shedengine.Shed, shedengine.Routing)
	}{
		{
			name:      "a recipe with no block",
			wantEntry: "row1",
			shedAndRouting: func(t *testing.T) (*shedengine.Shed, shedengine.Routing) {
				shed, err := NewShed([]byte(plainYAML), envkit.FullEnv(t), ShedPaths{MaxBounces: 9})
				if err != nil {
					t.Fatalf("NewShed() = _, %v; want nil", err)
				}
				routing, err := RoutingOf([]byte(plainYAML))
				if err != nil {
					t.Fatalf("RoutingOf() = _, %v; want nil", err)
				}
				return shed, routing
			},
		},
		{
			name:      "an expanded recipe with a block",
			wantEntry: "row1",
			shedAndRouting: func(t *testing.T) (*shedengine.Shed, shedengine.Routing) {
				parsed, err := Parse([]byte(blockParentYAML))
				if err != nil {
					t.Fatalf("Parse() = _, %v; want nil", err)
				}
				expanded, err := Expand(parsed, resolve)
				if err != nil {
					t.Fatalf("Expand() = _, %v; want nil", err)
				}
				shed, err := NewShedFrom(expanded, envkit.FullEnv(t), ShedPaths{MaxBounces: 9})
				if err != nil {
					t.Fatalf("NewShedFrom() = _, %v; want nil", err)
				}
				return shed, RoutingFrom(expanded)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, routing := tt.shedAndRouting(t)

			if routing.Entry != tt.wantEntry {
				t.Errorf("routing.Entry = %q; want %q", routing.Entry, tt.wantEntry)
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
				if got.Name != want.Name || got.OnDone != want.OnDone || got.OnStuck != want.OnStuck || got.OnReroute != want.OnReroute ||
					got.Segment != want.Segment || got.MaxBounces != want.MaxBounces || got.SkipTo != want.SkipTo || got.Block != want.Block {
					t.Errorf("row %d: projected {%s %s %s %s %s %d %s %s}; NewShed built {%s %s %s %s %s %d %s %s}", i,
						got.Name, got.OnDone, got.OnStuck, got.OnReroute, got.Segment, got.MaxBounces, got.SkipTo, got.Block,
						want.Name, want.OnDone, want.OnStuck, want.OnReroute, want.Segment, want.MaxBounces, want.SkipTo, want.Block)
				}
			}
		})
	}

	t.Run("the block rows carry the skip target and the block path", func(t *testing.T) {
		t.Parallel()
		_, routing := tests[1].shedAndRouting(t)

		byName := map[string]shedengine.ProducerDef{}
		for _, def := range routing.Producers {
			byName[def.Name] = def
		}
		if got := byName["Blk"]; got.SkipTo != "row3" || got.Block != "" {
			t.Errorf("Blk = {SkipTo %q Block %q}; want {SkipTo \"row3\" Block \"\"}", got.SkipTo, got.Block)
		}
		if got := byName["inner1"]; got.Block != "Blk" {
			t.Errorf("inner1.Block = %q; want %q", got.Block, "Blk")
		}
	})
}

// TestRoutingOf_ParseErrorSurfaces asserts a malformed recipe returns Parse's error, and a recipe holding a block row is refused naming Expand.
func TestRoutingOf_ParseErrorSurfaces(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if _, err := RoutingOf([]byte("version: [")); err == nil {
		t.Fatal("RoutingOf(malformed) = _, nil; want an error")
	}

	const blockRecipe = `
version: 1
entry: Blk
terminals: [Blk]
producers:
  - name: Blk
    block: sub
    block_gate: always
    on_done: Blk
`
	_, err := RoutingOf([]byte(blockRecipe))
	if err == nil || !strings.Contains(err.Error(), "shedbuild.Expand") {
		t.Errorf("RoutingOf(block row) = _, %v; want an error naming shedbuild.Expand", err)
	}
}
