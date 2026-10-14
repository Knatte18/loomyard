//go:build integration

// committedfile_integration_test.go covers CommittedAnchoredFile and CommittedAnchoredFileHistory against a wired hub fixture:
// the committed bytes come back despite a later working-tree edit, a path never committed reports found == false,
// and the history holds every committed version, newest first.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestCommittedAnchoredFile runs the single-revision outcomes and the history read over one hub.
// The steps run in order and touch disjoint paths.
func TestCommittedAnchoredFile(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// A path absent at HEAD reports found == false.
	t.Run("NeverCommitted", func(t *testing.T) {
		data, found, err := fabricengine.CommittedAnchoredFile(l, "_lyx/never-committed.txt")
		if err != nil {
			t.Fatalf("CommittedAnchoredFile error = %v", err)
		}
		if found || data != nil {
			t.Errorf("CommittedAnchoredFile = (%q, %v); want (nil, false)", data, found)
		}
	})

	// Commits an anchored file, edits the working-tree copy, and asserts the committed bytes come back.
	t.Run("ReturnsCommittedBytes", func(t *testing.T) {
		const rel = "_lyx/committedfile-probe.txt"
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

	// A never-committed path has an empty history; two committed versions come back newest first, without the edited working copy.
	t.Run("HistoryHoldsCommittedVersions", func(t *testing.T) {
		const rel = "_lyx/committedfile-history-probe.txt"
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
