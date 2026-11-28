// anchorchange_onerepository_integration_test.go covers the change of a worktree's anchor in a one-repository hub.
// The hub is built by hubforge.NewOneRepoHub, so the move is one records commit on the pair's own branch.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newOneRepositoryAnchorHub returns a one-repository hub at "." whose prime tracks sub/, the directory a pair moves to.
func newOneRepositoryAnchorHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.NewOneRepoHub(t, ".")
	gitkit.CommitFile(t, h.PrimeWorktree(), "sub/.keep", "", "track sub")
	return h
}

// moveOriginBranchToSub moves branch on the hub's origin to the anchor sub through a second clone of the code repository.
func moveOriginBranchToSub(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	second := filepath.Join(t.TempDir(), "second")
	gitkit.Git(t, filepath.Dir(second), "clone", "-q", h.CodeBare, second)
	gitkit.Git(t, second, "checkout", "-q", branch)
	gitkit.Git(t, second, "mv", "_lym", "sub/_lym")
	if err := os.WriteFile(filepath.Join(second, lymcwd.AnchorFileName), []byte("sub\n"), 0o644); err != nil {
		t.Fatalf("write the marker: %v", err)
	}
	gitkit.Git(t, second, "add", "--", lymcwd.AnchorFileName)
	gitkit.Git(t, second, "commit", "-q", "-m", "move the anchor to sub")
	gitkit.Git(t, second, "push", "-q", "origin", branch)
}

func TestAnchorChange_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	const slug = "mover"

	t.Run("MovesRecordsAndMarkerInOneCommit", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryAnchorHub(t)
		res := hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		before := gitkit.RevParse(t, res.Path, "HEAD")

		c, err := beginAnchorChange(t, h, slug, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		completeAnchorChange(t, c)

		if commits := gitkit.RevListCount(t, res.Path, before+"..HEAD"); commits != 1 {
			t.Errorf("the change made %d commits; want one", commits)
		}
		if message, want := gitkit.Git(t, res.Path, "log", "-1", "--format=%B"), fabricengine.AnchorChangeTrailerKey+": sub"; !strings.Contains(message, want) {
			t.Errorf("commit message = %q; want the trailer %q", message, want)
		}
		head := gitkit.RevParse(t, res.Path, "HEAD")
		recordsPaths, err := fabricengine.RecordsPathsAt(res.Path, head)
		if err != nil {
			t.Fatalf("RecordsPathsAt() error = %v", err)
		}
		changed := strings.Fields(gitkit.Git(t, res.Path, "diff-tree", "-r", "--no-commit-id", "--name-only", head))
		if len(changed) == 0 {
			t.Fatalf("the commit changes no path; want the records and the marker")
		}
		for _, path := range changed {
			onRecordsPath := false
			for _, recordsPath := range recordsPaths {
				onRecordsPath = onRecordsPath || path == recordsPath || strings.HasPrefix(path, recordsPath+"/")
			}
			if !onRecordsPath {
				t.Errorf("the commit changes %s; want records paths %v alone", path, recordsPaths)
			}
		}
		pair, err := lymcwd.NamedWorktree(h.Path, slug)
		if err != nil || pair.AnchorRel != "sub" {
			t.Errorf("NamedWorktree() = (anchor %q, %v); want the pair at sub", pair.AnchorRel, err)
		}
		requireRealDirectory(t, filepath.Join(res.Path, "sub", "_lym"))
		if status := gitkit.GitStatusPorcelain(t, res.Path); status != "" {
			t.Errorf("worktree status = %q; want a clean tree", status)
		}
	})

	t.Run("OriginAtAnotherAnchorStops", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryAnchorHub(t)
		res := hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

		moveOriginBranchToSub(t, h, res.Branch)
		before := gitkit.RevParse(t, res.Path, "HEAD")

		pair, err := lymcwd.ResolveWorktree(res.Path)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", res.Path, err)
		}
		f, err := fabricengine.Open(pair)
		if err != nil {
			t.Fatalf("Open(%s): %v", res.Path, err)
		}
		_, err = f.Pull(fabricengine.SyncOptions{})

		var stop *fabricengine.WeftStopError
		if !errors.Is(err, fabricengine.ErrAnchorChange) || !errors.As(err, &stop) || !strings.Contains(stop.WayForward, "lym fabric anchor") {
			t.Errorf("Pull() error = %v; want an anchor-change stop naming `lym fabric anchor`", err)
		}
		if got := gitkit.RevParse(t, res.Path, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want %s unchanged by the stopped pull", got, before)
		}
	})

	t.Run("IntegrationOverALocalCodeCommitStopsNamingItAndMovesNothing", func(t *testing.T) {
		t.Parallel()
		h := newOneRepositoryAnchorHub(t)
		res := hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		moveOriginBranchToSub(t, h, res.Branch)
		localCode := gitkit.CommitFile(t, res.Path, "code.txt", "local\n", "local code")
		gitkit.Git(t, res.Path, "fetch", "-q")
		before := gitkit.RevParse(t, res.Path, "HEAD")
		c, err := beginAnchorChange(t, h, slug, "sub")
		if err != nil {
			t.Fatalf("BeginAnchorChange() error = %v; want nil", err)
		}
		defer c.Close()

		landed, err := c.Apply(fabricengine.NewMutations(""))

		if landed || err == nil || !strings.Contains(err.Error(), localCode) {
			t.Errorf("Apply() = (landed %v, %v); want an error naming the local code commit %s", landed, err, localCode)
		}
		if got := gitkit.RevParse(t, res.Path, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want %s unmoved by the stopped integration", got, before)
		}
	})
}
