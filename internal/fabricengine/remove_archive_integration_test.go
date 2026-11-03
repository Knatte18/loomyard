// remove_archive_integration_test.go covers Remove's archive step: after every refusal and before any mutation it tags the pair's weft tip under archive/<slug>/ and pushes the tag to the weft origin, so the run records committed on that branch outlive the branch itself.
// It covers the tag's landing, its reuse when the same-tip tag is already on the origin, the commit-then-archive flow for a weft worktree holding uncommitted records, the fail-closed shape of an unreachable origin, the fact that neither force nor remote=false skips the step, the no-origin skip, and that a rolled-back Add never archives.
//
// Every hub is built through hubforge.NewHub, with the hub's RecordsBare as the weft origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// archiveTagsAt lists the archive/ tags present in the repo at repoRoot.
func archiveTagsAt(t *testing.T, repoRoot string) []string {
	t.Helper()

	out, err := gitexec.Run([]string{"tag", "--list", "archive/*"}, repoRoot)
	if err != nil {
		t.Fatalf("list archive tags in %s: %v", repoRoot, err)
	}
	return strings.Fields(out)
}

// TestRemove_ArchivesWeftTipBeforeTeardown covers the happy path: the tag on the origin points at the tip carrying the committed _lym file, and the result names it.
func TestRemove_ArchivesWeftTipBeforeTeardown(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-happy"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	tip := gitkit.CommitFile(t, fabricengine.RecordsWorktreePath(l, slug), "_lym/record.txt", "run record\n", "record")

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if res.ArchiveSkippedReason != "" {
		t.Errorf("ArchiveSkippedReason = %q; want empty", res.ArchiveSkippedReason)
	}
	if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
		t.Errorf("origin tag %s points at %q; want the tip %s", wantTag, got, tip)
	}
}

// TestRemove_ReusesSameTipArchiveTag covers archiveWeftTip's reuse path: an archive tag for the same tip already on the origin, as a teardown that failed after the archive leaves it, is reused rather than pushed again.
func TestRemove_ReusesSameTipArchiveTag(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-reuse"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	tip := gitkit.CommitFile(t, fabricengine.RecordsWorktreePath(l, slug), "_lym/record.txt", "run record\n", "record")

	// Plant the state an earlier archive leaves: a lightweight tag at the tip, as archiveWeftTip itself creates, pushed to the origin.
	wantTag := "archive/" + slug + "/" + tip[:12]
	gitkit.MustRun(t, weftRoot, "git", "tag", wantTag, tip)
	gitkit.MustRun(t, weftRoot, "git", "push", "origin", "refs/tags/"+wantTag)

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove error = %v", err)
	}
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want the reused %q", res.ArchiveTag, wantTag)
	}
	if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
		t.Errorf("origin tag %s points at %q; want %s", wantTag, got, tip)
	}
	if tags := archiveTagsAt(t, h.RecordsBare); len(tags) != 1 || tags[0] != wantTag {
		t.Errorf("origin archive tags = %v; want exactly [%s]", tags, wantTag)
	}
}

// TestRemove_PendingRecordsAreCommittedAndArchivedOnce covers the pre-archive record commit: a single no-force Remove of a pair whose weft worktree holds an uncommitted record commits it and archives the post-commit tip, once.
func TestRemove_PendingRecordsAreCommittedAndArchivedOnce(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-weft-dirty"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	weftWorktree := fabricengine.RecordsWorktreePath(l, slug)
	dir := filepath.Join(weftWorktree, "_lym")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "record.txt"), []byte("run record\n"), 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}
	tipBefore := gitkit.RevParse(t, weftWorktree, "HEAD")

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove on a weft worktree holding an uncommitted record error = %v; want it committed and archived", err)
	}
	tip := tagTargetAt(t, h.RecordsBare, res.ArchiveTag)
	if tip == tipBefore {
		t.Fatalf("archived tip %s equals the pre-Remove tip; want a new commit carrying the record", tip)
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if tags := archiveTagsAt(t, h.RecordsBare); len(tags) != 1 || tags[0] != wantTag {
		t.Errorf("origin archive tags = %v; want exactly [%s]", tags, wantTag)
	}
	if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
		t.Errorf("origin tag %s points at %q; want the post-commit tip %s", wantTag, got, tip)
	}
	if got := showAtTag(t, h.RecordsBare, wantTag, "_lym/record.txt"); got != "run record\n" {
		t.Errorf("archived _lym/record.txt = %q; want the committed record", got)
	}
	if tags := archiveTagsAt(t, weftRoot); len(tags) != 1 {
		t.Errorf("local archive tags = %v; want exactly one", tags)
	}
}

// TestRemove_UnreachableOriginFailsClosed covers an unreachable origin: Remove errors and leaves both worktrees, the launchers and both branches in place.
func TestRemove_UnreachableOriginFailsClosed(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-unreachable"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	mustBreakOrigin(t, weftRoot)

	if _, err := topology.Remove(l, slug, false, false); err == nil {
		t.Fatalf("Remove with an unreachable origin = nil error; want the archive failure")
	}

	for _, p := range []string{
		fabricengine.WorktreePath(l, slug),
		fabricengine.RecordsWorktreePath(l, slug),
		fabricengine.LauncherDir(l, slug),
	} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s missing after a failed archive: %v", p, err)
		}
	}
	if !gitkit.BranchExists(t, weftRoot, fabricengine.RecordsBranchName(slug)) {
		t.Errorf("weft branch gone after a failed archive")
	}
	if !gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("warp branch gone after a failed archive")
	}
}

// TestRemove_ForceStillArchives covers force: it answers dirtiness only, so the archive still runs.
func TestRemove_ForceStillArchives(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-force"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	tip := gitkit.CommitFile(t, fabricengine.RecordsWorktreePath(l, slug), "_lym/record.txt", "run record\n", "record")

	res, err := topology.Remove(l, slug, true, false)
	if err != nil {
		t.Fatalf("Remove(force) error = %v", err)
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
		t.Errorf("origin tag %s points at %q; want %s", wantTag, got, tip)
	}
}

// TestRemove_RemoteFalseStillPushesArchiveTag covers remote=false: the branch deletion stays local, but the archive tag is still pushed.
func TestRemove_RemoteFalseStillPushesArchiveTag(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-noremote"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	tip := gitkit.CommitFile(t, fabricengine.RecordsWorktreePath(l, slug), "_lym/record.txt", "run record\n", "record")

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(remote=false) error = %v", err)
	}
	if res.RemoteBranchDeleted {
		t.Errorf("RemoteBranchDeleted = true; want false")
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
		t.Errorf("origin tag %s points at %q; want %s", wantTag, got, tip)
	}
}

// TestRemove_NoOriginSkipsArchiveAndCompletes covers a weft repo with no origin: the removal completes and the result carries the skip reason.
func TestRemove_NoOriginSkipsArchiveAndCompletes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-archive-noorigin"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	mustRemoveOrigin(t, weftRoot)

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove without an origin error = %v; want completion", err)
	}
	if res.ArchiveTag != "" || res.ArchiveSkippedReason == "" {
		t.Errorf("archive = (%q, %q); want no tag and a skip reason", res.ArchiveTag, res.ArchiveSkippedReason)
	}
	if _, err := os.Stat(fabricengine.RecordsWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree still present after Remove")
	}
}

// installUpdateHook installs an update hook on the bare repository at bare whose script is body, over the hook's $1 (ref), $2 (old tip) and $3 (new tip).
func installUpdateHook(t *testing.T, bare, body string) {
	t.Helper()

	script := "#!/bin/sh\n" + body + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bare, "hooks", "update"), []byte(script), 0o755); err != nil {
		t.Fatalf("install the update hook on %s: %v", bare, err)
	}
}

// TestRemove_ArchiveOrder covers the order and the extent of Remove's archive: the weft branch goes to origin before the tag that names its tip, a diverged or ahead origin tip gets a tag of its own, a retryable tag refusal is retried and a refused branch push never fails the removal.
// One hub per row, each topology's archive sleep shortened.
// The retry row sets a package-level hook, so neither the test nor its rows are parallel.
func TestRemove_ArchiveOrder(t *testing.T) {
	tiergate.Require(t, 1)

	newPairHub := func(t *testing.T, slug string) *hubforge.Hub {
		t.Helper()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		fabricengine.SetArchiveSleepForTest(h.Topology, func(time.Duration) {})
		return h
	}

	t.Run("BranchIsPushedBeforeTheTagThatNamesIt", func(t *testing.T) {
		const slug = "order-branch-first"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		warpBranchCommit(t, h.Location, slug, "unpushed-code.txt")
		codeTipBefore := gitkit.RevParse(t, h.CodeBare, slug)
		tip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/record.txt", "run record\n", "record")
		installUpdateHook(t, h.RecordsBare, "case \"$1\" in\nrefs/tags/archive/*)\n  git merge-base --is-ancestor \"$3\" refs/heads/"+weftBranch+" || { echo \"archive tag target is not on the weft branch\" >&2; exit 1; }\n  ;;\nesac")

		res, err := h.Topology.Remove(h.Location, slug, false, false)
		if err != nil {
			t.Fatalf("Remove error = %v; want the tag accepted after the branch push", err)
		}
		wantTag := wantArchiveTag(slug, tip)
		if res.ArchiveTag != wantTag || res.OriginArchiveTag != "" {
			t.Errorf("tags = (%q, %q); want (%q, none)", res.ArchiveTag, res.OriginArchiveTag, wantTag)
		}
		if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
			t.Errorf("origin tag %s points at %q; want the tip %s", wantTag, got, tip)
		}
		if got := gitkit.RevParse(t, h.CodeBare, slug); got != codeTipBefore {
			t.Errorf("origin task branch = %s; want unchanged %s, the code branch is not pushed", got, codeTipBefore)
		}
	})

	t.Run("DivergedOriginTipGetsATagOfItsOwn", func(t *testing.T) {
		const slug = "order-diverged"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		localTip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/local.txt", "local\n", "local records")
		pushCommitToOrigin(t, h.RecordsBare, weftBranch)
		originTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)

		res, err := h.Topology.Remove(h.Location, slug, false, false)
		if err != nil {
			t.Fatalf("Remove error = %v", err)
		}
		wantLocal, wantOrigin := wantArchiveTag(slug, localTip), wantArchiveTag(slug, originTip)
		if res.ArchiveTag != wantLocal || res.OriginArchiveTag != wantOrigin {
			t.Errorf("tags = (%q, %q); want (%q, %q)", res.ArchiveTag, res.OriginArchiveTag, wantLocal, wantOrigin)
		}
		for tag, want := range map[string]string{wantLocal: localTip, wantOrigin: originTip} {
			if got := tagTargetAt(t, h.RecordsBare, tag); got != want {
				t.Errorf("origin tag %s points at %q; want %s", tag, got, want)
			}
		}
		if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != originTip {
			t.Errorf("origin weft branch = %s; want the diverged branch kept at %s", got, originTip)
		}

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		freshTip := gitkit.RevParse(t, h.PairRecordsSibling(slug), "HEAD")
		if freshTip == localTip || freshTip == originTip {
			t.Errorf("re-added weft tip = %s; want a fresh pair, not an adoption of an archived tip", freshTip)
		}
	})

	t.Run("AheadOriginTipGetsATagAndNoBranchIsPushed", func(t *testing.T) {
		const slug = "order-ahead"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		localTip := gitkit.RevParse(t, h.PairRecordsSibling(slug), "HEAD")
		pushCommitToOrigin(t, h.RecordsBare, weftBranch)
		originTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)

		res, err := h.Topology.Remove(h.Location, slug, false, false)
		if err != nil {
			t.Fatalf("Remove error = %v", err)
		}
		wantLocal, wantOrigin := wantArchiveTag(slug, localTip), wantArchiveTag(slug, originTip)
		if res.ArchiveTag != wantLocal || res.OriginArchiveTag != wantOrigin {
			t.Errorf("tags = (%q, %q); want (%q, %q)", res.ArchiveTag, res.OriginArchiveTag, wantLocal, wantOrigin)
		}
		if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != originTip {
			t.Errorf("origin weft branch = %s; want it left at %s", got, originTip)
		}
	})

	t.Run("RefusedTagPushIsRetriedOnMissingObjects", func(t *testing.T) {
		const slug = "order-retry"
		h := newPairHub(t, slug)
		tip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/record.txt", "run record\n", "record")
		attempts := 0
		fabricengine.SetArchiveTagPushHookForTest(t, func(tagRef string) error {
			attempts++
			if attempts > 1 {
				return nil
			}
			return &gitexec.GitError{Args: []string{"push"}, ExitCode: 1, Stderr: " ! [remote rejected] " + tagRef + " -> " + tagRef + " (missing necessary objects)\n"}
		})

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove error = %v; want the refused attempt retried", err)
		}
		wantTag := wantArchiveTag(slug, tip)
		if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
			t.Errorf("origin tag %s points at %q; want %s", wantTag, got, tip)
		}
		if attempts != 2 {
			t.Errorf("tag push attempts = %d; want the refused one and its retry", attempts)
		}
	})

	t.Run("RefusedBranchPushNeverFailsTheRemoval", func(t *testing.T) {
		const slug = "order-branch-refused"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		tip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/record.txt", "run record\n", "record")
		installUpdateHook(t, h.RecordsBare, "case \"$1\" in\nrefs/heads/"+weftBranch+")\n  echo \"weft branch updates refused\" >&2\n  exit 1\n  ;;\nesac")

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove error = %v; want a refused branch push only logged", err)
		}
		wantTag := wantArchiveTag(slug, tip)
		if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
			t.Errorf("origin tag %s points at %q; want %s", wantTag, got, tip)
		}
	})
}

// TestAddRollback_LeavesNoArchiveTag covers a rolled-back Add: it removes the weft branch it just created without archiving, so no archive/ tag appears locally or on the origin.
func TestAddRollback_LeavesNoArchiveTag(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "add-rollback-no-archive"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	// A blocker directory at the ide launcher script path fails Add after its weft branch exists, triggering rollbackAdd.
	plantLauncherBlocker(t, l, slug)

	topology := h.Topology
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
		t.Fatalf("Add should have failed (launcher blocker)")
	}

	if tags := archiveTagsAt(t, weftRoot); len(tags) != 0 {
		t.Errorf("local archive tags after a rolled-back Add = %v; want none", tags)
	}
	if tags := archiveTagsAt(t, h.RecordsBare); len(tags) != 0 {
		t.Errorf("origin archive tags after a rolled-back Add = %v; want none", tags)
	}
}
