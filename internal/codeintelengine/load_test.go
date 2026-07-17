// load_test.go table-drives LoadRegistry against t.TempDir fixtures, using
// hubgeometry.ConfigFile to build every servers.yaml path per the Hub
// Geometry Invariant (which applies to test code too), mirroring
// modelspec/load_test.go's pattern.

package codeintelengine

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// writeServersYAML writes contents to the servers.yaml path under baseDir,
// creating the _lyx/config directory tree first.
func writeServersYAML(t *testing.T, baseDir, contents string) {
	t.Helper()
	path := hubgeometry.ConfigFile(baseDir, "servers")
	if err := os.MkdirAll(hubgeometry.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", hubgeometry.ConfigDir(baseDir), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestLoadRegistry_AbsentFileYieldsBuiltins(t *testing.T) {
	baseDir := t.TempDir()

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(absent) returned unexpected error: %v", err)
	}

	want := builtins()
	if len(got) != len(want) {
		t.Fatalf("LoadRegistry(absent) has %d entries; want %d", len(got), len(want))
	}
	for lang, wantEntry := range want {
		gotEntry, ok := got[lang]
		if !ok || gotEntry.Match != wantEntry.Match || len(gotEntry.Markers) != len(wantEntry.Markers) {
			t.Errorf("LoadRegistry(absent)[%q] = %+v; want %+v", lang, gotEntry, wantEntry)
		}
	}
}

func TestLoadRegistry_EmptyFileYieldsBuiltins(t *testing.T) {
	baseDir := t.TempDir()
	writeServersYAML(t, baseDir, "# comments only, no entries\n")

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(comments-only) returned unexpected error: %v", err)
	}
	if len(got) != len(builtins()) {
		t.Fatalf("LoadRegistry(comments-only) has %d entries; want %d (builtins unchanged)", len(got), len(builtins()))
	}
}

func TestLoadRegistry_FileOverridesWholeEntry(t *testing.T) {
	baseDir := t.TempDir()
	writeServersYAML(t, baseDir, `
go:
  markers: [go.mod, go.work]
  match: any
  command: [gopls, serve]
  install_hint: "brew install gopls"
`)

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(override) returned unexpected error: %v", err)
	}

	// The four other built-ins are still present alongside the override.
	for _, lang := range []string{"python", "csharp", "typescript", "rust"} {
		if _, ok := got[lang]; !ok {
			t.Errorf("LoadRegistry(override) missing built-in language %q", lang)
		}
	}

	goEntry, ok := got["go"]
	if !ok {
		t.Fatal("LoadRegistry(override) missing overridden language \"go\"")
	}
	if !stringSlicesEqual(goEntry.Markers, []string{"go.mod", "go.work"}) {
		t.Errorf("LoadRegistry(override)[\"go\"].Markers = %v; want [go.mod go.work]", goEntry.Markers)
	}
	if !stringSlicesEqual(goEntry.Command, []string{"gopls", "serve"}) {
		t.Errorf("LoadRegistry(override)[\"go\"].Command = %v; want [gopls serve]", goEntry.Command)
	}
	if goEntry.InstallHint != "brew install gopls" {
		t.Errorf("LoadRegistry(override)[\"go\"].InstallHint = %q; want %q", goEntry.InstallHint, "brew install gopls")
	}
}

func TestLoadRegistry_FileExtendsWithNewLanguage(t *testing.T) {
	baseDir := t.TempDir()
	writeServersYAML(t, baseDir, `
zig:
  markers: [build.zig]
  match: any
  command: [zls]
  install_hint: "install zls"
`)

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(extends) returned unexpected error: %v", err)
	}
	for lang := range builtins() {
		if _, ok := got[lang]; !ok {
			t.Errorf("LoadRegistry(extends) missing built-in language %q", lang)
		}
	}
	zig, ok := got["zig"]
	if !ok {
		t.Fatal("LoadRegistry(extends) missing new language \"zig\"")
	}
	if zig.Match != "any" || zig.Command[0] != "zls" {
		t.Errorf("LoadRegistry(extends)[\"zig\"] = %+v; want match any, command starting with zls", zig)
	}
}

func TestLoadRegistry_RejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name       string
		contents   string
		wantSubstr string
	}{
		{
			name: "unknown yaml field",
			contents: `
go:
  markers: [go.mod]
  match: any
  command: [gopls]
  install_hint: "go install gopls"
  weight: 5
`,
			wantSubstr: "field",
		},
		{
			name: "out-of-vocab match",
			contents: `
go:
  markers: [go.mod]
  match: sometimes
  command: [gopls]
  install_hint: "go install gopls"
`,
			wantSubstr: "invalid match",
		},
		{
			name:       "malformed yaml",
			contents:   "go: [this is not a map",
			wantSubstr: "parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			writeServersYAML(t, baseDir, tt.contents)

			_, err := LoadRegistry(baseDir)
			if err == nil {
				t.Fatalf("LoadRegistry(%s) returned nil error; want error containing %q", tt.name, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("LoadRegistry(%s) error = %q; want substring %q", tt.name, err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestLoadRegistry_InvalidEntryErrorNamesAliasAndPath(t *testing.T) {
	baseDir := t.TempDir()
	writeServersYAML(t, baseDir, `
go:
  markers: [go.mod]
  match: sometimes
  command: [gopls]
  install_hint: "go install gopls"
`)

	_, err := LoadRegistry(baseDir)
	if err == nil {
		t.Fatal("LoadRegistry(invalid match) returned nil error; want error naming the alias and path")
	}
	path := hubgeometry.ConfigFile(baseDir, "servers")
	if !strings.Contains(err.Error(), `"go"`) {
		t.Errorf("LoadRegistry error = %q; want it to name the alias %q", err.Error(), "go")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("LoadRegistry error = %q; want it to name the file path %q", err.Error(), path)
	}
}
