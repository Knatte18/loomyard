// driverdeps_test.go covers the seams batten fills the shed driver with: which are wired, which are left to remove their rule-table rows, and the paths and lock they name.

package battencli

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
		"Arbiter":           deps.Arbiter == nil,
		"Verify":            deps.Verify == nil,
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
	if !strings.Contains(driveLock.Refusal, battenDriveRefusal) || !strings.Contains(driveLock.Refusal, "lyx batten status "+c.slug) {
		t.Errorf("drive lock Refusal = %q; want it to carry %q and name `lyx batten status %s`", driveLock.Refusal, battenDriveRefusal, c.slug)
	}

	sent, err := deps.HandBack.Notify(sheddriver.Stop{State: "paused", CurrentProducer: "Run-Shed", HistoryLength: 2}, "the run is paused")
	if sent || err != nil {
		t.Errorf("Notify(paused stop) = (%v, %v); want (false, nil), since a pause is the operator's own act", sent, err)
	}
	markers, err := filepath.Glob(filepath.Join(c.env.ScratchDir, "*row-notice"))
	if err != nil || len(markers) > 0 {
		t.Errorf("Notify(paused stop) left row notice markers %v (glob error %v); want none", markers, err)
	}

	if got, want := childGotoArgv("Merge-In"), []string{"loom", "goto", "--to", "Merge-In", "--by", "shed"}; !slices.Equal(got, want) {
		t.Errorf("childGotoArgv(Merge-In) = %v; want %v", got, want)
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
		st, _, err := state.ReadJSONNoMkdir[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
		if err != nil {
			t.Fatalf("read status: %v", err)
		}
		return st
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
