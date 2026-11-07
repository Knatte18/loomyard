// build_engines_test.go holds one test that asserts every name shedrecipe.Names() returns:
// it is buildable from a recipe given a sufficiently filled environment.
// The test is not filesystem-free: the bouncer and burler-round engines create the run directory they resolve,
// and the bouncer engine eagerly reads its configured stencil.
// It covers every engine, including those with the most complex config shapes.

package shedbuild

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// engineMinimalConfig maps each engine name to the minimal Config its row needs to build.
// An engine it omits needs no Config value, so none is given;
// a non-empty block on an engine with no keys is an error from its constructor.
// DiscussionWrite and PlanWrite take their Spec or table as an injected Env closure rather than recipe Config,
// so they have no keys of their own.
// The MultiLLM seats take the "opus" model alias, which envkit.FullEnv's Models carries.
// advisorStencilName and leadStencilName name the MultiLLM advisor's and lead's stencils;
// the lead's includes seat-directive-chair.
func engineMinimalConfig(advisorStencilName, leadStencilName, rubricStencilName string) map[string]map[string]any {
	return map[string]map[string]any{
		"Bouncer": {
			"run_subdir":     "review-segment",
			"artifact_paths": []string{"out/artifact.md"},
			"rubric_stencil": rubricStencilName,
		},
		"BurlerReview": {
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric": "a rubric"},
		},
		"BurlerFix": {
			"run_subdir": "review-segment",
			"review_row": "Row-Burler-Review",
			"profile":    map[string]any{"fix-scope": "source"},
		},
		"Command": {
			"command": "batten.open_window_command",
		},
		"DriftBaseline": {
			"subject": "decision-record",
		},
		"DriftExpect": {
			"subject": "decision-record",
			"gates":   []any{map[string]any{"name": "expectation", "attempts": 3}},
		},
		"DriftVerify": {
			"subject": "decision-record",
			"gates":   []any{map[string]any{"name": "judgement", "attempts": 3}},
		},
		"Implementer": {
			"implementer_setting": "tapestry.implementer",
		},
		"PRReview": {},
		"MultiLLM": {
			"role":    "coverage",
			"segment": "plan",
			"seats": []any{
				map[string]any{
					"stencil": leadStencilName,
					"model":   "opus",
					"inputs":  []string{"out/advisor.md"},
					"outputs": []string{"out/chair.md"},
				},
				map[string]any{
					"name":    "advisor-1",
					"stencil": advisorStencilName,
					"model":   "opus",
					"outputs": []string{"out/advisor.md"},
				},
			},
		},
	}
}

// TestBuild_EveryRegisteredEngineBuilds asserts every name shedrecipe.Names() returns builds from a one-row Recipe.
// It drives the assertion off shedrecipe.Names() rather than a local list,
// so a newly registered engine fails this test until the fixture covers it.
func TestBuild_EveryRegisteredEngineBuilds(t *testing.T) {
	tiergate.Require(t, 0)
	env := envkit.FullEnv(t)

	const advisorStencil = "advisor-coverage"
	const bouncerRubricStencil = "bouncer-coverage"
	const multiLLMLeadStencil = "multillm-lead-coverage"
	stencilkit.SeedInto(t, env.StencilsDir)
	writeStencilFile(t, env.StencilsDir, multiLLMLeadStencil, `{{template "seat-directive-chair"}}`+"\n")
	writeStencilFile(t, env.StencilsDir, advisorStencil, "Body text with no markers.\n")
	writeStencilFile(t, env.StencilsDir, bouncerRubricStencil, "Rubric body with no markers.\n")

	configByEngine := engineMinimalConfig(advisorStencil, multiLLMLeadStencil, bouncerRubricStencil)

	// The three review engines read their segment's settings, which FullEnv wires for Plan-Review.
	reviewSegments := map[string]string{"Bouncer": "Plan-Review", "BurlerReview": "Plan-Review", "BurlerFix": "Plan-Review"}

	for _, name := range shedrecipe.Names() {
		name := name
		t.Run(name, func(t *testing.T) {
			row := Row{Name: "Row-" + name, Engine: name, Segment: reviewSegments[name], Config: configByEngine[name]}
			if name == shedrecipe.EngineBlock {
				row.BlockGate = shedrecipe.BlockGateAlways
			}
			r := Recipe{Producers: []Row{row}}
			defs, err := Build(r, env)
			if err != nil {
				t.Fatalf("Build() for engine %q: error = %v; want nil", name, err)
			}
			if len(defs) != 1 {
				t.Fatalf("Build() for engine %q: returned %d defs; want 1", name, len(defs))
			}
			if defs[0].Producer == nil {
				t.Fatalf("Build() for engine %q: Producer = nil; want non-nil", name)
			}
		})
	}
}
