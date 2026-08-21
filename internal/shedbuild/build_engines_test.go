// build_engines_test.go holds one test asserting every name shedrecipe.Names() returns is
// buildable from a recipe given a sufficiently filled environment. It is deliberately not
// filesystem-free: the bouncer and burler-round engines create the run directory they resolve, and
// the bouncer and single-LLM engines eagerly read their configured stencil. Restricting the
// assertion to the nine effect-free engines was rejected -- it would drop exactly the three
// engines whose config shapes are the most complex, which is the coverage most worth having.

package shedbuild

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// engineMinimalConfig maps an engine name to its minimal Config, covering only the three engines
// that need one. The other nine engines take no config at all and are given none in this test,
// since a non-empty config block on any of them is an error from the constructor.
func engineMinimalConfig(stencilName, rubricStencilName string) map[string]map[string]any {
	return map[string]map[string]any{
		"SingleLLM": {
			"stencil":      stencilName,
			"output_files": []string{"out/singlellm.md"},
		},
		"Bouncer": {
			"run_subdir":     "review-segment",
			"artifact_paths": []string{"out/artifact.md"},
			"rubric_stencil": rubricStencilName,
		},
		"BurlerRound": {
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric": "a rubric"},
		},
	}
}

// TestBuild_EveryRegisteredEngineBuilds asserts every name shedrecipe.Names() returns builds from
// a one-row Recipe against a sufficiently filled shedrecipe.Env, driving the assertion off
// shedrecipe.Names() rather than a local list so a thirteenth registered engine fails this test
// until this fixture covers it.
func TestBuild_EveryRegisteredEngineBuilds(t *testing.T) {
	env := newTestEnv(t)

	const singleLLMStencil = "singlellm-coverage"
	const bouncerRubricStencil = "bouncer-coverage"
	writeStencilFile(t, env.StencilsDir, singleLLMStencil, "Body text with no markers.\n")
	writeStencilFile(t, env.StencilsDir, bouncerRubricStencil, "Rubric body with no markers.\n")

	configByEngine := engineMinimalConfig(singleLLMStencil, bouncerRubricStencil)

	for _, name := range shedrecipe.Names() {
		name := name
		t.Run(name, func(t *testing.T) {
			r := Recipe{
				Producers: []Row{
					{Name: "Row-" + name, Engine: name, Config: configByEngine[name]},
				},
			}
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
