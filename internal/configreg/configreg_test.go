// configreg_test.go — tests for the module registry.

package configreg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestNames pins the registry's ORDER and shape: every `lyx config` surface (help text, unknown-module error, --print sections, reconcile output, menu numbering) renders it in registry order,
// so an out-of-sort or repeated entry is user-visible, and every module carries a template and a load check.
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
		if module, _ := Lookup(name); module.CheckLoad == nil {
			t.Errorf("Lookup(%q).CheckLoad is nil; want the module's loader", name)
		}
	}
}

// TestModules_SeedOnlyAndHubWideFlags pins the two flags: "models" and "profiles" carry an open-ended, operator-owned key set (model aliases and run profiles respectively), so they are the only entries with SeedOnly == true, "fabric", "board", "gate", "landing", "darn", "language" and "profiles" describe hub-level facts, so they are the only entries with HubWide == true, and "landing", whose per-worktree copy was in effect before it moved hub-wide, is the only entry with SeedsFromPrime == true.
//
//testtiming:keep pins which modules carry the SeedOnly and HubWide flags, which the covering TestFill never asserts
func TestModules_SeedOnlyAndHubWideFlags(t *testing.T) {
	t.Parallel()
	for _, m := range Modules() {
		wantSeedOnly := m.Name == "models" || m.Name == "profiles"
		if m.SeedOnly != wantSeedOnly {
			t.Errorf("Modules(): module %q SeedOnly = %v; want %v", m.Name, m.SeedOnly, wantSeedOnly)
		}
		wantHubWide := m.Name == "fabric" || m.Name == "board" || m.Name == "gate" || m.Name == "landing" || m.Name == "darn" || m.Name == "language" || m.Name == "profiles"
		if m.HubWide != wantHubWide {
			t.Errorf("Modules(): module %q HubWide = %v; want %v", m.Name, m.HubWide, wantHubWide)
		}
		wantSeedsFromPrime := m.Name == "landing"
		if m.SeedsFromPrime != wantSeedsFromPrime {
			t.Errorf("Modules(): module %q SeedsFromPrime = %v; want %v", m.Name, m.SeedsFromPrime, wantSeedsFromPrime)
		}
		spec := m.Spec()
		if spec.Name != m.Name || string(spec.Template) != m.Template() || !slices.Equal(spec.OpenMaps, m.OpenMaps) || !slices.Equal(spec.PerNameMaps, m.PerNameMaps) || spec.HubWide != m.HubWide || spec.SeedOnly != m.SeedOnly {
			t.Errorf("Modules(): module %q Spec() = {Name: %q, OpenMaps: %v, PerNameMaps: %v, HubWide: %v, SeedOnly: %v}; want it to carry the entry's own name, template text, open maps, per-name maps, hub-wide flag and seed-only flag", m.Name, spec.Name, spec.OpenMaps, spec.PerNameMaps, spec.HubWide, spec.SeedOnly)
		}
	}
}

// TestModules_EveryTemplateKeyClassified pins that each module's Categories classify every template unit and name only paths the template holds,
// and that the derived LocalKeys and LocalDenied match the record.
func TestModules_EveryTemplateKeyClassified(t *testing.T) {
	t.Parallel()
	wantLocalKeys := map[string][]string{
		"darn":    {"driver_kind", "pr_review", "tackler", "tackler_max_spawns", "tackler_timeout_min", "verify_attempts", "writer", "writer_timeout_min"},
		"landing": {"conflict", "conflict_timeout_min", "describe", "describe_timeout_min", "fixer", "fixer_timeout_min"},
	}
	wantLocalDenied := map[string][]string{
		"batten":  {"open_ide_command", "open_window_command", "seed_ide_command"},
		"claude":  {"binary", "deny_agent_tool", "deny_ask_user_question"},
		"reed":    {"debug_log", "mouse", "shell", "tmux", "tmux_conf"},
		"shuttle": {"deny_python", "run_dir"},
		"loom":    {"alignment", "code_auto_accept", "code_fixer_fan", "code_reviewer_fan", "discussion_advisors", "discussion_auto_accept", "discussion_fixer_fan", "discussion_lenses", "discussion_reviewer_fan", "parent_review_wait_min", "plan_auto_accept", "plan_fixer_fan", "plan_lenses", "plan_reviewer_fan", "skip_alignment"},
		"lenses":  {"fans", "lens_set", "lenses", "remove"},
	}
	for _, m := range Modules() {
		t.Run(m.Name, func(t *testing.T) {
			t.Parallel()
			spec := m.Spec()
			units, err := yamlengine.LeafPaths([]byte(m.Template()), spec.OpenMaps, spec.WholeEntries)
			if err != nil {
				t.Fatalf("LeafPaths(%s template) error: %v", m.Name, err)
			}
			for _, unit := range units {
				if !coveredByEntry(spec, unit) {
					t.Errorf("template unit %q of %s is not covered by a Categories entry; classify it", unit, m.Name)
				}
			}
			for entry := range spec.Categories {
				if !holdsPath(units, entry) {
					t.Errorf("Categories entry %q of %s names no template path", entry, m.Name)
				}
			}
			if want := wantLocalKeys[m.Name]; m.HubWide && !slices.Equal(m.LocalKeys, want) {
				t.Errorf("%s LocalKeys = %v; want %v", m.Name, m.LocalKeys, want)
			}
			if want := wantLocalDenied[m.Name]; !m.HubWide && !slices.Equal(m.LocalDenied, want) {
				t.Errorf("%s LocalDenied = %v; want %v", m.Name, m.LocalDenied, want)
			}
			if m.HubWide && len(m.LocalDenied) != 0 || !m.HubWide && len(m.LocalKeys) != 0 {
				t.Errorf("%s LocalKeys = %v, LocalDenied = %v; want only the list of its own kind", m.Name, m.LocalKeys, m.LocalDenied)
			}
			if refusal := machineKeyRefusal(m); refusal != "" {
				t.Error(refusal)
			}
			wantMachineKeys := []string{}
			if m.Name == "gate" {
				wantMachineKeys = []string{"slots", "tier_parallel"}
			}
			if got := configengine.MachineKeys(spec); !slices.Equal(got, wantMachineKeys) {
				t.Errorf("%s MachineKeys = %v; want %v, so no stray machine file is read by a registered loader", m.Name, got, wantMachineKeys)
			}
		})
	}

	t.Run("a per-worktree module with a machine key is refused", func(t *testing.T) {
		t.Parallel()
		synthetic := Module{
			Name: "synthetic",
			Spec: func() configengine.Spec {
				return configengine.Spec{
					Name:       "synthetic",
					Categories: map[string]configengine.Category{"limit": configengine.CategoryMachine},
				}
			},
		}
		if machineKeyRefusal(synthetic) == "" {
			t.Error("machineKeyRefusal(per-worktree module with a machine key) = \"\"; want a refusal")
		}
		synthetic.HubWide = true
		if refusal := machineKeyRefusal(synthetic); refusal != "" {
			t.Errorf("machineKeyRefusal(hub-wide module with a machine key) = %q; want none", refusal)
		}
	})
}

// machineKeyRefusal returns the message refusing a per-worktree module that classifies a key CategoryMachine, and "" for any other module.
// Only a hub-wide module reads a machine file, so such a key would be a value no loader reads.
func machineKeyRefusal(m Module) string {
	machineKeys := configengine.MachineKeys(m.Spec())
	if m.HubWide || len(machineKeys) == 0 {
		return ""
	}
	return fmt.Sprintf("module %q is per-worktree but classifies %v as machine keys; only a hub-wide module reads a machine file", m.Name, machineKeys)
}

// coveredByEntry reports whether a Categories entry equals path or is an ancestor of it.
func coveredByEntry(spec configengine.Spec, path string) bool {
	for entry := range spec.Categories {
		if path == entry || strings.HasPrefix(path, entry+".") {
			return true
		}
	}
	return false
}

// holdsPath reports whether some unit equals entry or lies under it.
func holdsPath(units []string, entry string) bool {
	for _, unit := range units {
		if unit == entry || strings.HasPrefix(unit, entry+".") {
			return true
		}
	}
	return false
}

// TestModules_LocalLayerRefusals pins the local-layer verdict of keys no loader card pins through its real load.
func TestModules_LocalLayerRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		module  string
		local   string
		wantKey string
		allowed bool
	}{
		{"darn", "verify_attempts: 2\n", "verify_attempts", true},
		{"landing", "deploy_branches: false\n", "deploy_branches", false},
		{"language", "skills:\n  - scribe:a\n", "skills", false},
		{"reed", "mouse: true\n", "mouse", false},
		{"loom", "discussion_advisors:\n  - name: critic\n    model: sonnet[low]\n", "discussion_advisors", false},
		{"loom", "discussion_lenses: twill-discussion\n", "discussion_lenses", false},
		{"loom", "plan_lenses: twill-plan\n", "plan_lenses", false},
		{"loom", "plan_reviewer_fan: \"\"\n", "plan_reviewer_fan", false},
		{"shuttle", "claude_prompt_cache_ttl_roles:\n  worker: 1h\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.module+"/"+tt.wantKey, func(t *testing.T) {
			t.Parallel()
			m, ok := Lookup(tt.module)
			if !ok {
				t.Fatalf("Lookup(%q) found nothing", tt.module)
			}
			dirs := configengine.Dirs{Anchor: t.TempDir(), Board: t.TempDir()}
			localFile := configengine.LocalFile(dirs.Anchor, tt.module)
			if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(localFile, []byte(tt.local), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			_, err := configengine.Layers(dirs, m.Spec())
			if tt.allowed {
				if err != nil {
					t.Fatalf("Layers() error = %v; want the local override accepted", err)
				}
				return
			}
			if !errors.Is(err, configengine.ErrInvalid) {
				t.Fatalf("Layers() error = %v; want ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), localFile) || !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("Layers() error = %q; want it to name %q and %q", err, localFile, tt.wantKey)
			}
		})
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
		{"batcher", true, nil},
		{"board", true, []string{"types", "labels"}},
		{"loom", true, []string{
			"reviewer_model", "fixer_model",
			"discussion_reviewer_model", "discussion_fixer_model", "plan_reviewer_model", "plan_fixer_model",
			"code_reviewer_model", "code_fixer_model",
			"discussion_reviewer_fan", "plan_reviewer_fan", "code_reviewer_fan",
			"discussion_fixer_fan", "plan_fixer_fan", "code_fixer_fan",
			"discussion_advisors",
		}},
		{"darn", true, []string{"tree_scope"}},
		{"shuttle", true, nil},
		{"burler", false, nil},
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

// TestFingerprint pins what the registry fingerprint is sensitive to: a module's template, flags, open maps, per-name maps, name and overlay documents, and not its Migrate hook or an empty overlay.
func TestFingerprint(t *testing.T) {
	t.Parallel()
	base := func() []Module {
		return []Module{
			{Name: "alpha", Template: func() string { return "a: 1\n" }, OpenMaps: []string{"a"}},
			{Name: "beta", Template: func() string { return "b: 2\n" }},
		}
	}
	withOverlayDocument := func(document string) func(m []Module) {
		return func(m []Module) {
			m[1].Spec = func() configengine.Spec {
				return configengine.Spec{Overlay: configengine.Overlay{Selector: "pick", Documents: map[string][]byte{"x": []byte(document)}}}
			}
		}
	}
	tests := []struct {
		name   string
		mutate func(m []Module)
		same   bool
	}{
		{"identical copy", func(m []Module) {}, true},
		{"a Spec holding no overlay", func(m []Module) {
			m[1].Spec = func() configengine.Spec { return configengine.Spec{} }
		}, true},
		{"a Spec holding an overlay document", withOverlayDocument("a: 1\n"), false},
		{"changed template", func(m []Module) { m[1].Template = func() string { return "b: 3\n" } }, false},
		{"flipped HubWide", func(m []Module) { m[0].HubWide = true }, false},
		{"flipped SeedOnly", func(m []Module) { m[0].SeedOnly = true }, false},
		{"flipped SeedsFromPrime", func(m []Module) { m[0].SeedsFromPrime = true }, false},
		{"added OpenMaps entry", func(m []Module) { m[1].OpenMaps = []string{"b"} }, false},
		{"added PerNameMaps entry", func(m []Module) { m[1].PerNameMaps = []string{"b"} }, false},
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

	one, other := base(), base()
	withOverlayDocument("a: 1\n")(one)
	withOverlayDocument("a: 2\n")(other)
	if fingerprintOf(one) == fingerprintOf(other) {
		t.Errorf("fingerprintOf is equal for overlay documents that differ in bytes; want it to differ")
	}

	if got := Fingerprint(); got != fingerprintOf(Modules()) || got != Fingerprint() {
		t.Errorf("Fingerprint() = %q, want it equal to fingerprintOf(Modules()) and stable across calls", got)
	}
}
