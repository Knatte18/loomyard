// relay_test.go covers the relay process, its starter and the waits of a start.
// It runs over the in-memory seams of inbound_test.go and temp-dir files.

package slackengine_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// fakePID is the process id the relay under test reports for itself.
	fakePID = 7
	// gonePID is a process id no process holds.
	gonePID = 2147483000
)

// readAllCount returns how many times the mailbox's ReadAll was called.
func (m *fakeMailbox) readAllCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readAllCalls
}

// writeRelayStateJSON writes content as the relay state file of geom.
func writeRelayStateJSON(t *testing.T, geom slackengine.Geometry, content string) {
	t.Helper()
	if err := os.MkdirAll(geom.Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	if err := os.WriteFile(geom.RelayStatePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write the relay state = %v", err)
	}
}

// holdRelayLock takes the relay lock of geom and releases it at cleanup, standing for another relay.
func holdRelayLock(t *testing.T, geom slackengine.Geometry) *lock.FileLock {
	t.Helper()
	if err := os.MkdirAll(geom.Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock = %v, %v; want the lock", acquired, err)
	}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { _ = held.Release() }) })
	return held
}

// relayRun is a relay over a configured harness, run synchronously until a test stops it.
type relayRun struct {
	*harness
	deps       slackengine.RelayDeps
	registered atomic.Int32
	waits      atomic.Int32
	mu         sync.Mutex
	cancel     context.CancelFunc
}

// newRelayRun returns a relay whose outbound read blocks as a long poll with no mail does.
func newRelayRun(t *testing.T) *relayRun {
	t.Helper()
	h := newHarness(t)
	block := make(chan struct{})
	h.mailbox.blockReadWait = block
	t.Cleanup(func() {
		// The blocked outbound read wakes once the block closes and takes one more wait;
		// no hook of a finished test may run then.
		h.clock.mu.Lock()
		h.clock.onWait = nil
		h.clock.mu.Unlock()
		close(block)
	})
	r := &relayRun{harness: h}
	r.deps = slackengine.RelayDeps{
		Mailbox:       h.mailbox,
		Slack:         h.slack,
		Hub:           h.hub.hub(),
		Clock:         h.clock,
		PID:           func() int { return fakePID },
		CheckBinary:   func(func(), func(error)) {},
		ReapInherited: func() error { return nil },
		Register:      func() error { r.registered.Add(1); return nil },
	}
	return r
}

func (r *relayRun) setPID(pid int) { r.deps.PID = func() int { return pid } }

// onWait runs hook after each wait the relay takes on the clock, with the wait's 1-based number and length.
func (r *relayRun) onWait(hook func(n int, d time.Duration)) {
	r.waits.Store(0)
	r.clock.onWait = func(d time.Duration) { hook(int(r.waits.Add(1)), d) }
}

// stop ends the run in progress.
func (r *relayRun) stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	cancel()
}

// run runs the relay until it returns.
func (r *relayRun) run() error {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
	defer cancel()
	return slackengine.RunRelay(ctx, r.geom, r.deps)
}

func (r *relayRun) relayState() slackengine.RelayState {
	r.t.Helper()
	relayState, _, err := slackengine.ReadRelayState(r.geom)
	if err != nil {
		r.t.Fatalf("ReadRelayState = %v", err)
	}
	return relayState
}

// stopAtFirstWait ends the run at the relay's first wait, after one pass of its loop.
func (r *relayRun) stopAtFirstWait() {
	r.onWait(func(int, time.Duration) { r.stop() })
}

func TestRunRelay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("HubWithNoTokenOrNoSetupIsRefusedAndTakesNoLock", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		if err := os.Remove(r.geom.TokenPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		if err := r.run(); err == nil || !strings.Contains(err.Error(), "lym slack login") {
			t.Errorf("RunRelay(no token) = %v; want an error naming lym slack login", err)
		}
		if err := slackengine.WriteToken(r.geom.TokenPath, "xoxb-secret"); err != nil {
			t.Fatalf("WriteToken = %v", err)
		}
		if err := os.Remove(r.geom.SetupPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		if err := r.run(); err == nil || !strings.Contains(err.Error(), "lym slack setup") {
			t.Errorf("RunRelay(no setup) = %v; want an error naming lym slack setup", err)
		}
		if _, err := os.Stat(r.geom.LockPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(lock) = %v; want no lock file", err)
		}
	})

	t.Run("FirstReadIsAcceptedAndRegisteredBeforeTheFirstSlackCall", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		var once sync.Once
		var firstReadAtFirstCall time.Time
		var registeredAtFirstCall int32
		r.slack.onCall = func() {
			once.Do(func() {
				firstReadAtFirstCall = r.relayState().FirstReadAt
				registeredAtFirstCall = r.registered.Load()
			})
		}
		r.stopAtFirstWait()
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		if registeredAtFirstCall != 1 || firstReadAtFirstCall.IsZero() {
			t.Errorf("at the first Slack call: registered %d, FirstReadAt %v; want one registration and a recorded first read", registeredAtFirstCall, firstReadAtFirstCall)
		}
		if relayState := r.relayState(); relayState.PID != fakePID || r.mailbox.readAllCount() != 1 {
			t.Errorf("state pid %d after %d first reads; want pid %d and one first read", relayState.PID, r.mailbox.readAllCount(), fakePID)
		}
	})

	t.Run("FirstReadWithUnreadMailIsAcceptedAndLeavesTheMailUnread", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.mailbox.scriptReadAll(readStep{messages: []creelengine.Message{{ID: 1, From: "ly:orch", Body: "waiting"}}})
		r.stopAtFirstWait()
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		if len(r.mailbox.readAllAcks) != 0 {
			t.Errorf("first read acknowledged %v; want the mail left unread", r.mailbox.readAllAcks)
		}
		if r.slack.callCount() == 0 || r.relayState().FirstReadAt.IsZero() {
			t.Errorf("the relay did not go on to poll after an accepted first read")
		}
	})

	t.Run("FirstReadRefusedFromAStrandPaneReturnsTheErrorAndFreesTheLock", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.mailbox.scriptReadAll(readStep{err: fmt.Errorf("creel: refused: %w", creelengine.ErrProgramClaimInStrandPane)})
		err := r.run()
		if !errors.Is(err, slackengine.ErrRelayInStrandPane) || !errors.Is(err, creelengine.ErrProgramClaimInStrandPane) || !strings.Contains(err.Error(), "lym slack start") {
			t.Fatalf("RunRelay = %v; want ErrRelayInStrandPane wrapping creel's refusal and naming lym slack start", err)
		}
		if r.slack.callCount() != 0 || r.relayState().FirstReadRefusal == "" {
			t.Errorf("Slack calls %d, refusal %q; want no call and the refusal recorded", r.slack.callCount(), r.relayState().FirstReadRefusal)
		}
		if live, err := slackengine.RelayLive(r.geom); err != nil || live {
			t.Errorf("RelayLive = %v, %v; want the lock free", live, err)
		}
		r.setPID(8)
		r.stopAtFirstWait()
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay of another pid = %v", err)
		}
		if relayState := r.relayState(); relayState.PID != 8 || relayState.FirstReadAt.IsZero() {
			t.Errorf("state after the second relay = %+v; want pid 8 with a first read", relayState)
		}
	})

	t.Run("OtherFirstReadRefusalKeepsTheLockAndRetries", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.mailbox.scriptReadAll(readStep{err: errors.New("creel: the pane process is unknown")})
		var liveDuringRetry bool
		var callsDuringRetry int
		var refusalDuringRetry string
		r.onWait(func(n int, _ time.Duration) {
			if n == 1 {
				liveDuringRetry, _ = slackengine.RelayLive(r.geom)
				callsDuringRetry = r.slack.callCount()
				refusalDuringRetry = r.relayState().FirstReadRefusal
				return
			}
			r.stop()
		})
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		if !liveDuringRetry || callsDuringRetry != 0 || !strings.Contains(refusalDuringRetry, "pane process is unknown") {
			t.Errorf("during the retry: live %v, Slack calls %d, refusal %q; want the lock held, no call and the refusal recorded", liveDuringRetry, callsDuringRetry, refusalDuringRetry)
		}
		if relayState := r.relayState(); relayState.FirstReadRefusal != "" || relayState.FirstReadAt.IsZero() || r.slack.callCount() == 0 {
			t.Errorf("after the retry: state %+v, Slack calls %d; want the refusal cleared, a first read and a poll", relayState, r.slack.callCount())
		}
	})

	t.Run("SecondRelayWhileTheFirstHoldsTheLockReturnsBeforeAnyRead", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		var registeredByOther atomic.Int32
		other := r.deps
		other.PID = func() int { return 9 }
		other.Register = func() error { registeredByOther.Add(1); return nil }
		var otherErr error
		readsBefore, readsAfter := 0, 0
		r.onWait(func(int, time.Duration) {
			readsBefore = r.mailbox.readAllCount()
			otherErr = slackengine.RunRelay(context.Background(), r.geom, other)
			readsAfter = r.mailbox.readAllCount()
			r.stop()
		})
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		if otherErr != nil || readsBefore != readsAfter || registeredByOther.Load() != 0 {
			t.Errorf("second relay: error %v, reads %d -> %d, registrations %d; want a quiet return before any read", otherErr, readsBefore, readsAfter, registeredByOther.Load())
		}
	})

	t.Run("LockHandling", func(t *testing.T) {
		t.Parallel()

		t.Run("ReexecWaitsForTheLockItsOldImageReleased", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			r.deps.ReexecLockWait = 10 * time.Second
			writeRelayStateJSON(t, r.geom, fmt.Sprintf(`{"pid":%d}`, fakePID))
			held := holdRelayLock(t, r.geom)
			time.AfterFunc(2*lock.ProbeGrace, func() { _ = held.Release() })
			r.stopAtFirstWait()
			if err := r.run(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if r.relayState().FirstReadAt.IsZero() {
				t.Errorf("the re-exec gave up on the lock before its old image released it")
			}
		})

		t.Run("FreshRunAgainstAHeldLockReturns", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.deps.ReexecLockWait = 10 * time.Second
			writeRelayStateJSON(t, r.geom, fmt.Sprintf(`{"pid":%d}`, fakePID))
			holdRelayLock(t, r.geom)
			r.setPID(8)
			if err := r.run(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if r.mailbox.readAllCount() != 0 {
				t.Errorf("a fresh relay read creel with the lock held by another")
			}
		})

		t.Run("LockHeldOnlyForAProbesGraceIsTaken", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			held := holdRelayLock(t, r.geom)
			time.AfterFunc(lock.ProbeGrace/5, func() { _ = held.Release() })
			r.setPID(8)
			r.stopAtFirstWait()
			if err := r.run(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if r.relayState().FirstReadAt.IsZero() {
				t.Errorf("a fresh relay gave up on a lock a probe held for a moment")
			}
		})
	})

	t.Run("ReplacedBinary", func(t *testing.T) {
		t.Parallel()

		// replaced returns a CheckBinary that stands for a replaced binary on its first check:
		// it releases the lock in beforeExec, runs during, and then reports a failed exec.
		replaced := func(during func()) func(func(), func(error)) {
			calls := 0
			return func(beforeExec func(), failed func(error)) {
				calls++
				if calls > 1 {
					return
				}
				beforeExec()
				during()
				failed(errors.New("exec: permission denied"))
			}
		}
		logged := func(t *testing.T, r *relayRun) string {
			t.Helper()
			data, err := os.ReadFile(r.geom.LogPath)
			if err != nil {
				t.Fatalf("read the relay log = %v", err)
			}
			return string(data)
		}

		t.Run("FailedExecIsLoggedAndTheLockRetaken", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			var liveDuringExec, liveAfter bool
			r.deps.CheckBinary = replaced(func() { liveDuringExec, _ = slackengine.RelayLive(r.geom) })
			r.onWait(func(int, time.Duration) {
				liveAfter, _ = slackengine.RelayLive(r.geom)
				r.stop()
			})
			if err := r.run(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if liveDuringExec || !liveAfter || !strings.Contains(logged(t, r), "permission denied") {
				t.Errorf("live during exec %v, live after %v, log %q; want the lock released in beforeExec, retaken in failed and the exec error logged", liveDuringExec, liveAfter, logged(t, r))
			}
		})

		t.Run("LockHeldOnlyForAProbesGraceIsRetaken", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			var liveAfter bool
			r.deps.CheckBinary = replaced(func() {
				probe, acquired, err := lock.TryAcquireWriteLock(r.geom.LockPath)
				if err != nil || !acquired {
					t.Errorf("TryAcquireWriteLock = %v, %v; want the lock the relay released", acquired, err)
					return
				}
				time.AfterFunc(lock.ProbeGrace/5, func() { _ = probe.Release() })
			})
			r.onWait(func(int, time.Duration) {
				liveAfter, _ = slackengine.RelayLive(r.geom)
				r.stop()
			})
			if err := r.run(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if !liveAfter {
				t.Errorf("the relay did not retake a lock a probe held for a moment")
			}
		})

		t.Run("LockKeptByAnotherRelayEndsThisOne", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			r.deps.CheckBinary = replaced(func() {
				other, acquired, err := lock.TryAcquireWriteLock(r.geom.LockPath)
				if err != nil || !acquired {
					t.Errorf("TryAcquireWriteLock = %v, %v; want the lock the relay released", acquired, err)
					return
				}
				t.Cleanup(func() { _ = other.Release() })
			})
			r.onWait(func(int, time.Duration) { t.Errorf("the relay went on polling after another relay took its lock") })
			if err := r.run(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if !strings.Contains(logged(t, r), "another relay holds the lock") {
				t.Errorf("log %q; want the exit logged", logged(t, r))
			}
		})
	})

	t.Run("SuccessfulSlackCallStampsLastSlackOK", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.stopAtFirstWait()
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		if r.relayState().LastSlackOK.IsZero() {
			t.Errorf("LastSlackOK is zero after a successful Slack call")
		}
	})

	t.Run("FirstReadKeepsTheStartersFields", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		writeRelayStateJSON(t, r.geom, `{"start_failures":4,"next_start_at":"2026-11-05T12:30:00Z","last_spawn_at":"2026-11-05T12:00:00Z"}`)
		r.stopAtFirstWait()
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		relayState := r.relayState()
		if relayState.FirstReadAt.IsZero() || relayState.StartFailures != 4 ||
			!relayState.NextStartAt.Equal(time.Date(2026, 11, 5, 12, 30, 0, 0, time.UTC)) ||
			!relayState.LastSpawnAt.Equal(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)) {
			t.Errorf("state = %+v; want a first read with StartFailures, NextStartAt and LastSpawnAt as they stood", relayState)
		}
	})

	t.Run("InboundWaitDoublesToTheCeilingResetsOnActivityAndARateLimitRaisesTheFloor", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		next := 100
		newTS := func() string {
			next++
			return fmt.Sprintf("%d.000001", next)
		}
		r.onWait(func(n int, _ time.Duration) {
			switch n {
			case 5:
				r.say(newTS(), "hello")
			case 7:
				r.slack.setHistoryErr(&slackengine.RateLimitError{Method: "conversations.history", RetryAfter: 3 * time.Second})
			case 8:
				r.slack.setHistoryErr(nil)
				r.say(newTS(), "again")
			case 9:
				r.stop()
			}
		})
		if err := r.run(); err != nil {
			t.Fatalf("RunRelay = %v", err)
		}
		want := []time.Duration{
			2 * slackengine.Floor, 4 * slackengine.Floor, 8 * slackengine.Floor, slackengine.Ceiling, slackengine.Ceiling,
			slackengine.Floor, 2 * slackengine.Floor,
			3 * time.Second,
			2 * slackengine.Floor,
		}
		if got := r.clock.recordedWaits(); !cmp.Equal(got, want) {
			t.Errorf("inbound waits mismatch (-want +got):\n%s", cmp.Diff(want, got))
		}
	})
}

// spawnCall is one call of the fake spawn.
type spawnCall struct {
	args []string
	wait bool
}

// fakeSpawn records the spawns a starter makes and stands for the relay each one started.
type fakeSpawn struct {
	mu    sync.Mutex
	calls []spawnCall
	// err is returned by every spawn.
	err error
	// onSpawn runs for each spawn with its 1-based number, as the relay the spawn started would.
	onSpawn func(n int)
}

func (f *fakeSpawn) spawn(_ slackengine.Geometry, args []string, wait bool) error {
	f.mu.Lock()
	f.calls = append(f.calls, spawnCall{args: args, wait: wait})
	n, onSpawn, err := len(f.calls), f.onSpawn, f.err
	f.mu.Unlock()
	if onSpawn != nil {
		onSpawn(n)
	}
	return err
}

func (f *fakeSpawn) spawned() []spawnCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]spawnCall(nil), f.calls...)
}

// ensureHarness is a starter over a configured hub, a fake spawn and the harness's clock.
type ensureHarness struct {
	*harness
	spawn   *fakeSpawn
	starter slackengine.Starter
}

func newEnsureHarness(t *testing.T) *ensureHarness {
	t.Helper()
	h := newHarness(t)
	spawn := &fakeSpawn{}
	return &ensureHarness{harness: h, spawn: spawn, starter: slackengine.NewStarter(spawn.spawn, h.clock, func() int { return 4242 })}
}

func (e *ensureHarness) away() (slackengine.StartOutcome, error) {
	return e.starter.Ensure(e.geom, "main", true)
}

func (e *ensureHarness) mustAway(want slackengine.StartOutcome) {
	e.t.Helper()
	if got, err := e.away(); err != nil || got != want {
		e.t.Fatalf("Ensure(whileAway) = %q, %v; want %q", got, err, want)
	}
}

// setAway writes the slack presence.
func (e *ensureHarness) setAway() {
	e.t.Helper()
	if _, err := slackengine.WritePresence(e.geom.PresencePath, slackengine.PresenceSlack, e.clock.Now()); err != nil {
		e.t.Fatalf("WritePresence = %v", err)
	}
}

func (e *ensureHarness) relayState() slackengine.RelayState {
	e.t.Helper()
	relayState, _, err := slackengine.ReadRelayState(e.geom)
	if err != nil {
		e.t.Fatalf("ReadRelayState = %v", err)
	}
	return relayState
}

// update applies mutate to the relay state as a relay would.
func (e *ensureHarness) update(mutate func(*slackengine.RelayState)) {
	e.t.Helper()
	if err := slackengine.UpdateRelayState(e.geom, mutate); err != nil {
		e.t.Fatalf("UpdateRelayState = %v", err)
	}
}

func (e *ensureHarness) wantSpawns(n int) {
	e.t.Helper()
	if got := len(e.spawn.spawned()); got != n {
		e.t.Errorf("spawns = %d; want %d", got, n)
	}
}

// diesBeforeItsFirstRead makes each spawned relay reach its first step and die: it records its pid and nothing else.
func (e *ensureHarness) diesBeforeItsFirstRead() {
	e.spawn.onSpawn = func(int) { e.update(func(relayState *slackengine.RelayState) { relayState.PID = fakePID }) }
}

func TestEnsureRelay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	primeArgs := func(e *ensureHarness, extra ...string) []string {
		return append([]string{"slack", "relay", "--hub-path", e.geom.HubPath, "--prime", "main"}, extra...)
	}

	t.Run("HubWithNoTokenOrNoSetupIsNotConfiguredAndSpawnsNothing", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		if err := os.Remove(e.geom.TokenPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeNotConfigured {
			t.Errorf("Ensure(no token) = %q, %v; want not-configured", got, err)
		}
		if err := slackengine.WriteToken(e.geom.TokenPath, "xoxb-secret"); err != nil {
			t.Fatalf("WriteToken = %v", err)
		}
		if err := os.Remove(e.geom.SetupPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeNotConfigured {
			t.Errorf("Ensure(no setup) = %q, %v; want not-configured", got, err)
		}
		e.wantSpawns(0)
	})

	t.Run("LiveRelaySpawnsNothing", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		holdRelayLock(t, e.geom)
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeLive {
			t.Errorf("Ensure(live) = %q, %v; want live", got, err)
		}
		e.wantSpawns(0)
	})

	t.Run("DownRelayIsSpawnedOnceWithItsRecordsAlreadyStored", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		var stored slackengine.RelayState
		e.spawn.onSpawn = func(int) { stored = e.relayState() }
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeStarted {
			t.Fatalf("Ensure = %q, %v; want started", got, err)
		}
		calls := e.spawn.spawned()
		if len(calls) != 1 || !calls[0].wait || !cmp.Equal(calls[0].args, primeArgs(e, "--fork")) {
			t.Errorf("spawns = %+v; want one waited spawn of %v", calls, primeArgs(e, "--fork"))
		}
		if stored.LastSpawnAt.IsZero() || stored.NextStartAt.IsZero() {
			t.Errorf("state at the spawn = %+v; want LastSpawnAt and NextStartAt already stored", stored)
		}
	})

	t.Run("SpawnRelayStartsTheRelayWithItsIntermediateAndDoesNotWait", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		if err := e.starter.SpawnRelay(e.geom, "main"); err != nil {
			t.Fatalf("SpawnRelay = %v", err)
		}
		calls := e.spawn.spawned()
		if len(calls) != 1 || calls[0].wait || !cmp.Equal(calls[0].args, primeArgs(e, "--after", "4242")) {
			t.Errorf("spawns = %+v; want one unwaited spawn of %v", calls, primeArgs(e, "--after", "4242"))
		}
	})

	t.Run("WhileAwayStartsOnlyWhilePresenceIsSlack", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		e.mustAway(slackengine.OutcomeTerminal)
		e.wantSpawns(0)
		e.setAway()
		e.mustAway(slackengine.OutcomeStarted)
		e.wantSpawns(1)
	})

	t.Run("WhileAwayWaitsAMinuteAfterASpawnAndCountsNothingInside", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		e.setAway()
		e.mustAway(slackengine.OutcomeStarted)
		for range 59 {
			e.clock.Advance(time.Second)
			e.mustAway(slackengine.OutcomeWaiting)
		}
		e.wantSpawns(1)
		if failures := e.relayState().StartFailures; failures != 0 {
			t.Errorf("StartFailures = %d inside the wait; want 0", failures)
		}
		e.clock.Advance(time.Second)
		e.mustAway(slackengine.OutcomeStarted)
		e.wantSpawns(2)
		if failures := e.relayState().StartFailures; failures != 1 {
			t.Errorf("StartFailures = %d after a relay that died with no Slack call; want 1", failures)
		}
	})

	t.Run("FiveRelaysInARowThatDieBeforeTheirFirstReadEndTheStartsUntilAnOperatorStart", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		e.setAway()
		e.diesBeforeItsFirstRead()
		for range slackengine.MaxStartFailures {
			e.clock.Advance(time.Hour)
			e.mustAway(slackengine.OutcomeStarted)
		}
		e.clock.Advance(time.Hour)
		e.mustAway(slackengine.OutcomeGaveUp)
		e.wantSpawns(slackengine.MaxStartFailures)
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeStarted {
			t.Fatalf("Ensure(operator) = %q, %v; want started", got, err)
		}
		if failures := e.relayState().StartFailures; failures != 0 {
			t.Errorf("StartFailures = %d after an operator start; want the cap reset to 0", failures)
		}
	})

	t.Run("FiveRelaysThatLiveAMinuteEachEndTheStarts", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		e.setAway()
		e.spawn.onSpawn = func(int) {
			e.update(func(relayState *slackengine.RelayState) {
				relayState.FirstReadAt = relayState.LastSpawnAt.Add(time.Second)
				relayState.LastSlackOK = relayState.LastSpawnAt.Add(time.Minute)
			})
		}
		for range slackengine.MaxStartFailures {
			e.clock.Advance(time.Hour)
			e.mustAway(slackengine.OutcomeStarted)
		}
		e.clock.Advance(time.Hour)
		e.mustAway(slackengine.OutcomeGaveUp)
		e.wantSpawns(slackengine.MaxStartFailures)
	})

	t.Run("RelayThatDiesBeforeAnySlackCallCountsEvenWithAnEarlierRelaysLastSlackOK", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		e.setAway()
		spawnedAt := e.clock.Now()
		e.update(func(relayState *slackengine.RelayState) {
			relayState.LastSpawnAt = spawnedAt
			relayState.LastSlackOK = spawnedAt.Add(-time.Hour)
			relayState.NextStartAt = spawnedAt.Add(time.Minute)
		})
		e.clock.Advance(2 * time.Minute)
		e.mustAway(slackengine.OutcomeStarted)
		if failures := e.relayState().StartFailures; failures != 1 {
			t.Errorf("StartFailures = %d; want the dead relay counted once", failures)
		}
	})

	t.Run("SpawnThatFailsToStartIsCalledFiveTimesThenGivesUp", func(t *testing.T) {
		t.Parallel()
		e := newEnsureHarness(t)
		e.setAway()
		e.spawn.err = errors.New("exec: no such file")
		var nextStarts []time.Time
		for i := range slackengine.MaxStartFailures {
			e.clock.Advance(time.Hour)
			if got, err := e.away(); !errors.Is(err, e.spawn.err) || got != "" {
				t.Fatalf("Ensure #%d = %q, %v; want the spawn's error", i+1, got, err)
			}
			relayState := e.relayState()
			if !relayState.LastSpawnAt.Equal(e.clock.Now()) {
				t.Errorf("LastSpawnAt after spawn #%d = %v; want the spawn time %v", i+1, relayState.LastSpawnAt, e.clock.Now())
			}
			nextStarts = append(nextStarts, relayState.NextStartAt)
		}
		for i := 1; i < len(nextStarts); i++ {
			if !nextStarts[i].After(nextStarts[i-1]) {
				t.Errorf("NextStartAt did not rise between spawns %d and %d: %v", i, i+1, nextStarts)
			}
		}
		e.clock.Advance(time.Hour)
		e.mustAway(slackengine.OutcomeGaveUp)
		e.wantSpawns(slackengine.MaxStartFailures)
	})

	t.Run("RelayLifetimeAtOrOverTheHealthyBoundResetsTheCount", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			lifetime     time.Duration
			wantOutcome  slackengine.StartOutcome
			wantSpawns   int
			wantFailures int
		}{
			{"ElevenMinutes", 11 * time.Minute, slackengine.OutcomeStarted, 1, 0},
			{"ExactlyTheHealthyLifetime", slackengine.RelayHealthyLifetime, slackengine.OutcomeStarted, 1, 0},
			{"OneSecondUnderIsAFifthFailure", slackengine.RelayHealthyLifetime - time.Second, slackengine.OutcomeGaveUp, 0, slackengine.MaxStartFailures},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newEnsureHarness(t)
				e.setAway()
				spawnedAt := e.clock.Now()
				e.update(func(relayState *slackengine.RelayState) {
					relayState.StartFailures = slackengine.MaxStartFailures - 1
					relayState.LastSpawnAt = spawnedAt
					relayState.LastSlackOK = spawnedAt.Add(tt.lifetime)
					relayState.NextStartAt = spawnedAt.Add(time.Minute)
				})
				e.clock.Advance(time.Hour)
				e.mustAway(tt.wantOutcome)
				e.wantSpawns(tt.wantSpawns)
				if failures := e.relayState().StartFailures; failures != tt.wantFailures {
					t.Errorf("StartFailures = %d; want %d", failures, tt.wantFailures)
				}
			})
		}
	})
}

func TestRelayWaits(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AwaitReparentReturnsOnceTheParentChanges", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		calls := 0
		parent := func() int {
			calls++
			if calls <= 3 {
				return 500
			}
			return 1
		}
		if err := slackengine.AwaitReparent(500, parent, clock, slackengine.ReparentBound); err != nil {
			t.Errorf("AwaitReparent = %v; want nil once the parent changed", err)
		}
		if got, want := clock.recordedWaits(), []time.Duration{time.Second, time.Second, time.Second}; !cmp.Equal(got, want) {
			t.Errorf("waits = %v; want one second per read of the parent", got)
		}
	})

	t.Run("AwaitReparentErrorsAtTheBoundNamingTheIntermediate", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		err := slackengine.AwaitReparent(500, func() int { return 500 }, clock, slackengine.ReparentBound)
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Errorf("AwaitReparent = %v; want an error naming the intermediate 500", err)
		}
		if got := len(clock.recordedWaits()); got != int(slackengine.ReparentBound/time.Second) {
			t.Errorf("waits = %d; want one per second of the bound", got)
		}
	})

	liveRelayJSON := func(t *testing.T, extra string) string {
		t.Helper()
		pid := os.Getpid()
		startTime, ok := proc.StartTime(pid)
		if !ok {
			t.Skip("the process start time is not readable on this platform")
		}
		return fmt.Sprintf(`{"pid":%d,"start_time":%q%s}`, pid, startTime, extra)
	}
	const firstReadJSON = `,"first_read_at":"2026-11-05T12:00:00Z"`

	t.Run("AwaitFirstReadReturnsOnALiveRelaysFirstRead", func(t *testing.T) {
		t.Parallel()
		geom, clock := newGeometry(t), newFakeClock()
		writeRelayStateJSON(t, geom, liveRelayJSON(t, firstReadJSON))
		if err := slackengine.AwaitFirstRead(geom, clock, slackengine.FirstReadBound); err != nil {
			t.Errorf("AwaitFirstRead = %v; want nil", err)
		}
	})

	t.Run("AwaitFirstReadIgnoresAGoneRelaysStateUntilAFreshRelayWritesItsFirstRead", func(t *testing.T) {
		t.Parallel()
		geom, clock := newGeometry(t), newFakeClock()
		writeRelayStateJSON(t, geom, fmt.Sprintf(`{"pid":%d,"start_time":"1","first_read_refusal":"an earlier relay's refusal"%s}`, gonePID, firstReadJSON))
		clock.onWait = func(time.Duration) { writeRelayStateJSON(t, geom, liveRelayJSON(t, firstReadJSON)) }
		if err := slackengine.AwaitFirstRead(geom, clock, slackengine.FirstReadBound); err != nil {
			t.Errorf("AwaitFirstRead = %v; want nil once a live relay wrote its first read", err)
		}
		if got := len(clock.recordedWaits()); got != 1 {
			t.Errorf("waits = %d; want the gone relay's state read as not yet written, then one fresh read", got)
		}
	})

	t.Run("AwaitFirstReadRefusesAtTheBoundNamingTheRefusalOrTheLog", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			extra func(geom slackengine.Geometry) (extra, wantNamed string)
		}{
			{"RecordedRefusal", func(slackengine.Geometry) (string, string) {
				return `,"first_read_refusal":"creel: the pane process is unknown"`, "creel: the pane process is unknown"
			}},
			{"NoRefusalNamesTheLog", func(geom slackengine.Geometry) (string, string) { return "", geom.LogPath }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				geom, clock := newGeometry(t), newFakeClock()
				extra, wantNamed := tt.extra(geom)
				writeRelayStateJSON(t, geom, liveRelayJSON(t, extra))
				err := slackengine.AwaitFirstRead(geom, clock, slackengine.FirstReadBound)
				if err == nil || !strings.Contains(err.Error(), wantNamed) || !strings.Contains(err.Error(), "lym slack away") {
					t.Errorf("AwaitFirstRead = %v; want a refusal naming %q and lym slack away", err, wantNamed)
				}
			})
		}
	})
}
