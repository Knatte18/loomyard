//go:build integration

// pairbranch_archive_integration_test.go covers the archive step of the two verbs that delete an existing pair's weft branch besides Remove: RemovePairBranch and Cleanup's apply loop each tag the branch tip under archive/<slug>/ and push the tag to the weft origin before deleting the branch.
//
// Every hub is built through hubforge.NewHub, with the hub's WeftBare as the weft origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// wantArchiveTag returns the archive tag name archiveWeftTip derives for slug and tip.
func wantArchiveTag(slug, tip string) string {
	return "archive/" + slug + "/" + tip[:12]
}

// TestRemovePairBranch_ArchivesTipBeforeDeleting covers the happy path: the tag lands on the origin at the branch tip and the branch is then deleted locally and on the origin.
func TestRemovePairBranch_ArchivesTipBeforeDeleting(t *testing.T) {
	t.Parallel()

	const slug = "pair-archive"
	branch := fabricengine.WeftBranchName(slug)
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	tip := gitkit.RevParse(t, weftRoot, branch)

	res, err := h.Topology.RemovePairBranch(l, slug)
	if err != nil {
		t.Fatalf("RemovePairBranch() error = %v", err)
	}

	wantTag := wantArchiveTag(slug, tip)
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if got := tagTargetAt(t, h.WeftBare, wantTag); got != tip {
		t.Errorf("origin tag %s = %q; want the branch tip %s", wantTag, got, tip)
	}
	if !res.LocalDeleted || !res.RemoteDeleted {
		t.Errorf("RemovePairBranch() = %+v; want LocalDeleted and RemoteDeleted", res)
	}
	if gitkit.BranchExists(t, weftRoot, branch) || gitkit.BranchExists(t, h.WeftBare, branch) {
		t.Errorf("branch %q still present locally or on the origin", branch)
	}
}

// TestRemovePairBranch_ArchivesFromOriginWhenOnlyOriginCopyRemains covers the already-gone arm: the local branch is deleted, so the tag is made from the origin's copy.
func TestRemovePairBranch_ArchivesFromOriginWhenOnlyOriginCopyRemains(t *testing.T) {
	t.Parallel()

	const slug = "pair-archive-origin-only"
	branch := fabricengine.WeftBranchName(slug)
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	tip := gitkit.RevParse(t, weftRoot, branch)
	gitkit.MustRun(t, weftRoot, "git", "branch", "-D", branch)

	res, err := h.Topology.RemovePairBranch(l, slug)
	if err != nil {
		t.Fatalf("RemovePairBranch() error = %v", err)
	}

	wantTag := wantArchiveTag(slug, tip)
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if got := tagTargetAt(t, h.WeftBare, wantTag); got != tip {
		t.Errorf("origin tag %s = %q; want the branch tip %s", wantTag, got, tip)
	}
	if !res.RemoteDeleted || gitkit.BranchExists(t, h.WeftBare, branch) {
		t.Errorf("RemovePairBranch() = %+v; want the origin copy deleted", res)
	}
}

// TestRemovePairBranch_UnreachableOriginErrorsAndKeepsBranch covers the fail-closed shape: with the origin unreachable the call returns an error and the branch is still present.
func TestRemovePairBranch_UnreachableOriginErrorsAndKeepsBranch(t *testing.T) {
	t.Parallel()

	const slug = "pair-archive-unreachable"
	branch := fabricengine.WeftBranchName(slug)
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	if _, err := h.Topology.RemovePairBranch(l, slug); err == nil {
		t.Fatal("RemovePairBranch() with an unreachable origin = nil error; want the archive failure")
	}
	if !gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q was deleted despite the failed archive", branch)
	}
}

// TestCleanup_ApplyArchivesEachOrphanBeforeDeleting covers Cleanup's apply loop: every orphan weft branch is tagged on the origin at its tip and then deleted.
func TestCleanup_ApplyArchivesEachOrphanBeforeDeleting(t *testing.T) {
	t.Parallel()

	slugs := []string{"cleanup-archive-a", "cleanup-archive-b"}
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	tips := make(map[string]string, len(slugs))
	for _, slug := range slugs {
		branch := fabricengine.WeftBranchName(slug)
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		tips[slug] = gitkit.RevParse(t, weftRoot, branch)
	}

	res, err := h.Topology.Cleanup(l, true, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true) error = %v", err)
	}

	for _, slug := range slugs {
		branch := fabricengine.WeftBranchName(slug)
		wantTag := wantArchiveTag(slug, tips[slug])
		entry := findCleanupEntry(t, res.Entries, branch)
		if !entry.Deleted || entry.ArchiveTag != wantTag {
			t.Errorf("entry %s = %+v; want Deleted with ArchiveTag %q", branch, *entry, wantTag)
		}
		if got := tagTargetAt(t, h.WeftBare, wantTag); got != tips[slug] {
			t.Errorf("origin tag %s = %q; want %s", wantTag, got, tips[slug])
		}
		if gitkit.BranchExists(t, weftRoot, branch) {
			t.Errorf("branch %q still present after Cleanup", branch)
		}
	}
	if res.ArchiveSkippedReason != "" {
		t.Errorf("ArchiveSkippedReason = %q; want empty with an origin configured", res.ArchiveSkippedReason)
	}
}

// TestCleanup_ArchiveFailureKeepsBranchAndContinuesSweep covers a failed archive: the entry carries the error, the branch stays, and Cleanup itself still returns no error.
func TestCleanup_ArchiveFailureKeepsBranchAndContinuesSweep(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-archive-fail-weft"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	res, err := h.Topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true) error = %v; want the failure on the entry only", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if entry.Error == "" || entry.Deleted {
		t.Errorf("entry = %+v; want Error set and Deleted false", *entry)
	}
	if !gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q was deleted despite the failed archive", branch)
	}
}

// TestCleanup_NoOriginDeletesAndReportsArchiveSkip covers a weft repo with no origin: the orphan is deleted as before and the result reports the skipped archive once.
func TestCleanup_NoOriginDeletesAndReportsArchiveSkip(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-archive-noorigin-weft"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
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
