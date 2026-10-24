// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// writeModuleFile writes content as module's config file under dir, creating the config directory.
// It writes nothing for empty content.
func writeModuleFile(t *testing.T, dir, module, content string) {
	t.Helper()
	if content == "" {
		return
	}
	if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(dir, module), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s.yaml: %v", module, err)
	}
}

// lensesFixture returns a lenses.yaml document: the template decoded, changed by edit and marshalled back.
func lensesFixture(t *testing.T, edit func(cfg *lenses.Config)) string {
	t.Helper()
	var cfg lenses.Config
	if err := yaml.Unmarshal([]byte(lenses.ConfigTemplate()), &cfg); err != nil {
		t.Fatalf("decode the lenses template: %v", err)
	}
	edit(&cfg)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal the lenses fixture: %v", err)
	}
	return string(out)
}

// burlerFixture returns a burler.yaml document: the template's lenses and fans decoded, changed by edit and marshalled back holding only those two keys.
// Every key of such a document is one the burler-to-lenses moves take out, so the move leaves it empty.
func burlerFixture(t *testing.T, edit func(cfg *lenses.Config)) string {
	t.Helper()
	var cfg lenses.Config
	if err := yaml.Unmarshal([]byte(lenses.ConfigTemplate()), &cfg); err != nil {
		t.Fatalf("decode the lenses template: %v", err)
	}
	edit(&cfg)
	out, err := yaml.Marshal(struct {
		Lenses map[string]string   `yaml:"lenses"`
		Fans   map[string][]string `yaml:"fans"`
	}{cfg.Lenses, cfg.Fans})
	if err != nil {
		t.Fatalf("marshal the burler fixture: %v", err)
	}
	return string(out)
}

// loadLensesWith returns lenses.Load over a scratch anchor whose lenses.yaml holds content, none when content is empty.
func loadLensesWith(t *testing.T, content string) lenses.Config {
	t.Helper()
	anchor := t.TempDir()
	writeModuleFile(t, anchor, "lenses", content)
	cfg, err := lenses.Load(configengine.Dirs{Anchor: anchor})
	if err != nil {
		t.Fatalf("lenses.Load over %q: %v", content, err)
	}
	return cfg
}

// decodeDocument decodes a YAML document into generic values, so two documents compare regardless of key order.
func decodeDocument(t *testing.T, data []byte) any {
	t.Helper()
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode a resolved document: %v", err)
	}
	return document
}

// loadLensesAt returns lenses.Load over dirs, failing the test on an error.
func loadLensesAt(t *testing.T, dirs configengine.Dirs) lenses.Config {
	t.Helper()
	cfg, err := lenses.Load(dirs)
	if err != nil {
		t.Fatalf("lenses.Load(%+v): %v", dirs, err)
	}
	return cfg
}

// lensSetFixture returns a sparse lenses.yaml document that picks lensSet, none when it is empty, and holds the given lens entries.
func lensSetFixture(t *testing.T, lensSet string, entries map[string]string) string {
	t.Helper()
	out, err := yaml.Marshal(struct {
		LensSet string            `yaml:"lens_set,omitempty"`
		Lenses  map[string]string `yaml:"lenses"`
	}{lensSet, entries})
	if err != nil {
		t.Fatalf("marshal the lens-set fixture: %v", err)
	}
	return string(out)
}

// moduleWant is what one ReconcileAll row expects of one module's Result and config file.
type moduleWant struct {
	applied bool
	// addedAny and removedNone pin that the Result reports some added keys and no removed ones.
	addedAny    bool
	removedNone bool
	// addedNone pins that the Result reports no added keys.
	addedNone  bool
	addedAll   []string
	removedAll []string
	// wantKeptExtra is the exact Result.KeptExtra; nil skips the check.
	wantKeptExtra []string
	// droppedAll lists names Result.Dropped must hold.
	droppedAll []string
	// wantFile is the file's exact content after the run; nil skips the check.
	wantFile        *string
	fileContains    []string
	fileNotContains []string
	// migratedAll lists the rewrites Result.Migrated must hold; migratedNone pins it empty.
	migratedAll  []string
	migratedNone bool
	// loadsAsBatcher pins that batcher.Active loads the file after the run.
	loadsAsBatcher bool
	// wantLoadedLenses is what lenses.Load resolves after the run; nil skips the check.
	wantLoadedLenses *lenses.Config
	// loadUnchanged pins that lenses.Load over the base dir resolves the same before and after the run.
	loadUnchanged bool
}

// TestReconcileAll pins what a run does to each module's file and Result: a dry run reports and writes nothing, an apply adds the template's missing keys and prunes stale ones, an absent file is seeded from the template, and a seed-only module ("models" today) is materialized verbatim exactly once and never rewritten after -- not even to resurrect a key the operator removed -- while a non-seed-only module in the same run is still pruned.
func TestReconcileAll(t *testing.T) {
	t.Parallel()
	const loomStale = "discussion_timeout_min: 480\nstale_key: old_value\n"
	const modelsAlias = "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
	const modelsTrimmed = "sonnet:\n  engine: claude\n  model: sonnet\n"
	loggerTemplate := loggerconfig.ConfigTemplate()
	modelsTemplate := modelspec.ConfigTemplate()
	batcherTemplate := batcher.ConfigTemplate()
	retiredBatcher := strings.Replace(batcherTemplate, "orientation: 31400", "master_base: 52000", 1)
	masterBaseRewrites := []string{"profiles.cautious.weights.master_base: removed", "profiles.cautious.weights.orientation: added 31400"}
	lensesLacksSecurity := lensesFixture(t, func(cfg *lenses.Config) {
		delete(cfg.Lenses, "security")
		cfg.Lenses["hub-lens"] = "Hub emphasis.\n"
	})
	lensesWithHubLens := lensesFixture(t, func(cfg *lenses.Config) {
		cfg.Lenses["hub-lens"] = "Hub emphasis.\n"
	})
	var hubLensConfig lenses.Config
	if err := yaml.Unmarshal([]byte(lensesWithHubLens), &hubLensConfig); err != nil {
		t.Fatalf("decode the hub-lens fixture: %v", err)
	}
	csharpLenses, baseLenses := loadLensesWith(t, "lens_set: csharp\n").Lenses, loadLensesWith(t, "").Lenses
	lensesCSharpPick := lensSetFixture(t, "csharp", map[string]string{
		"concurrency":    csharpLenses["concurrency"],
		"error-handling": baseLenses["error-handling"],
	})

	tests := []struct {
		name  string
		seed  map[string]string
		apply bool
		want  map[string]moduleWant
	}{
		{
			name:  "dry run reports missing and stale keys and writes nothing",
			seed:  map[string]string{"loom": loomStale},
			apply: false,
			want: map[string]moduleWant{
				"loom": {applied: false, addedAny: true, removedAll: []string{"stale_key"}, wantFile: ptr(loomStale)},
			},
		},
		{
			name:  "apply rewrites a stale file and leaves a present seed-only file alone",
			seed:  map[string]string{"loom": loomStale, "models": modelsAlias},
			apply: true,
			want: map[string]moduleWant{
				"loom": {
					applied:         true,
					removedAll:      []string{"stale_key"},
					fileContains:    []string{"review_max_bounces:"},
					fileNotContains: []string{"stale_key"},
				},
				"models": {applied: false, removedNone: true, wantFile: ptr(modelsAlias)},
			},
		},
		{
			name:  "absent logger.yaml is seeded from the template",
			apply: true,
			want:  map[string]moduleWant{"logger": {applied: true, wantFile: &loggerTemplate}},
		},
		{
			name:  "stale reed claude key is reconciled away",
			seed:  map[string]string{"reed": "tmux: C:\\tools\\tmux.exe\nclaude: C:\\tools\\claude.exe\n"},
			apply: true,
			want: map[string]moduleWant{
				"reed": {applied: true, removedAll: []string{"claude"}, fileNotContains: []string{"claude:"}},
			},
		},
		{
			name:  "stale reed header block is removed",
			seed:  map[string]string{"reed": "tmux: C:\\tools\\tmux.exe\nheader:\n  template: \"\"\n  height_rows: 1\n"},
			apply: true,
			want: map[string]moduleWant{
				"reed": {
					applied:         true,
					removedAll:      []string{"header.template", "header.height_rows"},
					fileNotContains: []string{"header:"},
				},
			},
		},
		{
			name:  "retired reed layout keys are reconciled away",
			seed:  map[string]string{"reed": "tmux: C:\\tools\\tmux.exe\ncollapsed_rows: 3\nmin_full_rows: 3\nselvage:\n  height_rows: 1\n"},
			apply: true,
			want: map[string]moduleWant{
				"reed": {
					applied:         true,
					removedAll:      []string{"collapsed_rows", "min_full_rows", "selvage.height_rows"},
					fileNotContains: []string{"collapsed_rows:", "min_full_rows:", "selvage:"},
				},
			},
		},
		{
			name:  "dry run lists a batcher file's retired-key rewrites and writes nothing",
			seed:  map[string]string{"batcher": retiredBatcher},
			apply: false,
			want:  map[string]moduleWant{"batcher": {applied: false, migratedAll: masterBaseRewrites, wantFile: &retiredBatcher}},
		},
		{
			name:  "apply writes a migrated batcher file that batcher.Active loads",
			seed:  map[string]string{"batcher": retiredBatcher},
			apply: true,
			want: map[string]moduleWant{"batcher": {
				applied:         true,
				migratedAll:     masterBaseRewrites,
				fileContains:    []string{"profiles: {}"},
				fileNotContains: []string{"master_base"},
				loadsAsBatcher:  true,
			}},
		},
		{
			name:  "absent batcher file is seeded from the template and not migrated",
			apply: true,
			want:  map[string]moduleWant{"batcher": {applied: true, migratedNone: true, fileContains: []string{"profiles: {}"}, loadsAsBatcher: true}},
		},
		{
			name:  "absent seed-only file is materialized verbatim",
			apply: true,
			want:  map[string]moduleWant{"models": {applied: true, addedAny: true, removedNone: true, wantFile: &modelsTemplate}},
		},
		{
			name:  "present seed-only file with an operator alias is untouched",
			seed:  map[string]string{"models": modelsAlias},
			apply: true,
			want:  map[string]moduleWant{"models": {applied: false, removedNone: true, wantFile: ptr(modelsAlias)}},
		},
		{
			name:  "present seed-only file with a template key removed is not resurrected",
			seed:  map[string]string{"models": modelsTrimmed},
			apply: true,
			want:  map[string]moduleWant{"models": {applied: false, wantFile: ptr(modelsTrimmed)}},
		},
		{
			name:  "dry run reports template-equal entries dropped and a hub-only lens kept, adds no missing lens and writes nothing",
			seed:  map[string]string{"lenses": lensesLacksSecurity},
			apply: false,
			want: map[string]moduleWant{"lenses": {
				addedNone:     true,
				removedNone:   true,
				wantKeptExtra: []string{"lenses.hub-lens"},
				droppedAll:    []string{"fans.standard", "lenses.generic"},
				wantFile:      &lensesLacksSecurity,
			}},
		},
		{
			name:  "apply over the template plus a hub-only lens keeps only the hub-only lens",
			seed:  map[string]string{"lenses": lensesWithHubLens},
			apply: true,
			want: map[string]moduleWant{"lenses": {
				applied:          true,
				addedNone:        true,
				removedNone:      true,
				wantKeptExtra:    []string{"lenses.hub-lens"},
				droppedAll:       []string{"fans.standard", "lenses.generic", "lenses.security"},
				fileContains:     []string{"lenses:\n    hub-lens: |\n        Hub emphasis.\n", "fans: {}"},
				fileNotContains:  []string{"Review broadly with no single area of emphasis"},
				wantLoadedLenses: &hubLensConfig,
			}},
		},
		{
			name:  "apply under a standalone lens_set drops a lens equal to the layer's prose and keeps one equal to the base's",
			seed:  map[string]string{"lenses": lensesCSharpPick},
			apply: true,
			want: map[string]moduleWant{"lenses": {
				applied:         true,
				droppedAll:      []string{"lenses.concurrency"},
				fileContains:    []string{"error-handling:", "lens_set: csharp"},
				fileNotContains: []string{"concurrency:"},
				loadUnchanged:   true,
			}},
		},
	}

	// Each floored module's below-floor key: a dry run reports the raise and writes nothing, an apply writes the key at its floor.
	for _, floored := range []struct{ module, seed, line, raised, stale string }{
		{"shuttle", "poll_interval_ms: 500\n", "poll_interval_ms: raised from 500 to 1000", "poll_interval_ms: 1000", "poll_interval_ms: 500"},
		{"orch", "poll_interval_ms: 500\n", "poll_interval_ms: raised from 500 to 1000", "poll_interval_ms: 1000", "poll_interval_ms: 500"},
		{"loom", "step_idle_timeout_min: 0\n", "step_idle_timeout_min: raised from 0 to 1", "step_idle_timeout_min: 1", "step_idle_timeout_min: 0"},
	} {
		tests = append(tests,
			struct {
				name  string
				seed  map[string]string
				apply bool
				want  map[string]moduleWant
			}{
				name: "dry run reports a below-floor " + floored.module + " key and writes nothing",
				seed: map[string]string{floored.module: floored.seed},
				want: map[string]moduleWant{floored.module: {migratedAll: []string{floored.line}, wantFile: ptr(floored.seed)}},
			},
			struct {
				name  string
				seed  map[string]string
				apply bool
				want  map[string]moduleWant
			}{
				name:  "apply writes a below-floor " + floored.module + " key at its floor",
				seed:  map[string]string{floored.module: floored.seed},
				apply: true,
				want: map[string]moduleWant{floored.module: {
					applied:         true,
					migratedAll:     []string{floored.line},
					fileContains:    []string{floored.raised},
					fileNotContains: []string{floored.stale},
				}},
			},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			for module, content := range tt.seed {
				writeModuleFile(t, baseDir, module, content)
			}

			var loadedBefore lenses.Config
			if tt.want["lenses"].loadUnchanged {
				loadedBefore = loadLensesAt(t, configengine.Dirs{Anchor: baseDir})
			}

			results, err := ReconcileAll(baseDir, "", Options{Apply: tt.apply})
			if err != nil {
				t.Fatalf("ReconcileAll(%v): %v", tt.apply, err)
			}

			for module, want := range tt.want {
				if want.loadUnchanged {
					if loadedAfter := loadLensesAt(t, configengine.Dirs{Anchor: baseDir}); !reflect.DeepEqual(loadedAfter, loadedBefore) {
						t.Errorf("lenses.Load after the run differs from before: got %+v; want %+v", loadedAfter, loadedBefore)
					}
				}
				result := findResult(results, module)
				if result == nil {
					t.Errorf("%s result not found", module)
					continue
				}
				if result.Applied != want.applied {
					t.Errorf("%s.Applied = %v; want %v", module, result.Applied, want.applied)
				}
				if want.addedAny && len(result.Added) == 0 {
					t.Errorf("%s.Added is empty; want the template's missing keys", module)
				}
				if want.removedNone && len(result.Removed) != 0 {
					t.Errorf("%s.Removed = %v; want empty", module, result.Removed)
				}
				if want.addedNone && len(result.Added) != 0 {
					t.Errorf("%s.Added = %v; want empty", module, result.Added)
				}
				if want.wantKeptExtra != nil && !slices.Equal(result.KeptExtra, want.wantKeptExtra) {
					t.Errorf("%s.KeptExtra = %v; want %v", module, result.KeptExtra, want.wantKeptExtra)
				}
				for _, name := range want.droppedAll {
					if !slices.Contains(result.Dropped, name) {
						t.Errorf("%s.Dropped = %v; want it to contain %q", module, result.Dropped, name)
					}
				}
				for _, key := range want.addedAll {
					if !slices.Contains(result.Added, key) {
						t.Errorf("%s.Added = %v; want it to contain %q", module, result.Added, key)
					}
				}
				for _, key := range want.removedAll {
					if !slices.Contains(result.Removed, key) {
						t.Errorf("%s.Removed = %v; want it to contain %q", module, result.Removed, key)
					}
				}
				if want.migratedNone && len(result.Migrated) != 0 {
					t.Errorf("%s.Migrated = %v; want empty", module, result.Migrated)
				}
				for _, rewrite := range want.migratedAll {
					if !slices.Contains(result.Migrated, rewrite) {
						t.Errorf("%s.Migrated = %v; want it to contain %q", module, result.Migrated, rewrite)
					}
				}
				if want.loadsAsBatcher {
					if _, err := batcher.Active(configengine.Dirs{Anchor: baseDir}); err != nil {
						t.Errorf("batcher.Active after the run = %v; want nil error", err)
					}
				}
				if want.wantLoadedLenses != nil {
					loaded, err := lenses.Load(configengine.Dirs{Anchor: baseDir})
					if err != nil {
						t.Errorf("lenses.Load after the run = %v; want nil error", err)
					} else if !reflect.DeepEqual(loaded, *want.wantLoadedLenses) {
						t.Errorf("lenses.Load after the run = %+v; want %+v", loaded, *want.wantLoadedLenses)
					}
				}

				got, err := os.ReadFile(configengine.ConfigFile(baseDir, module))
				if err != nil {
					t.Fatalf("read %s.yaml: %v", module, err)
				}
				if want.wantFile != nil && string(got) != *want.wantFile {
					t.Errorf("%s.yaml = %q; want %q", module, got, *want.wantFile)
				}
				for _, sub := range want.fileContains {
					if !strings.Contains(string(got), sub) {
						t.Errorf("%s.yaml = %q; want it to contain %q", module, got, sub)
					}
				}
				for _, sub := range want.fileNotContains {
					if strings.Contains(string(got), sub) {
						t.Errorf("%s.yaml = %q; want it not to contain %q", module, got, sub)
					}
				}
			}
		})
	}
}

func TestReconcileAll_Idempotent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// First apply
	results1, err := ReconcileAll(tmpDir, "", Options{Apply: true})
	if err != nil {
		t.Fatalf("ReconcileAll first apply: %v", err)
	}

	// Second apply should be idempotent
	results2, err := ReconcileAll(tmpDir, "", Options{Apply: true})
	if err != nil {
		t.Fatalf("ReconcileAll second apply: %v", err)
	}

	// All results should show Applied=false on second run (no changes)
	if len(results2) != len(results1) {
		t.Errorf("result count changed: %d -> %d", len(results1), len(results2))
	}

	for _, result := range results2 {
		if result.Applied {
			t.Errorf("module %s shows Applied=true on second run; want false (idempotent)", result.Module)
		}
		if len(result.Added) > 0 {
			t.Errorf("module %s reports Added on second run; want empty (idempotent)", result.Module)
		}
		if len(result.Removed) > 0 {
			t.Errorf("module %s reports Removed on second run; want empty (idempotent)", result.Module)
		}
	}
}

// reconcileFabricResult runs ReconcileHubWideAt and returns the "fabric" result.
func reconcileFabricResult(boardDir, primeBaseDir string, apply bool) (Result, error) {
	results, err := ReconcileHubWideAt(boardDir, primeBaseDir, HubWideOptions{Apply: apply})
	if err != nil {
		return Result{}, err
	}
	result := findResult(results, "fabric")
	if result == nil {
		return Result{}, fmt.Errorf("ReconcileHubWideAt returned no fabric result: %+v", results)
	}
	return *result, nil
}

// TestReconcileHubWideAt_SeedsFromHubPrimeLegacyOrTemplate pins which input each hub-wide module's
// reconcile starts from:
// a present hub file is never replaced, an absent board.yaml or landing.yaml is seeded from the prime's copy when it has one, and an absent fabric.yaml never reads the prime.
func TestReconcileHubWideAt_SeedsFromHubPrimeLegacyOrTemplate(t *testing.T) {
	const primeBoard = "types:\n  spike: A time-boxed investigation\nlabels:\n  area-x: Area X\n"
	const hubBoard = "readme: README.md\ndesign_prefix: design-\ntypes:\n  bug: Hub bug\nlabels: {}\n"
	t.Parallel()
	lensesTunedHubFile := lensesFixture(t, func(cfg *lenses.Config) {
		delete(cfg.Lenses, "security")
		delete(cfg.Fans, "twill-discussion")
		cfg.Lenses["generic"] = "Tuned generic prose.\n"
		cfg.Lenses["hub-lens"] = "Hub emphasis.\n"
		cfg.Fans["standard"] = []string{"hub-lens"}
		cfg.Fans["hub-fan"] = []string{"generic"}
	})
	lensesCSharpPick := lensSetFixture(t, "csharp", map[string]string{
		"concurrency":    loadLensesWith(t, "lens_set: csharp\n").Lenses["concurrency"],
		"error-handling": loadLensesWith(t, "").Lenses["error-handling"],
	})

	tests := []struct {
		name         string
		module       string
		hubFile      string
		primeFile    string
		legacyFirst  string
		seedDefaults bool
		// primeMarked writes the prime worktree's sparse marker.
		primeMarked bool
		// wantNoFile pins that the file is still absent after an apply.
		wantNoFile   bool
		wantSeed     string
		wantContains []string
		wantAbsent   []string
		// wantAdded and wantKeptExtra are the applied result's Added and KeptExtra, compared sorted; nil skips the check.
		wantAdded     []string
		wantKeptExtra []string
		// wantDropped and wantNotDropped are names the applied result's Dropped must and must not hold.
		wantDropped    []string
		wantNotDropped []string
		// loadUnchanged pins that lenses.Load over the board dir resolves the same before the dry run and after the apply.
		loadUnchanged bool
	}{
		{
			name:           "lenses default under lens_set drops a lens equal to the layer's prose and keeps one equal to the base's",
			module:         "lenses",
			hubFile:        lensesCSharpPick,
			wantSeed:       SeedHub,
			wantDropped:    []string{"lenses.concurrency"},
			wantNotDropped: []string{"lenses.error-handling"},
			loadUnchanged:  true,
		},
		{
			name:         "lenses default absent is seeded from a prime holding lens_set and drops the layer-equal lens",
			module:       "lenses",
			primeFile:    lensesCSharpPick,
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"lens_set: csharp", "error-handling:"},
			wantAbsent:   []string{"concurrency:"},
		},
		{
			name:     "lenses default present keeps tuned and hub-only entries and drops template-equal ones",
			module:   "lenses",
			hubFile:  lensesTunedHubFile,
			wantSeed: SeedHub,
			wantContains: []string{
				"Tuned generic prose.", "standard:\n        - hub-lens\n", "hub-lens:", "hub-fan:",
			},
			wantAbsent:     []string{"security:", "twill-discussion:", "correctness:", "full:"},
			wantAdded:      []string{},
			wantKeptExtra:  []string{"fans.hub-fan", "lenses.hub-lens"},
			wantDropped:    []string{"fans.full", "lenses.correctness"},
			wantNotDropped: []string{"fans.standard", "fans.twill-discussion", "lenses.generic", "lenses.security"},
		},
		{
			name:         "loom default absent with a prime copy is seeded with the prime's non-template units",
			module:       "loom",
			primeFile:    "discussion_timeout_min: 500\n",
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"discussion_timeout_min: 500"},
			wantAbsent:   []string{"review_max_bounces:"},
			wantAdded:    []string{},
		},
		{
			name:         "loom default seeded from an unmarked prime drops a former template value",
			module:       "loom",
			primeFile:    "discussion_lenses: \"\"\nreview_max_bounces: 99\n",
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"review_max_bounces: 99"},
			wantAbsent:   []string{"discussion_lenses"},
		},
		{
			name:         "loom default seeded from a marked prime keeps a former template value as tuned",
			module:       "loom",
			primeFile:    "discussion_lenses: \"\"\nreview_max_bounces: 99\n",
			primeMarked:  true,
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"discussion_lenses: \"\"", "review_max_bounces: 99"},
		},
		{
			name:         "loom default absent and no prime copy starts as the header-only base",
			module:       "loom",
			seedDefaults: true,
			wantSeed:     SeedTemplate,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"review_max_bounces:"},
		},
		{
			name:         "loom default present is pruned in place and gains no template key",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\nstale_key: old\n",
			primeFile:    "discussion_timeout_min: 480\n",
			seedDefaults: true,
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7"},
			wantAbsent:   []string{"stale_key", "480", "review_max_bounces:"},
			wantAdded:    []string{},
		},
		{
			name:         "loom default present is pruned without SeedDefaults",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\nstale_key: old\n",
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7"},
			wantAbsent:   []string{"stale_key", "review_max_bounces:"},
		},
		{
			name:         "loom default left with no unit is written as the header-only base",
			module:       "loom",
			hubFile:      "stale_key: old\n",
			wantSeed:     SeedHub,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"stale_key"},
		},
		{
			name:       "loom default absent stays absent without SeedDefaults",
			module:     "loom",
			primeFile:  "discussion_timeout_min: 480\n",
			wantNoFile: true,
		},
		{
			name:         "models default absent is seeded from the prime verbatim",
			module:       "models",
			primeFile:    "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n",
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"zephyr:"},
			wantAbsent:   []string{"sonnet:"},
		},
		{
			name:         "models default present is never rewritten",
			module:       "models",
			hubFile:      "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n",
			seedDefaults: true,
			wantSeed:     SeedHub,
			wantContains: []string{"zephyr:"},
			wantAbsent:   []string{"sonnet:"},
		},
		{
			name:         "board absent with a prime copy is seeded from the prime",
			module:       "board",
			primeFile:    primeBoard,
			wantSeed:     SeedPrime,
			wantContains: []string{"spike: A time-boxed investigation", "area-x: Area X"},
			wantAbsent:   []string{"enhancement:", "design_prefix:"},
		},
		{
			name:         "board absent and no prime copy starts as the header-only base",
			module:       "board",
			wantSeed:     SeedTemplate,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"enhancement:"},
		},
		{
			name:         "board present is never replaced by a differing prime copy",
			module:       "board",
			hubFile:      hubBoard,
			primeFile:    primeBoard,
			wantSeed:     SeedHub,
			wantContains: []string{"bug: Hub bug"},
			wantAbsent:   []string{"spike:", "area-x:"},
		},
		{
			name:         "landing absent with a prime copy is seeded from the prime",
			module:       "landing",
			primeFile:    "deploy: make deploy-prime\n",
			wantSeed:     SeedPrime,
			wantContains: []string{"make deploy-prime"},
			wantAbsent:   []string{"deploy_branches:"},
		},
		{
			name:         "landing absent and no prime copy starts as the header-only base",
			module:       "landing",
			wantSeed:     SeedTemplate,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"deploy_branches:"},
		},
		{
			name:         "fabric absent ignores the prime copy",
			module:       "fabric",
			primeFile:    "branch_prefix: prime/\npathspec: \"\"\n",
			wantSeed:     SeedTemplate,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"prime/", "branch_prefix:"},
		},
		{
			name:         "profiles absent starts from the template's starting profiles",
			module:       "profiles",
			wantSeed:     SeedTemplate,
			wantContains: []string{"light:", "budget:", "docs:"},
		},
		{
			name:         "profiles present with a starting profile removed is never rewritten",
			module:       "profiles",
			hubFile:      "wide:\n  darn: {review_max_bounces: 2}\n",
			wantSeed:     SeedHub,
			wantContains: []string{"wide:"},
			wantAbsent:   []string{"light:", "budget:", "docs:"},
		},
		{
			name:         "darn present keeps its tree_scope trees and their targets",
			module:       "darn",
			hubFile:      "tree_scope:\n  tools: ./tools/...\n  docs: \"\"\n",
			wantSeed:     SeedHub,
			wantContains: []string{"tools: ./tools/...", "docs: \"\""},
			wantAbsent:   []string{"review_max_bounces:"},
		},
		{
			name:         "fabric absent with a legacy warp.yaml folds it in",
			module:       "fabric",
			legacyFirst:  "branch_prefix: legacy/\n",
			wantSeed:     SeedLegacy,
			wantContains: []string{"branch_prefix: legacy/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			primeDir := t.TempDir()
			writeModuleFile(t, boardDir, tt.module, tt.hubFile)
			writeModuleFile(t, primeDir, tt.module, tt.primeFile)
			if tt.primeMarked {
				if err := os.WriteFile(configengine.SparseMarker(primeDir), []byte("# sparse\n"), 0o644); err != nil {
					t.Fatalf("write the prime's sparse marker: %v", err)
				}
			}
			writeModuleFile(t, boardDir, "warp", tt.legacyFirst)
			hubPath := configengine.ConfigFile(boardDir, tt.module)
			var loadedBefore lenses.Config
			if tt.loadUnchanged {
				loadedBefore = loadLensesAt(t, configengine.Dirs{Board: boardDir})
			}

			dry, err := ReconcileHubWideAt(boardDir, primeDir, HubWideOptions{SeedDefaults: tt.seedDefaults})
			if err != nil {
				t.Fatalf("ReconcileHubWideAt(dry): %v", err)
			}
			dryResult := findResult(dry, tt.module)
			if dryResult == nil || dryResult.Seed != tt.wantSeed || dryResult.Applied || dryResult.Layer != configengine.LayerDefault {
				t.Errorf("dry-run result = %+v; want Seed %q, Layer default and Applied false", dryResult, tt.wantSeed)
			}
			if tt.hubFile == "" {
				if _, err := os.Stat(hubPath); !os.IsNotExist(err) {
					t.Errorf("hub file written on a dry run (stat err = %v)", err)
				}
			}
			if tt.legacyFirst != "" {
				if _, err := os.Stat(configengine.ConfigFile(boardDir, "warp")); err != nil {
					t.Errorf("legacy warp.yaml removed on a dry run: %v", err)
				}
			}

			applied, err := ReconcileHubWideAt(boardDir, primeDir, HubWideOptions{Apply: true, SeedDefaults: tt.seedDefaults})
			if err != nil {
				t.Fatalf("ReconcileHubWideAt(apply): %v", err)
			}
			appliedResult := findResult(applied, tt.module)
			if appliedResult == nil || appliedResult.Seed != tt.wantSeed {
				t.Fatalf("applied result = %+v; want Seed %q", appliedResult, tt.wantSeed)
			}
			if tt.wantAdded != nil && !slices.Equal(sortedCopy(appliedResult.Added), tt.wantAdded) {
				t.Errorf("applied Added = %v; want %v", appliedResult.Added, tt.wantAdded)
			}
			if tt.wantKeptExtra != nil && !slices.Equal(sortedCopy(appliedResult.KeptExtra), tt.wantKeptExtra) {
				t.Errorf("applied KeptExtra = %v; want %v", appliedResult.KeptExtra, tt.wantKeptExtra)
			}
			for _, name := range tt.wantDropped {
				if !slices.Contains(appliedResult.Dropped, name) {
					t.Errorf("applied Dropped = %v; want it to contain %q", appliedResult.Dropped, name)
				}
			}
			for _, name := range tt.wantNotDropped {
				if slices.Contains(appliedResult.Dropped, name) {
					t.Errorf("applied Dropped = %v; want it not to contain %q", appliedResult.Dropped, name)
				}
			}
			if tt.loadUnchanged {
				if loadedAfter := loadLensesAt(t, configengine.Dirs{Board: boardDir}); !reflect.DeepEqual(loadedAfter, loadedBefore) {
					t.Errorf("lenses.Load after the apply differs from before: got %+v; want %+v", loadedAfter, loadedBefore)
				}
			}
			if tt.wantNoFile {
				if _, err := os.Stat(hubPath); !os.IsNotExist(err) {
					t.Errorf("default written without SeedDefaults (stat err = %v)", err)
				}
				return
			}
			got, err := os.ReadFile(hubPath)
			if err != nil {
				t.Fatalf("read hub file: %v", err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(string(got), want) {
					t.Errorf("hub file = %q; want it to contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(string(got), absent) {
					t.Errorf("hub file = %q; want it not to contain %q", got, absent)
				}
			}
		})
	}
}

// TestReconcileHubWideAt_MigratesLegacyFabricConfig pins the fabric-cutover's one-shot migration
// (F-D): a pre-cutover hub's warp.yaml/weft.yaml values must be folded into fabric.yaml's first
// write instead of silently discarded in favor of the bare template default,
// and the legacy files must be pruned afterward so the migration does not re-fire.
// Routed through ReconcileHubWideAt(boardDir, "", apply), since ReconcileAll never reconciles a
// hub-wide module (see TestReconcileAll_HubWideCopy).
func TestReconcileHubWideAt_MigratesLegacyFabricConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		legacyFirst  string
		legacySecond string
		fabric       string
		apply        bool
		// wantApplied is checked only when non-nil.
		wantApplied  *bool
		wantMigrated []string
		// wantFabric is a substring list of the written fabric.yaml; wantNoFabric means the file must not exist.
		wantFabric []string
		// wantFabricAbsent are substrings the written fabric.yaml must not contain.
		wantFabricAbsent []string
		wantNoFabric     bool
		// wantPruned and wantKept name the legacy modules whose file must be gone or still on disk.
		wantPruned []string
		wantKept   []string
	}{
		{
			name:         "both legacy files present, both values migrate, both files pruned",
			legacyFirst:  "branch_prefix: hanf/\n",
			legacySecond: "pathspec: _lyx custom-dir\n",
			apply:        true,
			wantApplied:  ptr(true),
			wantMigrated: []string{"warp", "weft"},
			wantFabric:   []string{"branch_prefix: hanf/", "pathspec: _lyx custom-dir"},
			wantPruned:   []string{"warp", "weft"},
		},
		{
			// _lyx is structural, injected in code and never read from the pathspec key, so the sparse fabric.yaml holds no pathspec and the load resolves the template's empty one.
			name:             "only warp.yaml present, the seeded file holds only the legacy unit",
			legacyFirst:      "branch_prefix: hanf/\n",
			apply:            true,
			wantMigrated:     []string{"warp"},
			wantFabric:       []string{"branch_prefix: hanf/"},
			wantFabricAbsent: []string{"pathspec"},
			wantPruned:       []string{"warp"},
		},
		{
			name:         "dry run reports the pending migration but writes and deletes nothing",
			legacyFirst:  "branch_prefix: hanf/\n",
			wantApplied:  ptr(false),
			wantMigrated: []string{"warp"},
			wantNoFabric: true,
			wantKept:     []string{"warp"},
		},
		{
			name:        "fabric.yaml already present, legacy files untouched and not migrated",
			legacyFirst: "branch_prefix: stale/\n",
			fabric:      "branch_prefix: existing/\npathspec: _lyx\n",
			apply:       true,
			wantFabric:  []string{"branch_prefix: existing/"},
			wantKept:    []string{"warp"},
		},
		{
			name:        "unparseable legacy file is skipped, left on disk, and not migrated",
			legacyFirst: "branch_prefix: [unterminated\n",
			apply:       true,
			wantKept:    []string{"warp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			writeModuleFile(t, boardDir, "warp", tt.legacyFirst)
			writeModuleFile(t, boardDir, "weft", tt.legacySecond)
			writeModuleFile(t, boardDir, "fabric", tt.fabric)

			fabricResult, err := reconcileFabricResult(boardDir, "", tt.apply)
			if err != nil {
				t.Fatalf("ReconcileHubWideAt(%v): %v", tt.apply, err)
			}

			if tt.wantApplied != nil && fabricResult.Applied != *tt.wantApplied {
				t.Errorf("fabric.Applied = %v; want %v", fabricResult.Applied, *tt.wantApplied)
			}
			if !slices.Equal(sortedCopy(fabricResult.MigratedFrom), tt.wantMigrated) {
				t.Errorf("fabric.MigratedFrom = %v; want %v", fabricResult.MigratedFrom, tt.wantMigrated)
			}

			fabricPath := configengine.ConfigFile(boardDir, "fabric")
			if tt.wantNoFabric {
				if _, err := os.Stat(fabricPath); !os.IsNotExist(err) {
					t.Errorf("fabric.yaml was written on a dry run; want absent (stat err = %v)", err)
				}
			}
			if len(tt.wantFabric) > 0 {
				got, err := os.ReadFile(fabricPath)
				if err != nil {
					t.Fatalf("read fabric.yaml: %v", err)
				}
				for _, want := range tt.wantFabric {
					if !strings.Contains(string(got), want) {
						t.Errorf("fabric.yaml = %q; want it to contain %q", got, want)
					}
				}
				for _, absent := range tt.wantFabricAbsent {
					if strings.Contains(string(got), absent) {
						t.Errorf("fabric.yaml = %q; want it not to contain %q", got, absent)
					}
				}
			}
			for _, module := range tt.wantPruned {
				if _, err := os.Stat(configengine.ConfigFile(boardDir, module)); !os.IsNotExist(err) {
					t.Errorf("%s.yaml still exists after migration; want pruned (stat err = %v)", module, err)
				}
			}
			for _, module := range tt.wantKept {
				if _, err := os.Stat(configengine.ConfigFile(boardDir, module)); err != nil {
					t.Errorf("%s.yaml was removed; want it left on disk (stat err = %v)", module, err)
				}
			}
		})
	}
}

// TestReconcileHubWideAt_StripsAnUnmigratedBaseOnce pins the one-time strip of a hub's base files: while the board's base-sparse marker is absent, a walk's pass (StripBase and Apply) reduces every non-seed-only base to the units that differ from the template and from a former template value, and writes the marker only when every module's file exists;
// without StripBase or on a dry run the strip is only reported, a marked board keeps a unit equal to a former value as a pin, and a seed-only file is never rewritten.
func TestReconcileHubWideAt_StripsAnUnmigratedBaseOnce(t *testing.T) {
	t.Parallel()
	loomTemplate := moduleTemplate(t, "loom")
	darnTemplate := moduleTemplate(t, "darn")
	const modelsTuned = "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
	const formerRoles = "prompt_cache_ttl_roles:\n  driver: 1h\n  webster: 1h\n"
	loomFormer := strings.Replace(loomTemplate, "discussion_lenses: twill-discussion", `discussion_lenses: ""`, 1)
	darnTuned := strings.Replace(darnTemplate, "writer_timeout_min: 180", "writer_timeout_min: 77", 1)
	if loomFormer == loomTemplate || darnTuned == darnTemplate {
		t.Fatal("the loom or darn template no longer holds the line this test rewrites")
	}

	tests := []struct {
		name string
		// overrides replaces a module's written file; every other module is written as its template.
		overrides map[string]string
		// withheld names modules whose file is not written.
		withheld    []string
		boardMarked bool
		options     HubWideOptions
		// wantUnchangedAll pins every file as written; otherwise a non-seed-only file ends header-only unless wantKept names it.
		wantUnchangedAll bool
		wantKept         map[string][]string
		// wantStripped lists keys a module's Result.Stripped must hold; wantNoStripped names modules whose Stripped must be empty.
		wantStripped   map[string][]string
		wantNoStripped []string
		wantMarker     bool
	}{
		{
			name:         "unmigrated board is stripped to header-only bases, keeps a tuned unit and gets the marker",
			overrides:    map[string]string{"loom": loomFormer, "darn": darnTuned, "models": modelsTuned},
			options:      HubWideOptions{Apply: true, StripBase: true, SeedDefaults: true},
			wantKept:     map[string][]string{"darn": {"writer_timeout_min: 77"}},
			wantStripped: map[string][]string{"loom": {"discussion_lenses", "review_max_bounces"}, "darn": {"verify_attempts"}},
			wantMarker:   true,
		},
		{
			name:             "without StripBase nothing is stripped or marked and the keys are pending",
			overrides:        map[string]string{"loom": loomFormer},
			options:          HubWideOptions{Apply: true, SeedDefaults: true},
			wantUnchangedAll: true,
			wantStripped:     map[string][]string{"loom": {"discussion_lenses", "review_max_bounces"}},
		},
		{
			name:             "a dry run with StripBase writes nothing",
			overrides:        map[string]string{"loom": loomFormer},
			options:          HubWideOptions{StripBase: true, SeedDefaults: true},
			wantUnchangedAll: true,
			wantStripped:     map[string][]string{"loom": {"discussion_lenses", "review_max_bounces"}},
		},
		{
			name:         "a claude base holding the former roles map whole is stripped to the header",
			overrides:    map[string]string{"claude": formerRoles},
			options:      HubWideOptions{Apply: true, StripBase: true, SeedDefaults: true},
			wantStripped: map[string][]string{"claude": {"prompt_cache_ttl_roles"}},
			wantMarker:   true,
		},
		{
			name:           "a roles map with one role changed is kept whole",
			overrides:      map[string]string{"claude": "prompt_cache_ttl_roles:\n  driver: 1h\n  webster: 5m\n"},
			options:        HubWideOptions{Apply: true, StripBase: true, SeedDefaults: true},
			wantKept:       map[string][]string{"claude": {"webster: 5m"}},
			wantNoStripped: []string{"claude"},
			wantMarker:     true,
		},
		{
			name:             "a marked board keeps a unit equal to a former value as a pin",
			overrides:        map[string]string{"loom": "discussion_lenses: \"\"\n"},
			boardMarked:      true,
			options:          HubWideOptions{Apply: true, StripBase: true, SeedDefaults: true},
			wantUnchangedAll: true,
			wantNoStripped:   []string{"loom"},
			wantMarker:       true,
		},
		{
			name:       "an absent per-worktree base strips the others and withholds the marker",
			withheld:   []string{"loom"},
			options:    HubWideOptions{Apply: true, StripBase: true},
			wantMarker: false,
		},
		{
			name:       "a key move carrying the former roles map into an absent base leaves the created base header-only",
			overrides:  map[string]string{"shuttle": "claude_prompt_cache_ttl_roles:\n  driver: 1h\n  webster: 1h\n"},
			withheld:   []string{"claude"},
			options:    HubWideOptions{Apply: true, StripBase: true, SeedDefaults: true},
			wantMarker: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			written := map[string]string{}
			for _, m := range configreg.Modules() {
				if slices.Contains(tt.withheld, m.Name) {
					continue
				}
				content := m.Template()
				if override, ok := tt.overrides[m.Name]; ok {
					content = override
				}
				written[m.Name] = content
				writeModuleFile(t, boardDir, m.Name, content)
			}
			if tt.boardMarked {
				if err := WriteBaseSparseMarker(boardDir); err != nil {
					t.Fatalf("WriteBaseSparseMarker: %v", err)
				}
			}

			results, err := ReconcileHubWideAt(boardDir, "", tt.options)
			if err != nil {
				t.Fatalf("ReconcileHubWideAt: %v", err)
			}

			for module, content := range written {
				registered, _ := configreg.Lookup(module)
				gotBytes, err := os.ReadFile(configengine.ConfigFile(boardDir, module))
				if err != nil {
					t.Fatalf("read %s.yaml: %v", module, err)
				}
				got := string(gotBytes)
				switch kept, isKept := tt.wantKept[module]; {
				case tt.wantUnchangedAll || registered.SeedOnly:
					// A per-name map drops its template-equal names on every applied reconcile, marked board or not.
					if len(registered.PerNameMaps) > 0 && tt.options.Apply && !registered.SeedOnly {
						continue
					}
					if got != content {
						t.Errorf("%s.yaml changed; want it byte for byte as written", module)
					}
				case isKept:
					for _, want := range kept {
						if !strings.Contains(got, want) {
							t.Errorf("%s.yaml = %q; want it to contain %q", module, got, want)
						}
					}
				case got != configengine.SparseBaseHeader:
					t.Errorf("%s.yaml = %q; want only the sparse header", module, got)
				}
			}
			for _, module := range tt.withheld {
				created := tt.options.SeedDefaults && tt.options.Apply
				got, err := os.ReadFile(configengine.ConfigFile(boardDir, module))
				switch {
				case !created && !os.IsNotExist(err):
					t.Errorf("%s.yaml written although the pass leaves an absent per-worktree base absent (read err = %v)", module, err)
				case created && (err != nil || string(got) != configengine.SparseBaseHeader):
					t.Errorf("%s.yaml = %q (read err = %v); want the created base to hold only the sparse header", module, got, err)
				}
			}
			for module, keys := range tt.wantStripped {
				result := findResult(results, module)
				if result == nil {
					t.Fatalf("no result for %s", module)
				}
				for _, key := range keys {
					if !slices.Contains(result.Stripped, key) {
						t.Errorf("%s Stripped = %v; want it to hold %q", module, result.Stripped, key)
					}
				}
			}
			for _, module := range tt.wantNoStripped {
				if result := findResult(results, module); result == nil || len(result.Stripped) > 0 {
					t.Errorf("%s result = %+v; want a result with no Stripped key", module, result)
				}
			}
			_, markerErr := os.Stat(configengine.BaseSparseMarker(boardDir))
			if marker := markerErr == nil; marker != tt.wantMarker {
				t.Errorf("base-sparse marker present = %v; want %v", marker, tt.wantMarker)
			}
		})
	}
}

// moduleTemplate returns the registered module's template text.
func moduleTemplate(t *testing.T, module string) string {
	t.Helper()
	template, ok := configreg.Template(module)
	if !ok {
		t.Fatalf("configreg.Template(%q): module not registered", module)
	}
	return template()
}

// findResult returns the Result for the named module, or nil if absent.
func findResult(results []Result, module string) *Result {
	for i := range results {
		if results[i].Module == module {
			return &results[i]
		}
	}
	return nil
}

// sortedCopy returns a sorted copy of keys, so a comparison ignores report order.
func sortedCopy(keys []string) []string {
	out := slices.Clone(keys)
	slices.Sort(out)
	return out
}

// ptr returns a pointer to v, for a table field where nil means "unchecked".
func ptr[T any](v T) *T { return &v }

// TestReconcileAll_HubWideCopy pins how ReconcileAll treats a hub-wide module ("board"): it never
// reconciles or writes it under the worktree, and a leftover per-worktree copy is kept, reported
// Retired when the hub file subsumes it (apply deletes it) or Divergent when it holds an entry the
// hub file lacks.
func TestReconcileAll_HubWideCopy(t *testing.T) {
	t.Parallel()
	const hubBoard = "types:\n  bug: Hub bug\nlabels:\n  area-x: Area X\n"

	tests := []struct {
		name          string
		copyFile      string
		hubFile       string
		wantRetired   bool
		wantDivergent []string
		wantKept      bool
	}{
		{name: "no per-worktree copy"},
		{name: "copy and no hub file keeps the copy", copyFile: hubBoard, wantKept: true},
		{name: "copy subsumed by the hub file is retired", copyFile: "labels:\n  area-x: Area X\n", hubFile: hubBoard, wantRetired: true},
		{
			name:          "copy holding a label the hub file lacks is divergent",
			copyFile:      "labels:\n  area-x: Area X\n  area-y: Area Y\n",
			hubFile:       hubBoard,
			wantDivergent: []string{"labels.area-y: Area Y"},
			wantKept:      true,
		},
		{
			name:          "copy value differing from the hub file is divergent",
			copyFile:      "types:\n  bug: Local bug\n",
			hubFile:       hubBoard,
			wantDivergent: []string{"types.bug: Local bug"},
			wantKept:      true,
		},
		{name: "copy holding a template default over a hub file lacking that key is retired", copyFile: "readme: ${env:LYX_README:-README.md}\n", hubFile: hubBoard, wantRetired: true},
		{
			name:          "copy value differing from both the template and the hub file is divergent",
			copyFile:      "readme: custom.md\n",
			hubFile:       hubBoard,
			wantDivergent: []string{"readme: custom.md"},
			wantKept:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			boardDir := t.TempDir()
			writeModuleFile(t, baseDir, "board", tt.copyFile)
			writeModuleFile(t, boardDir, "board", tt.hubFile)
			copyPath := configengine.ConfigFile(baseDir, "board")

			check := func(apply bool) {
				t.Helper()
				results, err := ReconcileAll(baseDir, boardDir, Options{Apply: apply})
				if err != nil {
					t.Fatalf("ReconcileAll(%v): %v", apply, err)
				}
				r := findResult(results, "board")
				if r == nil {
					t.Fatal("no result for board")
				}
				if r.Retired != tt.wantRetired {
					t.Errorf("apply=%v Retired = %v; want %v", apply, r.Retired, tt.wantRetired)
				}
				if r.Applied != (tt.wantRetired && apply) {
					t.Errorf("apply=%v Applied = %v; want %v", apply, r.Applied, tt.wantRetired && apply)
				}
				if fmt.Sprint(r.Divergent) != fmt.Sprint(tt.wantDivergent) {
					t.Errorf("apply=%v Divergent = %v; want %v", apply, r.Divergent, tt.wantDivergent)
				}
				_, statErr := os.Stat(copyPath)
				wantPresent := tt.wantKept || (tt.wantRetired && !apply)
				if present := statErr == nil; present != wantPresent {
					t.Errorf("apply=%v per-worktree board.yaml present = %v; want %v", apply, present, wantPresent)
				}
				if tt.copyFile == "" {
					if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
						t.Errorf("apply=%v per-worktree board.yaml created (stat err = %v)", apply, err)
					}
				}
				if _, err := os.Stat(configengine.ConfigFile(baseDir, "fabric")); !os.IsNotExist(err) {
					t.Errorf("apply=%v per-worktree fabric.yaml written (stat err = %v)", apply, err)
				}
			}
			check(false)
			check(true)
		})
	}
}

// TestReconcileAll_SparseLayers pins that in a hub a worktree file and a local file are sparse layers: retired keys are pruned and a below-floor value is migrated, but nothing is ever added, an absent file stays absent, a seed-only module's files are untouched, and a file emptied by a prune is deleted.
func TestReconcileAll_SparseLayers(t *testing.T) {
	t.Parallel()
	const modelsAlias = "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
	tests := []struct {
		name   string
		module string
		layer  configengine.Layer
		// content is the layer file before the run; empty means the file is absent.
		content string
		// wantFile is the file after the run; nil means it must not exist.
		wantFile    *string
		wantApplied bool
		wantRemoved []string
		wantMigrate []string
		// wantResult is false when the layer is absent and no result is reported.
		wantResult    bool
		wantKeptExtra []string
	}{
		{
			name: "worktree file keeps a hub-only lens and stays byte-identical", module: "lenses", layer: configengine.LayerWorktree,
			content: "lenses:\n  hub-lens: |\n    Hub emphasis.\n", wantFile: ptr("lenses:\n  hub-lens: |\n    Hub emphasis.\n"),
			wantKeptExtra: []string{"lenses.hub-lens"}, wantResult: true,
		},
		{
			name: "worktree file loses a retired key and gains no template key", module: "loom", layer: configengine.LayerWorktree,
			content: "discussion_timeout_min: 480\nstale_key: old\n", wantFile: ptr("discussion_timeout_min: 480\n"),
			wantApplied: true, wantRemoved: []string{"stale_key"}, wantResult: true,
		},
		{
			name: "below-floor loom value is raised by Migrate", module: "loom", layer: configengine.LayerWorktree,
			content: "step_idle_timeout_min: 0\n", wantFile: ptr("step_idle_timeout_min: 1\n"),
			wantApplied: true, wantMigrate: []string{"step_idle_timeout_min: raised from 0 to 1"}, wantResult: true,
		},
		{
			name: "absent worktree file stays absent", module: "loom", layer: configengine.LayerWorktree,
			wantResult: true,
		},
		{
			name: "local file is pruned and reported with layer local", module: "loom", layer: configengine.LayerLocal,
			content: "review_max_bounces: 2\nstale_key: old\n", wantFile: ptr("review_max_bounces: 2\n"),
			wantApplied: true, wantRemoved: []string{"stale_key"}, wantResult: true,
		},
		{
			name: "seed-only worktree file is left byte-identical", module: "models", layer: configengine.LayerWorktree,
			content: modelsAlias, wantFile: ptr(modelsAlias), wantResult: true,
		},
		{
			name: "seed-only local file is left byte-identical", module: "models", layer: configengine.LayerLocal,
			content: modelsAlias, wantFile: ptr(modelsAlias), wantResult: true,
		},
		{
			name: "file emptied by a prune is deleted", module: "loom", layer: configengine.LayerWorktree,
			content: "stale_key: old\n", wantApplied: true, wantRemoved: []string{"stale_key"}, wantResult: true,
		},
		{
			name: "absent local file reports nothing", module: "loom", layer: configengine.LayerLocal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor, boardDir := t.TempDir(), t.TempDir()
			path := configengine.ConfigFile(anchor, tt.module)
			if tt.layer == configengine.LayerLocal {
				path = configengine.LocalFile(anchor, tt.module)
			}
			if tt.content != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}

			dry, err := ReconcileAll(anchor, boardDir, Options{})
			if err != nil {
				t.Fatalf("ReconcileAll(dry): %v", err)
			}
			if tt.content != "" {
				if got, _ := os.ReadFile(path); string(got) != tt.content {
					t.Errorf("dry run changed %s to %q; want %q", path, got, tt.content)
				}
			}
			if dryResult := findLayerResult(dry, tt.module, tt.layer); dryResult != nil && dryResult.Applied {
				t.Errorf("dry run result = %+v; want Applied false", dryResult)
			}

			results, err := ReconcileAll(anchor, boardDir, Options{Apply: true})
			if err != nil {
				t.Fatalf("ReconcileAll(apply): %v", err)
			}
			result := findLayerResult(results, tt.module, tt.layer)
			if (result != nil) != tt.wantResult {
				t.Fatalf("result for %s layer %s = %+v; want present = %v", tt.module, tt.layer, result, tt.wantResult)
			}
			if result != nil {
				if result.Applied != tt.wantApplied || !slices.Equal(result.Removed, tt.wantRemoved) || !slices.Equal(result.Migrated, tt.wantMigrate) || len(result.Added) != 0 {
					t.Errorf("result = %+v; want Applied %v, Removed %v, Migrated %v and nothing Added", result, tt.wantApplied, tt.wantRemoved, tt.wantMigrate)
				}
				if !slices.Equal(result.KeptExtra, tt.wantKeptExtra) {
					t.Errorf("KeptExtra = %v; want %v", result.KeptExtra, tt.wantKeptExtra)
				}
			}
			got, readErr := os.ReadFile(path)
			if tt.wantFile == nil {
				if !os.IsNotExist(readErr) {
					t.Errorf("%s = %q (err %v); want it absent", path, got, readErr)
				}
				return
			}
			if string(got) != *tt.wantFile {
				t.Errorf("%s = %q; want %q", path, got, *tt.wantFile)
			}
		})
	}
}

// TestReconcileAll_MigratesToSparse pins the one-time strip of a worktree's full files to sparse overrides:
// the keys equal to what the layers below resolve to are stripped and reported, an emptied file is deleted, and every key the file held resolves unchanged.
// The keys the repo default adds are reported as inherited.
// A marked worktree, a dry run, a run without Migrate and a hub missing a repo default strip nothing.
func TestReconcileAll_MigratesToSparse(t *testing.T) {
	t.Parallel()
	const (
		stripFile    = "discussion_timeout_min: 480\nreview_max_bounces: 99\n"
		markerPlan   = "plan: \"${env:LYXMIG_PLAN:-sonnet}\"\n"
		literalPlan  = "plan: sonnet\n"
		defaultAlias = "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
	)
	batcherTemplate := batcher.ConfigTemplate()
	batcherDiffering := strings.Replace(batcherTemplate, "orientation: 31400", "orientation: 31401", 1)
	modelsTemplate := modelspec.ConfigTemplate()
	migrate := Options{Apply: true, Migrate: true}
	lensesCSharpConcurrency := lensSetFixture(t, "", map[string]string{"concurrency": loadLensesWith(t, "lens_set: csharp\n").Lenses["concurrency"]})

	tests := []struct {
		name         string
		module       string
		opts         Options
		defaultFile  string
		worktreeFile string
		marked       bool
		noDefault    bool
		// wantFile is the worktree file's exact content afterwards; nil leaves it to wantDeleted and the substring lists.
		wantFile        *string
		wantDeleted     bool
		wantContains    []string
		wantNotContains []string
		wantStripped    []string
		wantInherited   []string
		wantMarker      bool
		// resolutionChanges marks a row whose strip drops a former value, so the worktree resolves to the layer below's value.
		resolutionChanges bool
	}{
		{
			name: "a former value is stripped even where the default tuned another, and the worktree resolves to the default's", module: "loom", opts: migrate,
			defaultFile: loomTemplateWith(loomTemplate(), "discussion_lenses", "discussion_lenses: tuned-fan"), worktreeFile: "discussion_lenses: \"\"\nreview_max_bounces: 99\n",
			wantContains: []string{"review_max_bounces: 99"}, wantNotContains: []string{"discussion_lenses"},
			wantStripped: []string{"discussion_lenses"}, wantMarker: true, resolutionChanges: true,
		},
		{
			name: "a whole open map equal to the former map is stripped", module: "claude", opts: migrate,
			worktreeFile: "prompt_cache_ttl_roles:\n  driver: 1h\n  webster: 1h\n",
			wantStripped: []string{"prompt_cache_ttl_roles"}, wantDeleted: true, wantMarker: true, resolutionChanges: true,
		},
		{
			name: "a marked worktree keeps a former value byte for byte", module: "loom", opts: migrate, marked: true,
			worktreeFile: "discussion_lenses: \"\"\n", wantFile: ptr("discussion_lenses: \"\"\n"),
		},
		{
			name: "keys equal to the default are stripped and reported", module: "loom", opts: migrate,
			worktreeFile: stripFile, wantContains: []string{"review_max_bounces: 99"}, wantNotContains: []string{"discussion_timeout_min"},
			wantStripped: []string{"discussion_timeout_min"}, wantMarker: true,
		},
		{
			name: "an emptied file is deleted", module: "loom", opts: migrate,
			worktreeFile: "discussion_timeout_min: 480\n", wantStripped: []string{"discussion_timeout_min"}, wantDeleted: true, wantMarker: true,
		},
		{
			name: "an open map differing in one entry is kept whole", module: "claude", opts: migrate,
			worktreeFile: "prompt_cache_ttl_roles:\n  driver: 1h\n  webster: 2h\n", wantContains: []string{"driver: 1h", "webster: 2h"}, wantMarker: true,
		},
		{
			name: "a per-name map keeps only the keys differing from the layers below", module: "batcher", opts: migrate,
			worktreeFile: batcherDiffering, wantContains: []string{"orientation: 31401"}, wantMarker: true,
			wantStripped: []string{
				"active", "profiles.cautious.batchifier", "profiles.cautious.budget", "profiles.cautious.max_cards",
				"profiles.cautious.weights.batch_growth", "profiles.cautious.weights.context_per_line", "profiles.cautious.weights.fork_messages",
				"profiles.cautious.weights.message_context", "profiles.cautious.weights.package_context", "profiles.cautious.weights.target_messages",
				"profiles.cautious.weights.test_file_messages", "profiles.cautious.weights.uses_messages", "profiles.cautious.weights.write_per_card_line",
				"profiles.identity.batchifier",
			},
		},
		{
			name: "a literal equal to the expansion of a marker below is kept", module: "loom", opts: migrate,
			defaultFile: loomTemplateWith(loomTemplate(), "plan", markerPlan), worktreeFile: literalPlan,
			wantFile: ptr(literalPlan), wantMarker: true,
		},
		{
			name: "a marker over a literal equal to its expansion is kept", module: "loom", opts: migrate,
			defaultFile: loomTemplateWith(loomTemplate(), "plan", literalPlan), worktreeFile: markerPlan,
			wantFile: ptr(markerPlan), wantMarker: true,
		},
		{
			name: "a seed-only key the default holds and the file lacks is reported inherited", module: "models", opts: migrate,
			defaultFile: modelsTemplate + defaultAlias, worktreeFile: modelsTemplate,
			wantStripped: []string{"fable", "haiku", "opus", "sonnet"}, wantInherited: []string{"zephyr"}, wantDeleted: true, wantMarker: true,
		},
		{
			name: "a marked worktree is left alone", module: "loom", opts: migrate, marked: true,
			worktreeFile: stripFile, wantFile: ptr(stripFile),
		},
		{
			name: "a dry run strips nothing", module: "loom", opts: Options{Migrate: true},
			worktreeFile: stripFile, wantFile: ptr(stripFile), wantStripped: []string{"discussion_timeout_min"},
		},
		{
			name: "an apply without Migrate strips nothing", module: "loom", opts: Options{Apply: true},
			worktreeFile: stripFile, wantFile: ptr(stripFile), wantStripped: []string{"discussion_timeout_min"},
		},
		{
			name: "a hub missing a repo default strips nothing", module: "loom", opts: migrate, noDefault: true,
			worktreeFile: stripFile, wantFile: ptr(stripFile),
		},
		{
			name: "a lens equal to the prose of the layer the default picks is stripped and the picked selector is inherited", module: "lenses", opts: migrate,
			defaultFile: "lens_set: csharp\n", worktreeFile: lensesCSharpConcurrency,
			wantStripped: []string{"lenses.concurrency"}, wantInherited: []string{"lens_set"}, wantDeleted: true, wantMarker: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor, boardDir := t.TempDir(), t.TempDir()
			for _, m := range configreg.Modules() {
				if !m.HubWide {
					writeModuleFile(t, boardDir, m.Name, m.Template())
				}
			}
			writeModuleFile(t, boardDir, tt.module, tt.defaultFile)
			if tt.noDefault {
				if err := os.Remove(configengine.ConfigFile(boardDir, "batcher")); err != nil {
					t.Fatalf("remove batcher default: %v", err)
				}
			}
			writeModuleFile(t, anchor, tt.module, tt.worktreeFile)
			if tt.marked {
				if err := os.WriteFile(configengine.SparseMarker(anchor), []byte("marked\n"), 0o644); err != nil {
					t.Fatalf("write marker: %v", err)
				}
			}
			module, _ := configreg.Lookup(tt.module)
			dirs := configengine.Dirs{Anchor: anchor, Board: boardDir}
			before, err := configengine.Load(dirs, module.Spec())
			if err != nil {
				t.Fatalf("Load before: %v", err)
			}

			results, err := ReconcileAll(anchor, boardDir, tt.opts)
			if err != nil {
				t.Fatalf("ReconcileAll: %v", err)
			}

			result := findLayerResult(results, tt.module, configengine.LayerWorktree)
			if result == nil {
				t.Fatalf("no worktree-layer result for %s", tt.module)
			}
			if !slices.Equal(result.Stripped, tt.wantStripped) && !(len(result.Stripped) == 0 && len(tt.wantStripped) == 0) {
				t.Errorf("Stripped = %v; want %v", result.Stripped, tt.wantStripped)
			}
			if !slices.Equal(result.Inherited, tt.wantInherited) && !(len(result.Inherited) == 0 && len(tt.wantInherited) == 0) {
				t.Errorf("Inherited = %v; want %v", result.Inherited, tt.wantInherited)
			}

			got, readErr := os.ReadFile(configengine.ConfigFile(anchor, tt.module))
			switch {
			case tt.wantDeleted:
				if !os.IsNotExist(readErr) {
					t.Errorf("worktree file = %q (err %v); want it deleted", got, readErr)
				}
			case tt.wantFile != nil:
				if string(got) != *tt.wantFile {
					t.Errorf("worktree file = %q; want %q", got, *tt.wantFile)
				}
			default:
				for _, sub := range tt.wantContains {
					if !strings.Contains(string(got), sub) {
						t.Errorf("worktree file = %q; want it to contain %q", got, sub)
					}
				}
				for _, sub := range tt.wantNotContains {
					if strings.Contains(string(got), sub) {
						t.Errorf("worktree file = %q; want it not to contain %q", got, sub)
					}
				}
			}

			_, markerErr := os.Stat(configengine.SparseMarker(anchor))
			if markerPresent := markerErr == nil; markerPresent != (tt.wantMarker || tt.marked) {
				t.Errorf("marker present = %v; want %v", markerPresent, tt.wantMarker || tt.marked)
			}

			after, err := configengine.Load(dirs, module.Spec())
			if err != nil {
				t.Fatalf("Load after: %v", err)
			}
			// A stripped per-name entry resolves from the template, so the keys may come back in another order.
			sameResolution := reflect.DeepEqual(decodeDocument(t, before), decodeDocument(t, after))
			if !tt.resolutionChanges && !sameResolution {
				t.Errorf("resolution changed by the migration:\nbefore:\n%s\nafter:\n%s", before, after)
			}
			if tt.resolutionChanges && sameResolution {
				t.Errorf("resolution unchanged by a strip of a former value:\n%s", after)
			}
		})
	}
}

// TestFormerSourceDefaults_NameDeclaredMoveSources pins that every former source default names the source key of a declared key move.
func TestFormerSourceDefaults_NameDeclaredMoveSources(t *testing.T) {
	t.Parallel()
	for module, keys := range formerSourceDefaults {
		for key := range keys {
			declared := slices.ContainsFunc(configreg.KeyMoves(), func(move configreg.KeyMove) bool {
				return move.FromModule == module && move.FromKey == key
			})
			if !declared {
				t.Errorf("formerSourceDefaults names %s.%s, which no declared key move takes as its source", module, key)
			}
		}
	}
}

// TestFormerTemplateValues_DifferFromTheTemplate pins that every former value names a registered module and a unit its template holds, and differs from the template's current value, compared as YAML nodes with the module's open maps.
func TestFormerTemplateValues_DifferFromTheTemplate(t *testing.T) {
	t.Parallel()
	for module := range formerTemplateValues {
		m, ok := configreg.Lookup(module)
		if !ok {
			t.Errorf("formerTemplateValues names module %q, which is not registered", module)
			continue
		}
		spec := m.Spec()
		held, err := yamlengine.LeafPaths(spec.Template, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			t.Fatalf("LeafPaths(%s template): %v", module, err)
		}
		units, err := yamlengine.LeafPaths(formerValuesDoc(module), spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			t.Fatalf("LeafPaths(%s former values): %v", module, err)
		}
		_, equal, err := yamlengine.Strip(spec.Template, formerValuesDoc(module), spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			t.Fatalf("Strip(%s template, former values): %v", module, err)
		}
		for _, unit := range units {
			if !slices.Contains(held, unit) {
				t.Errorf("former value %s.%s: the template holds no such unit", module, unit)
			}
			if slices.Contains(equal, unit) {
				t.Errorf("former value %s.%s equals the template's current value", module, unit)
			}
		}
	}
}

// loomTemplate returns loom's registered template.
func loomTemplate() string {
	template, _ := configreg.Template("loom")
	return template()
}

// loomTemplateWith returns template with the line of the named top-level key replaced by replacement, a whole "key: value" line.
func loomTemplateWith(template, key, replacement string) string {
	lines := strings.Split(template, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, key+":") {
			lines[i] = strings.TrimSuffix(replacement, "\n")
		}
	}
	return strings.Join(lines, "\n")
}

// findLayerResult returns the Result for the named module and layer, or nil if absent.
func findLayerResult(results []Result, module string, layer configengine.Layer) *Result {
	for i := range results {
		if results[i].Module == module && results[i].Layer == layer {
			return &results[i]
		}
	}
	return nil
}

// TestReconcile_FailureNamesItsFile pins that a reconcile failure is a *FileError naming the module and the absolute path of the file that failed, with the path leading its message.
func TestReconcile_FailureNamesItsFile(t *testing.T) {
	t.Parallel()
	const unparseable = "key: [unclosed\n"
	const unknownLensSet = "lens_set: nope\n"
	tests := []struct {
		name       string
		run        func(baseDir, boardDir string) error
		module     string
		wantInBase bool
		hubFile    string
		// content is the failing file's bytes; empty keeps unparseable.
		content string
		// wantErr are substrings the message must hold.
		wantErr []string
		// worktreeFile is written as the module's file in the base dir beside a content file at the board dir.
		worktreeFile string
		// burlerFile is written as the board's burler.yaml.
		burlerFile string
	}{
		{
			name: "ReconcileAll on an unparseable per-worktree file",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileAll(baseDir, boardDir, Options{})
				return err
			},
			module:     "batcher",
			wantInBase: true,
		},
		{
			name: "ReconcileAll on an unparseable shuttle.yaml",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileAll(baseDir, boardDir, Options{})
				return err
			},
			module:     "shuttle",
			wantInBase: true,
		},
		{
			name: "ReconcileHubWideAt on an unparseable hub file",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileHubWideAt(boardDir, "", HubWideOptions{})
				return err
			},
			module: "fabric",
		},
		{
			name: "ReconcileAll retiring an unparseable hub-wide copy",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileAll(baseDir, boardDir, Options{})
				return err
			},
			module:     "board",
			wantInBase: true,
			hubFile:    "readme: README.md\n",
		},
		{
			name: "ReconcileAll standalone on an unknown lens_set names its own file",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileAll(baseDir, "", Options{})
				return err
			},
			module:     "lenses",
			wantInBase: true,
			content:    unknownLensSet,
			wantErr:    []string{"names no shipped layer"},
		},
		{
			name: "ReconcileAll in a hub on a worktree file under an unknown lens_set names the board file",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileAll(baseDir, boardDir, Options{})
				return err
			},
			module:       "lenses",
			content:      unknownLensSet,
			worktreeFile: "fans:\n  mine:\n    - generic\n",
			wantErr:      []string{"names no shipped layer"},
		},
		{
			name: "ReconcileHubWideAt on a hub file holding an unknown lens_set names the hub file",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileHubWideAt(boardDir, "", HubWideOptions{})
				return err
			},
			module:  "lenses",
			content: unknownLensSet,
			wantErr: []string{"names no shipped layer"},
		},
		{
			name: "ReconcileHubWideAt seeding from a prime copy under an unknown lens_set names the prime's copy",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileHubWideAt(boardDir, baseDir, HubWideOptions{SeedDefaults: true})
				return err
			},
			module:     "lenses",
			wantInBase: true,
			content:    unknownLensSet,
			wantErr:    []string{"names no shipped layer", "delete the lens_set line from"},
		},
		{
			name: "a key move seeding the board lenses.yaml from a prime copy under an unknown lens_set names the prime's copy",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileHubWideAt(boardDir, baseDir, HubWideOptions{SeedDefaults: true})
				return err
			},
			module:     "lenses",
			wantInBase: true,
			content:    unknownLensSet,
			burlerFile: "fans:\n  theirs:\n    - generic\n",
			wantErr:    []string{"names no shipped layer", "delete the lens_set line from"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir, boardDir := t.TempDir(), t.TempDir()
			fileDir := boardDir
			if tt.wantInBase {
				fileDir = baseDir
			}
			content := tt.content
			if content == "" {
				content = unparseable
			}
			writeModuleFile(t, fileDir, tt.module, content)
			writeModuleFile(t, boardDir, tt.module, tt.hubFile)
			if tt.worktreeFile != "" {
				writeModuleFile(t, baseDir, tt.module, tt.worktreeFile)
			}
			writeModuleFile(t, boardDir, "burler", tt.burlerFile)

			err := tt.run(baseDir, boardDir)

			var fileErr *FileError
			if !errors.As(err, &fileErr) {
				t.Fatalf("error = %v, want a *FileError", err)
			}
			wantPath := configengine.ConfigFile(fileDir, tt.module)
			if fileErr.Module != tt.module || fileErr.Path != wantPath {
				t.Errorf("FileError{Module: %q, Path: %q}, want module %q path %q", fileErr.Module, fileErr.Path, tt.module, wantPath)
			}
			if !strings.HasPrefix(err.Error(), wantPath+": ") {
				t.Errorf("message = %q, want it to start with %q", err.Error(), wantPath+": ")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message = %q, want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestPrimeSeedError pins that an invalid-config seed failure names the prime's copy, that the extra "delete the selector line" step is added only for a module that has an overlay selector, and that any other failure is returned unchanged.
func TestPrimeSeedError(t *testing.T) {
	t.Parallel()
	invalid := configengine.MarkInvalid(errors.New("names no shipped layer"))
	other := errors.New("disk full")
	tests := []struct {
		name   string
		module string
		err    error
		// wantFileError says the result is a *FileError naming the prime's copy; otherwise it is err itself.
		wantFileError bool
		wantDelete    bool
	}{
		{"InvalidWithSelector", "lenses", invalid, true, true},
		{"InvalidWithoutSelector", "batcher", invalid, true, false},
		{"OtherFailureWithSelector", "lenses", other, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			module, ok := configreg.Lookup(tt.module)
			if !ok {
				t.Fatalf("Lookup(%q) found no module", tt.module)
			}
			primeDir := t.TempDir()

			got := primeSeedError(module, primeDir, tt.err)

			if !tt.wantFileError {
				if got != tt.err {
					t.Errorf("primeSeedError() = %v; want the error unchanged", got)
				}
				return
			}
			var fileErr *FileError
			wantPath := configengine.ConfigFile(primeDir, tt.module)
			if !errors.As(got, &fileErr) || fileErr.Module != tt.module || fileErr.Path != wantPath {
				t.Fatalf("primeSeedError() = %v; want a *FileError for module %q at %q", got, tt.module, wantPath)
			}
			if !errors.Is(got, configengine.ErrInvalid) {
				t.Errorf("primeSeedError() = %v; want it to keep ErrInvalid", got)
			}
			if hasDelete := strings.Contains(got.Error(), "also delete"); hasDelete != tt.wantDelete {
				t.Errorf("primeSeedError() = %q; delete step present = %v, want %v", got, hasDelete, tt.wantDelete)
			}
		})
	}
}

// TestReconcile_MovesKeysAcrossModules pins the declared shuttle-to-claude and burler-to-lenses key moves at every reconcile entry point: where the value lands at each layer, what a collision keeps or overwrites, and that a dry run writes nothing and a second pass moves nothing.
func TestReconcile_MovesKeysAcrossModules(t *testing.T) {
	t.Parallel()
	claudeTemplate, _ := configreg.Template("claude")
	claudeModule, _ := configreg.Lookup("claude")
	const templateBinary = "${env:LYX_SHUTTLE_CLAUDE:-}"
	const tunedGeneric = "Tuned for the hub."
	const formerRoles = "prompt_cache_ttl_roles:\n  driver: 1h\n  webster: 1h\n"
	burlerToday := burlerFixture(t, func(cfg *lenses.Config) {
		delete(cfg.Lenses, "security")
		delete(cfg.Fans, "full")
		cfg.Lenses["generic"] = tunedGeneric + "\n"
	})
	burlerOldTemplate := burlerFixture(t, func(cfg *lenses.Config) {
		cfg.Fans["twill-discussion"] = cfg.Fans["twill-discussion"][:6]
	})
	lensesEqualTemplate := lensesFixture(t, func(cfg *lenses.Config) {})
	lensesWithHubLens := lensesFixture(t, func(cfg *lenses.Config) {
		cfg.Lenses["hub-lens"] = "Hub emphasis.\n"
	})
	const sparseFans = "fans:\n  mine:\n    - generic\n"
	const burlerFans = "fans:\n  theirs:\n    - generic\n"
	csharpConcurrency := loadLensesWith(t, "lens_set: csharp\n").Lenses["concurrency"]
	keptLenses := []string{"burler.fans dropped, lenses.fans kept", "burler.lenses dropped, lenses.lenses kept"}
	movedLenses := []string{"burler.fans -> lenses.fans", "burler.lenses -> lenses.lenses"}
	tests := []struct {
		name string
		// files maps "<base|board|prime|local>:<module>" to the file's content.
		files map[string]string
		// call is "hub" (ReconcileAll over a board dir), "standalone" (ReconcileAll without one) or "board" (ReconcileHubWideAt seeding from the prime).
		call   string
		dryRun bool
		repeat bool
		// boardMarker writes the board's base-sparse marker before the run, and primeMarked the prime's sparse marker; their files keys name module files only.
		boardMarker bool
		primeMarked bool
		// contains and excludes map a file key to substrings its content must and must not hold after the run.
		contains map[string][]string
		excludes map[string][]string
		absent   []string
		// unchanged lists file keys whose content after the run equals the content before.
		unchanged []string
		// moved, kept and overwritten are the exact report lines the run must produce across all its results.
		moved, kept, overwritten []string
		// added lists lines each of which must appear in some result's Added.
		added []string
		// noAdded lists modules none of whose results carries an Added line.
		noAdded []string
		// stripped lists entries some result's Stripped must hold; nil skips the check.
		stripped []string
		// loadContains is what claude's resolved base+worktree stack must hold after the run.
		loadContains string
		// checkLoadRefuses is the file name claude's loader must refuse naming after the run.
		checkLoadRefuses string
	}{
		{
			name:     "a repo default moves into a created claude.yaml holding only the moved unit and leaves its emptied source as the header-only base",
			files:    map[string]string{"board:shuttle": "claude: /x\n"},
			call:     "board",
			contains: map[string][]string{"board:claude": {"binary: /x"}, "board:shuttle": {configengine.SparseBaseHeader}},
			excludes: map[string][]string{"board:claude": {"deny_agent_tool"}, "board:shuttle": {"claude: /x"}},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:     "a hub's tuned darn perch keys move to loom's Code-Review keys",
			files:    map[string]string{"board:darn": "review_max_bounces: 0\ndarn_fixer_fan: fix-plan\n"},
			call:     "board",
			contains: map[string][]string{"board:loom": {"code_max_bounces: 0", "code_fixer_fan: fix-plan"}, "board:darn": {configengine.SparseBaseHeader}},
			excludes: map[string][]string{"board:darn": {"review_max_bounces", "darn_fixer_fan"}},
			moved:    []string{"darn.darn_fixer_fan -> loom.code_fixer_fan", "darn.review_max_bounces -> loom.code_max_bounces"},
		},
		{
			name: "a hub's tuned darn model, fan and amend keys move to loom's Code-Review keys",
			files: map[string]string{"board:darn": "darn_reviewer_model: opus[high]\ndarn_fixer_model: opus[low]\ndarn_reviewer_fan: review-plan\n" +
				"review_fan: review-plan\nreview_fix_fan: fix-plan\nreview_hard_max_bounces: 3\nreview_amend_max_bounces: 0\n"},
			call: "board",
			contains: map[string][]string{"board:loom": {"code_reviewer_model: opus[high]", "code_fixer_model: opus[low]", "code_reviewer_fan: review-plan",
				"code_fixer_fan: fix-plan", "code_hard_max_bounces: 3", "review_amend_max_bounces: 0"}, "board:darn": {configengine.SparseBaseHeader}},
			moved: []string{
				"darn.darn_fixer_model -> loom.code_fixer_model", "darn.darn_reviewer_fan -> loom.code_reviewer_fan", "darn.darn_reviewer_model -> loom.code_reviewer_model",
				"darn.review_amend_max_bounces -> loom.review_amend_max_bounces", "darn.review_fix_fan -> loom.code_fixer_fan",
				"darn.review_hard_max_bounces -> loom.code_hard_max_bounces",
			},
			kept: []string{"darn.review_fan: review-plan dropped, loom.code_reviewer_fan: review-plan kept"},
		},
		{
			name:     "an untuned darn.yaml's former defaults are dropped and leave loom's Code-Review keys as they are",
			files:    map[string]string{"board:darn": "darn_reviewer_model: sonnet[high]\nreview_hard_max_bounces: 2\ndarn_fixer_fan: \"\"\nreview_fan: \"\"\n", "board:loom": "code_hard_max_bounces: 0\ncode_reviewer_model: sonnet[medium]\ncode_max_bounces: 1\n"},
			call:     "board",
			contains: map[string][]string{"board:loom": {"code_hard_max_bounces: 0", "code_reviewer_model: sonnet[medium]"}, "board:darn": {configengine.SparseBaseHeader}},
			excludes: map[string][]string{"board:loom": {"sonnet[high]", "code_hard_max_bounces: 2", "code_fixer_fan: \"\""}},
			kept: []string{
				"darn.darn_fixer_fan:  dropped, a former template default", "darn.review_fan:  dropped, a former template default", "darn.darn_reviewer_model: sonnet[high] dropped, a former template default",
				"darn.review_hard_max_bounces: 2 dropped, a former template default",
			},
		},
		{
			name:     "a former default dropped from a prime-seeded darn.yaml is reported though the move writes no file",
			files:    map[string]string{"prime:darn": "review_hard_max_bounces: 2\n"},
			call:     "board",
			excludes: map[string][]string{"board:loom": {"code_hard_max_bounces: 2"}},
			kept:     []string{"darn.review_hard_max_bounces: 2 dropped, a former template default"},
		},
		{
			name:     "a loom.yaml's tuned code value is kept over a darn value at a base layer",
			files:    map[string]string{"base:darn": "review_max_bounces: 0\n", "base:loom": "code_max_bounces: 2\n"},
			call:     "standalone",
			contains: map[string][]string{"base:loom": {"code_max_bounces: 2"}},
			absent:   []string{"base:darn"},
			kept:     []string{"darn.review_max_bounces: 0 dropped, loom.code_max_bounces: 2 kept"},
		},
		{
			name:        "a loom.yaml's tuned code value is kept over a darn value at a sparse layer",
			files:       map[string]string{"board:darn": "review_max_bounces: 0\n", "board:loom": "code_max_bounces: 2\n"},
			boardMarker: true,
			call:        "board",
			contains:    map[string][]string{"board:loom": {"code_max_bounces: 2"}},
			excludes:    map[string][]string{"board:darn": {"review_max_bounces"}},
			kept:        []string{"darn.review_max_bounces: 0 dropped, loom.code_max_bounces: 2 kept"},
		},
		{
			name:     "a board default seeded from the prime's shuttle.yaml carries the moved value and the prime's non-template units",
			files:    map[string]string{"prime:shuttle": "claude: /x\n", "prime:claude": "deny_agent_tool: false\nprompt_cache_ttl: 5m\n"},
			call:     "board",
			contains: map[string][]string{"board:claude": {"binary: /x", "deny_agent_tool: false"}, "prime:shuttle": {"claude: /x"}},
			excludes: map[string][]string{"board:claude": {"prompt_cache_ttl"}, "board:shuttle": {"claude: /x"}},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:     "a board target seeded from an unmarked prime drops a former template value",
			files:    map[string]string{"board:shuttle": "claude: /x\n", "prime:claude": formerRoles},
			call:     "board",
			contains: map[string][]string{"board:claude": {"binary: /x"}},
			excludes: map[string][]string{"board:claude": {"prompt_cache_ttl_roles"}},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:        "a board target seeded from a marked prime keeps a former template value as tuned",
			files:       map[string]string{"board:shuttle": "claude: /x\n", "prime:claude": formerRoles},
			primeMarked: true,
			call:        "board",
			contains:    map[string][]string{"board:claude": {"binary: /x", "prompt_cache_ttl_roles"}},
			moved:       []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:        "a board base target holding the template default is overwritten while the board is unmarked",
			files:       map[string]string{"board:shuttle": "claude: /x\n", "board:claude": "binary: " + templateBinary + "\n"},
			call:        "board",
			contains:    map[string][]string{"board:claude": {"binary: /x"}},
			moved:       []string{"shuttle.claude -> claude.binary"},
			overwritten: []string{"shuttle.claude: /x replaced claude.binary's template default " + templateBinary},
		},
		{
			name:        "a board base target holding the template default keeps it as a pin once the board is marked",
			files:       map[string]string{"board:shuttle": "claude: /x\n", "board:claude": "binary: " + templateBinary + "\n"},
			boardMarker: true,
			call:        "board",
			contains:    map[string][]string{"board:claude": {templateBinary}},
			excludes:    map[string][]string{"board:claude": {"/x"}},
			kept:        []string{"shuttle.claude: /x dropped, claude.binary: " + templateBinary + " kept"},
		},
		{
			name:     "a worktree layer in a hub moves into a created sparse claude.yaml",
			files:    map[string]string{"base:shuttle": "claude: /x\n"},
			call:     "hub",
			repeat:   true,
			contains: map[string][]string{"base:claude": {"binary: /x"}},
			excludes: map[string][]string{"base:claude": {"deny_agent_tool"}},
			absent:   []string{"base:shuttle"},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:     "a local layer moves into a created sparse claude.local.yaml and the emptied source is removed",
			files:    map[string]string{"local:shuttle": "claude_prompt_cache_ttl: 1h\n"},
			call:     "hub",
			contains: map[string][]string{"local:claude": {"prompt_cache_ttl: 1h"}},
			absent:   []string{"local:shuttle"},
			moved:    []string{"shuttle.claude_prompt_cache_ttl -> claude.prompt_cache_ttl"},
		},
		{
			name:         "a base target holding the template default is overwritten",
			files:        map[string]string{"base:shuttle": "claude: /x\n", "base:claude": claudeTemplate()},
			call:         "standalone",
			contains:     map[string][]string{"base:claude": {"binary: /x"}},
			moved:        []string{"shuttle.claude -> claude.binary"},
			overwritten:  []string{"shuttle.claude: /x replaced claude.binary's template default " + templateBinary},
			loadContains: "binary: /x",
		},
		{
			name:     "a standalone worktree creates a full claude.yaml from the template",
			files:    map[string]string{"base:shuttle": "claude: /x\n"},
			call:     "standalone",
			contains: map[string][]string{"base:claude": {"binary: /x", "deny_agent_tool"}},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:     "a base target holding its own value keeps it",
			files:    map[string]string{"base:shuttle": "claude: /x\n", "base:claude": "binary: /keep\n"},
			call:     "standalone",
			contains: map[string][]string{"base:claude": {"binary: /keep"}},
			excludes: map[string][]string{"base:claude": {"/x"}},
			kept:     []string{"shuttle.claude: /x dropped, claude.binary: /keep kept"},
		},
		{
			name:     "a sparse target holding the template default keeps it",
			files:    map[string]string{"base:shuttle": "claude: /x\n", "base:claude": "binary: " + templateBinary + "\n"},
			call:     "hub",
			contains: map[string][]string{"base:claude": {templateBinary}},
			excludes: map[string][]string{"base:claude": {"/x"}},
			kept:     []string{"shuttle.claude: /x dropped, claude.binary: " + templateBinary + " kept"},
		},
		{
			name:     "a dry run writes nothing and reports the pending move",
			files:    map[string]string{"base:shuttle": "claude: /x\n"},
			call:     "hub",
			dryRun:   true,
			contains: map[string][]string{"base:shuttle": {"claude: /x"}},
			absent:   []string{"base:claude"},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:             "a local ttl map is moved verbatim and then refused by claude's loader",
			files:            map[string]string{"board:claude": claudeTemplate(), "base:reed": "", "local:shuttle": "claude_prompt_cache_ttl:\n  a: 1\n"},
			call:             "hub",
			contains:         map[string][]string{"local:claude": {"a: 1"}},
			absent:           []string{"local:shuttle"},
			moved:            []string{"shuttle.claude_prompt_cache_ttl -> claude.prompt_cache_ttl"},
			checkLoadRefuses: "claude.local.yaml",
		},
		{
			name:     "a board burler.yaml of today's shape moves into a created lenses.yaml that reconcile cuts to its tuned entries",
			files:    map[string]string{"board:burler": burlerToday},
			call:     "board",
			contains: map[string][]string{"board:lenses": {tunedGeneric, "fans: {}"}},
			excludes: map[string][]string{"board:lenses": {"twill-discussion:", "security:", "full:", "correctness:"}},
			absent:   []string{"board:burler"},
			moved:    movedLenses,
			noAdded:  []string{"lenses"},
		},
		{
			name:     "a board dry run writes nothing and reports no seed additions for the lenses.yaml the moves would write",
			files:    map[string]string{"board:burler": burlerToday},
			call:     "board",
			dryRun:   true,
			contains: map[string][]string{"board:burler": {tunedGeneric}},
			absent:   []string{"board:lenses"},
			moved:    movedLenses,
			noAdded:  []string{"lenses"},
		},
		{
			name:     "a lenses.yaml equal to the template is kept over an old-template burler.yaml and cut to empty maps",
			files:    map[string]string{"board:lenses": lensesEqualTemplate, "board:burler": burlerOldTemplate},
			call:     "board",
			contains: map[string][]string{"board:lenses": {"lenses: {}", "fans: {}"}},
			absent:   []string{"board:burler"},
			kept:     keptLenses,
		},
		{
			name:     "a non-default board lenses.yaml is kept over a burler.yaml and cut to its hub-only lens",
			files:    map[string]string{"board:lenses": lensesWithHubLens, "board:burler": burlerToday},
			call:     "board",
			contains: map[string][]string{"board:lenses": {"hub-lens:", "fans: {}"}},
			excludes: map[string][]string{"board:lenses": {tunedGeneric, "Review broadly with no single area of emphasis"}},
			absent:   []string{"board:burler"},
			kept:     keptLenses,
		},
		{
			name:      "a sparse lenses.yaml on a hub's worktree layer is kept beside a burler.yaml",
			files:     map[string]string{"base:lenses": sparseFans, "base:burler": burlerFans},
			call:      "hub",
			absent:    []string{"base:burler"},
			unchanged: []string{"base:lenses"},
			kept:      []string{"burler.fans dropped, lenses.fans kept"},
		},
		{
			name:     "a board burler.yaml seeded from a prime lenses.yaml keeps the prime's fans and moves burler's lenses",
			files:    map[string]string{"board:burler": burlerToday, "prime:lenses": sparseFans},
			call:     "board",
			contains: map[string][]string{"board:lenses": {tunedGeneric, "mine:"}},
			absent:   []string{"board:burler"},
			moved:    []string{"burler.lenses -> lenses.lenses"},
			kept:     []string{"burler.fans dropped, lenses.fans kept"},
		},
		{
			name: "moved lenses are judged under the lens_set of the lenses.yaml the move creates from the prime's seed",
			files: map[string]string{
				"prime:lenses": "lens_set: csharp\n",
				"board:burler": lensSetFixture(t, "", map[string]string{"concurrency": csharpConcurrency, "hub-lens": "Hub emphasis.\n"}),
			},
			call:     "board",
			contains: map[string][]string{"board:lenses": {"lens_set: csharp", "hub-lens:"}},
			excludes: map[string][]string{"board:lenses": {"concurrency:"}},
			stripped: []string{"lenses.concurrency"},
			moved:    []string{"burler.lenses -> lenses.lenses"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := map[string]string{"base": t.TempDir(), "board": t.TempDir(), "prime": t.TempDir()}
			pathOf := func(key string) string {
				kind, module, _ := strings.Cut(key, ":")
				if kind == "local" {
					return configengine.LocalFile(dirs["base"], module)
				}
				return configengine.ConfigFile(dirs[kind], module)
			}
			for key, content := range tt.files {
				path := pathOf(key)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir for %s: %v", key, err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", key, err)
				}
			}
			if tt.boardMarker {
				if err := WriteBaseSparseMarker(dirs["board"]); err != nil {
					t.Fatalf("WriteBaseSparseMarker: %v", err)
				}
			}
			if tt.primeMarked {
				if err := os.WriteFile(configengine.SparseMarker(dirs["prime"]), []byte("# sparse\n"), 0o644); err != nil {
					t.Fatalf("write the prime's sparse marker: %v", err)
				}
			}
			run := func() []Result {
				var results []Result
				var err error
				switch tt.call {
				case "hub":
					results, err = ReconcileAll(dirs["base"], dirs["board"], Options{Apply: !tt.dryRun})
				case "standalone":
					results, err = ReconcileAll(dirs["base"], "", Options{Apply: !tt.dryRun})
				case "board":
					results, err = ReconcileHubWideAt(dirs["board"], dirs["prime"], HubWideOptions{Apply: !tt.dryRun, SeedDefaults: true})
				}
				if err != nil {
					t.Fatalf("reconcile: %v", err)
				}
				return results
			}
			lines := func(results []Result, pick func(Result) []string) []string {
				var all []string
				for _, result := range results {
					for _, line := range pick(result) {
						if !slices.Contains(all, line) {
							all = append(all, line)
						}
					}
				}
				slices.Sort(all)
				return all
			}
			wantLines := func(want []string) []string {
				slices.Sort(want)
				return want
			}

			results := run()

			for name, got := range map[string][]string{
				"Moved":       lines(results, func(r Result) []string { return r.Moved }),
				"KeptTarget":  lines(results, func(r Result) []string { return r.KeptTarget }),
				"Overwritten": lines(results, func(r Result) []string { return r.Overwritten }),
			} {
				want := map[string][]string{"Moved": tt.moved, "KeptTarget": tt.kept, "Overwritten": tt.overwritten}[name]
				if !slices.Equal(got, wantLines(slices.Clone(want))) {
					t.Errorf("%s lines = %q; want %q", name, got, want)
				}
			}
			for key, wants := range tt.contains {
				content, err := os.ReadFile(pathOf(key))
				if err != nil {
					t.Fatalf("read %s: %v", key, err)
				}
				for _, want := range wants {
					if !strings.Contains(string(content), want) {
						t.Errorf("%s = %q; want it to contain %q", key, content, want)
					}
				}
			}
			for key, unwanted := range tt.excludes {
				content, err := os.ReadFile(pathOf(key))
				if err != nil {
					t.Fatalf("read %s: %v", key, err)
				}
				for _, bad := range unwanted {
					if strings.Contains(string(content), bad) {
						t.Errorf("%s = %q; want it without %q", key, content, bad)
					}
				}
			}
			for _, key := range tt.absent {
				if _, err := os.Stat(pathOf(key)); !os.IsNotExist(err) {
					t.Errorf("%s exists (stat err = %v); want it absent", key, err)
				}
			}
			addedLines := lines(results, func(r Result) []string { return r.Added })
			for _, want := range tt.added {
				if !slices.Contains(addedLines, want) {
					t.Errorf("Added lines = %q; want one to be %q", addedLines, want)
				}
			}
			strippedLines := lines(results, func(r Result) []string { return r.Stripped })
			for _, want := range tt.stripped {
				if !slices.Contains(strippedLines, want) {
					t.Errorf("Stripped lines = %q; want one to be %q", strippedLines, want)
				}
			}
			for _, module := range tt.noAdded {
				for _, result := range results {
					if result.Module == module && len(result.Added) > 0 {
						t.Errorf("module %s Added = %q; want none", module, result.Added)
					}
				}
			}
			for _, key := range tt.unchanged {
				content, err := os.ReadFile(pathOf(key))
				if err != nil {
					t.Fatalf("read %s: %v", key, err)
				}
				if string(content) != tt.files[key] {
					t.Errorf("%s = %q after the run; want it unchanged from %q", key, content, tt.files[key])
				}
			}
			if tt.loadContains != "" {
				loaded, err := configengine.Load(configengine.Dirs{Anchor: dirs["base"]}, claudeModule.Spec())
				if err != nil {
					t.Fatalf("Load claude: %v", err)
				}
				if !strings.Contains(string(loaded), tt.loadContains) {
					t.Errorf("claude resolves to %q; want it to contain %q", loaded, tt.loadContains)
				}
			}
			if tt.checkLoadRefuses != "" {
				err := claudeModule.CheckLoad(configengine.Dirs{Anchor: dirs["base"], Board: dirs["board"]})
				if err == nil || !strings.Contains(err.Error(), tt.checkLoadRefuses) {
					t.Errorf("claude CheckLoad error = %v; want it to name %q", err, tt.checkLoadRefuses)
				}
			}
			if tt.repeat {
				if again := lines(run(), func(r Result) []string { return r.Moved }); len(again) > 0 {
					t.Errorf("second pass Moved = %q; want none", again)
				}
			}
		})
	}
}
