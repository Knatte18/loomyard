// pusher_test.go drives the seams the pusher verb fills over hand-built locations and a recording push.

package loomcli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
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

// TestPusherDeps covers what the pusher verb fills over the hub's reads: the push, the pre-push guard and the notice.
func TestPusherDeps(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	prime := &lymcwd.Location{RepoName: "repo", HubPath: hub, WorktreeName: "prime", AnchorRel: "."}
	pair := &lymcwd.Location{RepoName: "repo", HubPath: hub, WorktreeName: "task", AnchorRel: "."}

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
					LockWait:       fabricengine.StatusPushLockWait,
					TeardownWait:   fabricengine.StatusPushLockWait,
					OwedUnrecorded: true,
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
				push: pairteardown.RunPush{LockWait: fabricengine.StatusPushLockWait, OwedUnrecorded: true},
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
			if deps.Notify == nil {
				t.Error("Notify = nil; want the orch notice")
			}
		})
	}
}
