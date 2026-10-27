//go:build tier1

// check_integration_test.go drives the adopt check over a hubforge hub: its reconcile and its gate key.

package adoptcli

import (
	"context"
	"io"
	"os"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

func TestHubCheckDeps_StaleStampRunsTheTunedFinalizeTiers(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.NewHub(t, ".")
	tier := map[string]any{"command": "true", "all_targets": ".", "timeout_min": 1, "counter": "go-json"}
	hubFile, err := yaml.Marshal(map[string]any{
		"slots":  1,
		"scoper": "none",
		"tiers":  map[string]any{"tier0": tier, "tier1": tier},
		"gates":  map[string]any{"finalize": map[string]any{"tiers": []int{0, 1}, "scope": "all"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("gate"), string(hubFile), "fixture: a hub tuning finalize to tiers 0 and 1")
	stamp := hubreconcile.Geometry{BoardDir: h.BoardDir()}.StampPath()
	if err := os.Remove(stamp); err != nil && !os.IsNotExist(err) {
		t.Fatalf("make the build stamp stale: %v", err)
	}

	deps := hubCheckDeps(h.Location)
	var adoptTiers []int
	var gateErr error
	deps.RunGate = func(context.Context, io.Writer, gatecli.TierObserver) int {
		var cfg gateslot.Config
		if cfg, gateErr = gateslot.LoadConfig(fabricengine.ConfigDirs(h.Location)); gateErr == nil {
			adoptTiers, gateErr = cfg.TierList("adopt")
		}
		return 0
	}
	deps.Notice = func(string) error { return nil }

	record, ran, err := runCheck(context.Background(), deps)
	if err != nil || !ran || record.State != checkGreen {
		t.Fatalf("runCheck() = %+v, ran %v, error %v; want a green check", record, ran, err)
	}
	if gateErr != nil || !slices.Equal(adoptTiers, []int{0, 1}) {
		t.Errorf("adopt tiers read by the gate = %v, error %v; want [0 1], the tuned finalize's", adoptTiers, gateErr)
	}
	data, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "gate"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Gates map[string]any `yaml:"gates"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if _, held := file.Gates["adopt"]; !held {
		t.Errorf("hub gate.yaml gates = %v; want adopt added beside the tuned finalize", file.Gates)
	}
}
