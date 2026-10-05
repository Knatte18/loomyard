// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"fmt"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

func TestReconcileAll_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed loom.yaml with a missing key and a stale key
	loomPath := configengine.ConfigFile(tmpDir, "loom")
	if err := os.WriteFile(loomPath, []byte("discussion_timeout_min: 480\nstale_key: old_value\n"), 0o644); err != nil {
		t.Fatalf("write loom.yaml: %v", err)
	}

	// Run ReconcileAll with apply=false
	results, err := ReconcileAll(tmpDir, t.TempDir(), false)
	if err != nil {
		t.Fatalf("ReconcileAll(false): %v", err)
	}

	// Find loom result
	loomResult := findResult(results, "loom")
	if loomResult == nil {
		t.Error("loom result not found")
	} else {
		// Loom should have added keys (the template has more keys than the seed)
		if len(loomResult.Added) == 0 {
			t.Errorf("loom.Added is empty; want non-empty (template has missing keys)")
		}
		// Loom should have removed keys (stale_key)
		if len(loomResult.Removed) == 0 {
			t.Errorf("loom.Removed is empty; want non-empty (stale_key should be reported)")
		}
		// Dry-run should never apply
		if loomResult.Applied {
			t.Error("loom.Applied is true; want false (dry-run)")
		}
	}

	// Verify file is unchanged
	content, err := os.ReadFile(loomPath)
	if err != nil {
		t.Fatalf("read loom.yaml: %v", err)
	}
	if !contains(string(content), "stale_key") {
		t.Error("loom.yaml was modified during dry-run; stale_key should still be present")
	}
}

func TestReconcileAll_ApplyCreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed loom.yaml
	loomPath := configengine.ConfigFile(tmpDir, "loom")
	if err := os.WriteFile(loomPath, []byte("discussion_timeout_min: 480\nstale_key: old_value\n"), 0o644); err != nil {
		t.Fatalf("write loom.yaml: %v", err)
	}

	// Run ReconcileAll with apply=true
	results, err := ReconcileAll(tmpDir, t.TempDir(), true)
	if err != nil {
		t.Fatalf("ReconcileAll(true): %v", err)
	}

	// Loom result should show it was applied
	loomResult := findResult(results, "loom")
	if loomResult == nil {
		t.Error("loom result not found")
	} else if !loomResult.Applied {
		t.Error("loom.Applied is false; want true (changes should be applied)")
	}

	// Verify loom.yaml was rewritten: stale_key removed and the template's missing keys added.
	content, err := os.ReadFile(loomPath)
	if err != nil {
		t.Fatalf("read loom.yaml: %v", err)
	}
	if contains(string(content), "stale_key") {
		t.Error("loom.yaml still contains stale_key after apply; should have been removed")
	}
	if !contains(string(content), "review_max_bounces:") {
		t.Error("loom.yaml lacks review_max_bounces: after apply; should have been added from the template")
	}
}

// TestReconcileAll_SeedsLoggerYAML pins that reconcile seeds logger.yaml from the template when the file is absent.
func TestReconcileAll_SeedsLoggerYAML(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(configengine.ConfigDir(tmpDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	results, err := ReconcileAll(tmpDir, t.TempDir(), true)
	if err != nil {
		t.Fatalf("ReconcileAll(true): %v", err)
	}

	var loggerResult *Result
	for i := range results {
		if results[i].Module == "logger" {
			loggerResult = &results[i]
			break
		}
	}
	if loggerResult == nil {
		t.Fatal("logger result not found")
	}
	if !loggerResult.Applied {
		t.Error("logger.Applied is false; want true (absent file should be seeded)")
	}

	content, err := os.ReadFile(configengine.ConfigFile(tmpDir, "logger"))
	if err != nil {
		t.Fatalf("read logger.yaml: %v", err)
	}
	if string(content) != loggerconfig.ConfigTemplate() {
		t.Errorf("logger.yaml = %q; want the template %q", content, loggerconfig.ConfigTemplate())
	}
}

// TestReconcileAll_DropsStaleReedClaudeKey pins the specific removed-key case this batch
// introduces: an existing user reed.yaml written before the claude: key was dropped from the
// template must have that key reconciled away, exactly like any other stale key a template no
// longer declares.
func TestReconcileAll_DropsStaleReedClaudeKey(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed reed.yaml as it would exist on disk for a user who set up their
	// worktree before the claude: key was removed from the template.
	reedPath := configengine.ConfigFile(tmpDir, "reed")
	seedContent := "tmux: C:\\tools\\tmux.exe\nclaude: C:\\tools\\claude.exe\n"
	if err := os.WriteFile(reedPath, []byte(seedContent), 0o644); err != nil {
		t.Fatalf("write reed.yaml: %v", err)
	}

	results, err := ReconcileAll(tmpDir, t.TempDir(), true)
	if err != nil {
		t.Fatalf("ReconcileAll(true): %v", err)
	}

	var reedResult *Result
	for i := range results {
		if results[i].Module == "reed" {
			reedResult = &results[i]
			break
		}
	}
	if reedResult == nil {
		t.Fatal("reed result not found")
	}
	if !reedResult.Applied {
		t.Error("reed.Applied is false; want true (stale claude key should trigger a rewrite)")
	}

	found := false
	for _, r := range reedResult.Removed {
		if r == "claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("reed.Removed = %v, want it to contain %q", reedResult.Removed, "claude")
	}

	content, err := os.ReadFile(reedPath)
	if err != nil {
		t.Fatalf("read reed.yaml: %v", err)
	}
	if contains(string(content), "claude:") {
		t.Error("reed.yaml still contains claude: key after apply; should have been removed")
	}
}

// TestReconcileAll_DropsStaleReedHeaderBlock pins the migration path an operator actually gets from
// "lyx config reconcile --apply": a reed.yaml written before the header: block was split into
// status_line: and selvage: must have header.template and header.height_rows reconciled away, and
// the new status_line/selvage leaves added, exactly like TestReconcileAll_DropsStaleReedClaudeKey
// pins the earlier claude: key removal.
func TestReconcileAll_DropsStaleReedHeaderBlock(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed reed.yaml as it would exist on disk for a user who set up their
	// worktree before the header: block was split into status_line: and
	// selvage:.
	reedPath := configengine.ConfigFile(tmpDir, "reed")
	seedContent := "tmux: C:\\tools\\tmux.exe\nheader:\n  template: \"\"\n  height_rows: 1\n"
	if err := os.WriteFile(reedPath, []byte(seedContent), 0o644); err != nil {
		t.Fatalf("write reed.yaml: %v", err)
	}

	results, err := ReconcileAll(tmpDir, t.TempDir(), true)
	if err != nil {
		t.Fatalf("ReconcileAll(true): %v", err)
	}

	reedResult := findResult(results, "reed")
	if reedResult == nil {
		t.Fatal("reed result not found")
	}
	if !reedResult.Applied {
		t.Error("reed.Applied is false; want true (stale header block should trigger a rewrite)")
	}

	wantRemoved := map[string]bool{"header.template": false, "header.height_rows": false}
	for _, r := range reedResult.Removed {
		if _, ok := wantRemoved[r]; ok {
			wantRemoved[r] = true
		}
	}
	for key, found := range wantRemoved {
		if !found {
			t.Errorf("reed.Removed = %v, want it to contain %q", reedResult.Removed, key)
		}
	}

	wantAdded := map[string]bool{"status_line.template": false, "selvage.height_rows": false}
	for _, a := range reedResult.Added {
		if _, ok := wantAdded[a]; ok {
			wantAdded[a] = true
		}
	}
	for key, found := range wantAdded {
		if !found {
			t.Errorf("reed.Added = %v, want it to contain %q", reedResult.Added, key)
		}
	}

	content, err := os.ReadFile(reedPath)
	if err != nil {
		t.Fatalf("read reed.yaml: %v", err)
	}
	if contains(string(content), "header:") {
		t.Error("reed.yaml still contains a header: block after apply; should have been removed")
	}
}

func TestReconcileAll_Idempotent(t *testing.T) {
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

// TestReconcileAll_SeedOnly pins the anti-prune contract for seed-only modules ("models" today):
// the seed is materialized verbatim exactly once,
// and once present is never rewritten — not even to resurrect a key the operator deliberately
// removed.
// A non-seed-only module in the same run still gets the ordinary prune behavior, guarding against
// an over-broad skip that would silently disable reconcile for every module.
func TestReconcileAll_SeedOnly(t *testing.T) {
	t.Run("absent file materializes template verbatim", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := configengine.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		results, err := ReconcileAll(tmpDir, t.TempDir(), true)
		if err != nil {
			t.Fatalf("ReconcileAll(true): %v", err)
		}

		result := findResult(results, "models")
		if result == nil {
			t.Fatal("models result not found")
		}
		if !result.Applied {
			t.Error("models.Applied is false; want true (absent file should be materialized)")
		}
		if len(result.Added) == 0 {
			t.Error("models.Added is empty; want every template leaf key-path")
		}
		if len(result.Removed) != 0 {
			t.Errorf("models.Removed = %v; want empty (seed-only never reports removed)", result.Removed)
		}

		modelsPath := configengine.ConfigFile(tmpDir, "models")
		got, err := os.ReadFile(modelsPath)
		if err != nil {
			t.Fatalf("read models.yaml: %v", err)
		}
		want := modelspec.ConfigTemplate()
		if string(got) != want {
			t.Errorf("models.yaml = %q; want byte-identical template %q", got, want)
		}
	})

	t.Run("present file with operator-added alias is untouched", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := configengine.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		modelsPath := configengine.ConfigFile(tmpDir, "models")
		seedContent := "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
		if err := os.WriteFile(modelsPath, []byte(seedContent), 0o644); err != nil {
			t.Fatalf("write models.yaml: %v", err)
		}

		results, err := ReconcileAll(tmpDir, t.TempDir(), true)
		if err != nil {
			t.Fatalf("ReconcileAll(true): %v", err)
		}

		result := findResult(results, "models")
		if result == nil {
			t.Fatal("models result not found")
		}
		if result.Applied {
			t.Error("models.Applied is true; want false (present seed-only file is never rewritten)")
		}
		if len(result.Added) != 0 || len(result.Removed) != 0 {
			t.Errorf("models Added=%v Removed=%v; want both empty (file is never parsed/diffed)", result.Added, result.Removed)
		}

		got, err := os.ReadFile(modelsPath)
		if err != nil {
			t.Fatalf("read models.yaml: %v", err)
		}
		if string(got) != seedContent {
			t.Errorf("models.yaml = %q; want unchanged %q", got, seedContent)
		}
	})

	t.Run("present file with template key removed is not resurrected", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := configengine.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		modelsPath := configengine.ConfigFile(tmpDir, "models")
		// The sonnet block's defaults/effort keys are deliberately absent
		// relative to modelspec.ConfigTemplate().
		seedContent := "sonnet:\n  engine: claude\n  model: sonnet\n"
		if err := os.WriteFile(modelsPath, []byte(seedContent), 0o644); err != nil {
			t.Fatalf("write models.yaml: %v", err)
		}

		results, err := ReconcileAll(tmpDir, t.TempDir(), true)
		if err != nil {
			t.Fatalf("ReconcileAll(true): %v", err)
		}

		result := findResult(results, "models")
		if result == nil {
			t.Fatal("models result not found")
		}
		if result.Applied {
			t.Error("models.Applied is true; want false (present seed-only file is never rewritten)")
		}

		got, err := os.ReadFile(modelsPath)
		if err != nil {
			t.Fatalf("read models.yaml: %v", err)
		}
		if string(got) != seedContent {
			t.Errorf("models.yaml = %q; want unchanged (no silent resurrection of removed keys) %q", got, seedContent)
		}
	})

	t.Run("non-seed-only module still gets pruned in the same run", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := configengine.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Seed loom.yaml (non-seed-only) with a stale key alongside an
		// untouched models.yaml, to guard against the seed-only branch
		// over-broadly skipping every module's reconcile.
		loomPath := configengine.ConfigFile(tmpDir, "loom")
		if err := os.WriteFile(loomPath, []byte("discussion_timeout_min: 480\nstale_key: old_value\n"), 0o644); err != nil {
			t.Fatalf("write loom.yaml: %v", err)
		}

		results, err := ReconcileAll(tmpDir, t.TempDir(), true)
		if err != nil {
			t.Fatalf("ReconcileAll(true): %v", err)
		}

		loomResult := findResult(results, "loom")
		if loomResult == nil {
			t.Fatal("loom result not found")
		}
		if !loomResult.Applied {
			t.Error("loom.Applied is false; want true (stale key should still trigger a rewrite)")
		}
		found := false
		for _, r := range loomResult.Removed {
			if r == "stale_key" {
				found = true
			}
		}
		if !found {
			t.Errorf("loom.Removed = %v; want it to contain %q", loomResult.Removed, "stale_key")
		}

		content, err := os.ReadFile(loomPath)
		if err != nil {
			t.Fatalf("read loom.yaml: %v", err)
		}
		if contains(string(content), "stale_key") {
			t.Error("loom.yaml still contains stale_key after apply; should have been removed (pruning must still work)")
		}
	})
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
			writeModule := func(dir, module, content string) {
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
			writeModule(boardDir, tt.module, tt.hubFile)
			writeModule(primeDir, tt.module, tt.primeFile)
			writeModule(boardDir, "warp", tt.legacyFirst)
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
				if !contains(string(got), want) {
					t.Errorf("hub file = %q; want it to contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if contains(string(got), absent) {
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
	t.Run("both legacy files present, both values migrate, both files pruned", func(t *testing.T) {
		boardDir := t.TempDir()
		configDir := configengine.ConfigDir(boardDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		legacyFirstPath := configengine.ConfigFile(boardDir, "warp")
		if err := os.WriteFile(legacyFirstPath, []byte("branch_prefix: hanf/\n"), 0o644); err != nil {
			t.Fatalf("write warp.yaml: %v", err)
		}
		legacySecondPath := configengine.ConfigFile(boardDir, "weft")
		if err := os.WriteFile(legacySecondPath, []byte("pathspec: _lyx custom-dir\n"), 0o644); err != nil {
			t.Fatalf("write weft.yaml: %v", err)
		}

		fabricResult, err := reconcileFabricResult(boardDir, "", true)
		if err != nil {
			t.Fatalf("ReconcileHubWideAt(true): %v", err)
		}

		if !fabricResult.Applied {
			t.Error("fabric.Applied is false; want true (absent file should be created)")
		}
		wantMigrated := map[string]bool{"warp": true, "weft": true}
		if len(fabricResult.MigratedFrom) != 2 {
			t.Errorf("fabric.MigratedFrom = %v; want both warp and weft", fabricResult.MigratedFrom)
		}
		for _, m := range fabricResult.MigratedFrom {
			if !wantMigrated[m] {
				t.Errorf("fabric.MigratedFrom contains unexpected entry %q", m)
			}
		}

		fabricPath := configengine.ConfigFile(boardDir, "fabric")
		got, err := os.ReadFile(fabricPath)
		if err != nil {
			t.Fatalf("read fabric.yaml: %v", err)
		}
		if !contains(string(got), "branch_prefix: hanf/") {
			t.Errorf("fabric.yaml = %q; want migrated branch_prefix: hanf/", got)
		}
		if !contains(string(got), "pathspec: _lyx custom-dir") {
			t.Errorf("fabric.yaml = %q; want migrated pathspec: _lyx custom-dir", got)
		}

		if _, err := os.Stat(legacyFirstPath); !os.IsNotExist(err) {
			t.Errorf("warp.yaml still exists after migration; want pruned (stat err = %v)", err)
		}
		if _, err := os.Stat(legacySecondPath); !os.IsNotExist(err) {
			t.Errorf("weft.yaml still exists after migration; want pruned (stat err = %v)", err)
		}
	})

	t.Run("only warp.yaml present, pathspec falls back to template default", func(t *testing.T) {
		boardDir := t.TempDir()
		configDir := configengine.ConfigDir(boardDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		legacyFirstPath := configengine.ConfigFile(boardDir, "warp")
		if err := os.WriteFile(legacyFirstPath, []byte("branch_prefix: hanf/\n"), 0o644); err != nil {
			t.Fatalf("write warp.yaml: %v", err)
		}

		fabricResult, err := reconcileFabricResult(boardDir, "", true)
		if err != nil {
			t.Fatalf("ReconcileHubWideAt(true): %v", err)
		}

		if len(fabricResult.MigratedFrom) != 1 || fabricResult.MigratedFrom[0] != "warp" {
			t.Errorf("fabric.MigratedFrom = %v; want exactly [warp]", fabricResult.MigratedFrom)
		}

		fabricPath := configengine.ConfigFile(boardDir, "fabric")
		got, err := os.ReadFile(fabricPath)
		if err != nil {
			t.Fatalf("read fabric.yaml: %v", err)
		}
		if !contains(string(got), "branch_prefix: hanf/") {
			t.Errorf("fabric.yaml = %q; want migrated branch_prefix: hanf/", got)
		}
		if !contains(string(got), `pathspec: "" #`) {
			t.Errorf("fabric.yaml = %q; want template-default empty pathspec (weft.yaml was absent; _lyx is now structural, injected in code, never read from this key)", got)
		}

		if _, err := os.Stat(legacyFirstPath); !os.IsNotExist(err) {
			t.Errorf("warp.yaml still exists after migration; want pruned (stat err = %v)", err)
		}
	})

	t.Run("dry run reports the pending migration but writes and deletes nothing", func(t *testing.T) {
		boardDir := t.TempDir()
		configDir := configengine.ConfigDir(boardDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		legacyFirstPath := configengine.ConfigFile(boardDir, "warp")
		if err := os.WriteFile(legacyFirstPath, []byte("branch_prefix: hanf/\n"), 0o644); err != nil {
			t.Fatalf("write warp.yaml: %v", err)
		}

		fabricResult, err := reconcileFabricResult(boardDir, "", false)
		if err != nil {
			t.Fatalf("ReconcileHubWideAt(false): %v", err)
		}

		if fabricResult.Applied {
			t.Error("fabric.Applied is true; want false (dry-run)")
		}
		if len(fabricResult.MigratedFrom) != 1 || fabricResult.MigratedFrom[0] != "warp" {
			t.Errorf("fabric.MigratedFrom = %v; want exactly [warp] even on a dry run", fabricResult.MigratedFrom)
		}

		fabricPath := configengine.ConfigFile(boardDir, "fabric")
		if _, err := os.Stat(fabricPath); !os.IsNotExist(err) {
			t.Errorf("fabric.yaml was written on a dry run; want absent (stat err = %v)", err)
		}
		if _, err := os.Stat(legacyFirstPath); err != nil {
			t.Errorf("warp.yaml was removed on a dry run; want it left alone (stat err = %v)", err)
		}
	})

	t.Run("fabric.yaml already present, legacy files untouched and not migrated", func(t *testing.T) {
		boardDir := t.TempDir()
		configDir := configengine.ConfigDir(boardDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		fabricPath := configengine.ConfigFile(boardDir, "fabric")
		if err := os.WriteFile(fabricPath, []byte("branch_prefix: existing/\npathspec: _lyx\n"), 0o644); err != nil {
			t.Fatalf("write fabric.yaml: %v", err)
		}
		legacyFirstPath := configengine.ConfigFile(boardDir, "warp")
		if err := os.WriteFile(legacyFirstPath, []byte("branch_prefix: stale/\n"), 0o644); err != nil {
			t.Fatalf("write warp.yaml: %v", err)
		}

		fabricResult, err := reconcileFabricResult(boardDir, "", true)
		if err != nil {
			t.Fatalf("ReconcileHubWideAt(true): %v", err)
		}

		if len(fabricResult.MigratedFrom) != 0 {
			t.Errorf("fabric.MigratedFrom = %v; want empty (fabric.yaml already present)", fabricResult.MigratedFrom)
		}

		got, err := os.ReadFile(fabricPath)
		if err != nil {
			t.Fatalf("read fabric.yaml: %v", err)
		}
		if !contains(string(got), "branch_prefix: existing/") {
			t.Errorf("fabric.yaml = %q; want its own pre-existing branch_prefix, not warp.yaml's stale one", got)
		}
		if _, err := os.Stat(legacyFirstPath); err != nil {
			t.Errorf("warp.yaml was removed even though fabric.yaml already existed; want it left alone (stat err = %v)", err)
		}
	})

	t.Run("unparseable legacy file is skipped, left on disk, and not migrated", func(t *testing.T) {
		boardDir := t.TempDir()
		configDir := configengine.ConfigDir(boardDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		legacyFirstPath := configengine.ConfigFile(boardDir, "warp")
		if err := os.WriteFile(legacyFirstPath, []byte("branch_prefix: [unterminated\n"), 0o644); err != nil {
			t.Fatalf("write corrupt warp.yaml: %v", err)
		}

		fabricResult, err := reconcileFabricResult(boardDir, "", true)
		if err != nil {
			t.Fatalf("ReconcileHubWideAt(true): %v", err)
		}

		if len(fabricResult.MigratedFrom) != 0 {
			t.Errorf("fabric.MigratedFrom = %v; want empty (warp.yaml is unparseable)", fabricResult.MigratedFrom)
		}
		if _, err := os.Stat(legacyFirstPath); err != nil {
			t.Errorf("corrupt warp.yaml was removed; want it left alone for the operator to inspect (stat err = %v)", err)
		}
	})
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

// contains reports whether s contains substr as a substring.
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

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
			write := func(dir, content string) {
				if content == "" {
					return
				}
				if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(configengine.ConfigFile(dir, "board"), []byte(content), 0o644); err != nil {
					t.Fatalf("write board.yaml: %v", err)
				}
			}
			write(baseDir, tt.copyFile)
			write(boardDir, tt.hubFile)
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
