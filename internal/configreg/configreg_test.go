// configreg_test.go — tests for the module registry.

package configreg

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestNames pins the registry's ORDER and shape: every `lyx config` surface (help text,
// unknown-module error, --print sections, reconcile output, menu numbering) renders it in
// registry order, so an out-of-sort or repeated entry is user-visible.
// Membership is pinned by TestRegistration_MatchesDeclarers.
func TestNames(t *testing.T) {
	got := Names()
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Names() = %v; want strictly ascending, got %q before %q", got, got[i-1], got[i])
		}
	}
	for _, name := range got {
		tmpl, ok := Template(name)
		if !ok || tmpl == nil {
			t.Errorf("Template(%q) = _, %v; want a template function", name, ok)
			continue
		}
		if tmpl() == "" {
			t.Errorf("Template(%q)() is empty; want a non-empty template", name)
		}
	}
}

// TestModules_SeedOnly pins the seed-only flag: "models" and "burler" are the two modules carrying
// an open-ended, operator-owned key set (model aliases; lenses/fans respectively), so they are the
// only entries with SeedOnly == true.
func TestModules_SeedOnly(t *testing.T) {
	for _, m := range Modules() {
		want := m.Name == "models" || m.Name == "burler"
		if m.SeedOnly != want {
			t.Errorf("Modules(): module %q SeedOnly = %v; want %v", m.Name, m.SeedOnly, want)
		}
	}
}

func TestTemplate_Found(t *testing.T) {
	got, ok := Template("fabric")
	if !ok {
		t.Error("Template(\"fabric\") = _, false; want _, true")
		return
	}
	if got == nil {
		t.Error("Template(\"fabric\") returned nil function; want non-nil")
		return
	}
	// Verify the template function returns the expected content.
	want := fabricengine.ConfigTemplate()
	if got() != want {
		t.Errorf("Template(\"fabric\")() = %q; want %q", got(), want)
	}
}

func TestTemplate_NotFound(t *testing.T) {
	_, ok := Template("nope")
	if ok {
		t.Error("Template(\"nope\") = _, true; want _, false")
	}
}
