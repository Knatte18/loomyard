// exitcode_test.go asserts the exit-code contract for the lyx cobra root via the run() seam.
// It covers four distinct exit paths: help (exit 0), unknown command (exit 1, cobra text), handler
// failure (exit 1, JSON envelope), and confirms that help paths never emit a JSON error envelope.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// setupBoardConfig creates a minimal board.yaml in a temp directory and changes cwd.
func setupBoardConfig(t *testing.T) {
	t.Helper()
	cwd := t.TempDir()

	lyxDir := filepath.Join(cwd, lyxdirs.LyxDirName)
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

// TestExitCode_HelpPaths asserts help paths exit 0, never emit JSON error envelopes, and name the modules or text a help listing must carry.
// run() rewrites package-global flag state in newRoot, so neither the test nor its rows run in parallel.
func TestExitCode_HelpPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// wantInOutput is a substring the help output must carry; empty asserts only non-empty output.
		wantInOutput string
	}{
		{"bare lyx", nil, "board"},
		{"lyx board (no subcommand)", []string{"board"}, ""},
		{"lyx --help", []string{"--help"}, ""},
		{"lyx config --help lists the reconcile verb", []string{"config", "--help"}, "reconcile"},
		{"lyx orch start --help lists the --adopt flag", []string{"orch", "start", "--help"}, "--adopt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run(tt.args, &out)
			if code != 0 {
				t.Errorf("run(%v) = %d; want 0. output:\n%s", tt.args, code, out.String())
			}

			got := out.String()
			if got == "" {
				t.Errorf("run(%v) printed no help output", tt.args)
			}
			if !strings.Contains(got, tt.wantInOutput) {
				t.Errorf("help output of %v does not name %q; got:\n%s", tt.args, tt.wantInOutput, got)
			}
			if strings.Contains(got, `"ok":false`) {
				t.Errorf("help path %v emitted error envelope; output:\n%s", tt.args, got)
			}
		})
	}
}

// TestExitCode_UnknownModule asserts unknown modules, including the removed "update" verb, exit 1 with "unknown command" in the JSON error field.
// run() rewrites package-global flag state in newRoot, so neither the test nor its rows run in parallel.
func TestExitCode_UnknownModule(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"bare unknown module", []string{"bogus"}},
		{"unknown module with a verb", []string{"bogus", "list"}},
		{"update was folded into config reconcile", []string{"update"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run(tt.args, &out)
			if code != 1 {
				t.Fatalf("run(%v) = %d; want 1. output:\n%s", tt.args, code, out.String())
			}

			envelope.RequireErr(t, out.String(), "unknown command")
		})
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

	envelope.Decode(t, got)
}
