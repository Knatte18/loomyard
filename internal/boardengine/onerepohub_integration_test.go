// onerepohub_integration_test.go covers a board write in a one-repository hub.
// There the board is a branch of the code repository.
// The write must reach origin on that branch alone and leave every code branch where it was.

package boardengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remoteBranchTips returns the tip of every branch in the bare repository at bare, keyed by branch name.
func remoteBranchTips(t *testing.T, bare string) map[string]string {
	t.Helper()

	tips := make(map[string]string)
	for _, line := range strings.Split(gitkit.Git(t, bare, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			tips[fields[0]] = fields[1]
		}
	}
	return tips
}

// TestBoardWrite_OneRepositoryHub writes a file into a one-repository hub's board and syncs it.
// The commit and push land on the board branch, and no code branch moves on origin.
func TestBoardWrite_OneRepositoryHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.NewOneRepoHub(t, ".")
	before := remoteBranchTips(t, h.CodeBare)
	boardBefore := gitkit.RevParse(t, h.BoardDir(), "HEAD")

	if err := os.WriteFile(filepath.Join(h.BoardDir(), "one-repository-note.md"), []byte("a board write\n"), 0o644); err != nil {
		t.Fatalf("write into the board: %v", err)
	}
	if err := boardengine.Sync(h.BoardDir(), false, false); err != nil {
		t.Fatalf("Sync(%s) error = %v; want nil", h.BoardDir(), err)
	}

	boardAfter := gitkit.RevParse(t, h.BoardDir(), "HEAD")
	if boardAfter == boardBefore {
		t.Fatalf("the board's HEAD stayed %s after a write and a sync; want a commit", boardBefore)
	}
	after := remoteBranchTips(t, h.CodeBare)
	if got := after[fabricengine.BoltBranchName]; got != boardAfter {
		t.Errorf("origin's %s = %s; want the board's HEAD %s", fabricengine.BoltBranchName, got, boardAfter)
	}
	for branch, tip := range before {
		if branch != fabricengine.BoltBranchName && after[branch] != tip {
			t.Errorf("origin's %s = %s after a board write; want it to stay %s", branch, after[branch], tip)
		}
	}
	if len(after) != len(before) {
		t.Errorf("origin holds branches %v after a board write; want the same set as %v", after, before)
	}
}
