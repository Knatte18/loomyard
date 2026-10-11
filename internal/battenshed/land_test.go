// land_test.go covers NewLand over fake seams, a fake clock and a recording Notify:
// the landing dispositions, every failing step, the dirty-prime and land-lock waits and the ready notice.

package battenshed

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// readyAnswer is one answer of the fake ReadyTasks seam.
type readyAnswer struct {
	slugs []string
	err   error
}

// landFixture holds the fake seams of one Land row and what they recorded.
// The knobs are set before Call; an answer list repeats its last entry once exhausted.
type landFixture struct {
	scratchDir string

	parent, prime       string
	parentErr, primeErr error
	dirty               [][]string
	dirtyErr            error
	pullErr             error
	landing             LandingSettings
	landingErr          error
	deployResult        DeployResult
	deployErr           error
	ready               []readyAnswer
	pruneErr            error
	pauseAfterSleeps    int
	pauseErr            error
	landLockBusy        int
	landLockErr         error
	primeLockBusy       int
	historyLen          int
	onSleep             func()

	calls            []string
	notices          []string
	slept            []time.Duration
	now              time.Time
	primeHeld        bool
	deployWithPrime  bool
	dirtyReads       int
	readyReads       int
	deployedCommands []string
}

func newLandFixture(t *testing.T) *landFixture {
	t.Helper()
	return &landFixture{
		scratchDir: t.TempDir(),
		parent:     "main",
		prime:      "main",
		landing:    LandingSettings{Deploy: "deploy.sh", RequirePRToBase: []string{"main"}},
		ready:      []readyAnswer{{}},
		now:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (f *landFixture) sleep(ctx context.Context, d time.Duration) {
	f.slept = append(f.slept, d)
	f.now = f.now.Add(d)
	if f.onSleep != nil {
		f.onSleep()
	}
}

func (f *landFixture) deps() LandDeps {
	return LandDeps{
		ParentBranch: func() (string, error) { return f.parent, f.parentErr },
		PrimeBranch:  func() (string, error) { return f.prime, f.primeErr },
		PrimeDirty: func() ([]string, error) {
			f.calls = append(f.calls, "dirty")
			i := min(f.dirtyReads, len(f.dirty)-1)
			f.dirtyReads++
			if i < 0 {
				return nil, f.dirtyErr
			}
			return f.dirty[i], f.dirtyErr
		},
		Pull: func(context.Context) error {
			f.calls = append(f.calls, "pull")
			return f.pullErr
		},
		Landing: func() (LandingSettings, error) { return f.landing, f.landingErr },
		RunDeploy: func(_ context.Context, command, logPath string, timeout time.Duration) (DeployResult, error) {
			f.calls = append(f.calls, "deploy")
			f.deployedCommands = append(f.deployedCommands, command)
			f.deployWithPrime = f.deployWithPrime || f.primeHeld
			if !strings.HasPrefix(logPath, f.scratchDir) || timeout != deployTimeout {
				return DeployResult{}, errors.New("deploy told the wrong log path or timeout")
			}
			return f.deployResult, f.deployErr
		},
		ReadyTasks: func() ([]string, error) {
			i := min(f.readyReads, len(f.ready)-1)
			f.readyReads++
			return f.ready[i].slugs, f.ready[i].err
		},
		PruneBoard: func() error {
			f.calls = append(f.calls, "prune")
			return f.pruneErr
		},
		LandLock: PrimeLock{
			Path: "/lock/land",
			Acquire: func() (func() error, bool, error) {
				if f.landLockErr != nil {
					return nil, false, f.landLockErr
				}
				if f.landLockBusy != 0 {
					f.landLockBusy--
					return nil, false, nil
				}
				return func() error { return nil }, true, nil
			},
		},
		PauseRequested: func() (bool, error) {
			return f.pauseAfterSleeps > 0 && len(f.slept) >= f.pauseAfterSleeps, f.pauseErr
		},
		Sleep: f.sleep,
		Now:   func() time.Time { return f.now },
		Notices: RowNoticeDeps{
			Notify: func(_ context.Context, line string) (bool, error) {
				f.notices = append(f.notices, line)
				return true, nil
			},
			BattenHistoryLen: func() (int, error) { return f.historyLen, nil },
			Sleep:            func(context.Context, time.Duration) {},
		},
	}
}

func (f *landFixture) producer() shedengine.ShedProducer {
	primeLock := PrimeLock{
		Path: "/lock/prime",
		Acquire: func() (func() error, bool, error) {
			if f.primeLockBusy != 0 {
				if f.primeLockBusy > 0 {
					f.primeLockBusy--
				}
				return nil, false, nil
			}
			f.primeHeld = true
			return func() error {
				f.primeHeld = false
				return nil
			}, true, nil
		},
		Sleep: func(context.Context, time.Duration) {},
	}
	return NewLand("Land", "myslug", f.deps(), primeLock, f.scratchDir)
}

// TestLand_Dispositions asserts which of the pull, the deploy and the prune the row runs for each landing.
// A parent that is the prime's branch pulls, deploys once and prunes with no notice.
// Another parent skips the pull and the deploy without reading the prime's tree, a parent outside the deploy list pulls only, and an empty deploy command runs nothing.
func TestLand_Dispositions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(f *landFixture)
		wantCalls []string
	}{
		{name: "ParentIsPrimeBranch", configure: func(f *landFixture) {}, wantCalls: []string{"dirty", "pull", "deploy", "prune"}},
		{name: "OtherParentSkipsPullAndDeploy", configure: func(f *landFixture) { f.parent = "feature" }, wantCalls: []string{"prune"}},
		{name: "ParentOutsideRequirePRToBase", configure: func(f *landFixture) {
			f.parent, f.prime = "dev", "dev"
		}, wantCalls: []string{"dirty", "pull", "prune"}},
		{name: "EmptyDeployRunsNothing", configure: func(f *landFixture) { f.landing.Deploy = "" }, wantCalls: []string{"dirty", "pull", "prune"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLandFixture(t)
			tt.configure(f)
			if outcome, _, err := f.producer().Call(context.Background()); err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if !slices.Equal(f.calls, tt.wantCalls) {
				t.Errorf("calls = %v; want %v", f.calls, tt.wantCalls)
			}
			if len(f.notices) != 0 {
				t.Errorf("notices = %v; want none", f.notices)
			}
		})
	}
}

// TestLand_FailingStepsAreStuckWithOneNotice asserts every failing step is Stuck naming the step and the reason, queues one notice naming the step, and queues no second one on a re-entry at the same history length.
// It also asserts a ready read that failed after the prune sends the ready notice when the re-entry succeeds.
func TestLand_FailingStepsAreStuckWithOneNotice(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		configure  func(f *landFixture)
		wantStep   string
		wantReason string
	}{
		{name: "DeployNonZeroExit", configure: func(f *landFixture) { f.deployResult = DeployResult{ExitCode: 2} }, wantStep: "deploy", wantReason: "exit code 2; log: "},
		{name: "DeployTimedOut", configure: func(f *landFixture) { f.deployResult = DeployResult{TimedOut: true} }, wantStep: "deploy", wantReason: "timed out after 30m0s; log: "},
		{name: "DeployShellCannotStart", configure: func(f *landFixture) { f.deployErr = boom }, wantStep: "deploy", wantReason: "the shell could not start (boom); log: "},
		{name: "PullRefusal", configure: func(f *landFixture) { f.pullErr = boom }, wantStep: "pull", wantReason: "pull failed: boom"},
		{name: "PrimeLockWaitSpent", configure: func(f *landFixture) { f.primeLockBusy = -1 }, wantStep: "prime lock", wantReason: "/lock/prime"},
		{name: "ReadyTasksErrorAtSnapshot", configure: func(f *landFixture) { f.ready = []readyAnswer{{err: boom}} }, wantStep: "ready snapshot", wantReason: "ready snapshot failed: boom"},
		{name: "PruneFailure", configure: func(f *landFixture) { f.pruneErr = boom }, wantStep: "board prune", wantReason: "board prune failed: boom"},
		{name: "ReadyTasksErrorAfterPrune", configure: func(f *landFixture) { f.ready = []readyAnswer{{slugs: []string{"a"}}, {err: boom}} }, wantStep: "ready read", wantReason: "ready read failed: boom"},
		{name: "ParentBranchReadError", configure: func(f *landFixture) { f.parentErr = boom }, wantStep: "prime check", wantReason: "prime check failed: boom"},
		{name: "ParentBranchEmpty", configure: func(f *landFixture) { f.parent = "" }, wantStep: "prime check", wantReason: "carries no parent branch"},
		{name: "PrimeBranchReadError", configure: func(f *landFixture) { f.primeErr = boom }, wantStep: "prime check", wantReason: "prime check failed: boom"},
		{name: "PrimeDirtyReadError", configure: func(f *landFixture) { f.dirtyErr = boom }, wantStep: "prime check", wantReason: "uncommitted paths: boom"},
		{name: "LandingReadError", configure: func(f *landFixture) { f.landingErr = boom }, wantStep: "deploy", wantReason: "read the landing config: boom"},
		{name: "PauseReadErrorInLandLockWait", configure: func(f *landFixture) {
			f.landLockBusy, f.pauseErr = 100, boom
		}, wantStep: "land lock", wantReason: "read the pause flag: boom"},
		{name: "PauseReadErrorInDirtyPrimeWait", configure: func(f *landFixture) {
			f.dirty, f.pauseErr = [][]string{{"a.go"}}, boom
		}, wantStep: "prime check", wantReason: "read the pause flag: boom"},
		{name: "LandLockAcquireError", configure: func(f *landFixture) { f.landLockErr = boom }, wantStep: "land lock", wantReason: "acquire \"/lock/land\": boom"},
		{name: "SnapshotPathOccupiedByDirectory", configure: func(f *landFixture) {
			if err := os.Mkdir(f.scratchDir+"/Land"+readySnapshotFileSuffix, 0o755); err != nil {
				t.Fatal(err)
			}
		}, wantStep: "ready read", wantReason: "read the ready snapshot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLandFixture(t)
			tt.configure(f)
			producer := f.producer()

			outcome, ptr, err := producer.Call(context.Background())
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			if !strings.Contains(ptr.Reason, tt.wantReason) {
				t.Errorf("reason = %q; want it to contain %q", ptr.Reason, tt.wantReason)
			}
			if len(f.notices) != 1 || !strings.Contains(f.notices[0], "Land failed at "+tt.wantStep+": ") {
				t.Fatalf("notices = %v; want one naming the failed step %q", f.notices, tt.wantStep)
			}

			if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Stuck {
				t.Fatalf("re-entry Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			if len(f.notices) != 1 {
				t.Errorf("notices after a re-entry at the same history length = %v; want still one", f.notices)
			}

			if tt.name != "ReadyTasksErrorAfterPrune" {
				return
			}
			f.ready = []readyAnswer{{slugs: []string{"a", "b"}}}
			f.readyReads = 0
			if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
				t.Fatalf("recovered Call() = %q, %v; want Done, nil", outcome, err)
			}
			if len(f.notices) != 2 || !strings.HasSuffix(f.notices[1], "landed; newly ready: b") {
				t.Errorf("notices after the recovered re-entry = %v; want the ready notice naming b", f.notices)
			}
		})
	}
}

// TestLand_DirtyPrimeWait asserts a prime that turns clean within the bound lands, one still dirty past the bound is Stuck naming the paths and notices, and a pause during the wait is Stuck with no notice and a reason naming the goto and run way forward.
func TestLand_DirtyPrimeWait(t *testing.T) {
	t.Parallel()

	t.Run("CleanWithinBound", func(t *testing.T) {
		t.Parallel()
		f := newLandFixture(t)
		f.dirty = [][]string{{"a.go"}, {"a.go"}, nil}

		if outcome, _, err := f.producer().Call(context.Background()); err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		if want := []time.Duration{primeDirtyPollInterval, primeDirtyPollInterval}; !slices.Equal(f.slept, want) {
			t.Errorf("sleeps = %v; want %v", f.slept, want)
		}
	})

	t.Run("DirtyPastBound", func(t *testing.T) {
		t.Parallel()
		f := newLandFixture(t)
		f.dirty = [][]string{{"a.go", "b.go"}}

		outcome, ptr, err := f.producer().Call(context.Background())
		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
		}
		if !strings.Contains(ptr.Reason, "a.go, b.go") {
			t.Errorf("reason = %q; want it to name the paths", ptr.Reason)
		}
		if want := int(primeDirtyWaitBound / primeDirtyPollInterval); len(f.slept) != want {
			t.Errorf("sleeps = %d; want %d", len(f.slept), want)
		}
		if len(f.notices) != 1 {
			t.Errorf("notices = %v; want one", f.notices)
		}
		if slices.Contains(f.calls, "pull") {
			t.Errorf("calls = %v; want no pull while the prime is dirty", f.calls)
		}
	})

	t.Run("PauseHonoured", func(t *testing.T) {
		t.Parallel()
		f := newLandFixture(t)
		f.dirty = [][]string{{"a.go"}}
		f.pauseAfterSleeps = 1

		outcome, ptr, err := f.producer().Call(context.Background())
		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
		}
		for _, want := range []string{`lyx batten goto myslug --to Land`, `lyx batten run myslug`} {
			if !strings.Contains(ptr.Reason, want) {
				t.Errorf("reason = %q; want it to contain %q", ptr.Reason, want)
			}
		}
		if len(f.notices) != 0 {
			t.Errorf("notices = %v; want none for a pause", f.notices)
		}
	})
}

// TestLand_LandLockWait asserts a contended land lock is polled with sleeps doubling from the base to the ceiling until it frees, the prime lock is held only around the pull, a pause ends the wait with no notice, and a cancelled context is a hard error.
func TestLand_LandLockWait(t *testing.T) {
	t.Parallel()

	t.Run("PollsUntilFreeAndHoldsPrimeLockOnlyAroundPull", func(t *testing.T) {
		t.Parallel()
		f := newLandFixture(t)
		f.landLockBusy = 7
		var primeHeldDuringPull bool
		deps := f.deps()
		pull := deps.Pull
		deps.Pull = func(ctx context.Context) error {
			primeHeldDuringPull = f.primeHeld
			return pull(ctx)
		}
		primeLock := PrimeLock{
			Path: "/lock/prime",
			Acquire: func() (func() error, bool, error) {
				f.primeHeld = true
				return func() error { f.primeHeld = false; return nil }, true, nil
			},
		}

		outcome, _, err := NewLand("Land", "myslug", deps, primeLock, f.scratchDir).Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
		if !slices.Equal(f.slept, want) {
			t.Errorf("sleeps = %v; want %v", f.slept, want)
		}
		if !primeHeldDuringPull {
			t.Error("prime lock was not held during the pull")
		}
		if f.deployWithPrime {
			t.Error("prime lock was held during the deploy")
		}
	})

	t.Run("PauseEndsTheWaitWithNoNotice", func(t *testing.T) {
		t.Parallel()
		f := newLandFixture(t)
		f.landLockBusy = 100
		f.pauseAfterSleeps = 2

		outcome, ptr, err := f.producer().Call(context.Background())
		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
		}
		if !strings.Contains(ptr.Reason, `lyx batten goto myslug --to Land`) {
			t.Errorf("reason = %q; want the goto way forward", ptr.Reason)
		}
		if len(f.notices) != 0 {
			t.Errorf("notices = %v; want none for a pause", f.notices)
		}
	})

	t.Run("CancelledContextIsAHardError", func(t *testing.T) {
		t.Parallel()
		f := newLandFixture(t)
		f.landLockBusy = 100
		ctx, cancel := context.WithCancel(context.Background())
		f.onSleep = cancel

		_, _, err := f.producer().Call(ctx)
		if err == nil || !strings.Contains(err.Error(), "context cancelled during run") {
			t.Fatalf("Call() error = %v; want the cancelled-during-run diagnosis", err)
		}
		if len(f.notices) != 0 {
			t.Errorf("notices = %v; want none for a cancelled context", f.notices)
		}
	})
}

// TestLand_ReadyNotice asserts the ready notice names exactly the tasks the landing made ready:
// one notice for a landing that readies a dependant, none when nothing is readied, one on a restart after the prune with the snapshot present, none repeated on a re-entry after it was queued, and never a task already in the snapshot.
func TestLand_ReadyNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		snapshot   string
		ready      []readyAnswer
		wantNotice string
	}{
		{name: "ReadiesADependant", ready: []readyAnswer{{slugs: []string{"a"}}, {slugs: []string{"a", "b"}}}, wantNotice: "[batten notice] myslug: landed; newly ready: b"},
		{name: "ReadiesNothing", ready: []readyAnswer{{slugs: []string{"a"}}, {slugs: []string{"a"}}}},
		{name: "RestartAfterPruneWithSnapshot", snapshot: "a\n", ready: []readyAnswer{{slugs: []string{"a", "c"}}}, wantNotice: "[batten notice] myslug: landed; newly ready: c"},
		{name: "TaskAlreadyInSnapshotIsNotNamed", ready: []readyAnswer{{slugs: []string{"a", "b"}}, {slugs: []string{"c", "b", "a"}}}, wantNotice: "[batten notice] myslug: landed; newly ready: c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLandFixture(t)
			f.ready = tt.ready
			if tt.snapshot != "" {
				if err := os.WriteFile(f.scratchDir+"/Land"+readySnapshotFileSuffix, []byte(tt.snapshot), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			producer := f.producer()

			if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			var want []string
			if tt.wantNotice != "" {
				want = []string{tt.wantNotice}
			}
			if !slices.Equal(f.notices, want) {
				t.Fatalf("notices = %q; want %q", f.notices, want)
			}

			if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
				t.Fatalf("re-entry Call() = %q, %v; want Done, nil", outcome, err)
			}
			if !slices.Equal(f.notices, want) {
				t.Errorf("notices after a re-entry = %q; want %q, none repeated", f.notices, want)
			}
		})
	}
}
