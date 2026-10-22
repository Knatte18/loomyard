// resume_test.go drives the resume sequence, the revive sequence and the status read over the fakes pause_test.go declares.

package tenderengine

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestResume(t *testing.T) {
	pausedP1 := &shuttleengine.PausedError{Slug: "p1", Err: shuttleengine.ErrPaused}
	// addLiveP1 adds a live entry of scope p1 owning sessions, the pause that lands after the lift's lock.
	addLiveP1 := func(t *testing.T, e *env, sessions ...pauserecord.Session) {
		t.Helper()
		now := e.clock.Now()
		_, err := pauserecord.Update(e.path, now, func(r *pauserecord.Record) error {
			r.Live = append(r.Live, pauserecord.Entry{Scope: "p1", PausedAt: now, GraceEnd: now, Caller: "EXTERNAL", Sessions: sessions})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// liveP1Sessions returns the sessions the live p1 entry owns, nil when none is live.
	liveP1Sessions := func(e *env) []pauserecord.Session {
		for _, entry := range e.record().Live {
			if entry.Scope == "p1" {
				return entry.Sessions
			}
		}
		return nil
	}
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
		{"a lift by identity lifts the hub entry alone, gives its sessions their line and leaves a slug entry live", func(t *testing.T) {
			p1 := newPair("p1", "a")
			e := newEnv(t, p1, newPair("p2"))
			hub := hubEntry(e.clock.Now(), recorded("p1", "a"), pauserecord.Session{GUID: "orch-guid", Orch: true})
			slug := pauserecord.Entry{Scope: "p2", PausedAt: e.clock.Now(), GraceEnd: e.clock.Now(), Caller: "EXTERNAL", Sessions: []pauserecord.Session{}}
			e.seedLive(hub, slug)

			report, err := Lift(e.deps(), EntryRef{Scope: hub.Scope, Caller: hub.Caller, PausedAt: hub.PausedAt})

			if err != nil || !reflect.DeepEqual(report.Resumed, []string{pauserecord.ScopeHub}) || !reflect.DeepEqual(report.Lines, []string{"a"}) || !report.OrchLeftToWatcher {
				t.Fatalf("report %+v, err %v; want the hub entry lifted, a given its line and the orch left to its watcher", report, err)
			}
			if !reflect.DeepEqual(p1.sends, []string{"g-a"}) {
				t.Errorf("sends = %v; want one line for a", p1.sends)
			}
			if live := e.record().Live; len(live) != 1 || live[0].Scope != "p2" {
				t.Errorf("live entries = %+v; want the p2 entry alone", live)
			}
		}},
		{"a lift whose ref matches no live entry resumes nothing and sends nothing", func(t *testing.T) {
			later := newFakeClock().Now().Add(time.Hour)
			refs := []struct {
				name string
				ref  EntryRef
			}{
				{"an entry no longer live while a later hub entry stands", EntryRef{Scope: pauserecord.ScopeHub, Caller: "EXTERNAL", PausedAt: later.Add(-time.Minute)}},
				{"the live entry's scope and time under another caller", EntryRef{Scope: pauserecord.ScopeHub, Caller: "someone-else", PausedAt: later}},
			}
			for _, tc := range refs {
				p := newPair("p1", "a")
				e := newEnv(t, p)
				e.seedLive(hubEntry(later, recorded("p1", "a")))

				report, err := Lift(e.deps(), tc.ref)

				if err != nil || len(report.Resumed)+len(report.Lines)+len(report.Moved)+len(report.NoLine) != 0 || len(p.sends) != 0 {
					t.Errorf("%s: report %+v, err %v, sends %v; want nothing lifted", tc.name, report, err, p.sends)
				}
				if live := e.record().Live; len(live) != 1 || len(live[0].Sessions) != 1 {
					t.Errorf("%s: live entries = %+v; want the hub entry live with its session", tc.name, live)
				}
			}
		}},
		{"a send refused by a pause added after the lift claims the session into it, which owns it and gives the session no line now", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))
			p.onSend = func(string) error {
				addLiveP1(t, e)
				return pausedP1
			}

			report, err := e.resume("")

			if err != nil || !reflect.DeepEqual(report.Moved, []string{"a"}) || len(report.Lines)+len(report.NoLine) != 0 || len(p.sends) != 0 {
				t.Errorf("report %+v, err %v, sends %v; want a moved and no line", report, err, p.sends)
			}
			if got := liveP1Sessions(e); !reflect.DeepEqual(got, []pauserecord.Session{recorded("p1", "a")}) {
				t.Errorf("live p1 sessions = %+v; want a owned by the new entry", got)
			}
		}},
		{"a refusal whose pause ended is sent again and the second send lands", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))
			refused := false
			p.onSend = func(string) error {
				if refused {
					return nil
				}
				refused = true
				return pausedP1
			}

			report, err := e.resume("")

			if err != nil || !reflect.DeepEqual(report.Lines, []string{"a"}) || len(report.Moved)+len(report.NoLine) != 0 || !reflect.DeepEqual(p.sends, []string{"g-a"}) {
				t.Errorf("report %+v, err %v, sends %v; want a given its line by the second send", report, err, p.sends)
			}
		}},
		{"a second refusal by a newer pause claims the session into it", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))
			sends := 0
			p.onSend = func(string) error {
				sends++
				if sends == 2 {
					addLiveP1(t, e)
				}
				return pausedP1
			}

			report, err := e.resume("")

			if err != nil || sends != 2 || !reflect.DeepEqual(report.Moved, []string{"a"}) || len(report.Lines)+len(report.NoLine) != 0 {
				t.Errorf("report %+v, err %v, %d sends; want a moved after two sends", report, err, sends)
			}
			if got := liveP1Sessions(e); !reflect.DeepEqual(got, []pauserecord.Session{recorded("p1", "a")}) {
				t.Errorf("live p1 sessions = %+v; want a owned by the newer entry", got)
			}
		}},
		{"two refusals with no pause left name the ended-pause reason and send no third time", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))
			sends := 0
			p.onSend = func(string) error {
				sends++
				return pausedP1
			}

			report, err := e.resume("")

			want := []SessionNote{{"a", "p1", "the send was held by a pause that has since ended and a second one"}}
			if err != nil || sends != 2 || !reflect.DeepEqual(report.NoLine, want) || len(report.Lines)+len(report.Moved) != 0 {
				t.Errorf("report %+v, err %v, %d sends; want a under no_line with the ended-pause reason after two sends", report, err, sends)
			}
		}},
		{"a refusing entry that already owns the session reports it moved and keeps one copy", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))
			p.onSend = func(string) error {
				addLiveP1(t, e, recorded("p1", "a"))
				return pausedP1
			}

			report, err := e.resume("")

			if err != nil || !reflect.DeepEqual(report.Moved, []string{"a"}) || len(report.Lines)+len(report.NoLine) != 0 {
				t.Errorf("report %+v, err %v; want a moved", report, err)
			}
			if got := liveP1Sessions(e); !reflect.DeepEqual(got, []pauserecord.Session{recorded("p1", "a")}) {
				t.Errorf("live p1 sessions = %+v; want one copy of a", got)
			}
		}},
		{"a record made undecodable during the send names the claim failure", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))
			p.onSend = func(string) error {
				if err := os.WriteFile(e.path, []byte("{not json"), 0o644); err != nil {
					t.Error(err)
				}
				return pausedP1
			}

			report, err := e.resume("")

			if err != nil || len(report.NoLine) != 1 || !strings.HasPrefix(report.NoLine[0].Reason, "the send was held by a pause and the claim failed: ") || len(report.Lines)+len(report.Moved) != 0 {
				t.Errorf("report %+v, err %v; want a under no_line with the claim-failed reason", report, err)
			}
		}},
		{"a prime session a pause recorded gets its line through the prime pair", func(t *testing.T) {
			prime := newPair("", "x")
			e := newEnv(t, newPair("p1"))
			e.prime = prime
			e.seedLive(hubEntry(e.clock.Now(), recorded("", "x")))

			report, err := e.resume("")

			if err != nil || !reflect.DeepEqual(report.Lines, []string{"x"}) || len(report.NoLine) != 0 || !reflect.DeepEqual(prime.sends, []string{"g-x"}) {
				t.Errorf("report %+v, err %v, prime sends %v; want the prime session given its line", report, err, prime.sends)
			}
		}},
		{"resuming nothing succeeds with nothing resumed and spawns nothing", func(t *testing.T) {
			e := newEnv(t, newPair("p1"))
			for _, slug := range []string{"", "p1"} {
				report, err := e.resume(slug)
				if err != nil || len(report.Resumed) != 0 || len(report.Lines) != 0 || len(report.Spawned) != 0 || len(report.SpawnFailed) != 0 {
					t.Errorf("resume(%q) = %+v, %v; want a no-op success", slug, report, err)
				}
			}
			if len(e.pushedPairs) != 0 || e.primePushes != 0 || e.boardSyncs != 0 {
				t.Errorf("spawns = %v, %d, %d; want none", e.pushedPairs, e.primePushes, e.boardSyncs)
			}
		}},
		{"a lifted slug entry spawns that pair's push, the prime records push and the board sync", func(t *testing.T) {
			e := newEnv(t, newPair("p1"), newPair("p2"))
			e.seedLive(pauserecord.Entry{Scope: "p1", PausedAt: e.clock.Now(), GraceEnd: e.clock.Now(), Caller: "EXTERNAL"})

			report, err := e.resume("p1")

			if err != nil || !reflect.DeepEqual(report.Spawned, []string{"pair:p1", "prime-records", "board-sync"}) || len(report.SpawnFailed) != 0 {
				t.Errorf("report %+v, err %v; want pair:p1, prime-records and board-sync spawned", report, err)
			}
			if !reflect.DeepEqual(e.pushedPairs, []string{"p1"}) || e.primePushes != 1 || e.boardSyncs != 1 {
				t.Errorf("spawns = %v, %d, %d; want p1, one prime push and one sync", e.pushedPairs, e.primePushes, e.boardSyncs)
			}
		}},
		{"a lifted hub entry spawns every pair's push but one a live entry covers, one owned elsewhere and one that is gone", func(t *testing.T) {
			e := newEnv(t, newPair("p1"), newPair("p2"), newPair("p3"), newPair("p4"))
			e.owners = []PairOwner{
				{Slug: "p1", Local: true, OwnedHere: true},
				{Slug: "p2", Owner: "other", Local: true},
				{Slug: "p3", Local: true, OwnedHere: true},
				{Slug: "p4", Local: true, OwnedHere: true},
			}
			e.pairs = e.pairs[:3]
			now := e.clock.Now()
			e.seedLive(hubEntry(now), pauserecord.Entry{Scope: "p3", PausedAt: now, GraceEnd: now, Caller: "EXTERNAL"})

			report, err := Lift(e.deps(), EntryRef{Scope: pauserecord.ScopeHub, Caller: "EXTERNAL", PausedAt: now})

			if err != nil || !reflect.DeepEqual(e.pushedPairs, []string{"p1"}) || !reflect.DeepEqual(report.Spawned, []string{"pair:p1", "prime-records", "board-sync"}) {
				t.Errorf("report %+v, err %v, pushed %v; want only p1 pushed", report, err, e.pushedPairs)
			}
		}},
		{"a lifted entry whose pair is gone spawns no pair push", func(t *testing.T) {
			e := newEnv(t, newPair("p1"))
			e.seedLive(pauserecord.Entry{Scope: "gone", PausedAt: e.clock.Now(), GraceEnd: e.clock.Now(), Caller: "EXTERNAL"})

			report, err := e.resume("gone")

			if err != nil || len(e.pushedPairs) != 0 || !reflect.DeepEqual(report.Spawned, []string{"prime-records", "board-sync"}) {
				t.Errorf("report %+v, err %v, pushed %v; want no pair push", report, err, e.pushedPairs)
			}
		}},
		{"a failing pair push is listed under spawn_failed and the lift still succeeds", func(t *testing.T) {
			e := newEnv(t, newPair("p1"))
			e.pushPairErr = errors.New("spawn refused")
			e.seedLive(pauserecord.Entry{Scope: "p1", PausedAt: e.clock.Now(), GraceEnd: e.clock.Now(), Caller: "EXTERNAL"})

			report, err := e.resume("p1")

			want := []SessionNote{{"pair:p1", "p1", "spawn refused"}}
			if err != nil || !reflect.DeepEqual(report.SpawnFailed, want) || !reflect.DeepEqual(report.Resumed, []string{"p1"}) {
				t.Errorf("report %+v, err %v; want the push failure under spawn_failed and p1 resumed", report, err)
			}
		}},
		{"an owners error spawns no pair push and is listed under spawn_failed", func(t *testing.T) {
			e := newEnv(t, newPair("p1"))
			e.ownersErr = errors.New("board unreadable")
			e.seedLive(hubEntry(e.clock.Now()))

			report, err := e.resume("")

			if err != nil || len(e.pushedPairs) != 0 || len(report.SpawnFailed) != 1 || !strings.Contains(report.SpawnFailed[0].Reason, "board unreadable") {
				t.Errorf("report %+v, err %v, pushed %v; want no pair push and the owners error listed", report, err, e.pushedPairs)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestRevive(t *testing.T) {
	stalled := shuttleengine.SessionState{Name: shuttleengine.SessionIdleStalled, Cause: shuttleengine.SessionCauseAPIError}
	strands := func(notes []SessionNote) []string {
		names := []string{}
		for _, note := range notes {
			names = append(names, note.Strand)
		}
		return names
	}
	revive := func(t *testing.T, e *env, slug string) ReviveReport {
		t.Helper()
		report, err := Revive(e.deps(), ReviveRequest{Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		return report
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"nothing stalled lists nothing, reports no orch and sends nothing", func(t *testing.T) {
			p := newPair("p1")
			e := newEnv(t, p)

			report := revive(t, e, "")

			empty := len(report.Sent)+len(report.SentNoTurnStart)+len(report.NotLanded)+len(report.Paused)+len(report.Skipped)+len(report.Refused) == 0
			if !empty || report.Sent == nil || report.Orch != nil || len(p.conditioned) != 0 {
				t.Errorf("report %+v, sent to %v; want every list empty, no orch part and nothing sent", report, p.conditioned)
			}
		}},
		{"only a session stalled on an api error is sent the line", func(t *testing.T) {
			p := newPair("p1", "a", "b", "c", "d")
			p.states["g-a"] = stalled
			p.states["g-b"] = shuttleengine.SessionState{Name: shuttleengine.SessionBusy, Cause: shuttleengine.SessionCauseTurn}
			p.states["g-d"] = shuttleengine.SessionState{Name: shuttleengine.SessionIdleStalled, Cause: shuttleengine.SessionCauseInterrupt}
			e := newEnv(t, p)

			report := revive(t, e, "")

			if !reflect.DeepEqual(report.Sent, []string{"a"}) || !reflect.DeepEqual(strands(report.Skipped), []string{"b", "c", "d"}) || !reflect.DeepEqual(p.conditioned, []string{"g-a"}) {
				t.Errorf("report %+v, sent to %v; want a sent and b, c and d skipped with one send", report, p.conditioned)
			}
			if report.Skipped[0].Reason != "busy (turn)" {
				t.Errorf("b's skip reason = %q; want the state read", report.Skipped[0].Reason)
			}
		}},
		{"a session a live entry covers is reported paused and sent nothing", func(t *testing.T) {
			p := newPair("p1", "a")
			p.states["g-a"] = stalled
			e := newEnv(t, p)
			now := e.clock.Now()
			e.seedLive(pauserecord.Entry{Scope: "p1", PausedAt: now, GraceEnd: now, Caller: "EXTERNAL"})

			report := revive(t, e, "")

			if !reflect.DeepEqual(strands(report.Paused), []string{"a"}) || len(p.conditioned) != 0 {
				t.Errorf("report %+v, sent to %v; want a paused and nothing sent", report, p.conditioned)
			}
		}},
		{"the send holds the record lock, so a pause added meanwhile waits and then interrupts the started turn", func(t *testing.T) {
			p := newPair("p1", "a")
			p.states["g-a"] = stalled
			e := newEnv(t, p)
			done := make(chan struct{})
			var pauseErr error
			p.onConditioned = func(string) {
				held, acquired, err := lock.TryAcquireWriteLock(e.path + ".lock")
				if err != nil {
					t.Errorf("TryAcquireWriteLock: %v", err)
				}
				if acquired {
					_ = held.Release()
					t.Errorf("the record lock was free during the conditioned send")
				}
				p.mu.Lock()
				p.busy["g-a"] = []bool{true}
				p.mu.Unlock()
				go func() {
					defer close(done)
					_, pauseErr = Pause(e.deps(), PauseRequest{})
				}()
			}

			report := revive(t, e, "")
			<-done

			if pauseErr != nil || !reflect.DeepEqual(report.Sent, []string{"a"}) {
				t.Fatalf("pause error %v, report %+v; want a sent and the pause to succeed", pauseErr, report)
			}
			live := e.record().Live
			if len(live) != 1 || len(live[0].Sessions) != 1 || live[0].Sessions[0].GUID != "g-a" || len(p.interrupts) == 0 {
				t.Errorf("live entries %+v, interrupts %v; want the pause's entry to record a and interrupt it", live, p.interrupts)
			}
		}},
		{"a line that landed with no turn start seen is reported as such", func(t *testing.T) {
			p := newPair("p1", "a")
			p.states["g-a"] = stalled
			p.noTurnStart["g-a"] = true
			e := newEnv(t, p)

			report := revive(t, e, "")

			if !reflect.DeepEqual(report.SentNoTurnStart, []string{"a"}) || len(report.Sent) != 0 {
				t.Errorf("report %+v; want a under sent_no_turn_start only", report)
			}
		}},
		{"a refused or unlanded send is reported with its reason and sent once", func(t *testing.T) {
			p := newPair("p1", "a", "b")
			p.states["g-a"], p.states["g-b"] = stalled, stalled
			p.conditionedErr["g-a"] = &shuttleengine.SessionBusyError{Reason: "the input box holds a draft", Err: shuttleengine.ErrSessionBusy}
			p.conditionedErr["g-b"] = fmt.Errorf("%w: strand g-b: the text stayed in the box", shuttleengine.ErrSubmissionNotLanded)
			e := newEnv(t, p)

			report := revive(t, e, "")

			if len(report.Refused) != 1 || report.Refused[0].Strand != "a" || report.Refused[0].Reason != "the input box holds a draft" {
				t.Errorf("refused = %+v; want a with the guard's reason", report.Refused)
			}
			if len(report.NotLanded) != 1 || report.NotLanded[0].Strand != "b" || !strings.Contains(report.NotLanded[0].Reason, "the text stayed in the box") {
				t.Errorf("not landed = %+v; want b with its reason", report.NotLanded)
			}
			if !reflect.DeepEqual(p.conditioned, []string{"g-a", "g-b"}) {
				t.Errorf("sent to %v; want each session once", p.conditioned)
			}
		}},
		{"a session that turned busy before the send is skipped, and a second revive sends nothing", func(t *testing.T) {
			p := newPair("p1", "a")
			p.states["g-a"] = stalled
			p.conditionedErr["g-a"] = fmt.Errorf("%w: strand g-a reads busy", shuttleengine.ErrSendConditionUnmet)
			e := newEnv(t, p)

			first := revive(t, e, "")

			if !reflect.DeepEqual(strands(first.Skipped), []string{"a"}) || len(p.conditioned) != 1 {
				t.Fatalf("first revive %+v, sent to %v; want a skipped after one send", first, p.conditioned)
			}
			p.states["g-a"] = shuttleengine.SessionState{Name: shuttleengine.SessionIdleDone, Cause: shuttleengine.SessionCauseDone}

			second := revive(t, e, "")

			if !reflect.DeepEqual(strands(second.Skipped), []string{"a"}) || len(p.conditioned) != 1 {
				t.Errorf("second revive %+v, sent to %v; want a skipped and no further send", second, p.conditioned)
			}
		}},
		{"a failed state reading refuses the session and sends nothing", func(t *testing.T) {
			p := newPair("p1", "a")
			p.stateErr["g-a"] = errors.New("events unreadable")
			e := newEnv(t, p)

			report := revive(t, e, "")

			if len(report.Refused) != 1 || !strings.Contains(report.Refused[0].Reason, "events unreadable") || len(p.conditioned) != 0 {
				t.Errorf("report %+v, sent to %v; want a refused with the error and nothing sent", report, p.conditioned)
			}
		}},
		{"a stalled orch gets one inject, a revive while it has no outcome injects nothing, and no other prime session is read or reported", func(t *testing.T) {
			prime := newPair("prime", "orch", "other")
			prime.states["g-orch"], prime.states["g-other"] = stalled, stalled
			e := newEnv(t, newPair("p1"))
			e.prime, e.orchStrand = prime, "g-orch"

			first := revive(t, e, "")
			if first.Orch == nil || first.Orch.Result != "injected" || first.Orch.InjectID != 1 || !reflect.DeepEqual(e.creel.injected(), []string{ReviveLine}) {
				t.Errorf("orch part %+v, injected %v; want one injected line with id 1", first.Orch, e.creel.injected())
			}
			if len(prime.conditioned) != 0 || len(first.Sent)+len(first.Skipped)+len(first.Refused) != 0 {
				t.Errorf("prime sent to %v, report %+v; want no prime session sent to or reported", prime.conditioned, first)
			}

			if again := revive(t, e, ""); again.Orch == nil || again.Orch.Result != "pending" || len(e.creel.injected()) != 1 {
				t.Errorf("orch part %+v, injected %v; want pending and no second inject", again.Orch, e.creel.injected())
			}
			if slugScope := revive(t, e, "p1"); slugScope.Orch != nil {
				t.Errorf("orch part %+v for a slug revive; want none", slugScope.Orch)
			}

			e.creel.settle()
			if settled := revive(t, e, ""); settled.Orch == nil || settled.Orch.Result != "injected" || settled.Orch.InjectID != 2 || len(e.creel.injected()) != 2 {
				t.Errorf("orch part %+v, injected %v; want a second inject with id 2 once the first had an outcome", settled.Orch, e.creel.injected())
			}

			e.creel.settle()
			e.seedLive(hubEntry(e.clock.Now()))
			if covered := revive(t, e, ""); covered.Orch == nil || covered.Orch.Result != "paused" || len(e.creel.injected()) != 2 {
				t.Errorf("orch part %+v, injected %v; want paused and nothing injected under a hub entry", covered.Orch, e.creel.injected())
			}
		}},
		{"an orch that reads another state is skipped with the state and nothing is injected", func(t *testing.T) {
			prime := newPair("prime", "orch")
			prime.states["g-orch"] = shuttleengine.SessionState{Name: shuttleengine.SessionBusy, Cause: shuttleengine.SessionCauseTurn}
			e := newEnv(t, newPair("p1"))
			e.prime, e.orchStrand = prime, "g-orch"

			report := revive(t, e, "")

			if report.Orch == nil || report.Orch.Result != "skipped" || report.Orch.Reason != "busy (turn)" || len(e.creel.injected()) != 0 {
				t.Errorf("orch part %+v, injected %v; want skipped with the state read and nothing injected", report.Orch, e.creel.injected())
			}
		}},
		{"two concurrent revives over a stalled orch inject exactly once", func(t *testing.T) {
			prime := newPair("prime", "orch")
			prime.states["g-orch"] = stalled
			e := newEnv(t, newPair("p1"))
			e.prime, e.orchStrand = prime, "g-orch"

			const revives = 2
			results := make([]string, revives)
			var wg sync.WaitGroup
			for i := range revives {
				wg.Add(1)
				go func() {
					defer wg.Done()
					report, err := Revive(e.deps(), ReviveRequest{})
					if err != nil || report.Orch == nil {
						t.Errorf("Revive() = %+v, %v; want an orch part", report, err)
						return
					}
					results[i] = report.Orch.Result
				}()
			}
			wg.Wait()

			if got := e.creel.injected(); len(got) != 1 {
				t.Errorf("injected %v; want exactly one inject", got)
			}
			slices.Sort(results)
			if !reflect.DeepEqual(results, []string{"injected", "pending"}) {
				t.Errorf("results = %v; want one injected and one pending", results)
			}
		}},
		{"a failed pending lookup or inject is returned naming the orch revive, and the lookup's failure injects nothing", func(t *testing.T) {
			prime := newPair("prime", "orch")
			prime.states["g-orch"] = stalled
			e := newEnv(t, newPair("p1"))
			e.prime, e.orchStrand = prime, "g-orch"

			e.creel.pendingErr = errors.New("creel unreachable")
			if _, err := Revive(e.deps(), ReviveRequest{}); err == nil || !strings.Contains(err.Error(), "revive: look up the orch's revive inject") || !strings.Contains(err.Error(), "creel unreachable") || len(e.creel.injected()) != 0 {
				t.Errorf("Revive() error = %v, injected %v; want the lookup failure and nothing injected", err, e.creel.injected())
			}

			e.creel.pendingErr, e.creel.injectErr = nil, errors.New("creel refused")
			if _, err := Revive(e.deps(), ReviveRequest{}); err == nil || !strings.Contains(err.Error(), "revive: inject the orch's carry-on line") || !strings.Contains(err.Error(), "creel refused") {
				t.Errorf("Revive() error = %v; want the inject failure", err)
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

	t.Run("the owners part carries the seam's answer, and a failed seam leaves the rest of the report intact", func(t *testing.T) {
		answer := []PairOwner{{Slug: "p1", Owner: "laptop", Local: true}, {Slug: "p2", Owner: "desktop", OwnedHere: false}, {Slug: "p3", Owner: "here", Local: true, OwnedHere: true}}
		tests := []struct {
			name      string
			owners    func() ([]PairOwner, error)
			want      []PairOwner
			wantError string
		}{
			{"Answer", func() ([]PairOwner, error) { return answer, nil }, answer, ""},
			{"NilSeam", nil, []PairOwner{}, ""},
			{"SeamError", func() ([]PairOwner, error) { return nil, errors.New("board unreadable") }, []PairOwner{}, "board unreadable"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e := newEnv(t, newPair("p1", "a"))
				e.seedLive(hubEntry(e.clock.Now()))
				deps := e.deps()
				deps.Owners = tt.owners

				report, err := Status(deps)

				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(report.Owners, tt.want) || report.OwnersError != tt.wantError {
					t.Errorf("owners = %+v, error = %q; want %+v, %q", report.Owners, report.OwnersError, tt.want, tt.wantError)
				}
				if len(report.Sessions) != 1 || len(report.Live) != 1 {
					t.Errorf("report = %+v; want sessions and live entries still read", report)
				}
			})
		}
	})

	t.Run("the processes part lists each registered process and marks the orphans", func(t *testing.T) {
		registeredAt := newFakeClock().Now().Add(-90 * time.Second)
		record := func(pid int, role string) runproc.Record {
			return runproc.Record{PID: pid, Role: role, RegisteredAt: registeredAt}
		}
		listed := func(pid int, cwd string) proc.Process { return proc.Process{PID: pid, Cwd: cwd} }
		tests := []struct {
			name      string
			runProcs  map[string][]runproc.Record
			worktrees []string
			processes []proc.Process
			want      map[int]bool
		}{
			{"SlugWithRegisteredWorktreeIsKept", map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop)}}, []string{"/hub/gone"}, nil, map[int]bool{10: false}},
			{"SlugWithNoWorktreeAndNoBattenIsOrphan", map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop)}}, []string{"/hub/other"}, nil, map[int]bool{10: true}},
			{"SlugWithLiveBattenIsKept", map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop), record(11, runproc.RoleBatten)}}, nil, nil, map[int]bool{10: false, 11: false}},
			{"RecordWorkingInsideAWorktreeIsKept", map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop), record(11, runproc.RoleDriver)}}, []string{"/hub/other"}, []proc.Process{listed(10, "/hub/other/sub"), listed(11, "/elsewhere")}, map[int]bool{10: false, 11: true}},
			{"HubOwnerIsNeverOrphan", map[string][]runproc.Record{runproc.HubOwner: {record(10, runproc.RoleSync)}}, nil, nil, map[int]bool{10: false}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e := newEnv(t)
				e.runProcs, e.worktrees, e.processes, e.executable = tt.runProcs, tt.worktrees, tt.processes, "/bin/lyx"

				report := read(t, e).Processes

				got := map[int]bool{}
				for _, owned := range report.Owned {
					got[owned.PID] = owned.Orphan
					if owned.AgeSeconds != 90 {
						t.Errorf("process %+v; want age 90 seconds", owned)
					}
				}
				if !reflect.DeepEqual(got, tt.want) || report.Error != "" {
					t.Errorf("orphans = %v, error = %q; want %v", got, report.Error, tt.want)
				}
			})
		}

		e := newEnv(t)
		e.runProcs = map[string][]runproc.Record{"b": {record(2, runproc.RoleLoop)}, "a": {record(1, runproc.RoleDriver)}}
		e.worktrees = []string{"/hub/a", "/hub/b"}
		want := []OwnedProcess{{Owner: "a", Role: runproc.RoleDriver, PID: 1, AgeSeconds: 90}, {Owner: "b", Role: runproc.RoleLoop, PID: 2, AgeSeconds: 90}}
		if got := read(t, e).Processes.Owned; !reflect.DeepEqual(got, want) {
			t.Errorf("owned = %+v; want %+v in owner order with owner, role, pid and age", got, want)
		}
	})

	t.Run("a failed record or worktree reading lands in the processes error and the rest stands", func(t *testing.T) {
		registeredAt := newFakeClock().Now()
		runProcs := map[string][]runproc.Record{"gone": {{PID: 10, Role: runproc.RoleLoop, RegisteredAt: registeredAt}}}
		tests := []struct {
			name         string
			runProcs     map[string][]runproc.Record
			runProcsErr  error
			worktreesErr error
			wantOwned    int
			wantError    string
		}{
			{"RunProcsFails", runProcs, errors.New("record unreadable"), nil, 0, "record unreadable"},
			{"WorktreesFail", runProcs, nil, errors.New("git unreadable"), 1, "git unreadable"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e := newEnv(t, newPair("p1", "a"))
				e.seedLive(hubEntry(e.clock.Now()))
				e.runProcs, e.runProcsErr, e.worktreesErr = tt.runProcs, tt.runProcsErr, tt.worktreesErr

				report := read(t, e)

				if report.Processes.Error != tt.wantError || len(report.Processes.Owned) != tt.wantOwned {
					t.Errorf("processes = %+v; want error %q and %d owned", report.Processes, tt.wantError, tt.wantOwned)
				}
				for _, owned := range report.Processes.Owned {
					if owned.Orphan {
						t.Errorf("process %+v read as an orphan with the worktrees unread", owned)
					}
				}
				if len(report.Sessions) != 1 || len(report.Live) != 1 {
					t.Errorf("report = %+v; want sessions and live entries still read", report)
				}
			})
		}
	})

	t.Run("unregistered lyx processes are those in a deleted hub directory outside every worktree that no record owns", func(t *testing.T) {
		e := newEnv(t)
		e.hubPath = "/home/u/hub"
		e.executable = "/home/u/go/bin/lyx"
		e.worktrees = []string{"/home/u/hub/live"}
		e.runProcs = map[string][]runproc.Record{"p": {{PID: 4, Role: runproc.RoleLoop, RegisteredAt: e.clock.Now()}}}
		listed := func(pid int, exe, cwd string) proc.Process {
			return proc.Process{PID: pid, StartTime: "t" + fmt.Sprint(pid), Exe: exe, Cwd: cwd, Argv: []string{"lyx", "x"}}
		}
		const lyx = "/home/u/go/bin/lyx"
		e.processes = []proc.Process{
			listed(1, lyx, "/home/u/hub/gone (deleted)"),
			listed(2, lyx, "/home/u/hub/live/sub (deleted)"),
			listed(3, lyx, "/home/u/other (deleted)"),
			listed(4, lyx, "/home/u/hub/gone (deleted)"),
			listed(5, lyx+" (deleted)", "/home/u/hub/gone (deleted)"),
			listed(6, lyx, "/home/u/hub/live"),
			listed(7, "/usr/bin/other", "/home/u/hub/gone (deleted)"),
		}

		report := read(t, e).Processes

		var pids []int
		for _, process := range report.Unregistered {
			pids = append(pids, process.PID)
		}
		if !report.UnregisteredSupported || !reflect.DeepEqual(pids, []int{1}) {
			t.Errorf("unregistered = %+v; want only pid 1 on a supported platform", report)
		}
	})

	t.Run("an unsupported process reading keeps the owned list and marks the unregistered part unsupported", func(t *testing.T) {
		e := newEnv(t)
		e.runProcs = map[string][]runproc.Record{"p": {{PID: 4, Role: runproc.RoleLoop, RegisteredAt: e.clock.Now()}}}
		e.processErr = fmt.Errorf("proc: %w", errors.ErrUnsupported)

		report := read(t, e).Processes

		if report.UnregisteredSupported || report.Error != "" || len(report.Owned) != 1 || len(report.Unregistered) != 0 {
			t.Errorf("processes = %+v; want the owned record listed, unregistered unsupported and no error", report)
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
