// pausedsend_test.go covers the refusal of a send into a session a pause covers: which slug judges the target, when coverage is read, and what a refused send leaves untyped.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// swallowingReed is a fakeReed whose pane reads busy until idleAt and idle after, then shows the last text that landed.
// The first swallow SendText calls land nowhere, as when a provider TUI drops the input.
type swallowingReed struct {
	*fakeReed

	clock   Clock
	idleAt  time.Time
	swallow int
	landed  string
}

func (r *swallowingReed) SendText(guid, text string, submit bool) error {
	if err := r.fakeReed.SendText(guid, text, submit); err != nil {
		return err
	}
	if r.swallow > 0 {
		r.swallow--
		return nil
	}
	r.landed = text
	return nil
}

func (r *swallowingReed) CapturePane(string) (string, error) {
	if r.clock.Now().Before(r.idleAt) {
		return "working", nil
	}
	return "IDLE\n" + r.landed, nil
}

// TestSend_RefusesACoveredTarget pins the send refusal: a *PausedError wrapping ErrPaused, judged by the target's own slug, with nothing typed.
//
// It is not parallel: logcapture redirects the process-global logger.
func TestSend_RefusesACoveredTarget(t *testing.T) {
	start := time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// scope is the live entry's scope, empty for a record holding none.
		scope string
		// geomSlug is the slug the runner was told; noGeometry tells the runner none.
		geomSlug   string
		noGeometry bool
		// strand is the strand's name: on the run record, or in reed's status for a recordless send.
		strand     string
		recordless bool
		// corrupt writes an undecodable record, and sends is how many sends the row makes, one when zero.
		corrupt bool
		sends   int
		// lateScope, when set, adds an entry of that scope to the record at the first Sleep of the send's idle wait, or with swallowFirst at the first Sleep of its verify polls.
		lateScope    string
		swallowFirst bool
		// wantSlug is the refusal's slug; wantRefused false makes the row type.
		wantRefused bool
		wantSlug    string
		// wantTyped is how many SendText calls the row makes.
		wantTyped int
		wantWarns int
	}{
		{name: "a hub entry refuses a send into a task strand from a runner of another slug", scope: pauserecord.ScopeHub, geomSlug: "other", strand: "ly:p1:impl", wantRefused: true, wantSlug: "p1"},
		{name: "a slug entry refuses that same send, the runner's own slug reading uncovered", scope: "p1", geomSlug: "other", strand: "ly:p1:impl", wantRefused: true, wantSlug: "p1"},
		{name: "a slug entry does not refuse a send into a prime strand", scope: "p1", geomSlug: "p1", strand: "ly:impl", wantTyped: 1},
		{name: "a hub entry refuses a send into a prime strand", scope: pauserecord.ScopeHub, geomSlug: "other", strand: "ly:impl", wantRefused: true, wantSlug: ""},
		{name: "a name that does not parse falls back to the runner's slug", scope: "p1", geomSlug: "p1", strand: "not-a-name", wantRefused: true, wantSlug: "p1"},
		{name: "a slug entry refuses a send into a live strand no run record names", scope: "p1", geomSlug: "other", strand: "ly:p1:impl", recordless: true, wantRefused: true, wantSlug: "p1"},
		{name: "a pause added while the send waits for an idle session refuses with nothing typed", geomSlug: "other", strand: "ly:p1:impl", lateScope: pauserecord.ScopeHub, wantRefused: true, wantSlug: "p1"},
		{name: "a pause added after a swallowed first typing refuses the replay", geomSlug: "other", strand: "ly:p1:impl", lateScope: pauserecord.ScopeHub, swallowFirst: true, wantRefused: true, wantSlug: "p1", wantTyped: 1},
		{name: "a runner told no geometry types under a live hub entry", scope: pauserecord.ScopeHub, noGeometry: true, strand: "ly:p1:impl", wantTyped: 1},
		{name: "an undecodable record types and warns once across two sends", corrupt: true, geomSlug: "other", strand: "ly:p1:impl", sends: 2, wantTyped: 2, wantWarns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			var clock Clock = newFakeClock(start)
			recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
			switch {
			case tt.corrupt:
				if err := os.WriteFile(recordPath, []byte("{not json"), 0o644); err != nil {
					t.Fatalf("write corrupt record: %v", err)
				}
			case tt.scope != "":
				addLiveEntry(t, recordPath, tt.scope, start)
			}
			reed := &swallowingReed{
				fakeReed: &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Name: tt.strand, PaneID: "%0", Live: true}}}}},
				clock:    clock,
			}
			if tt.lateScope != "" {
				lifted := &liftClock{Clock: clock, liftAfter: 1, lift: func() { addLiveEntry(t, recordPath, tt.lateScope, start) }}
				clock = lifted
				reed.clock = lifted
				if tt.swallowFirst {
					reed.swallow = 1
				} else {
					reed.idleAt = start.Add(300 * time.Millisecond)
				}
			}
			cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5, SendReadyTimeoutS: 60}
			engine := &idleEngine{}
			engine.StartupScript = []StartupState{StartupReady}
			fx := newFixture(t, reed, engine, withConfig(cfg), withClock(clock))
			if !tt.noGeometry {
				fx.Runner.SetPause(pauserecord.Geometry{RecordPath: recordPath, Slug: tt.geomSlug})
			}
			run := fx.newRun(Spec{}, withRunState(RunState{StrandGUID: "strand-1", StrandName: tt.strand}), withRunEvents(""), withRunClock(clock, start.Add(time.Hour)))

			var err error
			for i := range max(tt.sends, 1) {
				text := "hello"
				if i > 0 {
					text = "hello again"
				}
				if tt.recordless {
					err = fx.Runner.Send("strand-1", text)
				} else {
					err = run.Send(text)
				}
				if err != nil {
					break
				}
			}

			if got := len(reed.SendTextCalls); got != tt.wantTyped {
				t.Errorf("SendText calls = %d, want %d", got, tt.wantTyped)
			}
			if got := len(reed.PasteSubmitCalls); got != 0 {
				t.Errorf("PasteSubmit calls = %d, want none", got)
			}
			if got := countLevelLines(buf.String(), "WARN", "pause record unreadable"); got != tt.wantWarns {
				t.Errorf("pause record WARN lines = %d, want %d; log: %s", got, tt.wantWarns, buf.String())
			}
			if !tt.wantRefused {
				if err != nil {
					t.Fatalf("Send: %v", err)
				}
				return
			}
			var paused *PausedError
			if !errors.Is(err, ErrPaused) || !errors.As(err, &paused) {
				t.Fatalf("Send error = %v, want a *PausedError wrapping ErrPaused", err)
			}
			if errors.Is(err, ErrSessionBusy) {
				t.Errorf("Send error %v wraps ErrSessionBusy, want a refusal of its own", err)
			}
			if paused.Slug != tt.wantSlug {
				t.Errorf("PausedError.Slug = %q, want %q", paused.Slug, tt.wantSlug)
			}
			if !paused.Entry.ResumedAt.IsZero() || paused.Entry.PausedAt.IsZero() {
				t.Errorf("PausedError.Entry = %+v, want the live covering entry", paused.Entry)
			}
		})
	}

	t.Run("a failed reed status judges an unnamed target by the runner's slug and warns once", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		fx := newFixture(t, &fakeReed{StatusErr: errors.New("reed gone")}, &idleEngine{})
		fx.Runner.SetPause(pauserecord.Geometry{RecordPath: filepath.Join(t.TempDir(), pauserecord.RecordFileName), Slug: "p9"})
		for range 2 {
			if got := fx.Runner.targetSlug("strand-1", ""); got != "p9" {
				t.Errorf("targetSlug = %q, want the runner's slug p9", got)
			}
		}
		if got := countLevelLines(buf.String(), "WARN", "reed status unreadable"); got != 1 {
			t.Errorf("reed status WARN lines = %d, want 1; log: %s", got, buf.String())
		}
	})
}
