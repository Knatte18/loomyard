// relay_test.go covers the relay process, its starter and the waits of a start.
// It runs over the in-memory seams of inbound_test.go and the manual clock and fake dialer of holder_test.go.
// Its files are in temp directories.

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
	"github.com/Knatte18/loomyard/internal/fswatch"
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

// hasWait reports whether a wait of d is pending.
func (c *manualClock) hasWait(d time.Duration) bool {
	for _, pending := range c.pending() {
		if pending == d {
			return true
		}
	}
	return false
}

// invitedTo returns the users the fake Slack invited to the channel.
func invitedTo(s *fakeSlack, channel string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.invited[channel]...)
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

// relayRun is a relay over a configured harness, run in a goroutine of the test until the test stops it.
// Its clock is a manual one: each loop of the relay blocks at its wait until a row advances the time.
type relayRun struct {
	*harness
	// clock shadows the harness's clock, which only the harness's own inbound read and keeper use.
	clock *manualClock
	// dialer is the relay's Socket Mode dial; with no scripted step a dial blocks until the relay ends.
	dialer       *fakeDialer
	deps         slackengine.RelayDeps
	registered   atomic.Int32
	binaryChecks atomic.Int32
	beamReads    atomic.Int32

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

// newRelayRun returns a relay whose outbound read answers at once with no mail, as a read that timed out does.
// The hub lists the beam of the harness's second channel, so no channel of the map is retired.
func newRelayRun(t *testing.T) *relayRun {
	t.Helper()
	h := newHarness(t)
	h.hub.set(func(hub *fakeHub) { hub.beams = []string{"payments"} })
	r := &relayRun{harness: h, clock: newManualClock(), dialer: &fakeDialer{}}
	hub := h.hub.hub()
	r.deps = slackengine.RelayDeps{
		Mailbox: h.mailbox,
		Slack:   h.slack,
		Dial:    r.dialer.dial,
		Hub: slackengine.Hub{
			Beams: func() ([]string, error) {
				r.beamReads.Add(1)
				return hub.Beams()
			},
			Paused: hub.Paused,
		},
		LiveHubs:      func() ([]slackengine.Geometry, error) { return []slackengine.Geometry{h.geom}, nil },
		Clock:         r.clock,
		PID:           func() int { return fakePID },
		CheckBinary:   func(func(), func(error)) { r.binaryChecks.Add(1) },
		ReapInherited: func() error { return nil },
		Register:      func() error { r.registered.Add(1); return nil },
	}
	t.Cleanup(r.end)
	return r
}

func (r *relayRun) setPID(pid int) { r.deps.PID = func() int { return pid } }

// start runs the relay in a goroutine.
func (r *relayRun) start() {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.mu.Lock()
	r.cancel, r.done, r.err = cancel, done, nil
	r.mu.Unlock()
	go func() {
		defer close(done)
		err := slackengine.RunRelay(ctx, r.geom, r.deps)
		r.mu.Lock()
		r.err = err
		r.mu.Unlock()
	}()
}

// end cancels the run in progress and waits for RunRelay to return.
func (r *relayRun) end() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(awaitTimeout):
		r.t.Errorf("RunRelay did not return after its context ended")
	}
}

// result returns what RunRelay returned.
func (r *relayRun) result() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// finish runs the relay until it returns by itself, and returns what it returned.
func (r *relayRun) finish() error {
	r.t.Helper()
	r.start()
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	select {
	case <-done:
	case <-time.After(awaitTimeout):
		r.t.Fatalf("RunRelay did not return by itself")
	}
	return r.result()
}

// awaitOutboundWait blocks until the outbound loop is parked at a wait, and returns that wait.
// The loop takes its waits from the floor up to the ceiling.
// No other wait of the relay equals one of them but the holder's ping.
func (r *relayRun) awaitOutboundWait() time.Duration {
	r.t.Helper()
	var found time.Duration
	eventually(r.t, "the outbound loop to park at a wait", func() bool {
		for _, wait := range r.clock.pending() {
			if wait >= slackengine.Floor && wait != slackengine.PingInterval {
				found = wait
				return true
			}
		}
		return false
	})
	return found
}

// settle runs the relay and blocks until its start work and its first wake are done, and returns the outbound wait.
func (r *relayRun) settle() time.Duration {
	r.t.Helper()
	r.start()
	return r.awaitOutboundWait()
}

// stop ends the relay once the outbound loop is parked at a wait, and returns what RunRelay returned.
func (r *relayRun) stop() error {
	r.t.Helper()
	r.awaitOutboundWait()
	r.end()
	return r.result()
}

// driveUntil advances the clock past each wait of the wake loop's floor until condition holds.
func (r *relayRun) driveUntil(what string, condition func() bool) {
	r.t.Helper()
	eventually(r.t, what, func() bool {
		if condition() {
			return true
		}
		if r.clock.hasWait(slackengine.WakeFloor) {
			r.clock.Advance(slackengine.WakeFloor)
		}
		return false
	})
}

func (r *relayRun) relayState() slackengine.RelayState {
	r.t.Helper()
	relayState, _, err := slackengine.ReadRelayState(r.geom)
	if err != nil {
		r.t.Fatalf("ReadRelayState = %v", err)
	}
	return relayState
}

// removeEntry drops the orch's entry from the channel map.
func (r *relayRun) removeEntry(orch string) {
	r.t.Helper()
	err := slackengine.UpdateChannelMap(r.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
		delete(channelMap.Channels, orch)
		return nil
	})
	if err != nil {
		r.t.Fatalf("UpdateChannelMap = %v", err)
	}
}

// setPresence writes the presence.
func (r *relayRun) setPresence(presence slackengine.Presence) {
	r.t.Helper()
	if _, err := slackengine.WritePresence(r.geom.PresencePath, presence, r.clock.Now()); err != nil {
		r.t.Fatalf("WritePresence = %v", err)
	}
}

// createdChannels returns the names of the channels the relay asked Slack to create, in order.
func (r *relayRun) createdChannels() []string {
	var names []string
	for _, call := range r.slack.logged("CreateChannel") {
		names = append(names, call.Arg)
	}
	return names
}

// forwarded reports whether the relay mailed the operator's text to the hub's orch.
func (r *relayRun) forwarded(text string) bool {
	for _, posted := range r.mailbox.accepted() {
		if posted.Receiver == primeOrch && posted.Body == text {
			return true
		}
	}
	return false
}

// presenceNotices returns the mail the relay sent as `[slack presence]` notices, in the order it was accepted.
func (r *relayRun) presenceNotices() []mail {
	var notices []mail
	for _, posted := range r.mailbox.accepted() {
		if strings.HasPrefix(posted.Body, "[slack presence]") {
			notices = append(notices, posted)
		}
	}
	return notices
}

// awaitForwarded blocks until the relay mailed the operator's text to the hub's orch.
func (r *relayRun) awaitForwarded(text string) {
	r.t.Helper()
	eventually(r.t, "the operator's text to be forwarded", func() bool { return r.forwarded(text) })
}

func TestRunRelay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("RefusedHubMailsTheOperatorOnceAndExitsWithTheRefusal", func(t *testing.T) {
		t.Parallel()
		bothTokens := slackengine.Credentials{BotToken: "xoxb-secret", AppToken: "xapp-secret", AppID: "A1"}
		withOnlyAnOlderSetupChannel := func(r *relayRun) {
			if err := os.Remove(r.geom.ChannelMapPath); err != nil {
				r.t.Fatalf("Remove = %v", err)
			}
			if err := slackengine.WriteSetup(r.geom.SetupPath, slackengine.Setup{ChannelID: "C7", OperatorID: testOperator}); err != nil {
				r.t.Fatalf("WriteSetup = %v", err)
			}
		}
		tests := []struct {
			name        string
			credentials slackengine.Credentials
			prepare     func(r *relayRun)
			wantNamed   []string
			// wantPostTo is the channel the same line is posted to, empty for none.
			wantPostTo string
		}{
			{"BotTokenAloneNamesTheAppLevelTokenAndPostsToThePrimeChannel",
				slackengine.Credentials{BotToken: "xoxb-secret"}, nil,
				[]string{"app-level token (xapp-)", "lym slack login", "lym slack start"}, testChannel},
			{"BotTokenAloneWithOnlyAnOlderSetupChannelPostsThere",
				slackengine.Credentials{BotToken: "xoxb-secret"}, withOnlyAnOlderSetupChannel,
				[]string{"app-level token (xapp-)", "lym slack login", "lym slack start"}, "C7"},
			{"AppLevelTokenAloneNamesTheBotTokenAndPostsNothing",
				slackengine.Credentials{AppToken: "xapp-secret", AppID: "A1"}, nil,
				[]string{"bot token (xoxb-)", "lym slack login", "lym slack start"}, ""},
			{"AppLevelTokenAloneWithOnlyAnOlderSetupChannelPostsNothing",
				slackengine.Credentials{AppToken: "xapp-secret", AppID: "A1"}, withOnlyAnOlderSetupChannel,
				[]string{"bot token (xoxb-)", "lym slack login", "lym slack start"}, ""},
			{"BothTokensAndNoSetupNamesTheOperatorVerbAndPostsToThePrimeChannel",
				bothTokens, func(r *relayRun) {
					if err := os.Remove(r.geom.SetupPath); err != nil {
						r.t.Fatalf("Remove = %v", err)
					}
				},
				[]string{"lym slack setup --operator", "lym slack start"}, testChannel},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				r := newRelayRun(t)
				if err := slackengine.WriteCredentials(r.geom.TokenPath, tt.credentials); err != nil {
					t.Fatalf("WriteCredentials = %v", err)
				}
				if tt.prepare != nil {
					tt.prepare(r)
				}
				err := r.finish()
				if err == nil || !strings.Contains(err.Error(), tt.wantNamed[0]) {
					t.Fatalf("RunRelay = %v; want the refusal naming %q", err, tt.wantNamed[0])
				}
				notices := r.relayNotices()
				if len(notices) != 1 {
					t.Fatalf("notices = %q; want exactly one", notices)
				}
				for _, want := range tt.wantNamed {
					if !strings.Contains(notices[0], want) {
						t.Errorf("notice = %q; want it to name %q", notices[0], want)
					}
				}
				posts := r.slack.logged("PostAs")
				switch {
				case tt.wantPostTo == "" && len(posts) != 0:
					t.Errorf("PostAs calls = %+v; want none", posts)
				case tt.wantPostTo != "" && (len(posts) != 1 || posts[0].Channel != tt.wantPostTo || !strings.HasPrefix(posts[0].Arg, "[slack relay]")):
					t.Errorf("PostAs calls = %+v; want one relay notice to %s", posts, tt.wantPostTo)
				}
				if r.mailbox.readAllCount() != 1 {
					t.Errorf("first creel reads = %d; want 1 before the refusal", r.mailbox.readAllCount())
				}
				if live, err := slackengine.RelayLive(r.geom); err != nil || live {
					t.Errorf("RelayLive = %v, %v; want the lock free after the exit", live, err)
				}
			})
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
				relayState, _, _ := slackengine.ReadRelayState(r.geom)
				firstReadAtFirstCall = relayState.FirstReadAt
				registeredAtFirstCall = r.registered.Load()
			})
		}
		r.settle()
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
		r.settle()
		r.mailbox.mu.Lock()
		acknowledged := r.mailbox.readAllAcks
		r.mailbox.mu.Unlock()
		if len(acknowledged) != 0 {
			t.Errorf("first read acknowledged %v; want the mail left unread", acknowledged)
		}
		if r.slack.callCount() == 0 || r.relayState().FirstReadAt.IsZero() {
			t.Errorf("the relay did not go on to read its channels after an accepted first read")
		}
	})

	t.Run("FirstReadRefusedFromAStrandPaneReturnsTheErrorAndFreesTheLock", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.mailbox.scriptReadAll(readStep{err: fmt.Errorf("creel: refused: %w", creelengine.ErrProgramClaimInStrandPane)})
		err := r.finish()
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
		r.settle()
		if relayState := r.relayState(); relayState.PID != 8 || relayState.FirstReadAt.IsZero() {
			t.Errorf("state after the second relay = %+v; want pid 8 with a first read", relayState)
		}
	})

	t.Run("OtherFirstReadRefusalKeepsTheLockAndRetries", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.mailbox.scriptReadAll(readStep{err: errors.New("creel: the pane process is unknown")})
		r.start()
		r.clock.awaitWait(t, 2*slackengine.Floor)
		liveDuringRetry, _ := slackengine.RelayLive(r.geom)
		callsDuringRetry := r.slack.callCount()
		refusalDuringRetry := r.relayState().FirstReadRefusal
		if !liveDuringRetry || callsDuringRetry != 0 || !strings.Contains(refusalDuringRetry, "pane process is unknown") {
			t.Errorf("during the retry: live %v, Slack calls %d, refusal %q; want the lock held, no call and the refusal recorded", liveDuringRetry, callsDuringRetry, refusalDuringRetry)
		}
		r.clock.Advance(2 * slackengine.Floor)
		r.awaitOutboundWait()
		if relayState := r.relayState(); relayState.FirstReadRefusal != "" || relayState.FirstReadAt.IsZero() || r.slack.callCount() == 0 {
			t.Errorf("after the retry: state %+v, Slack calls %d; want the refusal cleared, a first read and the channel reads", relayState, r.slack.callCount())
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
		r.settle()
		readsBefore := r.mailbox.readAllCount()
		otherErr := slackengine.RunRelay(context.Background(), r.geom, other)
		readsAfter := r.mailbox.readAllCount()
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
			r.settle()
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
			if err := r.finish(); err != nil {
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
			r.settle()
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
			var liveDuringExec bool
			r.deps.CheckBinary = replaced(func() { liveDuringExec, _ = slackengine.RelayLive(r.geom) })
			r.settle()
			liveAfter, _ := slackengine.RelayLive(r.geom)
			if liveDuringExec || !liveAfter || !strings.Contains(logged(t, r), "permission denied") {
				t.Errorf("live during exec %v, live after %v, log %q; want the lock released in beforeExec, retaken in failed and the exec error logged", liveDuringExec, liveAfter, logged(t, r))
			}
		})

		t.Run("LockHeldOnlyForAProbesGraceIsRetaken", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			r.deps.CheckBinary = replaced(func() {
				probe, acquired, err := lock.TryAcquireWriteLock(r.geom.LockPath)
				if err != nil || !acquired {
					t.Errorf("TryAcquireWriteLock = %v, %v; want the lock the relay released", acquired, err)
					return
				}
				time.AfterFunc(lock.ProbeGrace/5, func() { _ = probe.Release() })
			})
			r.settle()
			if liveAfter, _ := slackengine.RelayLive(r.geom); !liveAfter {
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
			if err := r.finish(); err != nil {
				t.Fatalf("RunRelay = %v", err)
			}
			if !strings.Contains(logged(t, r), "another relay holds the lock") {
				t.Errorf("log %q; want the exit logged", logged(t, r))
			}
		})

		t.Run("BeforeExecFreesTheSocketLockAndAFailedExecDialsAgainOnlyOnceTheHolderHoldsItAgain", func(t *testing.T) {
			t.Parallel()
			r := newRelayRun(t)
			r.startAt("100.000000")
			dialedFirst := r.dialer.hand(newFakeSocket(t))
			dialedAgain := r.dialer.hand(newFakeSocket(t))
			var socketLockFreed bool
			var dialsWhileTheTestHoldsTheLock int
			checks := 0
			r.deps.CheckBinary = func(beforeExec func(), failed func(error)) {
				checks++
				if checks > 1 {
					return
				}
				select {
				case <-dialedFirst:
				case <-time.After(awaitTimeout):
					t.Errorf("the holder did not dial before the binary check")
					return
				}
				beforeExec()
				socketLock, acquired, err := lock.TryAcquireWriteLock(r.geom.SocketLockPath)
				socketLockFreed = err == nil && acquired
				failed(errors.New("exec: permission denied"))
				dialsWhileTheTestHoldsTheLock = r.dialer.count()
				if socketLockFreed {
					_ = socketLock.Release()
				}
			}
			r.start()
			awaitClosed(t, dialedAgain)
			if !socketLockFreed || dialsWhileTheTestHoldsTheLock != 1 {
				t.Errorf("socket lock free after beforeExec %v, dials while it was held %d; want the lock free and the one first dial", socketLockFreed, dialsWhileTheTestHoldsTheLock)
			}
		})
	})

	t.Run("OutboundReadStampsLastHealthyWithNoSlackCall", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.slack.setHistoryErr(errors.New("slack down"))
		r.settle()
		if r.relayState().LastSlackOK.IsZero() {
			t.Errorf("LastSlackOK is zero after an outbound read of a relay whose Slack calls all fail")
		}
	})

	t.Run("RelayReadingNoMailForElevenMinutesCountsAsHealthyToTheStarter", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.setPresence(slackengine.PresenceSlack)
		spawnedAt := r.clock.Now()
		if err := slackengine.UpdateRelayState(r.geom, func(relayState *slackengine.RelayState) {
			relayState.LastSpawnAt = spawnedAt
			relayState.NextStartAt = spawnedAt.Add(time.Minute)
		}); err != nil {
			t.Fatalf("UpdateRelayState = %v", err)
		}
		r.start()
		for r.relayState().LastSlackOK.Sub(spawnedAt) < 11*time.Minute {
			r.clock.Advance(r.awaitOutboundWait())
		}
		r.stop()
		spawn := &fakeSpawn{}
		starter := slackengine.NewStarter(spawn.spawn, r.clock, func() int { return 4242 })
		if got, err := starter.Ensure(r.geom, "main", true); err != nil || got != slackengine.OutcomeStarted {
			t.Fatalf("Ensure(whileAway) = %q, %v; want started", got, err)
		}
		if failures := r.relayState().StartFailures; failures != 0 {
			t.Errorf("StartFailures = %d after a relay that read no mail for eleven minutes; want 0", failures)
		}
	})

	t.Run("FirstReadKeepsTheStartersFields", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		writeRelayStateJSON(t, r.geom, `{"start_failures":4,"next_start_at":"2026-11-05T12:30:00Z","last_spawn_at":"2026-11-05T12:00:00Z"}`)
		r.settle()
		relayState := r.relayState()
		if relayState.FirstReadAt.IsZero() || relayState.StartFailures != 4 ||
			!relayState.NextStartAt.Equal(time.Date(2026, 11, 5, 12, 30, 0, 0, time.UTC)) ||
			!relayState.LastSpawnAt.Equal(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)) {
			t.Errorf("state = %+v; want a first read with StartFailures, NextStartAt and LastSpawnAt as they stood", relayState)
		}
	})

	t.Run("OutboundWakesRetryTheOwedWorkAndLeaveTheOutboundReadsBackoffAlone", func(t *testing.T) {
		t.Parallel()
		refusedInvite := &slackengine.APIError{Method: "conversations.invite", Code: "user_not_found"}
		tests := []struct {
			name    string
			prepare func(r *relayRun)
			// retries counts the calls the retried work made.
			retries func(r *relayRun) int
		}{
			{"OwedReadFailingOnAServerError",
				func(r *relayRun) { r.slack.setHistoryErr(errors.New("slack server error")) },
				func(r *relayRun) int { return len(r.slack.logged("History")) }},
			{"RefusedOperatorInvite",
				func(r *relayRun) {
					for _, orch := range []string{primeOrch, paymentsOrch} {
						r.updateEntry(orch, func(entry *slackengine.ChannelEntry) { entry.Operator = "" })
					}
					refusals := make([]error, 40)
					for i := range refusals {
						refusals[i] = refusedInvite
					}
					r.slack.failNext("Invite", refusals...)
				},
				func(r *relayRun) int { return len(r.slack.logged("Invite")) }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				r := newRelayRun(t)
				tt.prepare(r)
				r.start()
				var waits []time.Duration
				retriesAtFirstWait := 0
				for i := range 5 {
					wait := r.awaitOutboundWait()
					if i == 0 {
						retriesAtFirstWait = tt.retries(r)
					}
					waits = append(waits, wait)
					r.clock.Advance(wait)
				}
				want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
				if diff := cmp.Diff(want, waits); diff != "" {
					t.Errorf("outbound waits mismatch (-want +got):\n%s", diff)
				}
				r.awaitOutboundWait()
				if got := tt.retries(r); got <= retriesAtFirstWait {
					t.Errorf("calls of the retried work = %d after five more outbound wakes, %d at the first wait; want them to grow", got, retriesAtFirstWait)
				}
			})
		}
	})

	t.Run("PrimeEnsureThatDidNotStandAtStartIsMadeAtTheNextOutboundWakes", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			failure error
		}{
			{"TransportError", errors.New("dial tcp: connection refused")},
			{"SlackRefusal", &slackengine.APIError{Method: "conversations.create", Code: "restricted_action"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				r := newRelayRun(t)
				r.removeEntry(primeOrch)
				r.slack.failNext("CreateChannel", tt.failure, tt.failure)
				wait := r.settle()
				if got := len(r.createdChannels()); got != 2 || r.entryOf(primeOrch).ID != "" {
					t.Errorf("after the start and the first outbound wake: %d create calls, prime entry %+v; want two failed creates and no entry", got, r.entryOf(primeOrch))
				}
				r.clock.Advance(wait)
				r.clock.awaitWait(t, 2*wait)
				if got := len(r.createdChannels()); got != 3 || r.entryOf(primeOrch).ID == "" {
					t.Errorf("after the next outbound wake: %d create calls, prime entry %+v; want a third create that stood", got, r.entryOf(primeOrch))
				}
			})
		}
	})

	t.Run("EventArrivingDuringAnOutboundWakeIsDrainedAfterThatWakeEndsNeverBesideIt", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.deps.CheckBinary = func(func(), func(error)) {
			if r.binaryChecks.Add(1) != 2 {
				return
			}
			r.slack.add(testChannel, slackengine.Message{TS: "200.000001", User: testOperator, Text: "late"})
			if err := slackengine.WriteHint(r.geom, testChannel); err != nil {
				t.Errorf("WriteHint = %v", err)
			}
		}
		wait := r.settle()
		r.clock.Advance(wait)
		r.clock.awaitWait(t, slackengine.WakeFloor)
		if r.forwarded("late") {
			t.Errorf("the message was forwarded before the wake loop was free of the outbound wake that saw the hint")
		}
		r.clock.Advance(slackengine.WakeFloor)
		r.awaitForwarded("late")
	})

	t.Run("HintInTheInboxWakesTheRelayThroughItsFileEventWithNoClockWait", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.settle()
		r.clock.Advance(slackengine.WakeFloor)
		r.say("200.000001", "go ahead")
		r.hint(testChannel)
		// The clock is manual and not advanced after the hint, so a forward proves the wake waited on no clock.
		r.awaitForwarded("go ahead")
	})

	t.Run("EventInsideASecondOfAWakesStartWaitsTheRestOfThatSecond", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.settle()
		r.say("200.000001", "wait for it")
		r.hint(testChannel)
		r.clock.awaitWait(t, slackengine.WakeFloor)
		if r.forwarded("wait for it") {
			t.Errorf("the message was forwarded before the rest of the second passed")
		}
		r.clock.Advance(slackengine.WakeFloor)
		r.awaitForwarded("wait for it")
	})

	t.Run("WritesToTheRelaysOwnRecordsWakeNothingAndAPresenceChangeWakes", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.settle()
		r.clock.Advance(slackengine.WakeFloor)
		beamReadsBefore := r.beamReads.Load()

		watcher, err := fswatch.Watch(r.geom.Dir)
		if err != nil {
			t.Fatalf("Watch = %v", err)
		}
		t.Cleanup(func() { _ = watcher.Close() })
		if err := slackengine.UpdateRelayState(r.geom, func(relayState *slackengine.RelayState) { relayState.StartFailures = 1 }); err != nil {
			t.Fatalf("UpdateRelayState = %v", err)
		}
		if err := os.WriteFile(r.geom.OutboxPath, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write the outbox = %v", err)
		}
		r.updateEntry(primeOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "300.000001" })
		if err := os.WriteFile(r.geom.LogPath, []byte("a note\n"), 0o600); err != nil {
			t.Fatalf("write the relay log = %v", err)
		}
		// The machine's token file shares the presence file's directory and wakes nothing either.
		bothTokens := slackengine.Credentials{BotToken: "xoxb-secret", AppToken: "xapp-secret", AppID: "A1"}
		if err := slackengine.WriteCredentials(r.geom.TokenPath, bothTokens); err != nil {
			t.Fatalf("WriteCredentials = %v", err)
		}
		unseen := map[string]bool{
			slackengine.RelayStateFileName: true, slackengine.OutboxFileName: true,
			slackengine.ChannelMapFileName: true, slackengine.RelayLogFileName: true,
		}
		deadline := time.After(awaitTimeout)
		for len(unseen) > 0 {
			select {
			case event := <-watcher.Events():
				delete(unseen, event.Name)
			case <-deadline:
				t.Fatalf("the files %v produced no event", unseen)
			}
		}

		r.hub.set(func(hub *fakeHub) { hub.beams = []string{"payments", "extra"} })
		r.setPresence(slackengine.PresenceSlack)
		eventually(t, "the channel of the new beam to be created", func() bool { return len(r.createdChannels()) == 1 })
		if got := r.beamReads.Load(); got != beamReadsBefore+1 {
			t.Errorf("beam reads = %d after the writes and a presence change; want %d, one wake for the presence change alone", got, beamReadsBefore+1)
		}
		if r.clock.hasWait(slackengine.WakeFloor) {
			t.Errorf("the presence change waited on the clock; a write to the relay's own records woke the relay")
		}
	})

	t.Run("TwoHubsRelaysSharingASocketLockDialOnceAndTheEventsOwnerForwardsIt", func(t *testing.T) {
		t.Parallel()
		a, b := newRelayRun(t), newRelayRun(t)
		b.geom.SocketLockPath = a.geom.SocketLockPath
		b.updateEntry(primeOrch, func(entry *slackengine.ChannelEntry) { entry.ID = "CB1" })
		b.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.ID = "CB2" })
		live := func() ([]slackengine.Geometry, error) { return []slackengine.Geometry{a.geom, b.geom}, nil }
		a.deps.LiveHubs, b.deps.LiveHubs = live, live
		socketA := newFakeSocket(t)
		dialedA := a.dialer.hand(socketA)
		dialedB := b.dialer.hand(newFakeSocket(t))

		a.start()
		awaitClosed(t, dialedA)
		b.settle()
		b.sayIn("CB1", "200.000001", "hello from hub B")
		socketA.sendEventAndAwaitAck(eventsEnvelope("e1", "CB1", "200.000001"))
		b.driveUntil("hub B to forward the message of its channel", func() bool { return b.forwarded("hello from hub B") })
		if got := b.dialer.count(); got != 0 {
			t.Errorf("hub B dialed %d times while hub A held the socket lock; want 0", got)
		}
		if a.forwarded("hello from hub B") {
			t.Errorf("hub A forwarded a message of hub B's channel")
		}

		a.stop()
		awaitClosed(t, dialedB)
	})

	t.Run("PrimeChannelIsEnsuredAtStartAndBeamChannelsOnceThePresenceTurnsToSlack", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.removeEntry(primeOrch)
		r.removeEntry(paymentsOrch)
		prime := slackengine.ChannelName(r.geom.Shortname, r.geom.Machine, "")
		payments := slackengine.ChannelName(r.geom.Shortname, r.geom.Machine, "payments")
		r.settle()
		if diff := cmp.Diff([]string{prime}, r.createdChannels()); diff != "" {
			t.Errorf("channels created at the start mismatch (-want +got):\n%s", diff)
		}
		r.clock.Advance(slackengine.WakeFloor)
		r.setPresence(slackengine.PresenceSlack)
		eventually(t, "the beam's channel to be created", func() bool { return len(r.createdChannels()) == 2 })
		if diff := cmp.Diff([]string{prime, payments}, r.createdChannels()); diff != "" {
			t.Errorf("channels created after the presence turned to slack mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("PresenceSlackAtStartEnsuresEveryListedBeamsChannelAtTheStart", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.removeEntry(primeOrch)
		r.removeEntry(paymentsOrch)
		r.setPresence(slackengine.PresenceSlack)
		prime := slackengine.ChannelName(r.geom.Shortname, r.geom.Machine, "")
		payments := slackengine.ChannelName(r.geom.Shortname, r.geom.Machine, "payments")
		r.settle()
		if diff := cmp.Diff([]string{prime, payments}, r.createdChannels()); diff != "" {
			t.Errorf("channels created at the start mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("PresenceChangeAfterTheStartIsMailedToTheHubOrchAndItsBeamOrchsAndTheStartsPresenceIsNot", func(t *testing.T) {
		t.Parallel()
		const slackNotice, terminalNotice = "[slack presence] presence is now slack", "[slack presence] presence is now terminal"
		tests := []struct {
			name string
			// atStart is the presence written before the relay starts, empty for none.
			atStart slackengine.Presence
			// beamsErr is what the hub's beams read answers.
			beamsErr error
			// changes are written one after the other once the relay's first wake is done.
			changes []slackengine.Presence
			want    []mail
		}{
			{"ChangeToSlackMailsTheHubOrchAndTheBeamOrch", "", nil,
				[]slackengine.Presence{slackengine.PresenceSlack},
				[]mail{{primeOrch, slackNotice}, {paymentsOrch, slackNotice}}},
			{"FailedBeamsReadMailsTheHubOrchAlone", "", errors.New("board unreadable"),
				[]slackengine.Presence{slackengine.PresenceSlack},
				[]mail{{primeOrch, slackNotice}}},
			{"PresenceFoundAtStartIsNotMailedButTheChangeFromItIs", slackengine.PresenceSlack, nil,
				[]slackengine.Presence{slackengine.PresenceTerminal},
				[]mail{{primeOrch, terminalNotice}, {paymentsOrch, terminalNotice}}},
			{"RewriteOfTheStoredPresenceMailsNothingButALaterChangeDoes", "", nil,
				[]slackengine.Presence{slackengine.PresenceTerminal, slackengine.PresenceSlack},
				[]mail{{primeOrch, slackNotice}, {paymentsOrch, slackNotice}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				r := newRelayRun(t)
				r.hub.set(func(hub *fakeHub) { hub.beamsErr = tt.beamsErr })
				if tt.atStart != "" {
					r.setPresence(tt.atStart)
				}
				r.settle()
				if notices := r.presenceNotices(); len(notices) != 0 {
					t.Fatalf("notices after the start = %+v; want none for the presence found at the start", notices)
				}
				for _, change := range tt.changes {
					r.setPresence(change)
				}
				r.driveUntil("the presence notices to be mailed", func() bool { return len(r.presenceNotices()) >= len(tt.want) })
				if diff := cmp.Diff(tt.want, r.presenceNotices()); diff != "" {
					t.Errorf("presence notices mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("OperatorsMessageThatTurnsThePresenceToSlackIsNoticedToEveryOrchButTheOneItWasForwardedTo", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.settle()
		r.clock.Advance(slackengine.WakeFloor)
		r.say("200.000001", "hello from slack")
		r.hint(testChannel)
		r.awaitForwarded("hello from slack")
		r.driveUntil("the presence notice to be mailed", func() bool { return len(r.presenceNotices()) >= 1 })
		want := []mail{{paymentsOrch, "[slack presence] presence is now slack"}}
		if diff := cmp.Diff(want, r.presenceNotices()); diff != "" {
			t.Errorf("presence notices mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("PresenceThatTurnedBackBeforeTheNoticeIsNoticedToEveryOrchAndTheKeptOrchesAreForgottenAtEveryWake", func(t *testing.T) {
		t.Parallel()
		// The mailbox hook below writes the presence terminal once, as the operator's message reaches the hub's orch,
		// so the wake that forwards the message also finds the presence changed again.
		const slackNotice, terminalNotice = "[slack presence] presence is now slack", "[slack presence] presence is now terminal"
		tests := []struct {
			name string
			// run drives the relay from a settled start and returns when the notices of want are mailed.
			run  func(r *relayRun, turnBack *atomic.Bool)
			want []mail
		}{
			{"NoticeOfTerminalReachesTheOrchTheMessageWasForwardedTo", func(r *relayRun, turnBack *atomic.Bool) {
				r.setPresence(slackengine.PresenceSlack)
				r.driveUntil("the slack notices to be mailed", func() bool { return len(r.presenceNotices()) >= 2 })
				r.setPresence(slackengine.PresenceTerminal)
				r.say("200.000001", "hello from slack")
				turnBack.Store(true)
				r.hint(testChannel)
				r.driveUntil("the terminal notices to be mailed", func() bool { return len(r.presenceNotices()) >= 4 })
			}, []mail{
				{primeOrch, slackNotice}, {paymentsOrch, slackNotice},
				{primeOrch, terminalNotice}, {paymentsOrch, terminalNotice},
			}},
			{"OrchKeptByAWakeThatSentNoNoticeDoesNotSuppressALaterNotice", func(r *relayRun, turnBack *atomic.Bool) {
				reads := r.beamReads.Load()
				r.say("200.000001", "hello from slack")
				turnBack.Store(true)
				r.hint(testChannel)
				r.driveUntil("a wake after the forwarding wake", func() bool { return r.beamReads.Load() >= reads+2 })
				r.setPresence(slackengine.PresenceSlack)
				r.driveUntil("the slack notices to be mailed", func() bool { return len(r.presenceNotices()) >= 2 })
			}, []mail{{primeOrch, slackNotice}, {paymentsOrch, slackNotice}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				r := newRelayRun(t)
				r.startAt("100.000000")
				var turnBack atomic.Bool
				r.mailbox.onPost = func(receiver string) {
					if receiver == primeOrch && turnBack.CompareAndSwap(true, false) {
						if _, err := slackengine.WritePresence(r.geom.PresencePath, slackengine.PresenceTerminal, r.clock.Now()); err != nil {
							t.Errorf("WritePresence = %v", err)
						}
					}
				}
				r.settle()
				tt.run(r, &turnBack)
				if diff := cmp.Diff(tt.want, r.presenceNotices()); diff != "" {
					t.Errorf("presence notices mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("RefusedOperatorInviteIsRetriedAtTheNextOutboundWake", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		for _, orch := range []string{primeOrch, paymentsOrch} {
			r.updateEntry(orch, func(entry *slackengine.ChannelEntry) { entry.Operator = "" })
		}
		r.slack.failNext("Invite", &slackengine.APIError{Method: "conversations.invite", Code: "user_not_found"})
		r.settle()
		for _, channel := range []string{testChannel, paymentsChannel} {
			if diff := cmp.Diff([]string{testOperator}, invitedTo(r.slack, channel)); diff != "" {
				t.Errorf("users invited to %s mismatch (-want +got):\n%s", channel, diff)
			}
		}
		if notices := r.relayNotices(); len(notices) != 1 || !strings.Contains(notices[0], "lym slack setup --operator") {
			t.Errorf("notices = %q; want one for the refused invite naming lym slack setup --operator", notices)
		}
	})

	t.Run("NewOperatorInTheSetupFileIsInvitedAtTheNextOutboundWakeWithTheRelayRunning", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		wait := r.settle()
		if err := slackengine.WriteSetup(r.geom.SetupPath, slackengine.Setup{OperatorID: "U2"}); err != nil {
			t.Fatalf("WriteSetup = %v", err)
		}
		r.clock.Advance(wait)
		for _, channel := range []string{testChannel, paymentsChannel} {
			eventually(t, "the new operator to be invited to "+channel, func() bool {
				return cmp.Equal(invitedTo(r.slack, channel), []string{"U2"})
			})
		}
	})

	t.Run("RetiredBeams", func(t *testing.T) {
		t.Parallel()
		// retireRig returns a settled relay whose second channel exists in Slack and whose hub lists no beam.
		retireRig := func(t *testing.T) *relayRun {
			t.Helper()
			r := newRelayRun(t)
			r.slack.seedChannel(slackengine.ChannelInfo{ID: paymentsChannel, Name: "ly-neve-payments", Creator: "UBOT", IsMember: true})
			r.settle()
			r.hub.set(func(hub *fakeHub) { hub.beams = nil })
			r.clock.Advance(slackengine.WakeFloor)
			return r
		}

		t.Run("UnlistedBeamsChannelIsRenamedAndArchivedAtAWake", func(t *testing.T) {
			t.Parallel()
			r := retireRig(t)
			r.setPresence(slackengine.PresenceSlack)
			eventually(t, "the unlisted beam's entry to be archived", func() bool { return r.entryOf(paymentsOrch).Archived })
			dated := slackengine.ArchivedName("ly-neve-payments", r.clock.Now(), 1)
			wantRenames := []slackCall{{Method: "Rename", Channel: paymentsChannel, Arg: dated}}
			if diff := cmp.Diff(wantRenames, r.slack.logged("Rename")); diff != "" {
				t.Errorf("rename calls mismatch (-want +got):\n%s", diff)
			}
			if entry := r.entryOf(paymentsOrch); entry.Name != dated {
				t.Errorf("archived entry = %+v; want the dated name %s", entry, dated)
			}
			if r.entryOf(primeOrch).Archived {
				t.Errorf("the prime's entry was archived")
			}
		})

		t.Run("FailingBeamsReadRetiresNothing", func(t *testing.T) {
			t.Parallel()
			r := retireRig(t)
			r.hub.set(func(hub *fakeHub) { hub.beamsErr = errors.New("beams unreadable") })
			before := r.beamReads.Load()
			r.setPresence(slackengine.PresenceSlack)
			eventually(t, "the wake to read the beams", func() bool { return r.beamReads.Load() > before })
			wait := r.awaitOutboundWait()
			r.clock.Advance(wait)
			r.clock.awaitWait(t, 2*wait)
			if renames, archives := r.slack.logged("Rename"), r.slack.logged("Archive"); len(renames)+len(archives) != 0 {
				t.Errorf("rename calls %+v and archive calls %+v; want none after a failed beams read", renames, archives)
			}
		})

		t.Run("RefusedArchiveIsRetriedAtTheNextOutboundWakeAndNotAtAFileEventWake", func(t *testing.T) {
			t.Parallel()
			r := retireRig(t)
			r.slack.failNext("Archive", &slackengine.APIError{Method: "conversations.archive", Code: "restricted_action"})
			r.setPresence(slackengine.PresenceSlack)
			eventually(t, "the first archive call", func() bool { return len(r.slack.logged("Archive")) == 1 })

			// An event inside the second of the first wake's start waits on the clock only once that wake has ended.
			r.say("200.000001", "one")
			r.hint(testChannel)
			r.clock.awaitWait(t, slackengine.WakeFloor)
			r.clock.Advance(slackengine.WakeFloor)
			r.awaitForwarded("one")
			r.say("200.000002", "two")
			r.hint(testChannel)
			r.clock.awaitWait(t, slackengine.WakeFloor)
			if got := len(r.slack.logged("Archive")); got != 1 {
				t.Errorf("archive calls = %d after a file-event wake; want the refused archive not retried", got)
			}

			r.clock.Advance(r.awaitOutboundWait())
			eventually(t, "the refused archive to be retried", func() bool { return r.entryOf(paymentsOrch).Archived })
			if got := len(r.slack.logged("Archive")); got != 2 {
				t.Errorf("archive calls = %d after the outbound wake; want the one retry", got)
			}
		})
	})

	t.Run("HintedReadFailingOnAServerErrorIsForwardedAfterTheNextOutboundWakeWithNoNewHint", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		wait := r.settle()
		r.clock.Advance(slackengine.WakeFloor)
		r.say("200.000001", "retry me")
		r.slack.failNext("History", errors.New("slack server error"))
		historyCalls := len(r.slack.logged("History"))
		r.hint(testChannel)
		eventually(t, "the hinted read to fail", func() bool { return len(r.slack.logged("History")) > historyCalls })
		if r.forwarded("retry me") {
			t.Errorf("the message was forwarded by a read that failed")
		}
		r.clock.Advance(wait)
		r.awaitForwarded("retry me")
	})

	t.Run("WatchThatFailsToOpenFallsBackToTheOnDiskCheckWithItsOwnBackoff", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		r.startAt("100.000000")
		r.deps.Watch = func(string, ...string) (*fswatch.Watcher, error) { return nil, errors.New("inotify is unavailable") }
		r.start()
		for _, wait := range []time.Duration{time.Second, 2 * time.Second} {
			r.clock.awaitWait(t, wait)
			r.clock.Advance(wait)
		}
		r.clock.awaitWait(t, 4*time.Second)
		r.say("200.000001", "found on disk")
		r.hint(testChannel)
		r.clock.Advance(4 * time.Second)
		r.awaitForwarded("found on disk")
		r.clock.awaitWait(t, time.Second)
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
		if err := slackengine.WriteCredentials(e.geom.TokenPath, slackengine.Credentials{BotToken: "xoxb-secret"}); err != nil {
			t.Fatalf("WriteCredentials = %v", err)
		}
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeNotConfigured {
			t.Errorf("Ensure(bot token alone) = %q, %v; want not-configured", got, err)
		}
		bothTokens := slackengine.Credentials{BotToken: "xoxb-secret", AppToken: "xapp-secret", AppID: "A1"}
		if err := slackengine.WriteCredentials(e.geom.TokenPath, bothTokens); err != nil {
			t.Fatalf("WriteCredentials = %v", err)
		}
		if err := os.Remove(e.geom.SetupPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		if got, err := e.starter.Ensure(e.geom, "main", false); err != nil || got != slackengine.OutcomeNotConfigured {
			t.Errorf("Ensure(no setup) = %q, %v; want not-configured", got, err)
		}
		e.wantSpawns(0)
	})

	t.Run("SwitchedOffMachineAnswersSwitchedOffInBothModesSpawnsNothingAndStartsOnceSwitchedOn", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name        string
			whileAway   bool
			removeToken bool
			want        slackengine.StartOutcome
		}{
			{"OperatorStart", false, false, slackengine.OutcomeSwitchedOff},
			{"WhileAway", true, false, slackengine.OutcomeSwitchedOff},
			{"NotConfiguredIsAnsweredBeforeTheSwitchIsRead", false, true, slackengine.OutcomeNotConfigured},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newEnsureHarness(t)
				e.setAway()
				if tt.removeToken {
					if err := os.Remove(e.geom.TokenPath); err != nil {
						t.Fatalf("Remove = %v", err)
					}
				}
				if err := slackengine.WriteSwitchedOff(e.geom.SwitchPath, true, e.clock.Now()); err != nil {
					t.Fatalf("WriteSwitchedOff = %v", err)
				}
				if got, err := e.starter.Ensure(e.geom, "main", tt.whileAway); err != nil || got != tt.want {
					t.Fatalf("Ensure(switched off) = %q, %v; want %q", got, err, tt.want)
				}
				e.wantSpawns(0)
				if tt.removeToken {
					return
				}
				if err := slackengine.WriteSwitchedOff(e.geom.SwitchPath, false, e.clock.Now()); err != nil {
					t.Fatalf("WriteSwitchedOff = %v", err)
				}
				if got, err := e.starter.Ensure(e.geom, "main", tt.whileAway); err != nil || got != slackengine.OutcomeStarted {
					t.Fatalf("Ensure(switched on) = %q, %v; want started", got, err)
				}
				e.wantSpawns(1)
			})
		}
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
