// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/modelspec"
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

// moduleWant is what one ReconcileAll row expects of one module's Result and config file.
type moduleWant struct {
	applied bool
	// addedAny and removedNone pin that the Result reports some added keys and no removed ones.
	addedAny    bool
	removedNone bool
	addedAll    []string
	removedAll  []string
	// wantFile is the file's exact content after the run; nil skips the check.
	wantFile        *string
	fileContains    []string
	fileNotContains []string
	// migratedAll lists the rewrites Result.Migrated must hold; migratedNone pins it empty.
	migratedAll  []string
	migratedNone bool
	// loadsAsBatcher pins that batcher.Active loads the file after the run.
	loadsAsBatcher bool
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
			name:  "stale reed header block is replaced by selvage",
			seed:  map[string]string{"reed": "tmux: C:\\tools\\tmux.exe\nheader:\n  template: \"\"\n  height_rows: 1\n"},
			apply: true,
			want: map[string]moduleWant{
				"reed": {
					applied:         true,
					removedAll:      []string{"header.template", "header.height_rows"},
					addedAll:        []string{"selvage.height_rows"},
					fileNotContains: []string{"header:"},
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
				fileContains:    []string{"orientation: 31400"},
				fileNotContains: []string{"master_base"},
				loadsAsBatcher:  true,
			}},
		},
		{
			name:  "absent batcher file is seeded from the template and not migrated",
			apply: true,
			want:  map[string]moduleWant{"batcher": {applied: true, migratedNone: true, fileContains: []string{"orientation: 31400"}, loadsAsBatcher: true}},
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

			results, err := ReconcileAll(baseDir, "", Options{Apply: tt.apply})
			if err != nil {
				t.Fatalf("ReconcileAll(%v): %v", tt.apply, err)
			}

			for module, want := range tt.want {
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

	tests := []struct {
		name         string
		module       string
		hubFile      string
		primeFile    string
		legacyFirst  string
		seedDefaults bool
		// wantNoFile pins that the file is still absent after an apply.
		wantNoFile   bool
		wantSeed     string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "loom default absent with a prime copy is seeded from the prime",
			module:       "loom",
			primeFile:    "discussion_timeout_min: 480\n",
			seedDefaults: true,
			wantSeed:     SeedPrime,
			wantContains: []string{"discussion_timeout_min: 480", "review_max_bounces:"},
		},
		{
			name:         "loom default absent and no prime copy starts from the template",
			module:       "loom",
			seedDefaults: true,
			wantSeed:     SeedTemplate,
			wantContains: []string{"review_max_bounces:"},
		},
		{
			name:         "loom default present is reconciled in place",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\nstale_key: old\n",
			primeFile:    "discussion_timeout_min: 480\n",
			seedDefaults: true,
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7", "review_max_bounces:"},
			wantAbsent:   []string{"stale_key", "480"},
		},
		{
			name:         "loom default present is reconciled without SeedDefaults",
			module:       "loom",
			hubFile:      "discussion_timeout_min: 7\nstale_key: old\n",
			wantSeed:     SeedHub,
			wantContains: []string{"discussion_timeout_min: 7", "review_max_bounces:"},
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
			wantContains: []string{"spike: A time-boxed investigation", "area-x: Area X", "design_prefix:"},
			wantAbsent:   []string{"enhancement:"},
		},
		{
			name:         "board absent and no prime copy starts from the template",
			module:       "board",
			wantSeed:     SeedTemplate,
			wantContains: []string{"enhancement:"},
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
			primeFile:    "publish_verify: go test -tags tmux ./...\n",
			wantSeed:     SeedPrime,
			wantContains: []string{"go test -tags tmux ./...", "require_pr_to_base:"},
		},
		{
			name:         "landing absent and no prime copy starts from the template",
			module:       "landing",
			wantSeed:     SeedTemplate,
			wantContains: []string{"require_pr_to_base:"},
		},
		{
			name:         "fabric absent ignores the prime copy",
			module:       "fabric",
			primeFile:    "branch_prefix: prime/\npathspec: \"\"\n",
			wantSeed:     SeedTemplate,
			wantContains: []string{"branch_prefix:"},
			wantAbsent:   []string{"prime/"},
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
			writeModuleFile(t, boardDir, "warp", tt.legacyFirst)
			hubPath := configengine.ConfigFile(boardDir, tt.module)

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
		wantFabric   []string
		wantNoFabric bool
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
			// _lyx is structural, injected in code and never read from the pathspec key, so the template-default empty pathspec is the expected value.
			name:         "only warp.yaml present, pathspec falls back to template default",
			legacyFirst:  "branch_prefix: hanf/\n",
			apply:        true,
			wantMigrated: []string{"warp"},
			wantFabric:   []string{"branch_prefix: hanf/", `pathspec: "" #`},
			wantPruned:   []string{"warp"},
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
		wantResult bool
	}{
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
	}{
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
			name: "an open map differing in one entry is kept whole", module: "batcher", opts: migrate,
			worktreeFile: batcherDiffering, wantContains: []string{"orientation: 31401"}, wantMarker: true,
			wantStripped: []string{"active"},
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
			if string(after) != string(before) {
				t.Errorf("resolution changed by the migration:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
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
	tests := []struct {
		name       string
		run        func(baseDir, boardDir string) error
		module     string
		wantInBase bool
		hubFile    string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir, boardDir := t.TempDir(), t.TempDir()
			fileDir := boardDir
			if tt.wantInBase {
				fileDir = baseDir
			}
			writeModuleFile(t, fileDir, tt.module, unparseable)
			writeModuleFile(t, boardDir, tt.module, tt.hubFile)

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
		})
	}
}

// TestReconcile_MovesKeysAcrossModules pins the declared shuttle-to-claude key move at every reconcile entry point: where the value lands at each layer, what a collision keeps or overwrites, and that a dry run writes nothing and a second pass moves nothing.
func TestReconcile_MovesKeysAcrossModules(t *testing.T) {
	t.Parallel()
	claudeTemplate, _ := configreg.Template("claude")
	claudeModule, _ := configreg.Lookup("claude")
	const templateBinary = "${env:LYX_SHUTTLE_CLAUDE:-}"
	tests := []struct {
		name string
		// files maps "<base|board|prime|local>:<module>" to the file's content.
		files map[string]string
		// call is "hub" (ReconcileAll over a board dir), "standalone" (ReconcileAll without one) or "board" (ReconcileHubWideAt seeding from the prime).
		call   string
		dryRun bool
		repeat bool
		// contains and excludes map a file key to substrings its content must and must not hold after the run.
		contains map[string][]string
		excludes map[string][]string
		absent   []string
		// moved, kept and overwritten are the exact report lines the run must produce across all its results.
		moved, kept, overwritten []string
		// loadContains is what claude's resolved base+worktree stack must hold after the run.
		loadContains string
		// checkLoadRefuses is the file name claude's loader must refuse naming after the run.
		checkLoadRefuses string
	}{
		{
			name:     "a repo default moves into a created full claude.yaml",
			files:    map[string]string{"board:shuttle": "claude: /x\n"},
			call:     "board",
			contains: map[string][]string{"board:claude": {"binary: /x", "deny_agent_tool"}},
			excludes: map[string][]string{"board:shuttle": {"claude: /x"}},
			moved:    []string{"shuttle.claude -> claude.binary"},
		},
		{
			name:     "a board default seeded from the prime's shuttle.yaml carries the moved value",
			files:    map[string]string{"prime:shuttle": "claude: /x\n"},
			call:     "board",
			contains: map[string][]string{"board:claude": {"binary: /x"}, "prime:shuttle": {"claude: /x"}},
			excludes: map[string][]string{"board:shuttle": {"claude: /x"}},
			moved:    []string{"shuttle.claude -> claude.binary"},
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
