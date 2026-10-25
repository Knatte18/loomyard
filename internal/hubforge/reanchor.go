// reanchor.go builds hubforge's second-anchor fixture: ReanchorPair moves one worktree of a built hub to another anchor.

package hubforge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ReanchorPair moves the worktree called slug, the prime included, from its current anchor to anchor, fataling on error.
// In the worktree's records worktree it moves each wired name's tracked content with `git mv`, writes anchor into the root marker, and commits both in one commit.
// In its code worktree it commits a tracked placeholder `<anchor>/.keep` when git does not already track anchor there, so anchor is a directory git tracks.
// It then unwires the worktree's junctions at the old anchor and wires them at anchor, so the hub holds two anchors.
// For the prime it also updates h.Location, h.Anchor and h.WeftBase to the new anchor.
func ReanchorPair(tb testing.TB, h *Hub, slug, anchor string) {
	tb.Helper()

	defer markFixtureBuild()()

	oldLoc, err := lyxcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		tb.Fatalf("ReanchorPair(%s): read the current anchor: %v", slug, err)
	}
	if oldLoc.AnchorRel == anchor {
		return
	}
	names, err := fabricengine.RepoWiredNames(oldLoc)
	if err != nil {
		tb.Fatalf("ReanchorPair(%s): load the wired names: %v", slug, err)
	}
	if _, err := fabricengine.UnwireJunctions(oldLoc, slug, names); err != nil {
		tb.Fatalf("ReanchorPair(%s): unwire the junctions at %q: %v", slug, oldLoc.AnchorRel, err)
	}

	weft := fabricengine.RecordsWorktreePath(oldLoc, slug)
	if err := os.MkdirAll(filepath.Join(weft, anchor), 0o755); err != nil {
		tb.Fatalf("ReanchorPair(%s): create the new anchor in the records worktree: %v", slug, err)
	}
	for _, name := range names {
		from := filepath.ToSlash(filepath.Join(oldLoc.AnchorRel, name))
		if strings.TrimSpace(gitkit.Git(tb, weft, "ls-files", "--", from)) == "" {
			continue
		}
		gitkit.MustRun(tb, weft, "git", "mv", from, filepath.ToSlash(filepath.Join(anchor, name)))
	}
	marker := filepath.Join(weft, lyxcwd.AnchorFileName)
	if err := os.WriteFile(marker, []byte(anchor+"\n"), 0o644); err != nil {
		tb.Fatalf("ReanchorPair(%s): write the marker: %v", slug, err)
	}
	gitkit.MustRun(tb, weft, "git", "add", "--", lyxcwd.AnchorFileName)
	gitkit.MustRun(tb, weft, "git", "commit", "--allow-empty", "-m", "hubforge: move "+slug+" to anchor "+anchor)

	code := fabricengine.WorktreePath(oldLoc, slug)
	if anchor != "." && strings.TrimSpace(gitkit.Git(tb, code, "ls-files", "--", filepath.ToSlash(anchor))) == "" {
		keep := filepath.Join(code, anchor, ".keep")
		if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
			tb.Fatalf("ReanchorPair(%s): create the anchor in the code worktree: %v", slug, err)
		}
		if err := os.WriteFile(keep, nil, 0o644); err != nil {
			tb.Fatalf("ReanchorPair(%s): write the anchor placeholder: %v", slug, err)
		}
		gitkit.MustRun(tb, code, "git", "add", "--", filepath.ToSlash(filepath.Join(anchor, ".keep")))
		gitkit.MustRun(tb, code, "git", "commit", "-m", "hubforge: track anchor "+anchor)
	}

	newLoc, err := lyxcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		tb.Fatalf("ReanchorPair(%s): read the new anchor: %v", slug, err)
	}
	if err := fabricengine.WireJunctions(newLoc, slug, names); err != nil {
		tb.Fatalf("ReanchorPair(%s): wire the junctions at %q: %v", slug, anchor, err)
	}

	if slug == h.Location.WorktreeName {
		h.Location = newLoc
		h.Anchor = anchor
		h.WeftBase = filepath.Join(weft, anchor)
	}
}
