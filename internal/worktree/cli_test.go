package worktree_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/worktree"
)

// setupCLIRepo creates a fresh repo, changes into it, and writes a
// worktree.yaml config so RunCLI can resolve configuration from the cwd.
// It returns the hub path.
func setupCLIRepo(t *testing.T) string {
	t.Helper()
	hub := newTestRepo(t)
	t.Chdir(hub)

	lyxDir := filepath.Join(hub, "_lyx")
	if err := os.MkdirAll(lyxDir, 0755); err != nil {
		t.Fatalf("create _lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lyxDir, "worktree.yaml"), []byte("branch_prefix: wt-\n"), 0644); err != nil {
		t.Fatalf("write worktree.yaml: %v", err)
	}
	return hub
}

// decodeResult parses RunCLI's JSON output into a generic map.
func decodeResult(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON output: %v\noutput: %s", err, buf.String())
	}
	return result
}

// TestRunCLI covers the subcommand router: a successful list, the unknown-
// subcommand error envelope, and remove with --force flag parsing.
func TestRunCLI(t *testing.T) {
	t.Run("List", func(t *testing.T) {
		setupCLIRepo(t)

		var buf bytes.Buffer
		if got := worktree.RunCLI(&buf, []string{"list"}); got != 0 {
			t.Errorf("RunCLI(list) = %d; want 0", got)
		}

		result := decodeResult(t, &buf)
		if ok, _ := result["ok"].(bool); !ok {
			t.Errorf("RunCLI(list) ok = %v; want true", result["ok"])
		}
		worktrees, isSlice := result["worktrees"].([]any)
		if !isSlice {
			t.Fatalf("RunCLI(list) worktrees missing; got %v", result)
		}
		if len(worktrees) != 1 {
			t.Errorf("RunCLI(list) len(worktrees) = %d; want 1", len(worktrees))
		}
	})

	t.Run("UnknownSubcommand", func(t *testing.T) {
		setupCLIRepo(t)

		var buf bytes.Buffer
		if got := worktree.RunCLI(&buf, []string{"bogus"}); got != 1 {
			t.Errorf("RunCLI(bogus) = %d; want 1", got)
		}

		result := decodeResult(t, &buf)
		if ok, _ := result["ok"].(bool); ok {
			t.Errorf("RunCLI(bogus) ok = %v; want false", result["ok"])
		}
	})

	t.Run("RemoveWithForceFlag", func(t *testing.T) {
		hub := setupCLIRepo(t)
		addRemote(t, hub)

		// Create a second worktree the remove subcommand will tear down.
		slug := "test-wt"
		branch := "wt-" + slug
		target := filepath.Join(filepath.Dir(hub), slug)
		mustRun(t, hub, "git", "worktree", "add", "-b", branch, target)

		var buf bytes.Buffer
		if got := worktree.RunCLI(&buf, []string{"remove", "--force", slug}); got != 0 {
			t.Errorf("RunCLI(remove --force) = %d; want 0\noutput: %s", got, buf.String())
		}

		result := decodeResult(t, &buf)
		if ok, _ := result["ok"].(bool); !ok {
			t.Errorf("RunCLI(remove --force) ok = %v; want true", result["ok"])
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("RunCLI(remove --force): %q still exists; want removed", target)
		}
	})
}
