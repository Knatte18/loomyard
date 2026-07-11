// load_test.go table-drives LoadRegistry against t.TempDir fixtures, using
// hubgeometry.ConfigFile to build every models.yaml path per the Hub Geometry
// Invariant (which applies to test code too).

package modelspec

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// writeModelsYAML writes contents to the models.yaml path under baseDir,
// creating the _lyx/config directory tree first.
func writeModelsYAML(t *testing.T, baseDir, contents string) {
	t.Helper()
	path := hubgeometry.ConfigFile(baseDir, "models")
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
	for alias, wantEntry := range want {
		gotEntry, ok := got[alias]
		if !ok || gotEntry.Engine != wantEntry.Engine || gotEntry.Model != wantEntry.Model || len(gotEntry.Defaults) != 0 {
			t.Errorf("LoadRegistry(absent)[%q] = %+v; want %+v", alias, gotEntry, wantEntry)
		}
	}
}

func TestLoadRegistry_EmptyFileYieldsBuiltins(t *testing.T) {
	baseDir := t.TempDir()
	writeModelsYAML(t, baseDir, "# comments only, no entries\n")

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(comments-only) returned unexpected error: %v", err)
	}
	if len(got) != len(builtins()) {
		t.Fatalf("LoadRegistry(comments-only) has %d entries; want %d (builtins unchanged)", len(got), len(builtins()))
	}
}

func TestLoadRegistry_FileExtends(t *testing.T) {
	baseDir := t.TempDir()
	writeModelsYAML(t, baseDir, `
zephyr:
  engine: claude
  model: claude-zephyr-1
  defaults:
    effort: high
`)

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(extends) returned unexpected error: %v", err)
	}

	// The four built-ins are still present alongside the new alias.
	for _, alias := range []string{"sonnet", "opus", "haiku", "fable"} {
		if _, ok := got[alias]; !ok {
			t.Errorf("LoadRegistry(extends) missing built-in alias %q", alias)
		}
	}
	zephyr, ok := got["zephyr"]
	if !ok {
		t.Fatal("LoadRegistry(extends) missing new alias \"zephyr\"")
	}
	if zephyr.Engine != "claude" || zephyr.Model != "claude-zephyr-1" || zephyr.Defaults["effort"] != "high" {
		t.Errorf("LoadRegistry(extends)[\"zephyr\"] = %+v; want engine claude, model claude-zephyr-1, defaults effort=high", zephyr)
	}
}

func TestLoadRegistry_FileOverridesWholeEntry(t *testing.T) {
	baseDir := t.TempDir()
	writeModelsYAML(t, baseDir, `
sonnet:
  engine: claude
  model: claude-sonnet-5
`)

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(override) returned unexpected error: %v", err)
	}
	sonnet := got["sonnet"]
	if sonnet.Model != "claude-sonnet-5" {
		t.Errorf("LoadRegistry(override)[\"sonnet\"].Model = %q; want %q", sonnet.Model, "claude-sonnet-5")
	}
	// Whole-entry replacement: the built-in had no Defaults to begin with, so
	// this also proves an override never inherits a stale built-in default —
	// there is none present here, and none must leak in.
	if len(sonnet.Defaults) != 0 {
		t.Errorf("LoadRegistry(override)[\"sonnet\"].Defaults = %v; want none (whole-entry replacement, no leaked defaults)", sonnet.Defaults)
	}
}

func TestLoadRegistry_RejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name       string
		contents   string
		wantSubstr string
	}{
		{
			name: "missing engine",
			contents: `
sonnet:
  model: sonnet
`,
			wantSubstr: "has no engine",
		},
		{
			name: "missing model",
			contents: `
sonnet:
  engine: claude
`,
			wantSubstr: "has no model",
		},
		{
			name: "unknown entry field",
			contents: `
sonnet:
  engine: claude
  model: sonnet
  weight: 5
`,
			wantSubstr: "field",
		},
		{
			name: "unknown defaults key",
			contents: `
sonnet:
  engine: claude
  model: sonnet
  defaults:
    speed: fast
`,
			wantSubstr: "unknown defaults key",
		},
		{
			name: "unknown engine",
			contents: `
sonnet:
  engine: gemini
  model: sonnet
`,
			wantSubstr: "unknown engine",
		},
		{
			name:       "malformed yaml",
			contents:   "sonnet: [this is not a map",
			wantSubstr: "parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			writeModelsYAML(t, baseDir, tt.contents)

			_, err := LoadRegistry(baseDir)
			if err == nil {
				t.Fatalf("LoadRegistry(%s) returned nil error; want error containing %q", tt.name, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("LoadRegistry(%s) error = %q; want substring %q", tt.name, err.Error(), tt.wantSubstr)
			}
			if !strings.HasPrefix(err.Error(), "modelspec: ") {
				t.Errorf("LoadRegistry(%s) error = %q; want prefix \"modelspec: \"", tt.name, err.Error())
			}
		})
	}
}
