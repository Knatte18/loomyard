// holdreading_test.go pins HoldReading over a synthetic task worktree on disk.

package arbiterstop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// holdFixture is one task worktree under a temp hub, with the loom config seeded.
type holdFixture struct {
	t        *testing.T
	location *lyxcwd.Location
	status   shedengine.Status
	attempts int
}

// holdBase is the fixed time the fixture's records are written at, so every expected time derives from it.
var holdBase = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

func newHoldFixture(t *testing.T, producer string, state shedengine.State) *holdFixture {
	t.Helper()
	location := locationkit.Location(t.TempDir(), "pair", ".")
	f := &holdFixture{
		t:        t,
		location: location,
		status: shedengine.Status{
			CurrentProducer: producer,
			State:           state,
			History:         []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Done}},
		},
	}
	f.writeConfig(loomengine.ConfigTemplate())
	return f
}

// writeConfig writes content as loom.yaml in both the board dir and the anchor, as a seeded hub holds it.
func (f *holdFixture) writeConfig(content string) {
	f.t.Helper()
	for _, base := range []string{fabricengine.BoardDir(f.location.HubPath), f.location.AnchorPath()} {
		dir := filepath.Join(base, "_lyx", "config")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "loom.yaml"), []byte(content), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
}

// escalate lays out Plan-Bouncer's run directory with a round-1 review and an escalation of cause, and the decision file when decision is non-empty.
func (f *holdFixture) escalate(cause, decision string) {
	f.t.Helper()
	runDir := filepath.Join(loomengine.LoomReviewsDir(f.location), "plan")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	files := map[string]string{
		"round-1-review.md":     "review\n",
		"round-1-escalation.md": "---\nround: 1\ncause: " + cause + "\n---\nbrief\n",
	}
	if decision != "" {
		files["round-1-circling-decision.md"] = "---\nround: 1\ndecision: " + decision + "\n---\n"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
}

// attempt writes the next attempt record at key with deadline, and returns its directory.
func (f *holdFixture) attempt(key sheddriver.StopKey, deadline time.Time) string {
	f.t.Helper()
	f.attempts++
	dir := filepath.Join(shedrun.ArbiterDir(f.location, shedrun.SelfRunID), "attempt-"+string(rune('0'+f.attempts)))
	if err := loomengine.WriteArbiterAttempt(dir, loomengine.ArbiterAttempt{Number: f.attempts, StopKey: key, Deadline: deadline}); err != nil {
		f.t.Fatal(err)
	}
	return dir
}

// envelope writes the attempt directory's envelope with verdict, written at at.
func (f *holdFixture) envelope(dir, verdict string, at time.Time) {
	f.t.Helper()
	path := loomengine.ArbiterEnvelopePath(dir)
	if err := loomengine.WriteArbiterEnvelope(path, loomengine.ArbiterEnvelope{Verdict: verdict}); err != nil {
		f.t.Fatal(err)
	}
	f.chtimes(path, at)
}

// applied marks the attempt directory applied at at.
func (f *holdFixture) applied(dir string, at time.Time) {
	f.t.Helper()
	if err := loomengine.MarkArbiterApplied(dir); err != nil {
		f.t.Fatal(err)
	}
	f.chtimes(loomengine.ArbiterAppliedPath(dir), at)
}

func (f *holdFixture) chtimes(path string, at time.Time) {
	f.t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		f.t.Fatal(err)
	}
}

// settle appends a settle record at key with action, at at.
func (f *holdFixture) settle(key sheddriver.StopKey, action string, at time.Time) {
	f.t.Helper()
	path := shedrun.SettleRecords(f.location, shedrun.SelfRunID)
	if err := sheddriver.AppendSettleRecord(path, sheddriver.SettleRecord{StopKey: key, Settler: sheddriver.SettlerGoRule, Action: action, At: at}); err != nil {
		f.t.Fatal(err)
	}
}

// utc normalizes every time of reading to UTC, so readings compare whatever zone a file time came back in.
func utc(reading battenshed.ChildArbiterStop) battenshed.ChildArbiterStop {
	reading.AttemptDeadline = reading.AttemptDeadline.UTC()
	reading.HandOnAt = reading.HandOnAt.UTC()
	reading.AppliedAt = reading.AppliedAt.UTC()
	reading.HandOverAt = reading.HandOverAt.UTC()
	return reading
}

// TestHoldReading pins HoldReading's whole reading for each stop layout.
func TestHoldReading(t *testing.T) {
	t.Parallel()
	const timeout = 30 * time.Minute
	eligible := battenshed.ChildArbiterStop{Eligible: true, On: true, Timeout: timeout, SpawnsLeft: true}
	deadline := holdBase.Add(timeout)
	handOnAt := holdBase.Add(time.Minute)
	appliedAt := holdBase.Add(2 * time.Minute)
	otherKey := sheddriver.StopKey{CurrentProducer: "Plan-Bouncer", HistoryLength: 9, State: "awaiting"}
	withAttempt := eligible
	withAttempt.Attempt, withAttempt.AttemptDeadline = true, deadline
	withMarks := withAttempt
	withMarks.HandOnAt, withMarks.AppliedAt = handOnAt, appliedAt
	withHandOver := eligible
	withHandOver.HandOverAt = handOnAt

	tests := []struct {
		name     string
		producer string
		state    shedengine.State
		setup    func(f *holdFixture, key sheddriver.StopKey)
		want     battenshed.ChildArbiterStop
	}{
		{name: "blocked stop at Plan-Write", producer: "Plan-Write", state: shedengine.StateBlocked, want: eligible},
		{name: "failed status is not eligible", producer: "Plan-Write", state: shedengine.StateFailed},
		{
			name: "awaiting at a budget escalation is not eligible", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *holdFixture, _ sheddriver.StopKey) { f.escalate("budget", "") },
		},
		{
			name: "awaiting at a circling escalation with no attempt", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *holdFixture, _ sheddriver.StopKey) { f.escalate("circling", "") },
			want:  eligible,
		},
		{
			name: "arbiter_max_spawns of 0 reads off", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *holdFixture, _ sheddriver.StopKey) {
				f.writeConfig(strings.Replace(loomengine.ConfigTemplate(), "arbiter_max_spawns: 3", "arbiter_max_spawns: 0", 1))
			},
			want: battenshed.ChildArbiterStop{Eligible: true, Timeout: timeout},
		},
		{
			name: "attempts at the cap, none at this key, read no spawns left", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *holdFixture, _ sheddriver.StopKey) {
				for range 3 {
					f.attempt(otherKey, deadline)
				}
			},
			want: battenshed.ChildArbiterStop{Eligible: true, On: true, Timeout: timeout},
		},
		{
			name: "an attempt at the key reads its deadline, hand-on time and applied time", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *holdFixture, key sheddriver.StopKey) {
				f.attempt(otherKey, holdBase)
				dir := f.attempt(key, deadline)
				f.envelope(dir, loomengine.ArbiterVerdictHandOn, handOnAt)
				f.applied(dir, appliedAt)
			},
			want: withMarks,
		},
		{
			name: "a resume envelope leaves HandOnAt zero", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *holdFixture, key sheddriver.StopKey) {
				f.envelope(f.attempt(key, deadline), loomengine.ArbiterVerdictResume, handOnAt)
			},
			want: withAttempt,
		},
		{
			name: "a newest hand-over settle record at the key fills HandOverAt", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *holdFixture, key sheddriver.StopKey) {
				f.settle(key, sheddriver.SettleHandBack, holdBase)
				f.settle(key, sheddriver.SettleHandOver, handOnAt)
				f.settle(otherKey, sheddriver.SettleHandBack, appliedAt)
			},
			want: withHandOver,
		},
		{
			name: "a newer hand-back at the key leaves HandOverAt zero", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *holdFixture, key sheddriver.StopKey) {
				f.settle(key, sheddriver.SettleHandOver, holdBase)
				f.settle(key, sheddriver.SettleHandBack, handOnAt)
			},
			want: eligible,
		},
		{
			name: "awaiting past the arbiter's own decision stays eligible until applied", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *holdFixture, key sheddriver.StopKey) {
				f.escalate("circling", "continue")
				f.envelope(f.attempt(key, deadline), loomengine.ArbiterVerdictResume, handOnAt)
			},
			want: withAttempt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newHoldFixture(t, tt.producer, tt.state)
			if tt.setup != nil {
				tt.setup(f, sheddriver.StopKeyOf(f.status))
			}
			got, err := HoldReading(f.location, f.status)
			if err != nil {
				t.Fatalf("HoldReading = %v; want nil", err)
			}
			if utc(got) != utc(tt.want) {
				t.Errorf("HoldReading = %+v; want %+v", got, tt.want)
			}
		})
	}
}
