// newdeps_integration_test.go drives the conditions NewDeps fills over a hubforge pair.

package runpusher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestNewDeps_ConditionsStopOnAHandOverMarkOrARefusedPush(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name  string
		block func(t *testing.T, h *hubforge.Hub, pair *lymcwd.Location)
		want  bool
	}{
		{"NothingStandingHolds", func(*testing.T, *hubforge.Hub, *lymcwd.Location) {}, true},
		{"HandOverMarkOfTheRunStops", func(t *testing.T, h *hubforge.Hub, _ *lymcwd.Location) {
			mark := fabriclayout.HandOverMark{Machine: "oslo", At: "2026-11-15T09:00:00Z", Written: time.Now()}
			if err := fabricengine.WriteHandOverMark(h.Path, "task1", mark); err != nil {
				t.Fatalf("write the hand-over mark: %v", err)
			}
		}, false},
		{"RefusedPushOfTheRecordsBranchStops", func(t *testing.T, h *hubforge.Hub, pair *lymcwd.Location) {
			hook := filepath.Join(h.RecordsBare, "hooks", "pre-receive")
			if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
				t.Fatalf("create the hooks directory: %v", err)
			}
			if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'declined' >&2\nexit 1\n"), 0o755); err != nil {
				t.Fatalf("write the hook: %v", err)
			}
			gitkit.CommitFile(t, h.PairRecordsSibling("task1"), "local.txt", "local\n", "local")
			options := fabricengine.SyncOptions{BackoffSleep: func(time.Duration) {}}
			if _, err := fabricengine.PushPairAnchored(pair, options, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}); err == nil {
				t.Fatal("PushPairAnchored() against a refusing remote = nil; want the refusal")
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, "task1")
			pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task1"))
			if err != nil {
				t.Fatalf("ResolveWorktree(task1) = %v; want nil", err)
			}
			tt.block(t, h, pair)
			deps := NewDeps(Target{Location: pair, Slug: "task1", Pair: true})

			got, err := deps.Conditions()

			if err != nil || got != tt.want {
				t.Errorf("Conditions() = %v, %v; want %v, nil", got, err, tt.want)
			}
			if deps.PauseCovers == nil {
				t.Errorf("PauseCovers = nil for a pair; want the pause read")
			}
			if prime := NewDeps(Target{Location: h.Location}); prime.PauseCovers != nil {
				t.Errorf("PauseCovers set for the prime; want nil")
			}
		})
	}
}
