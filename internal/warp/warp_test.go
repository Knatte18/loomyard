//go:build integration

// warp_test.go covers the RunCLI subcommand router using a CWD-based fixture.
// These tests keep t.Chdir and stay serial (no t.Parallel) because RunCLI
// reads os.Getwd() at the call edge.

package warp_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/warp"
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

// TestRunDispatchesToWarp covers the subcommand router: a successful list, the unknown-
// subcommand error envelope, and remove with --force flag parsing.
func TestRunDispatchesToWarp(t *testing.T) {
	// One shared hub for the two read-only subtests; RunCLI reads os.Getwd() so
	// t.Chdir is set once in the parent and inherited by both sequential subtests.
	setupCLIRepo(t)

	t.Run("List", func(t *testing.T) {
		var buf bytes.Buffer
		if got := warp.RunCLI(&buf, []string{"list"}); got != 0 {
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
		if got := warp.RunCLI(&buf, []string{"bogus"}); got != 1 {
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
		if got := warp.RunCLI(&buf, []string{"remove", "--force", slug}); got != 0 {
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
