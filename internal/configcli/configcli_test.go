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
	"github.com/Knatte18/loomyard/internal/fabricengine"
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

// fakeHubCommit is a fake hubCommitFunc that runs the write closure, records the call
// and returns the write error, or err when the write succeeded.
type fakeHubCommit struct {
	calls int
	err   error
}

func (f *fakeHubCommit) commitFunc() hubCommitFunc {
	return func(module string, write func() error) error {
		f.calls++
		if err := write(); err != nil {
			return err
		}
		return f.err
	}
}

// worktreeDirs returns config dirs for per-worktree modules only: the board dir is left empty.
func worktreeDirs(baseDir string) configDirs {
	return configDirs{worktree: baseDir}
}

// hubFixture is a layout whose worktree and board dir are separate directories under one hub.
type hubFixture struct {
	layout   *lyxcwd.Location
	worktree string
	board    string
}

// newHubFixture returns a hub fixture with an initialized (empty) config dir at the worktree and at the board dir.
func newHubFixture(t *testing.T) hubFixture {
	t.Helper()
	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: "wt", AnchorRel: "."}
	fx := hubFixture{layout: layout, worktree: baseDirOf(layout), board: fabricengine.BoardDir(hub)}
	for _, dir := range []string{fx.worktree, fx.board} {
		if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}
	}
	return fx
}

// TestEditOneSuccess tests the success path: valid YAML, sync succeeds (exit 0).
func TestEditOneSuccess(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/loom.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "loom"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write loom.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(worktreeDirs(baseDir), &out, "loom", fakeEditor("discussion_timeout_min: 1\n", nil), tracker.syncFunc(), nil)

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
	assertJSONOkContains(t, output, map[string]any{"module": "loom"})
}

// TestEditOneUnknownModule tests unknown module handling.
func TestEditOneUnknownModule(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/config directory
	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a fake _lyx/config/loom.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "loom"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write loom.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(worktreeDirs(baseDir), &out, "unknown", fakeEditor("test\n", nil), tracker.syncFunc(), nil)

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

	// Create a fake _lyx/config/loom.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "loom"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write loom.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := editOne(worktreeDirs(baseDir), &out, "loom", fakeEditor("test\n", errors.New("simulated editor exit 1")), tracker.syncFunc(), nil)

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

	// Create a fake _lyx/config/loom.yaml to satisfy FindBaseDir
	if err := os.WriteFile(configengine.ConfigFile(baseDir, "loom"), []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("failed to write loom.yaml: %v", err)
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 1}
	syncWithOutput := func(w io.Writer) int {
		tracker.called = true
		fmt.Fprint(w, "sync error: something went wrong")
		return 1
	}
	code := editOne(worktreeDirs(baseDir), &out, "loom", fakeEditor("discussion_timeout_min: 1\n", nil), syncWithOutput, nil)

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
// prints help rather than an envelope, and names reconcile, menu and every module.
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
	for _, verb := range []string{"reconcile", "menu"} {
		if !strings.Contains(got, verb) {
			t.Errorf("lyx config output does not name %s: %q", verb, got)
		}
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

// runMenuWith runs menu over a hub fixture with the given input,
// seeding each named module at the dir the registry says holds it,
// and returning the exit code, the output and the sync tracker.
func runMenuWith(t *testing.T, input string, seed ...string) (int, string, *fakeSyncTracker) {
	t.Helper()
	fx := newHubFixture(t)
	dirs := dirsOf(fx.layout)
	for _, name := range seed {
		mod, _ := configreg.Lookup(name)
		if err := os.WriteFile(configengine.ConfigFile(dirs.baseFor(mod), name), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatalf("failed to write %s.yaml: %v", name, err)
		}
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := menu(dirs, strings.NewReader(input), &out, fakeEditor("test: value\n", nil), tracker.syncFunc(), nil)
	return code, out.String(), tracker
}

// TestMenuSelection tests menu with a valid selection.
func TestMenuSelection(t *testing.T) {
	code, output, tracker := runMenuWith(t, "1\nq\n", "batcher")

	if code != 0 {
		t.Errorf("menu() = %d; want 0", code)
	}
	if !tracker.called {
		t.Error("sync should be called for selected module")
	}
	if !strings.Contains(output, "board") {
		t.Errorf("menu output missing board option; got %q", output)
	}
}

// TestMenuQuit tests menu with 'q' selection.
func TestMenuQuit(t *testing.T) {
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := menu(worktreeDirs(t.TempDir()), strings.NewReader("q\n"), &out, makeNeverCalledEditor(t), tracker.syncFunc(), nil)

	if code != 0 {
		t.Errorf("menu() = %d; want 0", code)
	}
	if tracker.called {
		t.Error("sync should not be called on quit")
	}
}

// TestMenuInvalidSelection tests that a non-number and an out-of-range number each exit 1
// without calling the editor or sync.
func TestMenuInvalidSelection(t *testing.T) {
	for _, input := range []string{"abc\n", "999\n"} {
		var out bytes.Buffer
		tracker := &fakeSyncTracker{exitCode: 0}
		code := menu(worktreeDirs(t.TempDir()), strings.NewReader(input), &out, makeNeverCalledEditor(t), tracker.syncFunc(), nil)

		if code != 1 {
			t.Errorf("menu(%q) = %d; want 1", input, code)
		}
		if tracker.called {
			t.Errorf("sync should not be called on input %q", input)
		}
		if !strings.Contains(out.String(), "invalid") {
			t.Errorf("menu(%q) output missing an invalid message; got %q", input, out.String())
		}
	}
}

// TestMenuStatus tests that menu marks seeded modules (configured) and unseeded ones (default),
// looking for a hub-wide module at the board dir and for any other at the worktree.
func TestMenuStatus(t *testing.T) {
	_, output, _ := runMenuWith(t, "q\n", "board", "loom")

	for _, want := range []string{"board (configured)", "loom (configured)", "fabric (default)", "reed (default)"} {
		if !strings.Contains(output, want) {
			t.Errorf("menu output missing %q; got %q", want, output)
		}
	}
}

// TestConfigMenuRejectsArgument verifies that `lyx config menu bogus` from a non-git directory
// exits 1 with a JSON error envelope naming the argument, before the handler resolves any cwd.
func TestConfigMenuRejectsArgument(t *testing.T) {
	var out bytes.Buffer
	code := RunCLIIn(t.TempDir(), &out, []string{"menu", "bogus"})

	if code != 1 {
		t.Fatalf("lyx config menu bogus = %d; want 1; output: %q", code, out.String())
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("output is not a JSON envelope: %v; got %q", err, out.String())
	}
	if msg, _ := env["error"].(string); !strings.Contains(msg, "bogus") {
		t.Errorf("error %q does not name bogus", msg)
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
	const loomYAML = "discussion_timeout_min: 60\n"
	seedModuleConfig(t, baseDir, "loom", loomYAML)

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, []string{"loom"}, makeNeverCalledEditor(t), nil, nil, true, nil)

	if code != 0 {
		t.Errorf("dispatch(print=true, seeded) = %d; want 0; output: %q", code, out.String())
	}
	if got := out.String(); got != loomYAML {
		t.Errorf("dispatch(print=true, seeded) output = %q; want %q", got, loomYAML)
	}
}

// TestPrintModule_KnownButUnseeded verifies that config <module> --print for a known module with no
// on-disk file returns an ok:false JSON envelope at exit 1.
func TestPrintModule_KnownButUnseeded(t *testing.T) {
	baseDir := t.TempDir()
	// Create the config directory but not the loom.yaml file.
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, []string{"loom"}, makeNeverCalledEditor(t), nil, nil, true, nil)

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
	const loomYAML = "discussion_timeout_min: 60\n"
	seedModuleConfig(t, baseDir, "loom", loomYAML)
	// reed is intentionally not seeded.

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	code := dispatch(l, &out, nil, makeNeverCalledEditor(t), nil, nil, true, nil)

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
	// loom is seeded; its YAML content must appear.
	if !strings.Contains(got, "discussion_timeout_min: 60") {
		t.Errorf("aggregate output missing seeded loom YAML; output:\n%s", got)
	}
	// The other modules are absent; their sections must each say # (not configured).
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
	code := dispatch(l, &out, []string{"bogus"}, makeNeverCalledEditor(t), nil, nil, true, nil)

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
	seedModuleConfig(t, baseDir, "loom", "discussion_timeout_min: 480\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"loom"}, countingEditor(&editorCalls), tracker.syncFunc(), nil, false, []string{"discussion_timeout_min=60"})

	if code != 0 {
		t.Errorf("dispatch(--set) = %d; want 0; output: %q", code, out.String())
	}
	if editorCalls != 0 {
		t.Errorf("dispatch(--set) invoked the editor %d times; want 0", editorCalls)
	}
	assertJSONOkContains(t, out.String(), map[string]any{"module": "loom"})
}

// TestDispatchSet_UnknownKeyNeverSyncs verifies that an unknown key passed to --set returns an
// error and the injected sync function is never invoked.
func TestDispatchSet_UnknownKeyNeverSyncs(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "loom", "discussion_timeout_min: 480\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	editorCalls := 0
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"loom"}, countingEditor(&editorCalls), tracker.syncFunc(), nil, false, []string{"bogus_key=x"})

	if code != 1 {
		t.Errorf("dispatch(--set unknown key) = %d; want 1", code)
	}
	if tracker.called {
		t.Error("sync should not be called when --set names an unknown key")
	}
	assertJSONErrContains(t, out.String(), "unknown config key")
}

// TestDispatchSet_OpenMapAddsLabel verifies that --set under a declared open map of a map-shaped board.yaml
// writes the entry under that map at the board dir and commits once.
func TestDispatchSet_OpenMapAddsLabel(t *testing.T) {
	fx := newHubFixture(t)
	seedModuleConfig(t, fx.board, "board", "types:\n  bug: a defect\nlabels:\n  old: kept\n")

	var out bytes.Buffer
	commit := &fakeHubCommit{}
	code := dispatch(fx.layout, &out, []string{"board"}, makeNeverCalledEditor(t), nil, commit.commitFunc(), false, []string{"labels.x=desc"})

	if code != 0 {
		t.Fatalf("dispatch(--set labels.x) = %d; want 0; output: %q", code, out.String())
	}
	if commit.calls != 1 {
		t.Errorf("hub commit ran %d times after a successful --set; want 1", commit.calls)
	}
	data, err := os.ReadFile(configengine.ConfigFile(fx.board, "board"))
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if !strings.Contains(string(data), "x: desc") || !strings.Contains(string(data), "old: kept") {
		t.Errorf("board.yaml lacks the new and the existing label; got %q", data)
	}
}

// TestDispatchSet_OpenMapRefusesListShape verifies that --set under an open map holding a list refuses,
// writes nothing to the board dir file.
func TestDispatchSet_OpenMapRefusesListShape(t *testing.T) {
	fx := newHubFixture(t)
	seeded := "types:\n  bug: a defect\nlabels:\n  - old\n"
	seedModuleConfig(t, fx.board, "board", seeded)

	var out bytes.Buffer
	commit := &fakeHubCommit{}
	code := dispatch(fx.layout, &out, []string{"board"}, makeNeverCalledEditor(t), nil, commit.commitFunc(), false, []string{"labels.x=desc"})

	if code != 1 {
		t.Errorf("dispatch(--set into list-shaped labels) = %d; want 1", code)
	}
	data, err := os.ReadFile(configengine.ConfigFile(fx.board, "board"))
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if string(data) != seeded {
		t.Errorf("board.yaml changed on a refused --set; got %q", data)
	}
}

// TestDispatchSet_UndeclaredNonexistentKeyStillRefuses verifies that an undeclared key on a module with open maps
// still refuses.
func TestDispatchSet_UndeclaredNonexistentKeyStillRefuses(t *testing.T) {
	fx := newHubFixture(t)
	seedModuleConfig(t, fx.board, "board", "types:\n  bug: a defect\nlabels:\n  old: kept\n")

	var out bytes.Buffer
	commit := &fakeHubCommit{}
	code := dispatch(fx.layout, &out, []string{"board"}, makeNeverCalledEditor(t), nil, commit.commitFunc(), false, []string{"bogus_key=x"})

	if code != 1 {
		t.Errorf("dispatch(--set undeclared key) = %d; want 1", code)
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
	code := dispatch(l, &out, []string{"loom"}, countingEditor(&editorCalls), tracker.syncFunc(), nil, true, []string{"discussion_timeout_min=60"})

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
	code := dispatch(l, &out, nil, makeNeverCalledEditor(t), tracker.syncFunc(), nil, false, []string{"discussion_timeout_min=60"})

	if code != 1 {
		t.Errorf("dispatch(--set, no module) = %d; want 1", code)
	}
	assertJSONErrContains(t, out.String(), "module required with --set")
}

// TestDispatchSet_MultipleValuesOneSync verifies that multiple --set values in one dispatch() call
// all land in a single sync invocation.
func TestDispatchSet_MultipleValuesOneSync(t *testing.T) {
	baseDir := t.TempDir()
	seedModuleConfig(t, baseDir, "loom", "discussion_timeout_min: 480\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	syncCalls := 0
	sync := func(w io.Writer) int {
		syncCalls++
		return 0
	}
	code := dispatch(l, &out, []string{"loom"}, makeNeverCalledEditor(t), sync, nil, false, []string{"discussion_timeout_min=60"})

	if code != 0 {
		t.Errorf("dispatch(--set multiple) = %d; want 0; output: %q", code, out.String())
	}
	if syncCalls != 1 {
		t.Errorf("dispatch(--set multiple) called sync %d times; want 1", syncCalls)
	}
	assertJSONOkContains(t, out.String(), map[string]any{"module": "loom"})
}

// TestDispatchSet_MalformedValue verifies that a malformed --set value with no '=' returns the
// parseSetFlags error.
func TestDispatchSet_MalformedValue(t *testing.T) {
	baseDir := t.TempDir()
	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"loom"}, makeNeverCalledEditor(t), tracker.syncFunc(), nil, false, []string{"no-equals-sign"})

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
	if !strings.Contains(longText, "code --wait") || !strings.Contains(longText, "nano") {
		t.Errorf("config Long missing code --wait/nano fallback documentation; Long = %q", longText)
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
	seedModuleConfig(t, baseDir, "loom", "discussion_timeout_min: 480\nlegacy_key: keepme\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"loom"}, makeNeverCalledEditor(t), tracker.syncFunc(), nil, false, []string{"discussion_timeout_min=60"})

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
	seedModuleConfig(t, baseDir, "loom", "discussion_timeout_min: 480\n")

	l := makeLayoutAt(baseDir)
	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := dispatch(l, &out, []string{"loom"}, makeNeverCalledEditor(t), tracker.syncFunc(), nil, false, []string{"discussion_timeout_min=60"})

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

// TestDispatchHubWideBoard drives the hub-wide module board through dispatch with the worktree
// and the board dir as separate directories: every row reads or writes the board dir file,
// never a worktree copy, and commits through the hub-commit seam, never through sync.
func TestDispatchHubWideBoard(t *testing.T) {
	t.Parallel()

	const seeded = "readme: Home.md\ndesign_prefix: d-\ntypes:\n  bug: a defect\nlabels:\n  old: kept\n"
	const edited = "readme: Edited.md\ndesign_prefix: d-\ntypes:\n  bug: a defect\nlabels:\n  old: kept\n"
	const rival = "readme: Rival.md\ndesign_prefix: d-\ntypes:\n  bug: a defect\nlabels:\n  old: kept\n"
	const invalid = "readme: [a]\ndesign_prefix: d-\ntypes:\n  bug: a defect\nlabels:\n  old: kept\n"
	const unresolved = "readme: ${env:LYX_CONFIGCLI_TEST_UNSET}\ndesign_prefix: d-\ntypes:\n  bug: a defect\nlabels:\n  old: kept\n"

	rows := []struct {
		name string
		// seed is the board dir file's starting bytes; empty means seeded.
		seed        string
		printOnly   bool
		setFlags    []string
		editor      func(fx hubFixture) configengine.EditorFunc
		commitErr   error
		wantCode    int
		wantCommits int
		// check runs after the common assertions, with the board dir file's bytes and the output.
		check func(t *testing.T, fx hubFixture, hubFile, out string)
	}{
		{
			name:        "set writes the board dir file and commits it",
			setFlags:    []string{"labels.x=desc"},
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if !strings.Contains(hubFile, "x: desc") || !strings.Contains(hubFile, "old: kept") {
					t.Errorf("hub file lacks the new and the existing label; got %q", hubFile)
				}
				assertJSONOkContains(t, out, map[string]any{"module": "board", "message": "edited and committed " + configengine.ConfigFile(fx.board, "board") + " in _board"})
			},
		},
		{
			name:      "print reads the board dir file",
			printOnly: true,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if out != seeded {
					t.Errorf("print output = %q; want %q", out, seeded)
				}
			},
		},
		{
			// The file already holds an env marker that cannot resolve, which the set leaves alone;
			// the strict load refuses the result of the set, and the file is put back.
			name:        "set the strict load rejects restores the file",
			seed:        unresolved,
			setFlags:    []string{"labels.x=desc"},
			wantCode:    1,
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != unresolved {
					t.Errorf("hub file changed on a refused set; got %q", hubFile)
				}
				assertJSONErrContains(t, out, "unchanged")
			},
		},
		{
			name:        "commit error leaves the edit on disk and says so",
			setFlags:    []string{"labels.x=desc"},
			commitErr:   errors.New("push refused"),
			wantCode:    1,
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if !strings.Contains(hubFile, "x: desc") {
					t.Errorf("hub file lost the edit after a commit error; got %q", hubFile)
				}
				assertJSONErrContains(t, out, configengine.ConfigFile(fx.board, "board"))
				assertJSONErrContains(t, out, "uncommitted")
			},
		},
		{
			name: "editor edit writes the staged bytes to the board dir file",
			editor: func(fx hubFixture) configengine.EditorFunc {
				return fakeEditor(edited, nil)
			},
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != edited {
					t.Errorf("hub file = %q; want the staged bytes %q", hubFile, edited)
				}
				if _, err := os.Stat(configengine.StagingFile(fx.worktree, "board")); !os.IsNotExist(err) {
					t.Errorf("staging file remains after a committed edit; stat err = %v", err)
				}
			},
		},
		{
			name: "editor edit the strict load rejects leaves the file unchanged and no staging copy",
			editor: func(fx hubFixture) configengine.EditorFunc {
				return fakeEditor(invalid, nil)
			},
			wantCode:    1,
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != seeded {
					t.Errorf("hub file changed on a rejected edit; got %q", hubFile)
				}
				if _, err := os.Stat(configengine.StagingFile(fx.worktree, "board")); !os.IsNotExist(err) {
					t.Errorf("staging file remains after a rejected edit; stat err = %v", err)
				}
				assertJSONErrContains(t, out, "unchanged")
			},
		},
		{
			name: "hub file changed while the editor was open is refused and the staging copy kept",
			editor: func(fx hubFixture) configengine.EditorFunc {
				return func(path string) error {
					if err := os.WriteFile(configengine.ConfigFile(fx.board, "board"), []byte(rival), 0o644); err != nil {
						return err
					}
					return os.WriteFile(path, []byte(edited), 0o644)
				}
			},
			wantCode:    1,
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != rival {
					t.Errorf("hub file = %q; want the other writer's bytes %q", hubFile, rival)
				}
				staged, err := os.ReadFile(configengine.StagingFile(fx.worktree, "board"))
				if err != nil || string(staged) != edited {
					t.Errorf("staging file = %q, %v; want the kept edit %q", staged, err, edited)
				}
				assertJSONErrContains(t, out, "changed while the editor was open")
				assertJSONErrContains(t, out, configengine.StagingFile(fx.worktree, "board"))
			},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			fx := newHubFixture(t)
			seed := seeded
			if row.seed != "" {
				seed = row.seed
			}
			seedModuleConfig(t, fx.board, "board", seed)
			edit := makeNeverCalledEditor(t)
			if row.editor != nil {
				edit = row.editor(fx)
			}
			commit := &fakeHubCommit{err: row.commitErr}
			sync := &fakeSyncTracker{}
			var out bytes.Buffer

			code := dispatch(fx.layout, &out, []string{"board"}, edit, sync.syncFunc(), commit.commitFunc(), row.printOnly, row.setFlags)

			if code != row.wantCode {
				t.Errorf("dispatch = %d; want %d; output: %q", code, row.wantCode, out.String())
			}
			if commit.calls != row.wantCommits {
				t.Errorf("hub commit ran %d times; want %d", commit.calls, row.wantCommits)
			}
			if sync.called {
				t.Error("fabric sync ran for a hub-wide module")
			}
			if _, err := os.Stat(configengine.ConfigFile(fx.worktree, "board")); !os.IsNotExist(err) {
				t.Errorf("a worktree board.yaml exists; stat err = %v", err)
			}
			hubFile, err := os.ReadFile(configengine.ConfigFile(fx.board, "board"))
			if err != nil {
				t.Fatalf("read hub board.yaml: %v", err)
			}
			row.check(t, fx, string(hubFile), out.String())
		})
	}
}
