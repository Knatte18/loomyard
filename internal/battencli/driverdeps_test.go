//go:build tier0

// driverdeps_test.go covers the seams batten fills the shed driver with: which are wired, which are left to remove their rule-table rows, and the paths and lock they name.

package battencli

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestBattenDriverDeps asserts a batten run's driver is armed with exactly the seams its rule table needs and no more.
// Every required seam is checked because a nil PauseHold panics only at the first re-step or tackler spawn, which no run test reaches.
//
//testtiming:keep pins the wiring of the seams and paths without running a driver; the run tests step through the seams only along the paths they take
func TestBattenDriverDeps(t *testing.T) {
	t.Parallel()

	c := newFakeReceiver(t, nil)
	deps, err := c.driverDepsFor()
	if err != nil {
		t.Fatalf("driverDepsFor() error = %v", err)
	}

	unwired := map[string]bool{
		"Watched":           deps.Watched == nil,
		"HandOver":          deps.HandOver == nil,
		"Tamper":            deps.Tamper == nil,
		"CommitRecords":     deps.CommitRecords == nil,
		"WriteFrictionNote": deps.WriteFrictionNote == nil,
	}
	for seam, isNil := range unwired {
		if !isNil {
			t.Errorf("driverDepsFor() wires %s; want it nil, so its rows leave the rule table", seam)
		}
	}

	required := map[string]bool{
		"Step":                  deps.Step != nil,
		"Goto":                  deps.Goto != nil,
		"ReadStatus":            deps.ReadStatus != nil,
		"RunLockHeld":           deps.RunLockHeld != nil,
		"SaveEnvelope":          deps.SaveEnvelope != nil,
		"NewReportPath":         deps.NewReportPath != nil,
		"Pause":                 deps.Pause != nil,
		"PauseHold":             deps.PauseHold != nil,
		"Tackler":               deps.Tackler != nil,
		"ConsumeTackler":        deps.ConsumeTackler != nil,
		"OpenAttempts":          deps.OpenAttempts != nil,
		"Arbiter":               deps.Arbiter != nil,
		"Now":                   deps.Now != nil,
		"HandBack.Notify":       deps.HandBack.Notify != nil,
		"HandBack.OrchRecorded": deps.HandBack.OrchRecorded != nil,
		"HandBack.Park":         deps.HandBack.Park != nil,
		"HandBack.Now":          deps.HandBack.Now != nil,
	}
	for seam, isSet := range required {
		if !isSet {
			t.Errorf("driverDepsFor() leaves %s nil; the driver calls it", seam)
		}
	}

	if pending, err := deps.Arbiter.EscalationPending(battenrecipe.NameRunShed); pending || err != nil {
		t.Errorf("Arbiter.EscalationPending() = (%v, %v); want (false, nil), since a batten stop never awaits a review escalation", pending, err)
	}
	key := sheddriver.StopKey{CurrentProducer: battenrecipe.NameLand, HistoryLength: 3, State: string(shedengine.StateBlocked)}
	arbiterDir := shedrun.ArbiterAttemptDir(c.location, c.slug, 1)
	if err := loomengine.WriteArbiterAttempt(arbiterDir, loomengine.ArbiterAttempt{Number: 1, StopKey: key}); err != nil {
		t.Fatalf("WriteArbiterAttempt: %v", err)
	}
	if _, arbiter, _, err := deps.OpenAttempts(key); arbiter != arbiterDir || err != nil {
		t.Errorf("OpenAttempts(%+v) arbiter = %q, %v; want %q", key, arbiter, err, arbiterDir)
	}
	if _, arbiter, _, err := deps.OpenAttempts(sheddriver.StopKey{CurrentProducer: battenrecipe.NameLand, HistoryLength: 4, State: string(shedengine.StateBlocked)}); arbiter != "" || err != nil {
		t.Errorf("OpenAttempts(another stop) arbiter = %q, %v; want none", arbiter, err)
	}

	if want := shedrun.SettleRecords(c.location, c.slug); deps.SettlePath != want {
		t.Errorf("SettlePath = %q; want %q", deps.SettlePath, want)
	}
	if want := shedrun.DriveReportsDir(c.location, c.slug); filepath.Dir(deps.NewReportPath()) != want {
		t.Errorf("NewReportPath() directory = %q; want %q", filepath.Dir(deps.NewReportPath()), want)
	}

	tackler := c.tacklerDepsFor(sheddriver.TacklerSettings{}, nil)
	if tackler.Recipe != battenTacklerRecipe || tackler.GotoRoutedFrom {
		t.Errorf("tackler Recipe = %q, GotoRoutedFrom = %v; want %q and false", tackler.Recipe, tackler.GotoRoutedFrom, battenTacklerRecipe)
	}
	if want := shedrun.TacklerDir(c.location, c.slug); tackler.TacklerDir != want {
		t.Errorf("tackler TacklerDir = %q; want %q", tackler.TacklerDir, want)
	}
	if want := shedrun.TacklerAttemptDir(c.location, c.slug, 2); tackler.AttemptDir(2) != want {
		t.Errorf("tackler AttemptDir(2) = %q; want %q", tackler.AttemptDir(2), want)
	}
	if want := shedrun.TacklerMarker(c.location, c.slug); tackler.MarkerPath != want {
		t.Errorf("tackler MarkerPath = %q; want %q", tackler.MarkerPath, want)
	}

	driveLock := battenDriveLock(c.location, c.slug)
	if want := shedrun.DriverLock(c.location, c.slug); driveLock.Path != want {
		t.Errorf("drive lock Path = %q; want %q", driveLock.Path, want)
	}
	if want := []string{shedrun.ParkMarker(c.location, c.slug), shedrun.ResumeReport(c.location, c.slug)}; !slices.Equal(driveLock.Stale, want) {
		t.Errorf("drive lock Stale = %v; want %v", driveLock.Stale, want)
	}
	if !strings.Contains(driveLock.Refusal, battenDriveRefusal) || !strings.Contains(driveLock.Refusal, "lym batten status "+c.slug) {
		t.Errorf("drive lock Refusal = %q; want it to carry %q and name `lym batten status %s`", driveLock.Refusal, battenDriveRefusal, c.slug)
	}

	sent, err := deps.HandBack.Notify(sheddriver.Stop{State: "paused", CurrentProducer: "Run-Shed", HistoryLength: 2}, "the run is paused")
	if sent || err != nil {
		t.Errorf("Notify(paused stop) = (%v, %v); want (false, nil), since a pause is the operator's own act", sent, err)
	}
	markers, err := filepath.Glob(filepath.Join(c.env.ScratchDir, "*row-notice"))
	if err != nil || len(markers) > 0 {
		t.Errorf("Notify(paused stop) left row notice markers %v (glob error %v); want none", markers, err)
	}

	// The goto and step seams label the run shed: the goto on the entry it appends, the step as the label pending before the loop is spawned.
	if err := c.loadRouting(); err != nil {
		t.Fatalf("loadRouting: %v", err)
	}
	c.untilStop = nil
	first, last := c.routing.Producers[0].Name, c.routing.Producers[len(c.routing.Producers)-1].Name
	c.spec = &shedverbs.Spec{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		Routing:        c.routing,
		Loop: shedverbs.LoopSpec{
			LockPath:     filepath.Join(filepath.Dir(c.shedPaths.StatusPath), "loop.lock"),
			PIDPath:      filepath.Join(filepath.Dir(c.shedPaths.StatusPath), "loop.pid"),
			EnvelopePath: filepath.Join(filepath.Dir(c.shedPaths.StatusPath), "loop-envelope.json"),
			Spawn:        func([]string) (<-chan struct{}, error) { return nil, errors.New("no loop in a test") },
		},
	}
	seedBlocked := func() {
		t.Helper()
		blocked := shedengine.Status{CurrentProducer: last, State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{}}
		if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, blocked); err != nil {
			t.Fatalf("seed blocked status: %v", err)
		}
	}
	readStatus := func() shedengine.Status {
		t.Helper()
		st, _, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
		if err != nil {
			t.Fatalf("read status: %v", err)
		}
		return st
	}
	atLand := shedengine.Status{CurrentProducer: battenrecipe.NameLand, State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{}}
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, atLand); err != nil {
		t.Fatalf("seed status at Land: %v", err)
	}
	guard := c.haltedRowGuard()
	wantRefusal := "goto target " + battenrecipe.NameSeedChild + " is not the halted row " + battenrecipe.NameLand + "; batten's arbiter resumes only the halted row; way forward: the run's parent runs lym batten goto " + c.slug + " --to " + battenrecipe.NameSeedChild
	if refused, err := guard(battenrecipe.NameSeedChild); refused != wantRefusal || err != nil {
		t.Errorf("GotoGuard(%s) = %q, %v; want %q", battenrecipe.NameSeedChild, refused, err, wantRefusal)
	}
	if refused, err := guard(battenrecipe.NameLand); refused != "" || err != nil {
		t.Errorf("GotoGuard(%s) = %q, %v; want the halted row admitted", battenrecipe.NameLand, refused, err)
	}

	seedBlocked()
	if err := deps.Goto(t.Context(), first); err != nil {
		t.Fatalf("deps.Goto(%s) = %v; want nil", first, err)
	}
	if st := readStatus(); len(st.History) != 1 || st.History[0].By != shedengine.CallerShed {
		t.Errorf("History = %+v; want one goto entry by %q", st.History, shedengine.CallerShed)
	}
	seedBlocked()
	if _, err := deps.Step(t.Context(), ""); err != nil {
		t.Fatalf("deps.Step = _, %v; want the envelope the refused spawn printed", err)
	}
	if st := readStatus(); st.PendingBy != shedengine.CallerShed {
		t.Errorf("PendingBy = %q; want %q", st.PendingBy, shedengine.CallerShed)
	}
}

// TestBattenConsumeAttempt asserts the consume hook refuses an attempt that is no positive number, naming the hand-back to the run's parent, before it reads any attempt directory.
func TestBattenConsumeAttempt(t *testing.T) {
	t.Parallel()

	c := &battenCLI{slug: "a-slug"}
	for _, attempt := range []string{"abc", "0"} {
		t.Run(attempt, func(t *testing.T) {
			t.Parallel()

			err := c.battenConsumeAttempt(t.Context(), attempt)
			if err == nil {
				t.Fatalf("battenConsumeAttempt(%q) = nil; want a refusal", attempt)
			}
			if want := "is not a positive integer; " + arbiterstop.BattenHandBackWayForward; !strings.Contains(err.Error(), want) {
				t.Errorf("battenConsumeAttempt(%q) = %q; want it to carry %q", attempt, err, want)
			}
		})
	}
}

// TestBattenHandBackNotice asserts the hand-back router posts the row-stop line for a failed Land or Worktree-Teardown stop, nothing for a paused stop or a Land stop that honoured a pause, and the hand-back's own line for a stop at Run-Shed.
func TestBattenHandBackNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stop     sheddriver.Stop
		wantSent bool
		wantLine string
	}{
		{
			name:     "AFailedLandStopPostsTheRowStopLine",
			stop:     sheddriver.Stop{State: "blocked", CurrentProducer: battenrecipe.NameLand, HistoryLength: 3, Reason: "pull failed: boom"},
			wantSent: true,
			wantLine: "Land failed: pull failed: boom; report /reports/stop.md; way forward: fix the cause, then run \"lym batten run a-slug\"",
		},
		{
			name:     "AFailedTeardownStopPostsTheRowStopLine",
			stop:     sheddriver.Stop{State: "blocked", CurrentProducer: battenrecipe.NameWorktreeTeardown, HistoryLength: 4, Reason: "session shutdown failed: boom"},
			wantSent: true,
			wantLine: "Worktree-Teardown failed: session shutdown failed: boom; report /reports/stop.md; way forward: fix the cause, then run \"lym batten run a-slug\"",
		},
		{
			name: "APauseHonouredLandStopPostsNothing",
			stop: sheddriver.Stop{State: "blocked", CurrentProducer: battenrecipe.NameLand, HistoryLength: 3, Reason: battenshed.PauseHonouredPrefix + ": a pause was requested"},
		},
		{
			name: "APausedStopPostsNothing",
			stop: sheddriver.Stop{State: "paused", CurrentProducer: battenrecipe.NameLand, HistoryLength: 3},
		},
		{
			name:     "AStopAtRunShedPostsTheHandBackLine",
			stop:     sheddriver.Stop{State: "blocked", CurrentProducer: battenrecipe.NameRunShed, HistoryLength: 2, Reason: "x"},
			wantSent: true,
			wantLine: "the hand-back line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var lines []string
			deps := battenshed.RowNoticeDeps{
				Notify: func(_ context.Context, line string) (bool, error) {
					lines = append(lines, line)
					return true, nil
				},
			}
			sent := battenHandBackNotice(deps, "a-slug", t.TempDir(), tt.stop, "the hand-back line", "/reports/stop.md")
			if sent != tt.wantSent {
				t.Errorf("battenHandBackNotice() = %v; want %v", sent, tt.wantSent)
			}
			if !tt.wantSent {
				if len(lines) != 0 {
					t.Errorf("notices = %q; want none", lines)
				}
				return
			}
			if len(lines) != 1 || !strings.HasSuffix(lines[0], tt.wantLine) {
				t.Errorf("notices = %q; want one ending in %q", lines, tt.wantLine)
			}
		})
	}
}
