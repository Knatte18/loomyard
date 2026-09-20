package loomcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Compile-time assertions that the two production adapters, and shuttleengine.Run itself, satisfy
// their interfaces.
var (
	_ driverStarter   = runnerDriverStarter{}
	_ driverPaneProbe = reedDriverPaneProbe{}
	_ driverHandle    = (*shuttleengine.Run)(nil)
)

// TestReedDriverPaneProbe_RemoveDriverStrand_PassesFalseCascade asserts the reed adapter's remove
// passes false for the cascade argument: a driver strand is parentless and childless, so the
// recursive form is never the right one.
func TestReedDriverPaneProbe_RemoveDriverStrand_PassesFalseCascade(t *testing.T) {
	var gotRecursive bool
	probe := reedDriverPaneProbe{
		remove: func(guid string, recursive bool) (reedengine.Removed, error) {
			gotRecursive = recursive
			return reedengine.Removed{}, nil
		},
	}

	if err := probe.RemoveDriverStrand("g0"); err != nil {
		t.Fatalf("RemoveDriverStrand() error = %v; want nil", err)
	}
	if gotRecursive {
		t.Error("RemoveDriverStrand() called remove with recursive = true; want false -- the cascade is inert for a parentless, childless driver strand")
	}
}

// TestReedDriverPaneProbe_Strands_ReturnsStatusStrands asserts Strands delegates to the engine's own
// Status and returns its strand slice.
func TestReedDriverPaneProbe_Strands_ReturnsStatusStrands(t *testing.T) {
	want := []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, Live: true}}
	probe := reedDriverPaneProbe{
		status: func() (reedengine.StatusResult, error) {
			return reedengine.StatusResult{Strands: want}, nil
		},
	}

	got, err := probe.Strands()
	if err != nil {
		t.Fatalf("Strands() error = %v; want nil", err)
	}
	if len(got) != 1 || got[0].GUID != "g0" {
		t.Errorf("Strands() = %+v; want %+v", got, want)
	}
}

// TestAwaitDriverPane_AlreadyDead is the refusing case, and the whole point of this probe: a strand
// whose pane is already dead must report not-ready within the budget. A test covering only the live
// case would pass against no probe at all.
func TestAwaitDriverPane_AlreadyDead(t *testing.T) {
	strands := func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "g0", Live: false}}, nil
	}
	waits := 0

	ready, err := awaitDriverPane(strands, "g0", countingWait(&waits), 5)
	if err != nil {
		t.Fatalf("awaitDriverPane() unexpected error: %v", err)
	}
	if ready {
		t.Error("awaitDriverPane() = true; want false -- a dead pane must never report ready")
	}
	if waits != 5 {
		t.Errorf("awaitDriverPane() waited %d times; want 5 (the attempt budget)", waits)
	}
}

// TestAwaitDriverPane_LiveOnFirstPoll asserts a strand that is live on the first poll reports ready
// immediately.
func TestAwaitDriverPane_LiveOnFirstPoll(t *testing.T) {
	strands := func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "g0", Live: true}}, nil
	}
	waits := 0

	ready, err := awaitDriverPane(strands, "g0", countingWait(&waits), 5)
	if err != nil {
		t.Fatalf("awaitDriverPane() unexpected error: %v", err)
	}
	if !ready {
		t.Error("awaitDriverPane() = false; want true -- the strand is live on the first poll")
	}
	if waits != 0 {
		t.Errorf("awaitDriverPane() waited %d times; want 0 -- readiness is observed on the first poll", waits)
	}
}

// TestAwaitDriverPane_AbsentStrand_TreatedAsNotReady asserts a strand absent from the slice entirely
// is treated as not-ready rather than as an error.
func TestAwaitDriverPane_AbsentStrand_TreatedAsNotReady(t *testing.T) {
	strands := func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "other", Live: true}}, nil
	}
	waits := 0

	ready, err := awaitDriverPane(strands, "g0", countingWait(&waits), 3)
	if err != nil {
		t.Fatalf("awaitDriverPane() unexpected error: %v", err)
	}
	if ready {
		t.Error("awaitDriverPane() = true; want false -- the addressed guid is absent from the slice")
	}
	if waits != 3 {
		t.Errorf("awaitDriverPane() waited %d times; want 3", waits)
	}
}

// TestAwaitDriverPane_SeamErrors asserts the seam erroring propagates that error.
func TestAwaitDriverPane_SeamErrors(t *testing.T) {
	wantErr := errors.New("boom")
	strands := func() ([]reedengine.StrandStatus, error) {
		return nil, wantErr
	}
	waits := 0

	_, err := awaitDriverPane(strands, "g0", countingWait(&waits), 5)
	if !errors.Is(err, wantErr) {
		t.Errorf("awaitDriverPane() error = %v; want %v", err, wantErr)
	}
	if waits != 0 {
		t.Errorf("awaitDriverPane() waited %d times on immediate seam error; want 0", waits)
	}
}

// TestAwaitDriverPane_AttemptBudgetCountedNotTimed asserts the attempt budget is respected by
// counting calls rather than by measuring elapsed time.
func TestAwaitDriverPane_AttemptBudgetCountedNotTimed(t *testing.T) {
	calls := 0
	strands := func() ([]reedengine.StrandStatus, error) {
		calls++
		return []reedengine.StrandStatus{{GUID: "g0", Live: false}}, nil
	}
	waits := 0

	_, err := awaitDriverPane(strands, "g0", countingWait(&waits), 7)
	if err != nil {
		t.Fatalf("awaitDriverPane() unexpected error: %v", err)
	}
	if calls != 7 {
		t.Errorf("awaitDriverPane() called strands() %d times; want 7 (the attempt budget), independent of any elapsed time", calls)
	}
}
