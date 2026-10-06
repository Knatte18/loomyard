// configreg_test.go — tests for the module registry.

package configreg

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestNames pins the registry's ORDER and shape: every `lyx config` surface (help text, unknown-module error, --print sections, reconcile output, menu numbering) renders it in registry order,
// so an out-of-sort or repeated entry is user-visible.
// Membership is pinned by TestRegistration_MatchesDeclarers.
func TestNames(t *testing.T) {
	t.Parallel()
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

// TestModules_SeedOnlyAndHubWideFlags pins the two flags: "models" and "burler" are the two modules
// carrying an open-ended, operator-owned key set (model aliases; lenses/fans respectively), so they
// are the only entries with SeedOnly == true, and "fabric" and "board" describe hub-level facts, so
// they are the only entries with HubWide == true.
//
//testtiming:keep pins which modules carry the SeedOnly and HubWide flags, which the covering TestFill never asserts
func TestModules_SeedOnlyAndHubWideFlags(t *testing.T) {
	t.Parallel()
	for _, m := range Modules() {
		wantSeedOnly := m.Name == "models" || m.Name == "burler"
		if m.SeedOnly != wantSeedOnly {
			t.Errorf("Modules(): module %q SeedOnly = %v; want %v", m.Name, m.SeedOnly, wantSeedOnly)
		}
		wantHubWide := m.Name == "fabric" || m.Name == "board"
		if m.HubWide != wantHubWide {
			t.Errorf("Modules(): module %q HubWide = %v; want %v", m.Name, m.HubWide, wantHubWide)
		}
	}
}

func TestTemplate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		module    string
		wantFound bool
	}{
		{"fabric", true},
		{"nope", false},
	}
	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			t.Parallel()
			got, ok := Template(tt.module)
			if ok != tt.wantFound {
				t.Fatalf("Template(%q) found = %v; want %v", tt.module, ok, tt.wantFound)
			}
			if !ok {
				return
			}
			if got == nil {
				t.Fatalf("Template(%q) returned nil function; want non-nil", tt.module)
			}
			if want := fabricengine.ConfigTemplate(); got() != want {
				t.Errorf("Template(%q)() = %q; want %q", tt.module, got(), want)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		module       string
		wantFound    bool
		wantOpenMaps []string
	}{
		{"board", true, []string{"types", "labels"}},
		{"bogus", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			t.Parallel()
			m, ok := Lookup(tt.module)
			if ok != tt.wantFound {
				t.Fatalf("Lookup(%q) found = %v; want %v", tt.module, ok, tt.wantFound)
			}
			if !ok {
				return
			}
			if m.Template == nil {
				t.Errorf("Lookup(%q).Template is nil; want the module's template", tt.module)
			}
			if !slices.Equal(m.OpenMaps, tt.wantOpenMaps) {
				t.Errorf("Lookup(%q).OpenMaps = %v; want %v", tt.module, m.OpenMaps, tt.wantOpenMaps)
			}
		})
	}
}
