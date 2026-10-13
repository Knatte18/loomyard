//go:build integration

// peers_integration_test.go drives the real hubPeersDeps closures over a hub forged by hubforge, with pairs added through hubforge.AddPair.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestHubPeersDeps_OverRealHub asserts four behaviors of the real closures.
// A peer's line lists exactly the paths it committed against its merge base with its parent and flags its decision record.
// The caller's parent commits are the parent's two new commits, newest first.
// A board task with no pair directory lists the pair as unreadable.
// A peer whose origin record is gone lists its branch as unreadable and is kept.
func TestHubPeersDeps_OverRealHub(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	for _, slug := range []string{"caller", "peer", "no-origin"} {
		hubforge.AddPair(t, h, slug)
	}

	boardConfig, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	boardConfig.Path = h.BoardDir()
	board := boardengine.New(boardConfig)
	for _, slug := range []string{"caller", "peer", "no-origin", "ghost"} {
		if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": "Title of " + slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
			t.Fatalf("seed board task %q: %v", slug, err)
		}
		if err := board.SetRunStatus(slug, "running · Plan-Write"); err != nil {
			t.Fatalf("set run status of %q: %v", slug, err)
		}
	}

	gitkit.CommitFile(t, h.PairCodeWorktree("caller"), "caller.txt", "c", "caller: change")
	gitkit.CommitFile(t, h.PairCodeWorktree("peer"), "z/last.txt", "z", "peer: last")
	gitkit.CommitFile(t, h.PairCodeWorktree("peer"), "a/first.txt", "a", "peer: first")
	firstOnParent := gitkit.CommitFile(t, h.PrimeWorktree(), "parent-one.txt", "1", "parent: one")
	secondOnParent := gitkit.CommitFile(t, h.PrimeWorktree(), "parent-two.txt", "2", "parent: two")

	peerLocation, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("peer"))
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(peer) error = %v; want nil", err)
	}
	recordPath := loomengine.DiscussionDecisionRecord(peerLocation)
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(recordPath), err)
	}
	if err := os.WriteFile(recordPath, []byte("# Decision record\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
	}
	if err := os.Remove(fabricengine.OriginRecordPathFor(h.Location, "no-origin")); err != nil {
		t.Fatalf("remove the origin record of no-origin: %v", err)
	}

	callerLocation, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("caller"))
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(caller) error = %v; want nil", err)
	}
	deps := hubPeersDeps(callerLocation)

	t.Run("a peer lists exactly its committed paths and its decision record", func(t *testing.T) {
		line := deps.readPeer("peer")
		if want := []string{"a/first.txt", "z/last.txt"}; !slices.Equal(line.ChangedPaths, want) {
			t.Errorf("changed paths = %v; want %v", line.ChangedPaths, want)
		}
		if !line.DecisionRecord || line.Plan || len(line.Unreadable) != 0 {
			t.Errorf("line = %+v; want a decision record, no plan and nothing unreadable", line)
		}
		if line.ParentBranch == "" {
			t.Errorf("parent branch is empty; want the branch the origin record names")
		}
	})

	t.Run("the caller's parent commits are the parent's new commits newest first", func(t *testing.T) {
		commits, err := deps.parentCommits()
		if err != nil {
			t.Fatalf("parentCommits() error = %v; want nil", err)
		}
		if want := []string{secondOnParent[:7] + " parent: two", firstOnParent[:7] + " parent: one"}; !slices.Equal(commits, want) {
			t.Errorf("parentCommits() = %v; want %v", commits, want)
		}
	})

	t.Run("a task whose pair directory was never created lists the pair as unreadable", func(t *testing.T) {
		line := deps.readPeer("ghost")
		if len(line.Unreadable) != 1 || !strings.HasPrefix(line.Unreadable[0], "pair: ") {
			t.Errorf("unreadable = %v; want one entry naming the pair", line.Unreadable)
		}
	})

	t.Run("a peer whose origin record is removed lists its branch as unreadable and keeps its line", func(t *testing.T) {
		line := deps.readPeer("no-origin")
		if len(line.Unreadable) != 1 || !strings.HasPrefix(line.Unreadable[0], "branch: ") {
			t.Errorf("unreadable = %v; want one entry naming the branch", line.Unreadable)
		}
	})

	t.Run("peers reports the embedded loom recipe's row for each peer and the caller", func(t *testing.T) {
		rows, err := loomrecipe.RowNames(shedrun.RecipeLoom)
		if err != nil {
			t.Fatalf("loomrecipe.RowNames(loom) error = %v; want nil", err)
		}
		want := float64(slices.Index(rows, "Plan-Write") + 1)
		if want == 0 {
			t.Fatal("loom recipe has no Plan-Write row")
		}
		var out bytes.Buffer
		if code := peersVerb(&out, deps); code != 0 {
			t.Fatalf("peersVerb exit = %d, out %s; want 0", code, out.String())
		}
		raw := envelope.RequireOK(t, out.String()).Raw
		if raw["own_row"] != want {
			t.Errorf("own_row = %v; want %v", raw["own_row"], want)
		}
		for _, peer := range raw["peers"].([]any) {
			line := peer.(map[string]any)
			if line["slug"] == "peer" && line["row"] != want {
				t.Errorf("peer row = %v; want %v", line["row"], want)
			}
		}
	})

	t.Run("the board lists the seeded tasks", func(t *testing.T) {
		tasks, hasBoard, err := deps.board()
		if err != nil || !hasBoard || len(tasks) != 4 {
			t.Errorf("board() = %d tasks, hasBoard %v, err %v; want the four seeded tasks", len(tasks), hasBoard, err)
		}
	})
}
