// load_test.go table-drives LoadRegistry against t.TempDir fixtures, using configengine.ConfigFile
// to build every models.yaml path per the Cwd Resolution Invariant (which applies to test code
// too).

package modelspec

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// writeModelsYAML writes contents to the models.yaml path under baseDir.
func writeModelsYAML(t *testing.T, baseDir, contents string) {
	t.Helper()
	path := configengine.ConfigFile(baseDir, "models")
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configengine.ConfigDir(baseDir), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// TestLoadRegistry_YieldsBuiltins asserts a missing models.yaml and a comments-only one both yield exactly the four built-in aliases,
// each with its own name as model and no defaults.
func TestLoadRegistry_YieldsBuiltins(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		contents string // empty means no models.yaml at all
	}{
		{"absent file", ""},
		{"comments only", "# comments only, no entries\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if tt.contents != "" {
				writeModelsYAML(t, baseDir, tt.contents)
			}

			got, err := LoadRegistry(baseDir)
			if err != nil {
				t.Fatalf("LoadRegistry(%s) returned unexpected error: %v", tt.name, err)
			}

			aliases := []string{"sonnet", "opus", "haiku", "fable"}
			if len(got) != len(aliases) {
				t.Fatalf("LoadRegistry(%s) has %d entries; want %d", tt.name, len(got), len(aliases))
			}
			for _, alias := range aliases {
				gotEntry, ok := got[alias]
				if !ok {
					t.Errorf("LoadRegistry(%s) missing alias %q", tt.name, alias)
					continue
				}
				if gotEntry.Engine != "claude" || gotEntry.Model != alias || len(gotEntry.Defaults) != 0 {
					t.Errorf("LoadRegistry(%s)[%q] = %+v; want engine claude, model %q, no defaults", tt.name, alias, gotEntry, alias)
				}
			}
		})
	}
}

func TestLoadRegistry_FileExtends(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
