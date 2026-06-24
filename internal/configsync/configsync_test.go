// configsync_test.go — tests for config reconciliation.

package configsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileAll_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed board.yaml with a missing key and a stale key
	boardPath := filepath.Join(configDir, "board.yaml")
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
	configDir := filepath.Join(tmpDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed board.yaml
	boardPath := filepath.Join(configDir, "board.yaml")
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
	weftPath := filepath.Join(configDir, "weft.yaml")
	if _, err := os.Stat(weftPath); err != nil {
		t.Errorf("weft.yaml was not created: %v", err)
	}

	// Verify board.yaml was rewritten (stale_key removed)
	content, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if contains(string(content), "stale_key") {
		t.Error("board.yaml still contains stale_key after apply; should have been removed")
	}
	if !contains(string(content), "path: board") {
		t.Error("board.yaml missing user value 'path: board' after apply; should be preserved")
	}
}

func TestReconcileAll_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "_lyx", "config")
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
