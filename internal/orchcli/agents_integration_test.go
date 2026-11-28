// agents_integration_test.go proves wireCLI hands the orch role's skill list from the hub's agents.yaml to the receiver.

package orchcli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOrchIntegration_WireCLITakesTheOrchSkillsFromAgentsYAML(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	agentsFile := configengine.ConfigFile(fabricengine.BoltDir(h.Path), agentconfig.ModuleName)
	if err := os.MkdirAll(filepath.Dir(agentsFile), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(agentsFile), err)
	}
	if err := os.WriteFile(agentsFile, []byte("skills:\n  orch: [scribe:distinct]\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", agentsFile, err)
	}

	c := &orchCLI{}
	if err := wireCLI(c, h.Location); err != nil {
		t.Fatalf("wireCLI() error = %v", err)
	}
	if want := []string{"scribe:distinct"}; !slices.Equal(c.orchSkills, want) {
		t.Errorf("orchSkills = %v; want %v from the hub's agents.yaml", c.orchSkills, want)
	}
}
