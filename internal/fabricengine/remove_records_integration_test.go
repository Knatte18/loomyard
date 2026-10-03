//go:build integration

// remove_records_integration_test.go covers Remove's pre-archive record commit and its read-only refusal probe.
// An uncommitted run record inside the record pathspec is committed and reaches the pushed archive tag;
// dirt outside the pathspec still refuses without force, before any mutation, and proceeds with force;
// and RemoveRefusal alone changes nothing for any refusal kind.
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
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// writeFile creates the parent directories of path and writes content to it.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// showAtTag returns the content of rel in the tree tag points at in the repo at repoRoot.
func showAtTag(t *testing.T, repoRoot, tag, rel string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"show", "refs/tags/" + tag + ":" + rel}, repoRoot)
	if err != nil {
		t.Fatalf("show %s:%s in %s: %v", tag, rel, repoRoot, err)
	}
	return out
}

// TestRemove_CommitsPendingRecordsIntoArchiveTag leaves an untracked stop report under the pair's drive-reports directory and asserts a no-force Remove commits it, with the Warp-SHA trailer, and that the pushed archive tag contains it.
func TestRemove_CommitsPendingRecordsIntoArchiveTag(t *testing.T) {
	t.Parallel()

	const slug = "records-commit"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	weftWorktree := fabricengine.WeftWorktreePath(l, slug)
	rel := "_lyx/shed/" + slug + "/drive-reports/stop.md"
	writeFile(t, filepath.Join(weftWorktree, filepath.FromSlash(rel)), "stopped\n")
	warpTip := gitkit.RevParse(t, fabricengine.WorktreePath(l, slug), "HEAD")

	res, err := h.Topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove error = %v", err)
	}
	if res.ArchiveTag == "" {
		t.Fatalf("ArchiveTag is empty; want the archive tag covering the committed record")
	}
	if got := showAtTag(t, h.WeftBare, res.ArchiveTag, rel); got != "stopped\n" {
		t.Errorf("archived %s = %q; want the committed report", rel, got)
	}
	tagTip := tagTargetAt(t, h.WeftBare, res.ArchiveTag)
	msg, err := gitexec.Run([]string{"log", "-1", "--format=%B", tagTip}, h.WeftBare)
	if err != nil {
		t.Fatalf("read the archived tip's message: %v", err)
	}
	if !strings.Contains(msg, fabricengine.DefaultCommitMessage) || !strings.Contains(msg, fabricengine.WarpSHATrailerKey+": "+warpTip) {
		t.Errorf("archived tip message = %q; want %q with a Warp-SHA trailer for %s", msg, fabricengine.DefaultCommitMessage, warpTip)
	}
	mutations := res.Mutated()
	if got := countKind(&mutations, fabricengine.KindCommitCreated); got != 1 {
		t.Errorf("commit_created mutations = %d; want 1", got)
	}
}

// TestRemove_OutOfPathspecSiblingDirtRefusesBeforeAnyMutation puts a stray file outside the record pathspec in the sibling worktree and asserts a no-force Remove refuses with no tag, no commit and an unchanged sibling tip, while force proceeds.
func TestRemove_OutOfPathspecSiblingDirtRefusesBeforeAnyMutation(t *testing.T) {
	t.Parallel()

	const slug = "records-stray"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	weftWorktree := fabricengine.WeftWorktreePath(l, slug)
	writeFile(t, filepath.Join(weftWorktree, "stray.txt"), "not a record\n")
	writeFile(t, filepath.Join(weftWorktree, "_lyx", "record.txt"), "a record\n")
	tipBefore := gitkit.RevParse(t, weftWorktree, "HEAD")

	res, err := h.Topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove(force=false) with out-of-pathspec sibling dirt = nil error; want the refusal")
	}
	if !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("error = %v; want it to wrap ErrPairSiblingDirty", err)
	}
	if !strings.Contains(err.Error(), "uncommitted changes") || !strings.Contains(err.Error(), "stray.txt") {
		t.Errorf("error = %q; want it to name the uncommitted out-of-pathspec path", err.Error())
	}
	if strings.Contains(err.Error(), "_lyx/record.txt") {
		t.Errorf("error = %q; the in-pathspec record must not be named as dirt", err.Error())
	}
	if n := res.Mutated().Len(); n != 0 {
		t.Errorf("refused Remove recorded %d mutations; want 0", n)
	}
	if tags := archiveTagsAt(t, h.WeftBare); len(tags) != 0 {
		t.Errorf("origin archive tags after a refused Remove = %v; want none", tags)
	}
	if tipAfter := gitkit.RevParse(t, weftRoot, fabricengine.WeftBranchName(slug)); tipAfter != tipBefore {
		t.Errorf("sibling branch tip = %s after a refused Remove; want unchanged %s", tipAfter, tipBefore)
	}

	forced, err := h.Topology.Remove(l, slug, true, false)
	if err != nil {
		t.Fatalf("Remove(force=true) error = %v", err)
	}
	if forced.ArchiveTag == "" {
		t.Errorf("forced Remove ArchiveTag is empty; want the archive to run")
	}
	if got := showAtTag(t, h.WeftBare, forced.ArchiveTag, "_lyx/record.txt"); got != "a record\n" {
		t.Errorf("forced Remove archived _lyx/record.txt = %q; want the committed record", got)
	}
}

// TestRemoveRefusal_ProbeLeavesPairUntouched asks RemoveRefusal for each refusal kind and asserts it names the refusal, returns nil for a clean pair, and leaves the pair, both branches and the weft origin untouched.
func TestRemoveRefusal_ProbeLeavesPairUntouched(t *testing.T) {
	t.Parallel()

	const slug = "records-probe"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	warpPath := fabricengine.WorktreePath(l, slug)
	weftWorktree := fabricengine.WeftWorktreePath(l, slug)

	assertUntouched := func(t *testing.T) {
		t.Helper()

		if tags := archiveTagsAt(t, h.WeftBare); len(tags) != 0 {
			t.Errorf("origin archive tags after RemoveRefusal = %v; want none", tags)
		}
		if tags := archiveTagsAt(t, weftRoot); len(tags) != 0 {
			t.Errorf("local archive tags after RemoveRefusal = %v; want none", tags)
		}
		for _, p := range []string{
			fabricengine.PortalLink(l, slug),
			fabricengine.LauncherDir(l, slug),
			warpPath,
			weftWorktree,
		} {
			if _, statErr := os.Lstat(p); statErr != nil {
				t.Errorf("%s missing after RemoveRefusal: %v", p, statErr)
			}
		}
		if !gitkit.BranchExists(t, l.WorktreePath(), slug) {
			t.Errorf("warp branch gone after RemoveRefusal")
		}
		if !gitkit.BranchExists(t, weftRoot, fabricengine.WeftBranchName(slug)) {
			t.Errorf("weft branch gone after RemoveRefusal")
		}
	}

	if err := h.Topology.RemoveRefusal(l, slug, false); err != nil {
		t.Errorf("RemoveRefusal on a clean pair = %v; want nil", err)
	}

	if err := h.Topology.RemoveRefusal(l, "_board", false); err == nil {
		t.Errorf("RemoveRefusal(%q) = nil; want the slug refusal", "_board")
	}

	primeName, err := fabricengine.PrimeName(l)
	if err != nil {
		t.Fatalf("PrimeName: %v", err)
	}
	if err := h.Topology.RemoveRefusal(l, primeName, false); err == nil {
		t.Errorf("RemoveRefusal(%q) = nil; want the prime refusal", primeName)
	}

	if err := h.Topology.RemoveRefusal(l, "no-such-pair", false); err == nil {
		t.Errorf("RemoveRefusal on an absent pair = nil; want the not-found refusal")
	}

	writeFile(t, filepath.Join(weftWorktree, "stray.txt"), "not a record\n")
	if err := h.Topology.RemoveRefusal(l, slug, false); !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("RemoveRefusal with out-of-pathspec sibling dirt = %v; want ErrPairSiblingDirty", err)
	}
	if err := h.Topology.RemoveRefusal(l, slug, true); err != nil {
		t.Errorf("RemoveRefusal(force=true) with sibling dirt = %v; want nil", err)
	}
	if err := os.Remove(filepath.Join(weftWorktree, "stray.txt")); err != nil {
		t.Fatalf("remove stray file: %v", err)
	}

	writeFile(t, filepath.Join(weftWorktree, "_lyx", "record.txt"), "a record\n")
	if err := h.Topology.RemoveRefusal(l, slug, false); err != nil {
		t.Errorf("RemoveRefusal with only an in-pathspec record = %v; want nil", err)
	}
	if _, err := os.Stat(filepath.Join(weftWorktree, "_lyx", "record.txt")); err != nil {
		t.Errorf("the probe disturbed the uncommitted record: %v", err)
	}

	writeFile(t, filepath.Join(warpPath, "task-dirt.txt"), "uncommitted\n")
	if err := h.Topology.RemoveRefusal(l, slug, false); err == nil || errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("RemoveRefusal with task-side dirt = %v; want a non-sibling refusal", err)
	}

	assertUntouched(t)
	if got := gitkit.RevParse(t, weftWorktree, "HEAD"); got != gitkit.RevParse(t, weftRoot, fabricengine.WeftBranchName(slug)) {
		t.Errorf("sibling HEAD and branch disagree after the probe")
	}
}
