// stalestamp_integration_test.go pins that the seat tables reconcile the hub's config before they load loom's.

package shuttleparity

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLoomConfigOf_StaleStampReconcilesTheHubBase(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("loom"), "code_max_bounces: 2\n", "fixture: old loom key")

	if _, err := loomConfigOf(SeatEnv{Location: h.Location, Dirs: fabricengine.ConfigDirs(h.Location)}); err != nil {
		t.Fatalf("loomConfigOf() = %v; want nil", err)
	}

	base, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "loom"))
	if err != nil {
		t.Fatalf("read the hub base loom.yaml: %v", err)
	}
	if got := string(base); !strings.Contains(got, "max_bounces: 2") || strings.Contains(got, "code_max_bounces") {
		t.Errorf("hub base loom.yaml = %q; want the old key nested", got)
	}
}
