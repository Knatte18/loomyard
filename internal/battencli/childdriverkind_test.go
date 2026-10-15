// childdriverkind_test.go pins childDriverKind's resolution of a child run's driver over temp config layers.

package battencli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestChildDriverKind verifies the driver comes from the child recipe's own driver_kind override, else shed.yaml's, so a darn child takes darn.yaml and a loom child loom.yaml.
func TestChildDriverKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		recipe     string
		loomKind   string
		darnKind   string
		shedKind   string
		wantDriver string
	}{
		{name: "a darn recipe takes darn.yaml's go over loom.yaml's llm", recipe: shedrun.RecipeDarn, loomKind: "llm", darnKind: "go", wantDriver: "go"},
		{name: "a loom recipe takes loom.yaml's llm over darn.yaml's go", recipe: shedrun.RecipeLoom, loomKind: "llm", darnKind: "go", wantDriver: "llm"},
		{name: "empty overrides take shed.yaml's go", recipe: shedrun.RecipeLoom, shedKind: "go", wantDriver: "go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ConfigDir(dir), err)
			}
			withKind := func(template, kind string) string {
				return strings.Replace(template, `driver_kind: ""`, "driver_kind: "+kind, 1)
			}
			documents := map[string]string{
				"loom": loomengine.ConfigTemplate(),
				"darn": darnengine.ConfigTemplate(),
				"shed": sheddriver.ConfigTemplate(),
			}
			if tt.loomKind != "" {
				documents["loom"] = withKind(documents["loom"], tt.loomKind)
			}
			if tt.darnKind != "" {
				documents["darn"] = withKind(documents["darn"], tt.darnKind)
			}
			if tt.shedKind != "" {
				documents["shed"] = strings.Replace(documents["shed"], "driver_kind: llm", "driver_kind: "+tt.shedKind, 1)
			}
			for module, document := range documents {
				if err := os.WriteFile(configengine.ConfigFile(dir, module), []byte(document), 0o644); err != nil {
					t.Fatalf("WriteFile(%s.yaml) = %v; want nil", module, err)
				}
			}

			got, err := childDriverKind(configengine.Dirs{Anchor: dir}, tt.recipe)
			if err != nil {
				t.Fatalf("childDriverKind(%q) = _, %v; want nil error", tt.recipe, err)
			}
			if got != tt.wantDriver {
				t.Errorf("childDriverKind(%q) = %q; want %q", tt.recipe, got, tt.wantDriver)
			}
		})
	}
}
