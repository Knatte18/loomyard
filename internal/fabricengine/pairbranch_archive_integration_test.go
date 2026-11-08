// pairbranch_archive_integration_test.go covers Cleanup's archive step for orphan pair weft branches.
// The step tags each branch tip under archive/<slug>/ and pushes the tag to the weft origin.
// Only then is the branch deleted.
//
// Every hub is built through hubforge.NewHub, with the hub's RecordsBare as the weft origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantArchiveTag returns the archive tag name archiveWeftTip derives for slug and tip.
func wantArchiveTag(slug, tip string) string {
	return "archive/" + slug + "/" + tip[:12]
}

// TestCleanup_ApplyArchivesEachOrphanBeforeDeleting covers Cleanup's apply loop:
// every orphan weft branch is tagged on the origin at its tip and then deleted.
// An orphan whose origin tip diverges from the local one gets a tag at each tip;
// the sweep pushes no branch, and the origin branch is deleted under a lease on the tip the archive read.
// A branch origin moved between that read and the delete stays on origin and is reported kept.
// A slug whose teardown lock is held is skipped whole; a pair a worktree turned live under the lock gets no entry,
// and a lock directory that cannot be made keeps the branch with an entry error.
// The moved row sets a package-level hook, so neither the test nor that row is parallel.
func TestCleanup_ApplyArchivesEachOrphanBeforeDeleting(t *testing.T) {
	tiergate.Require(t, 1)

	t.Run("TagsEveryOrphanAndDeletesItOnOriginToo", func(t *testing.T) {
		t.Parallel()

		slugs := []string{"cleanup-archive-a", "cleanup-archive-b"}
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		weftRoot := mustRecordsRepoRoot(t, l)
		tips := make(map[string]string, len(slugs))
		for _, slug := range slugs {
			branch := fabricengine.RecordsBranchName(slug)
			mustCreateOrphanWeftBranch(t, weftRoot, branch)
			tips[slug] = gitkit.RevParse(t, weftRoot, branch)
		}

		// A diverged orphan: origin's weft tip and the local one each hold a commit the other lacks.
		const divergedSlug = "cleanup-archive-diverged"
		divergedBranch := fabricengine.RecordsBranchName(divergedSlug)
		mustCreateOrphanWeftBranch(t, weftRoot, divergedBranch)
		mustPushBranch(t, weftRoot, divergedBranch)
		pushCommitToOrigin(t, h.RecordsBare, divergedBranch)
		originDivergedTip := gitkit.RevParse(t, h.RecordsBare, divergedBranch)
		localDivergedTip, err := gitexec.Run([]string{"commit-tree", "-m", "local-only", "-p", "HEAD", "HEAD^{tree}"}, weftRoot)
		if err != nil {
			t.Fatalf("commit-tree: %v", err)
		}
		localDivergedTip = strings.TrimSpace(localDivergedTip)
		gitkit.MustRun(t, weftRoot, "git", "branch", "-f", divergedBranch, localDivergedTip)

		res, err := h.Topology.Cleanup(l, true, false, true)
		if err != nil {
			t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
		}

		diverged := findCleanupEntry(t, res.Entries, divergedBranch)
		wantLocalTag, wantOriginTag := wantArchiveTag(divergedSlug, localDivergedTip), wantArchiveTag(divergedSlug, originDivergedTip)
		if !diverged.Deleted || diverged.ArchiveTag != wantLocalTag || diverged.OriginArchiveTag != wantOriginTag {
			t.Errorf("diverged entry = %+v; want Deleted with ArchiveTag %q and OriginArchiveTag %q", *diverged, wantLocalTag, wantOriginTag)
		}
		if !diverged.RemoteDeleted || diverged.RemoteError != "" || diverged.RemoteKeptReason != "" {
			t.Errorf("diverged entry = %+v; want its origin branch deleted under the lease on the tip the archive read", *diverged)
		}
		for tag, want := range map[string]string{wantLocalTag: localDivergedTip, wantOriginTag: originDivergedTip} {
			if got := tagTargetAt(t, h.RecordsBare, tag); got != want {
				t.Errorf("origin tag %s = %q; want %s", tag, got, want)
			}
		}
		if gitkit.BranchExists(t, h.RecordsBare, divergedBranch) {
			t.Errorf("origin branch %s still present; want it deleted, the sweep pushed no branch", divergedBranch)
		}

		for _, slug := range slugs {
			branch := fabricengine.RecordsBranchName(slug)
			wantTag := wantArchiveTag(slug, tips[slug])
			entry := findCleanupEntry(t, res.Entries, branch)
			if !entry.Deleted || entry.ArchiveTag != wantTag {
				t.Errorf("entry %s = %+v; want Deleted with ArchiveTag %q", branch, *entry, wantTag)
			}
			if entry.RemoteDeleted || entry.RemoteError != "" || entry.RemoteKeptReason != "" {
				t.Errorf("entry %s = %+v; want no remote step for a branch origin never held", branch, *entry)
			}
			if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tips[slug] {
				t.Errorf("origin tag %s = %q; want %s", wantTag, got, tips[slug])
			}
			if gitkit.BranchExists(t, weftRoot, branch) {
				t.Errorf("branch %q still present after Cleanup", branch)
			}
		}
		if res.ArchiveSkippedReason != "" {
			t.Errorf("ArchiveSkippedReason = %q; want empty with an origin configured", res.ArchiveSkippedReason)
		}
	})

	t.Run("OriginMovedBeforeTheDeleteKeepsItsBranch", func(t *testing.T) {
		const slug = "cleanup-archive-moved"
		branch := fabricengine.RecordsBranchName(slug)
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		weftRoot := mustRecordsRepoRoot(t, h.Location)
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		mustPushBranch(t, weftRoot, branch)
		var movedTip string
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			pushCommitToOrigin(t, h.RecordsBare, branch)
			movedTip = gitkit.RevParse(t, h.RecordsBare, branch)
		})

		res, err := h.Topology.Cleanup(h.Location, true, false, true)
		if err != nil {
			t.Fatalf("Cleanup(apply=true, remote=true) error = %v; want the kept branch on the entry only", err)
		}

		entry := findCleanupEntry(t, res.Entries, branch)
		if !entry.Deleted || entry.RemoteDeleted || entry.RemoteError != "" || !strings.Contains(entry.RemoteKeptReason, branch) {
			t.Errorf("entry = %+v; want Deleted locally, RemoteKeptReason naming %q and no RemoteError", *entry, branch)
		}
		if got := gitkit.RevParse(t, h.RecordsBare, branch); got != movedTip {
			t.Errorf("origin branch %s = %s; want it kept at the moved tip %s", branch, got, movedTip)
		}
	})

	t.Run("SkipsASlugWhoseTeardownLockIsHeldAndSweepsTheNext", func(t *testing.T) {
		t.Parallel()

		const heldSlug, freeSlug = "cleanup-lock-held", "cleanup-lock-free"
		heldBranch, freeBranch := fabricengine.RecordsBranchName(heldSlug), fabricengine.RecordsBranchName(freeSlug)
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		weftRoot := mustRecordsRepoRoot(t, h.Location)
		for _, branch := range []string{heldBranch, freeBranch} {
			mustCreateOrphanWeftBranch(t, weftRoot, branch)
			mustPushBranch(t, weftRoot, branch)
		}
		holdTeardownLock(t, h, heldSlug)

		res, err := h.Topology.Cleanup(h.Location, true, false, true)
		if err != nil {
			t.Fatalf("Cleanup(apply=true, remote=true) error = %v; want a held lock on the entry only", err)
		}

		held := findCleanupEntry(t, res.Entries, heldBranch)
		if !held.Skipped || !strings.Contains(held.SkippedReason, "teardown lock") || held.Deleted || held.Error != "" || held.ArchiveTag != "" {
			t.Errorf("held entry = %+v; want Skipped with a reason naming the teardown lock, nothing archived or deleted and no Error", *held)
		}
		if !gitkit.BranchExists(t, weftRoot, heldBranch) || !gitkit.BranchExists(t, h.RecordsBare, heldBranch) {
			t.Errorf("branch %q gone locally or on origin; want the skipped slug kept whole", heldBranch)
		}
		for _, tag := range archiveTagsAt(t, h.RecordsBare) {
			if strings.HasPrefix(tag, "archive/"+heldSlug+"/") {
				t.Errorf("origin archive tag %s of the skipped slug; want none", tag)
			}
		}
		free := findCleanupEntry(t, res.Entries, freeBranch)
		if free.Skipped || !free.Deleted || !free.RemoteDeleted || free.ArchiveTag == "" {
			t.Errorf("free entry = %+v; want archived and deleted locally and on origin", *free)
		}
	})

	t.Run("SkipsAnOrphanWhosePairTurnedLiveUnderTheLock", func(t *testing.T) {
		t.Parallel()

		const slug = "cleanup-turned-live"
		branch := fabricengine.RecordsBranchName(slug)
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		weftRoot := mustRecordsRepoRoot(t, h.Location)
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		mustPushBranch(t, weftRoot, branch)
		fabricengine.SetCleanupAfterEnumerateForTest(h.Topology, func() {
			gitkit.Git(t, h.PrimeWorktree(), "worktree", "add", "-b", slug, filepath.Join(t.TempDir(), "live"))
		})

		res, err := h.Topology.Cleanup(h.Location, true, false, true)
		if err != nil {
			t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
		}

		for _, entry := range res.Entries {
			if entry.Branch == branch {
				t.Errorf("entry = %+v; want none for an orphan found live under the lock", entry)
			}
		}
		if !gitkit.BranchExists(t, weftRoot, branch) || !gitkit.BranchExists(t, h.RecordsBare, branch) {
			t.Errorf("branch %q gone locally or on origin; want the live pair's branch kept", branch)
		}
		if tags := archiveTagsAt(t, h.RecordsBare); len(tags) != 0 {
			t.Errorf("origin archive tags = %v; want none for a pair found live", tags)
		}
	})

	t.Run("KeepsTheBranchWhenTheLockDirectoryCannotBeMade", func(t *testing.T) {
		t.Parallel()

		const slug = "cleanup-lock-dir-blocked"
		branch := fabricengine.RecordsBranchName(slug)
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		weftRoot := mustRecordsRepoRoot(t, h.Location)
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		mustPushBranch(t, weftRoot, branch)
		scratch := fabricengine.HubScratchDir(h.Path)
		if err := os.MkdirAll(scratch, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", scratch, err)
		}
		if err := os.WriteFile(filepath.Join(scratch, "teardown"), []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("plant a file at the teardown lock directory: %v", err)
		}

		res, err := h.Topology.Cleanup(h.Location, true, false, true)
		if err != nil {
			t.Fatalf("Cleanup(apply=true, remote=true) error = %v; want the lock error on the entry only", err)
		}

		entry := findCleanupEntry(t, res.Entries, branch)
		if entry.Error == "" || entry.Skipped || entry.Deleted {
			t.Errorf("entry = %+v; want Error set, not Skipped and not Deleted", *entry)
		}
		if !gitkit.BranchExists(t, weftRoot, branch) || !gitkit.BranchExists(t, h.RecordsBare, branch) {
			t.Errorf("branch %q gone locally or on origin; want it kept", branch)
		}
	})
}

// TestCleanup_ArchiveFailureKeepsTheLocalTagAndContinuesSweep covers a failed remote archive:
// the branch was deleted locally under its local archive tag and the entry carries the error.
// Cleanup still returns no error.
func TestCleanup_ArchiveFailureKeepsTheLocalTagAndContinuesSweep(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-archive-fail-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := gitkit.RevParse(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	res, err := h.Topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true) error = %v; want the failure on the entry only", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if entry.Error == "" || !entry.Deleted {
		t.Errorf("entry = %+v; want Error set and Deleted true", *entry)
	}
	slug, _ := fabricengine.WeftWarpSlug(branch)
	if got := gitkit.RevParse(t, weftRoot, "refs/tags/"+fabricengine.ArchiveTagName(slug, tip)); got != tip {
		t.Errorf("local archive tag points at %s; want the orphan's tip %s kept", got, tip)
	}
}

// TestCleanup_NoOriginDeletesAndReportsArchiveSkip covers a weft repo with no origin:
// the orphan is deleted and the result reports the skipped archive once.
//
//testtiming:keep no archive tag on the entry and a reported skip reason when the weft repo has no origin
func TestCleanup_NoOriginDeletesAndReportsArchiveSkip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-archive-noorigin-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustRemoveOrigin(t, weftRoot)

	res, err := h.Topology.Cleanup(l, true, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Deleted || entry.Error != "" || entry.ArchiveTag != "" {
		t.Errorf("entry = %+v; want Deleted, no Error, no ArchiveTag", *entry)
	}
	if res.ArchiveSkippedReason == "" {
		t.Error("ArchiveSkippedReason is empty; want the no-origin skip reported")
	}
	if gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q still present after Cleanup", branch)
	}
}
