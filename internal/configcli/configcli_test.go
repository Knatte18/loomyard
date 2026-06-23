// configcli_test.go — unit and integration tests for configcli.
//
// Unit tests (untagged): dispatch/editOne with fake editor+sync over temp baseDir.
// Integration test (//go:build integration): e2e test with real weft.RunCLI over CopyPaired.

package configcli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/config"
	"github.com/Knatte18/loomyard/internal/paths"
)

// fakeEditor returns a fake EditorFunc that writes the given valid YAML
// and returns the given error.
func fakeEditor(validYAML string, returnErr error) config.EditorFunc {
	return func(path string) error {
		if returnErr != nil {
			return returnErr
		}
		return os.WriteFile(path, []byte(validYAML), 0o644)
	}
}

// fakeSyncTracker is a wrapper for a fake syncFunc that records whether it was called.
type fakeSyncTracker struct {
	called   bool
	exitCode int
}

// syncFunc returns a fake syncFunc that records the call and returns the tracked exit code.
func (t *fakeSyncTracker) syncFunc() syncFunc {
	return func(w io.Writer) int {
		t.called = true
		return t.exitCode
	}
}

// TestEditOneSuccess tests the success path: valid YAML, sync succeeds (exit 0).
func TestEditOneSuccess(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "worktree", fakeEditor("branch_prefix: test\n", nil), tracker.syncFunc())

	if code != 0 {
		t.Errorf("editOne() = %d; want 0", code)
	}
	if !tracker.called {
		t.Error("sync was not called")
	}
	output := out.String()
	if !strings.Contains(output, "edited and synced") {
		t.Errorf("editOne output missing success message; got %q", output)
	}
}

// TestEditOneUnknownModule tests unknown module handling.
func TestEditOneUnknownModule(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "unknown", fakeEditor("test\n", nil), tracker.syncFunc())

	if code != 1 {
		t.Errorf("editOne() = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called for unknown module")
	}
	output := out.String()
	if !strings.Contains(output, "unknown config module") {
		t.Errorf("editOne output missing unknown module message; got %q", output)
	}
	if !strings.Contains(output, "known:") {
		t.Errorf("editOne output missing known modules list; got %q", output)
	}
}

// TestEditOneAbort tests the abort path: editor returns error (config.ErrAborted).
func TestEditOneAbort(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "worktree", fakeEditor("test\n", errors.New("simulated editor exit 1")), tracker.syncFunc())

	if code != 1 {
		t.Errorf("editOne() = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called on abort")
	}
	output := out.String()
	if !strings.Contains(output, "aborted") {
		t.Errorf("editOne output missing abort message; got %q", output)
	}
}

// TestEditOneSyncFails tests the sync-failure path: sync returns non-zero.
func TestEditOneSyncFails(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 1}
	syncWithOutput := func(w io.Writer) int {
		tracker.called = true
		fmt.Fprint(w, "sync error: something went wrong")
		return 1
	}
	code := editOne(baseDir, &out, "weft", fakeEditor("pathspec: _lyx\n", nil), syncWithOutput)

	if code != 1 {
		t.Errorf("editOne() = %d; want 1", code)
	}
	output := out.String()
	if !strings.Contains(output, "weft sync failed") {
		t.Errorf("editOne output missing sync-failed message; got %q", output)
	}
	if !strings.Contains(output, "sync error: something went wrong") {
		t.Errorf("editOne output missing sync error details; got %q", output)
	}
}

// TestMenuSelection tests menu with a valid selection.
func TestMenuSelection(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	l := &paths.Layout{
		WorktreeRoot: baseDir,
		RelPath:      ".",
	}

	// Simulate user input: select item 1 (board), then quit
	input := strings.NewReader("1\nq\n")
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := menu(l, baseDir, input, &out, fakeEditor("test: value\n", nil), tracker.syncFunc())

	if code != 0 {
		t.Errorf("menu() = %d; want 0", code)
	}
	if !tracker.called {
		t.Error("sync should be called for selected module")
	}
	output := out.String()
	if !strings.Contains(output, "board") {
		t.Errorf("menu output missing board option; got %q", output)
	}
}

// TestMenuQuit tests menu with 'q' selection.
func TestMenuQuit(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	l := &paths.Layout{
		WorktreeRoot: baseDir,
		RelPath:      ".",
	}

	input := strings.NewReader("q\n")
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := menu(l, baseDir, input, &out, fakeEditor("test: value\n", nil), tracker.syncFunc())

	if code != 0 {
		t.Errorf("menu() = %d; want 0", code)
	}
	if tracker.called {
		t.Error("sync should not be called on quit")
	}
}

// TestMenuInvalidSelection tests menu with invalid input.
func TestMenuInvalidSelection(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	l := &paths.Layout{
		WorktreeRoot: baseDir,
		RelPath:      ".",
	}

	input := strings.NewReader("999\n")
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := menu(l, baseDir, input, &out, fakeEditor("test: value\n", nil), tracker.syncFunc())

	if code != 1 {
		t.Errorf("menu() = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called on invalid selection")
	}
	output := out.String()
	if !strings.Contains(output, "invalid selection") {
		t.Errorf("menu output missing invalid selection message; got %q", output)
	}
}

// TestMenuStatus tests that menu marks modules as (configured) or (default).
func TestMenuStatus(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create board.yaml and worktree.yaml to mark them as (configured)
	if err := os.WriteFile(filepath.Join(configDir, "board.yaml"), []byte("# board\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "worktree.yaml"), []byte("# worktree\n"), 0o644); err != nil {
		t.Fatalf("failed to write worktree.yaml: %v", err)
	}
	// weft.yaml not created, so it should show (default)

	l := &paths.Layout{
		WorktreeRoot: baseDir,
		RelPath:      ".",
	}

	input := strings.NewReader("q\n")
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	_ = menu(l, baseDir, input, &out, fakeEditor("test: value\n", nil), tracker.syncFunc())

	output := out.String()
	if !strings.Contains(output, "board (configured)") {
		t.Errorf("menu output missing 'board (configured)'; got %q", output)
	}
	if !strings.Contains(output, "worktree (configured)") {
		t.Errorf("menu output missing 'worktree (configured)'; got %q", output)
	}
	if !strings.Contains(output, "weft (default)") {
		t.Errorf("menu output missing 'weft (default)'; got %q", output)
	}
}
