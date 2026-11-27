// newdeps_integration_test.go drives the conditions and the wait NewDeps fills.

package runpusher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
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
			deps := NewDeps(Target{Location: pair, Slug: "task1", Pair: true}, fabricengine.PushTiming{})

			got, err := deps.Conditions()

			if err != nil || got != tt.want {
				t.Errorf("Conditions() = %v, %v; want %v, nil", got, err, tt.want)
			}
			if deps.PauseCovers == nil {
				t.Errorf("PauseCovers = nil for a pair; want the pause read")
			}
			if prime := NewDeps(Target{Location: h.Location}, fabricengine.PushTiming{}); prime.PauseCovers != nil {
				t.Errorf("PauseCovers set for the prime; want nil")
			}
		})
	}
}

func TestNewDeps_WaitWakesOnAPushNowMark(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	deps := NewDeps(pairTargetIn(t), fabricengine.PushTiming{})
	write := func(entry pushledger.Entry) {
		t.Helper()
		err := pushledger.Update(deps.HubPath, deps.Key, time.Now(), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
			return entry, true, nil
		})
		if err != nil {
			t.Fatalf("write the ledger entry: %v", err)
		}
	}
	write(pushledger.Entry{FirstOwed: time.Now()})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- deps.Wait(ctx, time.Hour, nil) }()

	write(pushledger.Entry{FirstOwed: time.Now(), IntervalS: 120})
	// Longer than the wake floor, so a wake on the unmarked write would have ended the wait by now.
	select {
	case err := <-done:
		t.Fatalf("Wait() = %v after a write with no mark; want it still waiting", err)
	case <-time.After(markWakeFloor + markWakeFloor/2):
	}

	write(pushledger.Entry{FirstOwed: time.Now(), PushNow: true})
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait() = %v after the push-now mark; want nil", err)
		}
	case <-time.After(time.Minute):
		t.Fatal("Wait() did not return after the push-now mark")
	}
}
