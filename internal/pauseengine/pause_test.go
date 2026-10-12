// pause_test.go drives the pause sequence over fake pairs, a fake clock whose Sleep advances it, and a temp record.
// It also holds the fakes resume_test.go shares.

package pauseengine

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakePair is a Pair whose sessions answer scripted busy readings.
type fakePair struct {
	mu       sync.Mutex
	slug     string
	sessions []SessionInfo
	// busy holds each session's scripted busy answers, consumed in order with the last repeating; a session with none reads idle.
	busy         map[string][]bool
	dead         map[string]bool
	interruptErr map[string]error
	sendErr      map[string]error
	producer     string
	running      bool
	// onInterrupt runs after an interrupt is recorded, outside the lock.
	onInterrupt func(guid string)

	interrupts []string
	sends      []string
}

// newPair builds a pair with one session per strand name, whose guid is "g-" and the name.
func newPair(slug string, strands ...string) *fakePair {
	p := &fakePair{slug: slug, busy: map[string][]bool{}, dead: map[string]bool{}, interruptErr: map[string]error{}, sendErr: map[string]error{}}
	for _, strand := range strands {
		p.sessions = append(p.sessions, SessionInfo{Strand: strand, GUID: "g-" + strand, Slug: slug})
	}
	return p
}

func (p *fakePair) Slug() string                     { return p.slug }
func (p *fakePair) Sessions() ([]SessionInfo, error) { return p.sessions, nil }
func (p *fakePair) Running() (string, bool, error)   { return p.producer, p.running, nil }
func (p *fakePair) Live(guid string) (bool, error)   { return !p.dead[guid], nil }

func (p *fakePair) Busy(guid string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	answers := p.busy[guid]
	if len(answers) == 0 {
		return false, nil
	}
	if len(answers) > 1 {
		p.busy[guid] = answers[1:]
	}
	return answers[0], nil
}

func (p *fakePair) Interrupt(guid string) error {
	p.mu.Lock()
	p.interrupts = append(p.interrupts, guid)
	err, hook := p.interruptErr[guid], p.onInterrupt
	p.mu.Unlock()
	if hook != nil {
		hook(guid)
	}
	return err
}

func (p *fakePair) Send(guid, text string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.sendErr[guid]; err != nil {
		return err
	}
	if text != pauserecord.ResumeLine {
		return fmt.Errorf("sent %q; want the resume line", text)
	}
	p.sends = append(p.sends, guid)
	return nil
}

// fakeClock is a clock whose Sleep advances it at once.
type fakeClock struct {
	mu      sync.Mutex
	start   time.Time
	now     time.Time
	onSleep func()
}

func newFakeClock() *fakeClock {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return &fakeClock{start: start, now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	hook := c.onSleep
	c.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (c *fakeClock) elapsed() time.Duration { return c.Now().Sub(c.start) }

// env is one test's world: pairs, a record, a clock and the told holds.
type env struct {
	t           *testing.T
	path        string
	clock       *fakeClock
	pairs       []*fakePair
	netOps      []pauserecord.NetOp
	locks       []parentlock.Entry
	watcherLive bool
	caller      string
}

func newEnv(t *testing.T, pairs ...*fakePair) *env {
	t.Helper()
	return &env{t: t, path: filepath.Join(t.TempDir(), "pause", pauserecord.RecordFileName), clock: newFakeClock(), pairs: pairs}
}

func (e *env) deps() Deps {
	var pairs []Pair
	for _, p := range e.pairs {
		pairs = append(pairs, p)
	}
	return Deps{
		RecordPath:      e.path,
		Pairs:           pairs,
		NetOps:          func() ([]pauserecord.NetOp, error) { return e.netOps, nil },
		Locks:           func() ([]parentlock.Entry, error) { return e.locks, nil },
		OrchWatcherLive: func() bool { return e.watcherLive },
		Caller:          e.caller,
		Now:             e.clock.Now,
		Sleep:           e.clock.Sleep,
	}
}

func (e *env) record() pauserecord.Record {
	e.t.Helper()
	record, err := pauserecord.Read(e.path)
	if err != nil {
		e.t.Fatal(err)
	}
	return record
}

func (e *env) pause(slug string, grace time.Duration) PauseReport {
	e.t.Helper()
	report, err := Pause(e.deps(), PauseRequest{Slug: slug, Grace: grace})
	if err != nil {
		e.t.Fatal(err)
	}
	return report
}

func (e *env) resume(slug string) (ResumeReport, error) {
	e.t.Helper()
	return Resume(e.deps(), ResumeRequest{Slug: slug})
}

// interrupts lists the guids every pair was interrupted on, in pair order.
func (e *env) interrupts() []string {
	var guids []string
	for _, p := range e.pairs {
		guids = append(guids, p.interrupts...)
	}
	return guids
}

// seedLive writes the entries as the record's live ones.
func (e *env) seedLive(entries ...pauserecord.Entry) {
	e.t.Helper()
	_, err := pauserecord.Update(e.path, e.clock.Now(), func(r *pauserecord.Record) error {
		r.Live = entries
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func hubEntry(at time.Time, sessions ...pauserecord.Session) pauserecord.Entry {
	return pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: at, GraceEnd: at, Caller: "EXTERNAL", Sessions: sessions}
}

func recorded(slug, strand string) pauserecord.Session {
	return pauserecord.Session{Strand: strand, GUID: "g-" + strand, Slug: slug}
}

func TestPause(t *testing.T) {
	orchStrand, err := agentname.Format("lyx", "", agentname.RoleOrch)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"the entry is on disk and the session recorded in it before the first Esc", func(t *testing.T) {
			p := newPair("p1", "a")
			// The readings are the grace's probe, the step's check, then the one after the Esc.
			p.busy["g-a"] = []bool{true, true, false}
			e := newEnv(t, p)
			var atEsc pauserecord.Record
			p.onInterrupt = func(string) { atEsc = e.record() }

			report := e.pause("", 0)

			if len(atEsc.Live) != 1 || !reflect.DeepEqual(atEsc.Live[0].Sessions, []pauserecord.Session{recorded("p1", "a")}) {
				t.Errorf("record at the first Esc = %+v; want the hub entry owning the session", atEsc)
			}
			if !reflect.DeepEqual(report.Interrupted, []string{"a"}) || report.Scope != pauserecord.ScopeHub {
				t.Errorf("report = %+v; want scope hub and a interrupted", report)
			}
		}},
		{"the grace waits for a busy session and never past the grace end", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)

			e.pause("", 10*time.Second)

			if got := e.clock.elapsed(); got != 10*time.Second {
				t.Errorf("clock ran %s; want the whole 10s grace", got)
			}
		}},
		{"the grace waits for a covered network operation and ends when it finishes", func(t *testing.T) {
			p := newPair("p1", "a")
			e := newEnv(t, p)
			e.netOps = []pauserecord.NetOp{{Op: "fetch", Slug: "p1", Strand: "a"}}
			e.clock.onSleep = func() {
				if e.clock.elapsed() >= 3*time.Second {
					e.netOps = nil
				}
			}

			report := e.pause("", 10*time.Second)

			if got := e.clock.elapsed(); got != 3*time.Second || len(report.NetOps) != 0 {
				t.Errorf("clock ran %s with net ops %+v; want 3s and none left", got, report.NetOps)
			}
		}},
		{"a busy session is interrupted at most twice and reported still busy", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if !reflect.DeepEqual(p.interrupts, []string{"g-a", "g-a"}) || !reflect.DeepEqual(report.StillBusy, []string{"a"}) || len(report.Interrupted) != 0 {
				t.Errorf("interrupts %v and report %+v; want two Esc and a still busy", p.interrupts, report)
			}
		}},
		{"the caller's own strand is skipped", func(t *testing.T) {
			p := newPair("p1", "a", "b")
			p.busy["g-a"], p.busy["g-b"] = []bool{true}, []bool{true, false}
			e := newEnv(t, p)
			e.caller = "a"

			report := e.pause("", 0)

			if !reflect.DeepEqual(p.interrupts, []string{"g-b"}) || len(report.Skipped) != 1 || report.Skipped[0].Strand != "a" {
				t.Errorf("interrupts %v and skipped %+v; want only b interrupted and a skipped", p.interrupts, report.Skipped)
			}
		}},
		{"nothing running writes the entry with empty lists", func(t *testing.T) {
			e := newEnv(t)
			e.caller = "ignored"

			report := e.pause("", 0)

			record := e.record()
			if len(record.Live) != 1 || record.Live[0].Caller != "ignored" || len(record.Live[0].Sessions) != 0 {
				t.Errorf("record = %+v; want one entry with no sessions", record)
			}
			if len(report.Interrupted)+len(report.StillBusy)+len(report.GoProducers)+len(report.NetOps) != 0 {
				t.Errorf("report = %+v; want empty lists", report)
			}
		}},
		{"a repeated pause adds no entry and interrupts only sessions busy now", func(t *testing.T) {
			p := newPair("p1", "a", "b")
			p.busy["g-a"], p.busy["g-b"] = []bool{true, true, false}, []bool{false}
			e := newEnv(t, p)
			e.pause("", 0)

			p.busy["g-a"], p.busy["g-b"] = []bool{false}, []bool{true, true, false}
			report := e.pause("", 0)

			if !report.AlreadyPaused || len(e.record().Live) != 1 {
				t.Errorf("already paused %v, live entries %d; want a repeat that adds none", report.AlreadyPaused, len(e.record().Live))
			}
			if want := []string{"g-a", "g-b"}; !reflect.DeepEqual(p.interrupts, want) {
				t.Errorf("interrupts = %v; want %v, one per pause", p.interrupts, want)
			}
		}},
		{"an entry resumed during the grace sends no Esc and the report says so", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)
			e.clock.onSleep = func() {
				if _, err := e.resume(""); err != nil {
					t.Error(err)
				}
			}

			report := e.pause("", 10*time.Second)

			if len(p.interrupts) != 0 || !report.ResumedDuringGrace {
				t.Errorf("interrupts %v, resumed during grace %v; want no Esc and the flag", p.interrupts, report.ResumedDuringGrace)
			}
		}},
		{"a slug pause under a live hub entry owns only the sessions no entry owns", func(t *testing.T) {
			p := newPair("p1", "a", "b")
			p.busy["g-a"], p.busy["g-b"] = []bool{true}, []bool{true, false}
			e := newEnv(t, p)
			e.seedLive(hubEntry(e.clock.Now(), recorded("p1", "a")))

			report := e.pause("p1", 0)

			if !reflect.DeepEqual(p.interrupts, []string{"g-b"}) || len(report.Skipped) != 1 || report.Skipped[0].Strand != "a" {
				t.Errorf("interrupts %v and skipped %+v; want b interrupted and a skipped as owned", p.interrupts, report.Skipped)
			}
			var slugOwned []pauserecord.Session
			for _, entry := range e.record().Live {
				if entry.Scope == "p1" {
					slugOwned = entry.Sessions
				}
			}
			if !reflect.DeepEqual(slugOwned, []pauserecord.Session{recorded("p1", "b")}) {
				t.Errorf("slug entry sessions = %+v; want only b", slugOwned)
			}
		}},
		{"a pane refusal is reported as refused", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.interruptErr["g-a"] = fmt.Errorf("%w: strand g-a", shuttleengine.ErrPaneNotReady)
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(report.Refused) != 1 || report.Refused[0].Strand != "a" || len(report.StillBusy) != 0 {
				t.Errorf("report = %+v; want a refused", report)
			}
		}},
		{"the report names network operations, held branches, waiters, Go producers and the slug-only holds", func(t *testing.T) {
			world := func() *env {
				p2 := newPair("p2")
				p2.producer, p2.running = "Plan-Write", true
				e := newEnv(t, newPair("p1", "a"), p2)
				e.netOps = []pauserecord.NetOp{
					{Op: "fetch", Strand: "x", Slug: "p1"},
					{Op: "push", Strand: "y", Slug: "p2", UnderParentLock: true},
				}
				base := e.clock.Now()
				e.locks = []parentlock.Entry{{
					Record: parentlock.Record{Branch: "main", Owner: parentlock.Owner{Slug: "p1"}},
					Waiters: []parentlock.Ticket{
						{Owner: parentlock.Owner{Slug: "p3"}, QueuedAt: base.Add(2 * time.Second)},
						{Owner: parentlock.Owner{Slug: "p2"}, QueuedAt: base.Add(time.Second)},
					},
				}}
				return e
			}

			report := world().pause("", 0)

			wantNet := []NetOpReport{{Op: "fetch", Strand: "x", Slug: "p1"}, {Op: "push", Strand: "y", Slug: "p2", UnderParentLock: true}}
			if !reflect.DeepEqual(report.NetOps, wantNet) {
				t.Errorf("net ops = %+v; want %+v", report.NetOps, wantNet)
			}
			wantHolds := holdReport{
				Held:    []HeldBranch{{Branch: "main", Slug: "p1", Queued: []string{"p2", "p3"}}},
				Waiting: []WaitingRun{{Slug: "p2", Branch: "main", Behind: []string{"p3"}}, {Slug: "p3", Branch: "main", Behind: []string{}}},
			}
			if !reflect.DeepEqual(report.Holds, wantHolds) {
				t.Errorf("holds = %+v; want %+v", report.Holds, wantHolds)
			}
			if want := []GoProducer{{Slug: "p2", Producer: "Plan-Write"}}; !reflect.DeepEqual(report.GoProducers, want) {
				t.Errorf("go producers = %+v; want %+v", report.GoProducers, want)
			}

			slugOnly := world().pause("p1", 0)
			if len(slugOnly.Holds.Waiting) != 0 || len(slugOnly.NetOps) != 1 || len(slugOnly.GoProducers) != 0 {
				t.Errorf("slug pause report = %+v; want p1's holds and operations only", slugOnly)
			}
		}},
		{"the report names the orch's case under a hub entry and none under a slug entry", func(t *testing.T) {
			cases := []struct {
				caller      string
				watcherLive bool
				slug        string
				want        OrchCase
			}{
				{orchStrand, true, "", OrchRanPause},
				{"", true, "", OrchDelegated},
				{"", false, "", OrchNotInterrupted},
				{"", true, "p1", ""},
			}
			for _, tc := range cases {
				e := newEnv(t, newPair("p1"))
				e.caller, e.watcherLive = tc.caller, tc.watcherLive
				if got := e.pause(tc.slug, 0).Orch; got != tc.want {
					t.Errorf("caller %q, watcher %v, slug %q: orch case = %q; want %q", tc.caller, tc.watcherLive, tc.slug, got, tc.want)
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPause_RacingResumeLeavesADecodableRecord(t *testing.T) {
	p := newPair("p1", "a")
	p.busy["g-a"] = []bool{true}
	e := newEnv(t, p)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := Pause(e.deps(), PauseRequest{}); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		for {
			report, err := e.resume("")
			if err != nil {
				t.Error(err)
				return
			}
			if len(report.Resumed) > 0 {
				return
			}
		}
	}()
	wg.Wait()

	record := e.record()
	if len(record.Live) != 0 || len(record.Resumed) != 1 {
		t.Errorf("record = %+v; want the one entry resumed", record)
	}
	if len(p.interrupts) > maxInterrupts {
		t.Errorf("interrupts = %v; want at most %d", p.interrupts, maxInterrupts)
	}
}
