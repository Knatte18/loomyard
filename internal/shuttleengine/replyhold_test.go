// replyhold_test.go pins the reply hold:
// a turn end that would end a run, or start its gate's evaluation, waits while the run's strand awaits a reply.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// scriptedReplyProbe is a reply probe whose answer a test changes between ticks.
// It records the strands it was asked about.
type scriptedReplyProbe struct {
	mu      sync.Mutex
	awaits  bool
	err     error
	strands []string
}

func (p *scriptedReplyProbe) probe(strand string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.strands = append(p.strands, strand)
	return p.awaits, p.err
}

func (p *scriptedReplyProbe) set(awaits bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.awaits = awaits
}

func (p *scriptedReplyProbe) asked() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.strands)
}

// busyPaneReed is a fakeReed whose pane reads mid-turn while busy is set.
// Otherwise the pane reads idle and shows the last text typed into it, so a send confirms.
type busyPaneReed struct {
	*fakeReed
	busy atomic.Bool
}

func (r *busyPaneReed) CapturePane(string) (string, error) {
	if r.busy.Load() {
		return "RUNNING", nil
	}
	r.fakeReed.mu.Lock()
	defer r.fakeReed.mu.Unlock()
	typed := ""
	if n := len(r.fakeReed.SendTextCalls); n > 0 {
		typed = r.fakeReed.SendTextCalls[n-1].Text
	}
	return "IDLE\n" + typed, nil
}

var _ ReedOps = (*busyPaneReed)(nil)

// replyHoldRun is a run waiting on one seeded turn end while its strand awaits a reply.
type replyHoldRun struct {
	run        *Run
	clk        *multiStepClock
	reed       *busyPaneReed
	probe      *scriptedReplyProbe
	eventsPath string
}

// newReplyHoldRun builds a run over one seeded turn end, with every output file present and the probe awaiting a reply.
// The engine reads session signals and an idle pane.
// A nil gate leaves the run ungated, and a zero pause geometry leaves the runner with no pause record.
// A test scripts the run with at, one step per poll tick.
func newReplyHoldRun(t *testing.T, strand string, probeErr error, gate GateSpec, pause pauserecord.Geometry) *replyHoldRun {
	t.Helper()
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(eventsPath, []byte("STOP:asked my parent\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	touchOutputFile(t, outputFile)

	probe := &scriptedReplyProbe{awaits: true, err: probeErr}
	reed := &busyPaneReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}}
	engine := &idleEngine{sessionFakeEngine: sessionFakeEngine{
		waitingEngine: waitingEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}},
		liveness:      LivenessAlive,
	}}
	fx := newFixture(t, reed, engine, withConfig(gateConfig))
	fx.Runner.SetReplyProbe(probe.probe)
	if pause.RecordPath != "" {
		fx.Runner.SetPause(pause)
	}

	steps := make([]func(), 40)
	for i := range steps {
		steps[i] = func() {}
	}
	clk := &multiStepClock{fakeClock: newFakeClock(time.Now()), steps: steps}
	opts := []runOpt{
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", StrandName: strand, EventsPath: eventsPath}),
		withRunClock(clk, clk.Now().Add(time.Minute)),
	}
	if gate != nil {
		opts = append(opts, withRunGate(gate))
	}
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute}, opts...)
	return &replyHoldRun{run: run, clk: clk, reed: reed, probe: probe, eventsPath: eventsPath}
}

// at runs fn after the sleep of poll tick step, after anything already scripted for that step.
func (h *replyHoldRun) at(step int, fn func()) {
	previous := h.clk.steps[step]
	h.clk.steps[step] = func() {
		previous()
		fn()
	}
}

func TestRun_Wait_ReplyHold(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const askingStrand = "ly:task:darn"
	tests := []struct {
		name string
		// strand is the strand name the run's record holds.
		strand   string
		probeErr error
		gated    bool
		// replyAt is the script step at which the strand stops awaiting a reply, and replyTurn whether a turn end follows then.
		replyAt   int
		replyTurn bool
		// busyUntil is the script step at which the pane stops reading mid-turn; zero leaves the pane idle throughout.
		busyUntil int
		// eventLines maps a script step to a line appended to the events file then.
		eventLines map[int]string
		// wantSteps is how many script steps, one per poll tick, ran before Wait returned.
		wantSteps int
		// wantAsked is whether the probe was asked about the strand.
		wantAsked bool
	}{
		{
			name:   "a reply starts the agent's next turn, and that turn's end finishes the run",
			strand: askingStrand, replyAt: 3, replyTurn: true,
			wantSteps: 4, wantAsked: true,
		},
		{
			name:   "a gated run evaluates its gate only at the turn end after the reply",
			strand: askingStrand, gated: true, replyAt: 3, replyTurn: true,
			wantSteps: 4, wantAsked: true,
		},
		{
			// The probe runs every five seconds of a one-second poll, and two quiet probes end the hold: ten ticks.
			name:   "a question settled with no reply ends the hold after two quiet probes",
			strand: askingStrand, replyAt: 0,
			wantSteps: 10, wantAsked: true,
		},
		{
			name:   "a gated run whose question settles with no reply evaluates its gate when the hold ends",
			strand: askingStrand, gated: true, replyAt: 0,
			wantSteps: 10, wantAsked: true,
		},
		{
			// The reply's turn writes no turn start, so only the pane shows the agent is still working on it.
			name:   "a reply read with no turn start holds the run while the pane reads mid-turn",
			strand: askingStrand, replyAt: 3, busyUntil: 14,
			wantSteps: 15, wantAsked: true,
		},
		{
			name:   "a gated run holds its gate while a reply's turn writes no turn start and the pane reads mid-turn",
			strand: askingStrand, gated: true, replyAt: 3, busyUntil: 14,
			wantSteps: 15, wantAsked: true,
		},
		{
			name:   "a turn start after the reply ends the hold, and only that turn's end finishes the run",
			strand: askingStrand, replyAt: 3, eventLines: map[int]string{3: "START", 20: "STOP:answered"},
			wantSteps: 21, wantAsked: true,
		},
		{
			name:   "a probe error ends the run as a runner with no probe does",
			strand: askingStrand, probeErr: errors.New("creel daemon down"), replyAt: 3, replyTurn: true,
			wantSteps: 0, wantAsked: true,
		},
		{
			name:   "a run whose record names no strand is never probed",
			strand: "", replyAt: 3, replyTurn: true,
			wantSteps: 0, wantAsked: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gateCalls := 0
			var gate GateSpec
			if tt.gated {
				gate = GateSpec{{Gate: func() (GateResult, error) {
					gateCalls++
					return GateResult{Passed: true}, nil
				}, Attempts: 1}}
			}
			h := newReplyHoldRun(t, tt.strand, tt.probeErr, gate, pauserecord.Geometry{})

			gateCallsAtReply := -1
			h.at(tt.replyAt, func() {
				gateCallsAtReply = gateCalls
				h.probe.set(false)
				if tt.replyTurn {
					appendEventsLine(t, h.eventsPath, "STOP:answered")
				}
			})
			gateCallsWhileBusy := -1
			if tt.busyUntil > 0 {
				h.reed.busy.Store(true)
				h.at(tt.busyUntil, func() {
					gateCallsWhileBusy = gateCalls
					h.reed.busy.Store(false)
				})
			}
			for step, line := range tt.eventLines {
				h.at(step, func() { appendEventsLine(t, h.eventsPath, line) })
			}

			result, err := h.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q; want %q", result.Outcome, OutcomeDone)
			}
			if h.clk.next != tt.wantSteps {
				t.Errorf("script steps run before Wait returned = %d; want %d", h.clk.next, tt.wantSteps)
			}
			asked := h.probe.asked()
			if tt.wantAsked != (len(asked) > 0) {
				t.Errorf("probe asked about %q; want asked = %v", asked, tt.wantAsked)
			}
			for _, strand := range asked {
				if strand != tt.strand {
					t.Errorf("probe asked about %q; want only %q", strand, tt.strand)
				}
			}
			if tt.gated {
				if gateCallsAtReply != 0 {
					t.Errorf("gate evaluations before the hold ended = %d; want 0", gateCallsAtReply)
				}
				if gateCalls == 0 {
					t.Errorf("gate evaluations = 0; want the gate evaluated once the hold ended")
				}
				if tt.busyUntil > 0 && gateCallsWhileBusy != 0 {
					t.Errorf("gate evaluations while the pane read mid-turn = %d; want 0", gateCallsWhileBusy)
				}
			}
		})
	}
}

// TestRun_Wait_ReplyHold_ReleaseOfAFailingGateRepromptsOnce pins that a released hold is gone:
// the failing gate is evaluated once and re-prompts once, rather than again at every probe interval.
// It does not run in parallel: stubInputSleep replaces a package-level seam.
func TestRun_Wait_ReplyHold_ReleaseOfAFailingGateRepromptsOnce(t *testing.T) {
	tiergate.Require(t, 0)
	stubInputSleep(t)
	gateCalls := 0
	gate := GateSpec{{Gate: func() (GateResult, error) {
		gateCalls++
		if gateCalls == 1 {
			return GateResult{Findings: "the output misses a section"}, nil
		}
		return GateResult{Passed: true}, nil
	}, Attempts: 2}}
	h := newReplyHoldRun(t, "ly:task:darn", nil, gate, pauserecord.Geometry{})

	h.at(0, func() { h.probe.set(false) })
	// The hold releases at the eleventh tick, so a probe asked after step 12 shows a hold that outlived its release.
	probesAtRelease, probesBeforeTurnEnd := -1, -1
	h.at(12, func() { probesAtRelease = len(h.probe.asked()) })
	gateCallsBeforeTurnEnd, sendsBeforeTurnEnd := -1, -1
	h.at(19, func() {
		gateCallsBeforeTurnEnd = gateCalls
		sendsBeforeTurnEnd = len(h.reed.SendTextCalls)
		probesBeforeTurnEnd = len(h.probe.asked())
	})
	h.at(20, func() { appendEventsLine(t, h.eventsPath, "STOP:fixed") })

	result, err := h.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q", result.Outcome, OutcomeDone)
	}
	if gateCallsBeforeTurnEnd != 1 {
		t.Errorf("gate evaluations before the next turn end = %d; want 1", gateCallsBeforeTurnEnd)
	}
	if sendsBeforeTurnEnd != 1 {
		t.Errorf("re-prompts sent before the next turn end = %d; want 1", sendsBeforeTurnEnd)
	}
	if probesBeforeTurnEnd != probesAtRelease {
		t.Errorf("probes asked between the release and the next turn end = %d; want 0", probesBeforeTurnEnd-probesAtRelease)
	}
	if gateCalls != 2 {
		t.Errorf("gate evaluations = %d; want 2, one per turn end", gateCalls)
	}
}

// TestRun_Wait_ReplyHold_ReleaseUnderAPauseWaitsForTheLift pins a hold that releases while a pause covers the run:
// the gate runs on the first uncovered tick, and not before.
func TestRun_Wait_ReplyHold_ReleaseUnderAPauseWaitsForTheLift(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
	gateCalls := 0
	gate := GateSpec{{Gate: func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: true}, nil
	}, Attempts: 1}}
	h := newReplyHoldRun(t, "ly:task:darn", nil, gate, pauserecord.Geometry{RecordPath: recordPath, Slug: "slug"})

	h.at(0, func() { h.probe.set(false) })
	h.at(1, func() { addLiveEntry(t, recordPath, pauserecord.ScopeHub, h.clk.Now()) })
	gateCallsWhileCovered := -1
	h.at(13, func() {
		gateCallsWhileCovered = gateCalls
		resumedAt := h.clk.Now()
		_, err := pauserecord.Update(recordPath, resumedAt, func(record *pauserecord.Record) error {
			entry := record.Live[0]
			entry.ResumedAt = resumedAt
			record.Live = nil
			record.Resumed = append(record.Resumed, entry)
			return nil
		})
		if err != nil {
			t.Fatalf("lift the pause: %v", err)
		}
	})

	result, err := h.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q", result.Outcome, OutcomeDone)
	}
	if gateCallsWhileCovered != 0 {
		t.Errorf("gate evaluations while the pause covered the run = %d; want 0", gateCallsWhileCovered)
	}
	if gateCalls != 1 {
		t.Errorf("gate evaluations = %d; want 1, on the first uncovered tick", gateCalls)
	}
	if h.clk.next != 14 {
		t.Errorf("script steps run before Wait returned = %d; want 14, the tick after the lift", h.clk.next)
	}
}
