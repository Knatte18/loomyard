// calleefailures_test.go pins that a failure repeating on every tick of the name-repair loop logs once per episode: Warn at the first failure, Debug while it goes on, Info at recovery.

package reedengine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestCalleeFailuresLogOncePerEpisode drives each guarded site three times while its tmux call fails and once after it is healed, on one engine.
// Each site logs its failure message at one level=WARN line and two level=DEBUG lines carrying failed_ticks, and its recovery message at one level=INFO line.
// It captures the process-global logger at Debug, so it stays serial.
func TestCalleeFailuresLogOncePerEpisode(t *testing.T) {
	const failedCalls = 3
	zoomListing := "%1 0 0 10 3 1\n%2 0 3 10 3 2\n"
	keepZoomStep := func(e *Engine) { _ = e.keepZoomLocked(func() error { return nil }) }
	scriptZoomedWindow := func(e *Engine, f *fakeTmux) {
		f.answerFormat(zoomStateFormat, "1 %2", nil)
		f.answer("list-panes", zoomListing, nil)
	}
	healVerb := func(verb, out string) func(e *Engine, f *fakeTmux) {
		return func(_ *Engine, f *fakeTmux) { f.answer(verb, out, nil) }
	}
	failVerb := func(verb string) func(e *Engine, f *fakeTmux, call int) {
		return func(_ *Engine, f *fakeTmux, _ int) { f.answer(verb, "", errors.New(verb+" failed")) }
	}

	tests := []struct {
		name string
		// build returns the engine and fake the row runs on; nil means newTestEngine with installFakeTmux.
		build func(t *testing.T) (*Engine, *fakeTmux)
		// setup scripts the healthy parts of the fake before any call.
		setup func(e *Engine, f *fakeTmux)
		// fail scripts the fake before the failing call with that index; heal scripts it healthy before the last call.
		fail func(e *Engine, f *fakeTmux, call int)
		heal func(e *Engine, f *fakeTmux)
		call func(e *Engine)
		// drive, when set, replaces the call loop and runs the whole row itself.
		drive func(t *testing.T, e *Engine, f *fakeTmux)
		// failMsg is carried by the Warn and Debug lines, healMsg by the Info line.
		failMsg, healMsg string
		// wantInfoAfterHeal is a message that logs at one more level=INFO line after the heal.
		wantInfoAfterHeal string
	}{
		{
			name:    "live window size query",
			fail:    failVerb("display-message"),
			heal:    healVerb("display-message", "220 50"),
			call:    func(e *Engine) { e.liveBoxLocked(exactSessionWindowTarget(e.SessionName())) },
			failMsg: "falling back to configured box",
			healMsg: "reed: live window size query answers again",
		},
		{
			name: "malformed live window size answer joins the query episode",
			fail: func(_ *Engine, f *fakeTmux, call int) {
				if call%2 == 0 {
					f.answer("display-message", "", errors.New("display-message failed"))
					return
				}
				f.answer("display-message", "abc def", nil)
			},
			heal:    healVerb("display-message", "220 50"),
			call:    func(e *Engine) { e.liveBoxLocked(exactSessionWindowTarget(e.SessionName())) },
			failMsg: "falling back to configured box",
			healMsg: "reed: live window size query answers again",
		},
		{
			name:    "pane-border-status readback",
			fail:    failVerb("display-message"),
			heal:    healVerb("display-message", "top"),
			call:    func(e *Engine) { e.readBorderTitleRowLocked(exactSessionWindowTarget(e.SessionName())) },
			failMsg: "failed to read back pane-border-status",
			healMsg: "reed: pane-border-status reads back again",
		},
		{
			name:    "unzoom",
			setup:   scriptZoomedWindow,
			fail:    failVerb("resize-pane"),
			heal:    healVerb("resize-pane", ""),
			call:    keepZoomStep,
			failMsg: "failed to unzoom the strands' window",
			healMsg: "reed: unzooms the strands' window again",
		},
		{
			name:    "post-step listing",
			setup:   scriptZoomedWindow,
			fail:    failVerb("list-panes"),
			heal:    healVerb("list-panes", zoomListing),
			call:    keepZoomStep,
			failMsg: "failed to list the strands' window after a step",
			healMsg: "reed: lists the strands' window after a step again",
		},
		{
			name:  "re-zoom",
			setup: scriptZoomedWindow,
			fail: func(_ *Engine, f *fakeTmux, _ int) {
				// Each call unzooms with the odd resize-pane and re-zooms with the even one.
				resizes := 0
				f.answerFunc("resize-pane", func([]string) (string, error) {
					resizes++
					if resizes%2 == 0 {
						return "", errors.New("re-zoom failed")
					}
					return "", nil
				})
			},
			heal: func(_ *Engine, f *fakeTmux) {
				f.answerFunc("resize-pane", func([]string) (string, error) { return "", nil })
			},
			call:    keepZoomStep,
			failMsg: "failed to re-zoom the strands' window",
			healMsg: "reed: re-zooms the strands' window again",
		},
		{
			name:    "pane height pin",
			fail:    failVerb("resize-pane"),
			heal:    healVerb("resize-pane", ""),
			call:    func(e *Engine) { e.runResizePinsLocked([]render.Pin{{PaneID: "%2", Height: 10}}) },
			failMsg: "failed to pin a pane height",
			healMsg: "reed: pane height pin works again",
		},
		{
			name: "live pane generation read",
			fail: failVerb("display-message"),
			heal: func(e *Engine, f *fakeTmux) {
				answerGenerations(f, map[string]string{e.SessionName(): "$4|900|1787043918"})
			},
			call:    func(e *Engine) { _ = e.adoptPaneGenerationLocked(&ReedState{}) },
			failMsg: "could not read the live pane generation",
			healMsg: "reed: live pane generation reads again",
		},
		{
			name: "pane title repair",
			build: func(t *testing.T) (*Engine, *fakeTmux) {
				return newRepairTestEngine(t,
					[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
					[]LivePane{{ID: "%1", Title: "claude"}})
			},
			fail:              failVerb("select-pane"),
			heal:              healVerb("select-pane", ""),
			call:              func(e *Engine) { _, _ = e.repairNames(nil) },
			failMsg:           "could not repair pane title",
			healMsg:           "reed: pane title repair works again",
			wantInfoAfterHeal: "reed: repaired pane title",
		},
		{
			name: "name repair pass",
			build: func(t *testing.T) (*Engine, *fakeTmux) {
				return newRepairTestEngine(t,
					[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
					[]LivePane{{ID: "%1", Title: "tc:s:worker"}})
			},
			drive: func(t *testing.T, e *Engine, f *fakeTmux) {
				f.answer("has-session", "", errors.New("tmux unreachable"))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				afterCalls := 0
				timing := nameRepairTiming{
					Base:    time.Second,
					Ceiling: time.Second,
					After: func(time.Duration) <-chan time.Time {
						afterCalls++
						switch afterCalls {
						case failedCalls + 1:
							f.answer("has-session", "", nil)
						case failedCalls + 2:
							cancel()
							return nil
						}
						fired := make(chan time.Time, 1)
						fired <- time.Time{}
						return fired
					},
				}
				if err := e.watchNames(ctx, nil, timing); !errors.Is(err, context.Canceled) {
					t.Fatalf("watchNames = %v, want context.Canceled", err)
				}
			},
			failMsg: "name repair pass failed",
			healMsg: "reed: name repair pass works again",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			logger.SetVerbosity(2)
			var e *Engine
			var fake *fakeTmux
			if tt.build != nil {
				e, fake = tt.build(t)
			} else {
				e = newTestEngine(t)
				fake = installFakeTmux(t, e)
			}
			if tt.setup != nil {
				tt.setup(e, fake)
			}
			if tt.drive != nil {
				tt.drive(t, e, fake)
			} else {
				for call := 0; call < failedCalls; call++ {
					tt.fail(e, fake, call)
					tt.call(e)
				}
				tt.heal(e, fake)
				tt.call(e)
			}

			if got := countLogLines(logs.String(), "WARN", tt.failMsg); got != 1 {
				t.Errorf("failure WARN lines = %d, want exactly 1; log:\n%s", got, logs.String())
			}
			debugLines := 0
			for _, line := range strings.Split(logs.String(), "\n") {
				if !strings.Contains(line, "level=DEBUG") || !strings.Contains(line, tt.failMsg) {
					continue
				}
				debugLines++
				if !strings.Contains(line, "failed_ticks=") {
					t.Errorf("failure DEBUG line carries no failed_ticks: %s", line)
				}
			}
			if debugLines != failedCalls-1 {
				t.Errorf("failure DEBUG lines = %d, want %d; log:\n%s", debugLines, failedCalls-1, logs.String())
			}
			if got := countLogLines(logs.String(), "INFO", tt.healMsg); got != 1 {
				t.Errorf("recovery INFO lines = %d, want exactly 1; log:\n%s", got, logs.String())
			}
			if tt.wantInfoAfterHeal != "" {
				if got := countLogLines(logs.String(), "INFO", tt.wantInfoAfterHeal); got != 1 {
					t.Errorf("%q INFO lines = %d, want exactly 1; log:\n%s", tt.wantInfoAfterHeal, got, logs.String())
				}
			}
		})
	}
}

// countLogLines counts the captured lines at level that carry msg.
func countLogLines(output, level, msg string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "level="+level) && strings.Contains(line, msg) {
			count++
		}
	}
	return count
}
