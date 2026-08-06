// exitcode_test.go asserts the exit-code contract for the lyx cobra root via the run() seam.
// It covers four distinct exit paths: help (exit 0), unknown command (exit 1, cobra text), handler
// failure (exit 1, JSON envelope), and confirms that help paths never emit a JSON error envelope.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// setupBoardConfig creates a minimal board.yaml in a temp directory and changes cwd.
func setupBoardConfig(t *testing.T) {
	t.Helper()
	cwd := t.TempDir()

	lyxDir := filepath.Join(cwd, configengine.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("setupBoardConfig: MkdirAll _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("setupBoardConfig: MkdirAll _lyx/config: %v", err)
	}
	configPath := configengine.ConfigFile(cwd, "board")
	boardConfig := "path: board\nreadme: Home.md\ndesign_prefix: proposal-\n"
	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("setupBoardConfig: write board.yaml: %v", err)
	}
	t.Chdir(cwd)
}

// TestExitCode_HelpPaths asserts help paths exit 0 and never emit JSON error envelopes.
func TestExitCode_HelpPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"bare lyx", nil},
		{"lyx board (no subcommand)", []string{"board"}},
		{"lyx --help", []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run(tt.args, &out)
			if code != 0 {
				t.Errorf("run(%v) = %d; want 0. output:\n%s", tt.args, code, out.String())
			}

			got := out.String()
			if strings.Contains(got, `"ok":false`) {
				t.Errorf("help path %v emitted error envelope; output:\n%s", tt.args, got)
			}
		})
	}
}

// TestExitCode_UnknownModule asserts unknown modules exit 1 with "unknown command" in JSON error
// field.
func TestExitCode_UnknownModule(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"bogus"}, &out)
	if code != 1 {
		t.Fatalf("run([bogus]) = %d; want 1. output:\n%s", code, out.String())
	}

	if !strings.Contains(out.String(), "unknown command") {
		t.Fatalf("expected 'unknown command' in output for unknown module; got:\n%s", out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("run([bogus]) output is not valid JSON: %v; output:\n%s", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Fatalf("run([bogus]) envelope ok = true; want false")
	}
}

// TestExitCode_HandlerFailure asserts handler failures exit 1 with JSON {"ok":false} envelope.
func TestExitCode_HandlerFailure(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	setupBoardConfig(t)

	var out bytes.Buffer
	code := run([]string{"board", "upsert"}, &out)
	if code != 1 {
		t.Fatalf("run([board upsert]) = %d; want 1. output:\n%s", code, out.String())
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Fatalf("expected JSON error envelope; got:\n%s", got)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &env); err != nil {
		t.Fatalf("error envelope is not valid JSON: %v\noutput:\n%s", err, got)
	}
}
