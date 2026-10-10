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

// TestModules_SeedOnlyAndHubWideFlags pins the two flags: "models" and "burler" are the two modules carrying an open-ended, operator-owned key set (model aliases; lenses/fans respectively), so they are the only entries with SeedOnly == true, "fabric", "board", "gate", "landing" and "darn" describe hub-level facts, so they are the only entries with HubWide == true, and "landing", whose per-worktree copy was in effect before it moved hub-wide, is the only entry with SeedsFromPrime == true.
//
//testtiming:keep pins which modules carry the SeedOnly and HubWide flags, which the covering TestFill never asserts
func TestModules_SeedOnlyAndHubWideFlags(t *testing.T) {
	t.Parallel()
	for _, m := range Modules() {
		wantSeedOnly := m.Name == "models" || m.Name == "burler"
		if m.SeedOnly != wantSeedOnly {
			t.Errorf("Modules(): module %q SeedOnly = %v; want %v", m.Name, m.SeedOnly, wantSeedOnly)
		}
		wantHubWide := m.Name == "fabric" || m.Name == "board" || m.Name == "gate" || m.Name == "landing" || m.Name == "darn"
		if m.HubWide != wantHubWide {
			t.Errorf("Modules(): module %q HubWide = %v; want %v", m.Name, m.HubWide, wantHubWide)
		}
		wantSeedsFromPrime := m.Name == "landing"
		if m.SeedsFromPrime != wantSeedsFromPrime {
			t.Errorf("Modules(): module %q SeedsFromPrime = %v; want %v", m.Name, m.SeedsFromPrime, wantSeedsFromPrime)
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
		{"batcher", true, []string{"profiles"}},
		{"board", true, []string{"types", "labels"}},
		{"loom", true, []string{"review", "fix", "discussion_review", "discussion_fix", "plan_review", "plan_fix", "webster_review", "webster_fix", "fan_review", "discussion_advisors"}},
		{"shuttle", true, []string{"claude_prompt_cache_ttl_roles"}},
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

// TestFingerprint pins what the registry fingerprint is sensitive to: a module's template, flags, open maps and name, and not its Migrate hook.
func TestFingerprint(t *testing.T) {
	t.Parallel()
	base := func() []Module {
		return []Module{
			{Name: "alpha", Template: func() string { return "a: 1\n" }, OpenMaps: []string{"a"}},
			{Name: "beta", Template: func() string { return "b: 2\n" }},
		}
	}
	tests := []struct {
		name   string
		mutate func(m []Module)
		same   bool
	}{
		{"identical copy", func(m []Module) {}, true},
		{"changed template", func(m []Module) { m[1].Template = func() string { return "b: 3\n" } }, false},
		{"flipped HubWide", func(m []Module) { m[0].HubWide = true }, false},
		{"flipped SeedOnly", func(m []Module) { m[0].SeedOnly = true }, false},
		{"flipped SeedsFromPrime", func(m []Module) { m[0].SeedsFromPrime = true }, false},
		{"added OpenMaps entry", func(m []Module) { m[1].OpenMaps = []string{"b"} }, false},
		{"renamed module", func(m []Module) { m[0].Name = "gamma" }, false},
		{"swapped Migrate hook", func(m []Module) {
			m[0].Migrate = func(existing []byte) ([]byte, []string, error) { return existing, nil, nil }
		}, true},
	}
	want := fingerprintOf(base())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mods := base()
			tt.mutate(mods)
			if got := fingerprintOf(mods); (got == want) != tt.same {
				t.Errorf("fingerprintOf equals base = %v, want %v", got == want, tt.same)
			}
		})
	}

	if got := Fingerprint(); got != fingerprintOf(Modules()) || got != Fingerprint() {
		t.Errorf("Fingerprint() = %q, want it equal to fingerprintOf(Modules()) and stable across calls", got)
	}
}
