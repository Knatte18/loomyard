package orchengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orch")
	return Paths{
		Dir:              dir,
		StatePath:        filepath.Join(dir, "state.json"),
		StateLockPath:    filepath.Join(dir, "state.json.lock"),
		WatchLockPath:    filepath.Join(dir, "watch.lock"),
		StartLockPath:    filepath.Join(dir, "start.lock"),
		CycleRequestPath: filepath.Join(dir, "cycle-request"),
		HandoffsDir:      filepath.Join(dir, "handoffs"),
		WatchLogPath:     filepath.Join(dir, "watch.log"),
		RolePath:         filepath.Join(dir, "role.md"),
		NoteTemplatePath: filepath.Join(dir, "note-template.md"),
	}
}

func TestLoadState_AbsentIsIdle(t *testing.T) {
	s, err := LoadState(testPaths(t))
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Phase != PhaseIdle {
		t.Errorf("Phase = %q, want idle", s.Phase)
	}
}

func TestSaveLoadState_RoundTrip(t *testing.T) {
	p := testPaths(t)
	want := State{
		Strand:              "g1",
		Phase:               PhaseResuming,
		PhaseStrand:         "g1",
		PhaseEnteredAt:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		PhaseEventsOffset:   11,
		PhaseInjected:       true,
		PendingHandoff:      "h.md",
		PendingResume:       "resume",
		LastHandoff:         "prev.md",
		LastInjectionOffset: 22,
		LastContextTokens:   333,
		LastContextKnown:    true,
		CycleCount:          4,
		LastAbortReason:     "why",
		WatcherExit:         "bye",
		CycleTrigger:        TriggerSoft,
		LastDeferral:        time.Date(2026, 1, 2, 3, 5, 6, 0, time.UTC),
	}
	if err := SaveState(p, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(p)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !got.PhaseEnteredAt.Equal(want.PhaseEnteredAt) {
		t.Errorf("PhaseEnteredAt = %v, want %v", got.PhaseEnteredAt, want.PhaseEnteredAt)
	}
	if !got.LastDeferral.Equal(want.LastDeferral) {
		t.Errorf("LastDeferral = %v, want %v", got.LastDeferral, want.LastDeferral)
	}
	got.PhaseEnteredAt, got.LastDeferral = want.PhaseEnteredAt, want.LastDeferral
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestCycleRequest_WriteProbeClear(t *testing.T) {
	p := testPaths(t)
	if err := ClearCycleRequest(p); err != nil {
		t.Fatalf("clear absent: %v", err)
	}
	if ok, err := CycleRequested(p); err != nil || ok {
		t.Fatalf("CycleRequested before = %v, %v", ok, err)
	}
	if err := RequestCycle(p); err != nil {
		t.Fatalf("RequestCycle: %v", err)
	}
	if ok, err := CycleRequested(p); err != nil || !ok {
		t.Fatalf("CycleRequested after = %v, %v", ok, err)
	}
	if err := ClearCycleRequest(p); err != nil {
		t.Fatalf("ClearCycleRequest: %v", err)
	}
	if ok, _ := CycleRequested(p); ok {
		t.Error("request still present after clear")
	}
}

func TestNewHandoffPath_DistinctUnderHandoffsDir(t *testing.T) {
	p := testPaths(t)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a := NewHandoffPath(p, now)
	b := NewHandoffPath(p, now.Add(time.Second))
	if a == b {
		t.Errorf("paths equal: %s", a)
	}
	for _, x := range []string{a, b} {
		if filepath.Dir(x) != p.HandoffsDir {
			t.Errorf("%s not directly under %s", x, p.HandoffsDir)
		}
	}
}

func TestResetForFreshLaunch(t *testing.T) {
	deferred := time.Date(2026, 1, 2, 3, 5, 6, 0, time.UTC)
	t.Run("idle", func(t *testing.T) {
		in := State{Phase: PhaseIdle, LastHandoff: "h", PhaseEventsOffset: 5, LastInjectionOffset: 6, CycleCount: 2, LastContextTokens: 9, LastContextKnown: true}
		got := ResetForFreshLaunch(in, "g2")
		if got.LastAbortReason != "" {
			t.Errorf("LastAbortReason = %q, want empty", got.LastAbortReason)
		}
		if got.Strand != "g2" || got.Phase != PhaseIdle {
			t.Errorf("Strand/Phase = %q/%q", got.Strand, got.Phase)
		}
		if got.PhaseEventsOffset != 0 || got.LastInjectionOffset != 0 {
			t.Errorf("offsets not zeroed: %+v", got)
		}
		if got.LastHandoff != "h" || got.CycleCount != 2 {
			t.Errorf("survivors lost: %+v", got)
		}
		if got.LastContextTokens != 0 || got.LastContextKnown {
			t.Errorf("reading = %d known=%v, want unknown", got.LastContextTokens, got.LastContextKnown)
		}
	})
	t.Run("non-idle", func(t *testing.T) {
		in := State{CycleTrigger: TriggerSoft, LastDeferral: deferred, Phase: PhaseClearing, PhaseInjected: true, PendingHandoff: "p", PendingResume: "r", WatcherExit: "x", LastHandoff: "h", PhaseEventsOffset: 5, LastInjectionOffset: 6}
		got := ResetForFreshLaunch(in, "g2")
		if !strings.Contains(got.LastAbortReason, string(PhaseClearing)) {
			t.Errorf("LastAbortReason = %q, want it to name clearing", got.LastAbortReason)
		}
		if got.Phase != PhaseIdle || got.PhaseInjected || got.PendingHandoff != "" || got.PendingResume != "" || got.WatcherExit != "" {
			t.Errorf("not cleared: %+v", got)
		}
		if got.PhaseEventsOffset != 0 || got.LastInjectionOffset != 0 {
			t.Errorf("offsets not zeroed: %+v", got)
		}
		if got.LastHandoff != "h" {
			t.Errorf("LastHandoff = %q", got.LastHandoff)
		}
		if got.CycleTrigger != TriggerSoft || !got.LastDeferral.Equal(deferred) {
			t.Errorf("CycleTrigger/LastDeferral = %q/%v, want them kept", got.CycleTrigger, got.LastDeferral)
		}
	})
}
