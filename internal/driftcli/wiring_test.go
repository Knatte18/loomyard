// wiring_test.go checks that verbStatus carries the run counter's operator mark into the status a drift verb is decided on.

package driftcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestVerbStatus_OperatorSession covers the operator mark of an open agent session reaching VerbStatus, for an operator session and an autonomous one.
func TestVerbStatus_OperatorSession(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	rows := []struct {
		name     string
		operator bool
	}{
		{"OperatorSession", true},
		{"AutonomousSession", false},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
			loc, err := lymcwd.Resolve(h.PairCodeWorktree(hubforge.TemplatePairSlug))
			if err != nil {
				t.Fatal(err)
			}
			wt := &worktree{location: loc, paths: driftengine.NewPaths(loc.AnchorPath())}

			lockPath := shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				t.Fatal(err)
			}
			running := shedengine.Status{CurrentProducer: "Drift-Verify", State: shedengine.StateRunning}
			if err := state.WriteJSON(shedrun.StatusFile(loc, shedrun.SelfRunID), lockPath, running); err != nil {
				t.Fatal(err)
			}
			if err := driftengine.ResetRuns(wt.paths, "head", tt.operator); err != nil {
				t.Fatal(err)
			}

			got, err := wt.verbStatus()
			if err != nil {
				t.Fatalf("verbStatus() = %v; want nil", err)
			}
			if got.OperatorSession != tt.operator || got.AtOperatorSession() != tt.operator {
				t.Errorf("verbStatus() OperatorSession = %v, AtOperatorSession() = %v; want both %v", got.OperatorSession, got.AtOperatorSession(), tt.operator)
			}
		})
	}
}
