// parentguard_test.go drives the parent lock guard against a real parentlock.Store in a temporary directory and fake tip reads.

package landingshed

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

var (
	guardOwnerMe    = parentlock.Owner{Slug: "me", Pair: "/p/me", RunID: "r1", PID: 222, StartTime: "t2"}
	guardOwnerOther = parentlock.Owner{Slug: "other", Pair: "/p/other", RunID: "r9", PID: 111, StartTime: "t1"}
)

// newGuardFixture returns a guard over a fresh store in which every process reads as alive, and the wait marks it set in order.
func newGuardFixture(t *testing.T) (g parentGuard, store *parentlock.Store, marks *[]string) {
	t.Helper()

	dir := t.TempDir()
	store = &parentlock.Store{
		Dir:   filepath.Join(dir, "locks"),
		Alive: func(int, string) bool { return true },
	}
	var recorded []string
	var mu sync.Mutex
	g = parentGuard{
		lock: ParentLockDeps{
			Store:    store,
			Owner:    func() (parentlock.Owner, error) { return guardOwnerMe, nil },
			WaitPath: filepath.Join(dir, "wait.json"),
		},
		parentBranch: "main",
		waitMark: func(label string, _ time.Time) error {
			mu.Lock()
			defer mu.Unlock()
			recorded = append(recorded, label)
			return nil
		},
	}
	return g, store, &recorded
}

func TestParentGuard_EnsureTakesFreeLockAndWaitsBehindAnother(t *testing.T) {
	t.Parallel()

	t.Run("a free lock is taken at once", func(t *testing.T) {
		t.Parallel()
		g, store, marks := newGuardFixture(t)

		reason, err := g.ensure(context.Background(), "Publish")
		if reason != "" || err != nil {
			t.Fatalf("ensure() = %q, %v; want held", reason, err)
		}
		if rec, found, _ := store.Read("main"); !found || !rec.Owner.SameProcess(guardOwnerMe) {
			t.Errorf("record = %+v, %v; want it owned by the run", rec, found)
		}
		if len(*marks) != 0 {
			t.Errorf("marks = %v; want none without a wait", *marks)
		}
	})

	t.Run("it waits behind another owner with the record and mark set, then is granted", func(t *testing.T) {
		t.Parallel()
		g, store, marks := newGuardFixture(t)
		if _, err := store.TryAcquire("main", guardOwnerOther); err != nil {
			t.Fatal(err)
		}
		sleeping := make(chan struct{})
		proceed := make(chan struct{})
		var once sync.Once
		g.sleep = func(context.Context, time.Duration) error {
			once.Do(func() { close(sleeping) })
			<-proceed
			return nil
		}

		type outcome struct {
			reason string
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			reason, err := g.ensure(context.Background(), "Publish")
			done <- outcome{reason, err}
		}()

		<-sleeping
		wait, found, err := parentlock.ReadWait(g.lock.WaitPath, func(int, string) bool { return true })
		if err != nil || !found || wait.Branch != "main" || wait.Holder != "other" {
			t.Errorf("wait record = %+v, %v, %v; want a wait on main held by other", wait, found, err)
		}
		if released, err := store.Release("main", guardOwnerOther); err != nil || !released {
			t.Fatalf("Release() = %v, %v; want released", released, err)
		}
		close(proceed)

		got := <-done
		if got.reason != "" || got.err != nil {
			t.Fatalf("ensure() = %q, %v; want held after the release", got.reason, got.err)
		}
		if _, found, _ := parentlock.ReadWait(g.lock.WaitPath, func(int, string) bool { return true }); found {
			t.Error("wait record still present after the wait ended")
		}
		if want := []string{"parent lock main", ""}; strings.Join(*marks, "|") != strings.Join(want, "|") {
			t.Errorf("marks = %q; want set then cleared %q", *marks, want)
		}
	})
}

func TestParentGuard_EnsureReasonsAndErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange leaves the guard and store in the state under test and returns the context to wait in.
		arrange    func(t *testing.T, g *parentGuard, store *parentlock.Store) context.Context
		wantReason string
		wantErr    error
	}{
		{
			name: "a pause during the wait returns the paused reason and clears the wait",
			arrange: func(t *testing.T, g *parentGuard, store *parentlock.Store) context.Context {
				if _, err := store.TryAcquire("main", guardOwnerOther); err != nil {
					t.Fatal(err)
				}
				g.lock.PauseRequested = func() (bool, error) { return true, nil }
				return context.Background()
			},
			wantReason: `halted while waiting for the parent lock on "main" held by other, as "lyx loom pause --after Publish" asked; way forward: run "lyx loom goto --to Publish", then "lyx loom resume"`,
		},
		{
			name: "an undecodable record returns the release-verb reason",
			arrange: func(t *testing.T, g *parentGuard, store *parentlock.Store) context.Context {
				if err := os.MkdirAll(store.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				recordFile := filepath.Join(store.Dir, hex.EncodeToString([]byte("main"))+".json")
				if err := os.WriteFile(recordFile, []byte("{nope"), 0o644); err != nil {
					t.Fatal(err)
				}
				return context.Background()
			},
			wantReason: `way forward: run "lyx batten lock release main" from the prime, then "lyx loom resume" in the task worktree`,
		},
		{
			name: "a cancelled context is an error",
			arrange: func(t *testing.T, g *parentGuard, store *parentlock.Store) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, store, _ := newGuardFixture(t)
			ctx := tt.arrange(t, &g, store)

			reason, err := g.ensure(ctx, "Publish")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ensure() error = %v; want %v", err, tt.wantErr)
			}
			if tt.wantReason != "" && !strings.Contains(reason, tt.wantReason) {
				t.Errorf("ensure() reason = %q; want it to contain %q", reason, tt.wantReason)
			}
			if tt.wantReason == "" && reason != "" {
				t.Errorf("ensure() reason = %q; want none", reason)
			}
			if _, found, _ := parentlock.ReadWait(g.lock.WaitPath, func(int, string) bool { return true }); found {
				t.Error("wait record left behind")
			}
		})
	}
}

func TestParentGuard_ReleaseFreesOwnRecordAndLeavesAnotherProcessAlone(t *testing.T) {
	t.Parallel()

	g, store, _ := newGuardFixture(t)
	if _, err := store.TryAcquire("main", guardOwnerOther); err != nil {
		t.Fatal(err)
	}
	g.release("Publish")
	if rec, found, _ := store.Read("main"); !found || !rec.Owner.SameProcess(guardOwnerOther) {
		t.Fatalf("record = %+v, %v; want the other owner's record kept", rec, found)
	}

	if _, err := store.ForceRelease("main"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TryAcquire("main", guardOwnerMe); err != nil {
		t.Fatal(err)
	}
	g.release("Publish")
	if rec, found, _ := store.Read("main"); found {
		t.Errorf("record = %+v; want the run's own record freed", rec)
	}
}

// TestParentGuard_PauseReadFailureLogsOncePerEpisode asserts a pause-flag read that fails on every poll logs one Warn, Debug while it repeats and one Info at the next good read, and reads as not paused throughout.
// It is serial: it captures the process-global logger.
func TestParentGuard_PauseReadFailureLogsOncePerEpisode(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	logger.SetVerbosity(2)
	g, _, _ := newGuardFixture(t)
	reads := 0
	g.lock.PauseRequested = func() (bool, error) {
		reads++
		if reads <= 3 {
			return false, errors.New("unreadable")
		}
		return false, nil
	}
	var failures logger.Episode

	for i := 0; i < 4; i++ {
		if g.pauseRequested(&failures) {
			t.Errorf("pauseRequested() call %d = true; want false", i+1)
		}
	}

	const failure = "could not read the pause flag during the parent lock wait"
	for _, want := range []struct {
		level, text string
		count       int
	}{
		{"WARN", failure, 1},
		{"DEBUG", failure, 2},
		{"INFO", "the pause flag reads again during the parent lock wait", 1},
	} {
		if got := countLogLines(buf.String(), want.level, want.text); got != want.count {
			t.Errorf("%s lines containing %q = %d; want %d\n%s", want.level, want.text, got, want.count, buf.String())
		}
	}
}

// countLogLines counts the lines of log at level that contain text.
func countLogLines(log, level, text string) int {
	count := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "level="+level) && strings.Contains(line, text) {
			count++
		}
	}
	return count
}
