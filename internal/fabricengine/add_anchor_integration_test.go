//go:build tier1

// add_anchor_integration_test.go covers Add forking a pair at a requested anchor and EnsureAnchor settling the anchor of a present pair, over hubforge hubs whose prime warp tracks y/.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// newAtAnchorHub builds a "." hub whose prime warp tracks y/ before any pair is added.
func newAtAnchorHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.CommitFile(t, h.PrimeWorktree(), "y/.keep", "", "track y")
	return h
}

// requireWeftAtY fails the test unless slug's weft carries its content and its marker at y in the HEAD commit, with a clean tree, and the pair is wired there with no move record left.
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

// writeMoveRecordByHand writes the move record of slug's weft for a move from a to b, as a process killed in the middle of the move leaves it.
func writeMoveRecordByHand(t *testing.T, h *hubforge.Hub, slug, a, b string) {
	t.Helper()

	record := `{"a":"` + a + `","b":"` + b + `","kind":"move","pid":0}`
	if err := os.WriteFile(fabricengine.MoveRecordPath(h.PairRecordsSibling(slug)), []byte(record), 0o644); err != nil {
		t.Fatalf("write the move record of %s: %v", slug, err)
	}
}

// ensureAnchorOf settles slug's anchor with anchor and returns the error.
func ensureAnchorOf(t *testing.T, h *hubforge.Hub, slug, anchor string) error {
	t.Helper()

	pair, err := lymcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		t.Fatalf("NamedWorktree(%s): %v", slug, err)
	}
	return h.Topology.EnsureAnchor(fabricengine.NewMutations(""), pair, anchor)
}

func TestAdd_AtAnchor(t *testing.T) {
	t.Parallel()

	t.Run("A beam added at y has its marker, content, wiring and launcher there and no record", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		weftRoot := mustRecordsRepoRoot(t, h.Location)
		commitRunRecords(t, h.Location, weftRoot)

		hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "y"})

		requireWeftAtY(t, h, "beam")
		if tree := gitkit.Git(t, weftRoot, "ls-tree", "-r", "--name-only", fabricengine.RecordsBranchName("beam")); strings.Contains(tree, "shed") {
			t.Errorf("weft branch of the beam tracks run records:\n%s", tree)
		}
		if _, err := os.Stat(filepath.Join(h.PairRecordsSibling("beam"), "y", fabricengine.OriginRecordRel())); err != nil {
			t.Errorf("origin record at y: %v; want it", err)
		}
		if _, err := os.Stat(filepath.Join(h.Path, "_launchers", "y", "beam")); err != nil {
			t.Errorf("launcher directory at y: %v; want it", err)
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
				if gitkit.BranchExists(t, mustRecordsRepoRoot(t, h.Location), fabricengine.RecordsBranchName(slug)) {
					t.Errorf("weft branch of %s exists; want none created", slug)
				}
			})
		}
	})
}

func TestEnsureAnchor(t *testing.T) {
	t.Parallel()

	t.Run("The pair's own anchor, named or empty, skips the move and restores a removed junction", func(t *testing.T) {
		t.Parallel()
		for _, anchor := range []string{"y", ""} {
			h := newAtAnchorHub(t)
			hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "y"})
			tip := gitkit.RevParse(t, h.PairRecordsSibling("beam"), "HEAD")
			if err := os.Remove(filepath.Join(h.PairCodeWorktree("beam"), "y", lymdirs.LymDirName)); err != nil {
				t.Fatalf("remove the junction: %v", err)
			}

			if err := ensureAnchorOf(t, h, "beam", anchor); err != nil {
				t.Fatalf("EnsureAnchor(%q) error = %v; want nil", anchor, err)
			}

			requireWeftAtY(t, h, "beam")
			if got := gitkit.RevParse(t, h.PairRecordsSibling("beam"), "HEAD"); got != tip {
				t.Errorf("EnsureAnchor(%q) moved the weft tip %s -> %s; want it unmoved", anchor, tip, got)
			}
		}
	})

	t.Run("Another anchor is refused naming the move and the create", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "y"})

		err := ensureAnchorOf(t, h, "beam", "z")

		if err == nil || !strings.Contains(err.Error(), "lym fabric anchor beam --set z") || !strings.Contains(err.Error(), "lym beam create beam --anchor z") {
			t.Errorf("EnsureAnchor(z) error = %v; want it to name `lym fabric anchor` and `lym beam create`", err)
		}
	})

	t.Run("A kill before the move commit is undone and finished at y", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		hubforge.AddPair(t, h, "beam")
		weft := h.PairRecordsSibling("beam")
		writeMoveRecordByHand(t, h, "beam", ".", "y")
		if err := os.MkdirAll(filepath.Join(weft, "y"), 0o755); err != nil {
			t.Fatalf("create y in the weft: %v", err)
		}
		gitkit.MustRun(t, weft, "git", "mv", lymdirs.LymDirName, "y/"+lymdirs.LymDirName)

		if err := ensureAnchorOf(t, h, "beam", "y"); err != nil {
			t.Fatalf("EnsureAnchor(y) error = %v; want nil", err)
		}

		requireWeftAtY(t, h, "beam")
	})

	t.Run("A kill after the move commit goes on to the wiring at y", func(t *testing.T) {
		t.Parallel()
		h := newAtAnchorHub(t)
		hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Anchor: "y"})
		writeMoveRecordByHand(t, h, "beam", ".", "y")

		if err := ensureAnchorOf(t, h, "beam", "y"); err != nil {
			t.Fatalf("EnsureAnchor(y) error = %v; want nil", err)
		}

		requireWeftAtY(t, h, "beam")
	})
}
