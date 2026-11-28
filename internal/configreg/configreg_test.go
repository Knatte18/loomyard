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
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestLookup_BeamCarriesThePullRequestModesReinterpretationAfterThePerTaskOne pins a registration of the beam module.
// The reinterpretation of beam.pull_request's renamed on and merge comes after the per-task one.
// Dropping it leaves a hub's beam.yaml holding on or merge unloadable.
//
//testtiming:keep pins the order and shape of the beam module's two Reinterpretations entries, which no walk test reads
func TestLookup_BeamCarriesThePullRequestModesReinterpretationAfterThePerTaskOne(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	module, ok := Lookup("beam")
	if !ok {
		t.Fatal("Lookup(beam) found = false; want true")
	}
	var ids []string
	for _, change := range module.Reinterpretations {
		ids = append(ids, change.ID)
		if change.Rewrite == nil || !change.RunsWithoutFile {
			t.Errorf("reinterpretation %q has Rewrite nil %t, RunsWithoutFile %t; want a Rewrite and RunsWithoutFile set", change.ID, change.Rewrite == nil, change.RunsWithoutFile)
		}
	}
	want := []string{"beam-per-task-settings", "beam-pull-request-modes"}
	if !slices.Equal(ids, want) {
		t.Errorf("Lookup(beam).Reinterpretations IDs = %q; want %q", ids, want)
	}
}

// TestLookup_ProfilesCarriesTheJacquardBlocksReinterpretationAfterTheArrasOne pins the profiles reinterpretations.
// The jacquard blocks' carry runs last, so it judges the defaults the arras drop left.
// Dropping it gives a tuned profile's jacquard runs the template's values instead of the hub's tapestry ones.
//
//testtiming:keep pins the order of the profiles module's Reinterpretations entries, which no walk test reads
func TestLookup_ProfilesCarriesTheJacquardBlocksReinterpretationAfterTheArrasOne(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	module, ok := Lookup("profiles")
	if !ok {
		t.Fatal("Lookup(profiles) found = false; want true")
	}
	var ids []string
	for _, change := range module.Reinterpretations {
		ids = append(ids, change.ID)
		if change.Rewrite == nil {
			t.Errorf("reinterpretation %q has no Rewrite; want one", change.ID)
		}
	}
	want := []string{"profiles-pull-request-modes", "profiles-arras-default-dropped", "profiles-jacquard-blocks"}
	if !slices.Equal(ids, want) {
		t.Errorf("Lookup(profiles).Reinterpretations IDs = %q; want %q", ids, want)
	}
}

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
		wantHubWide := m.Name == "agents" || m.Name == "fabric" || m.Name == "board" || m.Name == "creel" || m.Name == "gate" || m.Name == "heddle" || m.Name == "hibernate" || m.Name == "ide" || m.Name == "landing" || m.Name == "darn" || m.Name == "desktop" || m.Name == "drift" || m.Name == "env" || m.Name == "hardener" || m.Name == "language" || m.Name == "profiles" || m.Name == "report" || m.Name == "sampler" || m.Name == "slack"
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
	for _, engine := range shuttleengines.ConfigModules() {
		if _, found := Lookup(engine.Name); !found {
			t.Errorf("Lookup(%q) found no module; want every shuttle engine's config module registered", engine.Name)
		}
	}
}

// TestModules_OrchRegistersRemoteControlReinterpretation pins the orch module's remote-control reinterpretation.
// Every hub gets the notice that its orch's remote control is now off only while the orch row declares it.
//
//testtiming:keep pins the orch row's Reinterpretations entry, which no walk test reads
func TestModules_OrchRegistersRemoteControlReinterpretation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	orch, found := Lookup("orch")
	if !found {
		t.Fatal("Lookup(\"orch\") found no module; want the orch module registered")
	}
	if len(orch.Reinterpretations) != 1 {
		t.Fatalf("orch Reinterpretations = %d entries; want 1", len(orch.Reinterpretations))
	}
	if got := orch.Reinterpretations[0].ID; got != "orch-remote-control-off-by-default" {
		t.Errorf("orch Reinterpretations[0].ID = %q; want %q", got, "orch-remote-control-off-by-default")
	}
	if orch.Reinterpretations[0].Rewrite == nil {
		t.Error("orch Reinterpretations[0].Rewrite is nil; want the rewrite function")
	}
}

// TestModules_LoomRegistersCodeAutoAcceptReinterpretation pins the loom module's loom.code.auto_accept reinterpretation.
// A hub that sets loom.code.auto_accept: true gets the notice that an open BLOCKING finding now escalates.
// It does so only while the row declares the reinterpretation.
//
//testtiming:keep pins the loom row's Reinterpretations entry, which no walk test reads
func TestModules_LoomRegistersCodeAutoAcceptReinterpretation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	loom, found := Lookup("loom")
	if !found {
		t.Fatal("Lookup(\"loom\") found no module; want the loom module registered")
	}
	if len(loom.Reinterpretations) != 1 {
		t.Fatalf("loom Reinterpretations = %d entries; want 1", len(loom.Reinterpretations))
	}
	if got := loom.Reinterpretations[0].ID; got != "loom-code-auto-accept-escalates-blocking" {
		t.Errorf("loom Reinterpretations[0].ID = %q; want %q", got, "loom-code-auto-accept-escalates-blocking")
	}
	if loom.Reinterpretations[0].Rewrite == nil {
		t.Error("loom Reinterpretations[0].Rewrite is nil; want the rewrite function")
	}
}

// TestModules_EveryTemplateKeyClassified pins that each module's Categories classify every template unit.
// It also pins that Categories name only paths the template holds.
// The derived LocalKeys, LocalDenied and MachineKeys must match the record.
// It is the one test that pins each module's whole key set, so a key nobody classified fails here,
// and each failure names the key and the list or function it goes in.
//
//testtiming:keep pins the classification of every module's whole key set, which TestPrepareRunSettings never asserts
func TestModules_EveryTemplateKeyClassified(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	wantLocalKeys := map[string][]string{
		"agents":   {"efforts"},
		"darn":     {"gate_fixer", "report_dir", "verify_attempts", "verify_budget_min", "writer", "writer_timeout_min"},
		"drift":    {"enabled", "expect_model", "judge_ask_parent", "judge_interactive", "judge_model", "judge_parent_timeout_min", "scope"},
		"hardener": {"concurrency", "fixer", "hard_max_bounces", "judge", "live_attempts", "llm_driving", "max_bounces", "packages", "reviewer", "seed", "verify_attempts"},
		"ide":      {"open_files", "render_markdown"},
		"landing":  {"conflict", "conflict_timeout_min", "describe", "describe_review", "describe_review_timeout_min", "describe_timeout_min", "fixer", "fixer_timeout_min", "pr_reviewer"},
		"sampler":  {"agent", "code_review"},
		"slack":    {"remind_after_min"},
	}
	wantLocalDenied := map[string][]string{
		"batten":   {"open_ide_command", "open_window_command", "seed_ide_command"},
		"claude":   {"account_pins", "accounts", "binary", "deny_ask_user_question"},
		"codex":    {"binary", "sandbox_mode", "writable_roots"},
		"reed":     {"buttons", "debug_log", "mouse", "shell", "tmux", "tmux_conf"},
		"shuttle":  {"deny_python", "run_dir"},
		"loom":     {"code.auto_accept", "code.fixer_fan", "discussion.advisors", "discussion.auto_accept", "discussion.fixer_fan", "discussion.lenses", "discussion.reviewer_fan", "split.auto_accept", "split.fixer_fan", "split.reviewer_fan"},
		"tapestry": {"plan.auto_accept", "plan.fixer_fan", "plan.lenses", "plan.reviewer_fan"},
		"lenses":   {"fans", "lens_set", "lenses", "remove"},
		"weaver":   {"verify_gate"},
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
			if m.Name == "claude" {
				wantMachineKeys = []string{"account_pins", "accounts"}
			}
			if m.Name == "codex" {
				wantMachineKeys = []string{"sandbox_mode"}
			}
			if m.Name == "fabric" {
				wantMachineKeys = []string{"branch_prefix", "push_ceiling_s", "push_interval_s", "push_notice_after_min"}
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
//
//testtiming:keep pins per-module local-layer verdicts and the refusal naming file and key, which TestLayerRunSettings never asserts
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
		{"loom", "discussion:\n  advisors:\n    - name: critic\n      model: sonnet[low]\n", "discussion.advisors", false},
		{"loom", "discussion: {lenses: twill-discussion}\n", "discussion.lenses", false},
		{"tapestry", "plan: {lenses: twill-plan}\n", "plan.lenses", false},
		{"tapestry", "plan: {reviewer_fan: \"\"}\n", "plan.reviewer_fan", false},
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

//testtiming:keep pins each module's registered OpenMaps and found flag, which TestPrepareRunSettings never asserts
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
			"review.reviewer_model", "review.fixer_model",
			"discussion.reviewer_model", "discussion.fixer_model",
			"code.reviewer_model", "code.fixer_model",
			"split.reviewer_model", "split.fixer_model",
			"discussion.reviewer_fan", "code.reviewer_fan", "split.reviewer_fan",
			"discussion.fixer_fan", "code.fixer_fan", "split.fixer_fan",
			"discussion.assist_reviewer_model", "code.assist_reviewer_model",
			"discussion.assist_reviewer_fan", "code.assist_reviewer_fan",
			"discussion.advisors",
		}},
		{"tapestry", true, []string{"plan.reviewer_model", "plan.fixer_model", "plan.reviewer_fan", "plan.fixer_fan", "plan.assist_reviewer_model", "plan.assist_reviewer_fan"}},
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
// It does not change with its CheckLoad hook or an empty overlay.
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
		{"swapped CheckLoad hook", func(m []Module) {
			m[0].CheckLoad = func(configengine.Dirs) error { return nil }
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
