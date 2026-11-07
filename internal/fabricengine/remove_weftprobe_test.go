// remove_weftprobe_test.go pins refuseDirtyWeftWorktree's three answers.
// Only the absent-worktree case is untagged Tier 1, since it needs no git spawn.
// That case is the one an empty branch of the dirty check would conflate with "clean".

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRefuseDirtyWeftWorktree_AbsentIsNotARefusal asserts an absent weft worktree passes the gate:
// there is no uncommitted work to lose, and tearing down a half-present pair is Remove's job.
//
//testtiming:keep an absent weft worktree passing the dirty-weft gate; coverage of its blocks by other tests does not show an assertion of this
func TestRefuseDirtyWeftWorktree_AbsentIsNotARefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	absent := filepath.Join(t.TempDir(), "no-such-weft")
	if err := refuseDirtyWeftWorktree(absent, ".", nil); err != nil {
		t.Errorf("refuseDirtyWeftWorktree(%q) = %v; want nil for an absent weft worktree", absent, err)
	}
}
