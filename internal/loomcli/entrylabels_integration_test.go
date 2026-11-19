// entrylabels_integration_test.go drives entryLabels against a real hub pair and its hub board.

package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestEntryLabels_ReadsTheRunsBoardEntryAtEachCall pins that the closure reads the worktree's board entry at every call.
// It is serial because it sets the board skip variable, which is process-global.
func TestEntryLabels_ReadsTheRunsBoardEntryAtEachCall(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("BOARD_SKIP_GIT", "1")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	const slug = "labelled"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree() error = %v; want nil", err)
	}
	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		t.Fatalf("OpenHub() error = %v; want nil", err)
	}
	labels := entryLabels(location)

	got, err := labels()
	if err != nil || got != nil {
		t.Fatalf("entryLabels()() with no board entry = %v, %v; want nil labels and no error", got, err)
	}

	for _, want := range [][]string{{"bug"}, {"enhancement"}} {
		if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": "Fix it", "kind": "task", "labels": want}); err != nil {
			t.Fatalf("UpsertTask(%v) error = %v; want nil", want, err)
		}
		got, err := labels()
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("entryLabels()() = %v, %v; want %v and no error, read from the board at this call", got, err, want)
		}
	}
}
