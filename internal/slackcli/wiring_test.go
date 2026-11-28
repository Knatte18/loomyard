// wiring_test.go covers the production seams of wiring.go that read a real hub on disk.

package slackcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReceiverPaused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	pairSlug := hubforge.TemplatePairSlug
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	prime, err := lymcwd.ResolveWorktree(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", hub.PrimeWorktree(), err)
	}
	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		t.Fatalf("PauseGeometry() = %v", err)
	}
	_, err = pauserecord.Update(geometry.RecordPath, time.Now(), func(record *pauserecord.Record) error {
		record.Live = []pauserecord.Entry{{Scope: pairSlug, PausedAt: time.Now().Add(-time.Minute)}}
		return nil
	})
	if err != nil {
		t.Fatalf("Update() = %v", err)
	}

	tests := []struct {
		name     string
		receiver string
		want     bool
	}{
		{"HubOrchReadsThePrimesPauseAndIsNotCoveredByAPairEntry", "ly:orch", false},
		{"BeamOrchReadsItsOwnWorktreesPauseAndIsCoveredByItsPairEntry", "ly:" + pairSlug + ":orch", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := receiverPaused(hub.Path, prime, tt.receiver)
			if err != nil || got != tt.want {
				t.Errorf("receiverPaused(%q) = (%v, %v); want (%v, nil)", tt.receiver, got, err, tt.want)
			}
		})
	}
}

func TestWorktreeOrch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	pairSlug := hubforge.TemplatePairSlug
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	prime, err := lymcwd.ResolveWorktree(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", hub.PrimeWorktree(), err)
	}

	const beamSlug = "beam-one"
	hubforge.AddPairWith(t, hub, beamSlug, fabricengine.AddOptions{Beam: true})

	tests := []struct {
		name     string
		worktree string
		want     string
	}{
		{"PrimeGetsTheHubsOrch", prime.WorktreeName, "ly:orch"},
		{"ListedBeamGetsItsOrch", beamSlug, "ly:" + beamSlug + ":orch"},
		{"PairThatIsNoBeamGetsNone", pairSlug, ""},
		{"UnlistedWorktreeGetsNone", "unlisted-worktree", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := *prime
			location.WorktreeName = tt.worktree
			got, err := worktreeOrch(&location, prime.WorktreeName, prime, "ly")
			if err != nil || got != tt.want {
				t.Errorf("worktreeOrch(%q) = (%q, %v); want (%q, nil)", tt.worktree, got, err, tt.want)
			}
		})
	}
}

func TestOrchProject(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	board, err := boardengine.OpenHub(hub.Path)
	if err != nil {
		t.Fatalf("OpenHub(%q) = %v", hub.Path, err)
	}
	const projectSlug = "adopt-takeover"
	project, err := board.UpsertTask(map[string]any{"slug": projectSlug, "kind": "project", "title": "Adopt takeover", "brief": "Groups the work.", "labels": []string{"bug"}})
	if err != nil {
		t.Fatalf("UpsertTask() = %v", err)
	}
	unreadableHub := t.TempDir()
	if err := os.MkdirAll(fabricengine.BoardDir(unreadableHub), 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(unreadableHub), "board.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile = %v", err)
	}

	tests := []struct {
		name       string
		hubPath    string
		slug       string
		wantHandle string
		wantName   string
		wantErr    bool
	}{
		{"HubOrchRunsTheRootProject", hub.Path, "", "P0", "root", false},
		{"BeamRunsItsProjectEntry", hub.Path, projectSlug, project.Handle, projectSlug, false},
		{"BeamWithNoProjectEntryHasNoHandleAndKeepsItsSlug", hub.Path, "other", "", "other", false},
		{"UnreadableBoardIsAnError", unreadableHub, projectSlug, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handle, name, err := orchProject(tt.hubPath, tt.slug)
			if (err != nil) != tt.wantErr || handle != tt.wantHandle || name != tt.wantName {
				t.Errorf("orchProject(%q) = (%q, %q, %v); want (%q, %q, error %v)", tt.slug, handle, name, err, tt.wantHandle, tt.wantName, tt.wantErr)
			}
		})
	}
}
