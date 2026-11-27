// add_anchor_integration_test.go covers Add forking a pair at a requested anchor.
// It runs over hubforge hubs whose prime warp tracks y/.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newAtAnchorHub builds a "." hub whose prime warp tracks y/ before any pair is added.
func newAtAnchorHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.CommitFile(t, h.PrimeWorktree(), "y/.keep", "", "track y")
	return h
}

// requireWeftAtY fails the test unless slug's weft carries its content and its marker at y in the HEAD commit.
// It also requires a clean tree, the pair wired there, and no move record left.
func requireWeftAtY(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	weft := h.PairRecordsSibling(slug)
	if marker := gitkit.Git(t, weft, "show", "HEAD:"+lymcwd.AnchorFileName); marker != "y" {
		t.Errorf("committed marker of %s = %q; want y", slug, marker)
	}
	for _, tracked := range gitkit.LsFiles(t, weft) {
		if tracked != lymcwd.AnchorFileName && !strings.HasPrefix(tracked, "y/") {
			t.Errorf("weft of %s tracks %s; want everything but the marker under y/", slug, tracked)
		}
	}
	if status := gitkit.GitStatusPorcelain(t, weft); status != "" {
		t.Errorf("weft of %s status = %q; want a clean tree", slug, status)
	}
	requireWiredOnlyAt(t, h, slug, "y", ".")
	requireNoMoveRecord(t, weft)
}

func TestAdd_AtAnchor(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("A beam added at y has its marker, content and wiring there and no record", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		weftRoot := mustRecordsRepoRoot(t, h.Location)
		commitRunRecords(t, h.Location, weftRoot)

		hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "y"})

		requireWeftAtY(t, h, "beam")
		if tree := gitkit.Git(t, weftRoot, "ls-tree", "-r", "--name-only", fabriclayout.RecordsBranchName("beam")); strings.Contains(tree, "shed") {
			t.Errorf("weft branch of the beam tracks run records:\n%s", tree)
		}
		if _, err := os.Stat(filepath.Join(h.PairRecordsSibling("beam"), "y", fabriclayout.OriginRecordRel())); err != nil {
			t.Errorf("origin record at y: %v; want it", err)
		}
	})

	t.Run("A fork point with no committed marker is moved from its resolved anchor and gets the marker", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		gitkit.Git(t, h.PrimeRecords(), "rm", "-q", "--", lymcwd.AnchorFileName)
		gitkit.Git(t, h.PrimeRecords(), "commit", "-q", "-m", "drop the marker")

		hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "y"})

		requireWeftAtY(t, h, "beam")
	})

	t.Run("An anchor equal to the fork point's is no move", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)

		hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "."})

		if subject := gitkit.Git(t, h.PairRecordsSibling("beam"), "log", "-1", "--format=%s"); strings.Contains(subject, "fabric anchor") {
			t.Errorf("weft HEAD subject = %q; want no move commit", subject)
		}
		requireNoMoveRecord(t, h.PairRecordsSibling("beam"))
	})

	t.Run("An anchor the fork point cannot hold is refused with nothing created", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		if err := os.Symlink("y", filepath.Join(h.PrimeWorktree(), "lnk")); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		tests := []struct {
			name   string
			anchor string
			want   string
		}{
			{"Invalid", "../escape", "choose another directory"},
			{"Untracked", "absent", "commit the directory in the warp checkout first"},
			{"CrossingALink", "lnk", "passes through the link"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				slug := "beam-" + strings.ToLower(tt.name)
				_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{Anchor: tt.anchor})
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("Add(Anchor %q) error = %v; want it to name %q", tt.anchor, err, tt.want)
				}
				if _, statErr := os.Stat(h.PairCodeWorktree(slug)); !os.IsNotExist(statErr) {
					t.Errorf("warp worktree of %s: %v; want none created", slug, statErr)
				}
				if _, statErr := os.Stat(h.PairRecordsSibling(slug)); !os.IsNotExist(statErr) {
					t.Errorf("weft worktree of %s: %v; want none created", slug, statErr)
				}
				if gitkit.BranchExists(t, mustRecordsRepoRoot(t, h.Location), fabriclayout.RecordsBranchName(slug)) {
					t.Errorf("weft branch of %s exists; want none created", slug)
				}
			})
		}
	})
}
