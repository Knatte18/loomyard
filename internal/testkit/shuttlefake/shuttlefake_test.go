package shuttlefake

import (
	"errors"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestEngine_InertDefaults(t *testing.T) {
	e := &Engine{}

	launch, err := e.Prepare("run", shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{})
	if err != nil || launch.Cmd == "" || launch.SessionID == "" {
		t.Fatalf("Prepare = %+v, %v; want a canned launch and no error", launch, err)
	}
	if events, err := e.ParseEvents([]byte("x")); events != nil || err != nil {
		t.Errorf("ParseEvents = %v, %v; want nil, nil", events, err)
	}
	if got := e.Startup("x"); got != shuttleengine.StartupReady {
		t.Errorf("Startup = %v, want StartupReady", got)
	}
	if e.InterruptSequence() != nil || e.TrustDismissSequence("x") != nil || e.ComposeSend("x") != nil || e.ModelSwitchSequence("x") != nil {
		t.Error("key sequences must default to no inputs")
	}
	if audit, err := e.AuditForks("s", "w"); err != nil || audit.SpawnCalls != 0 {
		t.Errorf("AuditForks = %+v, %v; want zero, nil", audit, err)
	}
	if audit, err := e.AuditForksIncremental("s", "w", nil); err != nil || audit.SpawnCalls != 0 {
		t.Errorf("AuditForksIncremental = %+v, %v; want zero, nil", audit, err)
	}
}

func TestEngine_PrepareRecordsCalls(t *testing.T) {
	e := &Engine{}
	if _, err := e.Prepare("run", shuttleengine.Spec{Prompt: "first"}, shuttleengine.Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prepare("run", shuttleengine.Spec{Prompt: "second"}, shuttleengine.Config{}); err != nil {
		t.Fatal(err)
	}
	if e.PrepareCalls != 2 || e.LastPrompt != "second" || e.LastSpec.Prompt != "second" {
		t.Errorf("PrepareCalls=%d LastPrompt=%q LastSpec.Prompt=%q; want 2, second, second", e.PrepareCalls, e.LastPrompt, e.LastSpec.Prompt)
	}
}

func TestEngine_Overrides(t *testing.T) {
	boom := errors.New("boom")
	scripted := &shuttleengine.Launch{Cmd: "c", SessionID: "s"}

	t.Run("PrepareLaunch", func(t *testing.T) {
		e := &Engine{PrepareLaunch: scripted}
		if got, _ := e.Prepare("r", shuttleengine.Spec{}, shuttleengine.Config{}); got != *scripted {
			t.Errorf("Prepare = %+v, want %+v", got, *scripted)
		}
	})
	t.Run("PrepareErr", func(t *testing.T) {
		e := &Engine{PrepareErr: boom}
		if _, err := e.Prepare("r", shuttleengine.Spec{}, shuttleengine.Config{}); !errors.Is(err, boom) {
			t.Errorf("Prepare err = %v, want boom", err)
		}
		if e.PrepareCalls != 1 {
			t.Errorf("PrepareCalls = %d, want 1: a failing Prepare is still recorded", e.PrepareCalls)
		}
	})
	t.Run("PrepareFn", func(t *testing.T) {
		e := &Engine{PrepareErr: boom, PrepareFn: func(runDir string, _ shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
			return shuttleengine.Launch{Cmd: runDir}, nil
		}}
		if got, err := e.Prepare("dir", shuttleengine.Spec{}, shuttleengine.Config{}); err != nil || got.Cmd != "dir" {
			t.Errorf("Prepare = %+v, %v; want Cmd dir", got, err)
		}
	})
	t.Run("Events", func(t *testing.T) {
		e := &Engine{Events: []shuttleengine.Event{{Message: "m"}}}
		if got, _ := e.ParseEvents(nil); len(got) != 1 || got[0].Message != "m" {
			t.Errorf("ParseEvents = %v", got)
		}
		e.EventsErr = boom
		if _, err := e.ParseEvents(nil); !errors.Is(err, boom) {
			t.Errorf("ParseEvents err = %v, want boom", err)
		}
		e.ParseEventsFn = func([]byte) ([]shuttleengine.Event, error) { return nil, nil }
		if _, err := e.ParseEvents(nil); err != nil {
			t.Errorf("ParseEventsFn must win over EventsErr, got %v", err)
		}
	})
	t.Run("Startup", func(t *testing.T) {
		e := &Engine{StartupFn: func(string) shuttleengine.StartupState { return shuttleengine.StartupPending }}
		if got := e.Startup(""); got != shuttleengine.StartupPending {
			t.Errorf("Startup = %v, want StartupPending", got)
		}
	})
	t.Run("Sequences", func(t *testing.T) {
		in := []shuttleengine.PaneInput{{Key: "k"}}
		e := &Engine{
			InterruptSequenceFn:    func() []shuttleengine.PaneInput { return in },
			TrustDismissSequenceFn: func(c string) []shuttleengine.PaneInput { return []shuttleengine.PaneInput{{Text: c}} },
			ComposeSendFn:          func(s string) []shuttleengine.PaneInput { return []shuttleengine.PaneInput{{Text: s}} },
			ModelSwitchSequenceFn:  func(m string) []shuttleengine.PaneInput { return []shuttleengine.PaneInput{{Text: m}} },
		}
		if got := e.InterruptSequence(); len(got) != 1 || got[0].Key != "k" {
			t.Errorf("InterruptSequence = %v", got)
		}
		if got := e.TrustDismissSequence("cap"); got[0].Text != "cap" {
			t.Errorf("TrustDismissSequence = %v", got)
		}
		if got := e.ComposeSend("hi"); got[0].Text != "hi" {
			t.Errorf("ComposeSend = %v", got)
		}
		if got := e.ModelSwitchSequence("m"); got[0].Text != "m" {
			t.Errorf("ModelSwitchSequence = %v", got)
		}
	})
	t.Run("Audit", func(t *testing.T) {
		e := &Engine{Audit: shuttleengine.ForkAudit{SpawnCalls: 3}}
		if got, _ := e.AuditForks("s", "w"); got.SpawnCalls != 3 {
			t.Errorf("AuditForks = %+v", got)
		}
		if got, _ := e.AuditForksIncremental("s", "w", nil); got.SpawnCalls != 3 {
			t.Errorf("AuditForksIncremental = %+v", got)
		}
		e.AuditErr = boom
		if _, err := e.AuditForks("s", "w"); !errors.Is(err, boom) {
			t.Errorf("AuditForks err = %v, want boom", err)
		}
		e.AuditForksFn = func(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
			return shuttleengine.ForkAudit{NamedSpawns: len(sessionID + workdir)}, nil
		}
		e.AuditForksIncrementalFn = func(_, _ string, seen map[string]bool) (shuttleengine.ForkAudit, error) {
			return shuttleengine.ForkAudit{NamedSpawns: len(seen)}, nil
		}
		if got, err := e.AuditForks("ab", "c"); err != nil || got.NamedSpawns != 3 {
			t.Errorf("AuditForksFn = %+v, %v", got, err)
		}
		if got, err := e.AuditForksIncremental("", "", map[string]bool{"x": true}); err != nil || got.NamedSpawns != 1 {
			t.Errorf("AuditForksIncrementalFn = %+v, %v", got, err)
		}
	})
}

func TestReed_StrandLivenessAcrossAddAndRemove(t *testing.T) {
	r := &Reed{}

	a, err := r.AddStrand(reedengine.AddSpec{Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.AddStrand(reedengine.AddSpec{NameOverride: "named", Role: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if a.GUID == "" || a.GUID == b.GUID {
		t.Fatalf("GUIDs %q and %q must be distinct and non-empty", a.GUID, b.GUID)
	}
	if a.Name != "worker" || b.Name != "named" {
		t.Errorf("names = %q, %q; want worker, named", a.Name, b.Name)
	}

	status, err := r.Status()
	if err != nil || len(status.Strands) != 2 {
		t.Fatalf("Status = %+v, %v; want two strands", status, err)
	}
	for _, s := range status.Strands {
		if !s.Live {
			t.Errorf("strand %s must be live after add", s.GUID)
		}
	}

	removed, err := r.RemoveStrand(a.GUID, true)
	if err != nil || len(removed.Strands) != 1 || removed.Strands[0].GUID != a.GUID {
		t.Fatalf("RemoveStrand = %+v, %v", removed, err)
	}
	status, _ = r.Status()
	if len(status.Strands) != 1 || status.Strands[0].GUID != b.GUID {
		t.Errorf("Status after remove = %+v, want only %s", status, b.GUID)
	}
	if len(r.RemovedGUIDs) != 1 || r.RemovedGUIDs[0] != a.GUID {
		t.Errorf("RemovedGUIDs = %v, want [%s]", r.RemovedGUIDs, a.GUID)
	}
	if len(r.AddedSpecs) != 2 {
		t.Errorf("AddedSpecs = %d, want 2", len(r.AddedSpecs))
	}
}

func TestReed_StatusReturnsACopy(t *testing.T) {
	r := &Reed{}
	if _, err := r.AddStrand(reedengine.AddSpec{Role: "w"}); err != nil {
		t.Fatal(err)
	}
	status, _ := r.Status()
	status.Strands[0].Live = false
	again, _ := r.Status()
	if !again.Strands[0].Live {
		t.Error("mutating a returned Status must not change the table")
	}
}

func TestReed_Overrides(t *testing.T) {
	boom := errors.New("boom")

	t.Run("Errors", func(t *testing.T) {
		r := &Reed{AddErr: boom, StatusErr: boom, RemoveErr: boom}
		if _, err := r.AddStrand(reedengine.AddSpec{}); !errors.Is(err, boom) {
			t.Errorf("AddStrand err = %v", err)
		}
		if _, err := r.Status(); !errors.Is(err, boom) {
			t.Errorf("Status err = %v", err)
		}
		if _, err := r.RemoveStrand("g", false); !errors.Is(err, boom) {
			t.Errorf("RemoveStrand err = %v", err)
		}
		if len(r.RemovedGUIDs) != 0 {
			t.Errorf("a failed RemoveStrand must not record a retirement, got %v", r.RemovedGUIDs)
		}
	})
	t.Run("RemoveErr keeps the strand", func(t *testing.T) {
		r := &Reed{}
		s, _ := r.AddStrand(reedengine.AddSpec{Role: "w"})
		r.RemoveErr = boom
		if _, err := r.RemoveStrand(s.GUID, false); !errors.Is(err, boom) {
			t.Fatalf("RemoveStrand err = %v", err)
		}
		status, _ := r.Status()
		if len(status.Strands) != 1 {
			t.Errorf("strand must survive a refused remove, got %+v", status)
		}
	})
	t.Run("Fns", func(t *testing.T) {
		r := &Reed{
			AddStrandFn:    func(reedengine.AddSpec) (reedengine.Strand, error) { return reedengine.Strand{GUID: "scripted"}, nil },
			RemoveStrandFn: func(string, bool) (reedengine.Removed, error) { return reedengine.Removed{}, boom },
			StatusFn:       func() (reedengine.StatusResult, error) { return reedengine.StatusResult{Session: "sess"}, nil },
			SendTextFn:     func(string, string, bool) error { return boom },
			SendKeyFn:      func(string, string) error { return boom },
			CapturePaneFn:  func(string) (string, error) { return "pane", nil },
		}
		if s, _ := r.AddStrand(reedengine.AddSpec{}); s.GUID != "scripted" {
			t.Errorf("AddStrandFn ignored: %+v", s)
		}
		if _, err := r.RemoveStrand("g", false); !errors.Is(err, boom) {
			t.Errorf("RemoveStrandFn ignored: %v", err)
		}
		if st, _ := r.Status(); st.Session != "sess" {
			t.Errorf("StatusFn ignored: %+v", st)
		}
		if err := r.SendText("g", "t", true); !errors.Is(err, boom) {
			t.Errorf("SendTextFn ignored: %v", err)
		}
		if err := r.SendKey("g", "k"); !errors.Is(err, boom) {
			t.Errorf("SendKeyFn ignored: %v", err)
		}
		if got, _ := r.CapturePane("g"); got != "pane" {
			t.Errorf("CapturePaneFn ignored: %q", got)
		}
	})
}

func TestReed_PaneTransportRecordsCalls(t *testing.T) {
	r := &Reed{}
	if err := r.SendText("g1", "hello", true); err != nil {
		t.Fatal(err)
	}
	if err := r.SendKey("g2", "Escape"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.CapturePane("g3"); got != "" || err != nil {
		t.Errorf("CapturePane = %q, %v; want empty, nil", got, err)
	}
	if len(r.SendTextCalls) != 1 || r.SendTextCalls[0] != (SendTextCall{GUID: "g1", Text: "hello", Submit: true}) {
		t.Errorf("SendTextCalls = %+v", r.SendTextCalls)
	}
	if len(r.SendKeyCalls) != 1 || r.SendKeyCalls[0] != (SendKeyCall{GUID: "g2", Key: "Escape"}) {
		t.Errorf("SendKeyCalls = %+v", r.SendKeyCalls)
	}
	if len(r.CaptureCalls) != 1 || r.CaptureCalls[0] != "g3" {
		t.Errorf("CaptureCalls = %v", r.CaptureCalls)
	}
}

func TestReed_ConcurrentAddAndStatus(t *testing.T) {
	const adders = 16
	r := &Reed{}
	var wg sync.WaitGroup
	guids := make(chan string, adders)
	for i := 0; i < adders; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s, err := r.AddStrand(reedengine.AddSpec{Role: "w"})
			if err != nil {
				t.Error(err)
				return
			}
			guids <- s.GUID
		}()
		go func() {
			defer wg.Done()
			if _, err := r.Status(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(guids)

	seen := map[string]bool{}
	for g := range guids {
		if seen[g] {
			t.Errorf("GUID %s minted twice", g)
		}
		seen[g] = true
	}
	if status, _ := r.Status(); len(status.Strands) != adders {
		t.Errorf("Status holds %d strands, want %d", len(status.Strands), adders)
	}
}
