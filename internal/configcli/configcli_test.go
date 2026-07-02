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

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// fakeEditor returns a fake EditorFunc that writes the given valid YAML
// and returns the given error.
func fakeEditor(validYAML string, returnErr error) configengine.EditorFunc {
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
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	assertJSONOkContains(t, output, map[string]any{"module": "warp"})
}

// TestEditOneUnknownModule tests unknown module handling.
func TestEditOneUnknownModule(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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

// TestEditOneAbort tests the abort path: editor returns error (configengine.ErrAborted).
func TestEditOneAbort(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	l := &hubgeometry.Layout{
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
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	l := &hubgeometry.Layout{
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
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	l := &hubgeometry.Layout{
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
	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create board.yaml and warp.yaml to mark them as (configured)
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# board\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "warp"), []byte("# warp\n"), 0o644); err != nil {
		t.Fatalf("failed to write warp.yaml: %v", err)
	}
	// weft.yaml not created, so it should show (default)

	l := &hubgeometry.Layout{
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
func makeNeverCalledEditor(t *testing.T) configengine.EditorFunc {
	t.Helper()
	return func(path string) error {
		t.Helper()
		t.Errorf("editor was called on path %q; --print must never launch the editor", path)
		return nil
	}
}

// makeLayoutAt returns a minimal *hubgeometry.Layout with WorktreeRoot at baseDir and RelPath ".".
func makeLayoutAt(baseDir string) *hubgeometry.Layout {
	return &hubgeometry.Layout{
		WorktreeRoot: baseDir,
		RelPath:      ".",
	}
}

// seedModuleConfig writes YAML content to the config file for the named module under baseDir.
func seedModuleConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()
	dir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
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

// assertJSONOkContains verifies that output is a well-formed JSON success envelope
// with ok:true and, for each key in wantFields, an equal value. Callers that need to
// assert a field's absence (e.g. "preserved" on a clean write) should decode env
// themselves rather than stretching this helper to cover that case.
func assertJSONOkContains(t *testing.T, output string, wantFields map[string]any) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v; got %q", err, output)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("JSON envelope ok = false; want true")
	}
	for key, want := range wantFields {
		got, present := env[key]
		if !present {
			t.Errorf("JSON envelope missing field %q; got %v", key, env)
			continue
		}
		if wantStr, ok := want.(string); ok {
			if gotStr, _ := got.(string); gotStr != wantStr {
				t.Errorf("JSON envelope field %q = %q; want %q", key, gotStr, wantStr)
			}
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("JSON envelope field %q = %v; want %v", key, got, want)
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
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true, nil)

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
	if err := os.MkdirAll(hubgeometry.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true, nil)

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
	code := dispatch(l, nil, &out, nil, makeNeverCalledEditor(t), nil, true, nil)

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
	code := dispatch(l, nil, &out, []string{"bogus"}, makeNeverCalledEditor(t), nil, true, nil)

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

// countingEditor returns a configengine.EditorFunc that increments *calls
// every time it is invoked, so tests can assert the --set path never opens
// the editor by asserting the counter stays at 0.
func countingEditor(calls *int) configengine.EditorFunc {
	return func(path string) error {
		*calls++
		return nil
	}
}

// TestDispatchSet_NeverInvokesEditor verifies that a successful --set
// invocation never calls the injected EditorFunc.
func TestDispatchSet_NeverInvokesEditor(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, []string{"warp"}, countingEditor(&editorCalls), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

	if code != 0 {
		t.Errorf("dispatch(--set) = %d; want 0; output: %q", code, out.String())
	}
	if editorCalls != 0 {
		t.Errorf("dispatch(--set) invoked the editor %d times; want 0", editorCalls)
	}
	assertJSONOkContains(t, out.String(), map[string]any{"module": "warp"})
}

// TestDispatchSet_UnknownKeyNeverSyncs verifies that an unknown key passed to
// --set returns an error and the injected sync function is never invoked.
func TestDispatchSet_UnknownKeyNeverSyncs(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, []string{"warp"}, countingEditor(&editorCalls), tracker.syncFunc(), false, []string{"bogus_key=x"})

	if code != 1 {
		t.Errorf("dispatch(--set unknown key) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called when --set names an unknown key")
	}
	assertJSONErrContains(t, out.String(), "unknown config key")
}

// TestDispatchSet_PrintMutuallyExclusive verifies that passing both --print
// and --set returns the mutual-exclusivity error, with neither the editor
// nor sync invoked.
func TestDispatchSet_PrintMutuallyExclusive(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, []string{"warp"}, countingEditor(&editorCalls), tracker.syncFunc(), true, []string{"branch_prefix=new-"})

	if code != 1 {
		t.Errorf("dispatch(--print, --set) = %d; want 1", code)
	}
	if editorCalls != 0 {
		t.Errorf("dispatch(--print, --set) invoked the editor %d times; want 0", editorCalls)
	}
	if tracker.called {
		t.Error("sync should not be called when --print and --set are both set")
	}
	assertJSONErrContains(t, out.String(), "mutually exclusive")
}

// TestDispatchSet_NoModuleRequiresOne verifies that --set with no module
// positional returns the module-required error.
func TestDispatchSet_NoModuleRequiresOne(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, nil, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

	if code != 1 {
		t.Errorf("dispatch(--set, no module) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "module required with --set")
}

// TestDispatchSet_MultipleValuesOneSync verifies that multiple --set values
// in one dispatch() call all land in a single sync invocation.
func TestDispatchSet_MultipleValuesOneSync(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	syncCalls := 0
	sync := func(w io.Writer) int {
		syncCalls++
		return 0
	}
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), sync, false, []string{"branch_prefix=new-"})

	if code != 0 {
		t.Errorf("dispatch(--set multiple) = %d; want 0; output: %q", code, out.String())
	}
	if syncCalls != 1 {
		t.Errorf("dispatch(--set multiple) called sync %d times; want 1", syncCalls)
	}
	assertJSONOkContains(t, out.String(), map[string]any{"module": "warp"})
}

// TestDispatchSet_MalformedValue verifies that a malformed --set value with
// no '=' returns the parseSetFlags error.
func TestDispatchSet_MalformedValue(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"no-equals-sign"})

	if code != 1 {
		t.Errorf("dispatch(--set malformed) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called for a malformed --set value")
	}
	assertJSONErrContains(t, out.String(), "expected key=value")
}

// TestConfigLong_MentionsEditorFallbackAndSet verifies that buildConfigLong's
// output documents both the EDITOR/VISUAL editor fallback and the --set flag.
func TestConfigLong_MentionsEditorFallbackAndSet(t *testing.T) {
	longText := buildConfigLong()
	if !strings.Contains(longText, "EDITOR") || !strings.Contains(longText, "VISUAL") {
		t.Errorf("config Long missing EDITOR/VISUAL fallback documentation; Long = %q", longText)
	}
	if !strings.Contains(longText, "--set") {
		t.Errorf("config Long missing --set documentation; Long = %q", longText)
	}
}

// TestDispatchSet_PreservesUnrecognizedKeyReportsWarning verifies that --set
// against a module file carrying an orphan key (one absent from the current
// template) preserves that key rather than dropping it, and reports it via
// the JSON envelope's "preserved" field.
func TestDispatchSet_PreservesUnrecognizedKeyReportsWarning(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\nlegacy_key: keepme\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

	if code != 0 {
		t.Fatalf("dispatch(--set, orphan key) = %d; want 0; output: %q", code, out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v; got %q", err, out.String())
	}
	preserved, ok := env["preserved"].([]any)
	if !ok {
		t.Fatalf("JSON envelope missing \"preserved\" field or wrong type; got %v", env)
	}
	if len(preserved) != 1 || preserved[0] != "legacy_key" {
		t.Errorf("preserved = %v; want [\"legacy_key\"]", preserved)
	}
}

// TestDispatchSet_CleanFileNoPreservedField verifies that --set against a
// module file with no orphan keys emits a JSON envelope with no "preserved"
// field at all, rather than an empty one.
func TestDispatchSet_CleanFileNoPreservedField(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

	if code != 0 {
		t.Fatalf("dispatch(--set, clean file) = %d; want 0; output: %q", code, out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v; got %q", err, out.String())
	}
	if _, ok := env["preserved"]; ok {
		t.Errorf("JSON envelope has a \"preserved\" field on a clean write; got %v", env)
	}
}

// TestDispatchSet_PreservedKeyDetectedByReconcile is the end-to-end test that
// closes the loop on the task's second symptom: reconcile "not detecting
// drift". It chains --set into reconcile so that a preserved orphan key
// planted by --set is then correctly reported by reconcile's own
// drift-detection, proving reconcile never gets a chance to look once --set
// stops silently destroying the key first.
func TestDispatchSet_PreservedKeyDetectedByReconcile(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo so hubgeometry.Resolve works for the
	// reconcile call below (RunCLI resolves its layout from cwd).
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	seedModuleConfig(t, tmpDir, "warp", "branch_prefix: old-\nlegacy_key: keepme\n")

	// Run --set via dispatch, exactly as
	// TestDispatchSet_PreservesUnrecognizedKeyReportsWarning does, using an
	// explicit *hubgeometry.Layout (dispatch takes one directly, unlike
	// RunCLI which resolves it from cwd).
	var setOut bytes.Buffer
	setCode := dispatch(makeLayoutAt(tmpDir), nil, &setOut, []string{"warp"}, makeNeverCalledEditor(t), (&fakeSyncTracker{exitCode: 0}).syncFunc(), false, []string{"branch_prefix=new-"})
	if setCode != 0 {
		t.Fatalf("dispatch(--set) = %d; want 0; output: %q", setCode, setOut.String())
	}

	// Chdir into the temp repo so hubgeometry.Getwd inside RunCLI resolves
	// there, then run reconcile.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCwd) //nolint:errcheck

	var reconcileOut bytes.Buffer
	reconcileCode := RunCLI(&reconcileOut, []string{"reconcile"})
	if reconcileCode != 0 {
		t.Fatalf("RunCLI(reconcile) = %d; want 0; output: %q", reconcileCode, reconcileOut.String())
	}

	var result map[string]any
	if err := json.Unmarshal(reconcileOut.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, reconcileOut.String())
	}
	modules, ok := result["modules"].([]any)
	if !ok {
		t.Fatalf("modules is not an array; got %v", result)
	}
	var warpMod map[string]any
	for _, m := range modules {
		mod, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if mod["module"] == "warp" {
			warpMod = mod
			break
		}
	}
	if warpMod == nil {
		t.Fatalf("no modules entry for \"warp\"; got %v", modules)
	}
	removed, ok := warpMod["removed"].([]any)
	if !ok {
		t.Fatalf("warp module entry missing \"removed\" field or wrong type; got %v", warpMod)
	}
	found := false
	for _, r := range removed {
		if r == "legacy_key" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warp module's removed = %v; want it to contain \"legacy_key\"", removed)
	}
}
