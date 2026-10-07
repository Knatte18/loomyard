//go:build integration

// archive_integration_test.go covers archiveWeftTip: the archive tag lands on the weft origin at the branch tip, the call is idempotent on an unchanged tip, and each degraded shape — no origin, an unreachable origin, a branch only on origin, a clashing tag — answers as the helper documents.
//
// Every hub is built through hubforge.NewHub, with the hub's RecordsBare as the weft origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

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

// TestArchiveWeftTip builds one hub and archives distinct branches on it, each step using its own
// slug and branch so the steps do not interact:
// the happy path and idempotence — the tag exists on the origin at the tip, and a second call on the
// same tip succeeds with the same tag;
// a branch present only on origin is archived from the fetched tip;
// a branch present nowhere has nothing to archive;
// and a tag of the archive name already present at another commit is an error with nothing pushed.
// The degraded shapes that break the shared origin — no origin, an unreachable origin — keep their
// own hubs in the tests below.
func TestArchiveWeftTip(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)

	t.Run("tags and pushes the tip, idempotently", func(t *testing.T) {
		const slug = "archive-happy"
		const branch = "archive-happy-weft"
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		tip := gitkit.RevParse(t, weftRoot, branch)

		rec := fabricengine.NewMutations(l.HubPath)
		tag, reason, err := fabricengine.ArchiveWeftTipForTest(rec, l, slug, branch)
		if err != nil {
			t.Fatalf("archiveWeftTip error = %v", err)
		}
		wantTag := "archive/" + slug + "/" + tip[:12]
		if tag != wantTag || reason != "" {
			t.Fatalf("archiveWeftTip = (%q, %q); want (%q, empty reason)", tag, reason, wantTag)
		}
		if got := tagTargetAt(t, h.RecordsBare, tag); got != tip {
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
	})

	t.Run("a branch only on origin is archived from the fetched tip", func(t *testing.T) {
		const slug = "archive-origin-only"
		const branch = "archive-origin-only-weft"
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		mustPushBranch(t, weftRoot, branch)
		tip := gitkit.RevParse(t, weftRoot, branch)
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
		if got := tagTargetAt(t, h.RecordsBare, tag); got != tip {
			t.Errorf("origin tag %s points at %q; want %s", tag, got, tip)
		}
	})

	t.Run("a branch present nowhere is a no-op", func(t *testing.T) {
		rec := fabricengine.NewMutations(l.HubPath)
		tag, reason, err := fabricengine.ArchiveWeftTipForTest(rec, l, "archive-missing", "archive-missing-weft")
		if err != nil || tag != "" || reason != "" {
			t.Errorf("archiveWeftTip = (%q, %q, %v); want all empty", tag, reason, err)
		}
	})

	t.Run("a clashing tag at another commit errors and pushes nothing", func(t *testing.T) {
		const slug = "archive-clash"
		const branch = "archive-clash-weft"
		mustCreateOrphanWeftBranch(t, weftRoot, branch)
		tip := gitkit.RevParse(t, weftRoot, branch)

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
		if got := tagTargetAt(t, h.RecordsBare, tag); got != "" {
			t.Errorf("origin holds tag %s at %s; want none pushed", tag, got)
		}
	})
}

// TestArchiveWeftTip_NoOriginSkips covers a weft repo with no origin: a skip reason, no tag, no error.
//
//testtiming:keep a weft repo with no origin giving a skip reason, no tag and no error; coverage of its blocks by other tests does not show an assertion of this
func TestArchiveWeftTip_NoOriginSkips(t *testing.T) {
	t.Parallel()

	const branch = "archive-no-origin-weft"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := gitkit.RevParse(t, weftRoot, branch)
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
//
//testtiming:keep a configured but unreachable origin giving an error naming the tag and no tag on the real remote; coverage of its blocks by other tests does not show an assertion of this
func TestArchiveWeftTip_UnreachableOriginErrors(t *testing.T) {
	t.Parallel()

	const slug = "archive-unreachable"
	const branch = "archive-unreachable-weft"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	tip := gitkit.RevParse(t, weftRoot, branch)
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
	if got := tagTargetAt(t, h.RecordsBare, wantTag); got != "" {
		t.Errorf("the real origin holds tag %s at %s; want none", wantTag, got)
	}
	if n := countKind(rec, fabricengine.KindTagPushed); n != 0 {
		t.Errorf("recorded %d %s entries; want 0", n, fabricengine.KindTagPushed)
	}
}
