// pusher_test.go drives the seams the pusher verb fills over hand-built locations and a recording push.

package loomcli

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pushCall records the arguments of one push the pusher's Push seam made.
type pushCall struct {
	prime *lymcwd.Location
	l     *lymcwd.Location
	slug  string
	push  pairteardown.RunPush
}

// TestPusherDeps covers what the pusher verb fills over the hub's reads.
// Those are the push, the pause, the pre-push guard, the landing target's level read and the timing.
// Neither worktree holds a fabric config, so every row runs on the default timing.
func TestPusherDeps(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	prime := &lymcwd.Location{RepoName: "repo", HubPath: hub, WorktreeName: "prime", AnchorRel: "."}
	pair := &lymcwd.Location{RepoName: "repo", HubPath: hub, WorktreeName: "task", AnchorRel: "."}

	t.Run("TheBoardTargetYieldsToTheGateAndPushesNoRun", func(t *testing.T) {
		t.Parallel()

		pushRun := func(*lymcwd.Location, *lymcwd.Location, string, pairteardown.RunPush) error {
			t.Error("pushRun called; want the board's own push")
			return nil
		}
		deps := pusherDeps(prime, runpusher.BoardTarget(prime), stoppingPrePushCheck(), pushRun)

		if deps.Key != pushledger.BoardKey || deps.Guard != nil || deps.Push == nil || deps.PauseCovers == nil {
			t.Fatalf("Key = %q, Guard set %v, Push set %v, PauseCovers set %v; want the board's key, no guard, a push and a pause read",
				deps.Key, deps.Guard != nil, deps.Push != nil, deps.PauseCovers != nil)
		}
		if covered, err := deps.PauseCovers(); covered || err != nil {
			t.Errorf("PauseCovers() = %v, %v; want false with no gate installed", covered, err)
		}
	})

	t.Run("TheLandingTargetIsAheadWhileALandingStands", func(t *testing.T) {
		t.Parallel()

		landingHub := t.TempDir()
		landingPrime := &lymcwd.Location{RepoName: "repo", HubPath: landingHub, WorktreeName: "prime", AnchorRel: "."}
		deps := pusherDeps(landingPrime, runpusher.LandingTarget(landingPrime), stoppingPrePushCheck(), nil)

		if deps.Guard != nil || deps.PauseCovers != nil {
			t.Errorf("Guard set %v, PauseCovers set %v; want neither", deps.Guard != nil, deps.PauseCovers != nil)
		}
		if deps.Parent.PushTip == nil || deps.Parent.HandOver == nil || deps.Parent.Refresh == nil {
			t.Errorf("PushTip set %v, HandOver set %v, Refresh set %v; want the parent step wired",
				deps.Parent.PushTip != nil, deps.Parent.HandOver != nil, deps.Parent.Refresh != nil)
		}
		if ahead, err := deps.Ahead(); ahead || err != nil {
			t.Errorf("Ahead() with no landing = %v, %v; want false, nil", ahead, err)
		}
		err := pushledger.Update(landingHub, pushledger.LandingKey, time.Unix(1, 0), func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
			entry.Landings = []pushledger.Landing{{Run: "task"}}
			return entry, true, nil
		})
		if err != nil {
			t.Fatalf("Update() = %v; want nil", err)
		}
		if ahead, err := deps.Ahead(); !ahead || err != nil {
			t.Errorf("Ahead() with a landing = %v, %v; want true, nil", ahead, err)
		}
		if err := deps.Push(); err != nil {
			t.Errorf("Push() = %v; want nil, since the landing target pushes only in its parent step", err)
		}
	})

	t.Run("TheTimingIsTheConfiguredOne", func(t *testing.T) {
		t.Parallel()

		configured := t.TempDir()
		boltDir := fabriclayout.BoltDir(configured)
		if err := os.MkdirAll(configengine.ConfigDir(boltDir), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		config := strings.NewReplacer("push_interval_s: 60", "push_interval_s: 7", "push_ceiling_s: 300", "push_ceiling_s: 70").
			Replace(fabricengine.ConfigTemplate())
		if err := os.WriteFile(configengine.ConfigFile(boltDir, "fabric"), []byte(config), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		location := &lymcwd.Location{RepoName: "repo", HubPath: configured, WorktreeName: "prime", AnchorRel: "."}

		timing := pusherDeps(location, runpusher.Target{Location: location}, stoppingPrePushCheck(), nil).Timing

		if timing.Interval != 7*time.Second || timing.Ceiling != 70*time.Second {
			t.Errorf("Timing = %+v; want the configured 7s interval and 70s ceiling", timing)
		}
	})

	tests := []struct {
		name      string
		target    runpusher.Target
		wantCall  pushCall
		wantGuard bool
	}{
		{
			name:   "APairPushesItsPairForItsSlugInsideThePair",
			target: runpusher.Target{Location: pair, Slug: "task", Pair: true},
			wantCall: pushCall{
				prime: prime, l: pair, slug: "task",
				push: pairteardown.RunPush{
					LockWait:       fabriclayout.StatusPushLockWait,
					TeardownWait:   fabriclayout.StatusPushLockWait,
					OwedUnrecorded: true,
					SingleAttempt:  true,
					InsidePair:     true,
				},
			},
			wantGuard: true,
		},
		{
			name:   "ThePrimePushesItsOwnRecordsNamingNoRun",
			target: runpusher.Target{Location: prime},
			wantCall: pushCall{
				prime: prime, l: prime, slug: "",
				push: pairteardown.RunPush{LockWait: fabriclayout.StatusPushLockWait, OwedUnrecorded: true, SingleAttempt: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls []pushCall
			pushRun := func(prime, l *lymcwd.Location, slug string, push pairteardown.RunPush) error {
				calls = append(calls, pushCall{prime: prime, l: l, slug: slug, push: push})
				return nil
			}

			deps := pusherDeps(prime, tt.target, stoppingPrePushCheck(), pushRun)

			if err := deps.Push(); err != nil {
				t.Fatalf("Push() error = %v; want nil", err)
			}
			if !reflect.DeepEqual(calls, []pushCall{tt.wantCall}) {
				t.Errorf("pushRun calls = %+v; want %+v", calls, []pushCall{tt.wantCall})
			}
			if (deps.Guard != nil) != tt.wantGuard {
				t.Fatalf("Guard set = %v; want %v", deps.Guard != nil, tt.wantGuard)
			}
			if tt.wantGuard {
				if err := deps.Guard(); !errors.Is(err, errPushStopped) {
					t.Errorf("Guard() error = %v; want one wrapping errPushStopped", err)
				}
			}
			if deps.Timing != (fabricengine.PushTiming{}) {
				t.Errorf("Timing = %+v; want the zero timing, which reads as the defaults", deps.Timing)
			}
		})
	}
}
