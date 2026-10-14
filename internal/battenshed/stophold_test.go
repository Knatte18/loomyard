// stophold_test.go covers the eligible-stop hold: when the state-changed notice of an arbiter-eligible stop waits for the arbiter, and when the report wait starts after the hold.

package battenshed

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// holdScenario is one row's running producer, with the time the stop began.
type holdScenario struct {
	h     *noticeHarness
	p     shedengine.ShedProducer
	since time.Time
}

// holdStep moves the fake clock to at after the stop began, applies mutate, makes one check and requires wantSent notices in all by then.
type holdStep struct {
	at       time.Duration
	mutate   func(s *holdScenario)
	wantSent int
}

// TestNotice_EligibleStopHold pins the hold over a blocked or awaiting stop whose reading is eligible: no notice while the arbiter may still settle the stop, and noticeReportWait counted from the hold's end.
func TestNotice_EligibleStopHold(t *testing.T) {
	const timeout = 30 * time.Minute
	eligible := ChildArbiterStop{Eligible: true, On: true, Timeout: timeout, SpawnsLeft: true}
	minutes := func(n int) time.Duration { return time.Duration(n) * time.Minute }
	reading := func(change func(r *ChildArbiterStop)) func(s *holdScenario) {
		return func(s *holdScenario) {
			r := s.h.arbiterStop
			change(&r)
			s.h.arbiterStop = r
		}
	}
	// attempt gives the reading an attempt at the stop's key whose deadline is the usual timeout after the stop began, then applies change.
	attempt := func(change func(since time.Time, r *ChildArbiterStop)) func(s *holdScenario) {
		return func(s *holdScenario) {
			r := s.h.arbiterStop
			r.Attempt, r.AttemptDeadline = true, s.since.Add(timeout)
			if change != nil {
				change(s.since, &r)
			}
			s.h.arbiterStop = r
		}
	}
	restart := func(s *holdScenario) { s.p = s.h.producer() }
	// awaiting is a watched awaiting stop with a parent notice, whose notice waits for its driver's relay.
	awaiting := func(h *noticeHarness) { h.watched = true }

	tests := []struct {
		name   string
		status shedengine.Status
		setup  func(h *noticeHarness)
		steps  []holdStep
		// wantText is a substring the last notice must carry.
		wantText string
	}{
		{
			name: "no arbiter marker yet holds until the backstop plus the report wait", status: parked(shedengine.StateBlocked),
			steps: []holdStep{{at: minutes(4), wantSent: 0}, {at: minutes(32), wantSent: 0}, {at: minutes(34), wantSent: 0}, {at: minutes(37), wantSent: 1}},
		},
		{
			name: "a marker whose strand is not alive holds", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(1), mutate: func(s *holdScenario) {
					s.h.arbiter = ChildArbiter{Deadline: s.since.Add(timeout), Alive: false}
					s.h.arbiterFound = true
				}},
				{at: minutes(4), wantSent: 0},
			},
		},
		{
			name: "an attempt with no marker and no hand-on holds", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(1), mutate: attempt(nil)},
				{at: minutes(10), wantSent: 0},
			},
		},
		{
			name: "a resume envelope with no applied mark holds until the mark, then the report wait runs from it", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(10), mutate: attempt(nil), wantSent: 0},
				{at: minutes(13), mutate: attempt(func(since time.Time, r *ChildArbiterStop) { r.AppliedAt = since.Add(minutes(11)) }), wantSent: 0},
				{at: minutes(15), wantSent: 1},
			},
		},
		{
			name: "a tackler-led attempt past the usual backstop holds to its own deadline plus the report wait", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(1), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.Attempt, r.AttemptDeadline = true, s.since.Add(minutes(45))
					s.h.arbiterStop = r
				}},
				{at: minutes(40), wantSent: 0},
				{at: minutes(47), wantSent: 0},
				{at: minutes(52), wantSent: 1},
			},
		},
		{
			name: "a park marker after the stop sends the notice at once with its report", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(5), mutate: func(s *holdScenario) { s.h.writeReport(s.since.Add(minutes(2))) }, wantSent: 1},
			},
			wantText: "report _lyx/stop-report.md",
		},
		{
			name: "a hand-on long after the stop at a watched awaiting stop, with no park marker, waits the report wait from the hand-on",
			status: func() shedengine.Status {
				s := parked(shedengine.StateAwaiting)
				s.ParentNotice = "pull request ready"
				return s
			}(),
			setup: awaiting,
			steps: []holdStep{
				{at: minutes(1), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.Attempt, r.AttemptDeadline, r.HandOnAt = true, s.since.Add(timeout), s.since.Add(minutes(5))
					s.h.arbiterStop = r
				}},
				{at: minutes(6), wantSent: 0},
				{at: minutes(7), wantSent: 0},
				{at: minutes(9), wantSent: 1},
			},
		},
		{
			name: "a park marker after the hand-on waits the relay wait from the marker",
			status: func() shedengine.Status {
				s := parked(shedengine.StateAwaiting)
				s.ParentNotice = "pull request ready"
				return s
			}(),
			setup: awaiting,
			steps: []holdStep{
				{at: minutes(1), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.Attempt, r.AttemptDeadline, r.HandOnAt = true, s.since.Add(timeout), s.since.Add(minutes(5))
					s.h.arbiterStop = r
				}},
				{at: minutes(7), mutate: func(s *holdScenario) { s.h.writeReport(s.since.Add(minutes(6))) }, wantSent: 0},
				{at: minutes(15), wantSent: 0},
				{at: minutes(17), wantSent: 1},
			},
		},
		{
			name: "the arbiter switched off mid-hold ends it, and the report wait runs from the pass that saw it off", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(5), wantSent: 0},
				{at: minutes(10), mutate: reading(func(r *ChildArbiterStop) { r.On = false }), wantSent: 0},
				{at: minutes(12), wantSent: 0},
				{at: minutes(14), wantSent: 1},
			},
		},
		{
			name: "a park marker with a hand-over at or after the stop keeps the hold until the successor's attempt hands on", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(4), mutate: func(s *holdScenario) {
					s.h.writeReport(s.since.Add(minutes(3)))
					r := s.h.arbiterStop
					r.HandOverAt = s.since.Add(minutes(2))
					s.h.arbiterStop = r
				}, wantSent: 0},
				{at: minutes(10), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.Attempt, r.AttemptDeadline, r.HandOnAt = true, s.since.Add(timeout), s.since.Add(minutes(9))
					s.h.arbiterStop = r
				}, wantSent: 1},
			},
		},
		{
			name: "a restart mid-hold still holds until the same backstop", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(5), wantSent: 0},
				{at: minutes(10), mutate: restart, wantSent: 0},
				{at: minutes(20), wantSent: 0},
				{at: minutes(37), wantSent: 1},
			},
		},
		{
			name: "a restart after a hand-on measures the report wait from the hand-on", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(6), mutate: attempt(nil), wantSent: 0},
				{at: minutes(7), mutate: func(s *holdScenario) {
					attempt(func(since time.Time, r *ChildArbiterStop) { r.HandOnAt = since.Add(minutes(5)) })(s)
					restart(s)
				}, wantSent: 0},
				{at: minutes(9), wantSent: 1},
			},
		},
		{
			name: "a restart after an applied resume measures the report wait from the mark", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(12), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.Attempt, r.AttemptDeadline, r.AppliedAt = true, s.since.Add(timeout), s.since.Add(minutes(11))
					s.h.arbiterStop = r
					restart(s)
				}, wantSent: 0},
				{at: minutes(15), wantSent: 1},
			},
		},
		{
			name: "a restart after the backstop measures the report wait from the backstop", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(34), mutate: restart, wantSent: 0},
				{at: minutes(37), wantSent: 1},
			},
		},
		{
			name: "a restart after the arbiter was switched off takes its own first observation", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(5), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.On = false
					s.h.arbiterStop = r
				}, wantSent: 0},
				{at: minutes(20), mutate: restart, wantSent: 0},
				{at: minutes(22), wantSent: 0},
				{at: minutes(24), wantSent: 1},
			},
		},
		{
			name: "the driver strand ending holds nothing during the hold and sends at the backstop", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(10), mutate: func(s *holdScenario) { s.h.alive = false }, wantSent: 0},
				{at: minutes(34), wantSent: 1},
			},
		},
		{
			name: "a reading that is not eligible does not start the hold", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: 0, mutate: reading(func(r *ChildArbiterStop) { r.Eligible = false }), wantSent: 0},
				{at: minutes(4), wantSent: 1},
			},
		},
		{
			name: "an arbiter off from the start does not start the hold", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: 0, mutate: reading(func(r *ChildArbiterStop) { r.On = false }), wantSent: 0},
				{at: minutes(4), wantSent: 1},
			},
		},
		{
			name: "no spawns left and no attempt at the key does not start the hold", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(1), mutate: reading(func(r *ChildArbiterStop) { r.SpawnsLeft = false }), wantSent: 0},
				{at: minutes(4), wantSent: 1},
			},
		},
		{
			name: "an applied mark from before the stop does not start the hold", status: parked(shedengine.StateBlocked),
			steps: []holdStep{
				{at: minutes(1), mutate: func(s *holdScenario) {
					r := s.h.arbiterStop
					r.Attempt, r.AttemptDeadline, r.AppliedAt = true, s.since.Add(timeout), s.since.Add(-minutes(1))
					s.h.arbiterStop = r
				}, wantSent: 0},
				{at: minutes(4), wantSent: 1},
			},
		},
		{
			name: "an error stop keeps the marker hold alone", status: parked(shedengine.StateFailed),
			steps: []holdStep{{at: minutes(1), wantSent: 0}, {at: minutes(4), wantSent: 1}},
		},
		{
			name: "a seam error holds nothing", status: parked(shedengine.StateBlocked),
			setup: func(h *noticeHarness) { h.arbiterStopErr = errors.New("worktree unreadable") },
			steps: []holdStep{{at: minutes(1), wantSent: 0}, {at: minutes(4), wantSent: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNoticeHarness(t)
			h.arbiterStop = eligible
			if tt.setup != nil {
				tt.setup(h)
			}
			p := h.producer()
			h.leaveRunning(p, tt.status)
			s := &holdScenario{h: h, p: p, since: h.touched}
			for i, step := range tt.steps {
				if target := s.since.Add(step.at); target.After(h.clock.Now()) {
					h.clock.advance(target.Sub(h.clock.Now()))
				}
				if step.mutate != nil {
					step.mutate(s)
				}
				h.call(s.p)
				if got := len(h.notified); got != step.wantSent {
					t.Fatalf("after step %d (+%s): notified = %v; want %d notice(s)", i, step.at, h.notified, step.wantSent)
				}
			}
			if tt.wantText != "" && !strings.Contains(h.notified[len(h.notified)-1], tt.wantText) {
				t.Errorf("last notice = %q; want it to contain %q", h.notified[len(h.notified)-1], tt.wantText)
			}
		})
	}
}
