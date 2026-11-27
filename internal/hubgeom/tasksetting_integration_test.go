// tasksetting_integration_test.go drives TaskSettingChain over one real hub from hubforge:
// a task under a beam, a task whose worktree is absent, and the prime alone.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestTaskSettingChain pins the anchors a per-task beam setting is looked up through:
// the task's worktree, each parent's and the prime's, nearest first, with the Bolt dir as the board.
// A slug with no worktree on this machine is left out, and an empty slug names the prime alone.
func TestTaskSettingChain(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	const (
		beamPair  = "ts-beam"
		task      = "ts-task"
		unspawned = "ts-unspawned"
		cycleA    = "ts-cycle-a"
		cycleB    = "ts-cycle-b"
	)
	hubforge.AddPairWith(t, h, beamPair, fabricengine.AddOptions{SkipPush: true})
	beamLocation, err := lymcwd.Resolve(fabricengine.WorktreePath(l, beamPair))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(beam pair): %v", err)
	}
	if _, err := h.Topology.Add(beamLocation, task, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) from the beam pair: %v", task, err)
	}
	board := `{"version":1,"entries":[` +
		`{"id":0,"slug":"` + beamPair + `","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":1,"slug":"` + unspawned + `","title":"Unspawned","kind":"task","labels":["bug"],"project":"` + beamPair + `"},` +
		`{"id":2,"slug":"` + cycleA + `","title":"CycleA","kind":"project","labels":["bug"],"beam":true,"project":"` + cycleB + `"},` +
		`{"id":3,"slug":"` + cycleB + `","title":"CycleB","kind":"project","labels":["bug"],"beam":true,"project":"` + cycleA + `"}]}`
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	wantBoard := fabriclayout.BoltDir(l.HubPath)
	tests := []struct {
		name string
		slug string
		want []string
	}{
		{"TaskUnderABeamListsTaskBeamAndPrime", task, []string{fabricengine.WorktreePath(l, task), fabricengine.WorktreePath(l, beamPair), l.AnchorPath()}},
		{"TaskWithNoWorktreeIsLeftOut", unspawned, []string{fabricengine.WorktreePath(l, beamPair), l.AnchorPath()}},
		{"EmptySlugIsThePrimeAlone", "", []string{l.AnchorPath()}},
		{"ParentCycleEndsTheChainAtThePrime", cycleA, []string{l.AnchorPath()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := hubgeom.TaskSettingChain(l, tt.slug)
			if err != nil {
				t.Fatalf("TaskSettingChain(%q) error = %v; want nil", tt.slug, err)
			}
			if !slices.Equal(got.Anchors, tt.want) {
				t.Errorf("TaskSettingChain(%q).Anchors = %v; want %v", tt.slug, got.Anchors, tt.want)
			}
			if got.Board != wantBoard {
				t.Errorf("TaskSettingChain(%q).Board = %q; want %q", tt.slug, got.Board, wantBoard)
			}
		})
	}
}

// TestTaskSettingChain_Errors pins the failures the chain names the slug for:
// a present worktree whose recorded anchor is unusable, and a slug with no origin record whose board does not read.
func TestTaskSettingChain_Errors(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const (
		badAnchor = "ts-bad-anchor"
		noOrigin  = "ts-no-origin"
	)
	if err := os.MkdirAll(fabricengine.WorktreePath(l, badAnchor), 0o755); err != nil {
		t.Fatalf("MkdirAll(worktree of %q): %v", badAnchor, err)
	}
	recordsSibling := h.PairRecordsSibling(badAnchor)
	if err := os.MkdirAll(recordsSibling, 0o755); err != nil {
		t.Fatalf("MkdirAll(records of %q): %v", badAnchor, err)
	}
	if err := os.WriteFile(filepath.Join(recordsSibling, lymcwd.AnchorFileName), []byte("../escape\n"), 0o644); err != nil {
		t.Fatalf("write the anchor record of %q: %v", badAnchor, err)
	}
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	tests := []struct {
		name string
		slug string
	}{
		{"APresentWorktreeWithAnUnusableAnchorNamesTheSlug", badAnchor},
		{"AnUnreadableBoardForASlugWithNoOriginRecordNamesTheSlug", noOrigin},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := hubgeom.TaskSettingChain(l, tt.slug)
			if err == nil || !strings.Contains(err.Error(), tt.slug) {
				t.Errorf("TaskSettingChain(%q) error = %v; want one naming the slug", tt.slug, err)
			}
		})
	}
}
