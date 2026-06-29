//go:build integration

// warp_test.go covers the RunCLI subcommand router using a CWD-based fixture.
// These tests keep t.Chdir and stay serial (no t.Parallel) because RunCLI
// reads os.Getwd() at the call edge.

package warpcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/warpcli"
)

// setupCLIRepo creates a hub via lyxtest.CopyHostHub, changes into it, and
// writes a _lyx/config/warp.yaml config so RunCLI can resolve configuration from
// the cwd. Returns the hub path.
// Stays serial (no t.Parallel) because t.Chdir is required for RunCLI.
func setupCLIRepo(t *testing.T) string {
	t.Helper()
	f := lyxtest.CopyHostHub(t)
	t.Chdir(f.Hub)

	if err := os.MkdirAll(paths.ConfigDir(f.Hub), 0755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(paths.ConfigFile(f.Hub, "warp"), []byte("branch_prefix: wt-\n"), 0644); err != nil {
		t.Fatalf("write warp.yaml: %v", err)
	}
	return f.Hub
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

// TestRunCLI_CloneResetRemovesHub verifies that --reset removes a pre-existing hub
// directory before delegating to the clone logic, and that without --reset the hub
// is left untouched when it already exists (clone returns "hub already exists").
//
// A fake URL (*.invalid) is used so the git clone fails quickly without network I/O;
// the test asserts filesystem state, not the clone outcome.
func TestRunCLI_CloneResetRemovesHub(t *testing.T) {
	// Set cwd to a temp dir so RunCLI derives the hub path relative to it.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// The URL "https://x.invalid/repo.git" → deriveHostName → "repo" → hub = "repo-HUB".
	hubPath := filepath.Join(tmpDir, "repo-HUB")

	t.Run("reset_removes_hub_before_clone", func(t *testing.T) {
		// Create the hub dir to simulate an existing stale hub.
		if err := os.MkdirAll(hubPath, 0o755); err != nil {
			t.Fatalf("create hub dir: %v", err)
		}
		var buf bytes.Buffer
		// Clone will fail (fake URL) but --reset must remove the hub first.
		_ = warpcli.RunCLI(&buf, []string{"clone", "--reset",
			"https://x.invalid/repo.git", "https://x.invalid/repo-weft.git"})
		// Regardless of clone outcome, the original hub must be gone.
		if _, err := os.Stat(hubPath); !os.IsNotExist(err) {
			t.Errorf("hub dir %q still exists after --reset; want removed", hubPath)
		}
	})

	t.Run("no_reset_preserves_hub_on_conflict", func(t *testing.T) {
		// Re-create the hub dir; without --reset the clone must not remove it.
		if err := os.MkdirAll(hubPath, 0o755); err != nil {
			t.Fatalf("create hub dir: %v", err)
		}
		var buf bytes.Buffer
		code := warpcli.RunCLI(&buf, []string{"clone",
			"https://x.invalid/repo.git", "https://x.invalid/repo-weft.git"})
		if code != 1 {
			t.Errorf("RunCLI(clone) without --reset when hub exists = %d; want 1", code)
		}
		// Hub must still exist: cloneHub returns "hub already exists" without removing it.
		if _, err := os.Stat(hubPath); os.IsNotExist(err) {
			t.Errorf("hub dir %q removed without --reset; want still present", hubPath)
		}
	})
}

// TestRunCLI_CloneHelp asserts that "warp clone --help" output contains the --reset flag
// and hub layout terms from the Long description.
func TestRunCLI_CloneHelp(t *testing.T) {
	var buf bytes.Buffer
	code := warpcli.RunCLI(&buf, []string{"clone", "--help"})
	if code != 0 {
		t.Fatalf("RunCLI(clone --help) = %d; want 0", code)
	}
	got := buf.String()
	for _, want := range []string{"--reset", "_board", "hub"} {
		if !strings.Contains(got, want) {
			t.Errorf("clone --help output missing %q; got:\n%s", want, got)
		}
	}
}

// TestRunCLI_PairsReplacesStatus verifies that "warp pairs" reaches the former status
// handler (ok=true with a "pairs" key), and that "warp status" is now an unknown
// subcommand returning ok=false at exit 1.
func TestRunCLI_PairsReplacesStatus(t *testing.T) {
	// pairs needs a valid warp layout to resolve config and enumerate worktrees.
	setupCLIRepo(t)

	t.Run("pairs_runs_pairs_handler", func(t *testing.T) {
		var buf bytes.Buffer
		code := warpcli.RunCLI(&buf, []string{"pairs"})
		if code != 0 {
			t.Errorf("RunCLI(pairs) = %d; want 0\noutput: %s", code, buf.String())
		}
		result := decodeResult(t, &buf)
		if ok, _ := result["ok"].(bool); !ok {
			t.Errorf("RunCLI(pairs) ok = %v; want true", result["ok"])
		}
		if _, hasPairs := result["pairs"]; !hasPairs {
			t.Errorf("RunCLI(pairs) output missing 'pairs' key; got %v", result)
		}
	})

	t.Run("status_is_unknown_subcommand", func(t *testing.T) {
		var buf bytes.Buffer
		code := warpcli.RunCLI(&buf, []string{"status"})
		if code != 1 {
			t.Errorf("RunCLI(status) = %d; want 1", code)
		}
		result := decodeResult(t, &buf)
		if ok, _ := result["ok"].(bool); ok {
			t.Errorf("RunCLI(status) ok = %v; want false", result["ok"])
		}
		errMsg, _ := result["error"].(string)
		if !strings.Contains(errMsg, "unknown") {
			t.Errorf("RunCLI(status) error = %q; want \"unknown\" substring", errMsg)
		}
	})
}

// TestRunCLI_AddHelpForksFromHead asserts that "warp add --help" Long contains the
// fork-point wording explaining that the new pair branches from the caller's HEAD.
func TestRunCLI_AddHelpForksFromHead(t *testing.T) {
	var buf bytes.Buffer
	code := warpcli.RunCLI(&buf, []string{"add", "--help"})
	if code != 0 {
		t.Fatalf("RunCLI(add --help) = %d; want 0", code)
	}
	got := buf.String()
	for _, want := range []string{"HEAD", "worktree"} {
		if !strings.Contains(got, want) {
			t.Errorf("add --help output missing %q; got:\n%s", want, got)
		}
	}
}

// TestRunDispatchesToWarp covers the subcommand router: a successful list, the unknown-
// subcommand error envelope, and remove with --force flag parsing.
func TestRunDispatchesToWarp(t *testing.T) {
	// One shared hub for the two read-only subtests; RunCLI reads os.Getwd() so
	// t.Chdir is set once in the parent and inherited by both sequential subtests.
	setupCLIRepo(t)

	t.Run("List", func(t *testing.T) {
		var buf bytes.Buffer
		if got := warpcli.RunCLI(&buf, []string{"list"}); got != 0 {
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
		var buf bytes.Buffer
		if got := warpcli.RunCLI(&buf, []string{"bogus"}); got != 1 {
			t.Errorf("RunCLI(bogus) = %d; want 1", got)
		}

		// GroupRunE now wraps the error in a JSON envelope; parse and assert ok=false.
		result := decodeResult(t, &buf)
		if ok, _ := result["ok"].(bool); ok {
			t.Errorf("RunCLI(bogus) ok = %v; want false", result["ok"])
		}
		// The error text contains "unknown" (GroupRunE produces "unknown subcommand").
		if errMsg, _ := result["error"].(string); !strings.Contains(errMsg, "unknown") {
			t.Errorf("RunCLI(bogus) error = %q; want \"unknown\" substring", errMsg)
		}
	})

	t.Run("RemoveWithForceFlag", func(t *testing.T) {
		hub := setupCLIRepo(t)
		// CopyHostHub already provides origin; no need for addRemote.

		// Create a second worktree the remove subcommand will tear down.
		slug := "test-wt"
		branch := "wt-" + slug
		target := filepath.Join(filepath.Dir(hub), slug)
		lyxtest.MustRun(t, hub, "git", "worktree", "add", "-b", branch, target)

		var buf bytes.Buffer
		if got := warpcli.RunCLI(&buf, []string{"remove", "--force", slug}); got != 0 {
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
