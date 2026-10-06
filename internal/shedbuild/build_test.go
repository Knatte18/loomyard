// build_test.go covers Build's own routing logic against Recipe values constructed in Go rather
// than parsed, so a Build failure can never be a Parse failure in disguise. The filesystem-backed
// twelve-engine coverage lives in build_engines_test.go instead -- see that file's own doc comment.

package shedbuild

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// TestBuild_RowsCarryRoutingInRecipeOrder asserts Build returns one definition per row, in recipe
// order, each carrying its row's routing fields and a producer.
func TestBuild_RowsCarryRoutingInRecipeOrder(t *testing.T) {
	t.Parallel()

	r := Recipe{
		Producers: []Row{
			{Name: "First", Engine: "Stub", OnDone: "Second", OnStuck: "First", Segment: "seg", MaxBounces: 3},
			{Name: "Second", Engine: "Stub"},
			{Name: "Third", Engine: "Stub"},
		},
	}

	defs, err := Build(r, shedrecipe.Env{})
	if err != nil {
		t.Fatalf("Build() error = %v; want nil", err)
	}
	if len(defs) != len(r.Producers) {
		t.Fatalf("Build() returned %d defs; want %d", len(defs), len(r.Producers))
	}
	for i, row := range r.Producers {
		def := defs[i]
		if def.Name != row.Name || def.OnDone != row.OnDone || def.OnStuck != row.OnStuck ||
			def.Segment != row.Segment || def.MaxBounces != row.MaxBounces {
			t.Errorf("defs[%d] = {%q %q %q %q %d}; want row {%q %q %q %q %d}", i,
				def.Name, def.OnDone, def.OnStuck, def.Segment, def.MaxBounces,
				row.Name, row.OnDone, row.OnStuck, row.Segment, row.MaxBounces)
		}
		if def.Producer == nil {
			t.Errorf("defs[%d].Producer = nil; want non-nil", i)
		}
	}
}

// TestBuild_Errors asserts each Build failure wraps the shedbuild prefix and names the offending
// row's index and name, an unknown engine, or the rejected config key.
func TestBuild_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rows []Row
		// fullEnv selects envkit.FullEnv over a zero shedrecipe.Env.
		fullEnv  bool
		wantSubs []string
	}{
		{
			name:     "no producers",
			wantSubs: []string{"shedbuild: "},
		},
		{
			name:     "unknown engine",
			rows:     []Row{{Name: "Good", Engine: "Stub"}, {Name: "Bad-Row", Engine: "NoSuchEngine"}},
			wantSubs: []string{"1", "Bad-Row", "NoSuchEngine"},
		},
		{
			name:     "preflight with a blank Env.Cwd",
			rows:     []Row{{Name: "Pre", Engine: "Preflight"}},
			wantSubs: []string{"0", "Pre", "Cwd"},
		},
		{
			name: "single-LLM missing its stencil key",
			rows: []Row{{Name: "LLM-Row", Engine: "SingleLLM", Config: map[string]any{
				"output_files": []string{"out/a.md"},
			}}},
			fullEnv:  true,
			wantSubs: []string{"0", "LLM-Row", "stencil"},
		},
		{
			// Proves cfg reaches the constructor rather than being dropped: an engine that accepts
			// no config keys rejects a non-empty block through its own unknown-key check.
			name:     "non-empty config on an engine that accepts none",
			rows:     []Row{{Name: "Stub-Row", Engine: "Stub", Config: map[string]any{"bogus": "value"}}},
			fullEnv:  true,
			wantSubs: []string{"bogus"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shedrecipe.Env{}
			if tt.fullEnv {
				env = envkit.FullEnv(t)
			}

			_, err := Build(Recipe{Producers: tt.rows}, env)
			if err == nil {
				t.Fatal("Build() error = nil; want non-nil")
			}
			for _, want := range tt.wantSubs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Build() error = %v; want it to contain %q", err, want)
				}
			}
		})
	}
}
