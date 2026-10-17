// configcli_test.go — unit and integration tests for configcli.
//
// Unit tests (untagged): dispatch/editOne/printModule/printAll with fake editor+sync over temp baseDirs seeded via the paths helpers.
// Integration test (//go:build integration): e2e test with real fabriccli.RunCLI over a real hub built by the hubforge package.
// The git-init-backed reconcile scenario lives in reconcile_integration_test.go per the Test Tier Purity Invariant.

package configcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/websterengine"
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

// TestEditOne pins the single-module edit: a valid edit is synced and reported as a JSON success, an unknown module is refused with the known list before anything runs, an editor failure aborts before the sync, and a failing sync is reported with its own output.
func TestEditOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		module     string
		editor     configengine.EditorFunc
		syncExit   int
		syncOutput string
		wantCode   int
		wantSynced bool
		wantOut    []string
		// wantOKModule is the module the JSON success envelope names; empty skips the check.
		wantOKModule string
		// wantErr are substrings the JSON error envelope's error field must hold.
		wantErr []string
	}{
		{
			name:         "valid edit is synced and reported",
			module:       "loom",
			editor:       fakeEditor("discussion_timeout_min: 1\n", nil),
			wantCode:     0,
			wantSynced:   true,
			wantOut:      []string{"edited and synced"},
			wantOKModule: "loom",
		},
		{
			name:     "unknown module is refused before the sync",
			module:   "unknown",
			editor:   fakeEditor("test\n", nil),
			wantCode: 1,
			wantErr:  []string{"unknown config module", "known:"},
		},
		{
			name:     "editor failure aborts before the sync",
			module:   "loom",
			editor:   fakeEditor("test\n", errors.New("simulated editor exit 1")),
			wantCode: 1,
			wantOut:  []string{"aborted"},
		},
		{
			name:       "failing sync is reported with its output",
			module:     "loom",
			editor:     fakeEditor("discussion_timeout_min: 1\n", nil),
			syncExit:   1,
			syncOutput: "sync error: something went wrong",
			wantCode:   1,
			wantSynced: true,
			wantOut:    []string{"fabric sync failed", "sync error: something went wrong"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			seedModuleConfig(t, baseDir, "loom", "# temp\n")

			var out bytes.Buffer
			synced := false
			sync := func(w io.Writer) int {
				synced = true
				fmt.Fprint(w, tt.syncOutput)
				return tt.syncExit
			}
			code := editOne(worktreeDirs(baseDir), &out, tt.module, tt.editor, sync, nil)

			if code != tt.wantCode {
				t.Errorf("editOne() = %d; want %d; output: %q", code, tt.wantCode, out.String())
			}
			if synced != tt.wantSynced {
				t.Errorf("sync called = %v; want %v", synced, tt.wantSynced)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("editOne output missing %q; got %q", want, out.String())
				}
			}
			if tt.wantOKModule != "" {
				assertJSONOkContains(t, out.String(), map[string]any{"module": tt.wantOKModule})
			}
			for _, want := range tt.wantErr {
				assertJSONErrContains(t, out.String(), want)
			}
		})
	}
}

// TestRunCLIIn_FromNonGitDirectory pins the verbs that answer before any layout resolves: bare `lyx config` prints help naming reconcile, menu and every module; a bad argument, and a verb that needs a repository, each exit 1 with a JSON error envelope naming the way forward.
// The bare row passes an empty argument list, since a nil one would make cobra read the test binary's own flags.
func TestRunCLIIn_FromNonGitDirectory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantCode int
		// wantHelp expects help text naming the verbs and every module instead of an envelope.
		wantHelp bool
		// wantErr are substrings of the envelope's error field; wantErrExact is its whole text.
		wantErr      []string
		wantErrExact string
		// viaProcessCwd runs the row through RunCLI, which reads the process cwd, instead of RunCLIIn.
		viaProcessCwd bool
	}{
		{name: "bare config prints help", args: []string{}, wantCode: 0, wantHelp: true},
		{name: "bare config through the process-cwd seam prints help", args: []string{}, wantCode: 0, wantHelp: true, viaProcessCwd: true},
		{
			name:     "unknown argument names itself and the way forward",
			args:     []string{"bogus"},
			wantCode: 1,
			wantErr:  []string{"unknown subcommand", "bogus", `run "lyx config" to list modules and verbs`},
		},
		{
			name:     "menu rejects an argument before resolving any cwd",
			args:     []string{"menu", "bogus"},
			wantCode: 1,
			wantErr:  []string{"bogus"},
		},
		{
			name:         "reconcile surfaces the bare not-a-repository sentinel",
			args:         []string{"reconcile"},
			wantCode:     1,
			wantErrExact: "not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			var code int
			if tt.viaProcessCwd {
				code = RunCLI(&out, tt.args)
			} else {
				code = RunCLIIn(t.TempDir(), &out, tt.args)
			}

			if code != tt.wantCode {
				t.Fatalf("lyx config %v = %d; want %d; output: %q", tt.args, code, tt.wantCode, out.String())
			}
			got := out.String()
			if tt.wantHelp {
				if strings.HasPrefix(strings.TrimSpace(got), "{") {
					t.Errorf("lyx config printed an envelope; want help text: %q", got)
				}
				for _, want := range append([]string{"reconcile", "menu"}, configreg.Names()...) {
					if !strings.Contains(got, want) {
						t.Errorf("lyx config output does not name %q: %q", want, got)
					}
				}
				return
			}
			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &env); err != nil {
				t.Fatalf("output is not a JSON envelope: %v; got %q", err, got)
			}
			msg, _ := env["error"].(string)
			for _, want := range tt.wantErr {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
			if tt.wantErrExact != "" && msg != tt.wantErrExact {
				t.Errorf("error = %q; want exactly %q", msg, tt.wantErrExact)
			}
		})
	}
}

// TestNoModuleSharesNameWithSubcommand verifies that no configreg module is named like a config subcommand,
// since a subcommand name always routes as a subcommand.
func TestNoModuleSharesNameWithSubcommand(t *testing.T) {
	t.Parallel()
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

// TestConfigLong verifies that the config command's Long help text names every module in configreg.Names(), so it stays in sync with the registry rather than drifting from a hardcoded list, and documents the EDITOR/VISUAL editor fallback and the --set flag.
func TestConfigLong(t *testing.T) {
	t.Parallel()
	longText := Command().Long
	wants := append([]string{"EDITOR", "VISUAL", "code --wait", "nano", "--set", "--layer"}, configreg.Names()...)
	for _, want := range wants {
		if !strings.Contains(longText, want) {
			t.Errorf("config Long missing %q; Long = %q", want, longText)
		}
	}
}

// runMenuWith runs menu over a hub fixture with the given input, terminal answer and editor, and returns the exit code, the output and the sync tracker.
// Each seed names a module, optionally as "<module>:default" or "<module>:local" for that layer's file; a bare name seeds the module's own default file.
func runMenuWith(t *testing.T, input string, terminal bool, editor configengine.EditorFunc, seed ...string) (int, string, *fakeSyncTracker) {
	t.Helper()
	fx := newHubFixture(t)
	dirs := dirsOf(fx.layout)
	for _, entry := range seed {
		name, layer, _ := strings.Cut(entry, ":")
		mod, _ := configreg.Lookup(name)
		path, _, err := dirs.layerFile(mod)
		switch layer {
		case "default":
			path = configengine.ConfigFile(dirs.board, name)
		case "local":
			path = configengine.LocalFile(dirs.worktree, name)
		case "":
			if err != nil {
				t.Fatalf("layerFile(%s): %v", name, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create the dir of %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	var out bytes.Buffer
	tracker := &fakeSyncTracker{exitCode: 0}
	code := menu(dirs, strings.NewReader(input), &out, editor, tracker.syncFunc(), nil, terminal)
	return code, out.String(), tracker
}

// TestMenu pins the interactive menu: a valid selection edits and syncs the module, quit exits 0 without either, a non-number and an out-of-range number each exit 1 with an invalid message and without either, and the listing marks each module with the layers that hold a file, or (template) when none does.
func TestMenu(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		seed  []string
		// editsAllowed lets the editor run; otherwise the editor fails the test if called.
		editsAllowed bool
		// noTerminal runs the menu with no terminal on its input.
		noTerminal bool
		wantCode   int
		wantSynced bool
		wantOut    []string
		// wantNotOut is text the output must not hold.
		wantNotOut []string
	}{
		{name: "no terminal refuses before reading a choice", input: "1\n", noTerminal: true, wantCode: 1, wantOut: []string{"--print", "--set"}, wantNotOut: []string{"1) "}},
		{name: "valid selection", input: "1\nq\n", seed: []string{"batcher"}, editsAllowed: true, wantSynced: true, wantOut: []string{"board"}},
		{name: "quit", input: "q\n"},
		{name: "non-number", input: "abc\n", wantCode: 1, wantOut: []string{"invalid"}},
		{name: "out-of-range number", input: "999\n", wantCode: 1, wantOut: []string{"invalid"}},
		{
			name:    "status marks seeded and unseeded modules",
			input:   "q\n",
			seed:    []string{"board", "loom"},
			wantOut: []string{"board (default)", "loom (worktree)", "fabric (template)", "reed (template)"},
		},
		{
			name:    "status lists every layer that holds a file",
			input:   "q\n",
			seed:    []string{"loom:default", "loom", "loom:local", "darn:local"},
			wantOut: []string{"loom (default, worktree, local)", "darn (local)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			editor := makeNeverCalledEditor(t)
			if tt.editsAllowed {
				editor = fakeEditor("test: value\n", nil)
			}

			code, output, tracker := runMenuWith(t, tt.input, !tt.noTerminal, editor, tt.seed...)

			if code != tt.wantCode {
				t.Errorf("menu(%q) = %d; want %d", tt.input, code, tt.wantCode)
			}
			if tracker.called != tt.wantSynced {
				t.Errorf("menu(%q) sync called = %v; want %v", tt.input, tracker.called, tt.wantSynced)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(output, want) {
					t.Errorf("menu(%q) output missing %q; got %q", tt.input, want, output)
				}
			}
			for _, unwanted := range tt.wantNotOut {
				if strings.Contains(output, unwanted) {
					t.Errorf("menu(%q) output holds %q; got %q", tt.input, unwanted, output)
				}
			}
		})
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

// TestPrint pins the --print form, which never opens an editor: a seeded module's on-disk YAML is emitted verbatim, a known module with no file is an ok:false envelope, an unknown module is an ok:false envelope naming it, and the aggregate form prints a deterministic header for every registry module with inline YAML for seeded ones and "# (not configured)" for absent ones.
func TestPrint(t *testing.T) {
	t.Parallel()
	const loomYAML = "discussion_timeout_min: 60\n"
	aggregateWants := []string{loomYAML[:len(loomYAML)-1]}
	for _, name := range configreg.Names() {
		aggregateWants = append(aggregateWants, "# "+name)
	}

	tests := []struct {
		name string
		// seedLoom is the loom.yaml content; empty seeds nothing but still creates the config dir.
		seedLoom string
		args     []string
		wantCode int
		// wantExact is the whole output; wantContains are substrings of it; wantErr a substring of the error envelope.
		wantExact    string
		wantContains []string
		wantErr      string
		// minNotConfigured is the least number of "# (not configured)" lines expected.
		minNotConfigured int
	}{
		{name: "seeded module is printed verbatim", seedLoom: loomYAML, args: []string{"loom"}, wantExact: loomYAML},
		{name: "known module with no file is not configured", args: []string{"loom"}, wantCode: 1, wantErr: "not configured"},
		{name: "unknown module is refused", args: []string{"bogus"}, wantCode: 1, wantErr: "unknown config module"},
		{name: "aggregate with a partial seed", seedLoom: loomYAML, wantContains: aggregateWants, minNotConfigured: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
				t.Fatalf("failed to create config dir: %v", err)
			}
			if tt.seedLoom != "" {
				seedModuleConfig(t, baseDir, "loom", tt.seedLoom)
			}

			var out bytes.Buffer
			code := dispatch(makeLayoutAt(baseDir), &out, tt.args, makeNeverCalledEditor(t), nil, nil, true, nil, "", nil, true)

			if code != tt.wantCode {
				t.Errorf("dispatch(print=true, %v) = %d; want %d; output: %q", tt.args, code, tt.wantCode, out.String())
			}
			if tt.wantExact != "" && out.String() != tt.wantExact {
				t.Errorf("output = %q; want %q", out.String(), tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q; output:\n%s", want, out.String())
				}
			}
			if count := strings.Count(out.String(), "# (not configured)"); count < tt.minNotConfigured {
				t.Errorf("got %d '# (not configured)' lines; want at least %d; output:\n%s", count, tt.minNotConfigured, out.String())
			}
			if tt.wantErr != "" {
				assertJSONErrContains(t, out.String(), tt.wantErr)
			}
		})
	}
}

// TestDispatchSet pins the --set path on a per-worktree module: it never opens the editor, syncs once for a successful write and never for a refused one, keeps an orphan key of a sparse worktree file and omits the "preserved" field, and refuses an unknown key, a --print/--set pair, a missing module and a value without '='.
func TestDispatchSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// seed is the loom.yaml content; empty seeds nothing.
		seed      string
		args      []string
		printOnly bool
		setFlags  []string
		wantCode  int
		wantSyncs int
		// wantErr is a substring of the error envelope; empty means a success envelope naming loom.
		wantErr string
		// wantPreserved is the "preserved" field; nil means the field must be absent.
		wantPreserved []string
	}{
		{
			name:      "set on a clean file syncs once and reports nothing preserved",
			seed:      "discussion_timeout_min: 480\n",
			args:      []string{"loom"},
			setFlags:  []string{"discussion_timeout_min=60"},
			wantSyncs: 1,
		},
		{
			name:      "orphan key in a sparse worktree file is kept and left to reconcile to report",
			seed:      "discussion_timeout_min: 480\nlegacy_key: keepme\n",
			args:      []string{"loom"},
			setFlags:  []string{"discussion_timeout_min=60"},
			wantSyncs: 1,
		},
		{
			name:     "unknown key never syncs",
			seed:     "discussion_timeout_min: 480\n",
			args:     []string{"loom"},
			setFlags: []string{"bogus_key=x"},
			wantCode: 1,
			wantErr:  "unknown config key",
		},
		{
			name:      "print and set are mutually exclusive",
			args:      []string{"loom"},
			printOnly: true,
			setFlags:  []string{"discussion_timeout_min=60"},
			wantCode:  1,
			wantErr:   "mutually exclusive",
		},
		{
			name:     "set with no module is refused",
			setFlags: []string{"discussion_timeout_min=60"},
			wantCode: 1,
			wantErr:  "module required with --set",
		},
		{
			name:     "value without an equals sign is refused",
			args:     []string{"loom"},
			setFlags: []string{"no-equals-sign"},
			wantCode: 1,
			wantErr:  "expected key=value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if tt.seed != "" {
				seedModuleConfig(t, baseDir, "loom", tt.seed)
			}

			var out bytes.Buffer
			editorCalls, syncCalls := 0, 0
			sync := func(w io.Writer) int {
				syncCalls++
				return 0
			}
			code := dispatch(makeLayoutAt(baseDir), &out, tt.args, countingEditor(&editorCalls), sync, nil, tt.printOnly, tt.setFlags, "", nil, true)

			if code != tt.wantCode {
				t.Fatalf("dispatch(--set) = %d; want %d; output: %q", code, tt.wantCode, out.String())
			}
			if editorCalls != 0 {
				t.Errorf("dispatch(--set) invoked the editor %d times; want 0", editorCalls)
			}
			if syncCalls != tt.wantSyncs {
				t.Errorf("dispatch(--set) called sync %d times; want %d", syncCalls, tt.wantSyncs)
			}
			if tt.wantErr != "" {
				assertJSONErrContains(t, out.String(), tt.wantErr)
				return
			}
			assertJSONOkContains(t, out.String(), map[string]any{"module": "loom"})

			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("output is not valid JSON: %v; got %q", err, out.String())
			}
			preserved, present := env["preserved"]
			if tt.wantPreserved == nil {
				if present {
					t.Errorf("JSON envelope has a \"preserved\" field on a clean write; got %v", env)
				}
				return
			}
			var got []string
			list, _ := preserved.([]any)
			for _, entry := range list {
				got = append(got, fmt.Sprint(entry))
			}
			if !slices.Equal(got, tt.wantPreserved) {
				t.Errorf("preserved = %v; want %v", preserved, tt.wantPreserved)
			}
		})
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
	const listShaped = "types:\n  bug: a defect\nlabels:\n  - old\n"

	rows := []struct {
		name string
		// seed is the board dir file's starting bytes; empty means seeded.
		seed      string
		printOnly bool
		setFlags  []string
		editor    func(fx hubFixture) configengine.EditorFunc
		commitErr error
		// guard is the hub-wide write guard; nil allows the write.
		guard       func() (string, error)
		wantCode    int
		wantCommits int
		// check runs after the common assertions, with the board dir file's bytes and the output.
		check func(t *testing.T, fx hubFixture, hubFile, out string)
	}{
		{
			name:     "set refused by the write guard commits nothing and leaves the file",
			setFlags: []string{"labels.x=desc"},
			guard:    func() (string, error) { return "refused: write from the prime", nil },
			wantCode: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != seeded {
					t.Errorf("hub file changed on a refused write; got %q", hubFile)
				}
				assertJSONErrContains(t, out, "refused: write from the prime")
			},
		},
		{
			name:     "editor edit refused by the write guard opens no editor",
			guard:    func() (string, error) { return "refused: write from the prime", nil },
			wantCode: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != seeded {
					t.Errorf("hub file changed on a refused write; got %q", hubFile)
				}
				assertJSONErrContains(t, out, "refused: write from the prime")
			},
		},
		{
			name:     "write guard that cannot decide fails the write",
			setFlags: []string{"labels.x=desc"},
			guard:    func() (string, error) { return "", errors.New("no prime worktree") },
			wantCode: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				assertJSONErrContains(t, out, "no prime worktree")
			},
		},
		{
			name:      "print never calls the write guard",
			printOnly: true,
			guard:     func() (string, error) { return "refused", nil },
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if out != seeded {
					t.Errorf("print output = %q; want %q", out, seeded)
				}
			},
		},
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
			name:        "open-map entry into a list-shaped map is refused and writes nothing",
			seed:        listShaped,
			setFlags:    []string{"labels.x=desc"},
			wantCode:    1,
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != listShaped {
					t.Errorf("hub file changed on a refused set; got %q", hubFile)
				}
			},
		},
		{
			name:        "undeclared key beside open maps is refused",
			setFlags:    []string{"bogus_key=x"},
			wantCode:    1,
			wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != seeded {
					t.Errorf("hub file changed on a refused set; got %q", hubFile)
				}
				assertJSONErrContains(t, out, "unknown config key")
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
			name: "an empty editor result commits nothing and leaves the hub file",
			editor: func(fx hubFixture) configengine.EditorFunc {
				return fakeEditor(" \n", nil)
			},
			wantCode: 1,
			check: func(t *testing.T, fx hubFixture, hubFile, out string) {
				if hubFile != seeded {
					t.Errorf("hub file changed on an empty editor result; got %q", hubFile)
				}
				if _, err := os.Stat(configengine.StagingFile(fx.worktree, "board")); !os.IsNotExist(err) {
					t.Errorf("staging file remains after an aborted edit; stat err = %v", err)
				}
				assertJSONErrContains(t, out, "aborted")
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

			code := dispatch(fx.layout, &out, []string{"board"}, edit, sync.syncFunc(), commit.commitFunc(), row.printOnly, row.setFlags, "", row.guard, true)

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

// TestDispatchLayers drives --layer through dispatch over a hub fixture: an unknown layer is refused naming the three, the default layer is written and committed from the prime only, a hub-wide module has no worktree layer, a local override is written uncommitted and refuses a key the local layer closes, a worktree write under a board is sparse, and --print reads the selected layer.
func TestDispatchLayers(t *testing.T) {
	t.Parallel()

	loomTemplate, _ := configreg.Template("loom")
	websterTemplate, _ := configreg.Template("webster")
	taskSession, err := agentname.Format("ly", "task-a", "loom")
	if err != nil {
		t.Fatalf("agentname.Format: %v", err)
	}
	const refused = "refused: write from the prime"

	rows := []struct {
		name      string
		module    string
		layer     string
		setFlags  []string
		printOnly bool
		// noTerminal runs the call with no terminal on its input.
		noTerminal bool
		// seedBoard and seedLocal are the module's board file and local file before the call; empty seeds nothing.
		seedBoard string
		seedLocal string
		guard     func(fx hubFixture) func() (string, error)
		wantCode  int
		// wantErr is a substring of the error envelope; empty means a success.
		wantErr     string
		wantCommits int
		wantSyncs   int
		// check runs after the common assertions.
		check func(t *testing.T, fx hubFixture, out string)
	}{
		{
			name: "an unknown layer is refused naming the three and writes nothing", module: "loom", layer: "bogus",
			setFlags: []string{"review_max_bounces=2"}, wantCode: 1, wantErr: "default, worktree or local",
			check: func(t *testing.T, fx hubFixture, out string) {
				for _, path := range []string{configengine.ConfigFile(fx.worktree, "loom"), configengine.ConfigFile(fx.board, "loom"), configengine.LocalFile(fx.worktree, "loom")} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("%s exists after a refused layer; stat err = %v", path, err)
					}
				}
			},
		},
		{
			name: "default layer writes and commits the repo default from the prime", module: "loom", layer: "default",
			setFlags: []string{"review_max_bounces=2"}, seedBoard: loomTemplate(), wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.ConfigFile(fx.board, "loom"))
				if err != nil || !strings.Contains(string(data), "review_max_bounces: 2") {
					t.Errorf("board loom.yaml = %q, %v; want review_max_bounces: 2", data, err)
				}
			},
		},
		{
			name: "default layer is refused from a pair", module: "loom", layer: "default",
			setFlags: []string{"review_max_bounces=2"}, seedBoard: loomTemplate(), wantCode: 1, wantErr: refused,
			guard: func(hubFixture) func() (string, error) { return func() (string, error) { return refused, nil } },
			check: func(t *testing.T, fx hubFixture, out string) {
				data, _ := os.ReadFile(configengine.ConfigFile(fx.board, "loom"))
				if string(data) != loomTemplate() {
					t.Errorf("board loom.yaml changed by a refused write")
				}
			},
		},
		{
			name: "default layer is refused from a slug-named session", module: "loom", layer: "default",
			setFlags: []string{"review_max_bounces=2"}, seedBoard: loomTemplate(), wantCode: 1, wantErr: taskSession,
			guard: func(fx hubFixture) func() (string, error) {
				return func() (string, error) { return hubWriteRefusal(fx.layout, taskSession) }
			},
		},
		{
			name: "worktree layer of a hub-wide module is refused with the ways forward", module: "darn", layer: "worktree",
			setFlags: []string{"writer=opus"}, wantCode: 1, wantErr: "--layer default",
			check: func(t *testing.T, fx hubFixture, out string) {
				assertJSONErrContains(t, out, "--layer local")
			},
		},
		{
			name: "local layer writes an open key uncommitted", module: "darn", layer: "local",
			setFlags: []string{"writer=opus"},
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.LocalFile(fx.worktree, "darn"))
				if err != nil || string(data) != "writer: opus\n" {
					t.Errorf("local darn.yaml = %q, %v; want only the writer key", data, err)
				}
			},
		},
		{
			name: "local layer refuses a closed key naming it and leaves no file", module: "darn", layer: "local",
			setFlags: []string{"verify=make"}, wantCode: 1, wantErr: "verify",
			check: func(t *testing.T, fx hubFixture, out string) {
				if _, err := os.Stat(configengine.LocalFile(fx.worktree, "darn")); !os.IsNotExist(err) {
					t.Errorf("local darn.yaml exists after a refused key; stat err = %v", err)
				}
			},
		},
		{
			name: "set into an absent worktree file writes only that key", module: "loom",
			setFlags: []string{"review_max_bounces=2"}, wantSyncs: 1,
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.ConfigFile(fx.worktree, "loom"))
				if err != nil || string(data) != "review_max_bounces: 2\n" {
					t.Errorf("worktree loom.yaml = %q, %v; want only the key set", data, err)
				}
			},
		},
		{
			name: "local layer refuses a value the loader rejects naming the key and leaves no file", module: "loom", layer: "local",
			setFlags: []string{`plan=["opus[medium]", "sonnet[high]"]`}, seedBoard: loomTemplate(), wantCode: 1, wantErr: `loom config key "plan"`,
			check: func(t *testing.T, fx hubFixture, out string) {
				if _, err := os.Stat(configengine.LocalFile(fx.worktree, "loom")); !os.IsNotExist(err) {
					t.Errorf("local loom.yaml exists after a refused value; stat err = %v", err)
				}
			},
		},
		{
			name: "local layer refusing a value puts the present file back", module: "loom", layer: "local",
			setFlags: []string{"plan=opus[medium] x"}, seedBoard: loomTemplate(), seedLocal: "review_max_bounces: 5\n", wantCode: 1, wantErr: `loom config key "plan"`,
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.LocalFile(fx.worktree, "loom"))
				if err != nil || string(data) != "review_max_bounces: 5\n" {
					t.Errorf("local loom.yaml = %q, %v; want the file as seeded", data, err)
				}
			},
		},
		{
			name: "local layer still writes a per-round list the loader takes", module: "loom", layer: "local",
			setFlags: []string{`plan_reviewer_model=["opus[medium]", "sonnet[high]"]`}, seedBoard: loomTemplate(),
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.LocalFile(fx.worktree, "loom"))
				if err != nil || string(data) != "plan_reviewer_model: [\"opus[medium]\", \"sonnet[high]\"]\n" {
					t.Errorf("local loom.yaml = %q, %v; want plan_reviewer_model as a list", data, err)
				}
			},
		},
		{
			name: "a write onto a stack already failing to load the same way still goes through", module: "loom", layer: "local",
			setFlags: []string{"review_max_bounces=2"}, seedBoard: loomTemplate(), seedLocal: "plan: opus[medium] x\n",
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.LocalFile(fx.worktree, "loom"))
				if err != nil || !strings.Contains(string(data), "review_max_bounces: 2") {
					t.Errorf("local loom.yaml = %q, %v; want review_max_bounces: 2", data, err)
				}
			},
		},
		{
			name: "worktree layer refuses a value the loader rejects and never syncs", module: "loom",
			setFlags: []string{`plan=["opus[medium]", "sonnet[high]"]`}, seedBoard: loomTemplate(), wantCode: 1, wantErr: `loom config key "plan"`,
			check: func(t *testing.T, fx hubFixture, out string) {
				if _, err := os.Stat(configengine.ConfigFile(fx.worktree, "loom")); !os.IsNotExist(err) {
					t.Errorf("worktree loom.yaml exists after a refused value; stat err = %v", err)
				}
			},
		},
		{
			name: "worktree layer refuses a kickstart_pack_per_card that is not a boolean and never syncs", module: "webster",
			setFlags: []string{"kickstart_pack_per_card=maybe"}, seedBoard: websterTemplate(), wantCode: 1, wantErr: "webster config",
			check: func(t *testing.T, fx hubFixture, out string) {
				if _, err := os.Stat(configengine.ConfigFile(fx.worktree, "webster")); !os.IsNotExist(err) {
					t.Errorf("worktree webster.yaml exists after a refused value; stat err = %v", err)
				}
			},
		},
		{
			name: "worktree layer sets kickstart_pack_per_card false and the loader reads it back", module: "webster", layer: "worktree",
			setFlags: []string{"kickstart_pack_per_card=false"}, seedBoard: websterTemplate(), wantSyncs: 1,
			check: func(t *testing.T, fx hubFixture, out string) {
				cfg, err := websterengine.LoadConfig(configengine.Dirs{Anchor: fx.worktree, Board: fx.board}, "webster")
				if err != nil {
					t.Fatalf("LoadConfig() error = %v; want nil", err)
				}
				if cfg.KickstartPackPerCard {
					t.Errorf("KickstartPackPerCard = true; want false after --set kickstart_pack_per_card=false")
				}
			},
		},
		{
			name: "default layer refuses a value the loader rejects and leaves the board file as it was", module: "loom", layer: "default",
			setFlags: []string{`plan=["opus[medium]", "sonnet[high]"]`}, seedBoard: loomTemplate(), wantCode: 1, wantErr: `loom config key "plan"`, wantCommits: 1,
			check: func(t *testing.T, fx hubFixture, out string) {
				data, _ := os.ReadFile(configengine.ConfigFile(fx.board, "loom"))
				if string(data) != loomTemplate() {
					t.Errorf("board loom.yaml changed by a refused value")
				}
			},
		},
		{
			name: "print reads the selected layer's file", module: "loom", layer: "local", printOnly: true,
			seedLocal: "review_max_bounces: 5\n",
			check: func(t *testing.T, fx hubFixture, out string) {
				if out != "review_max_bounces: 5\n" {
					t.Errorf("print output = %q; want the local file", out)
				}
			},
		},
		{
			// The refusal is before layer dispatch, so one row stands for every layer, and the failing guard pins that it also precedes the hub-write guard.
			name: "no terminal refuses the editor and writes nothing", module: "loom", layer: "default", noTerminal: true,
			seedBoard: loomTemplate(), seedLocal: "review_max_bounces: 5\n", wantCode: 1, wantErr: "--print",
			guard: func(hubFixture) func() (string, error) {
				return func() (string, error) {
					t.Error("the hub-write guard ran before the no-terminal refusal")
					return "", nil
				}
			},
			check: func(t *testing.T, fx hubFixture, out string) {
				assertJSONErrContains(t, out, "--set")
				if data, _ := os.ReadFile(configengine.ConfigFile(fx.board, "loom")); string(data) != loomTemplate() {
					t.Errorf("board loom.yaml = %q; want it unchanged", data)
				}
				if data, _ := os.ReadFile(configengine.LocalFile(fx.worktree, "loom")); string(data) != "review_max_bounces: 5\n" {
					t.Errorf("local loom.yaml = %q; want it unchanged", data)
				}
				if _, err := os.Stat(configengine.ConfigFile(fx.worktree, "loom")); !os.IsNotExist(err) {
					t.Errorf("worktree loom.yaml exists after a refused editor path; stat err = %v", err)
				}
			},
		},
		{
			name: "no terminal still prints", module: "loom", layer: "default", noTerminal: true, printOnly: true,
			seedBoard: loomTemplate(),
			check: func(t *testing.T, fx hubFixture, out string) {
				if out != loomTemplate() {
					t.Errorf("print output = %q; want the seeded bytes", out)
				}
			},
		},
		{
			name: "no terminal still sets", module: "loom", noTerminal: true,
			setFlags: []string{"discussion_timeout_min=60"}, wantSyncs: 1,
			check: func(t *testing.T, fx hubFixture, out string) {
				data, err := os.ReadFile(configengine.ConfigFile(fx.worktree, "loom"))
				if err != nil || string(data) != "discussion_timeout_min: 60\n" {
					t.Errorf("worktree loom.yaml = %q, %v; want the value written", data, err)
				}
			},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			fx := newHubFixture(t)
			if row.seedBoard != "" {
				seedModuleConfig(t, fx.board, row.module, row.seedBoard)
			}
			if row.seedLocal != "" {
				if err := os.MkdirAll(filepath.Dir(configengine.LocalFile(fx.worktree, row.module)), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(configengine.LocalFile(fx.worktree, row.module), []byte(row.seedLocal), 0o644); err != nil {
					t.Fatalf("write local file: %v", err)
				}
			}
			var guard func() (string, error)
			if row.guard != nil {
				guard = row.guard(fx)
			}
			commit := &fakeHubCommit{}
			syncs := 0
			sync := func(io.Writer) int { syncs++; return 0 }
			var out bytes.Buffer

			code := dispatch(fx.layout, &out, []string{row.module}, makeNeverCalledEditor(t), sync, commit.commitFunc(), row.printOnly, row.setFlags, row.layer, guard, !row.noTerminal)

			if code != row.wantCode {
				t.Errorf("dispatch = %d; want %d; output: %q", code, row.wantCode, out.String())
			}
			if row.wantErr != "" {
				assertJSONErrContains(t, out.String(), row.wantErr)
			}
			if commit.calls != row.wantCommits {
				t.Errorf("hub commit ran %d times; want %d", commit.calls, row.wantCommits)
			}
			if syncs != row.wantSyncs {
				t.Errorf("fabric sync ran %d times; want %d", syncs, row.wantSyncs)
			}
			if row.check != nil {
				row.check(t, fx, out.String())
			}
		})
	}
}

// TestPrintResolved pins that --resolved prints a module's resolved configuration with each unit annotated by the layer it came from, refuses an unknown module and an absent base layer with an error envelope, and is refused beside --print, --set, --layer or without a module.
func TestPrintResolved(t *testing.T) {
	t.Parallel()
	loomTemplate, _ := configreg.Template("loom")

	t.Run("annotates each unit with its layer", func(t *testing.T) {
		t.Parallel()
		fx := newHubFixture(t)
		seedModuleConfig(t, fx.board, "loom", loomTemplate())
		seedModuleConfig(t, fx.worktree, "loom", "review_max_bounces: 2\n")
		localFile := configengine.LocalFile(fx.worktree, "loom")
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatalf("mkdir local dir: %v", err)
		}
		if err := os.WriteFile(localFile, []byte("discussion_timeout_min: 7\n"), 0o644); err != nil {
			t.Fatalf("write local file: %v", err)
		}

		var out bytes.Buffer
		if code := printResolved(configDirs{worktree: fx.worktree, board: fx.board}, &out, "loom"); code != 0 {
			t.Fatalf("printResolved = %d; output: %s", code, out.String())
		}
		for _, want := range []string{"review_max_bounces: 2 # worktree", "discussion_timeout_min: 7 # local", "plan_timeout_min:"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("output missing %q; output:\n%s", want, out.String())
			}
		}
		if !strings.Contains(out.String(), "# default") {
			t.Errorf("output holds no default-layer unit; output:\n%s", out.String())
		}
	})

	failures := []struct {
		name    string
		module  string
		wantErr string
	}{
		{name: "unknown module", module: "bogus", wantErr: "unknown config module"},
		{name: "absent base layer", module: "loom", wantErr: "not found"},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newHubFixture(t)
			var out bytes.Buffer
			if code := printResolved(configDirs{worktree: fx.worktree, board: fx.board}, &out, tt.module); code != 1 {
				t.Fatalf("printResolved = %d; want 1; output: %s", code, out.String())
			}
			assertJSONErrContains(t, out.String(), tt.wantErr)
		})
	}

	refusals := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "beside --set", args: []string{"loom", "--resolved", "--set", "review_max_bounces=2"}, wantErr: "--resolved and --set are mutually exclusive"},
		{name: "beside --print", args: []string{"loom", "--resolved", "--print"}, wantErr: "--resolved and --print are mutually exclusive"},
		{name: "beside --layer", args: []string{"loom", "--resolved", "--layer", "local"}, wantErr: "--resolved and --layer are mutually exclusive"},
		{name: "without a module", args: []string{"--resolved"}, wantErr: "module required with --resolved"},
	}
	for _, tt := range refusals {
		t.Run("refused "+tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := RunCLI(&out, tt.args); code != 1 {
				t.Fatalf("RunCLI(%v) = %d; want 1; output: %s", tt.args, code, out.String())
			}
			assertJSONErrContains(t, out.String(), tt.wantErr)
		})
	}
}
