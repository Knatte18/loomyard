// remove_weftprobe_test.go pins refuseDirtyWeftWorktree's three answers.
// It is untagged Tier 1 for the absent-worktree case only — the case that needs no git spawn — since
// that is the one an empty if-branch used to conflate with "clean".

package fabricengine

import (
	"path/filepath"
	"testing"
)

// TestRefuseDirtyWeftWorktree_AbsentIsNotARefusal asserts an absent weft worktree passes the gate:
// there is no uncommitted work to lose, and tearing down a half-present pair is Remove's job.
func TestRefuseDirtyWeftWorktree_AbsentIsNotARefusal(t *testing.T) {
	t.Parallel()

	absent := filepath.Join(t.TempDir(), "no-such-weft")
	if err := refuseDirtyWeftWorktree(absent); err != nil {
		t.Errorf("refuseDirtyWeftWorktree(%q) = %v; want nil for an absent weft worktree", absent, err)
	}
}
