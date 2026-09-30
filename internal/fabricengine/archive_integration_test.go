//go:build integration

// archive_integration_test.go covers archiveWeftTip: the archive tag lands on the weft origin at the
// branch tip, the call is idempotent on an unchanged tip, and each degraded shape — no origin, an
// unreachable origin, a branch only on origin, a clashing tag — answers as the helper documents.
//
// Every hub is built through hubforge.NewHub via newFabricFixture, with the hub's WeftBare as the
// weft origin. Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// archiveTipOf returns the full SHA branch points at in the repo at repoRoot.
func archiveTipOf(t *testing.T, repoRoot, branch string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + branch}, repoRoot)
	if err != nil {
		t.Fatalf("rev-parse %s in %s: %v", branch, repoRoot, err)
	}
	return strings.TrimSpace(out)
}

// tagTargetAt returns the SHA the tag names in the repo at repoRoot, or "" when the tag is absent.
func tagTargetAt(t *testing.T, repoRoot, tag string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/tags/" + tag}, repoRoot)
	if err != nil {
		t.Fatalf("for-each-ref tag %s in %s: %v", tag, repoRoot, err)
	}
	return strings.TrimSpace(out)
}

// countKind counts the entries of kind in rec's record.
func countKind(rec *fabricengine.Mutations, kind fabricengine.Kind) int {
	n := 0
	for _, m := range rec.Snapshot().Entries() {
		if m.Kind == kind {
			n++
		}
	}
	return n
}

// TestArchiveWeftTip_TagsAndPushesTip covers the happy path and idempotence: the tag exists on the
// origin at the tip, and a second call on the same tip succeeds with the same tag.
func TestArchiveWeftTip_TagsAndPushesTip(t *testing.T) {
	t.Parallel()

	const slug = "archive-happy"
	const branch = "archive-happy-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := archiveTipOf(t, weftRoot, branch)

	rec := fabricengine.NewMutations(l.HubPath)
	tag, reason, err := fabricengine.ArchiveWeftTipForTest(rec, l, slug, branch)
	if err != nil {
		t.Fatalf("archiveWeftTip error = %v", err)
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if tag != wantTag || reason != "" {
		t.Fatalf("archiveWeftTip = (%q, %q); want (%q, empty reason)", tag, reason, wantTag)
	}
	if got := tagTargetAt(t, fixture.WeftBare, tag); got != tip {
		t.Errorf("origin tag %s points at %q; want the tip %s", tag, got, tip)
	}
	if n := countKind(rec, fabricengine.KindTagPushed); n != 1 {
		t.Errorf("first call recorded %d %s entries; want 1", n, fabricengine.KindTagPushed)
	}

	tag2, reason2, err := fabricengine.ArchiveWeftTipForTest(rec, l, slug, branch)
	if err != nil {
		t.Fatalf("second archiveWeftTip error = %v; want an idempotent success", err)
	}
	if tag2 != tag || reason2 != "" {
		t.Errorf("second archiveWeftTip = (%q, %q); want (%q, empty reason)", tag2, reason2, tag)
	}
}

// TestArchiveWeftTip_NoOriginSkips covers a weft repo with no origin: a skip reason, no tag, no error.
func TestArchiveWeftTip_NoOriginSkips(t *testing.T) {
	t.Parallel()

	const branch = "archive-no-origin-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := archiveTipOf(t, weftRoot, branch)
	mustRemoveOrigin(t, weftRoot)

	rec := fabricengine.NewMutations(l.HubPath)
	tag, reason, err := fabricengine.ArchiveWeftTipForTest(rec, l, "archive-no-origin", branch)
	if err != nil {
		t.Fatalf("archiveWeftTip error = %v; want nil", err)
	}
	if tag != "" || !strings.Contains(reason, "no archive tag pushed") {
		t.Errorf("archiveWeftTip = (%q, %q); want no tag and a no-archive-tag reason", tag, reason)
	}
	if got := tagTargetAt(t, weftRoot, "archive/archive-no-origin/"+tip[:12]); got != "" {
		t.Errorf("a local archive tag exists at %s; want none", got)
	}
	if n := countKind(rec, fabricengine.KindTagPushed); n != 0 {
		t.Errorf("recorded %d %s entries; want 0", n, fabricengine.KindTagPushed)
	}
}

// TestArchiveWeftTip_UnreachableOriginErrors covers an origin that is configured but unreachable:
// an error naming the tag, and no tag on the real remote.
func TestArchiveWeftTip_UnreachableOriginErrors(t *testing.T) {
	t.Parallel()

	const slug = "archive-unreachable"
	const branch = "archive-unreachable-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := archiveTipOf(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	rec := fabricengine.NewMutations(l.HubPath)
	tag, _, err := fabricengine.ArchiveWeftTipForTest(rec, l, slug, branch)
	if err == nil {
		t.Fatalf("archiveWeftTip error = nil; want a push failure")
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if !strings.Contains(err.Error(), wantTag) {
		t.Errorf("error = %v; want it to name the tag %q", err, wantTag)
	}
	if tag != "" {
		t.Errorf("tag = %q; want empty on error", tag)
	}
	if got := tagTargetAt(t, fixture.WeftBare, wantTag); got != "" {
		t.Errorf("the real origin holds tag %s at %s; want none", wantTag, got)
	}
	if n := countKind(rec, fabricengine.KindTagPushed); n != 0 {
		t.Errorf("recorded %d %s entries; want 0", n, fabricengine.KindTagPushed)
	}
}

// TestArchiveWeftTip_OriginOnlyBranch covers a branch present only on origin: it is archived from
// the fetched tip.
func TestArchiveWeftTip_OriginOnlyBranch(t *testing.T) {
	t.Parallel()

	const slug = "archive-origin-only"
	const branch = "archive-origin-only-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	tip := archiveTipOf(t, weftRoot, branch)
	gitkit.MustRun(t, weftRoot, "git", "branch", "-D", branch)

	rec := fabricengine.NewMutations(l.HubPath)
	tag, reason, err := fabricengine.ArchiveWeftTipForTest(rec, l, slug, branch)
	if err != nil {
		t.Fatalf("archiveWeftTip error = %v", err)
	}
	wantTag := "archive/" + slug + "/" + tip[:12]
	if tag != wantTag || reason != "" {
		t.Fatalf("archiveWeftTip = (%q, %q); want (%q, empty reason)", tag, reason, wantTag)
	}
	if got := tagTargetAt(t, fixture.WeftBare, tag); got != tip {
		t.Errorf("origin tag %s points at %q; want %s", tag, got, tip)
	}
}

// TestArchiveWeftTip_MissingBranchIsNoop covers a branch present nowhere: nothing to archive.
func TestArchiveWeftTip_MissingBranchIsNoop(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	rec := fabricengine.NewMutations(l.HubPath)
	tag, reason, err := fabricengine.ArchiveWeftTipForTest(rec, l, "archive-missing", "archive-missing-weft")
	if err != nil || tag != "" || reason != "" {
		t.Errorf("archiveWeftTip = (%q, %q, %v); want all empty", tag, reason, err)
	}
}

// TestArchiveWeftTip_ClashingTagErrors covers a tag of the archive name already present at another
// commit: an error, and nothing pushed.
func TestArchiveWeftTip_ClashingTagErrors(t *testing.T) {
	t.Parallel()

	const slug = "archive-clash"
	const branch = "archive-clash-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := archiveTipOf(t, weftRoot, branch)

	other, err := gitexec.Run([]string{"commit-tree", "-m", "unrelated", "HEAD^{tree}"}, weftRoot)
	if err != nil {
		t.Fatalf("commit-tree: %v", err)
	}
	other = strings.TrimSpace(other)
	tag := "archive/" + slug + "/" + tip[:12]
	gitkit.MustRun(t, weftRoot, "git", "tag", tag, other)

	rec := fabricengine.NewMutations(l.HubPath)
	if _, _, err := fabricengine.ArchiveWeftTipForTest(rec, l, slug, branch); err == nil {
		t.Fatalf("archiveWeftTip error = nil; want a clash error")
	}
	if got := tagTargetAt(t, fixture.WeftBare, tag); got != "" {
		t.Errorf("origin holds tag %s at %s; want none pushed", tag, got)
	}
}
