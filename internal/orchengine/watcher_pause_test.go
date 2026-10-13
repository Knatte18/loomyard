// watcher_pause_test.go drives the watcher over a temp pause record: what it interrupts, holds and queues while a hub pause entry is live and after it is resumed.

package orchengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const pauseGrace = 30 * time.Second

// pauseHub writes a live hub entry paused now with the given grace and caller.
func (e *watchEnv) pauseHub(caller string, grace time.Duration) {
	e.t.Helper()
	entry := pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: e.clock.now, GraceEnd: e.clock.now.Add(grace), Caller: caller}
	_, err := pauserecord.Update(e.pausePath, e.clock.now, func(r *pauserecord.Record) error {
		r.Live = append(r.Live, entry)
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// resumeHub moves every live entry to the resumed list as of now.
func (e *watchEnv) resumeHub() {
	e.t.Helper()
	_, err := pauserecord.Update(e.pausePath, e.clock.now, func(r *pauserecord.Record) error {
		for _, entry := range r.Live {
			entry.ResumedAt = e.clock.now
			r.Resumed = append(r.Resumed, entry)
		}
		r.Live = nil
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *watchEnv) pauseRecord() pauserecord.Record {
	e.t.Helper()
	record, err := pauserecord.Read(e.pausePath)
	if err != nil {
		e.t.Fatal(err)
	}
	return record
}

// orchMarker writes a network operation marker for the orch's strand beside the record.
func (e *watchEnv) orchMarker() {
	e.t.Helper()
	strand, err := agentname.Format("lyx", "", agentname.RoleOrch)
	if err != nil {
		e.t.Fatal(err)
	}
	data, err := json.Marshal(pauserecord.NetOp{PID: 1, Strand: strand, Op: "fetch", StartedAt: e.clock.now})
	if err != nil {
		e.t.Fatal(err)
	}
	dir := pauserecord.NetDir(e.pausePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1.json"), data, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *watchEnv) callCount(call string) int {
	n := 0
	for _, got := range e.s.calls {
		if got == call {
			n++
		}
	}
	return n
}

// orchSessions lists the orch sessions the record's live and resumed entries own.
func (e *watchEnv) orchSessions() []pauserecord.Session {
	record := e.pauseRecord()
	var sessions []pauserecord.Session
	for _, entry := range append(append([]pauserecord.Entry(nil), record.Live...), record.Resumed...) {
		sessions = append(sessions, entry.Sessions...)
	}
	return sessions
}

// assertPauseLogs checks the pause's guard logs in logs: wantInfo counts the "no running turn" Info lines and wantPicker the "rewind picker" Warn lines, each naming s1, and the could-not-interrupt Warn is absent.
func assertPauseLogs(t *testing.T, logs string, wantInfo, wantPicker int) {
	t.Helper()
	if got := strings.Count(logs, "the pause found no running turn on the orch pane"); got != wantInfo {
		t.Errorf("no-running-turn lines = %d; want %d; logs: %s", got, wantInfo, logs)
	}
	if got := strings.Count(logs, "the pause found the rewind picker open on the orch pane"); got != wantPicker {
		t.Errorf("rewind-picker lines = %d; want %d; logs: %s", got, wantPicker, logs)
	}
	if got := strings.Count(logs, "could not interrupt the orch"); got != 0 {
		t.Errorf("could-not-interrupt lines = %d; want 0; logs: %s", got, logs)
	}
	if wantInfo+wantPicker > 0 && !strings.Contains(logs, "s1") {
		t.Errorf("logs do not name the strand s1: %s", logs)
	}
}

func TestWatcher_HubPauseInterruptsAtGraceEndHoldsAndQueuesTheResumeLine(t *testing.T) {
	orchCaller, err := agentname.Format("lyx", "", agentname.RoleOrch)
	if err != nil {
		t.Fatal(err)
	}
	ownedBy := []pauserecord.Session{{GUID: "s1", Orch: true}}
	tests := []struct {
		name string
		run  func(t *testing.T, e *watchEnv)
	}{
		{"a busy orch is interrupted at the grace end and not before, a second time only on a tick past the settle while still busy, and recorded", func(t *testing.T, e *watchEnv) {
			e.s.settle = 2 * time.Second
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.tick()
			if n := e.callCount(interruptCall); n != 0 {
				t.Fatalf("interrupts before the grace end = %d; want 0", n)
			}
			e.clock.advance(pauseGrace)
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Fatalf("interrupts at the grace end = %d; want 1", n)
			}
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Fatalf("interrupts on a tick within the settle = %d; want still 1", n)
			}
			e.clock.advance(2 * time.Second)
			e.tick()
			if n := e.callCount(interruptCall); n != 2 {
				t.Errorf("interrupts after the settle = %d; want 2", n)
			}
			if got := e.orchSessions(); !reflect.DeepEqual(got, ownedBy) {
				t.Errorf("orch sessions in the entry = %+v; want %+v", got, ownedBy)
			}
		}},
		{"an orch idle at the settled re-probe gets no second Esc", func(t *testing.T, e *watchEnv) {
			e.s.settle = 2 * time.Second
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.s.idle = true
			e.clock.advance(2 * time.Second)
			e.tick()
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
		}},
		{"a resume during the settle sends no second Esc", func(t *testing.T, e *watchEnv) {
			e.s.settle = 2 * time.Second
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.resumeHub()
			e.clock.advance(2 * time.Second)
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
			if lines := e.noticeLines(); !reflect.DeepEqual(lines, []string{pauserecord.ResumeLine}) {
				t.Errorf("notices = %v; want the orch's resume line", lines)
			}
		}},
		{"a guarded refusal reads as an idle orch and is logged once at Info", func(t *testing.T, e *watchEnv) {
			logs := logcapture.CaptureVerbose(t)
			e.s.settle = 2 * time.Second
			e.s.interruptErrs = []error{fmt.Errorf("%w (strand %q)", shuttleengine.ErrNoRunningTurn, "s1")}
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.clock.advance(2 * time.Second)
			e.tick()
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
			assertPauseLogs(t, logs.String(), 1, 0)
		}},
		{"a guarded refusal of the settled second Esc is logged once at Info and ends the entry", func(t *testing.T, e *watchEnv) {
			logs := logcapture.CaptureVerbose(t)
			e.s.settle = 2 * time.Second
			e.s.interruptErrs = []error{nil, fmt.Errorf("%w (strand %q)", shuttleengine.ErrNoRunningTurn, "s1")}
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.clock.advance(2 * time.Second)
			e.tick()
			e.tick()
			if n := e.callCount(interruptCall); n != 2 {
				t.Errorf("interrupts = %d; want 2", n)
			}
			assertPauseLogs(t, logs.String(), 1, 0)
		}},
		{"an open rewind picker is logged at Warn and gets no further Esc", func(t *testing.T, e *watchEnv) {
			logs := logcapture.CaptureVerbose(t)
			e.s.settle = 2 * time.Second
			e.s.interruptErrs = []error{fmt.Errorf("%w (strand %q)", shuttleengine.ErrRewindPickerOpen, "s1")}
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.clock.advance(2 * time.Second)
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
			assertPauseLogs(t, logs.String(), 0, 1)
		}},
		{"a watcher restarted during the settle sends no second Esc", func(t *testing.T, e *watchEnv) {
			e.s.settle = 2 * time.Second
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.w = e.newWatcher()
			e.clock.advance(2 * time.Second)
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
		}},
		{"an idle orch with no marker is never interrupted or recorded, and gets no resume line", func(t *testing.T, e *watchEnv) {
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.tick()
			e.clock.advance(pauseGrace)
			e.s.idle = false
			e.tick()
			e.resumeHub()
			e.s.idle = true
			e.tick()
			if n := e.callCount(interruptCall); n != 0 {
				t.Errorf("interrupts = %d; want 0", n)
			}
			if got := e.orchSessions(); len(got) != 0 {
				t.Errorf("orch sessions = %+v; want none", got)
			}
			if lines := e.noticeLines(); len(lines) != 0 || len(e.s.calls) != 0 {
				t.Errorf("notices %v and calls %v; want none", lines, e.s.calls)
			}
		}},
		{"an idle orch with an open orch marker is left alone when it reads idle at the grace end", func(t *testing.T, e *watchEnv) {
			e.tick()
			e.orchMarker()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.tick()
			e.clock.advance(pauseGrace)
			e.tick()
			if n := e.callCount(interruptCall); n != 0 {
				t.Errorf("interrupts = %d; want 0 for an orch idle at the grace end", n)
			}
		}},
		{"an unreadable network operation marker list warns once across the grace's ticks and logs its recovery", func(t *testing.T, e *watchEnv) {
			logs := logcapture.CaptureVerbose(t)
			e.tick()
			netDir := pauserecord.NetDir(e.pausePath)
			if err := os.MkdirAll(filepath.Dir(netDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(netDir, []byte("not a directory"), 0o644); err != nil {
				t.Fatal(err)
			}
			e.pauseHub("EXTERNAL", pauseGrace)
			for range 3 {
				e.tick()
			}
			const unreadable, readable = "network operation markers cannot be read", "network operation markers are readable again"
			if got := strings.Count(logs.String(), unreadable); got != 1 {
				t.Fatalf("unreadable lines across three ticks = %d; want 1; logs: %s", got, logs.String())
			}
			if err := os.Remove(netDir); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(netDir, 0o755); err != nil {
				t.Fatal(err)
			}
			// A failed read marks its entry handled, so the readable list is first asked about by the next entry.
			e.resumeHub()
			e.clock.advance(time.Second)
			e.pauseHub("EXTERNAL", pauseGrace)
			e.tick()
			if got := strings.Count(logs.String(), readable); got != 1 {
				t.Errorf("recovery lines = %d; want 1; logs: %s", got, logs.String())
			}
			if got := strings.Count(logs.String(), unreadable); got != 1 {
				t.Errorf("unreadable lines after the recovery = %d; want still 1", got)
			}
		}},
		{"an idle orch with an open orch marker is interrupted when it reads busy at the grace end", func(t *testing.T, e *watchEnv) {
			e.tick()
			e.orchMarker()
			e.s.idleSeq = []bool{true}
			e.s.idle = false
			e.pauseHub("EXTERNAL", pauseGrace)
			e.tick()
			if n := e.callCount(interruptCall); n != 0 {
				t.Fatalf("interrupts before the grace end = %d; want 0", n)
			}
			e.clock.advance(pauseGrace)
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
		}},
		{"an entry the orch itself ran interrupts nothing", func(t *testing.T, e *watchEnv) {
			e.s.idle = false
			e.tick()
			e.pauseHub(orchCaller, pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			if n := e.callCount(interruptCall); n != 0 {
				t.Errorf("interrupts = %d; want 0", n)
			}
		}},
		{"a watcher created after the grace end interrupts nothing, still holds, and queues no resume line", func(t *testing.T, e *watchEnv) {
			e.s.idle = false
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(2 * pauseGrace)
			e.w = e.newWatcher()
			e.queue("hello")
			e.tick()
			if n := e.callCount(interruptCall); n != 0 || len(e.noticeLines()) != 1 {
				t.Fatalf("interrupts = %d and notices = %v; want none interrupted and the notice held", n, e.noticeLines())
			}
			e.resumeHub()
			e.s.idle = true
			e.tick()
			if want := []string{"send:hello"}; !reflect.DeepEqual(e.s.calls, want) {
				t.Errorf("calls after the resume = %v; want %v, with no resume line", e.s.calls, want)
			}
		}},
		{"a watcher running before the entry interrupts a busy orch on the first tick after a zero grace", func(t *testing.T, e *watchEnv) {
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", 0)
			e.clock.advance(5 * time.Second)
			e.tick()
			if n := e.callCount(interruptCall); n != 1 {
				t.Errorf("interrupts = %d; want 1", n)
			}
			if got := e.orchSessions(); !reflect.DeepEqual(got, ownedBy) {
				t.Errorf("orch sessions = %+v; want %+v", got, ownedBy)
			}
		}},
		{"an orch launched after the pause began is not interrupted and gets no resume line", func(t *testing.T, e *watchEnv) {
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(10 * time.Second)
			e.setState(func(st *State) { st.LaunchedAt = e.clock.now })
			e.clock.advance(pauseGrace)
			e.tick()
			e.resumeHub()
			e.s.idle = true
			e.tick()
			if n := e.callCount(interruptCall); n != 0 || len(e.s.calls) != 0 {
				t.Errorf("interrupts = %d and calls = %v; want none", n, e.s.calls)
			}
		}},
		{"a notice and a cycle request wait while the entry is live, then the resume line follows once", func(t *testing.T, e *watchEnv) {
			e.s.idle = false
			e.tick()
			e.pauseHub("EXTERNAL", pauseGrace)
			e.clock.advance(pauseGrace)
			e.tick()
			e.queue("n1")
			if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
				t.Fatal(err)
			}
			e.s.idle = true
			e.tick()
			if st := e.state(); st.Phase != PhaseIdle || e.callCount("send:n1") != 0 {
				t.Fatalf("phase = %s and calls = %v; want idle and nothing delivered while the entry is live", st.Phase, e.s.calls)
			}
			if err := ClearCycleRequest(e.paths); err != nil {
				t.Fatal(err)
			}
			e.resumeHub()
			// Notices sort by queue time, so the resume line must be queued after n1's instant.
			e.clock.advance(time.Millisecond)
			for range 4 {
				e.tick()
			}
			want := []string{interruptCall, "send:n1", "send:" + pauserecord.ResumeLine}
			if !reflect.DeepEqual(e.s.calls, want) {
				t.Errorf("calls = %v; want %v", e.s.calls, want)
			}
			if sessions := e.orchSessions(); len(sessions) != 1 || !sessions[0].ResumeNoticed {
				t.Errorf("orch sessions = %+v; want the one marked resume-noticed", sessions)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, newWatchEnv(t))
		})
	}
}
