// configcli_test.go — unit and integration tests for configcli.
//
// Unit tests (untagged): dispatch/editOne/printModule/printAll with fake editor+sync over temp
// baseDirs seeded via the paths helpers.
// Integration test (//go:build integration): e2e test with real fabriccli.RunCLI over a real hub
// built by the hubforge package.
// The git-init-backed TestDispatchSet_PreservedKeyDetectedByReconcile lives in
// configcli_integration_test.go per the Test Tier Purity Invariant.

package configcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "fabric", fakeEditor("branch_prefix: test\n", nil), tracker.syncFunc())

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
	assertJSONOkContains(t, output, map[string]any{"module": "fabric"})
}

// TestEditOneUnknownModule tests unknown module handling.
func TestEditOneUnknownModule(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
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
	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(baseDir, &out, "fabric", fakeEditor("test\n", errors.New("simulated editor exit 1")), tracker.syncFunc())

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
	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 1}
	syncWithOutput := func(w io.Writer) int {
		tracker.called = true
		fmt.Fprint(w, "sync error: something went wrong")
		return 1
	}
	code := editOne(baseDir, &out, "fabric", fakeEditor("pathspec: _lyx\n", nil), syncWithOutput)

	if code != 1 {
		t.Errorf("editOne() = %d; want 1", code)
	}
	output := out.String()
	if !strings.Contains(output, "fabric sync failed") {
		t.Errorf("editOne output missing sync-failed message; got %q", output)
	}
	if !strings.Contains(output, "sync error: something went wrong") {
		t.Errorf("editOne output missing sync error details; got %q", output)
	}
}

// TestBareConfigListsModulesAndVerbs verifies that bare `lyx config` from a non-git directory exits 0,
// prints help rather than an envelope, and names reconcile and every module.
func TestBareConfigListsModulesAndVerbs(t *testing.T) {
	var out bytes.Buffer
	code := RunCLIIn(t.TempDir(), &out, nil)

	if code != 0 {
		t.Fatalf("lyx config = %d; want 0; output: %q", code, out.String())
	}
	got := out.String()
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("lyx config printed an envelope; want help text: %q", got)
	}
	if !strings.Contains(got, "reconcile") {
		t.Errorf("lyx config output does not name reconcile: %q", got)
	}
	for _, name := range configreg.Names() {
		if !strings.Contains(got, name) {
			t.Errorf("lyx config output does not name module %q: %q", name, got)
		}
	}
}

// TestUnknownConfigArgumentRefuses verifies that `lyx config bogus` from a non-git directory exits 1
// with an unknown-subcommand envelope naming the argument and the way forward.
func TestUnknownConfigArgumentRefuses(t *testing.T) {
	var out bytes.Buffer
	code := RunCLIIn(t.TempDir(), &out, []string{"bogus"})

	if code != 1 {
		t.Fatalf("lyx config bogus = %d; want 1; output: %q", code, out.String())
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("output is not a JSON envelope: %v; got %q", err, out.String())
	}
	msg, _ := env["error"].(string)
	for _, want := range []string{"unknown subcommand", "bogus", `run "lyx config" to list modules and verbs`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

// TestNoModuleSharesNameWithSubcommand verifies that no configreg module is named like a config subcommand,
// since a subcommand name always routes as a subcommand.
func TestNoModuleSharesNameWithSubcommand(t *testing.T) {
	subs := map[string]bool{}
	for _, c := range Command().Commands() {
		subs[c.Name()] = true
	}
	for _, name := range configreg.Names() {
		if subs[name] {
			t.Errorf("module %q shares its name with a config subcommand", name)
		}
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

// makeLayoutAt returns a minimal *lyxcwd.Location with WorktreeRoot at baseDir and RelPath ".".
func makeLayoutAt(baseDir string) *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "."}
}

// seedModuleConfig writes YAML content to the config file for the named module under baseDir.
func seedModuleConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()
	dir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
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

// TestPrintModule_Seeded verifies that config <module> --print emits the on-disk YAML verbatim at
// exit 0 and never invokes the editor.
func TestPrintModule_Seeded(t *testing.T) {
	baseDir := t.TempDir()
	const fabricYAML = "branch_prefix: feature/\n"
	seedModuleConfig(t, baseDir, "fabric", fabricYAML)

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, []string{"fabric"}, makeNeverCalledEditor(t), nil, true, nil)

	if code != 0 {
		t.Errorf("dispatch(print=true, seeded) = %d; want 0; output: %q", code, out.String())
	}
	if got := out.String(); got != fabricYAML {
		t.Errorf("dispatch(print=true, seeded) output = %q; want %q", got, fabricYAML)
	}
}

// TestPrintModule_KnownButUnseeded verifies that config <module> --print for a known module with no
// on-disk file returns an ok:false JSON envelope at exit 1.
func TestPrintModule_KnownButUnseeded(t *testing.T) {
	baseDir := t.TempDir()
	// Create the config directory but not the fabric.yaml file.
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, []string{"fabric"}, makeNeverCalledEditor(t), nil, true, nil)

	if code != 1 {
		t.Errorf("dispatch(print=true, unseeded) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "not configured")
}

// TestPrintAggregate_PartialSeed verifies the aggregate --print form with a partial module seed.
// It asserts deterministic headers for every registry module, inline YAML for seeded ones, and #
// (not configured) for absent ones, all at exit 0.
func TestPrintAggregate_PartialSeed(t *testing.T) {
	baseDir := t.TempDir()
	const boardYAML = "path: board\nreadme: Home.md\n"
	seedModuleConfig(t, baseDir, "board", boardYAML)
	// fabric is intentionally not seeded.

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, nil, makeNeverCalledEditor(t), nil, true, nil)

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
	// The other nine modules are absent; their sections must each say # (not configured).
	if count := strings.Count(got, "# (not configured)"); count < 2 {
		t.Errorf("expected ≥2 '# (not configured)' lines; got %d; output:\n%s", count, got)
	}
}

// TestPrintUnknownModule verifies that config bogus --print returns an ok:false JSON envelope at
// exit 1 whose error field names the unknown module.
func TestPrintUnknownModule(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, []string{"bogus"}, makeNeverCalledEditor(t), nil, true, nil)

	if code != 1 {
		t.Errorf("dispatch(print=true, unknown) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "unknown config module")
}

// TestConfigLong_ContainsModuleNames verifies that the config command's Long help text includes
// every name from configreg.Names(), proving the help text stays in sync with the registry rather
// than drifting from a hardcoded list.
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

// TestDispatchSet_NeverInvokesEditor verifies that a successful --set invocation never calls the
// injected EditorFunc.
func TestDispatchSet_NeverInvokesEditor(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "fabric", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"fabric"}, countingEditor(&editorCalls), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

	if code != 0 {
		t.Errorf("dispatch(--set) = %d; want 0; output: %q", code, out.String())
	}
	if editorCalls != 0 {
		t.Errorf("dispatch(--set) invoked the editor %d times; want 0", editorCalls)
	}
	assertJSONOkContains(t, out.String(), map[string]any{"module": "fabric"})
}

// TestDispatchSet_UnknownKeyNeverSyncs verifies that an unknown key passed to --set returns an
// error and the injected sync function is never invoked.
func TestDispatchSet_UnknownKeyNeverSyncs(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "fabric", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"fabric"}, countingEditor(&editorCalls), tracker.syncFunc(), false, []string{"bogus_key=x"})

	if code != 1 {
		t.Errorf("dispatch(--set unknown key) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called when --set names an unknown key")
	}
	assertJSONErrContains(t, out.String(), "unknown config key")
}

// TestDispatchSet_OpenMapAddsLabel verifies that --set under a declared open map of a map-shaped board.yaml
// writes the entry under that map and syncs once.
func TestDispatchSet_OpenMapAddsLabel(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "board", "types:\n  bug: a defect\nlabels:\n  old: kept\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"board"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"labels.x=desc"})

	if code != 0 {
		t.Fatalf("dispatch(--set labels.x) = %d; want 0; output: %q", code, out.String())
	}
	if !tracker.called {
		t.Error("sync should run after a successful --set")
	}
	data, err := os.ReadFile(configengine.ConfigFile(baseDir, "board"))
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if !strings.Contains(string(data), "x: desc") || !strings.Contains(string(data), "old: kept") {
		t.Errorf("board.yaml lacks the new and the existing label; got %q", data)
	}
}

// TestDispatchSet_OpenMapRefusesListShape verifies that --set under an open map holding a list refuses,
// writes nothing and never syncs.
func TestDispatchSet_OpenMapRefusesListShape(t *testing.T) {
	baseDir := t.TempDir()
	seeded := "types:\n  bug: a defect\nlabels:\n  - old\n"
	seedModuleConfig(t, baseDir, "board", seeded)

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"board"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"labels.x=desc"})

	if code != 1 {
		t.Errorf("dispatch(--set into list-shaped labels) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called when --set refuses")
	}
	data, err := os.ReadFile(configengine.ConfigFile(baseDir, "board"))
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if string(data) != seeded {
		t.Errorf("board.yaml changed on a refused --set; got %q", data)
	}
}

// TestDispatchSet_UndeclaredNonexistentKeyStillRefuses verifies that an undeclared key on a module with open maps
// still refuses and never syncs.
func TestDispatchSet_UndeclaredNonexistentKeyStillRefuses(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "board", "types:\n  bug: a defect\nlabels:\n  old: kept\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"board"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"bogus_key=x"})

	if code != 1 {
		t.Errorf("dispatch(--set undeclared key) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called for an undeclared key")
	}
	assertJSONErrContains(t, out.String(), "unknown config key")
}

// TestDispatchSet_PrintMutuallyExclusive verifies that passing both --print and --set returns the
// mutual-exclusivity error, with neither the editor nor sync invoked.
func TestDispatchSet_PrintMutuallyExclusive(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"fabric"}, countingEditor(&editorCalls), tracker.syncFunc(), true, []string{"branch_prefix=new-"})

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

// TestDispatchSet_NoModuleRequiresOne verifies that --set with no module positional returns the
// module-required error.
func TestDispatchSet_NoModuleRequiresOne(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, nil, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

	if code != 1 {
		t.Errorf("dispatch(--set, no module) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "module required with --set")
}

// TestDispatchSet_MultipleValuesOneSync verifies that multiple --set values in one dispatch() call
// all land in a single sync invocation.
func TestDispatchSet_MultipleValuesOneSync(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "fabric", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	syncCalls := 0
	sync := func(w io.Writer) int {
		syncCalls++
		return 0
	}
	code := dispatch(l, &out, []string{"fabric"}, makeNeverCalledEditor(t), sync, false, []string{"branch_prefix=new-"})

	if code != 0 {
		t.Errorf("dispatch(--set multiple) = %d; want 0; output: %q", code, out.String())
	}
	if syncCalls != 1 {
		t.Errorf("dispatch(--set multiple) called sync %d times; want 1", syncCalls)
	}
	assertJSONOkContains(t, out.String(), map[string]any{"module": "fabric"})
}

// TestDispatchSet_MalformedValue verifies that a malformed --set value with no '=' returns the
// parseSetFlags error.
func TestDispatchSet_MalformedValue(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"fabric"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"no-equals-sign"})

	if code != 1 {
		t.Errorf("dispatch(--set malformed) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called for a malformed --set value")
	}
	assertJSONErrContains(t, out.String(), "expected key=value")
}

// TestConfigLong_MentionsEditorFallbackAndSet verifies that buildConfigLong's output documents both
// the EDITOR/VISUAL editor fallback and the --set flag.
func TestConfigLong_MentionsEditorFallbackAndSet(t *testing.T) {
	longText := buildConfigLong()
	if !strings.Contains(longText, "EDITOR") || !strings.Contains(longText, "VISUAL") {
		t.Errorf("config Long missing EDITOR/VISUAL fallback documentation; Long = %q", longText)
	}
	if !strings.Contains(longText, "--set") {
		t.Errorf("config Long missing --set documentation; Long = %q", longText)
	}
}

// TestDispatchSet_PreservesUnrecognizedKeyReportsWarning verifies that --set against a module file
// carrying an orphan key (one absent from the current template) preserves that key rather than
// dropping it,
// and reports it via the JSON envelope's "preserved" field.
func TestDispatchSet_PreservesUnrecognizedKeyReportsWarning(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "fabric", "branch_prefix: old-\nlegacy_key: keepme\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"fabric"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

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

// TestDispatchSet_CleanFileNoPreservedField verifies that --set against a module file with no
// orphan keys emits a JSON envelope with no "preserved" field at all, rather than an empty one.
func TestDispatchSet_CleanFileNoPreservedField(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "fabric", "branch_prefix: old-\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"fabric"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})

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
