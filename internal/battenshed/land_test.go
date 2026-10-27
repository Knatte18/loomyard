//go:build tier0

// land_test.go covers NewLand over fake seams, a fake clock and a recording Notify:
// the landing dispositions, every failing step, the dirty-prime and land-lock waits and the records-stop notice.

package battenshed

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// landFixture holds the fake seams of one Land row and what they recorded.
// The knobs are set before Call; an answer list repeats its last entry once exhausted.
type landFixture struct {
	scratchDir string

	parent, prime       string
	parentErr, primeErr error
	dirty               [][]string
	dirtyErr            error
	pullErr             error
	recordsStop         string
	landing             LandingSettings
	landingErr          error
	deployResult        DeployResult
	deployErr           error
	pruneErr            error
	pauseAfterSleeps    int
	pauseErr            error
	landLockBusy        int
	landLockErr         error
	primeLockBusy       int
	historyLen          int
	onSleep             func()
	// orchUnrecorded makes the orch read as having no strand recorded to receive a notice.
	orchUnrecorded bool

	calls            []string
	notices          []string
	slept            []time.Duration
	now              time.Time
	primeHeld        bool
	deployWithPrime  bool
	dirtyReads       int
	deployedCommands []string
}

func newLandFixture(t *testing.T) *landFixture {
	t.Helper()
	return &landFixture{
		scratchDir: t.TempDir(),
		parent:     "main",
		prime:      "main",
		landing:    LandingSettings{Deploy: "deploy.sh", DeployBranches: []string{"main"}},
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
		Pull: func(context.Context) (string, error) {
			f.calls = append(f.calls, "pull")
			return f.recordsStop, f.pullErr
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
			BattenHistoryLen:   func() (int, error) { return f.historyLen, nil },
			OrchStrandRecorded: func() (bool, error) { return !f.orchUnrecorded, nil },
			Sleep:              func(context.Context, time.Duration) {},
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
// A parent that is the prime's branch pulls, deploys once and prunes.
// No landing with a clean pull queues a notice.
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
		{name: "ParentOutsideDeployBranches", configure: func(f *landFixture) {
			f.parent, f.prime = "dev", "dev"
		}, wantCalls: []string{"dirty", "pull", "prune"}},
		{name: "EmptyDeployBranchesDeploysNothing", configure: func(f *landFixture) { f.landing.DeployBranches = []string{} }, wantCalls: []string{"dirty", "pull", "prune"}},
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
				t.Errorf("notices = %q; want none for a clean landing", f.notices)
			}
		})
	}
}

// TestLand_FailingStepsAreStuckWithOneNotice asserts every failing step is Stuck naming the reason and posts no notice, on the first call and on a re-entry.
func TestLand_FailingStepsAreStuckWithOneNotice(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		configure  func(f *landFixture)
		wantReason string
	}{
		{name: "DeployNonZeroExit", configure: func(f *landFixture) { f.deployResult = DeployResult{ExitCode: 2} }, wantReason: "exit code 2; log: "},
		{name: "DeployTimedOut", configure: func(f *landFixture) { f.deployResult = DeployResult{TimedOut: true} }, wantReason: "timed out after 30m0s; log: "},
		{name: "DeployShellCannotStart", configure: func(f *landFixture) { f.deployErr = boom }, wantReason: "the shell could not start (boom); log: "},
		{name: "PullRefusal", configure: func(f *landFixture) { f.pullErr = boom }, wantReason: "pull failed: boom"},
		{name: "PrimeLockWaitSpent", configure: func(f *landFixture) { f.primeLockBusy = -1 }, wantReason: "/lock/prime"},
		{name: "PruneFailure", configure: func(f *landFixture) { f.pruneErr = boom }, wantReason: "board prune failed: boom"},
		{name: "ParentBranchReadError", configure: func(f *landFixture) { f.parentErr = boom }, wantReason: "prime check failed: boom"},
		{name: "ParentBranchEmpty", configure: func(f *landFixture) { f.parent = "" }, wantReason: "carries no parent branch"},
		{name: "PrimeBranchReadError", configure: func(f *landFixture) { f.primeErr = boom }, wantReason: "prime check failed: boom"},
		{name: "PrimeDirtyReadError", configure: func(f *landFixture) { f.dirtyErr = boom }, wantReason: "uncommitted paths: boom"},
		{name: "LandingReadError", configure: func(f *landFixture) { f.landingErr = boom }, wantReason: "read the landing config: boom"},
		{name: "PauseReadErrorInLandLockWait", configure: func(f *landFixture) {
			f.landLockBusy, f.pauseErr = 100, boom
		}, wantReason: "read the pause flag: boom"},
		{name: "PauseReadErrorInDirtyPrimeWait", configure: func(f *landFixture) {
			f.dirty, f.pauseErr = [][]string{{"a.go"}}, boom
		}, wantReason: "read the pause flag: boom"},
		{name: "LandLockAcquireError", configure: func(f *landFixture) { f.landLockErr = boom }, wantReason: "acquire \"/lock/land\": boom"},
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

			if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Stuck {
				t.Fatalf("re-entry Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			if len(f.notices) != 0 {
				t.Errorf("notices = %v; want none from the row, since the hand-back tells the orch", f.notices)
			}
		})
	}
}

// TestLand_DirtyPrimeWait asserts a prime that turns clean within the bound lands, one still dirty past the bound is Stuck naming the paths, and a pause during the wait is Stuck with no notice and a reason naming the goto and run way forward.
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
		if len(f.notices) != 0 {
			t.Errorf("notices = %v; want none from the row", f.notices)
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
		for _, want := range []string{`lym batten goto myslug --to Land`, `lym batten run myslug`} {
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
		deps.Pull = func(ctx context.Context) (string, error) {
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
		if !strings.Contains(ptr.Reason, `lym batten goto myslug --to Land`) {
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

// TestLand_RecordsStopNotice asserts a landing whose pull reports a records stop still deploys and queues the one records-stop line, once across a resumed row.
// With no orch strand recorded nothing is queued and one Warn names the slug.
// It is serial: it captures the process-global logger.
func TestLand_RecordsStopNotice(t *testing.T) {
	tests := []struct {
		name           string
		orchUnrecorded bool
		wantNotice     string
		wantHeldWarns  int
	}{
		{name: "NamesTheRecordsStopAndStillDeploys", wantNotice: "[batten notice] myslug: landed; the prime's records did not pull: records diverged"},
		{name: "NoOrchStrandWarnsAndQueuesNothing", orchUnrecorded: true, wantHeldWarns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)

			f := newLandFixture(t)
			f.orchUnrecorded = tt.orchUnrecorded
			f.recordsStop = "records diverged"
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
			if !slices.Contains(f.calls, "deploy") {
				t.Errorf("calls = %v; want the deploy to run after a records stop", f.calls)
			}
			const held = "row notice held back; no orch strand is recorded"
			if got := countLogLines(buf.String(), "WARN", held); got != tt.wantHeldWarns {
				t.Errorf("WARN lines %q = %d; want %d\n%s", held, got, tt.wantHeldWarns, buf.String())
			}
			if tt.wantHeldWarns > 0 && !strings.Contains(buf.String(), "slug=myslug") {
				t.Errorf("held-back Warn does not name the slug\n%s", buf.String())
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
