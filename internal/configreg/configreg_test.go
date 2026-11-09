// configreg_test.go — tests for the module registry.

package configreg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestNames pins the registry's ORDER and shape: every `lym config` surface renders it in registry order,
// so an out-of-sort or repeated entry is user-visible, and every module carries a template and a load check.
// Membership is pinned by TestRegistration_MatchesDeclarers.
func TestNames(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestModules_CreelRegistersTheTrustedHubsReinterpretation pins that the creel module carries the change of trusted_hubs.
// The change goes from shortnames to repository names and rewrites a hub's creel.yaml with no hand step.
func TestModules_CreelRegistersTheTrustedHubsReinterpretation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	module, ok := Lookup("creel")
	if !ok {
		t.Fatal(`Lookup("creel") found no module`)
	}
	if len(module.Reinterpretations) != 1 || module.Reinterpretations[0].ID != creelconfig.TrustedHubsRepositoryReinterpretation || module.Reinterpretations[0].Rewrite == nil {
		t.Errorf("creel Reinterpretations = %+v; want one with ID %q and a rewrite", module.Reinterpretations, creelconfig.TrustedHubsRepositoryReinterpretation)
	}
}

// TestMarkUnresolvable pins which resolve failures drop a trusted_hubs entry and which end the rewrite.
func TestMarkUnresolvable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	unreadable := errors.New("the per-user state directory is unreadable")
	tests := []struct {
		name          string
		err           error
		wantUnmarked  bool
		wantUnchanged bool
	}{
		{"ShortnameNamingNoHub", fmt.Errorf("no hub: %w", hubregistry.ErrUnresolved), false, false},
		{"HubWithNoRepository", fmt.Errorf("no origin: %w", hubregistry.ErrNoRepository), false, false},
		{"UnreadableState", unreadable, true, true},
		{"NoError", nil, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := markUnresolvable(tt.err)
			if marked := errors.Is(got, creelconfig.ErrShortnameUnresolvable); marked == tt.wantUnmarked {
				t.Errorf("markUnresolvable(%v) matches ErrShortnameUnresolvable = %v; want %v", tt.err, marked, !tt.wantUnmarked)
			}
			if tt.wantUnchanged && got != tt.err {
				t.Errorf("markUnresolvable(%v) = %v; want it unchanged", tt.err, got)
			}
			if tt.err != nil && (got == nil || got.Error() != tt.err.Error() || !errors.Is(got, tt.err)) {
				t.Errorf("markUnresolvable(%v) = %v; want the same text and the same chain", tt.err, got)
			}
		})
	}
}

// TestModules_SeedOnlyAndHubWideFlags pins which modules carry the SeedOnly, HubWide and SeedsFromPrime flags.
// Seed-only modules hold an open-ended, operator-owned key set; hub-wide modules describe facts about the hub.
// The test body lists the members of each set.
//
//testtiming:keep pins which modules carry the SeedOnly and HubWide flags, which the covering TestFill never asserts
func TestModules_SeedOnlyAndHubWideFlags(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for _, m := range Modules() {
		wantSeedOnly := m.Name == "models" || m.Name == "profiles"
		if m.SeedOnly != wantSeedOnly {
			t.Errorf("Modules(): module %q SeedOnly = %v; want %v", m.Name, m.SeedOnly, wantSeedOnly)
		}
		wantHubWide := m.Name == "agents" || m.Name == "fabric" || m.Name == "board" || m.Name == "creel" || m.Name == "gate" || m.Name == "heddle" || m.Name == "hibernate" || m.Name == "ide" || m.Name == "landing" || m.Name == "darn" || m.Name == "desktop" || m.Name == "drift" || m.Name == "env" || m.Name == "language" || m.Name == "profiles" || m.Name == "report" || m.Name == "sampler" || m.Name == "slack"
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

// TestModules_EveryTemplateKeyClassified pins that each module's Categories classify every template unit.
// It also pins that Categories name only paths the template holds.
// The derived LocalKeys, LocalDenied and MachineKeys must match the record.
// It is the one test that pins each module's whole key set, so a key nobody classified fails here,
// and each failure names the key and the list or function it goes in.
func TestModules_EveryTemplateKeyClassified(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	wantLocalKeys := map[string][]string{
		"agents":  {"efforts"},
		"darn":    {"gate_fixer", "verify_attempts", "writer", "writer_timeout_min"},
		"drift":   {"enabled", "expect_model", "judge_interactive", "judge_model", "scope"},
		"ide":     {"open_files", "render_markdown"},
		"landing": {"conflict", "conflict_timeout_min", "describe", "describe_timeout_min", "fixer", "fixer_timeout_min", "pr_reviewer"},
		"sampler": {"agent", "code_review"},
		"slack":   {"remind_after_min"},
	}
	wantLocalDenied := map[string][]string{
		"batten":   {"open_ide_command", "open_window_command", "seed_ide_command"},
		"beam":     {"pull_request"},
		"claude":   {"binary", "deny_ask_user_question"},
		"codex":    {"binary", "sandbox_mode", "writable_roots"},
		"reed":     {"buttons", "debug_log", "mouse", "shell", "tmux", "tmux_conf"},
		"shuttle":  {"deny_python", "run_dir"},
		"loom":     {"code_auto_accept", "code_fixer_fan", "discussion_advisors", "discussion_auto_accept", "discussion_fixer_fan", "discussion_lenses", "discussion_reviewer_fan", "split_auto_accept", "split_fixer_fan", "split_reviewer_fan"},
		"tapestry": {"plan_auto_accept", "plan_fixer_fan", "plan_lenses", "plan_reviewer_fan"},
		"lenses":   {"fans", "lens_set", "lenses", "remove"},
	}
	for _, m := range Modules() {
		t.Run(m.Name, func(t *testing.T) {
			t.Parallel()
			spec := m.Spec()
			specFunc := runtime.FuncForPC(reflect.ValueOf(m.Spec).Pointer()).Name()
			units, err := yamlengine.LeafPaths([]byte(m.Template()), spec.OpenMaps, spec.WholeEntries)
			if err != nil {
				t.Fatalf("LeafPaths(%s template) error: %v", m.Name, err)
			}
			for _, unit := range units {
				if !coveredByEntry(spec, unit) {
					t.Errorf("template unit %q of %s is not classified; add %q to the Categories map %s returns, as configengine.CategoryNone to open it to the local layer or as the category that bars it", unit, m.Name, unit, specFunc)
				}
			}
			for entry := range spec.Categories {
				if !holdsPath(units, entry) {
					t.Errorf("Categories entry %q of %s names no template path; drop it from the Categories map %s returns, or add its line to the %s template", entry, m.Name, specFunc, m.Name)
				}
			}
			if want := wantLocalKeys[m.Name]; m.HubWide && !slices.Equal(m.LocalKeys, want) {
				t.Errorf("%s LocalKeys = %v; want %v; %s", m.Name, m.LocalKeys, want, keyListWayForward(fmt.Sprintf("wantLocalKeys[%q]", m.Name), m.LocalKeys, want))
			}
			if want := wantLocalDenied[m.Name]; !m.HubWide && !slices.Equal(m.LocalDenied, want) {
				t.Errorf("%s LocalDenied = %v; want %v; %s", m.Name, m.LocalDenied, want, keyListWayForward(fmt.Sprintf("wantLocalDenied[%q]", m.Name), m.LocalDenied, want))
			}
			if m.HubWide && len(m.LocalDenied) != 0 || !m.HubWide && len(m.LocalKeys) != 0 {
				t.Errorf("%s LocalKeys = %v, LocalDenied = %v; want only the list of its own kind", m.Name, m.LocalKeys, m.LocalDenied)
			}
			wantMachineKeys := []string{}
			if m.Name == "gate" {
				wantMachineKeys = []string{"slots", "tier_parallel"}
			}
			if m.Name == "desktop" {
				wantMachineKeys = []string{"buttons", "window_command", "window_wait_ms"}
			}
			if m.Name == "env" {
				wantMachineKeys = []string{"vars"}
			}
			if m.Name == "codex" {
				wantMachineKeys = []string{"sandbox_mode"}
			}
			if got := configengine.MachineKeys(spec); !slices.Equal(got, wantMachineKeys) {
				t.Errorf("%s MachineKeys = %v; want %v, so no stray machine file is read by a registered loader; %s", m.Name, got, wantMachineKeys, keyListWayForward(fmt.Sprintf("the wantMachineKeys of %s", m.Name), got, wantMachineKeys))
			}
		})
	}
}

// keyListWayForward names the keys to add to and drop from the expected list called list in this test,
// so that it matches got.
// It is the way forward when the classification change behind the mismatch is meant.
func keyListWayForward(list string, got, want []string) string {
	var steps []string
	if added := keysOutside(want, got); len(added) > 0 {
		steps = append(steps, fmt.Sprintf("add %q to %s", added, list))
	}
	if dropped := keysOutside(got, want); len(dropped) > 0 {
		steps = append(steps, fmt.Sprintf("drop %q from %s", dropped, list))
	}
	if len(steps) == 0 {
		return fmt.Sprintf("sort %s in this test", list)
	}
	return "if the classification is meant, " + strings.Join(steps, " and ") + " in this test"
}

// TestKeyListWayForward pins the message that tells which keys to add to or drop from an expected list.
func TestKeyListWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		got  []string
		want []string
		msg  string
	}{
		{"AddOnly", []string{"a", "b"}, []string{"a"}, `if the classification is meant, add ["b"] to wantList in this test`},
		{"DropOnly", []string{"a"}, []string{"a", "b"}, `if the classification is meant, drop ["b"] from wantList in this test`},
		{"AddAndDrop", []string{"a", "c"}, []string{"a", "b"}, `if the classification is meant, add ["c"] to wantList and drop ["b"] from wantList in this test`},
		{"OrderOnly", []string{"b", "a"}, []string{"a", "b"}, "sort wantList in this test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keyListWayForward("wantList", tt.got, tt.want); got != tt.msg {
				t.Errorf("keyListWayForward(%v, %v) = %q; want %q", tt.got, tt.want, got, tt.msg)
			}
		})
	}
}

// keysOutside returns the keys that list does not hold, in their own order.
func keysOutside(list, keys []string) []string {
	var outside []string
	for _, key := range keys {
		if !slices.Contains(list, key) {
			outside = append(outside, key)
		}
	}
	return outside
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

// TestModules_LocalLayerRefusals pins the local-layer verdict of keys that no loader test pins through its real load.
func TestModules_LocalLayerRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		module  string
		local   string
		wantKey string
		allowed bool
	}{
		{"agents", "skills:\n  darn: []\n", "skills", false},
		{"creel", "trusted_hubs:\n  - ly\n", "trusted_hubs", false},
		{"darn", "verify_attempts: 2\n", "verify_attempts", true},
		{"landing", "deploy_branches: false\n", "deploy_branches", false},
		{"language", "skills:\n  - scribe:a\n", "skills", false},
		{"reed", "mouse: true\n", "mouse", false},
		{"loom", "discussion_advisors:\n  - name: critic\n    model: sonnet[low]\n", "discussion_advisors", false},
		{"loom", "discussion_lenses: twill-discussion\n", "discussion_lenses", false},
		{"tapestry", "plan_lenses: twill-plan\n", "plan_lenses", false},
		{"tapestry", "plan_reviewer_fan: \"\"\n", "plan_reviewer_fan", false},
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
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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
			"discussion_reviewer_model", "discussion_fixer_model",
			"code_reviewer_model", "code_fixer_model",
			"split_reviewer_model", "split_fixer_model",
			"discussion_reviewer_fan", "code_reviewer_fan", "split_reviewer_fan",
			"discussion_fixer_fan", "code_fixer_fan", "split_fixer_fan",
			"discussion_assist_reviewer_model", "code_assist_reviewer_model",
			"discussion_assist_reviewer_fan", "code_assist_reviewer_fan",
			"discussion_advisors",
		}},
		{"tapestry", true, []string{"plan_reviewer_model", "plan_fixer_model", "plan_reviewer_fan", "plan_fixer_fan", "plan_assist_reviewer_model", "plan_assist_reviewer_fan"}},
		{"darn", true, []string{"tree_scope"}},
		{"reed", true, []string{"buttons.task", "buttons.beam", "buttons.prime"}},
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

// TestFingerprint pins what the registry fingerprint is sensitive to.
// It changes with a module's template, flags, open maps, per-name maps, name and overlay documents.
// It does not change with its Migrate hook or an empty overlay.
func TestFingerprint(t *testing.T) {
	tiergate.Require(t, 0)
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
