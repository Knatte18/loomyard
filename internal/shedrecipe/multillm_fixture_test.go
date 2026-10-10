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
			Name: seat.Name, Stencil: seat.Stencil, Model: seat.Model, Effort: seat.Effort,
			Inputs: seat.Inputs, Outputs: seat.Outputs,
		})
	}
	want := []seatengine.Seat{
		{
			Name: seatengine.RoleChair, Stencil: "seatfixture-chair", Model: "claude-opus-test", Effort: "high",
			Inputs:  []string{filepath.Join(root, "in/spec.md"), filepath.Join(root, "seats/advisor-1.md"), filepath.Join(root, "seats/advisor-2.md")},
			Outputs: []string{chairOutput},
		},
		{
			Name: seatengine.AdvisorName(1), Stencil: "seatfixture-advisor", Model: "claude-opus-test", Effort: "high",
			Outputs: []string{filepath.Join(root, "seats/advisor-1.md")},
		},
		{
			Name: seatengine.AdvisorName(2), Stencil: "seatfixture-advisor", Model: "claude-opus-test", Effort: "high",
			Outputs: []string{filepath.Join(root, "seats/advisor-2.md")},
		},
	}
	if !reflect.DeepEqual(seats, want) {
		t.Errorf("recorded seats = %+v; want %+v", seats, want)
	}
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
