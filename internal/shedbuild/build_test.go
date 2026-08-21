// build_test.go covers Build's own routing logic against Recipe values constructed in Go rather
// than parsed, so a Build failure can never be a Parse failure in disguise. The filesystem-backed
// twelve-engine coverage lives in build_engines_test.go instead -- see that file's own doc comment.

package shedbuild

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

func TestBuild_SingleStubRow(t *testing.T) {
	r := Recipe{
		Producers: []Row{
			{Name: "Row-1", Engine: "Stub", OnDone: "", OnStuck: "Row-1", Segment: "seg", MaxBounces: 3},
		},
	}

	defs, err := Build(r, shedrecipe.Env{})
	if err != nil {
		t.Fatalf("Build() error = %v; want nil", err)
	}
	if len(defs) != 1 {
		t.Fatalf("Build() returned %d defs; want 1", len(defs))
	}
	def := defs[0]
	if def.Name != "Row-1" {
		t.Errorf("def.Name = %q; want %q", def.Name, "Row-1")
	}
	if def.OnDone != "" {
		t.Errorf("def.OnDone = %q; want \"\"", def.OnDone)
	}
	if def.OnStuck != "Row-1" {
		t.Errorf("def.OnStuck = %q; want %q", def.OnStuck, "Row-1")
	}
	if def.Segment != "seg" {
		t.Errorf("def.Segment = %q; want %q", def.Segment, "seg")
	}
	if def.MaxBounces != 3 {
		t.Errorf("def.MaxBounces = %d; want 3", def.MaxBounces)
	}
	if def.Producer == nil {
		t.Error("def.Producer = nil; want non-nil")
	}
}

func TestBuild_UnknownEngine(t *testing.T) {
	r := Recipe{
		Producers: []Row{
			{Name: "Good", Engine: "Stub"},
			{Name: "Bad-Row", Engine: "NoSuchEngine"},
		},
	}

	_, err := Build(r, shedrecipe.Env{})
	if err == nil {
		t.Fatal("Build() error = nil; want non-nil for an unknown engine")
	}
	for _, want := range []string{"1", "Bad-Row", "NoSuchEngine"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Build() error = %v; want it to contain %q", err, want)
		}
	}
}

func TestBuild_ConstructorErrorWrapsRowIndexAndName(t *testing.T) {
	t.Run("PreflightBlankCwd", func(t *testing.T) {
		r := Recipe{
			Producers: []Row{
				{Name: "Pre", Engine: "Preflight"},
			},
		}
		_, err := Build(r, shedrecipe.Env{})
		if err == nil {
			t.Fatal("Build() error = nil; want non-nil for Preflight with a blank Env.Cwd")
		}
		for _, want := range []string{"0", "Pre", "Cwd"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Build() error = %v; want it to contain %q", err, want)
			}
		}
	})

	t.Run("SingleLLMMissingStencilKey", func(t *testing.T) {
		env := newTestEnv(t)
		r := Recipe{
			Producers: []Row{
				{Name: "LLM-Row", Engine: "SingleLLM", Config: map[string]any{
					"output_files": []string{"out/a.md"},
				}},
			},
		}
		_, err := Build(r, env)
		if err == nil {
			t.Fatal("Build() error = nil; want non-nil for SingleLLM with a missing stencil key")
		}
		for _, want := range []string{"0", "LLM-Row", "stencil"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Build() error = %v; want it to contain %q", err, want)
			}
		}
	})
}

// TestBuild_NonEmptyConfigOnNoConfigEngineErrors proves cfg is forwarded to the constructor rather
// than dropped: a non-empty config block on one of the nine engines that accept no config keys
// errors, because that constructor's own configRejectUnknown call rejects it.
func TestBuild_NonEmptyConfigOnNoConfigEngineErrors(t *testing.T) {
	env := newTestEnv(t)
	r := Recipe{
		Producers: []Row{
			{Name: "Stub-Row", Engine: "Stub", Config: map[string]any{"bogus": "value"}},
		},
	}
	_, err := Build(r, env)
	if err == nil {
		t.Fatal("Build() error = nil; want non-nil for a non-empty config block on Stub")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Build() error = %v; want it to contain %q", err, "bogus")
	}
}

func TestBuild_MultiRowBuildOrderMatchesRecipeOrder(t *testing.T) {
	r := Recipe{
		Producers: []Row{
			{Name: "First", Engine: "Stub"},
			{Name: "Second", Engine: "Stub"},
			{Name: "Third", Engine: "Stub"},
		},
	}

	defs, err := Build(r, shedrecipe.Env{})
	if err != nil {
		t.Fatalf("Build() error = %v; want nil", err)
	}
	want := []string{"First", "Second", "Third"}
	if len(defs) != len(want) {
		t.Fatalf("Build() returned %d defs; want %d", len(defs), len(want))
	}
	for i, name := range want {
		if defs[i].Name != name {
			t.Errorf("defs[%d].Name = %q; want %q", i, defs[i].Name, name)
		}
	}
}

func TestBuild_EmptyProducers(t *testing.T) {
	_, err := Build(Recipe{}, shedrecipe.Env{})
	if err == nil {
		t.Fatal("Build() error = nil; want non-nil for a Recipe with no producers")
	}
	if !strings.Contains(err.Error(), "shedbuild: ") {
		t.Errorf("Build() error = %v; want it to start with the shedbuild: prefix", err)
	}
}
