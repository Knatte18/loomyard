//go:build tier1

// cli_test.go covers the ide CLI cobra surface: spawn dispatch with a stubbed
// launcher, seed with and without a slug, the non-git refusal and the missing-slug error.

package idecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/ideengine"
)

// TestRunCLI_SpawnScenario drives "lym ide spawn" against one hub: dispatch with a stubbed launcher and a temporary keybindings file, then the missing-slug error.
// Neither step depends on the other's state.
// Stays serial (no t.Parallel): the dispatch step swaps the package-level ideengine.CodeLauncher and ideengine.KeybindingsPath and restores them in a defer, which under t.Parallel() is both a data race on a production package-level variable and a restore firing while sibling tests still run.
func TestRunCLI_SpawnScenario(t *testing.T) {
	// Create a real hub so lymcwd.Resolve succeeds inside the PersistentPreRunE.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "child", fabricengine.AddOptions{SkipPush: true})

	// Stub ideengine.CodeLauncher with a recorder so the test does not open VS Code.
	originalLauncher, originalKeybindingsPath := ideengine.CodeLauncher, ideengine.KeybindingsPath
	defer func() { ideengine.CodeLauncher, ideengine.KeybindingsPath = originalLauncher, originalKeybindingsPath }()
	var launches []string
	var launchErr error
	ideengine.CodeLauncher = func(dir string) error {
		launches = append(launches, dir)
		return launchErr
	}

	if !t.Run("dispatch", func(t *testing.T) {
		// Point the keybindings seam at a temporary file.
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
		if err != nil || !strings.Contains(string(data), "// lym:begin") || !strings.Contains(string(data), "workbench.action.terminal.sendSequence") {
			t.Errorf("seeded file = %q, %v; want the lym block", data, err)
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

	t.Run("seed writes the config and launches nothing", func(t *testing.T) {
		launches = nil
		var out bytes.Buffer
		code := RunCLIIn(h.PrimeWorktree(), &out, []string{"seed", "child"})

		if code != 0 {
			t.Fatalf("RunCLI(seed child) = %d; output: %s", code, out.String())
		}
		for _, name := range []string{"settings.json", "tasks.json"} {
			if _, err := os.Stat(filepath.Join(h.PairCodeWorktree("child"), ".vscode", name)); err != nil {
				t.Errorf(".vscode/%s: %v", name, err)
			}
		}
		if len(launches) != 0 {
			t.Errorf("seed launched %v; want no launch", launches)
		}
	})

	t.Run("seed without a slug seeds the worktree it runs in, the prime included", func(t *testing.T) {
		launches = nil
		var out bytes.Buffer
		code := RunCLIIn(h.PrimeWorktree(), &out, []string{"seed"})

		if code != 0 {
			t.Fatalf("RunCLI(seed) in the prime = %d; output: %s", code, out.String())
		}
		var envelope struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("envelope %q: %v", out.String(), err)
		}
		if want := filepath.Base(h.PrimeWorktree()); envelope.Slug != want {
			t.Errorf("seeded slug = %q; want the prime %q", envelope.Slug, want)
		}
		if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), ".vscode", "settings.json")); err != nil {
			t.Errorf("prime .vscode/settings.json: %v", err)
		}
		if len(launches) != 0 {
			t.Errorf("seed launched %v; want no launch", launches)
		}
	})

	t.Run("open launches once on the pair folder", func(t *testing.T) {
		launches = nil
		var out bytes.Buffer
		code := RunCLIIn(h.PrimeWorktree(), &out, []string{"open", "child"})

		if code != 0 {
			t.Fatalf("RunCLI(open child) = %d; output: %s", code, out.String())
		}
		if len(launches) != 1 || launches[0] != h.PairCodeWorktree("child") {
			t.Errorf("launches = %v; want exactly [%s]", launches, h.PairCodeWorktree("child"))
		}
	})

	refusals := []struct {
		name      string
		args      []string
		launchErr error
		wantVerb  string
		wantUsage bool
	}{
		{name: "open without a slug", args: []string{"open"}, wantVerb: "usage: lym ide open", wantUsage: true},
		{name: "seed of a slug with no pair", args: []string{"seed", "nosuch"}, wantVerb: "seed"},
		{name: "open of a slug with no pair", args: []string{"open", "nosuch"}, wantVerb: "open"},
		{name: "open of a path-shaped slug", args: []string{"open", "../child"}, wantVerb: "open"},
		{name: "open whose launcher fails", args: []string{"open", "child"}, launchErr: errors.New("no editor"), wantVerb: "open"},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			launches, launchErr = nil, tt.launchErr
			defer func() { launchErr = nil }()
			var out bytes.Buffer
			code := RunCLIIn(h.PrimeWorktree(), &out, tt.args)

			if code == 0 {
				t.Fatalf("RunCLI(%v) = 0; want non-zero; output: %s", tt.args, out.String())
			}
			if !strings.Contains(out.String(), tt.wantVerb) {
				t.Errorf("RunCLI(%v) output = %q; want it to name %q", tt.args, out.String(), tt.wantVerb)
			}
			wantLaunches := 0
			if tt.launchErr != nil {
				wantLaunches = 1
			}
			if len(launches) != wantLaunches {
				t.Errorf("RunCLI(%v) recorded launches %v; want %d", tt.args, launches, wantLaunches)
			}
		})
	}
}

// TestRunCLI_NotAGitRepo verifies that "lym ide menu" run from a non-git temp directory surfaces lymcwd's bare ErrNotAGitRepo sentinel with no "failed to resolve layout:" prefix and no raw "fatal:" git stderr — the PersistentPreRunE aborts before menu's body runs, so the interactive picker is never reached.
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
