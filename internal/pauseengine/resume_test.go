// resume_test.go drives the resume sequence and the status read over the fakes pause_test.go declares.

package pauseengine

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestResume(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"each owned session gets one line, one a killed pause recorded included, and the entry moves to resumed", func(t *testing.T) {
			p1, p2 := newPair("p1", "a"), newPair("p2", "b")
			e := newEnv(t, p1, p2)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a"), recorded("p2", "b")))

			report, err := e.resume("")

			if err != nil || !reflect.DeepEqual(report.Resumed, []string{pauserecord.ScopeHub}) || !reflect.DeepEqual(report.Lines, []string{"a", "b"}) {
				t.Fatalf("report %+v, err %v; want the hub entry resumed and a line for a and b", report, err)
			}
			if !reflect.DeepEqual(p1.sends, []string{"g-a"}) || !reflect.DeepEqual(p2.sends, []string{"g-b"}) {
				t.Errorf("sends %v and %v; want one line each", p1.sends, p2.sends)
			}
			record := e.record()
			if len(record.Live) != 0 || len(record.Resumed) != 1 || record.Resumed[0].ResumedAt.IsZero() {
				t.Errorf("record = %+v; want the entry moved to resumed with a time", record)
			}
		}},
		{"a dead strand, a held send and an open picker get no line, each named with its reason", func(t *testing.T) {
			p := newPair("p1", "dead", "held", "draft", "picker", "idle-at-pause")
			p.dead["g-dead"] = true
			p.sendErr["g-held"] = &shuttleengine.SessionBusyError{
				Reason: "a turn is running",
				Err:    fmt.Errorf("%w: strand: a turn is running; retry once the session is idle. The pane's last lines:\ntail", shuttleengine.ErrSessionBusy),
			}
			p.sendErr["g-draft"] = &shuttleengine.SessionBusyError{
				Reason: `the input box holds a draft: "x"`,
				Err:    fmt.Errorf("%w: strand: the input box holds a draft", shuttleengine.ErrSessionBusy),
			}
			p.sendErr["g-picker"] = fmt.Errorf("%w: strand g-picker: one interrupt key closed it", shuttleengine.ErrRewindPickerOpen)
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "dead"), recorded("p1", "held"), recorded("p1", "draft"), recorded("p1", "picker")))

			report, err := e.resume("")

			if err != nil {
				t.Fatal(err)
			}
			reasons := map[string]string{}
			for _, note := range report.NoLine {
				reasons[note.Strand] = note.Reason
			}
			if reason := reasons["picker"]; !strings.HasPrefix(reason, "the send failed: ") || !strings.Contains(reason, "rewind picker was open") {
				t.Errorf("picker reason = %q; want \"the send failed: \" and the picker's message", reason)
			}
			delete(reasons, "picker")
			want := map[string]string{"dead": "the strand is not live", "held": "a turn is running", "draft": `the input box holds a draft: "x"`}
			if !reflect.DeepEqual(reasons, want) || len(report.Lines) != 0 || len(p.sends) != 0 {
				t.Errorf("no-line reasons %v, lines %v, sends %v; want %v and nothing sent, the idle-at-pause session included", reasons, report.Lines, p.sends, want)
			}
		}},
		{"a recorded session that reads busy at resume still gets its line", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))

			report, err := e.resume("")

			if err != nil || !reflect.DeepEqual(report.Lines, []string{"a"}) || !reflect.DeepEqual(p.sends, []string{"g-a"}) {
				t.Errorf("report %+v, err %v, sends %v; want one line for a, the busy pre-check gone", report, err, p.sends)
			}
		}},
		{"a session two entries cover is interrupted once, moves with no line when the first resumes, and gets its line with the last", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.onInterrupt = func(guid string) { p.busy[guid] = []bool{false} }
			e := newEnv(t, p)
			e.pause("p1", 0)
			e.pause("", 0)
			if len(p.interrupts) != 1 {
				t.Fatalf("interrupts = %v; want the session interrupted once across both pauses", p.interrupts)
			}

			first, err := e.resume("p1")
			if err != nil || !reflect.DeepEqual(first.Moved, []string{"a"}) || len(first.Lines) != 0 || len(p.sends) != 0 {
				t.Fatalf("first resume %+v, err %v, sends %v; want a moved and no line", first, err, p.sends)
			}
			if live := e.record().Live; len(live) != 1 || live[0].Scope != pauserecord.ScopeHub || len(live[0].Sessions) != 1 {
				t.Fatalf("live entries = %+v; want the hub entry owning the session", live)
			}

			last, err := e.resume("")
			if err != nil || !reflect.DeepEqual(last.Lines, []string{"a"}) || !reflect.DeepEqual(p.sends, []string{"g-a"}) {
				t.Errorf("last resume %+v, err %v, sends %v; want one line for a", last, err, p.sends)
			}
		}},
		{"the orch's session is left to the watcher and gets no line", func(t *testing.T) {
			p := newPair("p1")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), pauserecord.Session{GUID: "orch-guid", Orch: true}))

			report, err := e.resume("")

			if err != nil || !report.OrchLeftToWatcher || len(report.Lines)+len(report.NoLine) != 0 {
				t.Errorf("report %+v, err %v; want the orch left to its watcher", report, err)
			}
		}},
		{"a slug that only a hub entry covers refuses naming lyx resume and changes nothing", func(t *testing.T) {
			e := newEnv(t, newPair("p1"))
			e.seedLive(hubEntry(e.clock.Now()))

			_, err := e.resume("p1")

			if err == nil || !strings.Contains(err.Error(), `"lyx resume"`) {
				t.Fatalf("err = %v; want a refusal naming lyx resume", err)
			}
			if live := e.record().Live; len(live) != 1 {
				t.Errorf("live entries = %+v; want the hub entry untouched", live)
			}
		}},
		{"resuming nothing succeeds with nothing resumed", func(t *testing.T) {
			e := newEnv(t, newPair("p1"))
			for _, slug := range []string{"", "p1"} {
				report, err := e.resume(slug)
				if err != nil || len(report.Resumed) != 0 || len(report.Lines) != 0 {
					t.Errorf("resume(%q) = %+v, %v; want a no-op success", slug, report, err)
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestStatus(t *testing.T) {
	p := newPair("p1", "a")
	e := newEnv(t, p)
	base := e.clock.Now()
	e.seedLive(hubEntry(base, recorded("p1", "a")))
	e.netOps = []pauserecord.NetOp{{Op: "push", Strand: "x", Slug: "p1", UnderParentLock: true}, {Op: "fetch", Strand: "y", Slug: "p2"}}
	e.locks = []parentlock.Entry{{
		Record:  parentlock.Record{Branch: "main", Owner: parentlock.Owner{Slug: "p1"}},
		Waiters: []parentlock.Ticket{{Owner: parentlock.Owner{Slug: "p2"}, QueuedAt: base}},
	}}
	before, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}

	report, err := Status(e.deps())

	if err != nil {
		t.Fatal(err)
	}
	if len(report.Live) != 1 || report.Live[0].Scope != pauserecord.ScopeHub {
		t.Errorf("live = %+v; want the hub entry", report.Live)
	}
	wantNet := []NetOpReport{{Op: "push", Strand: "x", Slug: "p1", UnderParentLock: true}, {Op: "fetch", Strand: "y", Slug: "p2"}}
	if !reflect.DeepEqual(report.NetOps, wantNet) {
		t.Errorf("net ops = %+v; want %+v with the parent-lock one marked", report.NetOps, wantNet)
	}
	if len(report.Holds.Held) != 1 || len(report.Holds.Waiting) != 1 {
		t.Errorf("holds = %+v; want the held branch and the waiter", report.Holds)
	}
	after, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(p.interrupts)+len(p.sends) != 0 {
		t.Errorf("status changed the record or touched a session")
	}

	read := func(t *testing.T, e *env) StatusReport {
		t.Helper()
		report, err := Status(e.deps())
		if err != nil {
			t.Fatal(err)
		}
		return report
	}

	t.Run("sessions list every pair and the prime with state, cause, since and paused", func(t *testing.T) {
		p1, p2, prime := newPair("p1", "a"), newPair("p2", "b"), newPair("prime", "orch")
		since := newFakeClock().Now()
		p1.states["g-a"] = shuttleengine.SessionState{Name: shuttleengine.SessionBusy, Cause: shuttleengine.SessionCauseTurn}
		p2.states["g-b"] = shuttleengine.SessionState{Name: shuttleengine.SessionIdleStalled, Cause: shuttleengine.SessionCauseAPIError, Since: since}
		e := newEnv(t, p1, p2)
		e.prime = prime
		e.seedLive(pauserecord.Entry{Scope: "p1", PausedAt: since, GraceEnd: since, Caller: "EXTERNAL"})

		want := []SessionStatus{
			{Slug: "p1", Strand: "a", State: "busy", Cause: "turn", Paused: true},
			{Slug: "p2", Strand: "b", State: "idle-stalled", Cause: "api-error", Since: since.Format(time.RFC3339)},
			{Slug: "prime", Strand: "orch", State: "idle-done", Cause: "done", Prime: true},
		}
		if got := read(t, e).Sessions; !reflect.DeepEqual(got, want) {
			t.Errorf("sessions under a slug entry = %+v; want %+v", got, want)
		}

		e.seedLive(hubEntry(since))
		for _, session := range read(t, e).Sessions {
			if !session.Paused {
				t.Errorf("session %+v under a hub entry; want it paused, the prime's included", session)
			}
		}
	})

	t.Run("stale processes keep a replaced lyx inside the hub and nothing else", func(t *testing.T) {
		e := newEnv(t)
		e.hubPath = "/home/u/hub"
		e.executable = "/home/u/go/bin/lyx"
		listed := func(pid int, exe, cwd string) proc.Process {
			return proc.Process{PID: pid, StartTime: "t" + fmt.Sprint(pid), Exe: exe, Cwd: cwd, Argv: []string{"lyx", "orch", "watch"}}
		}
		e.processes = []proc.Process{
			listed(1, "/home/u/go/bin/lyx (deleted)", "/home/u/hub/prime"),
			listed(2, "/home/u/hub/x/.dev-bin/lyx (deleted)", "/home/u/hub"),
			listed(3, "/home/u/go/bin/lyx", "/home/u/hub"),
			listed(4, "/home/u/go/bin/other (deleted)", "/home/u/hub"),
			listed(5, "/home/u/go/bin/lyx (deleted)", "/home/u/hub-other"),
			listed(6, "/home/u/go/bin/lyx (deleted)", "/home/u"),
			listed(7, "/home/u/go/bin/lyx (deleted)", "/home/u/hub/gone (deleted)"),
		}

		report := read(t, e)

		var pids []int
		for _, process := range report.Stale.Processes {
			pids = append(pids, process.PID)
		}
		if !report.Stale.Supported || !reflect.DeepEqual(pids, []int{1, 2, 7}) {
			t.Errorf("stale = %+v; want pids 1, 2 and 7 on a supported platform", report.Stale)
		}
		if first := report.Stale.Processes[0]; first.StartTime != "t1" || !reflect.DeepEqual(first.Argv, []string{"lyx", "orch", "watch"}) {
			t.Errorf("first stale process = %+v; want its start time and argv", first)
		}
	})

	t.Run("a failed reading is reported in its own part and the rest still reads", func(t *testing.T) {
		p := newPair("p1", "a", "b")
		p.stateErr["g-b"] = errors.New("events unreadable")
		e := newEnv(t, p)
		e.seedLive(hubEntry(e.clock.Now()))
		e.processErr = errors.New("proc is gone")

		report := read(t, e)

		if len(report.Sessions) != 2 || report.Sessions[1].State != "unknown" || report.Sessions[1].Cause != "events unreadable" || report.Sessions[0].State != "idle-done" {
			t.Errorf("sessions = %+v; want a read and b unknown with the error as its cause", report.Sessions)
		}
		if report.Stale.Error != "proc is gone" || !report.Stale.Supported || len(report.Stale.Processes) != 0 || len(report.Live) != 1 {
			t.Errorf("stale = %+v, live = %+v; want the error reported and the live entry still read", report.Stale, report.Live)
		}
	})

	t.Run("an unsupported process reading marks the part unsupported", func(t *testing.T) {
		e := newEnv(t, newPair("p1", "a"))
		e.seedLive(hubEntry(e.clock.Now()))
		e.processErr = fmt.Errorf("proc: %w", errors.ErrUnsupported)

		report := read(t, e)

		if report.Stale.Supported || report.Stale.Error != "" || len(report.Stale.Processes) != 0 || len(report.Sessions) != 1 || len(report.Live) != 1 {
			t.Errorf("report = %+v; want the stale part unsupported with no error and sessions and live entries read", report)
		}
	})

	t.Run("an empty hub reads empty lists, not nil", func(t *testing.T) {
		report := read(t, newEnv(t))

		if report.Live == nil || report.Sessions == nil || report.Stale.Processes == nil || !report.Stale.Supported {
			t.Errorf("report = %+v; want empty non-nil lists and a supported stale part", report)
		}
		if len(report.Live)+len(report.Sessions)+len(report.Stale.Processes) != 0 {
			t.Errorf("report = %+v; want nothing listed", report)
		}
	})
}
