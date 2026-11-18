// worktree_test.go verifies that the Claude engine keeps the worktree it is told at construction.

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestNewForWorktree pins that NewForWorktree keeps the told worktree and NewFromConfig keeps none.
//
//testtiming:keep pins that NewFromConfig tells no worktree, which no covering test asserts
func TestNewForWorktree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	told := shuttleengine.Worktree{Name: "beam-a", Kind: shuttleengine.WorktreeBeam, AccountDir: "/hub/accounts"}
	got := NewForWorktree(shuttleengine.Config{}, configengine.Dirs{}, told).worktree
	if got.Name != told.Name || got.Kind != told.Kind || got.AccountDir != told.AccountDir {
		t.Errorf("NewForWorktree kept worktree %+v; want %+v", got, told)
	}

	zero := NewFromConfig(shuttleengine.Config{}, configengine.Dirs{}).worktree
	if zero.Name != "" || zero.Kind != "" || zero.AccountDir != "" || zero.Names != nil {
		t.Errorf("NewFromConfig kept worktree %+v; want the zero value", zero)
	}
}
