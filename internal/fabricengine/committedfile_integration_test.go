//go:build integration

// committedfile_integration_test.go covers CommittedAnchoredFile against a wired hub fixture:
// the committed bytes come back despite a later working-tree edit, and a path never committed reports found == false.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestCommittedAnchoredFile_ReturnsCommittedBytes commits an anchored file, edits the working-tree copy, and asserts the committed bytes come back.
func TestCommittedAnchoredFile_ReturnsCommittedBytes(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const rel = "_lyx/committedfile-probe.txt"
	onDisk := filepath.Join(fabricengine.WeftWorktree(l), l.AnchorRel, rel)

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
}

// TestCommittedAnchoredFile_NeverCommitted asserts a path absent at HEAD reports found == false.
func TestCommittedAnchoredFile_NeverCommitted(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	data, found, err := fabricengine.CommittedAnchoredFile(h.Location, "_lyx/never-committed.txt")
	if err != nil {
		t.Fatalf("CommittedAnchoredFile error = %v", err)
	}
	if found || data != nil {
		t.Errorf("CommittedAnchoredFile = (%q, %v); want (nil, false)", data, found)
	}
}
