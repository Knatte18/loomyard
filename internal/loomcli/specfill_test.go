// specfill_test.go pins the run-identity fields loom's arming fills onto its Spec: RunID, StepsDir
// and Routing. It calls specFor and loadRouting on a hand-populated receiver, so it spawns nothing.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
