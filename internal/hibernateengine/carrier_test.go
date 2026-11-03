// carrier_test.go drives the takeover of a pending request and the Carrier's goroutine over fakeWorld.

package hibernateengine

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// waitTerminal polls the request file until it holds a terminal request, failing the test past a deadline, since the carrier's goroutine gives no signal when it ends.
func waitTerminal(t *testing.T, w *fakeWorld) Request {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		r, found, err := readRequest(w.requestPath)
		if err == nil && found && r.Terminal() {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("the request never ended: %+v, found %v, err %v", r, found, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTakeUp(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"a request no daemon carries, and one a dead daemon or the process itself before a re-exec recorded, is taken up waiting with the running process recorded", func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				pid   int
				start string
			}{
				{"no daemon", 0, ""},
				{"a dead daemon", 7, "s7"},
				{"the process itself", 4242, "self-start"},
			} {
				w := newFakeWorld(t)
				w.seed(Request{Trigger: TriggerNow, Requester: requesterExternal, DaemonPID: tc.pid, DaemonStart: tc.start, Phase: PhaseWaiting, Reason: "waiting"})

				got, taken, err := takeUp(w.deps())

				if err != nil || !taken || got.Phase != PhaseWaiting || got.Reason != "waiting" || got.DaemonPID != w.selfPID || got.DaemonStart != w.selfStart {
					t.Errorf("%s: got %+v, taken %v, %v; want it taken up waiting with this process recorded", tc.name, got, taken, err)
				}
				if written := w.request(); written.DaemonPID != w.selfPID || written.DaemonStart != w.selfStart {
					t.Errorf("%s: written %+v; want this process recorded as the carrier", tc.name, written)
				}
			}
		}},
		{"a request taken up in the pausing or confirming phase goes back to waiting with its owned entry lifted, and its sequence pauses afresh and ends done", func(t *testing.T) {
			for _, phase := range []Phase{PhasePausing, PhaseConfirming} {
				w := newFakeWorld(t)
				entry := w.addHubEntry(PauseCaller)
				w.seed(Request{Trigger: TriggerNow, Requester: requesterExternal, DaemonPID: 7, DaemonStart: "s7", Owned: refOf(entry), ResumeOnWake: true, Phase: phase})

				got, taken, err := takeUp(w.deps())

				if err != nil || !taken || got.Phase != PhaseWaiting || got.Owned != nil || !strings.Contains(got.Reason, "ended in phase "+string(phase)) {
					t.Fatalf("%s: got %+v, taken %v, %v; want it back at waiting owning nothing, the phase named", phase, got, taken, err)
				}
				if live := w.liveEntries(); len(live) != 0 {
					t.Errorf("%s: live %+v; want the owned entry lifted by the takeover", phase, live)
				}

				ended, err := carry(w.deps(), got)

				if err != nil || ended.Phase != PhaseDone || w.pauseCount() != 1 || w.hibernates() != 1 {
					t.Errorf("%s: ended %+v, %v, pauses %d, hibernate calls %d; want done after one fresh pause and one hibernate", phase, ended, err, w.pauseCount(), w.hibernates())
				}
			}
		}},
		{"a request taken up in the hibernating or resuming phase ends failed with one notice, its entry lifted only when it resumes on wake", func(t *testing.T) {
			for _, phase := range []Phase{PhaseHibernating, PhaseResuming} {
				for _, resume := range []bool{true, false} {
					w := newFakeWorld(t)
					entry := w.addHubEntry(PauseCaller)
					w.seed(Request{Trigger: TriggerNow, Requester: requesterExternal, DaemonPID: 7, DaemonStart: "s7", Owned: refOf(entry), ResumeOnWake: resume, Phase: phase})

					ended, err := runCarrier(w.deps())

					if err != nil || ended.Phase != PhaseFailed || !strings.Contains(ended.Reason, "ended in phase "+string(phase)) || w.pauseCount() != 0 || w.hibernates() != 0 {
						t.Errorf("%s resume %v: ended %+v, %v, pauses %d, hibernate calls %d; want failed naming the phase with no pause and no hibernate", phase, resume, ended, err, w.pauseCount(), w.hibernates())
					}
					wantLive := 1
					outcome := keptPauseOutcome
					if resume {
						wantLive, outcome = 0, "its pause was lifted."
					}
					if live := w.liveEntries(); len(live) != wantLive {
						t.Errorf("%s resume %v: live %+v; want %d", phase, resume, live, wantLive)
					}
					parts := []string{"The hibernate request failed", outcome}
					if phase == PhaseResuming {
						parts = append(parts, "wake reconcile may not have run", "lym cleanup --apply")
					}
					requireNotice(t, w, parts...)
				}
			}
		}},
		{"a takeover whose lift fails writes nothing and returns the error", func(t *testing.T) {
			w := newFakeWorld(t)
			entry := w.addHubEntry(PauseCaller)
			w.seed(Request{Trigger: TriggerNow, DaemonPID: 7, DaemonStart: "s7", Owned: refOf(entry), Phase: PhaseConfirming})
			before := readBytes(t, w.requestPath)
			w.liftErr = errors.New("record locked")

			_, taken, err := takeUp(w.deps())

			requireRefusal(t, err, "record locked")
			if taken || string(readBytes(t, w.requestPath)) != string(before) {
				t.Errorf("taken %v; want nothing taken and the file as it was", taken)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestCarrier(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	t.Run("Tick carries the request in a goroutine of its own, holds a re-exec only past the trigger wait, and starts no second sequence while one runs", func(t *testing.T) {
		t.Parallel()
		w := newFakeWorld(t)
		w.runs["p1"] = []RunStatus{running("Run-Shed"), {State: shedengine.StateDone, Producer: "Land"}}
		w.seed(Request{Trigger: TriggerAfter, Slug: "p1", Requester: requesterExternal, RequestedAt: w.wall, ResumeOnWake: true, Phase: PhaseWaiting})
		inTriggerWait, leaveTriggerWait := make(chan struct{}), make(chan struct{})
		inConfirmation, leaveConfirmation := make(chan struct{}), make(chan struct{})
		var waitOnce sync.Once
		w.onSleep = func() {
			waitOnce.Do(func() {
				close(inTriggerWait)
				<-leaveTriggerWait
			})
		}
		w.onStatus = func(call int) {
			if call == 0 {
				close(inConfirmation)
				<-leaveConfirmation
			}
		}
		carrier := NewCarrier(w.deps())

		if err := carrier.Tick(); err != nil {
			t.Fatal(err)
		}
		<-inTriggerWait
		if carrier.HoldsReexec() {
			t.Error("HoldsReexec() = true in the trigger wait; want false")
		}
		if err := carrier.Tick(); err != nil {
			t.Error(err)
		}
		close(leaveTriggerWait)
		<-inConfirmation
		if !carrier.HoldsReexec() {
			t.Error("HoldsReexec() = false while confirming; want true")
		}
		if err := carrier.Tick(); err != nil {
			t.Error(err)
		}
		close(leaveConfirmation)
		ended := waitTerminal(t, w)

		if ended.Phase != PhaseDone || ended.DaemonPID != w.selfPID || w.pauseCount() != 1 || w.hibernates() != 1 {
			t.Errorf("ended %+v, pauses %d, hibernate calls %d; want done, carried by this process, after one pause and one hibernate", ended, w.pauseCount(), w.hibernates())
		}
		// The request file turns terminal before the goroutine lifts, notifies and ends, so the hold releases a little later.
		holdDeadline := time.Now().Add(10 * time.Second)
		for carrier.HoldsReexec() {
			if time.Now().After(holdDeadline) {
				t.Fatal("HoldsReexec() = true long after the request ended; want false once the sequence ended")
			}
			time.Sleep(time.Millisecond)
		}
	})
	t.Run("HoldsReexec keeps holding after the request turns terminal until the sequence has sent its notice", func(t *testing.T) {
		t.Parallel()
		w := newFakeWorld(t)
		w.seed(w.ownRequest(TriggerNow, ""))
		inNotice, leaveNotice := make(chan struct{}), make(chan struct{})
		var noticeOnce sync.Once
		w.onNotify = func() {
			noticeOnce.Do(func() {
				close(inNotice)
				<-leaveNotice
			})
		}
		carrier := NewCarrier(w.deps())

		if err := carrier.Tick(); err != nil {
			t.Fatal(err)
		}
		<-inNotice
		if ended := w.request(); !ended.Terminal() {
			t.Errorf("request %+v; want it terminal while the notice is still unsent", ended)
		}
		if !carrier.HoldsReexec() {
			t.Error("HoldsReexec() = false while the notice is unsent; want true")
		}
		close(leaveNotice)

		holdDeadline := time.Now().Add(10 * time.Second)
		for carrier.HoldsReexec() {
			if time.Now().After(holdDeadline) {
				t.Fatal("HoldsReexec() = true long after the notice was sent; want false")
			}
			time.Sleep(time.Millisecond)
		}
	})
	t.Run("a sequence that ends after a cancel and a fresh submit touches the request file, and the next Tick carries the new request", func(t *testing.T) {
		t.Parallel()
		w := newFakeWorld(t)
		w.runs["p1"] = []RunStatus{running("Run-Shed")}
		w.seed(w.ownRequest(TriggerAfter, "p1"))
		inTriggerWait, leaveTriggerWait := make(chan struct{}), make(chan struct{})
		var waitOnce sync.Once
		w.onSleep = func() {
			waitOnce.Do(func() {
				close(inTriggerWait)
				<-leaveTriggerWait
			})
		}
		carrier := NewCarrier(w.deps())
		if err := carrier.Tick(); err != nil {
			t.Fatal(err)
		}
		<-inTriggerWait
		// The fresh request names no carrier, so the sequence in flight no longer owns the file.
		w.seed(Request{Trigger: TriggerNow, Requester: requesterExternal, RequestedAt: w.now(), Phase: PhaseWaiting})
		stale := time.Now().Add(-time.Hour)
		if err := os.Chtimes(w.requestPath, stale, stale); err != nil {
			t.Fatal(err)
		}

		close(leaveTriggerWait)
		deadline := time.Now().Add(10 * time.Second)
		for {
			info, err := os.Stat(w.requestPath)
			if err == nil && info.ModTime().After(stale) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the request file was never touched after the sequence ended: stat %v", err)
			}
			time.Sleep(time.Millisecond)
		}
		if err := carrier.Tick(); err != nil {
			t.Fatal(err)
		}
		ended := waitTerminal(t, w)

		if ended.Phase != PhaseDone || ended.DaemonPID != w.selfPID {
			t.Errorf("ended %+v; want the fresh request done, carried by this process", ended)
		}
	})
}
