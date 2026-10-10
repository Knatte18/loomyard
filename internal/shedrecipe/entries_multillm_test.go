// entries_multillm_test.go covers multiLLMEntry: its construction refusals over Config and Env,
// and the seat table a valid row hands the seat runner.

package shedrecipe

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// validMultiLLMConfig returns a Config multiLLMEntry accepts over an Env holding the "multillm-seat" stencil:
// a chair and one advisor whose output is the chair's input.
func validMultiLLMConfig() Config {
	return Config{
		"role":    "multi",
		"segment": "plan",
		"seats": []any{
			map[string]any{
				"name":    "chair",
				"stencil": "multillm-seat",
				"model":   "opus[high]",
				"inputs":  []string{"in/spec.md", "out/advisor.md"},
				"outputs": []string{"out/chair.md"},
			},
			map[string]any{
				"name":    "advisor-1",
				"stencil": "multillm-seat",
				"model":   "opus",
				"outputs": []string{"out/advisor.md"},
				"skills":  []string{"scribe:prose"},
				"values":  map[string]any{"focus": "naming"},
			},
		},
	}
}

// multiLLMSeatConfig returns the i-th element of cfg's seats list for in-place mutation.
func multiLLMSeatConfig(cfg Config, i int) map[string]any {
	return cfg["seats"].([]any)[i].(map[string]any)
}

func TestMultiLLMEntry_ConstructionFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(env *Env, cfg Config)
		want   string
	}{
		{"UnknownRowKey", func(_ *Env, cfg Config) { cfg["bogus_key"] = "x" }, "bogus_key"},
		{"UnknownSeatKey", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["bogus_seat_key"] = "x" }, "bogus_seat_key"},
		{"NoSeats", func(_ *Env, cfg Config) { delete(cfg, "seats") }, `"seats"`},
		{"MissingRole", func(_ *Env, cfg Config) { delete(cfg, "role") }, `"role"`},
		{"NegativeTimeout", func(_ *Env, cfg Config) { cfg["timeout_min"] = -1 }, "timeout_min"},
		{"NilSeatsSeam", func(env *Env, _ Config) { env.Seats = nil }, "Env.Seats"},
		{"RelativeWorktreeRoot", func(env *Env, _ Config) { env.WorktreeRoot = "rel" }, "WorktreeRoot"},
		{"BadSegment", func(_ *Env, cfg Config) { cfg["segment"] = "nowhere" }, "nowhere"},
		{"UnknownModelAlias", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["model"] = "no-such-alias" }, "no-such-alias"},
		{"UnparsableModelSpec", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 0)["model"] = "opus[" }, "seat 0"},
		{"ReservedToken", func(_ *Env, cfg Config) { cfg["tokens"] = map[string]any{"outputs": "x"} }, "outputs"},
		{"EmptyToken", func(_ *Env, cfg Config) { cfg["tokens"] = map[string]any{"focus": "  "} }, "focus"},
		{"ReservedSeatValue", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["values"] = map[string]any{"seat_name": "x"} }, "seat_name"},
		{"EmptySeatValue", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["values"] = map[string]any{"focus": ""} }, "focus"},
		{"AbsoluteInput", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 0)["inputs"] = []string{"/abs/in.md"} }, "must not be absolute"},
		{"EscapingOutput", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["outputs"] = []string{"../out.md"} }, "escapes root"},
		{"UnreadableStencil", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["stencil"] = "multillm-missing" }, "multillm-missing"},
		{"AdvisorOutputNotChairInput", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["outputs"] = []string{"out/other.md"} }, "not among the chair's inputs"},
		{"AdvisorMisnamed", func(_ *Env, cfg Config) { multiLLMSeatConfig(cfg, 1)["name"] = "advisor-2" }, "advisor-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			writeStencilFile(t, env.StencilsDir, "multillm-seat", "Body text with no markers.\n")
			cfg := validMultiLLMConfig()
			tc.mutate(&env, cfg)
			_, err := multiLLMEntry("Row", cfg, env)
			assertErrContains(t, err, tc.want)
		})
	}

	t.Run("IncludeThatIncludes", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		writeStencilFile(t, env.StencilsDir, "multillm-seat", `{{template "multillm-block"}}`+"\n")
		writeStencilFile(t, env.StencilsDir, "multillm-block", `{{template "multillm-other"}}`+"\n")
		writeStencilFile(t, env.StencilsDir, "multillm-other", "Leaf.\n")
		_, err := multiLLMEntry("Row", validMultiLLMConfig(), env)
		assertErrContains(t, err, "includes resolve one level deep")
	})
}

func TestMultiLLMEntry_ComposedTable(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	writeStencilFile(t, env.StencilsDir, "multillm-seat", "Body text with no markers.\n")
	fake := &shedfake.SeatRunner{Results: []seatengine.Result{{
		Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		ChairOutputs: []string{filepath.Join(env.WorktreeRoot, "out/chair.md")},
	}}}
	env.Seats = fake
	cfg := validMultiLLMConfig()
	cfg["timeout_min"] = 7
	cfg["interactive"] = true
	cfg["tokens"] = map[string]any{"topic": "caching"}
	cfg["gates"] = []any{map[string]any{"name": "plan", "attempts": 2}}

	producer, err := multiLLMEntry("Row", cfg, env)
	if err != nil {
		t.Fatalf("multiLLMEntry() error = %v; want nil", err)
	}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if len(fake.GotTables) != 1 {
		t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
	}
	table := fake.GotTables[0]

	root := env.WorktreeRoot
	want := seatengine.Table{
		RolePrefix:  "multi",
		Segment:     segmentcolor.Plan,
		Timeout:     7 * time.Minute,
		Interactive: true,
		Values:      map[string]string{"topic": "caching"},
		Seats: []seatengine.Seat{
			{
				Name:    seatengine.RoleChair,
				Stencil: "multillm-seat",
				Model:   "claude-opus-test",
				Inputs:  []string{filepath.Join(root, "in/spec.md"), filepath.Join(root, "out/advisor.md")},
				Outputs: []string{filepath.Join(root, "out/chair.md")},
			},
			{
				Name:    seatengine.AdvisorName(1),
				Stencil: "multillm-seat",
				Model:   "claude-opus-test",
				Outputs: []string{filepath.Join(root, "out/advisor.md")},
				Skills:  []string{"scribe:prose"},
				Values:  map[string]string{"focus": "naming"},
			},
		},
	}
	// The chair's "opus[high]" spec resolves its effort from the bracket.
	want.Seats[0].Effort = "high"
	// A gate holds closures, which DeepEqual never matches, so it is compared by name and budget.
	if len(table.Gate) != 1 || table.Gate[0].Name != "plan" || table.Gate[0].Attempts != 2 {
		t.Errorf("table gate = %+v; want one \"plan\" entry with 2 attempts", table.Gate)
	}
	table.Gate, want.Gate = nil, nil
	if !reflect.DeepEqual(table, want) {
		t.Errorf("table = %#v; want %#v", table, want)
	}
}
