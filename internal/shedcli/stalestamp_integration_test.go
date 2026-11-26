// stalestamp_integration_test.go pins that a shed verb reconciles the hub's config before the addressed recipe arms.

package shedcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The held run lock refuses the step after loom's strict config load and before reed starts, so no tmux server runs.
func TestShedVerbs_StaleStampReconcilesBeforeALoomRecipeArms(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "stale")
	cwd := h.PairCodeWorktree("stale")
	seedRunForTest(t, cwd, shedrun.SelfRunID, shedrun.RecipeTapestry)
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("loom"), "code_max_bounces: 2\n", "fixture: old loom key")

	location, err := lymcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	lockPath := shedrun.RunLock(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%s): %v", lockPath, err)
	}
	t.Cleanup(func() { _ = held.Release() })

	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), cwd, &out, []string{"step"})
	if kind := envelope.Decode(t, out.String()).Raw["kind"]; code != 1 || kind != "busy" || strings.Contains(out.String(), "was moved") {
		t.Errorf("shed step = %d, %s; want 1 and the busy refusal", code, out.String())
	}
	base, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "loom"))
	if err != nil {
		t.Fatalf("read the hub base loom.yaml: %v", err)
	}
	if got := string(base); !strings.Contains(got, "code:\n") || !strings.Contains(got, "max_bounces: 2") || strings.Contains(got, "code_max_bounces:") {
		t.Errorf("hub base loom.yaml = %q; want code.max_bounces: 2 and no old key", got)
	}
}
