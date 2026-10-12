// resume_test.go drives the resume sequence and the status read over the fakes pause_test.go declares.

package pauseengine

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
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
		{"a dead, busy or draft-holding session gets no line and each is named with its reason", func(t *testing.T) {
			p := newPair("p1", "dead", "busy", "draft", "idle-at-pause")
			p.dead["g-dead"] = true
			p.busy["g-busy"] = []bool{true}
			p.sendErr["g-draft"] = fmt.Errorf("%w: the input box holds a draft", shuttleengine.ErrSessionBusy)
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "dead"), recorded("p1", "busy"), recorded("p1", "draft")))

			report, err := e.resume("")

			if err != nil {
				t.Fatal(err)
			}
			reasons := map[string]string{}
			for _, note := range report.NoLine {
				reasons[note.Strand] = note.Reason
			}
			want := map[string]string{"dead": "the strand is not live", "busy": "the session reads busy", "draft": "the input box holds a draft"}
			if !reflect.DeepEqual(reasons, want) || len(report.Lines) != 0 || len(p.sends) != 0 {
				t.Errorf("no-line reasons %v, lines %v, sends %v; want %v and nothing sent, the idle-at-pause session included", reasons, report.Lines, p.sends, want)
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
}
