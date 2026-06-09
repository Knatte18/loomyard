// main_test.go — tests for the module dispatcher (main.go).
//
// Drives run() directly: argument routing, unknown-module handling, and that a
// dispatched module's exit code and output propagate unchanged.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests cover main's own responsibility — module routing — not the board
// behaviour itself (that lives in internal/board). They drive run() directly so
// no binary build or os.Exit is involved.

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 1 {
		t.Fatalf("expected exit 1 for no args, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no module output, got %q", out.String())
	}
}

func TestRunUnknownModule(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"bogus", "list"}, &out); code != 1 {
		t.Fatalf("expected exit 1 for unknown module, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no module output, got %q", out.String())
	}
}

func TestRunDispatchesToBoard(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Create temp cwd with _mhgo/board.yaml
	cwd := t.TempDir()
	mhgoDir := filepath.Join(cwd, "_mhgo")
	if err := os.MkdirAll(mhgoDir, 0o755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}
	configPath := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(configPath, []byte("path: board\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"board", "rerender"}, &out)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, out.String())
	}

	// run must forward the board module's JSON to out unchanged.
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse board output: %v; output: %s", err, out.String())
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected ok=true from dispatched board command, got %v", result)
	}
}

func TestRunBoardErrorPropagatesExitCode(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Create temp cwd with _mhgo/board.yaml
	cwd := t.TempDir()
	mhgoDir := filepath.Join(cwd, "_mhgo")
	if err := os.MkdirAll(mhgoDir, 0o755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}
	configPath := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(configPath, []byte("path: board\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	t.Chdir(cwd)

	// remove of a nonexistent task fails — exit code must bubble up through run.
	var out bytes.Buffer
	code := run([]string{"board", "remove", `{"id_or_slug":"nope"}`}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 from failing board command, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
	}
}
