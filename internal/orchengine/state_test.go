package orchengine

import (
	"os"
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

//testtiming:keep pins the full-field save and load round trip and the idle state of an absent file, which its covering test does not assert
func TestSaveLoadState_RoundTrip(t *testing.T) {
	p := testPaths(t)
	absent, err := LoadState(p)
	if err != nil {
		t.Fatalf("LoadState of an absent file: %v", err)
	}
	if absent.Phase != PhaseIdle {
		t.Errorf("absent Phase = %q, want idle", absent.Phase)
	}
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
		CycleMode:           CycleCompact,
		CycleRequestedAt:    time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
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
	if !got.CycleRequestedAt.Equal(want.CycleRequestedAt) {
		t.Errorf("CycleRequestedAt = %v, want %v", got.CycleRequestedAt, want.CycleRequestedAt)
	}
	got.PhaseEnteredAt, got.LastDeferral, got.CycleRequestedAt = want.PhaseEnteredAt, want.LastDeferral, want.CycleRequestedAt
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

//testtiming:keep pins the write, probe and clear round trip of the cycle request marker, which its covering test does not assert
func TestCycleRequest_WriteProbeClear(t *testing.T) {
	p := testPaths(t)
	if err := ClearCycleRequest(p); err != nil {
		t.Fatalf("clear absent: %v", err)
	}
	if _, ok, err := CycleRequested(p); err != nil || ok {
		t.Fatalf("CycleRequested before = %v, %v", ok, err)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := RequestCycle(p, CycleCompact, at); err != nil {
		t.Fatalf("RequestCycle: %v", err)
	}
	req, ok, err := CycleRequested(p)
	if err != nil || !ok {
		t.Fatalf("CycleRequested after = %v, %v", ok, err)
	}
	if req.Mode != CycleCompact || !req.RequestedAt.Equal(at) {
		t.Errorf("request = %+v, want mode %q at %v", req, CycleCompact, at)
	}
	if err := ClearCycleRequest(p); err != nil {
		t.Fatalf("ClearCycleRequest: %v", err)
	}
	if _, ok, _ := CycleRequested(p); ok {
		t.Error("request still present after clear")
	}
}

func TestCycleRequested_UnparseableMarkerIsPendingWithZeroRequest(t *testing.T) {
	for name, content := range map[string]string{"legacy": "cycle\n", "garbage": "{", "unknown mode": `{"mode":"nap","requested_at":"2026-01-02T03:04:05Z"}`} {
		t.Run(name, func(t *testing.T) {
			p := testPaths(t)
			if err := os.MkdirAll(p.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p.CycleRequestPath, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			req, ok, err := CycleRequested(p)
			if err != nil || !ok || req != (CycleRequest{}) {
				t.Errorf("CycleRequested = %+v, %v, %v; want a pending zero request", req, ok, err)
			}
		})
	}
}

//testtiming:keep pins that handoff paths are distinct and sit directly under the handoffs directory, which its covering test does not assert
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
	launched := time.Date(2026, 1, 2, 3, 6, 7, 0, time.UTC)
	t.Run("idle", func(t *testing.T) {
		in := State{Phase: PhaseIdle, LastHandoff: "h", PhaseEventsOffset: 5, LastInjectionOffset: 6, CycleCount: 2, LastContextTokens: 9, LastContextKnown: true, ReloadStep: 2, ReloadTypedAt: deferred}
		got := ResetForFreshLaunch(in, "g2", launched)
		if !got.CompactionBaseline.Equal(launched) {
			t.Errorf("CompactionBaseline = %v, want the launch time %v", got.CompactionBaseline, launched)
		}
		if got.ReloadStep != 0 || !got.ReloadTypedAt.IsZero() {
			t.Errorf("reload step = %d at %v, want cleared", got.ReloadStep, got.ReloadTypedAt)
		}
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
		got := ResetForFreshLaunch(in, "g2", launched)
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
