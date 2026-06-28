// configcli_test.go — unit and integration tests for configcli.
//
// Unit tests (untagged): dispatch/editOne/printModule/printAll with fake editor+sync
// over temp baseDirs seeded via the paths helpers. Integration test (//go:build
// integration): e2e test with real weft.RunCLI over CopyPaired.

package configcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/config"
	"github.com/Knatte18/loomyard/internal/configreg"
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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "warp", fakeEditor("branch_prefix: test\n", nil), tracker.syncFunc())

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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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

	// Verify the output is a valid JSON error envelope — errors are no longer plain text.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &env); err != nil {
		t.Fatalf("editOne(unknown) output is not valid JSON: %v; got %q", err, output)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("editOne(unknown) envelope ok = true; want false")
	}
	msg, _ := env["error"].(string)
	if !strings.Contains(msg, "unknown config module") {
		t.Errorf("editOne(unknown) error field missing 'unknown config module'; got %q", msg)
	}
	if !strings.Contains(msg, "known:") {
		t.Errorf("editOne(unknown) error field missing known-module list; got %q", msg)
	}
}

// TestEditOneAbort tests the abort path: editor returns error (config.ErrAborted).
func TestEditOneAbort(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "warp", fakeEditor("test\n", errors.New("simulated editor exit 1")), tracker.syncFunc())

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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create board.yaml and warp.yaml to mark them as (configured)
	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# board\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	if err := os.WriteFile(paths.ConfigFile(baseDir, "warp"), []byte("# warp\n"), 0o644); err != nil {
		t.Fatalf("failed to write warp.yaml: %v", err)
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
	if !strings.Contains(output, "warp (configured)") {
		t.Errorf("menu output missing 'warp (configured)'; got %q", output)
	}
	if !strings.Contains(output, "weft (default)") {
		t.Errorf("menu output missing 'weft (default)'; got %q", output)
	}
}

// makeNeverCalledEditor returns an EditorFunc that fails the test if called.
// Passed to dispatch in --print tests to prove the print path never opens an editor.
func makeNeverCalledEditor(t *testing.T) config.EditorFunc {
	t.Helper()
	return func(path string) error {
		t.Helper()
		t.Errorf("editor was called on path %q; --print must never launch the editor", path)
		return nil
	}
}

// makeLayoutAt returns a minimal *paths.Layout with WorktreeRoot at baseDir and RelPath ".".
func makeLayoutAt(baseDir string) *paths.Layout {
	return &paths.Layout{
		WorktreeRoot: baseDir,
		RelPath:      ".",
	}
}

// seedModuleConfig writes YAML content to the config file for the named module under baseDir.
func seedModuleConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()
	dir := paths.ConfigDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(paths.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to seed config for module %s: %v", module, err)
	}
}

// assertJSONErrContains verifies that output is a well-formed JSON error envelope
// with ok:false and an error field containing wantSubstr.
func assertJSONErrContains(t *testing.T, output, wantSubstr string) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v; got %q", err, output)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("JSON envelope ok = true; want false")
	}
	if wantSubstr != "" {
		msg, _ := env["error"].(string)
		if !strings.Contains(msg, wantSubstr) {
			t.Errorf("JSON error field missing %q; got %q", wantSubstr, msg)
		}
	}
}

// TestPrintModule_Seeded verifies that config <module> --print emits the on-disk
// YAML verbatim at exit 0 and never invokes the editor.
func TestPrintModule_Seeded(t *testing.T) {
	baseDir := t.TempDir()
	const warpYAML = "branch_prefix: feature/\n"
	seedModuleConfig(t, baseDir, "warp", warpYAML)

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true)

	if code != 0 {
		t.Errorf("dispatch(print=true, seeded) = %d; want 0; output: %q", code, out.String())
	}
	if got := out.String(); got != warpYAML {
		t.Errorf("dispatch(print=true, seeded) output = %q; want %q", got, warpYAML)
	}
}

// TestPrintModule_KnownButUnseeded verifies that config <module> --print for a known
// module with no on-disk file returns an ok:false JSON envelope at exit 1.
func TestPrintModule_KnownButUnseeded(t *testing.T) {
	baseDir := t.TempDir()
	// Create the config directory but not the warp.yaml file.
	if err := os.MkdirAll(paths.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true)

	if code != 1 {
		t.Errorf("dispatch(print=true, unseeded) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "not configured")
}

// TestPrintAggregate_PartialSeed verifies the aggregate --print form with a partial
// module seed. It asserts deterministic headers for every registry module, inline YAML
// for seeded ones, and # (not configured) for absent ones, all at exit 0.
func TestPrintAggregate_PartialSeed(t *testing.T) {
	baseDir := t.TempDir()
	const boardYAML = "path: board\nhome: Home.md\n"
	seedModuleConfig(t, baseDir, "board", boardYAML)
	// warp and weft are intentionally not seeded.

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, nil, &out, nil, makeNeverCalledEditor(t), nil, true)

	if code != 0 {
		t.Errorf("dispatch(print=true, aggregate) = %d; want 0; output: %q", code, out.String())
	}
	got := out.String()

	// Every registry module must have a section header in output order.
	for _, name := range configreg.Names() {
		if !strings.Contains(got, "# "+name) {
			t.Errorf("aggregate output missing header for %q; output:\n%s", name, got)
		}
	}
	// board is seeded; its YAML content must appear.
	if !strings.Contains(got, "path: board") {
		t.Errorf("aggregate output missing seeded board YAML; output:\n%s", got)
	}
	// warp and weft are absent; their sections must each say # (not configured).
	if count := strings.Count(got, "# (not configured)"); count < 2 {
		t.Errorf("expected ≥2 '# (not configured)' lines; got %d; output:\n%s", count, got)
	}
}

// TestPrintUnknownModule verifies that config bogus --print returns an ok:false JSON
// envelope at exit 1 whose error field names the unknown module.
func TestPrintUnknownModule(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, nil, &out, []string{"bogus"}, makeNeverCalledEditor(t), nil, true)

	if code != 1 {
		t.Errorf("dispatch(print=true, unknown) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "unknown config module")
}

// TestConfigLong_ContainsModuleNames verifies that the config command's Long help text
// includes every name from configreg.Names(), proving the help text stays in sync with
// the registry rather than drifting from a hardcoded list.
func TestConfigLong_ContainsModuleNames(t *testing.T) {
	longText := Command().Long
	for _, name := range configreg.Names() {
		if !strings.Contains(longText, name) {
			t.Errorf("config Long missing module name %q; Long = %q", name, longText)
		}
	}
}
