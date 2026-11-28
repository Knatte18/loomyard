//go:build !windows

// leftoverhandedover_integration_test.go drives RemoveHandedOverPair over a hub pair with a live reed session.
// It pins that the local removal ends the pair's session and deletes no origin branch.

package pairteardown_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestRemoveHandedOverPair drives RemoveHandedOverPair over a dirty pair with a live reed session.
// It needs a real multiplexer, since it asserts the session is ended.
func TestRemoveHandedOverPair(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()

	const slug = "handed-over-run"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
	tmuxkit.KillOnCleanup(t, cfg.Tmux, reedengine.ServerName(h.Path))
	p := addLivePair(t, h, cfg.Tmux, slug)
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	if _, err := fabricengine.PushPairAnchored(pair, fabricengine.EnvSyncOptions(), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
		t.Fatalf("priming push error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(h.PairCodeWorktree(slug), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write the uncommitted file: %v", err)
	}
	codeRef := "refs/heads/" + gitkit.CurrentBranch(t, h.PairCodeWorktree(slug))
	recordsRef := "refs/heads/" + fabricengine.RecordsBranchName(gitkit.CurrentBranch(t, h.PairCodeWorktree(slug)))
	codeBefore := gitkit.RevParse(t, h.CodeBare, codeRef)
	recordsBefore := gitkit.RevParse(t, h.RecordsBare, recordsRef)
	if !p.sessionUp(t) {
		t.Fatal("the pair's session is not up before the removal")
	}

	if _, err := pairteardown.RemoveHandedOverPair(h.Location, slug); err != nil {
		t.Fatalf("RemoveHandedOverPair() error = %v; want nil", err)
	}

	requirePairGone(t, h, slug)
	if p.sessionUp(t) {
		t.Error("the pair's session is still up after the removal")
	}
	if got := gitkit.RevParse(t, h.CodeBare, codeRef); got != codeBefore {
		t.Errorf("origin code branch = %s; want it standing at %s", got, codeBefore)
	}
	if got := gitkit.RevParse(t, h.RecordsBare, recordsRef); got != recordsBefore {
		t.Errorf("origin records branch = %s; want it standing at %s", got, recordsBefore)
	}
}
