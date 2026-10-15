// build_engines_test.go holds one test asserting every name shedrecipe.Names() returns is
// buildable from a recipe given a sufficiently filled environment. It is deliberately not
// filesystem-free: the bouncer and burler-round engines create the run directory they resolve, and
// the bouncer engine eagerly reads its configured stencil.
// Restricting the assertion to the effect-free engines was rejected:
// it would drop exactly the engines whose config shapes are the most complex, which is the coverage most worth having.

package shedbuild

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// engineMinimalConfig maps an engine name to its minimal Config, covering only the engines that need one.
// They are Bouncer, BurlerReview, BurlerFix and MultiLLM, whose lead and advisor both take a stencil name and the "opus" alias FullEnv's Models carries.
// Every other engine takes no config at all and are given none in this test,
// since a non-empty config block on any of them is an error from the constructor. All three
// lifecycle engines join them: WorktreeCreate and WorktreeTeardown take no Config keys at
// all, and Loom-Run's own two Config keys (poll_interval_s, poll_attempts) both default, so an
// empty Config block is legal there too.
//
// DiscussionWrite and PlanWrite are two of them: each wraps a producer behind its own commit decorator.
// Its Spec or table arrives as an injected Env closure (DiscussionTable or PlanTable) rather than as recipe Config.
// It therefore has no config keys of its own, and its seams are filled by envkit.FullEnv instead.
// leadStencilName names the MultiLLM lead's stencil, which includes seat-directive-chair.
// advisorStencilName names the MultiLLM advisor's stencil.
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

// TestBuild_EveryRegisteredEngineBuilds asserts every name shedrecipe.Names() returns builds from
// a one-row Recipe against a sufficiently filled shedrecipe.Env, driving the assertion off
// shedrecipe.Names() rather than a local list so a newly registered engine fails this test
// until this fixture covers it.
func TestBuild_EveryRegisteredEngineBuilds(t *testing.T) {
	env := envkit.FullEnv(t)

	const advisorStencil = "advisor-coverage"
	const bouncerRubricStencil = "bouncer-coverage"
	const multiLLMLeadStencil = "multillm-lead-coverage"
	stencilkit.SeedInto(t, env.StencilsDir)
	writeStencilFile(t, env.StencilsDir, multiLLMLeadStencil, `{{template "seat-directive-chair"}}`+"\n")
	writeStencilFile(t, env.StencilsDir, advisorStencil, "Body text with no markers.\n")
	writeStencilFile(t, env.StencilsDir, bouncerRubricStencil, "Rubric body with no markers.\n")

	configByEngine := engineMinimalConfig(advisorStencil, multiLLMLeadStencil, bouncerRubricStencil)
	env.RowBouncerModels = map[string]shedadapters.BouncerModels{"Row-Bouncer": {}}

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
