// template_test.go proves the embedded seed template always passes the
// loader's own validation: writing ConfigTemplate() verbatim to a fresh
// models.yaml must load with exactly the four documented live entries.

package modelspec

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

func TestConfigTemplate_LoadsWithLiveEntries(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(hubgeometry.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", hubgeometry.ConfigDir(baseDir), err)
	}
	path := hubgeometry.ConfigFile(baseDir, "models")
	if err := os.WriteFile(path, []byte(ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}

	got, err := LoadRegistry(baseDir)
	if err != nil {
		t.Fatalf("LoadRegistry(seeded template) returned unexpected error: %v", err)
	}

	tests := []struct {
		alias      string
		wantEffort string
		wantNoDef  bool
	}{
		{alias: "sonnet", wantEffort: "medium"},
		{alias: "opus", wantEffort: "high"},
		{alias: "haiku", wantNoDef: true},
		{alias: "fable", wantEffort: "high"},
	}
	if len(got) != len(tests) {
		t.Fatalf("LoadRegistry(seeded template) has %d entries; want %d", len(got), len(tests))
	}
	for _, tt := range tests {
		entry, ok := got[tt.alias]
		if !ok {
			t.Errorf("LoadRegistry(seeded template) missing alias %q", tt.alias)
			continue
		}
		if entry.Engine != "claude" || entry.Model != tt.alias {
			t.Errorf("LoadRegistry(seeded template)[%q] = %+v; want engine claude, model %q", tt.alias, entry, tt.alias)
		}
		if tt.wantNoDef {
			if len(entry.Defaults) != 0 {
				t.Errorf("LoadRegistry(seeded template)[%q].Defaults = %v; want none", tt.alias, entry.Defaults)
			}
			continue
		}
		if entry.Defaults["effort"] != tt.wantEffort {
			t.Errorf("LoadRegistry(seeded template)[%q].Defaults[\"effort\"] = %q; want %q", tt.alias, entry.Defaults["effort"], tt.wantEffort)
		}
	}
}
