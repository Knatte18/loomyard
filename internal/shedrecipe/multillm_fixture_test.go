//go:build tier0

// multillm_fixture_test.go proves the recipe path end to end against fakes:
// the fixture recipe is parsed and built through shedbuild, its one MultiLLM row is called, and the seat table the fake seat runner records is checked.
// It lives in the external test package because shedbuild imports shedrecipe.

package shedrecipe_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func TestMultiLLMFixtureRecipe_BuildsAndRunsAgainstFakeSeats(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("testdata", "multillm-recipe.yaml"))
	if err != nil {
		t.Fatalf("read fixture recipe: %v", err)
	}
	recipe, err := shedbuild.Parse(data)
	if err != nil {
		t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
	}

	env := envkit.FullEnv(t)
	stencilkit.SeedInto(t, env.StencilsDir)
	writeFixtureStencil(t, env.StencilsDir, "seatfixture-chair", `{{template "seat-directive-chair"}}`+"\n")
	writeFixtureStencil(t, env.StencilsDir, "seatfixture-advisor", `{{template "seat-directive-advisor"}}`+"\n")

	root := env.WorktreeRoot
	chairOutput := filepath.Join(root, "seats/chair.md")
	fake := &shedfake.SeatRunner{Results: []seatengine.Result{{
		Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		ChairOutputs: []string{chairOutput},
	}}}
	env.Seats = fake

	defs, err := shedbuild.Build(recipe, env)
	if err != nil {
		t.Fatalf("shedbuild.Build() error = %v; want nil", err)
	}
	if len(defs) != 1 || defs[0].Name != "Seat-Step" {
		t.Fatalf("shedbuild.Build() = %d defs; want the one \"Seat-Step\" row", len(defs))
	}

	outcome, pointer, err := defs[0].Producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if pointer.Path != chairOutput {
		t.Errorf("Call() pointer path = %q; want the chair's first output %q", pointer.Path, chairOutput)
	}

	if len(fake.GotTables) != 1 {
		t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
	}
	var seats []seatengine.Seat
	for _, seat := range fake.GotTables[0].Seats {
		seats = append(seats, seatengine.Seat{
			Name: seat.Name, Stencil: seat.Stencil, ModelChoice: seat.ModelChoice,
			Inputs: seat.Inputs, Outputs: seat.Outputs,
		})
	}
	choice := shuttleengine.ModelChoice{Engine: "claude", Model: "claude-opus-test", Effort: "high"}
	want := []seatengine.Seat{
		{
			Stencil: "seatfixture-chair", ModelChoice: choice,
			Inputs:  []string{filepath.Join(root, "in/spec.md"), filepath.Join(root, "seats/advisor-1.md"), filepath.Join(root, "seats/advisor-2.md")},
			Outputs: []string{chairOutput},
		},
		{
			Name: "advisor-1", Stencil: "seatfixture-advisor", ModelChoice: choice,
			Outputs: []string{filepath.Join(root, "seats/advisor-1.md")},
		},
		{
			Name: "advisor-2", Stencil: "seatfixture-advisor", ModelChoice: choice,
			Outputs: []string{filepath.Join(root, "seats/advisor-2.md")},
		},
	}
	if !reflect.DeepEqual(seats, want) {
		t.Errorf("recorded seats = %+v; want %+v", seats, want)
	}

	// A chair whose gate is exhausted keeps its findings in the told file and names it, or names the gate's own path when no file is told.
	exhausted := seatengine.Result{
		Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Attempts: 2, Findings: "the gate's findings", FindingsPath: "/gate/own-findings.md"}},
		ChairOutputs: []string{chairOutput},
	}
	told := env.GateFindingsFile("Seat-Step")
	if reason := callExhaustedChair(t, recipe, env, exhausted); reason != "gate did not pass after 2 attempts; findings: "+told {
		t.Errorf("Call() reason with a told findings file = %q; want it to name %q", reason, told)
	}
	if got, err := os.ReadFile(told); err != nil || string(got) != "the gate's findings" {
		t.Errorf("findings file = %q, %v; want the gate's findings", got, err)
	}
	env.GateFindingsFile = nil
	if reason := callExhaustedChair(t, recipe, env, exhausted); reason != "gate did not pass after 2 attempts; findings: /gate/own-findings.md" {
		t.Errorf("Call() reason with no told findings file = %q; want it to name the gate's own findings path", reason)
	}
}

// callExhaustedChair builds recipe over env with a seat runner answering result, calls its one row and returns the Stuck reason.
func callExhaustedChair(t *testing.T, recipe shedbuild.Recipe, env shedrecipe.Env, result seatengine.Result) string {
	t.Helper()
	env.Seats = &shedfake.SeatRunner{Results: []seatengine.Result{result}}
	defs, err := shedbuild.Build(recipe, env)
	if err != nil {
		t.Fatalf("shedbuild.Build() error = %v; want nil", err)
	}
	outcome, pointer, err := defs[0].Producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck", outcome, err)
	}
	return pointer.Reason
}

// writeFixtureStencil writes body as the stencil name in dir, where stencilstore reads it from.
func writeFixtureStencil(t *testing.T, dir, name, body string) {
	t.Helper()
	path := stencilstore.Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create stencil dir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", name, err)
	}
}
