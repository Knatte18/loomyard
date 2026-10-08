//go:build integration

// cli_test.go covers the ide CLI cobra surface: spawn dispatch with a stubbed
// launcher, the non-git refusal and the missing-slug error.

package idecli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/ideengine"
)

// TestRunCLI_SpawnScenario drives "lyx ide spawn" against one hub: dispatch with a stubbed launcher and a temporary keybindings file, then the missing-slug error.
// Neither step depends on the other's state.
// Stays serial (no t.Parallel): the dispatch step swaps the package-level ideengine.CodeLauncher and ideengine.KeybindingsPath and restores them in a defer, which under t.Parallel() is both a data race on a production package-level variable and a restore firing while sibling tests still run.
func TestRunCLI_SpawnScenario(t *testing.T) {
	// Create a real hub so lyxcwd.Resolve succeeds inside the PersistentPreRunE.
	h := hubforge.NewHub(t, ".")

	if !t.Run("dispatch", func(t *testing.T) {
		// Stub ideengine.CodeLauncher so the test does not open VS Code, and point the keybindings seam at a temporary file.
		originalLauncher, originalKeybindingsPath := ideengine.CodeLauncher, ideengine.KeybindingsPath
		defer func() { ideengine.CodeLauncher, ideengine.KeybindingsPath = originalLauncher, originalKeybindingsPath }()
		ideengine.CodeLauncher = func(dir string) error { return nil }
		keybindingsPath := filepath.Join(t.TempDir(), "keybindings.json")
		ideengine.KeybindingsPath = func() (string, error) { return keybindingsPath, nil }

		var out bytes.Buffer
		code := RunCLIIn(h.PrimeWorktree(), &out, []string{"spawn", "child"})

		// spawn should succeed or fail for a handler reason, not layout resolution.
		if code != 0 && !strings.Contains(out.String(), "spawn failed") {
			t.Fatalf("unexpected error during dispatch; output: %s", out.String())
		}
		if code != 0 {
			return
		}
		var envelope struct {
			Keybindings struct {
				Outcome string `json:"outcome"`
				Reason  string `json:"reason"`
			} `json:"keybindings"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("envelope %q: %v", out.String(), err)
		}
		if envelope.Keybindings.Outcome != "created" {
			t.Errorf("keybindings = %+v; want outcome created", envelope.Keybindings)
		}
		data, err := os.ReadFile(keybindingsPath)
		if err != nil || !strings.Contains(string(data), "// lyx:begin") || !strings.Contains(string(data), "workbench.action.terminal.sendSequence") {
			t.Errorf("seeded file = %q, %v; want the lyx block", data, err)
		}
	}) {
		return
	}

	t.Run("missing slug", func(t *testing.T) {
		var out bytes.Buffer
		code := RunCLIIn(h.PrimeWorktree(), &out, []string{"spawn"})

		if code != 1 {
			t.Errorf("RunCLI(spawn) with no slug = %d; want 1", code)
		}
		if !strings.Contains(out.String(), "spawn") {
			t.Errorf("RunCLI(spawn) output missing \"spawn\"; got: %q", out.String())
		}
	})
}

// TestRunCLI_NotAGitRepo verifies that "lyx ide menu" run from a non-git temp directory surfaces
// lyxcwd's bare ErrNotAGitRepo sentinel with no "failed to resolve layout:" prefix and no raw
// "fatal:" git stderr — the PersistentPreRunE aborts before menu's body runs, so the interactive
// picker is never reached.
func TestRunCLI_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	code := RunCLIIn(tmpDir, &out, []string{"menu"})

	if code != 1 {
		t.Errorf("RunCLI(menu) in non-git dir = %d; want 1", code)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(menu) output is not valid JSON: %v; got: %q", err, out.String())
	}
	errMsg, _ := env["error"].(string)
	if errMsg != "not a git repository" {
		t.Errorf("RunCLI(menu) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}
