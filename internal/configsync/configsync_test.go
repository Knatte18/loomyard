// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

func TestReconcileAll_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed board.yaml with a missing key and a stale key
	boardPath := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(boardPath, []byte("path: board\nstale_key: old_value\n"), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}

	// Run ReconcileAll with apply=false
	results, err := ReconcileAll(tmpDir, false)
	if err != nil {
		t.Fatalf("ReconcileAll(false): %v", err)
	}

	// Find board result
	var boardResult *Result
	for i := range results {
		if results[i].Module == "board" {
			boardResult = &results[i]
			break
		}
	}
	if boardResult == nil {
		t.Error("board result not found")
	} else {
		// Board should have added keys (the template has more keys than the seed)
		if len(boardResult.Added) == 0 {
			t.Errorf("board.Added is empty; want non-empty (template has missing keys)")
		}
		// Board should have removed keys (stale_key)
		if len(boardResult.Removed) == 0 {
			t.Errorf("board.Removed is empty; want non-empty (stale_key should be reported)")
		}
		// Dry-run should never apply
		if boardResult.Applied {
			t.Error("board.Applied is true; want false (dry-run)")
		}
	}

	// Verify file is unchanged
	content, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if !contains(string(content), "stale_key") {
		t.Error("board.yaml was modified during dry-run; stale_key should still be present")
	}
}

func TestReconcileAll_ApplyCreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed board.yaml
	boardPath := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(boardPath, []byte("path: board\nstale_key: old_value\n"), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}

	// Run ReconcileAll with apply=true
	results, err := ReconcileAll(tmpDir, true)
	if err != nil {
		t.Fatalf("ReconcileAll(true): %v", err)
	}

	// Board result should show it was applied
	var boardResult *Result
	for i := range results {
		if results[i].Module == "board" {
			boardResult = &results[i]
			break
		}
	}
	if boardResult == nil {
		t.Error("board result not found")
	} else if !boardResult.Applied {
		t.Error("board.Applied is false; want true (changes should be applied)")
	}

	// Weft result should show creation
	var weftResult *Result
	for i := range results {
		if results[i].Module == "weft" {
			weftResult = &results[i]
			break
		}
	}
	if weftResult == nil {
		t.Error("weft result not found")
	} else if !weftResult.Applied {
		t.Error("weft.Applied is false; want true (absent file should be created)")
	}

	// Verify weft.yaml was created
	weftPath := hubgeometry.ConfigFile(tmpDir, "weft")
	if _, err := os.Stat(weftPath); err != nil {
		t.Errorf("weft.yaml was not created: %v", err)
	}

	// Verify board.yaml was rewritten: stale_key removed and path: also removed
	// (path: is no longer in the board template; Reconcile treats it as an extra key).
	content, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if contains(string(content), "stale_key") {
		t.Error("board.yaml still contains stale_key after apply; should have been removed")
	}
	if contains(string(content), "path:") {
		t.Error("board.yaml still contains path: key after apply; should have been removed (not in template)")
	}
}

// TestReconcileAll_DropsStaleMuxClaudeKey pins the specific removed-key case
// this batch introduces: an existing user mux.yaml written before the
// claude: key was dropped from the template must have that key reconciled
// away, exactly like any other stale key a template no longer declares.
func TestReconcileAll_DropsStaleMuxClaudeKey(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed mux.yaml as it would exist on disk for a user who set up their
	// worktree before the claude: key was removed from the template.
	muxPath := hubgeometry.ConfigFile(tmpDir, "mux")
	seedContent := "psmux: C:\\tools\\psmux.exe\nclaude: C:\\tools\\claude.exe\n"
	if err := os.WriteFile(muxPath, []byte(seedContent), 0o644); err != nil {
		t.Fatalf("write mux.yaml: %v", err)
	}

	results, err := ReconcileAll(tmpDir, true)
	if err != nil {
		t.Fatalf("ReconcileAll(true): %v", err)
	}

	var muxResult *Result
	for i := range results {
		if results[i].Module == "mux" {
			muxResult = &results[i]
			break
		}
	}
	if muxResult == nil {
		t.Fatal("mux result not found")
	}
	if !muxResult.Applied {
		t.Error("mux.Applied is false; want true (stale claude key should trigger a rewrite)")
	}

	found := false
	for _, r := range muxResult.Removed {
		if r == "claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("mux.Removed = %v, want it to contain %q", muxResult.Removed, "claude")
	}

	content, err := os.ReadFile(muxPath)
	if err != nil {
		t.Fatalf("read mux.yaml: %v", err)
	}
	if contains(string(content), "claude:") {
		t.Error("mux.yaml still contains claude: key after apply; should have been removed")
	}
}

func TestReconcileAll_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// First apply
	results1, err := ReconcileAll(tmpDir, true)
	if err != nil {
		t.Fatalf("ReconcileAll first apply: %v", err)
	}

	// Second apply should be idempotent
	results2, err := ReconcileAll(tmpDir, true)
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

// TestReconcileAll_SeedOnly pins the anti-prune contract for seed-only
// modules ("models" today): the seed is materialized verbatim exactly once,
// and once present is never rewritten — not even to resurrect a key the
// operator deliberately removed. A non-seed-only module in the same run
// still gets the ordinary prune behavior, guarding against an over-broad
// skip that would silently disable reconcile for every module.
func TestReconcileAll_SeedOnly(t *testing.T) {
	t.Run("absent file materializes template verbatim", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := hubgeometry.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		results, err := ReconcileAll(tmpDir, true)
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

		modelsPath := hubgeometry.ConfigFile(tmpDir, "models")
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
		configDir := hubgeometry.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		modelsPath := hubgeometry.ConfigFile(tmpDir, "models")
		seedContent := "zephyr:\n  engine: claude\n  model: claude-zephyr-1\n"
		if err := os.WriteFile(modelsPath, []byte(seedContent), 0o644); err != nil {
			t.Fatalf("write models.yaml: %v", err)
		}

		results, err := ReconcileAll(tmpDir, true)
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
		configDir := hubgeometry.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		modelsPath := hubgeometry.ConfigFile(tmpDir, "models")
		// The sonnet block's defaults/effort keys are deliberately absent
		// relative to modelspec.ConfigTemplate().
		seedContent := "sonnet:\n  engine: claude\n  model: sonnet\n"
		if err := os.WriteFile(modelsPath, []byte(seedContent), 0o644); err != nil {
			t.Fatalf("write models.yaml: %v", err)
		}

		results, err := ReconcileAll(tmpDir, true)
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
		configDir := hubgeometry.ConfigDir(tmpDir)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Seed board.yaml (non-seed-only) with a stale key alongside an
		// untouched models.yaml, to guard against the seed-only branch
		// over-broadly skipping every module's reconcile.
		boardPath := hubgeometry.ConfigFile(tmpDir, "board")
		if err := os.WriteFile(boardPath, []byte("path: board\nstale_key: old_value\n"), 0o644); err != nil {
			t.Fatalf("write board.yaml: %v", err)
		}

		results, err := ReconcileAll(tmpDir, true)
		if err != nil {
			t.Fatalf("ReconcileAll(true): %v", err)
		}

		boardResult := findResult(results, "board")
		if boardResult == nil {
			t.Fatal("board result not found")
		}
		if !boardResult.Applied {
			t.Error("board.Applied is false; want true (stale key should still trigger a rewrite)")
		}
		found := false
		for _, r := range boardResult.Removed {
			if r == "stale_key" {
				found = true
			}
		}
		if !found {
			t.Errorf("board.Removed = %v; want it to contain %q", boardResult.Removed, "stale_key")
		}

		content, err := os.ReadFile(boardPath)
		if err != nil {
			t.Fatalf("read board.yaml: %v", err)
		}
		if contains(string(content), "stale_key") {
			t.Error("board.yaml still contains stale_key after apply; should have been removed (pruning must still work)")
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
