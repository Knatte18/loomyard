// waitreminder_test.go pins the wait reminders over a fake child reading and fake presence, interval and post seams: which waits are reminded, when, and what restarts them.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// reminderFake is what the wait reminder seams of one test read, and what they posted.
type reminderFake struct {
	away        bool
	awayErr     error
	awayReads   int
	intervalErr error
	resumed     time.Time
	resumeErr   error
	postErr     error
	posts       []string
}

// newReminderHarness returns a notice harness whose child waits at PR-Review with the operator away and a thirty-minute interval, and the fake its reminder seams read.
func newReminderHarness(t *testing.T) (*passHarness, *noticeFake, *reminderFake) {
	t.Helper()
	h, f := newNoticeHarness(t)
	f.child.CurrentProducer = "PR-Review"
	r := &reminderFake{away: true}
	h.d.deps.WaitReminders = WaitReminderDeps{
		Rows:         []string{"PR-Review", "Sampler-Session"},
		LatestResume: func(string) (time.Time, error) { return r.resumed, r.resumeErr },
		Away: func() (bool, error) {
			r.awayReads++
			return r.away, r.awayErr
		},
		Interval: func() (time.Duration, error) { return 30 * time.Minute, r.intervalErr },
		Post: func(_ context.Context, line string) error {
			if r.postErr != nil {
				return r.postErr
			}
			r.posts = append(r.posts, line)
			return nil
		},
	}
	return h, f, r
}

// minutes runs n ticks one minute apart, each one wait check.
func (h *passHarness) minutes(n int) {
	h.t.Helper()
	for range n {
		h.advance(50 * time.Second)
		h.tick(false)
	}
}

func TestWaitReminders(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	waited := func(minutes ...string) []string {
		var lines []string
		for _, count := range minutes {
			lines = append(lines, "task: waiting at PR-Review for "+count+" minutes")
		}
		return lines
	}

	tests := []struct {
		name string
		// setup runs before the first check, and act after it.
		setup func(h *passHarness, f *noticeFake, r *reminderFake)
		act   func(h *passHarness, f *noticeFake, r *reminderFake)
		want  []string
	}{
		{
			name: "NoneBeforeTheInterval",
			act:  func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(29) },
		},
		{
			name: "OneLinePerIntervalWhileTheWaitLasts",
			act:  func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(65) },
			want: waited("30", "60"),
		},
		{
			name:  "SamplerSessionIsReminded",
			setup: func(_ *passHarness, f *noticeFake, _ *reminderFake) { f.child.CurrentProducer = "Sampler-Session" },
			act:   func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(30) },
			want:  []string{"task: waiting at Sampler-Session for 30 minutes"},
		},
		{
			name:  "ARowOutsideTheListIsNot",
			setup: func(_ *passHarness, f *noticeFake, _ *reminderFake) { f.child.CurrentProducer = "Beam-Await" },
			act:   func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(61) },
		},
		{
			name:  "TerminalPresenceGetsNoneAndReturningRemindsAtOnceOfAWaitPastTheInterval",
			setup: func(_ *passHarness, _ *noticeFake, r *reminderFake) { r.away = false },
			act: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				h.minutes(45)
				r.away = true
				h.minutes(1)
			},
			want: waited("46"),
		},
		{
			name: "AReturnToSlackRemindsAtOnceWhateverTheLastReminder",
			act: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				h.minutes(35)
				r.away = false
				h.minutes(1)
				r.away = true
				h.minutes(1)
			},
			want: waited("30", "37"),
		},
		{
			name: "APauseTheDobbyReadRestartsTheWait",
			act: func(h *passHarness, f *noticeFake, _ *reminderFake) {
				h.minutes(20)
				f.pause.Covered = true
				h.minutes(5)
				f.pause.Covered = false
				h.minutes(31)
			},
			want: waited("30"),
		},
		{
			name: "APauseAndResumeNoCheckSawRestartsTheWaitAndItsReminders",
			act: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				h.minutes(31)
				r.resumed = h.clock.now()
				h.minutes(30)
			},
			want: waited("30", "30"),
		},
		{
			name: "AResumeBeforeTheWaitBeganMovesNothing",
			setup: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				r.resumed = h.clock.now().Add(-time.Hour)
			},
			act:  func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(30) },
			want: waited("30"),
		},
		{
			name: "ARunWithACancelMarkerIsSkippedAndRecordsNoWait",
			setup: func(h *passHarness, _ *noticeFake, _ *reminderFake) {
				h.fake.runs[0].CancelMarker = true
			},
			act: func(h *passHarness, _ *noticeFake, _ *reminderFake) {
				h.minutes(31)
				if got := h.state().OperatorWait; got != (OperatorWaitState{}) {
					h.t.Errorf("OperatorWait of a cancel-marked run = %+v; want none", got)
				}
			},
		},
		{
			name: "AStandingAnchorChangeStopsTheWaitCheck",
			setup: func(h *passHarness, _ *noticeFake, _ *reminderFake) {
				record := filepath.Join(h.t.TempDir(), "anchor-move.json")
				if err := os.WriteFile(record, []byte("{}"), 0o644); err != nil {
					h.t.Fatalf("WriteFile(%s) = %v; want nil", record, err)
				}
				h.d.geom.MoveRecords = []string{record}
			},
			act: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				h.minutes(31)
				if r.awayReads != 0 {
					h.t.Errorf("wait checks under a standing anchor change = %d; want 0", r.awayReads)
				}
			},
		},
		{
			name: "LeavingTheRowEndsTheWait",
			act: func(h *passHarness, f *noticeFake, _ *reminderFake) {
				h.minutes(20)
				f.child.CurrentProducer = "Finalize"
				h.minutes(1)
				f.child.CurrentProducer = "PR-Review"
				h.minutes(29)
			},
		},
		{
			name: "MovingToTheOtherRowStartsANewWait",
			act: func(h *passHarness, f *noticeFake, _ *reminderFake) {
				h.minutes(20)
				f.child.CurrentProducer = "Sampler-Session"
				h.minutes(31)
			},
			want: []string{"task: waiting at Sampler-Session for 30 minutes"},
		},
		{
			name: "AChildThatStopsRunningEndsTheWait",
			act: func(h *passHarness, f *noticeFake, _ *reminderFake) {
				h.minutes(20)
				f.child.State = "blocked"
				h.minutes(1)
				f.child.State = childStateRunning
				h.minutes(29)
			},
		},
		{
			name: "ARunBattenStoppedEndsTheWait",
			act: func(h *passHarness, _ *noticeFake, _ *reminderFake) {
				h.minutes(20)
				h.fake.runs[0].State = "blocked"
				h.minutes(1)
				h.fake.runs[0].State = runStateRunning
				h.minutes(29)
			},
		},
		{
			name: "APostThatFailsIsTriedAtTheNextCheck",
			act: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				r.postErr = errors.New("creel refused")
				h.minutes(30)
				r.postErr = nil
				h.minutes(1)
			},
			want: waited("31"),
		},
		{
			name: "ARestartKeepsTheStartAndMayRemindOnceEarly",
			act: func(h *passHarness, _ *noticeFake, _ *reminderFake) {
				h.minutes(20)
				h.restartDobby()
				h.minutes(15)
				h.restartDobby()
				h.minutes(1)
			},
			want: waited("30", "36"),
		},
		{
			name: "AChildThatDoesNotReadKeepsItsWaitAndGetsNoReminder",
			act: func(h *passHarness, f *noticeFake, _ *reminderFake) {
				h.minutes(29)
				f.childErr = errors.New("unreadable")
				h.minutes(5)
				f.childErr = nil
				h.minutes(1)
			},
			want: waited("35"),
		},
		{
			name: "AResumeThatDoesNotReadKeepsTheWaitAndGetsNoReminder",
			act: func(h *passHarness, _ *noticeFake, r *reminderFake) {
				h.minutes(29)
				r.resumeErr = errors.New("unreadable")
				h.minutes(5)
				r.resumeErr = nil
				h.minutes(1)
			},
			want: waited("35"),
		},
		{
			name:  "APresenceThatDoesNotReadGetsNone",
			setup: func(_ *passHarness, _ *noticeFake, r *reminderFake) { r.awayErr = errors.New("unreadable") },
			act:   func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(31) },
		},
		{
			name:  "AnIntervalThatDoesNotReadGetsNone",
			setup: func(_ *passHarness, _ *noticeFake, r *reminderFake) { r.intervalErr = errors.New("unreadable") },
			act:   func(h *passHarness, _ *noticeFake, _ *reminderFake) { h.minutes(31) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, f, r := newReminderHarness(t)
			if tt.setup != nil {
				tt.setup(h, f, r)
			}
			h.tick(false)
			tt.act(h, f, r)
			if !slices.Equal(r.posts, tt.want) {
				t.Errorf("posts = %q; want %q", r.posts, tt.want)
			}
		})
	}
}

func TestWaitReminders_CheckAtMostOnceAMinute(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	h, _, r := newReminderHarness(t)

	for range 6 {
		h.tick(false)
	}
	if r.awayReads != 1 {
		t.Errorf("checks over six ticks ten seconds apart = %d; want 1", r.awayReads)
	}
	h.tick(false)
	if r.awayReads != 2 {
		t.Errorf("checks once a minute passed = %d; want 2", r.awayReads)
	}
}

func TestWaitReminders_RecordTheWaitInTheStateRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	h, f, _ := newReminderHarness(t)

	h.tick(false)
	started := h.clock.now()
	h.minutes(2)
	if got := h.state().OperatorWait; got.Row != "PR-Review" || !got.Since.Equal(started) {
		t.Errorf("OperatorWait = %+v; want PR-Review since %s", got, started)
	}

	f.pause.Covered = true
	h.minutes(1)
	if got := h.state().OperatorWait; got != (OperatorWaitState{}) {
		t.Errorf("OperatorWait under a pause = %+v; want none", got)
	}
}
