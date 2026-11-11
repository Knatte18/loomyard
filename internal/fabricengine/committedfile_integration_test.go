// committedfile_integration_test.go covers committed anchored-file reads and their histories.
// The subjects are CommittedAnchoredFile, CommittedAnchoredFileHistory and AnchoredFileFirstParentHistory.
// The tests run against a wired hub fixture.
//
// It is in package fabricengine_test and shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAnchoredFileFirstParentHistory reads one file's first-parent versions over one hub.
// The steps run in order over one weft history, so none is parallel.
func TestAnchoredFileFirstParentHistory(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weft := fabricengine.RecordsWorktree(l)
	rec := fabricengine.NewMutations(l.HubPath)
	const rel = "_lym/first-parent-probe.txt"
	const unrelated = "_lym/first-parent-unrelated.txt"

	t.Run("NoHistoryIsEmpty", func(t *testing.T) {
		got, err := fabricengine.AnchoredFileFirstParentHistory(l, rel)
		if err != nil || len(got) != 0 {
			t.Fatalf("AnchoredFileFirstParentHistory before any commit = (%+v, %v); want empty and nil error", got, err)
		}
	})

	commit := func(path, content string) string {
		t.Helper()
		writeAnchored(t, l, path, content)
		sha, committed, err := fabricengine.CommitAnchoredPaths(rec, l, []string{path}, "commit "+content, fabricengine.SyncOptions{})
		if err != nil || !committed {
			t.Fatalf("CommitAnchoredPaths(%q) committed = %v, err = %v", content, committed, err)
		}
		return sha
	}
	// The three commits touching the file are made back to back, so they share a committer second.
	first := commit(rel, "first")
	commit(unrelated, "unrelated")
	second := commit(rel, "second")
	third := commit(rel, "third")

	t.Run("VersionsNewestFirstWithPositions", func(t *testing.T) {
		got, err := fabricengine.AnchoredFileFirstParentHistory(l, rel)
		if err != nil {
			t.Fatalf("AnchoredFileFirstParentHistory error = %v", err)
		}
		want := []fabricengine.AnchoredFileVersion{
			{SHA: third, Position: firstParentPosition(t, l, third), Data: []byte("third")},
			{SHA: second, Position: firstParentPosition(t, l, second), Data: []byte("second")},
			{SHA: first, Position: firstParentPosition(t, l, first), Data: []byte("first")},
		}
		if len(got) != len(want) {
			t.Fatalf("AnchoredFileFirstParentHistory = %+v; want %+v", got, want)
		}
		for i := range want {
			if got[i].SHA != want[i].SHA || got[i].Position != want[i].Position || string(got[i].Data) != string(want[i].Data) {
				t.Errorf("version %d = %+v; want %+v", i, got[i], want[i])
			}
		}
		if got[0].Position-got[1].Position != 1 || got[1].Position-got[2].Position != 2 {
			t.Errorf("positions = %d, %d, %d; want gaps of 1 and 2 (the unrelated commit counts)", got[0].Position, got[1].Position, got[2].Position)
		}
	})

	t.Run("DeletionCommitIsSkipped", func(t *testing.T) {
		gitkit.Git(t, weft, "rm", "--", anchoredWeftPath(l, rel))
		gitkit.Git(t, weft, "commit", "-m", "delete the probe")

		got, err := fabricengine.AnchoredFileFirstParentHistory(l, rel)
		if err != nil {
			t.Fatalf("AnchoredFileFirstParentHistory error = %v", err)
		}
		if len(got) != 3 || got[0].SHA != third {
			t.Errorf("AnchoredFileFirstParentHistory after a deletion = %+v; want the three earlier versions, newest %s", got, third)
		}
	})
}

// TestCommittedAnchoredFile runs the single-revision outcomes and the history read over one hub.
// The steps run in order and touch disjoint paths.
func TestCommittedAnchoredFile(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// A path absent at HEAD reports found == false.
	t.Run("NeverCommitted", func(t *testing.T) {
		data, found, err := fabricengine.CommittedAnchoredFile(l, "_lym/never-committed.txt")
		if err != nil {
			t.Fatalf("CommittedAnchoredFile error = %v", err)
		}
		if found || data != nil {
			t.Errorf("CommittedAnchoredFile = (%q, %v); want (nil, false)", data, found)
		}
	})

	// The read must return the committed bytes, not the edited working-tree copy.
	t.Run("ReturnsCommittedBytes", func(t *testing.T) {
		const rel = "_lym/committedfile-probe.txt"
		onDisk := filepath.Join(fabricengine.RecordsWorktree(l), l.AnchorRel, rel)

		if err := os.WriteFile(onDisk, []byte("committed"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		rec := fabricengine.NewMutations(l.HubPath)
		if _, committed, err := fabricengine.CommitAnchoredPaths(rec, l, []string{rel}, "probe", fabricengine.SyncOptions{}); err != nil || !committed {
			t.Fatalf("CommitAnchoredPaths committed = %v, err = %v", committed, err)
		}
		if err := os.WriteFile(onDisk, []byte("edited"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		data, found, err := fabricengine.CommittedAnchoredFile(l, rel)
		if err != nil {
			t.Fatalf("CommittedAnchoredFile error = %v", err)
		}
		if !found || string(data) != "committed" {
			t.Errorf("CommittedAnchoredFile = (%q, %v); want (\"committed\", true)", data, found)
		}
	})

	// A never-committed path has an empty history;
	// two committed versions come back newest first, without the edited working copy.
	t.Run("HistoryHoldsCommittedVersions", func(t *testing.T) {
		const rel = "_lym/committedfile-history-probe.txt"
		onDisk := filepath.Join(fabricengine.RecordsWorktree(l), l.AnchorRel, rel)

		empty, err := fabricengine.CommittedAnchoredFileHistory(l, rel)
		if err != nil || len(empty) != 0 {
			t.Fatalf("CommittedAnchoredFileHistory before any commit = (%q, %v); want empty and nil error", empty, err)
		}

		for _, content := range []string{"first", "second"} {
			if err := os.WriteFile(onDisk, []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			rec := fabricengine.NewMutations(l.HubPath)
			if _, committed, err := fabricengine.CommitAnchoredPaths(rec, l, []string{rel}, "history probe "+content, fabricengine.SyncOptions{}); err != nil || !committed {
				t.Fatalf("CommitAnchoredPaths(%q) committed = %v, err = %v", content, committed, err)
			}
		}
		if err := os.WriteFile(onDisk, []byte("edited"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		versions, err := fabricengine.CommittedAnchoredFileHistory(l, rel)
		if err != nil {
			t.Fatalf("CommittedAnchoredFileHistory error = %v", err)
		}
		got := make([]string, len(versions))
		for i, v := range versions {
			got[i] = string(v)
		}
		if want := []string{"second", "first"}; !slices.Equal(got, want) {
			t.Errorf("CommittedAnchoredFileHistory = %q; want %q", got, want)
		}
	})
}
