// config_test.go — unit tests for the strict and degrading config loaders.
//
// Tests cover: the strict Load contract using yamlengine + envsource, missing-key detection,
// absent-file errors, env variable resolution via templates, nested-key handling, the
// not-initialized error path, the ConfigDir/ConfigFile/ LyxDirName path constructors configengine
// singly declares, and the degrading LoadOrTemplate contract -- its two proven-absence fallback
// triggers, the strict-when-present boundary it shares with Load, and the absence-only
// discrimination that keeps a stat failure from being mistaken for absence.

package configengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// baseState is how much of a base directory's _lyx tree newBase creates.
type baseState int

const (
	// baseAbsent returns a base directory that does not exist on disk.
	baseAbsent baseState = iota
	// baseNone creates the base directory and nothing in it: no _lyx/.
	baseNone
	// baseLyx creates _lyx/ alone.
	baseLyx
	// baseConfigDir creates _lyx/config/ with no module file.
	baseConfigDir
	// baseWithFile creates _lyx/config/ and writes content as the module's config file.
	baseWithFile
	// baseUnreadable creates _lyx/config/ with a directory where the module's config file belongs, so reading it fails without the file being absent.
	baseUnreadable
)

// newBase builds a fresh temp base directory in the given state and returns it with the module's config file path.
func newBase(t *testing.T, state baseState, module, content string) (baseDir, path string) {
	t.Helper()
	baseDir = t.TempDir()
	switch state {
	case baseAbsent:
		baseDir = filepath.Join(baseDir, "does-not-exist")
	case baseLyx:
		if err := os.Mkdir(filepath.Join(baseDir, lyxdirs.LyxDirName), 0755); err != nil {
			t.Fatalf("failed to create _lyx: %v", err)
		}
	case baseConfigDir, baseWithFile, baseUnreadable:
		if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0755); err != nil {
			t.Fatalf("failed to create _lyx/config: %v", err)
		}
		if state == baseUnreadable {
			if err := os.Mkdir(configengine.ConfigFile(baseDir, module), 0755); err != nil {
				t.Fatalf("failed to create %s.yaml as a directory: %v", module, err)
			}
		}
		if state == baseWithFile {
			if err := os.WriteFile(configengine.ConfigFile(baseDir, module), []byte(content), 0644); err != nil {
				t.Fatalf("failed to write %s.yaml: %v", module, err)
			}
		}
	}
	return baseDir, configengine.ConfigFile(baseDir, module)
}

// assertFileUnchanged fails when the file at path no longer holds want.
func assertFileUnchanged(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to re-read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("config file was rewritten: got %q, want %q", got, want)
	}
}

// assertOneFillLine fails unless the captured log holds exactly one fill line naming the module and the key-path.
func assertOneFillLine(t *testing.T, buf *logcapture.Buffer, module, keyPath string) {
	t.Helper()
	log := buf.String()
	if n := strings.Count(log, "filled missing keys from template"); n != 1 {
		t.Fatalf("expected exactly one fill line, got %d in log: %s", n, log)
	}
	if !strings.Contains(log, "module="+module) {
		t.Errorf("fill line does not name module %q: %s", module, log)
	}
	if !strings.Contains(log, "keys="+keyPath) {
		t.Errorf("fill line does not name key-path %q: %s", keyPath, log)
	}
}

// openMapTemplate holds a mapping at labels that a module may declare open.
const openMapTemplate = "name: x\nlabels:\n  bug: a bug\n  feature: a feature\n"

// loadCase is one config file against a template, loaded by a strict or a degrading loader.
type loadCase struct {
	name     string
	template string
	content  string
	openMaps []string
	// env is set for the row with t.Setenv.
	env map[string]string
	// wantFill is the one key-path the fill line names; empty means no fill line.
	wantFill    string
	contains    []string
	notContains []string
}

// checkLoaded asserts a loaded config against the row: its bytes, the single fill line or none, and the file left untouched.
func checkLoaded(t *testing.T, tc loadCase, resolved []byte, buf *logcapture.Buffer, path string) {
	t.Helper()
	for _, want := range tc.contains {
		if !strings.Contains(string(resolved), want) {
			t.Errorf("resolved = %q; want it to contain %q", resolved, want)
		}
	}
	for _, unwanted := range tc.notContains {
		if strings.Contains(string(resolved), unwanted) {
			t.Errorf("resolved = %q; want it not to contain %q", resolved, unwanted)
		}
	}
	if tc.wantFill == "" {
		if strings.Contains(buf.String(), "filled missing keys") {
			t.Errorf("unexpected fill line: %s", buf.String())
		}
	} else {
		assertOneFillLine(t, buf, "board", tc.wantFill)
	}
	assertFileUnchanged(t, path, tc.content)
}

// TestLoad pins what the strict loader returns for a present file: file values beat template defaults, a missing key loads at its template default with one fill line and the file untouched, a present empty value or emptied list is kept, an extra key is tolerated, env markers resolve, and a declared open map is carried whole.
// It serializes its rows because they swap the process-global logger output and set env variables.
func TestLoad(t *testing.T) {
	tests := []loadCase{
		{
			name:     "file values round-trip",
			template: "path: _board\nhome: Home.md\n",
			content:  "path: custom_path\nhome: Index.md\n",
			contains: []string{"path: custom_path", "home: Index.md"},
		},
		{
			name:     "missing key loads at its template default",
			template: "path: _board\nhome: Home.md\n",
			content:  "path: custom_path\n",
			wantFill: "home",
			contains: []string{"path: custom_path", "home: Home.md"},
		},
		{
			name:     "complete file logs no fill",
			template: "path: _board\nhome: Home.md\n",
			content:  "path: a\nhome: b\n",
			contains: []string{"path: a", "home: b"},
		},
		{
			name:     "extra file key survives beside a filled one and empty values are kept",
			template: "name: tpl\nlabel: tpl\nrequire_pr_to_base:\n  - main\nadded: yes\n",
			content:  "extra_key: extra\nname: \"\"\nlabel: x\nrequire_pr_to_base: []\n",
			wantFill: "added",
			contains: []string{"extra_key: extra", `name: ""`, "label: x", "require_pr_to_base: []", "added: yes"},
		},
		{
			name:     "extra key is tolerated",
			template: "path: _board\n",
			content:  "path: custom_path\nextra_key: extra_value\n",
			contains: []string{"path: custom_path", "extra_key: extra_value"},
		},
		{
			name:     "nested keys round-trip",
			template: "server:\n  host: localhost\n  port: '8080'\n",
			content:  "server:\n  host: example.com\n  port: '9090'\n",
			contains: []string{"host: example.com", "port: '9090'"},
		},
		{
			name:     "required env marker resolves",
			template: "path: ${env:TEST_CONFIG_VAR}\n",
			content:  "path: ${env:TEST_CONFIG_VAR}\n",
			env:      map[string]string{"TEST_CONFIG_VAR": "resolved_value"},
			contains: []string{"path: resolved_value"},
		},
		{
			name:     "optional env marker falls back to its default",
			template: "path: ${env:TEST_OPTIONAL_VAR:-default_path}\n",
			content:  "path: ${env:TEST_OPTIONAL_VAR:-default_path}\n",
			contains: []string{"path: default_path"},
		},
		{
			name:        "declared open map carries the file's keys and none of the template's",
			template:    openMapTemplate,
			content:     "name: x\nlabels:\n  docs: documentation\n",
			openMaps:    []string{"labels"},
			contains:    []string{"docs: documentation"},
			notContains: []string{"bug:"},
		},
		{
			name:     "declared open map accepts a list",
			template: openMapTemplate,
			content:  "name: x\nlabels:\n  - bug\n  - docs\n",
			openMaps: []string{"labels"},
			contains: []string{"- bug", "- docs"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			baseDir, path := newBase(t, baseWithFile, "board", tc.content)

			resolved, err := configengine.Load(baseDir, "board", []byte(tc.template), tc.openMaps...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			checkLoaded(t, tc, resolved, buf, path)
		})
	}
}

// TestLoad_Refusals pins the strict loader's refusals and the text that names the way forward:
// an absent file points at "lyx config reconcile", a refusal reconcile cannot fix never hints at it.
func TestLoad_Refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		state      baseState
		template   string
		content    string
		openMaps   []string
		wantErr    []string
		wantNotErr []string
		// wantInvalid is whether the error is marked ErrInvalid, under Load and, for a present path, under LoadOrTemplate.
		wantInvalid bool
	}{
		{
			name:     "uninitialized base directory",
			state:    baseNone,
			template: "path: _board\n",
			wantErr:  []string{"not initialized"},
		},
		{
			name:     "absent config file",
			state:    baseConfigDir,
			template: "path: _board\n",
			wantErr:  []string{"not found", "lyx config reconcile"},
		},
		{
			name:     "config file that cannot be read",
			state:    baseUnreadable,
			template: "path: _board\n",
			wantErr:  []string{"read config file"},
		},
		{
			name:        "null where the template holds a mapping",
			state:       baseWithFile,
			template:    "server:\n  host: localhost\n",
			content:     "server:\n",
			wantErr:     []string{"server"},
			wantNotErr:  []string{"lyx config reconcile"},
			wantInvalid: true,
		},
		{
			name:        "mapping where the template holds a scalar",
			state:       baseWithFile,
			template:    "server: localhost\n",
			content:     "server:\n  host: x\n",
			wantErr:     []string{"server"},
			wantNotErr:  []string{"lyx config reconcile"},
			wantInvalid: true,
		},
		{
			name:        "key missing inside a present list element",
			state:       baseWithFile,
			template:    "items:\n  - name: a\n    size: 1\n",
			content:     "items:\n  - name: a\n",
			wantErr:     []string{"missing keys", "items"},
			wantNotErr:  []string{"lyx config reconcile"},
			wantInvalid: true,
		},
		{
			name:        "unparseable file",
			state:       baseWithFile,
			template:    "path: _board\n",
			content:     "path: [unclosed\n",
			wantErr:     []string{"board.yaml"},
			wantNotErr:  []string{"lyx config reconcile"},
			wantInvalid: true,
		},
		{
			name:        "filled env marker whose variable is unset",
			state:       baseWithFile,
			template:    "path: _board\ntoken: ${env:TEST_FILL_UNSET_VAR}\n",
			content:     "path: custom\n",
			wantErr:     []string{"TEST_FILL_UNSET_VAR"},
			wantNotErr:  []string{"lyx config reconcile"},
			wantInvalid: true,
		},
		{
			name:        "list at an undeclared open map",
			state:       baseWithFile,
			template:    openMapTemplate,
			content:     "name: x\nlabels:\n  - bug\n  - docs\n",
			wantErr:     []string{"labels"},
			wantNotErr:  []string{"lyx config reconcile"},
			wantInvalid: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			baseDir, _ := newBase(t, tc.state, "board", tc.content)

			_, err := configengine.Load(baseDir, "board", []byte(tc.template), tc.openMaps...)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("expected error containing %q, got: %v", want, err)
				}
			}
			for _, unwanted := range tc.wantNotErr {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("a refusal reconcile cannot fix must not hint at %q, got: %v", unwanted, err)
				}
			}
			if got := errors.Is(err, configengine.ErrInvalid); got != tc.wantInvalid {
				t.Errorf("Load error marked ErrInvalid = %v; want %v: %v", got, tc.wantInvalid, err)
			}

			// An absent file or _lyx/ degrades under LoadOrTemplate; a present path errors exactly as under Load.
			if tc.state != baseWithFile && tc.state != baseUnreadable {
				return
			}
			_, err = configengine.LoadOrTemplate(baseDir, "board", []byte(tc.template), tc.openMaps...)
			if err == nil {
				t.Fatalf("LoadOrTemplate: expected error, got nil")
			}
			if got := errors.Is(err, configengine.ErrInvalid); got != tc.wantInvalid {
				t.Errorf("LoadOrTemplate error marked ErrInvalid = %v; want %v: %v", got, tc.wantInvalid, err)
			}
		})
	}
}

// TestMarkInvalid pins the mark's contract: the error text is unchanged, the wrapped error stays reachable, and a nil error stays nil.
func TestMarkInvalid(t *testing.T) {
	t.Parallel()
	cause := errors.New("bad value")

	marked := configengine.MarkInvalid(cause)
	if !errors.Is(marked, configengine.ErrInvalid) {
		t.Errorf("MarkInvalid result does not match ErrInvalid: %v", marked)
	}
	if !errors.Is(marked, cause) {
		t.Errorf("MarkInvalid result lost the wrapped error: %v", marked)
	}
	if marked.Error() != cause.Error() {
		t.Errorf("MarkInvalid changed the text: got %q, want %q", marked.Error(), cause.Error())
	}
	if configengine.MarkInvalid(nil) != nil {
		t.Errorf("MarkInvalid(nil) = non-nil")
	}
}

// TestLoadOrTemplate pins the degrading loader: proven absence of _lyx/, of the config file or of the base directory resolves the template with env overrides honored, and a present file loads exactly as the strict loader loads it.
// It serializes its rows because they swap the process-global logger output and set env variables.
func TestLoadOrTemplate(t *testing.T) {
	tests := []struct {
		loadCase
		state baseState
	}{
		{
			loadCase{name: "absent _lyx resolves the template", template: "path: _board\nhome: Home.md\n", contains: []string{"path: _board", "home: Home.md"}},
			baseNone,
		},
		{
			loadCase{name: "absent config file resolves the template", template: "path: _board\n", contains: []string{"path: _board"}},
			baseConfigDir,
		},
		{
			loadCase{name: "base directory that does not exist resolves the template", template: "path: _board\n", contains: []string{"path: _board"}},
			baseAbsent,
		},
		{
			loadCase{
				name:     "env override reaches the template with no _lyx on disk",
				template: "path: ${env:TEST_FALLBACK_VAR:-default_path}\n",
				env:      map[string]string{"TEST_FALLBACK_VAR": "overridden_value"},
				contains: []string{"path: overridden_value"},
			},
			baseNone,
		},
		{
			loadCase{
				name:     "present file loads like the strict loader",
				template: "path: _board\nhome: Home.md\n",
				content:  "path: custom_path\nhome: Index.md\n",
				contains: []string{"path: custom_path", "home: Index.md"},
			},
			baseWithFile,
		},
		{
			loadCase{
				name:     "present file missing a key loads it at its template default",
				template: "path: _board\nhome: Home.md\n",
				content:  "path: custom_path\n",
				wantFill: "home",
				contains: []string{"path: custom_path", "home: Home.md"},
			},
			baseWithFile,
		},
		{
			loadCase{name: "present empty file loads as the template", template: "path: _board\n", wantFill: "path", contains: []string{"path: _board"}},
			baseWithFile,
		},
		{
			loadCase{
				name:     "present comments-only file loads as the template",
				template: "path: _board\n",
				content:  "# just a comment\n",
				wantFill: "path",
				contains: []string{"path: _board"},
			},
			baseWithFile,
		},
		{
			loadCase{
				name:        "declared open map is passed through",
				template:    openMapTemplate,
				content:     "name: x\nlabels:\n  docs: documentation\n",
				openMaps:    []string{"labels"},
				contains:    []string{"docs: documentation"},
				notContains: []string{"bug:"},
			},
			baseWithFile,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			baseDir, path := newBase(t, tc.state, "board", tc.content)

			resolved, err := configengine.LoadOrTemplate(baseDir, "board", []byte(tc.template), tc.openMaps...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.state == baseWithFile {
				checkLoaded(t, tc.loadCase, resolved, buf, path)
				want, err := configengine.Load(baseDir, "board", []byte(tc.template), tc.openMaps...)
				if err != nil {
					t.Fatalf("unexpected error from Load: %v", err)
				}
				if string(resolved) != string(want) {
					t.Errorf("LoadOrTemplate result = %q; want %q (Load result)", resolved, want)
				}
				return
			}
			for _, want := range tc.contains {
				if !strings.Contains(string(resolved), want) {
					t.Errorf("resolved = %q; want it to contain %q", resolved, want)
				}
			}
		})
	}
}

// TestLoadOrTemplate_UnsetRequiredEnv_WrapsAsConfigTemplate pins the fallback tail's own error wrap: with no _lyx/ on disk, a template whose required ${env:NAME} marker has no variable returns an error naming "config template:" and the module, and never a config-file path that does not exist.
func TestLoadOrTemplate_UnsetRequiredEnv_WrapsAsConfigTemplate(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	_, err := configengine.LoadOrTemplate(baseDir, "board", []byte("path: ${env:TEST_UNSET_REQUIRED_VAR}\n"))
	if err == nil {
		t.Fatalf("expected error for unset required env var, got nil")
	}
	if !strings.Contains(err.Error(), "config template:") {
		t.Errorf("expected error containing 'config template:', got: %v", err)
	}
	if !strings.Contains(err.Error(), "board") {
		t.Errorf("expected error containing module name 'board', got: %v", err)
	}
	if strings.Contains(err.Error(), "config file") {
		t.Errorf("expected error to NOT contain 'config file', got: %v", err)
	}
}

// TestLoadOrTemplate_LyxDirStatFailure_DoesNotFallback tests absence-only discrimination: an _lyx/ that exists but cannot be stat'd makes LoadOrTemplate return an error rather than falling back to the template.
//
//testtiming:keep pins that an _lyx/ that cannot be stat'd errors instead of resolving the template, which no covering test reaches
func TestLoadOrTemplate_LyxDirStatFailure_DoesNotFallback(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial is not meaningful on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory mode bits")
	}

	tmpDir, _ := newBase(t, baseLyx, "board", "")
	t.Cleanup(func() {
		if err := os.Chmod(tmpDir, 0755); err != nil {
			t.Errorf("failed to restore tmpDir mode: %v", err)
		}
	})

	if err := os.Chmod(tmpDir, 0o000); err != nil {
		t.Fatalf("failed to chmod tmpDir: %v", err)
	}

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", []byte("path: _board\n"))
	if err == nil {
		t.Fatalf("expected error for unstattable _lyx/, got nil")
	}
	if errors.Is(err, configengine.ErrNotInitialized) {
		t.Errorf("expected a stat failure to NOT satisfy errors.Is(err, ErrNotInitialized), got: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil resolved bytes, got: %v", resolved)
	}
}

// TestFindBaseDir pins that a base directory holding _lyx/ is returned as is, and that one without it is refused with an error that satisfies errors.Is(err, ErrNotInitialized) -- the sentinel is wrapped, not returned bare -- while its text still says "not initialized", since strict callers match on that text.
//
//testtiming:keep pins FindBaseDir's own return value and its wrapped sentinel, which the covering Edit rows only match as text
func TestFindBaseDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state baseState
		want  bool
	}{
		{"_lyx present", baseLyx, true},
		{"_lyx absent", baseNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir, _ := newBase(t, tt.state, "board", "")

			got, err := configengine.FindBaseDir(baseDir)
			if tt.want {
				if err != nil || got != baseDir {
					t.Errorf("FindBaseDir() = (%q, %v); want (%q, nil)", got, err, baseDir)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got nil; result: %v", got)
			}
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
			if !errors.Is(err, configengine.ErrNotInitialized) {
				t.Errorf("expected errors.Is(err, ErrNotInitialized) to hold, got: %v", err)
			}
			if !strings.Contains(err.Error(), "not initialized") {
				t.Errorf("expected error containing 'not initialized', got: %v", err)
			}
		})
	}
}

// TestErrNotInitialized_NotMatchedByUnrelatedError tests that a hand-constructed, unrelated error does not satisfy errors.Is(err, configengine.ErrNotInitialized).
//
//testtiming:keep pins that the sentinel matches by identity and not by message text, which no covering test asserts
func TestErrNotInitialized_NotMatchedByUnrelatedError(t *testing.T) {
	t.Parallel()
	unrelated := errors.New("not initialized")

	if errors.Is(unrelated, configengine.ErrNotInitialized) {
		t.Errorf("expected a hand-constructed error with the same text to NOT satisfy errors.Is against the sentinel")
	}
}

// TestConfigPaths pins the path constructors configengine singly declares: ConfigDir and ConfigFile live under LyxDirName, StagingFile lives under the .lyx counterpart, ConfigFileRel is relative and in lockstep with ConfigFile, and LyxDirName is "_lyx" -- the token internal/lyxdirs is the sole declarer of.
func TestConfigPaths(t *testing.T) {
	t.Parallel()

	base := "/home/user/project"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ConfigDir", configengine.ConfigDir(base), filepath.Join(base, lyxdirs.LyxDirName, "config")},
		{"ConfigFile", configengine.ConfigFile(base, "myapp"), filepath.Join(base, lyxdirs.LyxDirName, "config", "myapp.yaml")},
		{"ScratchDir", configengine.ScratchDir(base), filepath.Join(base, lyxdirs.DotLyxDirName, "config")},
		{"StagingFile", configengine.StagingFile(base, "board"), filepath.Join(base, lyxdirs.DotLyxDirName, "config", "board.yaml")},
		{"ConfigFileRel loom", configengine.ConfigFileRel("loom"), filepath.Join(lyxdirs.LyxDirName, "config", "loom.yaml")},
		{"ConfigFileRel board", configengine.ConfigFileRel("board"), filepath.Join(lyxdirs.LyxDirName, "config", "board.yaml")},
		{"ConfigFile is ConfigFileRel under the base", configengine.ConfigFile(base, "myapp"), filepath.Join(base, configengine.ConfigFileRel("myapp"))},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
		}
	}

	if got := configengine.StagingFile(base, "board"); got == configengine.ConfigFile(base, "board") {
		t.Errorf("StagingFile equals ConfigFile %q; want the .lyx counterpart", got)
	}
	if got := configengine.ConfigFileRel("loom"); filepath.IsAbs(got) {
		t.Errorf("ConfigFileRel(%q) = %q; want a relative path", "loom", got)
	}
	if lyxdirs.LyxDirName != "_lyx" {
		t.Errorf("LyxDirName = %q; want %q", lyxdirs.LyxDirName, "_lyx")
	}
}
