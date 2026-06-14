package ide

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
)

// TestMenuHardErrorOnMissingBoard is the primary Menu test
// (other Menu tests require actual git repos which are complex to set up in unit tests)
// This test focuses on the HealthCheck behavior.

// TestMenuHardErrorOnMissingBoard tests that Menu hard-errors when board HealthCheck fails.
func TestMenuHardErrorOnMissingBoard(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")

	// Create main with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	// Call Menu without a board directory
	var out bytes.Buffer
	in := strings.NewReader("")

	err := Menu(layout, in, &out)
	if err == nil {
		t.Fatalf("expected hard error when board is missing, got nil")
	}

	if !strings.Contains(err.Error(), "health check") {
		t.Fatalf("expected health check error, got: %v", err)
	}
}

// Note: Full integration tests of Menu require actual git worktrees,
// which are complex to set up in unit tests. The key behaviors are tested
// via the HealthCheck test below and through the dispatch tests in main_test.go.
