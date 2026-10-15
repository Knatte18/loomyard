// watch_test.go drives the daemon's sequence over fakeWorld.

package hibernateengine

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

func sessionReading(strand, state string) tenderengine.SessionStatus {
	return tenderengine.SessionStatus{Slug: "p1", Strand: strand, State: state, Cause: "test"}
}

// watchNow seeds a now request owned by the world's own daemon and runs Watch over it.
func watchNow(t *testing.T, w *fakeWorld) (Request, error) {
	t.Helper()
	w.seed(w.ownRequest(TriggerNow, ""))
	return Watch(w.deps())
}

// resumeLive lifts the live hub entry that caller added.
func resumeLive(t *testing.T, w *fakeWorld, caller string) {
	t.Helper()
	for _, entry := range w.liveEntries() {
		if entry.Scope == pauserecord.ScopeHub && entry.Caller == caller {
			if _, err := w.lift(*refOf(entry)); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no live hub entry of %q to resume in %+v", caller, w.liveEntries())
}

func countSleeps(w *fakeWorld, d time.Duration) int {
	count := 0
	for _, slept := range w.sleepList() {
		if slept == d {
			count++
		}
	}
	return count
}

// requireNotice fails unless exactly one notice was queued and it contains every part.
func requireNotice(t *testing.T, w *fakeWorld, parts ...string) {
	t.Helper()
	notices := w.noticeList()
	if len(notices) != 1 {
		t.Fatalf("notices = %q; want exactly one", notices)
	}
	if strings.HasPrefix(notices[0], "/") || strings.Contains(notices[0], "\n") {
		t.Errorf("notice %q starts with / or spans lines; the queue refuses both", notices[0])
	}
	for _, part := range parts {
		if !strings.Contains(notices[0], part) {
			t.Errorf("notice = %q; want it to contain %q", notices[0], part)
		}
	}
}

func TestWatch(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"now pauses the hub once, confirms, hibernates once, sees the wake and lifts the entry it owns", func(t *testing.T) {
			w := newFakeWorld(t)
			w.caller = orchStrand(t)
			var ownedAtFirstReading *tenderengine.EntryRef
			w.onStatus = func(call int) {
				if call == 0 {
					ownedAtFirstReading = w.request().Owned
				}
			}

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseDone || w.request().Phase != PhaseDone {
				t.Fatalf("got %+v, %v; want phase done", got, err)
			}
			if w.pauseCount() != 1 || w.pauses[0].Grace != 10*time.Second {
				t.Errorf("pauses = %+v; want one with a ten second grace", w.pauses)
			}
			if ref := ownedAtFirstReading; ref == nil || ref.Scope != pauserecord.ScopeHub || ref.Caller != PauseCaller || !ref.PausedAt.Equal(w.start) {
				t.Errorf("owned entry at the first reading = %+v; want the hub entry the pause added", ref)
			}
			if w.hibernates() != 1 || len(w.liveEntries()) != 0 {
				t.Errorf("hibernate calls %d, live entries %+v; want one call and the entry lifted", w.hibernates(), w.liveEntries())
			}
			requireNotice(t, w, "woke from hibernation", "its pause was lifted.")
		}},
		{"an owned entry already resumed, with another hub entry added meanwhile, lifts nothing and leaves that entry live", func(t *testing.T) {
			w := newFakeWorld(t)
			acted := false
			w.onSleep = func() {
				if acted || w.hibernates() == 0 {
					return
				}
				acted = true
				resumeLive(t, w, PauseCaller)
				w.addHubEntry("someone")
			}

			got, err := watchNow(t, w)

			live := w.liveEntries()
			if err != nil || got.Phase != PhaseDone || len(live) != 1 || live[0].Caller != "someone" {
				t.Errorf("got %+v, %v, live %+v; want done with the other entry still live", got, err, live)
			}
			requireNotice(t, w, "its pause had already been lifted.")
		}},
		{"a hub entry live before the request is neither owned nor lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			w.addHubEntry("EXTERNAL")

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseDone || got.Owned != nil || w.request().Owned != nil || len(w.liveEntries()) != 1 || w.hibernates() != 1 {
				t.Errorf("got %+v, %v, live %+v; want done, nothing owned and the entry live", got, err, w.liveEntries())
			}
			requireNotice(t, w, "it owned no pause, so the hub pause that stood before the request still stands.")
		}},
		{"a pause resumed while the readings withhold cancels the request with no hibernate", func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				before func(w *fakeWorld)
				caller string
			}{
				{"an entry that stood before the request", func(w *fakeWorld) { w.addHubEntry("EXTERNAL") }, "EXTERNAL"},
				{"the entry the daemon owns", func(*fakeWorld) {}, PauseCaller},
			} {
				w := newFakeWorld(t)
				tc.before(w)
				w.readings = []statusReading{{sessions: []tenderengine.SessionStatus{sessionReading("a", "busy")}}, {}}
				resumed := false
				w.onSleep = func() {
					if !resumed {
						resumed = true
						resumeLive(t, w, tc.caller)
					}
				}

				got, err := watchNow(t, w)

				if err != nil || got.Phase != PhaseCancelled || w.hibernates() != 0 {
					t.Errorf("%s: got %+v, %v, hibernate calls %d; want cancelled and no hibernate", tc.name, got, err, w.hibernates())
				}
			}
		}},
		{"the check before the hibernate cancels on a lifted pause or a cancelled request and calls nothing", func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				act        func(t *testing.T, w *fakeWorld)
				wantReason string
			}{
				{"the owned entry resumed", func(t *testing.T, w *fakeWorld) { resumeLive(t, w, PauseCaller) }, "the pause was lifted before the hibernate"},
				{"the request cancelled", func(t *testing.T, w *fakeWorld) {
					if _, err := Cancel(w.deps()); err != nil {
						t.Error(err)
					}
				}, "cancelled by EXTERNAL"},
			} {
				w := newFakeWorld(t)
				// Call 0 is the confirming reading and call 1 the check before the hibernate.
				w.onStatus = func(call int) {
					if call == 1 {
						tc.act(t, w)
					}
				}

				got, err := watchNow(t, w)

				final := w.request()
				if err != nil || final.Phase != PhaseCancelled || final.Reason != tc.wantReason || w.hibernates() != 0 {
					t.Errorf("%s: got %+v, final %+v, %v, hibernate calls %d; want cancelled with %q and no hibernate", tc.name, got, final, err, w.hibernates(), tc.wantReason)
				}
			}
		}},
		{"a pause report that was resumed during the grace cancels the request with no reading", func(t *testing.T) {
			w := newFakeWorld(t)
			w.resumedDuringGrace = true

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseCancelled || w.statusCalls != 0 || w.hibernates() != 0 {
				t.Errorf("got %+v, %v, status calls %d, hibernate calls %d; want cancelled before any reading", got, err, w.statusCalls, w.hibernates())
			}
		}},
		{"a pause error fails the request, lifts the owned entry and queues one notice", func(t *testing.T) {
			w := newFakeWorld(t)
			w.pauseErr = errors.New("tender down")

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseFailed || !strings.Contains(got.Reason, "tender down") || len(w.liveEntries()) != 0 || w.hibernates() != 0 {
				t.Errorf("got %+v, %v, live %+v; want failed with the error and the entry lifted", got, err, w.liveEntries())
			}
			requireNotice(t, w, "The hibernate request failed: tender down", "its pause was lifted.")
		}},
		{"a failed write of the owned entry still lifts the entry the pause added", func(t *testing.T) {
			w := newFakeWorld(t)
			w.onPause = func() {
				if err := os.Remove(w.requestPath); err != nil {
					t.Error(err)
				}
				if err := os.Mkdir(w.requestPath, 0o755); err != nil {
					t.Error(err)
				}
			}

			_, err := watchNow(t, w)

			if err == nil || len(w.liveEntries()) != 0 || w.hibernates() != 0 {
				t.Errorf("err %v, live %+v; want the write's error and the entry lifted", err, w.liveEntries())
			}
			requireNotice(t, w, "its pause was lifted.")
		}},
		{"idle-stalled, idle-fresh, asking and dead sessions confirm at the first reading for an external caller", func(t *testing.T) {
			w := newFakeWorld(t)
			w.readings = []statusReading{{sessions: []tenderengine.SessionStatus{
				sessionReading("a", "idle-stalled"), sessionReading("b", "idle-fresh"), sessionReading("c", "asking"), sessionReading("d", "dead"),
			}}}

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseDone || countSleeps(w, confirmInterval) != 0 || w.hibernates() != 1 {
				t.Errorf("got %+v, %v; want done with no re-read", got, err)
			}
		}},
		{"a status reading error counts as unconfirmed and a later clean reading confirms", func(t *testing.T) {
			w := newFakeWorld(t)
			w.readings = []statusReading{{err: errors.New("pane read failed")}, {}}
			var hibernatesAtSleep []int
			w.onSleep = func() { hibernatesAtSleep = append(hibernatesAtSleep, w.hibernates()) }

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseDone || countSleeps(w, confirmInterval) != 1 || w.hibernates() != 1 || hibernatesAtSleep[0] != 0 {
				t.Errorf("got %+v, %v, sleeps %v; want one re-read and the hibernate after it", got, err, w.sleepList())
			}
		}},
		{"each withholding condition holds the hibernate until the first reading where it is gone", func(t *testing.T) {
			background := sessionReading("a", "busy")
			background.Cause = "background"
			prime := sessionReading("orch", "busy")
			prime.Prime = true
			withholding := func(reading statusReading) func(w *fakeWorld) {
				return func(w *fakeWorld) { w.readings = []statusReading{reading, {}} }
			}
			for _, tc := range []struct {
				name  string
				setup func(w *fakeWorld)
			}{
				{"a pair session reading busy", withholding(statusReading{sessions: []tenderengine.SessionStatus{sessionReading("a", "busy")}})},
				{"a session busy with background work", withholding(statusReading{sessions: []tenderengine.SessionStatus{background}})},
				{"a session reading unknown", withholding(statusReading{sessions: []tenderengine.SessionStatus{sessionReading("a", "unknown")}})},
				{"the orch reading busy", withholding(statusReading{sessions: []tenderengine.SessionStatus{prime}})},
				{"a strand the pause left busy", func(w *fakeWorld) { w.stillBusy = []string{"a"} }},
				{"a session whose interrupt was refused", func(w *fakeWorld) {
					w.refused = []tenderengine.SessionNote{{Strand: "a", Slug: "p1", Reason: "the pane is not ready"}}
				}},
				{"a network operation in flight", withholding(statusReading{netOps: []tenderengine.NetOpReport{{Op: "push", Strand: "a", Slug: "p1"}}})},
				{"the land lock held", func(w *fakeWorld) { w.landLock = []bool{true, false} }},
			} {
				w := newFakeWorld(t)
				tc.setup(w)
				var hibernatesAtSleep []int
				w.onSleep = func() { hibernatesAtSleep = append(hibernatesAtSleep, w.hibernates()) }

				got, err := watchNow(t, w)

				if err != nil || got.Phase != PhaseDone || countSleeps(w, confirmInterval) != 1 || w.hibernates() != 1 || hibernatesAtSleep[0] != 0 {
					t.Errorf("%s: got %+v, %v, sleeps %v; want one withheld reading and the hibernate after it", tc.name, got, err, w.sleepList())
				}
			}
		}},
		{"a parent lock held alone confirms at the first reading", func(t *testing.T) {
			w := newFakeWorld(t)
			w.readings = []statusReading{{holds: []tenderengine.HeldBranch{{Branch: "main", Slug: "p1", Queued: []string{"p2"}}}}}

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseDone || countSleeps(w, confirmInterval) != 0 {
				t.Errorf("got %+v, %v; want done with no re-read", got, err)
			}
		}},
		{"unconfirmed re-reads fail the request after ten minutes with the entry lifted and no hibernate", func(t *testing.T) {
			w := newFakeWorld(t)
			w.readings = []statusReading{{sessions: []tenderengine.SessionStatus{sessionReading("a", "busy")}}}

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseFailed || !strings.Contains(got.Reason, "a reads busy") || w.elapsed() != 10*time.Minute || w.hibernates() != 0 || len(w.liveEntries()) != 0 {
				t.Errorf("got %+v, %v, elapsed %s, live %+v; want failed after exactly ten minutes with the entry lifted", got, err, w.elapsed(), w.liveEntries())
			}
			requireNotice(t, w, "The hibernate request failed: a reads busy", "its pause was lifted.")
		}},
		{"a hibernate error fails the request with the entry lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			w.hibernateErr = errors.New("job refused")

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseFailed || !strings.Contains(got.Reason, "job refused") || len(w.liveEntries()) != 0 {
				t.Errorf("got %+v, %v, live %+v; want failed with the error and the entry lifted", got, err, w.liveEntries())
			}
			requireNotice(t, w, "job refused", "its pause was lifted.")
		}},
		{"no sleep within five minutes of awake time after the hibernate fails the request with the entry lifted", func(t *testing.T) {
			w := newFakeWorld(t)
			w.noWake = true

			got, err := watchNow(t, w)

			if err != nil || got.Phase != PhaseFailed || got.Reason != "no wake was seen within five minutes" || countSleeps(w, wakePoll) != 300 || len(w.liveEntries()) != 0 {
				t.Errorf("got %+v, %v, live %+v; want failed after 300 wake polls with the entry lifted", got, err, w.liveEntries())
			}
			requireNotice(t, w, "no wake was seen within five minutes", "its pause was lifted.")
		}},
		{"an after trigger waits through the run's rows with a doubling poll, then runs the whole sequence", func(t *testing.T) {
			w := newFakeWorld(t)
			w.runs["p1"] = []RunStatus{
				running("Run-Shed"), running("Run-Shed"), running("Run-Shed"), running("Run-Shed"), running("Land"),
				{State: shedengine.StateBlocked, Producer: battenrecipe.NameWorktreeTeardown},
			}
			w.seed(w.ownRequest(TriggerAfter, "p1"))
			var pausesAtSleep []int
			var firstWaiting Request
			w.onSleep = func() {
				if len(pausesAtSleep) == 0 {
					firstWaiting = w.request()
				}
				pausesAtSleep = append(pausesAtSleep, w.pauseCount())
			}

			got, err := Watch(w.deps())

			if err != nil || got.Phase != PhaseDone {
				t.Fatalf("got %+v, %v; want done", got, err)
			}
			wantSleeps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, time.Second, wakePoll}
			if !reflect.DeepEqual(w.sleepList(), wantSleeps) {
				t.Errorf("sleeps = %v; want %v, the poll doubling and returning to its base when the reason changes", w.sleepList(), wantSleeps)
			}
			if !reflect.DeepEqual(pausesAtSleep, []int{0, 0, 0, 0, 0, 1}) {
				t.Errorf("pauses at each sleep = %v; want none until the Worktree-Teardown reading", pausesAtSleep)
			}
			if firstWaiting.Phase != PhaseWaiting || firstWaiting.Reason != "waiting for p1 to finish Land, now at Run-Shed" {
				t.Errorf("request while waiting = %+v; want the waiting reason written", firstWaiting)
			}
		}},
		{"an after trigger fires at the first reading of a run that is done or already at its teardown", func(t *testing.T) {
			for _, status := range []RunStatus{{State: shedengine.StateDone, Producer: "Land"}, running(battenrecipe.NameWorktreeTeardown)} {
				w := newFakeWorld(t)
				w.runs["p1"] = []RunStatus{status}
				w.seed(w.ownRequest(TriggerAfter, "p1"))

				got, err := Watch(w.deps())

				if err != nil || got.Phase != PhaseDone || !reflect.DeepEqual(w.sleepList(), []time.Duration{wakePoll}) {
					t.Errorf("%+v: got %+v, %v, sleeps %v; want done with no trigger wait", status, got, err, w.sleepList())
				}
			}
		}},
		{"an after trigger on a run halted before Land keeps waiting and ends when the request is cancelled", func(t *testing.T) {
			w := newFakeWorld(t)
			w.runs["p1"] = []RunStatus{{State: shedengine.StateBlocked, Producer: "Run-Shed"}}
			w.seed(w.ownRequest(TriggerAfter, "p1"))
			var waiting Request
			w.onSleep = func() {
				waiting = w.request()
				if _, err := Cancel(w.deps()); err != nil {
					t.Error(err)
				}
			}

			_, err := Watch(w.deps())

			if err != nil || w.pauseCount() != 0 || waiting.Phase != PhaseWaiting || !strings.Contains(waiting.Reason, "halted at Run-Shed (blocked)") {
				t.Errorf("err %v, pauses %d, request while waiting %+v; want the halt reason shown and no pause", err, w.pauseCount(), waiting)
			}
			if final := w.request(); final.Phase != PhaseCancelled || final.Reason != "cancelled by EXTERNAL" {
				t.Errorf("final request = %+v; want the cancel's write left alone", final)
			}
		}},
		{"an after trigger whose status goes missing fails the request with no pause", func(t *testing.T) {
			w := newFakeWorld(t)
			w.runs["p1"] = []RunStatus{running("Run-Shed")}
			w.seed(w.ownRequest(TriggerAfter, "p1"))
			w.onSleep = func() {
				w.mu.Lock()
				defer w.mu.Unlock()
				w.runsMissing["p1"] = true
			}

			got, err := Watch(w.deps())

			if err != nil || got.Phase != PhaseFailed || got.Reason != "the batten status of p1 is gone" || w.pauseCount() != 0 {
				t.Errorf("got %+v, %v, pauses %d; want failed with the reason and no pause", got, err, w.pauseCount())
			}
		}},
		{"an after-all trigger fires once no run is in flight, at once with no runs, and skips a slug whose status is missing", func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				setup      func(w *fakeWorld)
				wantSleeps []time.Duration
			}{
				{"one run finishing beside a halted one", func(w *fakeWorld) {
					w.runs["a"] = []RunStatus{running("Run-Shed"), {State: shedengine.StateDone, Producer: "Land"}}
					w.runs["b"] = []RunStatus{{State: shedengine.StateBlocked, Producer: "Run-Shed"}}
				}, []time.Duration{time.Second, wakePoll}},
				{"no runs", func(*fakeWorld) {}, []time.Duration{wakePoll}},
				{"a listed slug with no status", func(w *fakeWorld) {
					w.runs["a"] = []RunStatus{{State: shedengine.StateDone, Producer: "Land"}}
					w.runSlugs = []string{"a", "gone"}
					w.runsMissing["gone"] = true
				}, []time.Duration{wakePoll}},
			} {
				w := newFakeWorld(t)
				tc.setup(w)
				w.seed(w.ownRequest(TriggerAfterAll, ""))

				got, err := Watch(w.deps())

				if err != nil || got.Phase != PhaseDone || !reflect.DeepEqual(w.sleepList(), tc.wantSleeps) {
					t.Errorf("%s: got %+v, %v, sleeps %v; want done with sleeps %v", tc.name, got, err, w.sleepList(), tc.wantSleeps)
				}
			}
		}},
		{"no request, a terminal one and another daemon's pause and hibernate nothing and leave the file alone", func(t *testing.T) {
			w := newFakeWorld(t)
			if got, err := Watch(w.deps()); err != nil || !reflect.DeepEqual(got, Request{}) {
				t.Errorf("no request: got %+v, %v; want nothing", got, err)
			}
			requireNoRequestFile(t, w)

			done := w.ownRequest(TriggerNow, "")
			done.Phase = PhaseDone
			another := w.ownRequest(TriggerNow, "")
			another.DaemonPID = 9
			for name, r := range map[string]Request{"a terminal request": done, "another daemon's request": another} {
				w.seed(r)
				before := readBytes(t, w.requestPath)

				got, err := Watch(w.deps())

				if err != nil || !reflect.DeepEqual(got, Request{}) || string(readBytes(t, w.requestPath)) != string(before) {
					t.Errorf("%s: got %+v, %v; want nothing done and the file as it was", name, got, err)
				}
			}
			if w.pauseCount() != 0 || w.hibernates() != 0 {
				t.Errorf("pauses %d, hibernate calls %d; want none", w.pauseCount(), w.hibernates())
			}
		}},
		{"an agent caller and a non-Linux platform are refused with no pause", func(t *testing.T) {
			w := newFakeWorld(t)
			w.caller = agentStrand(t)
			_, err := watchNow(t, w)
			requireRefusal(t, err, "ask your parent session")

			w = newFakeWorld(t)
			w.goos = "darwin"
			_, err = watchNow(t, w)
			requireRefusal(t, err, "Linux only")
			if w.pauseCount() != 0 {
				t.Errorf("pauses = %d; want none", w.pauseCount())
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
