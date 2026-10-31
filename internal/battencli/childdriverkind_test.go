//go:build tier0

// childdriverkind_test.go pins childDriverKind's resolution of a child run's driver from shed.yaml over temp config layers.

package battencli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestChildDriverKind verifies the driver comes from shed.yaml's driver_kind for a darn, a tapestry and a pawl child alike.
func TestChildDriverKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		recipe string
	}{
		{name: "darn", recipe: shedrun.RecipeDarn},
		{name: "tapestry", recipe: shedrun.RecipeTapestry},
		{name: "pawl", recipe: shedrun.RecipePawl},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ConfigDir(dir), err)
			}
			const wantDriver = "llm"
			document := strings.Replace(sheddriver.ConfigTemplate(), "driver_kind: go", "driver_kind: llm", 1)
			if err := os.WriteFile(configengine.ConfigFile(dir, "shed"), []byte(document), 0o644); err != nil {
				t.Fatalf("WriteFile(shed.yaml) = %v; want nil", err)
			}

			got, err := childDriverKind(configengine.Dirs{Anchor: dir}, tt.recipe)
			if err != nil {
				t.Fatalf("childDriverKind(%q) = _, %v; want nil error", tt.recipe, err)
			}
			if got != wantDriver {
				t.Errorf("childDriverKind(%q) = %q; want %q", tt.recipe, got, wantDriver)
			}
		})
	}
}
