//go:build tier0

// beamawait_test.go covers the Beam-Await row over a fake beam: the accept path, every refusal path, the entry's markers and notices, the owner check and the wait.

package pawlengine_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// cancelAfter makes the n-th sleep end the scenario.
func cancelAfter(w *world, n int) {
	w.onSleep = func(count int) {
		if count == n {
			w.cancel()
		}
	}
}

// TestBeamAwait asserts the row's behavior over each state a run can enter it in and each answer to a land request:
// what it writes in order, which notices it posts, which files stand afterwards, and how long it waits.
func TestBeamAwait(t *testing.T) {
	t.Parallel()

	const (
		beamAwait   = "status:Beam-Await"
		parentAlign = "status:Parent-Align"
		stopped     = "stop marker=true"
	)
	failOnce := func(producer string, call int) func(string, int) error {
		return func(got string, n int) error {
			if got == producer && n == call {
				return errors.New("status write refused")
			}
			return nil
		}
	}
	passing := []string{beamAwait, parentAlign, stopped, "recheck", "snapshot"}
	refused := []string{beamAwait, parentAlign, stopped, "recheck", beamAwait, "start"}
	seconds := func(ns ...int) []time.Duration {
		out := make([]time.Duration, len(ns))
		for i, n := range ns {
			out[i] = time.Duration(n) * time.Second
		}
		return out
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, w *world)
		// wantOutcome is the verdict of a row that ends; a row the scenario cancels wants wantCancelled instead.
		wantOutcome   shedengine.Outcome
		wantCancelled bool
		wantEvents    []string
		// wantNotices holds, per posted notice in order, the substrings it carries.
		wantNotices [][]string
		// wantFiles maps request, stop and await to whether that file stands afterwards, and record to whether the await marker carries the posted dobby-down notice.
		wantFiles      map[string]bool
		wantDobbyReads int
		// wantSleeps is checked when non-nil.
		wantSleeps []time.Duration
	}{
		{
			name:        "AcceptedRequestThatPassesLandsAndClearsEveryFile",
			setup:       func(t *testing.T, w *world) { w.request(t, testRunID) },
			wantOutcome: shedengine.Done, wantEvents: passing,
			wantFiles: map[string]bool{"request": false, "stop": false, "await": false},
		},
		{
			name: "RequestOfAnotherRunIsRemovedAndTheRowWaits",
			setup: func(t *testing.T, w *world) {
				w.request(t, "an-earlier-run")
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait},
			wantFiles: map[string]bool{"request": false, "await": true},
		},
		{
			name: "FailingRecheckLiftsTheHoldRestartsTheDobbyAndPostsTheBlockers",
			setup: func(t *testing.T, w *world) {
				w.blockers = []string{"member a is open"}
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: refused,
			wantNotices: [][]string{{"member a is open", "lym beam land my-beam"}},
			wantFiles:   map[string]bool{"request": false, "stop": false, "await": true},
			wantSleeps:  seconds(1),
		},
		{
			name: "FailingRestartKeepsTheMarkerAndNamesBeamStart",
			setup: func(t *testing.T, w *world) {
				w.blockers = []string{"member a is open"}
				w.startErr = errors.New("dobby will not start")
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: refused,
			wantNotices: [][]string{{"member a is open", "dobby will not start", "lym beam start my-beam"}},
			wantFiles:   map[string]bool{"request": false, "stop": true},
		},
		{
			name: "RefusalThatRestartsTheDobbyClearsTheRecordedNotice",
			setup: func(t *testing.T, w *world) {
				w.blockers = []string{"member a is open"}
				seedDownNotice(t, w)
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: refused,
			wantNotices: [][]string{{"member a is open"}},
			wantFiles:   map[string]bool{"record": false, "await": true},
		},
		{
			name: "RefusalWhoseRestartFailsKeepsTheRecordedNotice",
			setup: func(t *testing.T, w *world) {
				w.blockers = []string{"member a is open"}
				w.startErr = errors.New("dobby will not start")
				seedDownNotice(t, w)
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: refused,
			wantNotices: [][]string{{"dobby will not start"}},
			wantFiles:   map[string]bool{"record": true},
		},
		{
			name: "FailingParentAlignWriteKeepsTheRequestForTheNextWake",
			setup: func(t *testing.T, w *world) {
				w.statusFail = failOnce("Parent-Align", 1)
				w.request(t, testRunID)
			},
			wantOutcome: shedengine.Done,
			wantEvents:  []string{beamAwait, parentAlign + "(failed)", parentAlign, stopped, "recheck", "snapshot"},
			wantFiles:   map[string]bool{"request": false, "stop": false, "await": false},
			wantSleeps:  seconds(1),
		},
		{
			name: "ReentryAtParentAlignWithNoRequestLiftsTheHoldAndPostsOneInterruptedNotice",
			setup: func(t *testing.T, w *world) {
				w.status = "Parent-Align"
				touch(t, w.paths.DobbyStopMarker)
				touch(t, w.paths.AwaitMarker)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait},
			wantNotices: [][]string{{"interrupted", "lym beam start my-beam", "lym beam land my-beam"}},
			wantFiles:   map[string]bool{"stop": false, "await": true},
		},
		{
			name: "ReentryWithNoRequestAndALiveDobbyPostsNothing",
			setup: func(t *testing.T, w *world) {
				w.status = "Beam-Await"
				touch(t, w.paths.AwaitMarker)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait}, wantDobbyReads: 1,
		},
		{
			name: "ReentryWithNoRequestAndADownDobbyPostsOneDobbyDownNotice",
			setup: func(t *testing.T, w *world) {
				w.status = "Beam-Await"
				w.dobbyLive = false
				touch(t, w.paths.AwaitMarker)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait}, wantDobbyReads: 1,
			wantNotices: [][]string{{"dobby is down", "lym beam start my-beam"}},
			wantFiles:   map[string]bool{"record": true},
		},
		{
			name: "ASecondReentryWithTheDobbyStillDownPostsNothing",
			setup: func(t *testing.T, w *world) {
				w.status = "Beam-Await"
				w.dobbyLive = false
				seedDownNotice(t, w)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait}, wantDobbyReads: 1,
			wantFiles: map[string]bool{"record": true},
		},
		{
			name: "ReentryThatFindsTheDobbyLiveAgainClearsTheRecordedNotice",
			setup: func(t *testing.T, w *world) {
				w.status = "Beam-Await"
				seedDownNotice(t, w)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait}, wantDobbyReads: 1,
			wantFiles: map[string]bool{"record": false, "await": true},
		},
		{
			name: "ANoticeThatCouldNotBePostedIsNotRecordedSoTheNextReentryPostsAgain",
			setup: func(t *testing.T, w *world) {
				w.status = "Beam-Await"
				w.dobbyLive = false
				w.notifyErr = errors.New("no orch strand")
				touch(t, w.paths.AwaitMarker)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait}, wantDobbyReads: 1,
			wantNotices: [][]string{{"dobby is down"}},
			wantFiles:   map[string]bool{"record": false},
		},
		{
			name: "FirstEntryReadsNoDobbyPostsNothingAndWritesTheAwaitMarker",
			setup: func(t *testing.T, w *world) {
				w.dobbyLive = false
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait},
			wantFiles: map[string]bool{"await": true},
		},
		{
			name: "ABeamAnotherMachineOwnsRemovesTheRequestAndYields",
			setup: func(t *testing.T, w *world) {
				w.owned = true
				w.request(t, testRunID)
			},
			wantOutcome: shedengine.Yield,
			wantFiles:   map[string]bool{"request": false},
		},
		{
			name: "FailedEntryStatusWriteIsRetriedAtTheNextWake",
			setup: func(t *testing.T, w *world) {
				w.statusFail = failOnce("Beam-Await", 1)
				cancelAfter(w, 2)
			},
			wantCancelled: true, wantEvents: []string{beamAwait + "(failed)", beamAwait},
			wantFiles:  map[string]bool{"await": true},
			wantSleeps: seconds(1, 2),
		},
		{
			name: "FailingBeamAwaitWriteAfterARefusalKeepsRequestAndMarkerUntilTheNextWakeRetries",
			setup: func(t *testing.T, w *world) {
				w.blockers = []string{"member a is open"}
				w.statusFail = failOnce("Beam-Await", 2)
				w.request(t, testRunID)
				cancelAfter(w, 2)
			},
			wantCancelled: true,
			wantEvents: []string{
				beamAwait, parentAlign, stopped, "recheck", beamAwait + "(failed)",
				parentAlign, stopped, "recheck", beamAwait, "start",
			},
			wantSleeps:  seconds(1, 1),
			wantNotices: [][]string{{"member a is open", "lym beam land my-beam"}},
			wantFiles:   map[string]bool{"request": false, "stop": false},
		},
		{
			name: "RecheckErrorIsAFailedRecheckWithTheErrorAsItsBlocker",
			setup: func(t *testing.T, w *world) {
				w.recheckErr = errors.New("board unreadable")
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: refused,
			wantNotices: [][]string{{"board unreadable", "lym beam land my-beam"}},
			wantFiles:   map[string]bool{"request": false, "stop": false},
		},
		{
			name: "FailingDobbyStopIsAFailedRecheckThatStillRestartsTheDobby",
			setup: func(t *testing.T, w *world) {
				w.stopErr = errors.New("no such process")
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait, parentAlign, stopped, beamAwait, "start"},
			wantNotices: [][]string{{"no such process", "lym beam land my-beam"}},
			wantFiles:   map[string]bool{"request": false, "stop": false},
		},
		{
			name: "FailingSnapshotWriteIsAFailedRecheck",
			setup: func(t *testing.T, w *world) {
				w.snapshotErr = errors.New("disk full")
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: []string{beamAwait, parentAlign, stopped, "recheck", "snapshot", beamAwait, "start"},
			wantNotices: [][]string{{"disk full", "lym beam land my-beam"}},
			wantFiles:   map[string]bool{"request": false, "stop": false},
		},
		{
			name: "FailingNotifyLeavesTheRowWaiting",
			setup: func(t *testing.T, w *world) {
				w.blockers = []string{"member a is open"}
				w.notifyErr = errors.New("no orch strand")
				w.request(t, testRunID)
				cancelAfter(w, 1)
			},
			wantCancelled: true, wantEvents: refused,
			wantNotices: [][]string{{"member a is open"}},
			wantFiles:   map[string]bool{"request": false},
			wantSleeps:  seconds(1),
		},
		{
			name:          "IdleTimerDoublesToTheCeilingWithNoWatch",
			setup:         func(t *testing.T, w *world) { cancelAfter(w, 8) },
			wantCancelled: true, wantEvents: []string{beamAwait},
			wantSleeps: seconds(1, 2, 4, 8, 16, 32, 60, 60),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			tt.setup(t, w)

			outcome, err := w.callAwait(t)
			if tt.wantCancelled {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Call() error = %v; want a cancellation", err)
				}
			} else {
				if err != nil {
					t.Fatalf("Call() error = %v", err)
				}
				if outcome != tt.wantOutcome {
					t.Errorf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
				}
			}
			if diff := cmp.Diff(tt.wantEvents, w.snapshotEvents()); diff != "" {
				t.Errorf("events mismatch (-want +got):\n%s", diff)
			}
			notices := w.snapshotNotices()
			if len(notices) != len(tt.wantNotices) {
				t.Fatalf("posted %d notices %q; want %d", len(notices), notices, len(tt.wantNotices))
			}
			for i, parts := range tt.wantNotices {
				for _, part := range parts {
					if !strings.Contains(notices[i], part) {
						t.Errorf("notice %d = %q; want it to contain %q", i, notices[i], part)
					}
				}
			}
			paths := map[string]string{"request": w.paths.LandRequest, "stop": w.paths.DobbyStopMarker, "await": w.paths.AwaitMarker}
			for key, want := range tt.wantFiles {
				got := exists(paths[key])
				if key == "record" {
					content, _ := os.ReadFile(w.paths.AwaitMarker)
					got = strings.Contains(string(content), "dobby-down-notified")
				}
				if got != want {
					t.Errorf("%s file standing = %t; want %t", key, got, want)
				}
			}
			if w.dobbyReads != tt.wantDobbyReads {
				t.Errorf("DobbyLive read %d times; want %d", w.dobbyReads, tt.wantDobbyReads)
			}
			if tt.wantSleeps != nil {
				if diff := cmp.Diff(tt.wantSleeps, w.snapshotSleeps()); diff != "" {
					t.Errorf("waits mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

// TestBeamAwait_WatchEvent asserts a change event wakes the row ahead of its timer, waits out the rest of the one-second floor and returns the timer to a second.
func TestBeamAwait_WatchEvent(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.watch = make(chan struct{}, 1)
	w.watch <- struct{}{}
	w.blockFirstSleep = true
	cancelAfter(w, 4)

	_, err := w.callAwait(t)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v; want a cancellation", err)
	}
	want := []time.Duration{time.Second, time.Second, time.Second, 2 * time.Second}
	if diff := cmp.Diff(want, w.snapshotSleeps()); diff != "" {
		t.Errorf("waits mismatch (-want +got):\n%s", diff)
	}
}

// TestBeamAwait_CancelledContext asserts a context cancelled before the call is an error and writes nothing.
func TestBeamAwait_CancelledContext(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	producer, err := pawlengine.NewBeamAwait(pawlengine.NameBeamAwait, w.deps())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := producer.Call(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want a cancellation", err)
	}
	if events := w.snapshotEvents(); len(events) != 0 {
		t.Errorf("events = %q; want none", events)
	}
}

// TestNextAwaitWait asserts the timer doubles while idle, stops at a minute, and returns to a second on an event.
func TestNextAwaitWait(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current time.Duration
		event   bool
		want    time.Duration
	}{
		{"DoublesWhileIdle", 4 * time.Second, false, 8 * time.Second},
		{"StartsAtASecondFromNothing", 0, false, time.Second},
		{"StopsAtTheCeiling", 40 * time.Second, false, time.Minute},
		{"StaysAtTheCeiling", time.Minute, false, time.Minute},
		{"ReturnsToASecondOnAnEvent", 32 * time.Second, true, time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := pawlengine.NextAwaitWait(tt.current, tt.event); got != tt.want {
				t.Errorf("NextAwaitWait(%v, %t) = %v; want %v", tt.current, tt.event, got, tt.want)
			}
		})
	}
}
