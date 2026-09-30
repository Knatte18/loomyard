package loomcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestObserveEntry_GotoIsNotACrashResume pins that a run moved by shedengine.Goto reads as an
// ordinary resume: goto writes paused, never running, so with the run lock free the entry
// observation carries no crash-resume signature.
func TestObserveEntry_GotoIsNotACrashResume(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	runLockPath := filepath.Join(dir, "run.lock")
	markerPath := filepath.Join(dir, "step-handoff.json")
	markerLockPath := filepath.Join(dir, "step-handoff.json.lock")

	writeSelfreportStatus(t, statusPath, statusLockPath, shedengine.Status{
		CurrentProducer: "Loom-Preflight",
		State:           shedengine.StateBlocked,
		History: []shedengine.HistoryEntry{
			{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-01-01T00:00:00Z"},
			{Producer: "Discussion-Write", Outcome: shedengine.Done, At: "2026-01-01T00:00:01Z"},
			{Producer: "Loom-Preflight", Outcome: shedengine.Stuck, At: "2026-01-01T00:00:02Z"},
		},
		Product: productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"}),
	})

	if _, err := shedengine.Goto(shedengine.GotoRequest{
		StatusPath:     statusPath,
		LockPath:       runLockPath,
		StatusLockPath: statusLockPath,
		Producers: []shedengine.ProducerDef{
			{Name: "Loom-Preflight"},
			{Name: "Discussion-Write"},
		},
		Target: "Discussion-Write",
	}); err != nil {
		t.Fatalf("shedengine.Goto() error = %v", err)
	}

	entry := observeEntry(true, runLockPath, statusPath, statusLockPath, markerPath, markerLockPath)
	if !entry.Observed {
		t.Fatalf("observeEntry: Observed = false; want true")
	}
	if entry.RunLockHeld {
		t.Fatalf("observeEntry: RunLockHeld = true; want the run lock free")
	}
	if entry.State != shedengine.StatePaused {
		t.Errorf("observeEntry: State = %q; want %q", entry.State, shedengine.StatePaused)
	}
	if anomaly, ok := loomengine.DetectCrashResume(entry); ok {
		t.Errorf("DetectCrashResume() = %+v, true; want no anomaly after goto", anomaly)
	}
}
