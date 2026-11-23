// strandrig_test.go pins how the card fixer's strand rig configures the runner it builds.

package gatecli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestNewStrandRig_RunnerHoldsAReplyProbe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	// A .git directory marks the worktree as the prime.
	if err := os.MkdirAll(filepath.Join(hub, "hub-repo", ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) = %v; want nil", err)
	}
	location := &lymcwd.Location{RepoName: "example", HubPath: hub, WorktreeName: "hub-repo", AnchorRel: "."}

	rig, err := newStrandRig(location)
	if err != nil {
		t.Fatalf("newStrandRig() error = %v; want nil", err)
	}
	if !rig.runner.HasReplyProbe() {
		t.Error("rig.runner.HasReplyProbe() = false; want the reply probe wired on the card fixer's runner")
	}
}
