// exitcode_test.go asserts the exit-code contract for the lyx cobra root via the
// run() seam. It covers four distinct exit paths: help (exit 0), unknown command
// (exit 1, cobra text), handler failure (exit 1, JSON envelope), and confirms that
// help paths never emit a JSON error envelope.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// setupBoardConfig creates a minimal _lyx/config/board.yaml in a temp directory
// and changes the test's working directory to that temp dir. This allows board's
// PersistentPreRunE to resolve config without requiring a real board repo on disk.
// The working directory is restored automatically by t.Chdir's cleanup.
func setupBoardConfig(t *testing.T) {
	t.Helper()
	cwd := t.TempDir()

	lyxDir := filepath.Join(cwd, paths.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("setupBoardConfig: MkdirAll _lyx: %v", err)
	}
	configDir := paths.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("setupBoardConfig: MkdirAll _lyx/config: %v", err)
	}
	configPath := paths.ConfigFile(cwd, "board")
	boardConfig := "path: board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("setupBoardConfig: write board.yaml: %v", err)
	}
	t.Chdir(cwd)
}

// TestExitCode_HelpPaths asserts that help invocation paths all exit 0 and never
// emit a JSON error envelope. Bare "lyx", a bare verb-module name, and "lyx --help"
// all trigger cobra's subcommand listing or root help, which is a successful help path.
func TestExitCode_HelpPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"bare lyx", nil},
		{"lyx board (no subcommand)", []string{"board"}},
		{"lyx warp (no subcommand)", []string{"warp"}},
		{"lyx --help", []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run(tt.args, &out)
			if code != 0 {
				t.Errorf("run(%v) = %d; want 0. output:\n%s", tt.args, code, out.String())
			}

			// Help paths must never emit a JSON error envelope.
			got := out.String()
			if strings.Contains(got, `"ok":false`) {
				t.Errorf("help path %v emitted error envelope; output:\n%s", tt.args, got)
			}
		})
	}
}

// TestExitCode_UnknownModule asserts that an unknown module (an argument that does
// not match any registered subcommand on the root) exits 1, contains cobra's
// "unknown command" text inside the JSON error field, and is a well-formed
// JSON envelope with ok=false. The plain-text "unknown command" substring must still
// be reachable inside the JSON value so callers can programmatically identify the class
// of error.
func TestExitCode_UnknownModule(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"bogus"}, &out)
	if code != 1 {
		t.Fatalf("run([bogus]) = %d; want 1. output:\n%s", code, out.String())
	}

	// The "unknown command" text must be present — now embedded in the JSON error value.
	if !strings.Contains(out.String(), "unknown command") {
		t.Fatalf("expected 'unknown command' in output for unknown module; got:\n%s", out.String())
	}

	// The output must be a well-formed JSON envelope with ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("run([bogus]) output is not valid JSON: %v; output:\n%s", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Fatalf("run([bogus]) envelope ok = true; want false")
	}
}

// TestExitCode_HandlerFailure asserts that a real handler failure exits 1 and emits
// a JSON {"ok":false} envelope on stdout. We drive "lyx board upsert" with no JSON
// payload: the board handler returns "json payload required" without needing a live
// board repo, so this is deterministic and needs no external state beyond a valid
// board config file (which setupBoardConfig provides). BOARD_SKIP_GIT prevents any
// git operations from being attempted.
func TestExitCode_HandlerFailure(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	setupBoardConfig(t)

	var out bytes.Buffer
	// "board upsert" with no positional arg causes the handler to return
	// {"ok":false,"error":"json payload required"} without contacting any remote.
	code := run([]string{"board", "upsert"}, &out)
	if code != 1 {
		t.Fatalf("run([board upsert]) = %d; want 1. output:\n%s", code, out.String())
	}

	// Handler must emit a JSON error envelope, not a cobra-level text error.
	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Fatalf("expected JSON error envelope; got:\n%s", got)
	}

	// Confirm the envelope is valid JSON so callers can unmarshal it.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &env); err != nil {
		t.Fatalf("error envelope is not valid JSON: %v\noutput:\n%s", err, got)
	}
}
