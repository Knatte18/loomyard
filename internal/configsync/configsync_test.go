// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
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

			results, err := ReconcileAll(baseDir, t.TempDir(), tt.apply)
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
					if _, err := batcher.Active(baseDir); err != nil {
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
	results1, err := ReconcileAll(tmpDir, t.TempDir(), true)
	if err != nil {
		t.Fatalf("ReconcileAll first apply: %v", err)
	}

	// Second apply should be idempotent
	results2, err := ReconcileAll(tmpDir, t.TempDir(), true)
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
	results, err := ReconcileHubWideAt(boardDir, primeBaseDir, apply)
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
// reconcile starts from: a present hub file is never replaced, an absent board.yaml is seeded from the
// prime's copy when it has one, and an absent fabric.yaml never reads the prime.
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
		wantSeed     string
		wantContains []string
		wantAbsent   []string
	}{
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

			dry, err := ReconcileHubWideAt(boardDir, primeDir, false)
			if err != nil {
				t.Fatalf("ReconcileHubWideAt(false): %v", err)
			}
			dryResult := findResult(dry, tt.module)
			if dryResult == nil || dryResult.Seed != tt.wantSeed || dryResult.Applied {
				t.Errorf("dry-run result = %+v; want Seed %q and Applied false", dryResult, tt.wantSeed)
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

			applied, err := ReconcileHubWideAt(boardDir, primeDir, true)
			if err != nil {
				t.Fatalf("ReconcileHubWideAt(true): %v", err)
			}
			appliedResult := findResult(applied, tt.module)
			if appliedResult == nil || appliedResult.Seed != tt.wantSeed {
				t.Fatalf("applied result = %+v; want Seed %q", appliedResult, tt.wantSeed)
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
				results, err := ReconcileAll(baseDir, boardDir, apply)
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
				_, err := ReconcileAll(baseDir, boardDir, false)
				return err
			},
			module:     "batcher",
			wantInBase: true,
		},
		{
			name: "ReconcileHubWideAt on an unparseable hub file",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileHubWideAt(boardDir, "", false)
				return err
			},
			module: "fabric",
		},
		{
			name: "ReconcileAll retiring an unparseable hub-wide copy",
			run: func(baseDir, boardDir string) error {
				_, err := ReconcileAll(baseDir, boardDir, false)
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
