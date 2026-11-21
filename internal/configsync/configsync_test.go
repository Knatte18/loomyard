// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"errors"
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
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// loadLensesAt returns lenses.Load over dirs, failing the test on an error.
func loadLensesAt(t *testing.T, dirs configengine.Dirs) lenses.Config {
	t.Helper()
	cfg, err := lenses.Load(dirs)
	if err != nil {
		t.Fatalf("lenses.Load(%+v): %v", dirs, err)
	}
	return cfg
}

// lensSetFixture returns a sparse lenses.yaml document holding the given lens entries.
// It sets lens_set to lensSet, and leaves it out when lensSet is empty.
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
	// loadsAsBatcher pins that batcher.Active loads the file after the run.
	loadsAsBatcher bool
	// wantLoadedLenses is what lenses.Load resolves after the run; nil skips the check.
	wantLoadedLenses *lenses.Config
	// loadUnchanged pins that lenses.Load over the base dir resolves the same before and after the run.
	loadUnchanged bool
}

// TestReconcileAll pins what a run does to each module's file and Result.
// A dry run reports and writes nothing.
// An apply adds the template's missing keys and prunes stale ones.
// An absent file is seeded from the template.
// A seed-only module (such as `models`) is materialized verbatim exactly once and never rewritten:
// not even to resurrect a key the operator removed.
// A non-seed-only module in the same run is still pruned.
//
//testtiming:keep pins the dry run's untouched bytes and each module's Applied, Removed, KeptExtra and Dropped result, which the idempotence and failure tests never assert
func TestReconcileAll(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const loomStale = "discussion_timeout_min: 480\nstale_key: old_value\n"
	const modelsAlias = "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
	const modelsTrimmed = "sonnet:\n  engine: claude\n  model: sonnet\n"
	loggerTemplate := loggerconfig.ConfigTemplate()
	modelsTemplate := modelspec.ConfigTemplate()
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
			name:  "absent batcher file is seeded from the template",
			apply: true,
			want:  map[string]moduleWant{"batcher": {applied: true, fileContains: []string{"profiles: {}"}, loadsAsBatcher: true}},
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
		{
			name:  "apply prunes the retired beam keys test_map_every and test_map_upkeep",
			seed:  map[string]string{"beam": "test_map_every: 0\ntest_map_upkeep: false\n"},
			apply: true,
			want: map[string]moduleWant{"beam": {
				applied:         true,
				removedAll:      []string{"test_map_every", "test_map_upkeep"},
				fileNotContains: []string{"test_map_every", "test_map_upkeep"},
			}},
		},
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
	tiergate.Require(t, 0)
	t.Parallel()
	tmpDir := t.TempDir()
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

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

// TestReconcileHubWideAt_SeedsFromHubPrimeOrTemplate pins where each hub-wide module's reconcile starts from:
// a present hub file is never replaced;
// an absent board.yaml or landing.yaml is seeded from the prime's copy when it has one,
// and an absent fabric.yaml never reads the prime.
func TestReconcileHubWideAt_SeedsFromHubPrimeOrTemplate(t *testing.T) {
	tiergate.Require(t, 0)
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
		seedDefaults bool
		pruneUnknown bool
		// wantNoFile pins that the file is still absent after an apply.
		wantNoFile   bool
		wantSeed     string
		wantContains []string
		wantAbsent   []string
		// withTemplateBase writes the module's current template as the template base beside the hub file.
		withTemplateBase bool
		// wantAdded and wantKeptExtra are the applied result's Added and KeptExtra, compared sorted; nil skips the check.
		wantAdded     []string
		wantKeptExtra []string
		// wantAddedHas names entries the applied result's Added must hold.
		wantAddedHas []string
		// wantDropped and wantNotDropped are names the applied result's Dropped must and must not hold.
		wantDropped    []string
		wantNotDropped []string
		// loadUnchanged pins that lenses.Load over the board dir resolves the same before the dry run and after the apply.
		loadUnchanged bool
		// wantFileError means both the dry run and the apply return a *FileError naming the module;
		// the hub file's bytes stay as they were.
		wantFileError bool
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
			name:         "loom default absent and no prime copy starts as the header-only base",
			module:       "loom",
			seedDefaults: true,
			wantSeed:     SeedTemplate,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"review_max_bounces:"},
		},
		{
			name:         "loom default present keeps a key the template lacks and gains no template key",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\npr_review: old\n",
			primeFile:    "discussion_timeout_min: 480\n",
			seedDefaults: true,
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7", "pr_review: old"},
			wantAbsent:   []string{"480", "review_max_bounces:"},
			wantAdded:    []string{},
		},
		{
			name:         "loom default present keeps a key the template lacks without SeedDefaults",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\npr_review: old\n",
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7", "pr_review: old"},
			wantAbsent:   []string{"review_max_bounces:"},
		},
		{
			name:         "loom default is pruned of a key the template lacks under PruneUnknown",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\npr_review: old\n",
			pruneUnknown: true,
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7"},
			wantAbsent:   []string{"pr_review", "review_max_bounces:"},
		},
		{
			name:         "loom default left with no unit by a prune is written as the header-only base",
			module:       "loom",
			hubFile:      "pr_review: old\n",
			pruneUnknown: true,
			wantSeed:     SeedHub,
			wantContains: []string{configengine.SparseBaseHeader},
			wantAbsent:   []string{"pr_review"},
		},
		{
			name:       "loom default absent stays absent without SeedDefaults",
			module:     "loom",
			primeFile:  "discussion_timeout_min: 480\n",
			wantNoFile: true,
		},
		{
			name:         "models default absent is seeded from the prime and gains the template aliases it lacks",
			module:       "models",
			primeFile:    "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n",
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"zephyr:\n  engine: claude\n  model: claude-zephyr-1\n", "sonnet:"},
		},
		{
			name:             "models default absent seeded from the prime gains the template aliases it lacks although a template base holds them",
			module:           "models",
			primeFile:        "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n",
			seedDefaults:     true,
			withTemplateBase: true,
			wantSeed:         SeedPrime,
			wantAddedHas:     []string{"sonnet"},
			wantContains:     []string{"zephyr:\n  engine: claude\n  model: claude-zephyr-1\n", "sonnet:"},
		},
		{
			name:         "models default present keeps its own alias and gains the template aliases it lacks",
			module:       "models",
			hubFile:      "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n",
			seedDefaults: true,
			wantSeed:     SeedHub,
			wantContains: []string{"zephyr:\n  engine: claude\n  model: claude-zephyr-1\n", "sonnet:"},
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
			wantContains: []string{"defaults:", "light:", "solid:", "docs:"},
		},
		{
			name:         "profiles present with no template base gain every template unit they lack and keep their own profile",
			module:       "profiles",
			hubFile:      "broad:\n  darn:\n    loom: {code_max_bounces: 2}\n",
			wantSeed:     SeedHub,
			wantContains: []string{"broad:\n  darn:\n    loom: {code_max_bounces: 2}\n", "defaults:", "light:", "solid:", "docs:"},
		},
		{
			name:          "profiles present that fails to parse is refused and left untouched",
			module:        "profiles",
			hubFile:       "light: [\n",
			wantFileError: true,
		},
		{
			name:         "darn present keeps its tree_scope trees and their targets",
			module:       "darn",
			hubFile:      "tree_scope:\n  tools: ./tools/...\n  docs: \"\"\n",
			wantSeed:     SeedHub,
			wantContains: []string{"tools: ./tools/...", "docs: \"\""},
			wantAbsent:   []string{"review_max_bounces:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			primeDir := t.TempDir()
			writeModuleFile(t, boardDir, tt.module, tt.hubFile)
			writeModuleFile(t, primeDir, tt.module, tt.primeFile)
			if tt.withTemplateBase {
				m, ok := configreg.Lookup(tt.module)
				if !ok {
					t.Fatalf("module %q is not registered", tt.module)
				}
				basePath := configengine.TemplateBaseFile(boardDir, tt.module)
				if err := os.MkdirAll(filepath.Dir(basePath), 0o755); err != nil {
					t.Fatalf("mkdir for the template base: %v", err)
				}
				if err := os.WriteFile(basePath, templateBaseContent(m), 0o644); err != nil {
					t.Fatalf("write the template base: %v", err)
				}
			}
			hubPath := configengine.ConfigFile(boardDir, tt.module)
			options := HubWideOptions{SeedDefaults: tt.seedDefaults, PruneUnknown: tt.pruneUnknown}
			var loadedBefore lenses.Config
			if tt.loadUnchanged {
				loadedBefore = loadLensesAt(t, configengine.Dirs{Board: boardDir})
			}

			if tt.wantFileError {
				for _, apply := range []bool{false, true} {
					options.Apply = apply
					_, err := ReconcileHubWideAt(boardDir, primeDir, options)
					var fileErr *FileError
					if !errors.As(err, &fileErr) || fileErr.Module != tt.module {
						t.Errorf("ReconcileHubWideAt(apply=%v) error = %v; want a *FileError naming module %q", apply, err, tt.module)
					}
				}
				if got, err := os.ReadFile(hubPath); err != nil || string(got) != tt.hubFile {
					t.Errorf("hub file = %q, %v after the refused reconcile; want %q unchanged", got, err, tt.hubFile)
				}
				return
			}

			dry, err := ReconcileHubWideAt(boardDir, primeDir, options)
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

			options.Apply = true
			applied, err := ReconcileHubWideAt(boardDir, primeDir, options)
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
			for _, name := range tt.wantAddedHas {
				if !slices.Contains(appliedResult.Added, name) {
					t.Errorf("applied Added = %v; want it to contain %q", appliedResult.Added, name)
				}
			}
			if tt.wantKeptExtra != nil && !slices.Equal(sortedCopy(appliedResult.KeptExtra), tt.wantKeptExtra) {
				t.Errorf("applied KeptExtra = %v; want %v", appliedResult.KeptExtra, tt.wantKeptExtra)
			}
			if tt.module == "profiles" && !appliedResult.WroteTemplateBase {
				t.Errorf("applied WroteTemplateBase = false; want the template base written beside a board that held none")
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

// TestReconcileHubWideAt_SeedOnlyFileFollowsItsTemplate pins how a present profiles.yaml follows its template.
// The reference is the template base beside the hub file:
// a key left at the base's value takes the template's change; a key the operator set or removed stays;
// a dry run writes nothing; an apply writes an absent or stale base and leaves nothing for the next apply.
func TestReconcileHubWideAt_SeedOnlyFileFollowsItsTemplate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const (
		lightDarn            = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_model: \"opus[medium]\"}\n"
		lightDarnSolo        = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_fan: \"\", code_reviewer_model: \"opus[medium]\"}\n"
		lightDarnOperator    = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_fan: standard, code_reviewer_model: \"opus[medium]\"}\n"
		lightDarnNoModel     = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2}\n"
		lightDarnOwnModel    = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_model: \"sonnet[high]\"}\n"
		lightSamplerReview   = "    loom: {code_max_bounces: 1, code_hard_max_bounces: 2}\n    sampler: {code_review: true}\n"
		lightSamplerNoReview = "    loom: {code_max_bounces: 1, code_hard_max_bounces: 2}\n"
	)
	template := runprofile.ConfigTemplate()
	older := strings.Replace(template, lightDarn, lightDarnSolo, 1)
	withoutSamplerReview := strings.Replace(template, lightSamplerReview, lightSamplerNoReview, 1)
	withoutReviewerModel := strings.Replace(template, lightDarn, lightDarnNoModel, 1)
	m, ok := configreg.Lookup(runprofile.ModuleName)
	if !ok {
		t.Fatalf("module %q is not registered", runprofile.ModuleName)
	}
	currentBase := string(templateBaseContent(m))

	tests := []struct {
		name    string
		hubFile string
		// baseFile is the template base beside the hub file; empty leaves it absent.
		baseFile string
		// baseIsDir makes the template base a directory, so reading it fails.
		baseIsDir bool
		// wantBaseError, when set, expects both runs to fail with a FileError naming the template base;
		// the error holds this text, and the hub file stays untouched.
		wantBaseError string
		// wantAdded nil skips the Added check, and wantAddedHas lists paths Added must hold.
		wantAdded     []string
		wantAddedHas  []string
		wantUpdated   []string
		wantRemoved   []string
		wantDiffers   []string
		wantContains  []string
		wantAbsent    []string
		wantRewritten bool
	}{
		{
			name:          "a key left at the base's value follows the template's removal",
			hubFile:       older,
			baseFile:      older,
			wantAdded:     []string{},
			wantRemoved:   []string{"light.darn.loom.code_reviewer_fan"},
			wantContains:  []string{lightDarn},
			wantAbsent:    []string{lightDarnSolo},
			wantRewritten: true,
		},
		{
			name:         "a key the operator set is kept and listed when the template moved it",
			hubFile:      strings.Replace(template, lightDarn, lightDarnOperator, 1),
			baseFile:     older,
			wantAdded:    []string{},
			wantDiffers:  []string{"light.darn.loom.code_reviewer_fan: file standard, template absent"},
			wantContains: []string{lightDarnOperator},
		},
		{
			name:          "an unedited file without the reviewer model gains the template's",
			hubFile:       withoutReviewerModel,
			baseFile:      withoutReviewerModel,
			wantAddedHas:  []string{"light.darn.loom.code_reviewer_model"},
			wantContains:  []string{lightDarn},
			wantRewritten: true,
		},
		{
			name:         "an operator's own reviewer model is kept and listed",
			hubFile:      strings.Replace(template, lightDarn, lightDarnOwnModel, 1),
			baseFile:     withoutReviewerModel,
			wantAdded:    []string{},
			wantDiffers:  []string{"light.darn.loom.code_reviewer_model: file \"sonnet[high]\", template \"opus[medium]\""},
			wantContains: []string{lightDarnOwnModel},
		},
		{
			name:          "a key the template added since the base is added",
			hubFile:       withoutSamplerReview,
			baseFile:      withoutSamplerReview,
			wantAdded:     []string{"light.sampler.sampler"},
			wantContains:  []string{lightSamplerReview},
			wantRewritten: true,
		},
		{
			name:      "a key the operator removed stays removed",
			hubFile:   withoutSamplerReview,
			baseFile:  template,
			wantAdded: []string{},
		},
		{
			name:          "with no base a lacking key is added and a differing one kept and listed",
			hubFile:       "light:\n  darn:\n    loom: {code_max_bounces: 4}\n",
			wantAddedHas:  []string{"light.darn.loom.code_hard_max_bounces", "solid"},
			wantDiffers:   []string{"light.darn.loom.code_max_bounces: file 4, template 1"},
			wantContains:  []string{"    loom: {code_max_bounces: 4, code_hard_max_bounces: 2, code_reviewer_model: ", "solid:"},
			wantRewritten: true,
		},
		{
			name:      "a file and base that hold the template change nothing",
			hubFile:   template,
			baseFile:  currentBase,
			wantAdded: []string{},
		},
		{
			name:          "a template base that is not a YAML mapping fails naming the base and the way forward",
			hubFile:       template,
			baseFile:      "- not\n- a mapping\n",
			wantBaseError: "delete it",
		},
		{
			name:          "an unreadable template base fails naming the base",
			hubFile:       template,
			baseIsDir:     true,
			wantBaseError: "read the template base",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			writeModuleFile(t, boardDir, runprofile.ModuleName, tt.hubFile)
			hubPath := configengine.ConfigFile(boardDir, runprofile.ModuleName)
			basePath := configengine.TemplateBaseFile(boardDir, runprofile.ModuleName)
			if tt.baseFile != "" {
				if err := os.WriteFile(basePath, []byte(tt.baseFile), 0o644); err != nil {
					t.Fatalf("write the template base: %v", err)
				}
			}
			if tt.baseIsDir {
				if err := os.Mkdir(basePath, 0o755); err != nil {
					t.Fatalf("make the template base a directory: %v", err)
				}
			}
			if tt.wantBaseError != "" {
				for _, apply := range []bool{false, true} {
					_, err := ReconcileHubWideAt(boardDir, "", HubWideOptions{Apply: apply})
					var fileErr *FileError
					if !errors.As(err, &fileErr) || fileErr.Path != basePath || !strings.Contains(err.Error(), tt.wantBaseError) {
						t.Errorf("ReconcileHubWideAt(apply=%v) error = %v; want a *FileError naming %s and holding %q", apply, err, basePath, tt.wantBaseError)
					}
				}
				if hub, err := os.ReadFile(hubPath); err != nil || string(hub) != tt.hubFile {
					t.Errorf("hub file = %q, %v after the failed runs; want it untouched", hub, err)
				}
				return
			}

			for _, apply := range []bool{false, true} {
				results, err := ReconcileHubWideAt(boardDir, "", HubWideOptions{Apply: apply})
				if err != nil {
					t.Fatalf("ReconcileHubWideAt(apply=%v): %v", apply, err)
				}
				result := findResult(results, runprofile.ModuleName)
				if result == nil {
					t.Fatalf("ReconcileHubWideAt(apply=%v) holds no profiles result", apply)
				}
				if tt.wantAdded != nil && !slices.Equal(result.Added, tt.wantAdded) {
					t.Errorf("apply=%v Added = %q; want %q", apply, result.Added, tt.wantAdded)
				}
				for _, path := range tt.wantAddedHas {
					if !slices.Contains(result.Added, path) {
						t.Errorf("apply=%v Added = %q; want it to hold %q", apply, result.Added, path)
					}
				}
				for _, field := range []struct {
					name      string
					got, want []string
				}{
					{"Updated", result.Updated, tt.wantUpdated},
					{"Removed", result.Removed, tt.wantRemoved},
					{"Differs", result.Differs, tt.wantDiffers},
				} {
					if !slices.Equal(field.got, field.want) {
						t.Errorf("apply=%v %s = %q; want %q", apply, field.name, field.got, field.want)
					}
				}

				hub, err := os.ReadFile(hubPath)
				if err != nil {
					t.Fatalf("read the hub file: %v", err)
				}
				base, baseErr := os.ReadFile(basePath)
				if !apply {
					if string(hub) != tt.hubFile {
						t.Errorf("hub file after the dry run = %q; want it unwritten", hub)
					}
					if (tt.baseFile == "") != os.IsNotExist(baseErr) || (tt.baseFile != "" && string(base) != tt.baseFile) {
						t.Errorf("template base after the dry run = %q, %v; want it unwritten", base, baseErr)
					}
					continue
				}
				if result.Applied != tt.wantRewritten {
					t.Errorf("Applied = %v; want %v", result.Applied, tt.wantRewritten)
				}
				if wantBase := tt.baseFile != currentBase; result.WroteTemplateBase != wantBase {
					t.Errorf("WroteTemplateBase = %v; want %v", result.WroteTemplateBase, wantBase)
				}
				if string(base) != currentBase {
					t.Errorf("template base after the apply = %q, %v; want the header over the running template", base, baseErr)
				}
				if !tt.wantRewritten && string(hub) != tt.hubFile {
					t.Errorf("hub file = %q; want it left as the operator holds it", hub)
				}
				for _, want := range tt.wantContains {
					if !strings.Contains(string(hub), want) {
						t.Errorf("hub file = %q; want it to contain %q", hub, want)
					}
				}
				for _, absent := range tt.wantAbsent {
					if strings.Contains(string(hub), absent) {
						t.Errorf("hub file = %q; want it not to contain %q", hub, absent)
					}
				}
			}

			again, err := ReconcileHubWideAt(boardDir, "", HubWideOptions{Apply: true})
			if err != nil {
				t.Fatalf("second ReconcileHubWideAt: %v", err)
			}
			if r := findResult(again, runprofile.ModuleName); r == nil || r.Applied || r.WroteTemplateBase || len(r.Added)+len(r.Updated)+len(r.Removed)+len(r.Differs) > 0 {
				t.Errorf("second apply result = %+v; want nothing written or reported, since the base now records the template", r)
			}
		})
	}
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

// TestReconcileAll_SparseLayers pins that in a hub a worktree file and a local file are sparse layers.
// Nothing is ever added, and a key the template lacks stays and is reported preserved unless PruneUnknown is set.
// An absent file stays absent, a seed-only module's files are untouched, and a file emptied by a prune is deleted.
// A local file loses such a key either way.
func TestReconcileAll_SparseLayers(t *testing.T) {
	tiergate.Require(t, 0)
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
		// wantResult is false when the layer is absent and no result is reported.
		wantResult    bool
		wantKeptExtra []string
		pruneUnknown  bool
		wantPreserved []string
	}{
		{
			name: "worktree file keeps a hub-only lens and stays byte-identical", module: "lenses", layer: configengine.LayerWorktree,
			content: "lenses:\n  hub-lens: |\n    Hub emphasis.\n", wantFile: ptr("lenses:\n  hub-lens: |\n    Hub emphasis.\n"),
			wantKeptExtra: []string{"lenses.hub-lens"}, wantResult: true,
		},
		{
			name: "worktree file loses a key the template lacks under PruneUnknown and gains no template key", module: "loom", layer: configengine.LayerWorktree,
			content: "discussion_timeout_min: 480\nstale_key: old\n", wantFile: ptr("discussion_timeout_min: 480\n"),
			wantApplied: true, wantRemoved: []string{"stale_key"}, wantResult: true, pruneUnknown: true,
		},
		{
			name: "worktree file keeps a key the template lacks without PruneUnknown and reports it preserved", module: "loom", layer: configengine.LayerWorktree,
			content: "discussion_timeout_min: 480\nnewer_key: x\n", wantFile: ptr("discussion_timeout_min: 480\nnewer_key: x\n"),
			wantResult: true, wantPreserved: []string{"newer_key"},
		},
		{
			name: "worktree file keeps a retired key without PruneUnknown like any other key the template lacks", module: "loom", layer: configengine.LayerWorktree,
			content: "discussion_timeout_min: 480\nnewer_key: x\npr_review: x\n", wantFile: ptr("discussion_timeout_min: 480\nnewer_key: x\npr_review: x\n"),
			wantResult: true, wantPreserved: []string{"newer_key", "pr_review"},
		},
		{
			name: "absent worktree file stays absent", module: "loom", layer: configengine.LayerWorktree,
			wantResult: true,
		},
		{
			name: "local file is pruned without PruneUnknown and reported with layer local", module: "loom", layer: configengine.LayerLocal,
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
			content: "stale_key: old\n", wantApplied: true, wantRemoved: []string{"stale_key"}, wantResult: true, pruneUnknown: true,
		},
		{
			name: "absent local file reports nothing", module: "loom", layer: configengine.LayerLocal,
		},
		{
			name: "a local loom file keeps a solo code_reviewer_fan", module: "loom", layer: configengine.LayerLocal,
			content: "code_reviewer_fan: \"\"\n", wantFile: ptr("code_reviewer_fan: \"\"\n"), wantResult: true,
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

			dry, err := ReconcileAll(anchor, boardDir, Options{PruneUnknown: tt.pruneUnknown})
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

			results, err := ReconcileAll(anchor, boardDir, Options{Apply: true, PruneUnknown: tt.pruneUnknown})
			if err != nil {
				t.Fatalf("ReconcileAll(apply): %v", err)
			}
			result := findLayerResult(results, tt.module, tt.layer)
			if (result != nil) != tt.wantResult {
				t.Fatalf("result for %s layer %s = %+v; want present = %v", tt.module, tt.layer, result, tt.wantResult)
			}
			if result != nil {
				if result.Applied != tt.wantApplied || !slices.Equal(result.Removed, tt.wantRemoved) || len(result.Added) != 0 {
					t.Errorf("result = %+v; want Applied %v, Removed %v and nothing Added", result, tt.wantApplied, tt.wantRemoved)
				}
				if !slices.Equal(result.KeptExtra, tt.wantKeptExtra) {
					t.Errorf("KeptExtra = %v; want %v", result.KeptExtra, tt.wantKeptExtra)
				}
				if !slices.Equal(result.Preserved, tt.wantPreserved) {
					t.Errorf("Preserved = %v; want %v", result.Preserved, tt.wantPreserved)
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

// TestReconcileAll_HubWideModuleHasNoWorktreeResult pins that a hub-wide module is never reconciled as a worktree file.
// Only its local override file is reconciled, and only when present.
//
//testtiming:keep pins that a hub-wide module yields no worktree result and that its per-worktree file is left as it was, which the sparse-layer test never asserts
func TestReconcileAll_HubWideModuleHasNoWorktreeResult(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	baseDir, boardDir := t.TempDir(), t.TempDir()
	writeModuleFile(t, baseDir, "board", "labels:\n  area-x: Area X\n")

	results, err := ReconcileAll(baseDir, boardDir, Options{Apply: true})
	if err != nil {
		t.Fatalf("ReconcileAll: %v", err)
	}
	for _, result := range results {
		if result.Module == "board" || result.Module == "fabric" {
			t.Errorf("result %+v reports a hub-wide module; want none", result)
		}
	}
	if got, err := os.ReadFile(configengine.ConfigFile(baseDir, "board")); err != nil || string(got) != "labels:\n  area-x: Area X\n" {
		t.Errorf("per-worktree board.yaml = %q, %v; want it left as it was", got, err)
	}
	if _, err := os.Stat(configengine.ConfigFile(baseDir, "fabric")); !os.IsNotExist(err) {
		t.Errorf("per-worktree fabric.yaml written (stat err = %v)", err)
	}
}

// TestReconcileHubWideAt_PrunesUnknownKeysOnlyWhenAsked pins that a present board file keeps a key the template lacks.
// That holds for a retired key as for any other: it stays and is reported preserved unless PruneUnknown is set.
//
//testtiming:keep pins the exact Preserved and Removed lists and the unchanged dry-run bytes of a kept unknown key, which the seed tests never assert
func TestReconcileHubWideAt_PrunesUnknownKeysOnlyWhenAsked(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name          string
		content       string
		pruneUnknown  bool
		wantFile      string
		wantApplied   bool
		wantRemoved   []string
		wantPreserved []string
	}{
		{
			name: "a key the template lacks stays and is reported preserved", content: "newer_key: 1\n",
			wantFile: "newer_key: 1\n", wantPreserved: []string{"newer_key"},
		},
		{
			name: "a retired key stays beside a preserved key", content: "newer_key: 1\npublish_verify: x\n",
			wantFile: "newer_key: 1\npublish_verify: x\n", wantPreserved: []string{"newer_key", "publish_verify"},
		},
		{
			name: "PruneUnknown prunes every key the template lacks", content: "newer_key: 1\npublish_verify: x\n", pruneUnknown: true,
			wantFile: configengine.SparseBaseHeader, wantApplied: true, wantRemoved: []string{"newer_key", "publish_verify"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			writeModuleFile(t, boardDir, "landing", tt.content)
			path := configengine.ConfigFile(boardDir, "landing")
			for _, apply := range []bool{false, true} {
				results, err := ReconcileHubWideAt(boardDir, "", HubWideOptions{Apply: apply, PruneUnknown: tt.pruneUnknown})
				if err != nil {
					t.Fatalf("ReconcileHubWideAt(apply %v): %v", apply, err)
				}
				result := findResult(results, "landing")
				if result == nil {
					t.Fatalf("ReconcileHubWideAt(apply %v) returned no landing result", apply)
				}
				if result.Applied != (apply && tt.wantApplied) || !slices.Equal(result.Removed, tt.wantRemoved) || !slices.Equal(result.Preserved, tt.wantPreserved) {
					t.Errorf("apply %v: result = %+v; want Applied %v, Removed %v, Preserved %v", apply, result, apply && tt.wantApplied, tt.wantRemoved, tt.wantPreserved)
				}
				if !apply {
					if got, _ := os.ReadFile(path); string(got) != tt.content {
						t.Errorf("dry run changed %s to %q; want %q", path, got, tt.content)
					}
				}
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.wantFile {
				t.Errorf("%s = %q, %v; want %q", path, got, err, tt.wantFile)
			}
		})
	}
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

// TestReconcile_FailureNamesItsFile pins that a reconcile failure is a *FileError.
// The error names the module and the absolute path of the file that failed, and the path leads the message.
func TestReconcile_FailureNamesItsFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const unparseable = "key: [unclosed\n"
	const unknownLensSet = "lens_set: nope\n"
	tests := []struct {
		name       string
		run        func(baseDir, boardDir string) error
		module     string
		wantInBase bool
		// content is the failing file's bytes; empty keeps unparseable.
		content string
		// wantErr are substrings the message must hold.
		wantErr []string
		// worktreeFile is written as the module's file in the base dir beside a content file at the board dir.
		worktreeFile string
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
			if tt.worktreeFile != "" {
				writeModuleFile(t, baseDir, tt.module, tt.worktreeFile)
			}

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

// TestPrimeSeedError pins how a failed prime seed is reported.
// An invalid-config failure names the prime's copy.
// The extra "delete the selector line" step is added only for a module that has an overlay selector.
// Any other failure is returned unchanged.
func TestPrimeSeedError(t *testing.T) {
	tiergate.Require(t, 0)
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
