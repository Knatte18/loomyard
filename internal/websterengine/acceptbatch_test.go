// acceptbatch_test.go covers AcceptBatchFabricReference: it clears a failed batch's pathless fabric-reference findings only on the no-commit, clean-tree evidence,
// records each as a batch audit warning, and refuses every other case without mutating the state.
// fakeGit only — Test Tier Purity Invariant.

package websterengine_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fabricEntry is an Uncheckable entry as record-batch writes it for a pathless fabric reference.
const fabricEntry = "fabric-reference: ran a fabric-referencing command (\"go run ./tools/tokencount -history ../x-records\")"

// acceptBatchFixture returns a state whose batch 8 failed on fabricEntry with no commit, and a geometry over g, whose head is the batch's start.
func acceptBatchFixture(t *testing.T) (*websterengine.State, websterengine.Geometry, *fakeGit) {
	t.Helper()
	g := newFakeGit()
	start := g.head
	st := &websterengine.State{Batches: map[int]*websterengine.BatchState{8: {
		StartSHA:    start,
		Terminal:    true,
		Status:      websterengine.DigestStatusFailed,
		Digest:      &websterengine.Digest{Status: websterengine.DigestStatusFailed, HeadSHA: start},
		Uncheckable: []string{fabricEntry},
	}}}
	root := t.TempDir()
	return st, websterengine.Geometry{WorktreeRoot: root, WebsterDir: filepath.Join(root, "_lyx", "webster"), Git: g}, g
}

func TestAcceptBatchFabricReference_ToleratesTheRunsOwnState(t *testing.T) {
	t.Parallel()
	st, geom, g := acceptBatchFixture(t)
	g.dirtyPaths = []string{"_lyx/", "_lyx/webster/state.json"}

	if _, err := websterengine.AcceptBatchFabricReference(st, geom, 8); err != nil {
		t.Fatalf("AcceptBatchFabricReference() error = %v; want nil over the run's own untracked state", err)
	}
}

func TestAcceptBatchFabricReference_ClearsOnNoCommitCleanTree(t *testing.T) {
	t.Parallel()
	st, geom, _ := acceptBatchFixture(t)

	accepted, err := websterengine.AcceptBatchFabricReference(st, geom, 8)
	if err != nil {
		t.Fatalf("AcceptBatchFabricReference() error = %v; want nil", err)
	}
	if !slices.Equal(accepted, []string{fabricEntry}) {
		t.Errorf("accepted = %v; want [%s]", accepted, fabricEntry)
	}
	bs := st.Batches[8]
	if len(bs.Uncheckable) != 0 {
		t.Errorf("Uncheckable = %v; want cleared", bs.Uncheckable)
	}
	if len(bs.AuditWarnings) != 1 || bs.AuditWarnings[0].Class != "fabric-reference" || !strings.Contains(bs.AuditWarnings[0].Detail, "tokencount") {
		t.Errorf("AuditWarnings = %+v; want one fabric-reference warning naming the command", bs.AuditWarnings)
	}
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("batch = terminal %v, status %q; want still terminal failed, for recover-batch", bs.Terminal, bs.Status)
	}
}

func TestAcceptBatchFabricReference_Refusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// mutate turns the accepting fixture into the refused case.
		mutate func(st *websterengine.State, g *fakeGit)
		batch  int
		want   string
	}{
		{name: "unrecorded batch", batch: 3, want: "batch 03 is not a failed batch"},
		{name: "done batch", batch: 8, mutate: func(st *websterengine.State, _ *fakeGit) { st.Batches[8].Status = websterengine.DigestStatusDone }, want: "is not a failed batch"},
		{name: "other uncheckable entry", batch: 8, mutate: func(st *websterengine.State, _ *fakeGit) {
			st.Batches[8].Uncheckable = append(st.Batches[8].Uncheckable, ".lyx/webster/pause")
		}, want: "not a pathless fabric reference"},
		{name: "batch made a commit", batch: 8, mutate: func(st *websterengine.State, g *fakeGit) {
			st.Batches[8].Digest.HeadSHA = g.commit()
		}, want: "made a commit"},
		{name: "HEAD moved past the start", batch: 8, mutate: func(_ *websterengine.State, g *fakeGit) { g.commit() }, want: "git reset --keep"},
		{name: "dirty worktree", batch: 8, mutate: func(_ *websterengine.State, g *fakeGit) { g.dirtyPaths = []string{"_lyx/", "internal/x/x.go"} }, want: "untracked changes: internal/x/x.go;"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st, geom, g := acceptBatchFixture(t)
			if tt.mutate != nil {
				tt.mutate(st, g)
			}
			before := slices.Clone(st.Batches[8].Uncheckable)

			_, err := websterengine.AcceptBatchFabricReference(st, geom, tt.batch)
			if !errors.Is(err, websterengine.ErrAuditNotAcceptable) {
				t.Fatalf("AcceptBatchFabricReference() error = %v; want ErrAuditNotAcceptable", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q; want it to contain %q", err, tt.want)
			}
			if bs := st.Batches[8]; !slices.Equal(bs.Uncheckable, before) || len(bs.AuditWarnings) != 0 {
				t.Errorf("batch 8 mutated: Uncheckable %v, AuditWarnings %v", bs.Uncheckable, bs.AuditWarnings)
			}
		})
	}
}
