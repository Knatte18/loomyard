// specfill_test.go pins the run-identity fields loom's arming fills onto its Spec: RunID, StepsDir
// and Routing. It calls specFor and loadRouting on a hand-populated receiver, so it spawns nothing.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestSpecFor_FillsRunIdentityForSelf verifies that a spec armed for "self" reports the worktree
// slug as RunID, a StepsDir under the slug's .lyx scratch directory, and the loom routing entry.
func TestSpecFor_FillsRunIdentityForSelf(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "wt", AnchorRel: "."}
	c := &loomCLI{
		location:  loc,
		runID:     shedrun.SelfRunID,
		shedPaths: shedbuild.ShedPaths{MaxBounces: 3},
	}
	if err := c.loadRouting(); err != nil {
		t.Fatalf("loadRouting: %v", err)
	}

	spec := c.specFor("status")

	if spec.RunID != "wt" {
		t.Errorf("RunID = %q, want %q", spec.RunID, "wt")
	}
	wantSteps := filepath.Join(loc.AnchorPath(), ".lyx", "shed", "wt", "steps")
	if spec.StepsDir != wantSteps {
		t.Errorf("StepsDir = %q, want %q", spec.StepsDir, wantSteps)
	}
	if spec.Routing.Entry != "Preflight" {
		t.Errorf("Routing.Entry = %q, want %q", spec.Routing.Entry, "Preflight")
	}
	if spec.Routing.MaxBounces != 3 {
		t.Errorf("Routing.MaxBounces = %d, want 3", spec.Routing.MaxBounces)
	}

	// The watchdog's pause seam reads the hub's record for the run's slug.
	gitdir := "gitdir: " + filepath.Join(loc.HubPath, "store", ".git", "worktrees", "wt")
	if err := os.MkdirAll(loc.WorktreePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loc.WorktreePath(), ".git"), []byte(gitdir), 0o644); err != nil {
		t.Fatal(err)
	}
	geometry, err := hubgeom.PauseGeometry(loc)
	if err != nil {
		t.Fatalf("PauseGeometry: %v", err)
	}
	pausedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := pauserecord.Update(geometry.RecordPath, pausedAt, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: "wt", PausedAt: pausedAt})
		return nil
	}); err != nil {
		t.Fatalf("write pause record: %v", err)
	}
	coverage, err := spec.Loop.Pause(pausedAt.Add(time.Hour))
	if err != nil || !coverage.Covered {
		t.Errorf("Loop.Pause covered/err = %v/%v; want the run covered by the live entry", coverage.Covered, err)
	}
}

// TestSpecFor_LegacySelfDirKeepsSlugRunID verifies that with only a legacy _lyx/shed/self/ present,
// RunID stays the slug while StepsDir keeps the legacy "self" segment.
func TestSpecFor_LegacySelfDirKeepsSlugRunID(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "wt", AnchorRel: "."}
	if err := os.MkdirAll(filepath.Join(loc.AnchorPath(), "_lyx", "shed", "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &loomCLI{location: loc, runID: shedrun.SelfRunID}

	spec := c.specFor("status")

	if spec.RunID != "wt" {
		t.Errorf("RunID = %q, want %q", spec.RunID, "wt")
	}
	wantSuffix := filepath.Join(".lyx", "shed", "self", "steps")
	if !strings.HasSuffix(spec.StepsDir, wantSuffix) {
		t.Errorf("StepsDir = %q, want suffix %q", spec.StepsDir, wantSuffix)
	}
}
