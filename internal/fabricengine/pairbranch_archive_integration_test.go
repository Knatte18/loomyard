//go:build integration

// pairbranch_archive_integration_test.go covers the archive step of the two verbs that delete an existing pair's weft branch besides Remove: RemovePairBranch and Cleanup's apply loop each tag the branch tip under archive/<slug>/ and push the tag to the weft origin before deleting the branch.
//
// Every hub is built through hubforge.NewHub via newFabricFixture, with the hub's WeftBare as the weft origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// branchTip returns the full SHA of branch in the repo at repoRoot.
func branchTip(t *testing.T, repoRoot, branch string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"rev-parse", "refs/heads/" + branch}, repoRoot)
	if err != nil {
		t.Fatalf("rev-parse %s in %s: %v", branch, repoRoot, err)
	}
	return strings.TrimSpace(out)
}

// wantArchiveTag returns the archive tag name archiveWeftTip derives for slug and tip.
func wantArchiveTag(slug, tip string) string {
	return "archive/" + slug + "/" + tip[:12]
}

// TestRemovePairBranch_ArchivesTipBeforeDeleting covers the happy path: the tag lands on the origin at the branch tip and the branch is then deleted locally and on the origin.
func TestRemovePairBranch_ArchivesTipBeforeDeleting(t *testing.T) {
	t.Parallel()

	const slug = "pair-archive"
	branch := fabricengine.WeftBranchName(slug)
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	tip := branchTip(t, weftRoot, branch)

	res, err := fabricengine.NewTopology(fabricengine.Config{}).RemovePairBranch(l, slug)
	if err != nil {
		t.Fatalf("RemovePairBranch() error = %v", err)
	}

	wantTag := wantArchiveTag(slug, tip)
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if got := tagTargetAt(t, fixture.WeftBare, wantTag); got != tip {
		t.Errorf("origin tag %s = %q; want the branch tip %s", wantTag, got, tip)
	}
	if !res.LocalDeleted || !res.RemoteDeleted {
		t.Errorf("RemovePairBranch() = %+v; want LocalDeleted and RemoteDeleted", res)
	}
	if branchExistsAt(t, weftRoot, branch) || branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("branch %q still present locally or on the origin", branch)
	}
}

// TestRemovePairBranch_ArchivesFromOriginWhenOnlyOriginCopyRemains covers the already-gone arm: the local branch is deleted, so the tag is made from the origin's copy.
func TestRemovePairBranch_ArchivesFromOriginWhenOnlyOriginCopyRemains(t *testing.T) {
	t.Parallel()

	const slug = "pair-archive-origin-only"
	branch := fabricengine.WeftBranchName(slug)
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	tip := branchTip(t, weftRoot, branch)
	gitkit.MustRun(t, weftRoot, "git", "branch", "-D", branch)

	res, err := fabricengine.NewTopology(fabricengine.Config{}).RemovePairBranch(l, slug)
	if err != nil {
		t.Fatalf("RemovePairBranch() error = %v", err)
	}

	wantTag := wantArchiveTag(slug, tip)
	if res.ArchiveTag != wantTag {
		t.Errorf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
	}
	if got := tagTargetAt(t, fixture.WeftBare, wantTag); got != tip {
		t.Errorf("origin tag %s = %q; want the branch tip %s", wantTag, got, tip)
	}
	if !res.RemoteDeleted || branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("RemovePairBranch() = %+v; want the origin copy deleted", res)
	}
}

// TestRemovePairBranch_UnreachableOriginErrorsAndKeepsBranch covers the fail-closed shape: with the origin unreachable the call returns an error and the branch is still present.
func TestRemovePairBranch_UnreachableOriginErrorsAndKeepsBranch(t *testing.T) {
	t.Parallel()

	const slug = "pair-archive-unreachable"
	branch := fabricengine.WeftBranchName(slug)
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	if _, err := fabricengine.NewTopology(fabricengine.Config{}).RemovePairBranch(l, slug); err == nil {
		t.Fatal("RemovePairBranch() with an unreachable origin = nil error; want the archive failure")
	}
	if !branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q was deleted despite the failed archive", branch)
	}
}

// TestCleanup_ApplyArchivesEachOrphanBeforeDeleting covers Cleanup's apply loop: every orphan weft branch is tagged on the origin at its tip and then deleted.
func TestCleanup_ApplyArchivesEachOrphanBeforeDeleting(t *testing.T) {
	t.Parallel()

	slugs := []string{"cleanup-archive-a", "cleanup-archive-b"}
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	tips := make(map[string]string, len(slugs))
	for _, slug := range slugs {
		branch := fabricengine.WeftBranchName(slug)
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		tips[slug] = branchTip(t, weftRoot, branch)
	}

	res, err := fabricengine.NewTopology(fabricengine.Config{}).Cleanup(l, true, false, false)
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
		if got := tagTargetAt(t, fixture.WeftBare, wantTag); got != tips[slug] {
			t.Errorf("origin tag %s = %q; want %s", wantTag, got, tips[slug])
		}
		if branchExistsAt(t, weftRoot, branch) {
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
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	res, err := fabricengine.NewTopology(fabricengine.Config{}).Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true) error = %v; want the failure on the entry only", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if entry.Error == "" || entry.Deleted {
		t.Errorf("entry = %+v; want Error set and Deleted false", *entry)
	}
	if !branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q was deleted despite the failed archive", branch)
	}
}

// TestCleanup_NoOriginDeletesAndReportsArchiveSkip covers a weft repo with no origin: the orphan is deleted as before and the result reports the skipped archive once.
func TestCleanup_NoOriginDeletesAndReportsArchiveSkip(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-archive-noorigin-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustRemoveOrigin(t, weftRoot)

	res, err := fabricengine.NewTopology(fabricengine.Config{}).Cleanup(l, true, false, false)
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
	if branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q still present after Cleanup", branch)
	}
}
