// pause_test.go drives the pause sequence over fake pairs, a fake clock whose Sleep advances it, and a temp record.
// It also holds the fakes resume_test.go shares.

package tenderengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakePair is a Pair whose sessions answer scripted busy readings.
type fakePair struct {
	mu       sync.Mutex
	slug     string
	sessions []SessionInfo
	// busy holds each session's scripted busy answers, consumed in order with the last repeating; a session with none reads idle.
	busy map[string][]bool
	dead map[string]bool
	// interruptErrs holds each session's scripted interrupt errors, consumed in order with the last repeating; an empty list answers nil.
	interruptErrs map[string][]error
	sendErr       map[string]error
	// background holds the labels of each session whose turn ended with background work; a key present reads as background.
	background map[string][]string
	// backgroundErr and paneErr fail Background and PaneRunningTurn for a key present.
	backgroundErr map[string]error
	paneErr       map[string]error
	// paneRunning is what PaneRunningTurn answers per session.
	paneRunning map[string]bool
	// states holds each session's scripted state; a session with none reads idle-done.
	states map[string]shuttleengine.SessionState
	// stateErr fails State for a key present.
	stateErr map[string]error
	// conditionedErr fails SendConditioned for a key present, and noTurnStart makes it land with no turn start seen.
	conditionedErr map[string]error
	noTurnStart    map[string]bool
	// onConditioned runs inside SendConditioned before it returns, outside the lock.
	onConditioned func(guid string)
	// settle is the InterruptSettle answer.
	settle   time.Duration
	producer string
	running  bool
	// onInterrupt runs after an interrupt is recorded, outside the lock.
	onInterrupt func(guid string)
	// onPaneRead runs after a pane reading is counted, outside the lock.
	onPaneRead func()

	interrupts []string
	sends      []string
	// conditioned holds the guids SendConditioned sent the revive line to.
	conditioned []string
	paneReads   int
}

// newPair builds a pair with one session per strand name, whose guid is "g-" and the name, and a 2 s interrupt settle.
func newPair(slug string, strands ...string) *fakePair {
	p := &fakePair{
		slug: slug, busy: map[string][]bool{}, dead: map[string]bool{}, interruptErrs: map[string][]error{}, sendErr: map[string]error{},
		background: map[string][]string{}, backgroundErr: map[string]error{}, paneErr: map[string]error{}, paneRunning: map[string]bool{},
		states: map[string]shuttleengine.SessionState{}, stateErr: map[string]error{}, conditionedErr: map[string]error{}, noTurnStart: map[string]bool{},
		settle: 2 * time.Second,
	}
	for _, strand := range strands {
		p.sessions = append(p.sessions, SessionInfo{Strand: strand, GUID: "g-" + strand, Slug: slug})
	}
	return p
}

func (p *fakePair) Slug() string                     { return p.slug }
func (p *fakePair) Sessions() ([]SessionInfo, error) { return p.sessions, nil }
func (p *fakePair) Running() (string, bool, error)   { return p.producer, p.running, nil }
func (p *fakePair) Live(guid string) (bool, error)   { return !p.dead[guid], nil }

func (p *fakePair) State(guid string) (shuttleengine.SessionState, error) {
	if err := p.stateErr[guid]; err != nil {
		return shuttleengine.SessionState{}, err
	}
	if state, scripted := p.states[guid]; scripted {
		return state, nil
	}
	return shuttleengine.SessionState{Name: shuttleengine.SessionIdleDone, Cause: shuttleengine.SessionCauseDone}, nil
}

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

func (p *fakePair) Background(guid string) ([]string, bool, error) {
	if err := p.backgroundErr[guid]; err != nil {
		return nil, false, err
	}
	labels, background := p.background[guid]
	return labels, background, nil
}

func (p *fakePair) PaneRunningTurn(guid string) (bool, error) {
	p.mu.Lock()
	p.paneReads++
	running, err, hook := p.paneRunning[guid], p.paneErr[guid], p.onPaneRead
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	return running, err
}

func (p *fakePair) InterruptSettle() time.Duration { return p.settle }

func (p *fakePair) Interrupt(guid string) error {
	p.mu.Lock()
	p.interrupts = append(p.interrupts, guid)
	var err error
	if answers := p.interruptErrs[guid]; len(answers) > 0 {
		err = answers[0]
		if len(answers) > 1 {
			p.interruptErrs[guid] = answers[1:]
		}
	}
	hook := p.onInterrupt
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

func (p *fakePair) SendConditioned(guid, text string, want shuttleengine.SendCondition) (bool, error) {
	p.mu.Lock()
	err, noTurnStart, hook := p.conditionedErr[guid], p.noTurnStart[guid], p.onConditioned
	if text != ReviveLine {
		err = fmt.Errorf("sent %q; want the revive line", text)
	} else {
		p.conditioned = append(p.conditioned, guid)
	}
	p.mu.Unlock()
	if hook != nil {
		hook(guid)
	}
	return err == nil && !noTurnStart, err
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
	// prime is the prime's pair, left out of the deps when nil.
	prime *fakePair
	// processes and processErr are what the process reading answers.
	processes  []proc.Process
	processErr error
	hubPath    string
	executable string
	// orchStrand is the orch strand the prime's state records; orchQueued holds each notice the queue accepted, and orchAlreadyQueued makes it refuse as a duplicate.
	orchStrand        string
	orchQueued        []string
	orchAlreadyQueued bool
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
	deps := Deps{
		RecordPath: e.path,
		Pairs:      pairs,
		Processes:  func() ([]proc.Process, error) { return e.processes, e.processErr },
		HubPath:    e.hubPath,
		Executable: e.executable,
		OrchStrand: func() (string, error) { return e.orchStrand, nil },
		QueueOrchRevive: func(line string) (bool, error) {
			if e.orchAlreadyQueued {
				return false, nil
			}
			e.orchQueued = append(e.orchQueued, line)
			return true, nil
		},
		NetOps:          func() ([]pauserecord.NetOp, error) { return e.netOps, nil },
		Locks:           func() ([]parentlock.Entry, error) { return e.locks, nil },
		OrchWatcherLive: func() bool { return e.watcherLive },
		Caller:          e.caller,
		Now:             e.clock.Now,
		Sleep:           e.clock.Sleep,
	}
	// A nil *fakePair in the interface would read as a prime.
	if e.prime != nil {
		deps.Prime = e.prime
	}
	return deps
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
			if !reflect.DeepEqual(report.Interrupted, []string{"a"}) || report.Scope != pauserecord.ScopeHub || len(p.interrupts) != 1 {
				t.Errorf("report = %+v with %d Esc; want scope hub and a interrupted by the first Esc alone", report, len(p.interrupts))
			}
		}},
		{"Added is called once with the added entry, before any Esc and before the grace, and not for a repeated pause", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)
			var got []pauserecord.Entry
			var recordAtCall pauserecord.Record
			var interruptsAtCall int
			var elapsedAtCall time.Duration
			added := func(entry pauserecord.Entry) error {
				got = append(got, entry)
				recordAtCall, interruptsAtCall, elapsedAtCall = e.record(), len(p.interrupts), e.clock.elapsed()
				return nil
			}

			first, err := Pause(e.deps(), PauseRequest{Grace: 10 * time.Second, Added: added})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Pause(e.deps(), PauseRequest{Added: added}); err != nil {
				t.Fatal(err)
			}

			if len(got) != 1 || got[0].Scope != pauserecord.ScopeHub || !got[0].PausedAt.Equal(first.PausedAt) {
				t.Fatalf("added entries = %+v; want the one hub entry the first pause added", got)
			}
			if len(recordAtCall.Live) != 1 || !recordAtCall.Live[0].PausedAt.Equal(got[0].PausedAt) || interruptsAtCall != 0 || elapsedAtCall != 0 {
				t.Errorf("at the call: record %+v, %d Esc, clock %s; want the entry recorded, no Esc and the clock unmoved", recordAtCall, interruptsAtCall, elapsedAtCall)
			}
		}},
		{"an Added error ends the pause before any Esc and leaves the entry live", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)
			boom := errors.New("boom")

			_, err := Pause(e.deps(), PauseRequest{Added: func(pauserecord.Entry) error { return boom }})

			if !errors.Is(err, boom) || len(p.interrupts) != 0 {
				t.Errorf("err %v, interrupts %v; want the wrapped error and no Esc", err, p.interrupts)
			}
			if live := e.record().Live; len(live) != 1 {
				t.Errorf("live entries = %+v; want the entry still live", live)
			}
		}},
		{"the grace waits for a busy session and never past the grace end", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			// The session stays busy through both keys, and the row pins the clock at exactly the grace.
			p.settle = 0
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
		{"busy sessions are interrupted at most twice, share each settle and are reported still busy", func(t *testing.T) {
			pa := newPair("p1", "a")
			pb := newPair("p2", "b")
			pa.busy["g-a"], pb.busy["g-b"] = []bool{true}, []bool{true}
			pa.paneRunning["g-a"], pb.paneRunning["g-b"] = true, true
			e := newEnv(t, pa, pb)
			var keyTimes, paneTimes []time.Duration
			hookInterrupt := func(string) { keyTimes = append(keyTimes, e.clock.elapsed()) }
			hookPane := func() { paneTimes = append(paneTimes, e.clock.elapsed()) }
			pa.onInterrupt, pb.onInterrupt = hookInterrupt, hookInterrupt
			pa.onPaneRead, pb.onPaneRead = hookPane, hookPane

			report := e.pause("", 0)

			if got := e.interrupts(); !reflect.DeepEqual(got, []string{"g-a", "g-a", "g-b", "g-b"}) || !reflect.DeepEqual(report.StillBusy, []string{"a", "b"}) || len(report.Interrupted) != 0 {
				t.Errorf("interrupts %v and report %+v; want two Esc each and both still busy", got, report)
			}
			// The keys of a pass share one settle: [first a, first b, second a, second b], then the pane reads.
			wantKeys := []time.Duration{0, 0, 2 * time.Second, 2 * time.Second}
			if !reflect.DeepEqual(keyTimes, wantKeys) {
				t.Errorf("key times = %v; want %v, each second key one settle after the first", keyTimes, wantKeys)
			}
			if want := []time.Duration{4 * time.Second, 4 * time.Second}; !reflect.DeepEqual(paneTimes, want) {
				t.Errorf("pane read times = %v; want %v, one settle after the second keys", paneTimes, want)
			}
			if got := e.clock.elapsed(); got != 4*time.Second {
				t.Errorf("clock ran %s; want exactly 2 settles", got)
			}
		}},
		{"a second Esc whose settle leaves no running turn on the pane is interrupted", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(p.interrupts) != 2 || !reflect.DeepEqual(report.Interrupted, []string{"a"}) || len(report.StillBusy) != 0 {
				t.Errorf("interrupts %v and report %+v; want two Esc and a interrupted", p.interrupts, report)
			}
		}},
		{"a guarded second Esc that finds no running turn ends interrupted", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.interruptErrs["g-a"] = []error{nil, fmt.Errorf("%w: strand g-a", shuttleengine.ErrNoRunningTurn)}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(p.interrupts) != 2 || !reflect.DeepEqual(report.Interrupted, []string{"a"}) || p.paneReads != 0 {
				t.Errorf("interrupts %v, pane reads %d and report %+v; want two Esc, a interrupted and no pane reading", p.interrupts, p.paneReads, report)
			}
		}},
		{"an open rewind picker found by the guarded second Esc is refused", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.interruptErrs["g-a"] = []error{nil, fmt.Errorf("%w: strand g-a: one interrupt key closed it", shuttleengine.ErrRewindPickerOpen)}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(p.interrupts) != 2 || len(report.Refused) != 1 || !strings.Contains(report.Refused[0].Reason, "rewind picker was open") {
				t.Errorf("interrupts %v and report %+v; want two Esc and a refused for the picker", p.interrupts, report)
			}
		}},
		{"a running state whose first guarded Esc finds no running turn is no_running_turn and stays recorded", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.interruptErrs["g-a"] = []error{fmt.Errorf("%w: strand g-a", shuttleengine.ErrNoRunningTurn)}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(p.interrupts) != 1 || !reflect.DeepEqual(report.NoRunningTurn, []string{"a"}) || len(report.Interrupted)+len(report.AlreadyIdle) != 0 {
				t.Errorf("interrupts %v and report %+v; want one Esc and a no_running_turn", p.interrupts, report)
			}
			if live := e.record().Live; len(live) != 1 || !reflect.DeepEqual(live[0].Sessions, []pauserecord.Session{recorded("p1", "a")}) {
				t.Errorf("live entries = %+v; want the session still recorded", live)
			}
		}},
		{"a background session holds no grace, gets no Esc, is not recorded and is listed with its tasks", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{false}
			p.background["g-a"] = []string{"sleep 600", "lyx shed step"}
			e := newEnv(t, p)

			report := e.pause("", 10*time.Second)

			want := []BackgroundSession{{Strand: "a", Slug: "p1", Tasks: []string{"sleep 600", "lyx shed step"}}}
			if !reflect.DeepEqual(report.Background, want) || len(p.interrupts) != 0 || e.clock.elapsed() != 0 {
				t.Errorf("report %+v, interrupts %v, clock %s; want the background listed, no Esc and no grace", report, p.interrupts, e.clock.elapsed())
			}
			if live := e.record().Live; len(live) != 1 || len(live[0].Sessions) != 0 {
				t.Errorf("live entries = %+v; want one with no sessions", live)
			}
		}},
		{"an open rewind picker found by the guarded Esc is refused with its message", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.interruptErrs["g-a"] = []error{fmt.Errorf("%w: strand g-a: one interrupt key closed it", shuttleengine.ErrRewindPickerOpen)}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(p.interrupts) != 1 || len(report.Refused) != 1 || !strings.Contains(report.Refused[0].Reason, "rewind picker was open") {
				t.Errorf("interrupts %v and report %+v; want one Esc and a refused for the picker", p.interrupts, report)
			}
		}},
		{"a failed guard capture is refused", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			p.interruptErrs["g-a"] = []error{errors.New("shuttle: capture strand \"g-a\"'s pane before the interrupt: boom")}
			e := newEnv(t, p)

			report := e.pause("", 0)

			if len(report.Refused) != 1 || !strings.Contains(report.Refused[0].Reason, "boom") {
				t.Errorf("report = %+v; want a refused naming the capture failure", report)
			}
		}},
		{"a resume during the settle sends no second Esc", func(t *testing.T) {
			p := newPair("p1", "a")
			p.busy["g-a"] = []bool{true}
			e := newEnv(t, p)
			p.onInterrupt = func(string) {
				e.clock.onSleep = func() {
					if _, err := e.resume(""); err != nil {
						t.Error(err)
					}
				}
			}

			report := e.pause("", 0)

			if len(p.interrupts) != 1 || !report.ResumedDuringGrace {
				t.Errorf("interrupts %v, resumed during grace %v; want one Esc and the flag", p.interrupts, report.ResumedDuringGrace)
			}
		}},
		{"a failed background or pane reading is refused with its prefix", func(t *testing.T) {
			p := newPair("p1", "a", "b")
			boom := errors.New("boom")
			p.busy["g-a"], p.busy["g-b"] = []bool{false}, []bool{true}
			p.backgroundErr["g-a"] = boom
			p.paneErr["g-b"] = boom
			e := newEnv(t, p)

			report := e.pause("", 0)

			if !reflect.DeepEqual(p.interrupts, []string{"g-b", "g-b"}) || len(report.Refused) != 2 {
				t.Fatalf("interrupts %v and report %+v; want only b interrupted, twice, and two refused", p.interrupts, report)
			}
			if got := report.Refused[0]; got.Strand != "a" || !strings.HasPrefix(got.Reason, "background reading failed: ") {
				t.Errorf("first refusal = %+v; want a's background reading", got)
			}
			if got := report.Refused[1]; got.Strand != "b" || !strings.HasPrefix(got.Reason, "pane reading failed: ") {
				t.Errorf("second refusal = %+v; want b's pane reading", got)
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
			p.interruptErrs["g-a"] = []error{fmt.Errorf("%w: strand g-a", shuttleengine.ErrPaneNotReady)}
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
