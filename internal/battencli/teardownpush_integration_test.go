// teardownpush_integration_test.go drives the teardown's push and level seams over hubforge pairs.
// It also drives the wired teardown row over a task pair with a bench pair.
// It covers the pushers a wired resume starts.

package battencli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestTeardownPushSeams_PushesOffLetTheRowFinishWithoutTouchingThePair drives the teardown row over a present pair.
// With git or pushes off the row reaches Done and removes the pair.
// Neither half reads the task worktree, no push is marked and no pusher is started.
func TestTeardownPushSeams_PushesOffLetTheRowFinishWithoutTouchingThePair(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")

	tests := []struct {
		name    string
		options fabricengine.SyncOptions
	}{
		{"GitOff", fabricengine.SyncOptions{SkipGit: true}},
		{"PushesOff", fabricengine.SyncOptions{SkipPush: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			locations := 0
			locateTask := func() (*lymcwd.Location, error) {
				locations++
				return nil, errors.New("the task worktree was read with pushes off")
			}
			starts := 0
			start := func(runpusher.Target) (bool, error) { starts++; return false, nil }
			push, level := teardownPushSeams(h.Location, "task1", locateTask, func() fabricengine.SyncOptions { return tt.options }, start)
			removes := 0
			deps := battenshed.TeardownDeps{
				Shutdown: func(context.Context) (string, error) { return "", nil },
				Push:     push,
				Level:    level,
				Remove:   func(context.Context) error { removes++; return nil },
			}
			lock := battenshed.PrimeLock{
				Path:    filepath.Join(t.TempDir(), "prime.lock"),
				Acquire: func() (func() error, bool, error) { return func() error { return nil }, true, nil },
			}
			producer := battenshed.NewWorktreeTeardown("Worktree-Teardown", "task1", deps, lock, t.TempDir())

			shedfake.RequireOutcome(t, producer, shedengine.Done)

			if locations != 0 {
				t.Errorf("task worktree reads = %d; want none", locations)
			}
			if removes != 1 {
				t.Errorf("removals = %d; want 1", removes)
			}
			if starts != 0 {
				t.Errorf("pusher starts = %d; want none", starts)
			}
			if entry, _, err := pushledger.Read(h.Path, pushledger.RecordsKey(h.PairRecordsSibling("task1"))); err != nil || entry.PushNow {
				t.Errorf("pair ledger entry = %+v, %v; want no push-now mark", entry, err)
			}
		})
	}
}

// TestWiredTeardown_RemovesTheBenchPairsBeforeTheTaskPair drives the wired teardown row.
// The hub holds a task pair and one of its bench pairs.
// A bench pair whose archive push origin refuses halts the row with the task pair still standing.
func TestWiredTeardown_RemovesTheBenchPairsBeforeTheTaskPair(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const (
		task  = "task1"
		bench = "task1-b1"
	)
	tests := []struct {
		name         string
		refuseBench  bool
		wantOutcome  shedengine.Outcome
		wantStanding bool
	}{
		{"BothPairsAreRemoved", false, shedengine.Done, false},
		{"ARefusedBenchArchiveKeepsTheTaskPair", true, shedengine.Stuck, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, task)
			hubforge.AddPair(t, h, bench)
			if tt.refuseBench {
				gitkit.CommitFile(t, h.PairCodeWorktree(bench), "local.txt", "local\n", "a commit origin lacks")
				hook := filepath.Join(h.CodeBare, "hooks", "pre-receive")
				if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
					t.Fatalf("create the hooks directory: %v", err)
				}
				script := "#!/bin/sh\nwhile read old new ref; do\n  case \"$ref\" in *" + bench + "*) exit 1 ;; esac\ndone\n"
				if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
					t.Fatalf("write the pre-receive hook: %v", err)
				}
			}
			c := wireForHub(t, h, task, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return shedengine.Status{State: shedengine.StateDone}, true, nil
			})
			producer := battenshed.NewWorktreeTeardown("Worktree-Teardown", task, c.env.Teardown, c.env.PrimeLock, c.env.ScratchDir)

			outcome, pointer, err := producer.Call(context.Background())

			if err != nil || outcome != tt.wantOutcome {
				t.Fatalf("Worktree-Teardown = %q, reason %q, error %v; want %q", outcome, pointer.Reason, err, tt.wantOutcome)
			}
			if tt.refuseBench && !strings.Contains(pointer.Reason, "the bench pairs of") {
				t.Errorf("Stuck reason = %q; want it to name the bench pairs", pointer.Reason)
			}
			for _, slug := range []string{task, bench} {
				if got := pathExists(h.PairCodeWorktree(slug)); got != tt.wantStanding {
					t.Errorf("pair %s standing = %v; want %v", slug, got, tt.wantStanding)
				}
			}
		})
	}
}

// TestTeardownPushSeams_PushWakesThePairsPusher asserts Push marks the pair's push as wanted now.
// It also asserts that Push asks the real pusher start for the pair.
// Level must read false while the run's landing stands, though origin holds both tips.
func TestTeardownPushSeams_PushWakesThePairsPusher(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	var asked []runpusher.Target
	start := func(target runpusher.Target) (bool, error) {
		asked = append(asked, target)
		return runpusher.Start(target)
	}
	push, level := teardownPushSeams(h.Location, "task1", taskWorktreeLocator(h.Location, "task1"), func() fabricengine.SyncOptions { return fabricengine.SyncOptions{} }, start)

	if err := push(context.Background()); err != nil {
		t.Fatalf("push() = %v; want nil", err)
	}

	entry, found, err := pushledger.Read(h.Path, pushledger.RecordsKey(h.PairRecordsSibling("task1")))
	if err != nil || !found || !entry.PushNow || entry.Slug != "task1" || entry.Worktree != h.PairCodeWorktree("task1") {
		t.Errorf("pair ledger entry = %+v, %v, %v; want the push-now mark with the pair's worktree and slug", entry, found, err)
	}
	if len(asked) != 1 || asked[0].Location.WorktreeName != "task1" || asked[0].Slug != "task1" || !asked[0].Pair {
		t.Errorf("pusher targets = %+v; want the pair's target alone", asked)
	}
	if got, err := level(); err != nil || !got {
		t.Fatalf("level() with both tips on origin = %v, %v; want true, nil", got, err)
	}

	err = pushledger.Update(h.Path, pushledger.LandingKey, time.Unix(1, 0), func(cur pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
		cur.Landings = []pushledger.Landing{{Run: "task1", Tip: "tip", ParentBranch: "main", At: time.Unix(1, 0)}}
		return cur, true, nil
	})
	if err != nil {
		t.Fatalf("record the landing: %v", err)
	}
	if got, err := level(); err != nil || got {
		t.Errorf("level() with the landing standing = %v, %v; want false, nil", got, err)
	}
}

// TestWiredStartPusher_StartsThePrimesPusherAndAStandingPairs asserts which targets a wired resume asks a pusher for.
// It is serial because wiring sets the process-global pause gate slug.
func TestWiredStartPusher_StartsThePrimesPusherAndAStandingPairs(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location

	tests := []struct {
		name string
		slug string
		want []string
	}{
		{"PairStandsStartsThePrimesAndThePairs", "task1", []string{prime.WorktreeName + "/", "task1/task1"}},
		{"PairAbsentStartsThePrimesAlone", "ghost", []string{prime.WorktreeName + "/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var started []string
			c := &battenCLI{pusherStart: func(target runpusher.Target) (bool, error) {
				started = append(started, target.Location.WorktreeName+"/"+target.Slug)
				return false, nil
			}}
			if err := c.wire(prime, tt.slug); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}

			c.startPusher()

			if !slices.Equal(started, tt.want) {
				t.Errorf("pushers asked for = %q; want %q", started, tt.want)
			}
		})
	}
}
