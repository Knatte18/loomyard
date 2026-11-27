// build_test.go covers Build's own routing logic against Recipe values constructed in Go rather than parsed,
// so a Build failure can never be a Parse failure in disguise.
// The filesystem-backed coverage of every registered engine lives in build_engines_test.go.

package shedbuild

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBuild_RowsCarryRoutingInRecipeOrder asserts Build returns one definition per row, in recipe order.
// Each definition carries its row's routing fields and a producer.
func TestBuild_RowsCarryRoutingInRecipeOrder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	r := Recipe{
		Producers: []Row{
			{Name: "First", Engine: "Stub", OnDone: "Second", OnStuck: "First", OnReroute: "Third", Segment: "seg", MaxBounces: 3, SkipTo: "Third", BlockPath: "Outer/Inner"},
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
		if def.Name != row.Name || def.OnDone != row.OnDone || def.OnStuck != row.OnStuck || def.OnReroute != row.OnReroute ||
			def.Segment != row.Segment || def.MaxBounces != row.MaxBounces || def.SkipTo != row.SkipTo || def.Block != row.BlockPath {
			t.Errorf("defs[%d] = {%q %q %q %q %q %d %q %q}; want row {%q %q %q %q %q %d %q %q}", i,
				def.Name, def.OnDone, def.OnStuck, def.OnReroute, def.Segment, def.MaxBounces, def.SkipTo, def.Block,
				row.Name, row.OnDone, row.OnStuck, row.OnReroute, row.Segment, row.MaxBounces, row.SkipTo, row.BlockPath)
		}
		if def.Producer == nil {
			t.Errorf("defs[%d].Producer = nil; want non-nil", i)
		}
	}
}

// TestBuild_Errors asserts each Build failure wraps the shedbuild prefix.
// The error names the offending row's index and name, an unknown engine, or the rejected config key.
func TestBuild_Errors(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		rows []Row
		// contract is the Contract the recipe declares, or empty for none.
		contract string
		// fullEnv selects envkit.FullEnv in place of the zero shedrecipe.Env.
		fullEnv  bool
		wantSubs []string
	}{
		{
			name:     "block row naming no block gate check",
			rows:     []Row{{Name: "Blk", Engine: shedrecipe.EngineBlock, Block: "sub", BlockGate: "no-such-check"}},
			wantSubs: []string{"producer 0", "Blk", "no-such-check", "way forward", shedrecipe.BlockGateAlways},
		},
		{
			// The contract is loom to prove the not-expanded refusal wins over the contract check.
			name:     "block row that was never expanded",
			rows:     []Row{{Name: "Raw-Block", Block: "sub", BlockGate: shedrecipe.BlockGateAlways}},
			contract: "loom",
			wantSubs: []string{"producer 0", "Raw-Block", "block row is not expanded", "way forward: expand the recipe with shedbuild.Expand"},
		},
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
			name:     "entry gate naming an unregistered module",
			rows:     []Row{{Name: "Gated", Engine: "Stub", EntryGate: EntryChecks{"nosuchmodule.key"}}},
			fullEnv:  true,
			wantSubs: []string{"Gated", "nosuchmodule.key", "not a registered config module"},
		},
		{
			name:     "entry gate naming a key the module's template lacks",
			rows:     []Row{{Name: "Gated", Engine: "Stub", EntryGate: EntryChecks{"loom.nosuchkey"}}},
			fullEnv:  true,
			wantSubs: []string{"Gated", "loom.nosuchkey", "is not a key of module"},
		},
		{
			name:     "entry gate naming a segment with no Env.Segments entry",
			rows:     []Row{{Name: "Gated", Engine: "Stub", EntryGate: EntryChecks{"budget.No-Such-Segment"}}},
			fullEnv:  true,
			wantSubs: []string{"Gated", "budget.No-Such-Segment", "has no Env.Segments entry"},
		},
		{
			name:     "entry gate naming a setting while Env.SettingKnown is nil",
			rows:     []Row{{Name: "Gated", Engine: "Stub", EntryGate: EntryChecks{"tapestry.plan_auto_accept"}}},
			wantSubs: []string{"Gated", "Env.SettingKnown is nil", "fill Env.SettingKnown in the caller"},
		},
		{
			// Proves cfg reaches the constructor rather than being dropped:
			// an engine that accepts no config keys rejects a non-empty block through its own unknown-key check.
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

			_, err := Build(Recipe{Contract: tt.contract, Producers: tt.rows}, env)
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

// TestBuild_LoomContract asserts Build refuses a recipe that declares the loom contract and breaks one of its clauses.
// The refusal names the clause and its way forward.
// A recipe without the contract is never checked.
func TestBuild_LoomContract(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// validLoom returns a recipe meeting every clause:
	// a record row, a build row after it, and a Finalize row followed only by a terminal.
	validLoom := func() Recipe {
		return Recipe{
			Contract:     "loom",
			VerifySource: "plan",
			Entry:        "Pre",
			Terminals:    []string{"Reflect"},
			Producers: []Row{
				{Name: "Pre", Engine: "Stub", OnDone: "Record"},
				{Name: "Record", Engine: "DiscussionWrite", OnDone: "Middle"},
				{Name: "Middle", Engine: "Stub", OnDone: "Build"},
				{Name: "Build", Engine: "Implementer", OnDone: "Land"},
				{Name: "Land", Engine: "Finalize", OnDone: "Reflect"},
				{Name: "Reflect", Engine: "Stub"},
			},
		}
	}
	setEngine := func(name, engine string) func(*Recipe) {
		return func(r *Recipe) {
			for i := range r.Producers {
				if r.Producers[i].Name == name {
					r.Producers[i].Engine = engine
				}
			}
		}
	}

	tests := []struct {
		name     string
		mutate   func(*Recipe)
		wantSubs []string
	}{
		{
			name:     "no record row",
			mutate:   setEngine("Record", "Stub"),
			wantSubs: []string{"shedbuild: ", "0 record rows", "way forward: give the recipe exactly one record row, engine DiscussionWrite or RecordWrite"},
		},
		{
			name:     "two record rows",
			mutate:   setEngine("Middle", "RecordWrite"),
			wantSubs: []string{"shedbuild: ", "2 record rows", "way forward: give the recipe exactly one record row, engine DiscussionWrite or RecordWrite"},
		},
		{
			name:     "no build row",
			mutate:   setEngine("Build", "Stub"),
			wantSubs: []string{"shedbuild: ", "0 build rows", "way forward: give the recipe exactly one build row, engine Implementer, DarnWrite, BeamAwait or SamplerSession"},
		},
		{
			name:     "two build rows",
			mutate:   setEngine("Middle", "DarnWrite"),
			wantSubs: []string{"shedbuild: ", "2 build rows", "way forward: give the recipe exactly one build row, engine Implementer, DarnWrite, BeamAwait or SamplerSession"},
		},
		{
			name: "build row before the record row on the on_done path",
			mutate: func(r *Recipe) {
				setEngine("Record", "Implementer")(r)
				setEngine("Build", "DiscussionWrite")(r)
			},
			wantSubs: []string{"shedbuild: ", "does not lie after the record row", "way forward: place the build row after the record row on the on_done path from the entry"},
		},
		{
			name:     "no Finalize row",
			mutate:   setEngine("Land", "Stub"),
			wantSubs: []string{"shedbuild: ", "holds no Finalize row", "way forward: end the recipe's on_done path at Finalize followed only by terminals"},
		},
		{
			name:     "Finalize followed by a row that is not a terminal",
			mutate:   func(r *Recipe) { r.Terminals = []string{"Elsewhere"} },
			wantSubs: []string{"shedbuild: ", `row "Reflect"`, "not a terminal", "way forward: end the recipe's on_done path at Finalize followed only by terminals"},
		},
		{
			name:     "no verify_source",
			mutate:   func(r *Recipe) { r.VerifySource = "" },
			wantSubs: []string{"shedbuild: ", "verify_source", "way forward: set verify_source to plan or tree_scope"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := validLoom()
			tt.mutate(&r)

			_, err := Build(r, shedrecipe.Env{})
			if err == nil {
				t.Fatal("Build() error = nil; want the loom contract refusal")
			}
			for _, want := range tt.wantSubs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Build() error = %v; want it to contain %q", err, want)
				}
			}
		})
	}

	t.Run("a valid recipe passes the check", func(t *testing.T) {
		t.Parallel()
		if err := checkLoomContract(validLoom()); err != nil {
			t.Errorf("checkLoomContract(valid recipe) = %v; want nil", err)
		}
	})

	t.Run("a record-only recipe, with neither a build row nor a Finalize row, passes with no verify_source", func(t *testing.T) {
		t.Parallel()
		r := validLoom()
		setEngine("Build", "Stub")(&r)
		setEngine("Land", "Stub")(&r)
		r.VerifySource = ""
		if err := checkLoomContract(r); err != nil {
			t.Errorf("checkLoomContract(record-only recipe) = %v; want nil", err)
		}
	})

	t.Run("a recipe without the contract is not checked", func(t *testing.T) {
		t.Parallel()
		r := Recipe{Producers: []Row{{Name: "Only", Engine: "Stub"}}}
		if _, err := Build(r, shedrecipe.Env{}); err != nil {
			t.Errorf("Build(recipe without contract) error = %v; want nil", err)
		}
	})
}
