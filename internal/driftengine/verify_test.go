//go:build tier0

// verify_test.go — tier 0 tests for the Drift-Verify producer over the fake Deps the baseline tests use, a scripted compare and a seat runner that evaluates the agent's gate the way the run loop does.

package driftengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// fakeChair is a live chair handle that does nothing.
type fakeChair struct{}

func (fakeChair) StrandGUID() string { return "guid" }
func (fakeChair) StrandName() string { return "ly:task:drift-verify" }
func (fakeChair) RunDir() string     { return "" }
func (fakeChair) SessionState() (shuttleengine.RunSessionState, error) {
	return shuttleengine.RunSessionState{}, nil
}
func (fakeChair) Wait() (shuttleengine.Result, error) { return shuttleengine.Result{}, nil }
func (fakeChair) Send(string) error                   { return nil }
func (fakeChair) Stop() error                         { return nil }

// gateLoopSeats returns a seat runner whose run and resume evaluate the table's gate entries in order, as the run loop does at each turn end.
// Before each arrival it runs the next scripted agent action, a commit, an Accept or dirt.
// A failing entry with attempts left is re-prompted and arrives again; a terminal result or a spent entry ends the run.
// The result's chair carries a GateOutcome with each entry's state and attempts, as shuttle reports it.
func gateLoopSeats(t *testing.T, actions []func()) *shedfake.SeatRunner {
	t.Helper()
	arrival := 0
	loop := func(table seatengine.Table) (seatengine.Result, error) {
		attempts := make([]int, len(table.Gate))
		for {
			if arrival < len(actions) && actions[arrival] != nil {
				actions[arrival]()
			}
			arrival++

			entries := make([]shuttleengine.GateEntryOutcome, len(table.Gate))
			failed := -1
			var failure shuttleengine.GateResult
			for i, entry := range table.Gate {
				entries[i] = shuttleengine.GateEntryOutcome{Name: entry.Name, Attempts: attempts[i]}
				switch {
				case entry.Attempts <= 0:
					entries[i].State = shuttleengine.GateEntryOff
				case failed >= 0:
					entries[i].State = shuttleengine.GateEntryNotReached
				default:
					result, err := entry.Gate()
					if err != nil {
						return seatengine.Result{}, err
					}
					entries[i].State = shuttleengine.GateEntryPassed
					if !result.Passed {
						entries[i].State = shuttleengine.GateEntryFailed
						failed, failure = i, result
					}
				}
			}

			total := 0
			for _, n := range attempts {
				total += n
			}
			outcome := &shuttleengine.GateOutcome{Passed: failed < 0, Entries: entries, Attempts: total}
			if failed >= 0 {
				outcome.FindingsPath, outcome.Findings = "gate-findings.md", failure.Findings
				if failure.Terminal {
					outcome.Reason = failure.Findings
				}
			}
			if failed < 0 || failure.Terminal || attempts[failed] >= table.Gate[failed].Attempts {
				return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: outcome}, ChairOutputs: table.Chair().Outputs}, nil
			}
			attempts[failed]++
		}
	}
	return &shedfake.SeatRunner{
		RunFn:    loop,
		ResumeFn: func(table seatengine.Table, _ seatengine.LiveTable) (seatengine.Result, error) { return loop(table) },
	}
}

// verifyFixture is the fake world one DriftVerify pass runs in.
type verifyFixture struct {
	*baselineFixture

	history   []shedengine.HistoryEntry
	compares  []CompareResult
	verifyFn  func() (shuttleengine.GateResult, error)
	liveChair bool
	events    []string
	told      []VerifyTold

	compareCalls int
	verifyCalls  int
}

var (
	passingCompare = CompareResult{Passed: true}
	failingCompare = CompareResult{ExitCode: 1, Log: "compare-1.log", Reason: "exit 1; log compare-1.log"}
)

// newVerifyFixture returns a fixture in a loom run with a counted capture at HEAD, a declared compare and a scope record of scope.
func newVerifyFixture(t *testing.T, scope ScopeRecord, actions []func()) *verifyFixture {
	t.Helper()
	f := &verifyFixture{baselineFixture: newBaselineFixture(t), compares: []CompareResult{passingCompare}}
	f.verifyFn = func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil }
	if scope.Scope != "" {
		scope.AwaitedAt = "2026-01-01T00:00:00Z"
		f.seedScopeRecord(scope, false)
	}
	f.writeCapture(CaptureRecord{State: CapturePassed, Head: "aaa"})

	f.seats = gateLoopSeats(t, actions)
	f.seats.ProbeFn = func(seatengine.Table) (seatengine.LiveTable, error) {
		f.events = append(f.events, "probe")
		if f.liveChair {
			return seatengine.LiveTable{Chair: fakeChair{}}, nil
		}
		return seatengine.LiveTable{}, nil
	}
	loopRun, loopResume := f.seats.RunFn, f.seats.ResumeFn
	f.seats.RunFn = func(table seatengine.Table) (seatengine.Result, error) {
		f.events = append(f.events, "run")
		return loopRun(table)
	}
	f.seats.ResumeFn = func(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error) {
		f.events = append(f.events, "resume")
		return loopResume(table, live)
	}

	d := &f.deps
	d.Config = func() (Config, error) {
		return Config{Compare: CommandStep{Command: "compare {baseline_dir}", Dir: ".", TimeoutMin: 1}}, nil
	}
	d.Seats = f.seats
	d.Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 700_000_000, time.UTC) }
	d.GateFindingsFile = filepath.Join(t.TempDir(), "gate-findings.md")
	d.Status = func() (shedengine.Status, error) {
		return shedengine.Status{CurrentProducer: "Drift-Verify", State: shedengine.StateRunning, History: f.history}, nil
	}
	dirty := d.DirtyPaths
	d.DirtyPaths = func() ([]string, error) {
		f.events = append(f.events, "dirty")
		return dirty()
	}
	d.VerifyTable = func(told VerifyTold) (seatengine.Table, error) {
		f.told = append(f.told, told)
		return seatengine.Table{Seats: []seatengine.Seat{{Outputs: []string{f.paths.VerifyReport}}}}, nil
	}
	d.Compare = func(context.Context) (CompareResult, error) {
		f.events = append(f.events, "compare")
		result := f.compares[min(f.compareCalls, len(f.compares)-1)]
		f.compareCalls++
		return result, nil
	}
	return f
}

// call runs one DriftVerify pass.
func (f *verifyFixture) call() (shedengine.Outcome, shedengine.OutputPointer, error) {
	verify := shuttleengine.GateEntry{Name: "verify", Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		f.events = append(f.events, "verify")
		f.verifyCalls++
		return f.verifyFn()
	}}
	return NewDriftVerify("Drift-Verify", Row{}, verify, f.deps).Call(context.Background())
}

// scope reads the scope record.
func (f *verifyFixture) scope() ScopeRecord {
	f.t.Helper()
	scope, _ := mustReadScope(f.t, f.paths)
	return scope
}

// stuckHistory sets the run's history to one Stuck of this row at the given times.
func (f *verifyFixture) stuckHistory(at, lastAt string) {
	f.history = []shedengine.HistoryEntry{{Producer: "Drift-Verify", Outcome: shedengine.Stuck, At: at, LastAt: lastAt}}
}

// agentAccept is the action of the agent accepting test t1.
func (f *verifyFixture) agentAccept() func() {
	return func() {
		if err := Accept(f.paths, statusAt("Drift-Verify", shedengine.StateRunning, "", true), "t1", "intended"); err != nil {
			f.t.Fatal(err)
		}
	}
}

// commit is the action of the agent committing a fix.
func (f *verifyFixture) commit() func() { return func() { f.head = "bbb" } }

// requireStuck checks the outcome is Stuck and the reason names every fragment; prefixed says whether it opens with the decision halt prefix.
func requireStuck(t *testing.T, outcome shedengine.Outcome, pointer shedengine.OutputPointer, err error, prefixed bool, fragments ...string) {
	t.Helper()
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck", outcome, err)
	}
	if got := strings.HasPrefix(pointer.Reason, shedadapters.DecisionHaltPrefix); got != prefixed {
		t.Errorf("reason %q opens with the decision halt prefix = %v; want %v", pointer.Reason, got, prefixed)
	}
	for _, fragment := range fragments {
		if !strings.Contains(pointer.Reason, fragment) {
			t.Errorf("reason %q does not name %q", pointer.Reason, fragment)
		}
	}
}

// requireDone runs one pass of f and checks it is Done.
func requireDone(f *verifyFixture) {
	f.t.Helper()
	outcome, pointer, err := f.call()
	if err != nil || outcome != shedengine.Done {
		f.t.Fatalf("Call() = %q, %v (%s); want Done", outcome, err, pointer.Reason)
	}
}

// TestDriftVerify covers the row's order of checks, its decision halts under each scope, the agent's gate, the halts that record gate_halt_at and the resumes from them, the interrupted agent, and the rulings under scope full.
func TestDriftVerify(t *testing.T) {
	t.Parallel()

	partial := ScopeRecord{Scope: ScopePartial, Skip: []string{"slow"}}
	full := ScopeRecord{Scope: ScopeFull}
	withdrawn := ScopeRecord{Scope: ScopePartial, Skip: []string{"slow"}, Withdrawn: []string{"t9"}}
	const markedAt = "2026-01-02T03:04:05Z"

	t.Run("a pass that needs no compare is Done", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name  string
			scope ScopeRecord
		}{
			{"no scope record", ScopeRecord{}},
			{"scope off", ScopeRecord{Scope: ScopeOff}},
			{"a waived scope", ScopeRecord{Scope: ScopeFull, WaivedAt: markedAt, WaivedReason: "no baseline possible"}},
		}
		for _, row := range rows {
			f := newVerifyFixture(t, row.scope, nil)
			requireDone(f)
			if f.compareCalls != 0 || len(f.events) != 0 {
				t.Errorf("%s: events = %v; want no compare, dirty check or agent", row.name, f.events)
			}
			if len(f.commits) != 1 || !slices.Equal(f.commits[0], []string{DirRel()}) {
				t.Errorf("%s: commits = %v; want the durable directory once", row.name, f.commits)
			}
		}
	})

	t.Run("no counted capture is a decision halt naming the waiver under every scope, in loom and darn", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			scope   ScopeRecord
			arrange func(f *verifyFixture)
		}{
			{"partial in loom, passed but not awaited", partial, func(f *verifyFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopePartial, Skip: []string{"slow"}}, false)
			}},
			{"full in loom, passed but not awaited", full, func(f *verifyFixture) { f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false) }},
			{"partial in darn, no record", partial, func(f *verifyFixture) {
				f.deps.AwaitRow = false
				if err := os.Remove(f.paths.Capture); err != nil {
					f.t.Fatal(err)
				}
			}},
			{"full in darn, no record", full, func(f *verifyFixture) {
				f.deps.AwaitRow = false
				if err := os.Remove(f.paths.Capture); err != nil {
					f.t.Fatal(err)
				}
			}},
		}
		for _, row := range rows {
			f := newVerifyFixture(t, row.scope, nil)
			row.arrange(f)
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, true, "lym drift waive", "lym loom resume")
			if f.compareCalls != 0 {
				t.Errorf("%s: the compare ran %d times; want none", row.name, f.compareCalls)
			}
		}
	})

	t.Run("an undeclared compare is Stuck naming lym config drift, with the prefix under full", func(t *testing.T) {
		t.Parallel()
		for _, scope := range []ScopeRecord{partial, full} {
			f := newVerifyFixture(t, scope, nil)
			f.deps.Config = func() (Config, error) { return Config{}, nil }
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, scope.Scope == ScopeFull, "lym config drift", "lym loom resume")
		}
	})

	t.Run("a passing compare is Done with no agent", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t, partial, nil)
		requireDone(f)
		if f.seats.Calls != 0 || f.compareCalls != 1 {
			t.Errorf("agents %d, compares %d; want no agent and one compare", f.seats.Calls, f.compareCalls)
		}
	})

	t.Run("a failing compare spawns one agent and resets the run count", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t, partial, nil)
		f.compares = []CompareResult{failingCompare, passingCompare}
		if err := ResetRuns(f.paths, "old"); err != nil {
			t.Fatal(err)
		}
		if _, err := TakeRun(f.paths, 8); err != nil {
			t.Fatal(err)
		}
		if err := CloseAgentSession(f.paths); err != nil {
			t.Fatal(err)
		}

		requireDone(f)
		if f.seats.Calls != 1 {
			t.Errorf("agents = %d; want one", f.seats.Calls)
		}
		if head, found, err := AttemptHead(f.paths); err != nil || !found || head != "aaa" {
			t.Errorf("AttemptHead() = %q, %v, %v; want the spawn's HEAD", head, found, err)
		}
		if count, err := RunCount(f.paths); err != nil || count != 0 {
			t.Errorf("RunCount() = %d, %v; want the count reset", count, err)
		}
		if want := (VerifyTold{CompareLog: "compare-1.log", BaselineHead: "aaa"}); len(f.told) == 0 || f.told[0] != want {
			t.Errorf("told = %v; want %v", f.told, want)
		}
		if open, _ := AgentSessionOpen(f.paths); open {
			t.Errorf("the agent-session mark reads open after the agent's end")
		}
	})

	t.Run("a final compare still failing is Stuck naming accept, with the prefix by scope", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name     string
			scope    ScopeRecord
			prefixed bool
		}{
			{"partial", partial, false},
			{"partial with a withdrawn test", withdrawn, true},
			{"full", full, true},
		}
		for _, row := range rows {
			f := newVerifyFixture(t, row.scope, nil)
			f.compares = []CompareResult{failingCompare, failingCompare}
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, row.prefixed, "lym drift accept", "verify-report.md", "lym loom resume")
		}
	})

	t.Run("a dirty tree re-prompts and, left dirty, is Stuck naming committing or cleaning", func(t *testing.T) {
		t.Parallel()
		clean := newVerifyFixture(t, partial, nil)
		arrivals := 0
		clean.compares = []CompareResult{failingCompare, passingCompare}
		clean.seats = gateLoopSeats(t, []func(){
			func() { arrivals++; clean.dirty = []string{"a.go"} },
			func() { arrivals++; clean.dirty = nil },
		})
		clean.deps.Seats = clean.seats
		requireDone(clean)
		if arrivals != 2 {
			t.Errorf("arrivals = %d; want the dirty tree re-prompted once", arrivals)
		}

		for _, scope := range []ScopeRecord{partial, full} {
			f := newVerifyFixture(t, scope, nil)
			f.compares = []CompareResult{failingCompare}
			f.seats = gateLoopSeats(t, []func(){func() { f.dirty = []string{"a.go"} }})
			f.deps.Seats = f.seats
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, scope.Scope == ScopeFull, dirtyHaltOpening, "commit or clean", "lym loom resume", f.deps.GateFindingsFile)
			if f.scope().GateHaltAt != markedAt {
				t.Errorf("gate_halt_at = %q; want %q", f.scope().GateHaltAt, markedAt)
			}
		}
	})

	t.Run("a verify entry failing its attempts is Stuck naming its findings file, with the prefix under full", func(t *testing.T) {
		t.Parallel()
		for _, scope := range []ScopeRecord{partial, full} {
			f := newVerifyFixture(t, scope, nil)
			f.compares = []CompareResult{failingCompare}
			f.seats = gateLoopSeats(t, []func(){f.commit()})
			f.deps.Seats = f.seats
			f.verifyFn = func() (shuttleengine.GateResult, error) {
				return shuttleengine.GateResult{Findings: "TestCache failed"}, nil
			}
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, scope.Scope == ScopeFull, gateHaltOpening, f.deps.GateFindingsFile, "lym loom resume")
			if f.verifyCalls != 4 {
				t.Errorf("verify ran %d times; want the first arrival and three re-prompts", f.verifyCalls)
			}
			if f.scope().GateHaltAt != markedAt {
				t.Errorf("gate_halt_at = %q; want %q", f.scope().GateHaltAt, markedAt)
			}
		}
	})

	t.Run("the verify entry waits for the agent's commit and a fix after its re-prompt is compared again", func(t *testing.T) {
		t.Parallel()
		unchanged := newVerifyFixture(t, partial, nil)
		unchanged.compares = []CompareResult{failingCompare, passingCompare}
		requireDone(unchanged)
		if unchanged.verifyCalls != 0 {
			t.Errorf("verify ran %d times at an unchanged HEAD; want none", unchanged.verifyCalls)
		}

		f := newVerifyFixture(t, partial, nil)
		f.compares = []CompareResult{failingCompare, passingCompare}
		f.seats = gateLoopSeats(t, []func(){f.commit(), nil})
		f.deps.Seats = f.seats
		results := []shuttleengine.GateResult{{Findings: "TestCache failed"}, {Passed: true}}
		f.verifyFn = func() (shuttleengine.GateResult, error) {
			result := results[0]
			results = results[1:]
			return result, nil
		}
		requireDone(f)
		if f.verifyCalls != 2 || f.compareCalls != 2 {
			t.Errorf("verify ran %d times and the compare %d; want two each, the fix compared again before Done", f.verifyCalls, f.compareCalls)
		}
	})

	t.Run("an entry with a dirty tree is Stuck before any compare, and records-side dirt is no dirty tree", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t, partial, nil)
		f.dirty = []string{"a.go"}
		outcome, pointer, err := f.call()
		requireStuck(t, outcome, pointer, err, false, dirtyHaltOpening, "a.go", "commit or clean")
		if f.compareCalls != 0 || f.scope().GateHaltAt != markedAt {
			t.Errorf("compares %d, gate_halt_at %q; want no compare and the halt recorded", f.compareCalls, f.scope().GateHaltAt)
		}

		// The scope record under the durable directory is a records-side file the fake dirty reading never lists.
		records := newVerifyFixture(t, partial, nil)
		requireDone(records)
	})

	t.Run("a resume from a gate halt", func(t *testing.T) {
		t.Parallel()
		seed := func(f *verifyFixture, attemptHead string, headNow string) {
			f.t.Helper()
			f.head = headNow
			if err := ResetRuns(f.paths, attemptHead); err != nil {
				f.t.Fatal(err)
			}
			if err := CloseAgentSession(f.paths); err != nil {
				f.t.Fatal(err)
			}
			f.seedScopeRecord(ScopeRecord{Scope: ScopePartial, Skip: []string{"slow"}, AwaitedAt: "2026-01-01T00:00:00Z", GateHaltAt: markedAt}, false)
			f.stuckHistory(markedAt, "")
		}

		t.Run("a failing gate is Stuck before any compare and keeps gate_halt_at", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, ScopeRecord{}, nil)
			seed(f, "aaa", "bbb")
			f.verifyFn = func() (shuttleengine.GateResult, error) {
				return shuttleengine.GateResult{Findings: "TestCache failed"}, nil
			}
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, false, gateHaltOpening, f.deps.GateFindingsFile, "lym loom resume")
			if f.compareCalls != 0 || f.scope().GateHaltAt != markedAt {
				t.Errorf("compares %d, gate_halt_at %q; want no compare and the seeded mark kept", f.compareCalls, f.scope().GateHaltAt)
			}
		})

		t.Run("a passing gate clears gate_halt_at and goes on to the compare", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, ScopeRecord{}, nil)
			seed(f, "aaa", "bbb")
			requireDone(f)
			if f.verifyCalls != 1 || f.compareCalls != 1 || f.scope().GateHaltAt != "" {
				t.Errorf("verify %d, compares %d, gate_halt_at %q; want one each and the mark cleared", f.verifyCalls, f.compareCalls, f.scope().GateHaltAt)
			}
		})

		t.Run("a context error leaves gate_halt_at set and the next entry runs the gate again", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, ScopeRecord{}, nil)
			seed(f, "aaa", "bbb")
			f.verifyFn = func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{}, context.Canceled }
			if _, _, err := f.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Call() error = %v; want the context error", err)
			}
			if f.scope().GateHaltAt != markedAt || f.compareCalls != 0 {
				t.Errorf("gate_halt_at %q, compares %d; want the mark kept and no compare", f.scope().GateHaltAt, f.compareCalls)
			}
			f.verifyFn = func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil }
			requireDone(f)
			if f.verifyCalls != 2 || f.compareCalls != 1 {
				t.Errorf("verify %d, compares %d; want the gate run again before the compare", f.verifyCalls, f.compareCalls)
			}
		})

		t.Run("an attempt head equal to HEAD runs no verify gate, clears the mark and compares", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, ScopeRecord{}, nil)
			seed(f, "aaa", "aaa")
			requireDone(f)
			if f.verifyCalls != 0 || f.compareCalls != 1 || f.scope().GateHaltAt != "" {
				t.Errorf("verify %d, compares %d, gate_halt_at %q; want no verify, one compare and the mark cleared", f.verifyCalls, f.compareCalls, f.scope().GateHaltAt)
			}
		})

		t.Run("a halt stamped from a sub-second clock is read as the resume at its second", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, partial, nil)
			f.dirty = []string{"a.go"}
			if _, _, err := f.call(); err != nil {
				t.Fatal(err)
			}
			if err := ResetRuns(f.paths, "aaa"); err != nil {
				t.Fatal(err)
			}
			if err := CloseAgentSession(f.paths); err != nil {
				t.Fatal(err)
			}
			f.dirty, f.head = nil, "bbb"
			f.stuckHistory(markedAt, "")
			requireDone(f)
			if f.verifyCalls != 1 {
				t.Errorf("verify ran %d times; want the resume to run the gate once", f.verifyCalls)
			}
		})

		notResumes := []struct {
			name    string
			history []shedengine.HistoryEntry
			attempt bool
		}{
			{"a last history entry of another row", []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Stuck, At: markedAt}}, true},
			{"a last history entry older than the mark", []shedengine.HistoryEntry{{Producer: "Drift-Verify", Outcome: shedengine.Stuck, At: "2026-01-02T03:04:04Z"}}, true},
			{"a first entry with no attempt head", []shedengine.HistoryEntry{{Producer: "Drift-Verify", Outcome: shedengine.Stuck, At: markedAt}}, false},
		}
		for _, tt := range notResumes {
			t.Run(tt.name+" runs no verify gate", func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t, ScopeRecord{}, nil)
				seed(f, "aaa", "bbb")
				f.history = tt.history
				if !tt.attempt {
					if err := os.Remove(f.paths.Runs); err != nil {
						t.Fatal(err)
					}
				}
				requireDone(f)
				if f.verifyCalls != 0 || f.compareCalls != 1 {
					t.Errorf("verify %d, compares %d; want no verify gate and one compare", f.verifyCalls, f.compareCalls)
				}
			})
		}

		t.Run("a mark left by a gate halt the run left by a goto is cleared by the next entry", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, ScopeRecord{}, nil)
			seed(f, "aaa", "bbb")
			f.history = []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Stuck, At: markedAt}}
			f.deps.AwaitRow = true
			f.writeCapture(CaptureRecord{State: CaptureRunning})
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, true, "lym drift waive")
			if f.scope().GateHaltAt != "" {
				t.Errorf("gate_halt_at = %q; want the stale mark cleared", f.scope().GateHaltAt)
			}
			f.stuckHistory(markedAt, "")
			outcome, pointer, err = f.call()
			requireStuck(t, outcome, pointer, err, true, "lym drift waive")
			if f.verifyCalls != 0 {
				t.Errorf("verify ran %d times; want none, the second entry being no resume", f.verifyCalls)
			}
		})
	})

	t.Run("an interrupted agent", func(t *testing.T) {
		t.Parallel()
		arrange := func(f *verifyFixture) {
			f.t.Helper()
			if err := ResetRuns(f.paths, "aaa"); err != nil {
				f.t.Fatal(err)
			}
			if err := os.MkdirAll(f.paths.LogsDir, 0o755); err != nil {
				f.t.Fatal(err)
			}
			for _, name := range []string{"compare-20260101T000000.000Z.log", "compare-20260102T000000.000Z.log"} {
				if err := os.WriteFile(filepath.Join(f.paths.LogsDir, name), nil, 0o644); err != nil {
					f.t.Fatal(err)
				}
			}
		}

		t.Run("a live chair is resumed before any check", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, partial, nil)
			arrange(f)
			f.liveChair = true
			requireDone(f)
			if want := []string{"probe", "probe", "resume"}; !slices.Equal(f.events[:3], want) {
				t.Errorf("events = %v; want the two probes and the resume first, before any dirty check, verify gate or compare", f.events)
			}
			if f.seats.Calls != 0 || f.seats.ResumeCalls != 1 {
				t.Errorf("runs %d, resumes %d; want the live chair resumed and none started", f.seats.Calls, f.seats.ResumeCalls)
			}
			if want := filepath.Join(f.paths.LogsDir, "compare-20260102T000000.000Z.log"); len(f.told) == 0 || f.told[0].CompareLog != want {
				t.Errorf("told = %v; want the newest compare log %s", f.told, want)
			}
			if open, _ := AgentSessionOpen(f.paths); open {
				t.Errorf("the agent-session mark reads open after the agent's end")
			}
		})

		t.Run("no live chair closes the mark and goes on to the compare", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, partial, nil)
			arrange(f)
			requireDone(f)
			if f.seats.Calls != 0 || f.compareCalls != 1 {
				t.Errorf("runs %d, compares %d; want no agent and one compare", f.seats.Calls, f.compareCalls)
			}
			if open, _ := AgentSessionOpen(f.paths); open {
				t.Errorf("the agent-session mark reads open")
			}
		})

		t.Run("the mark reads closed after an agent that errors", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, partial, nil)
			f.compares = []CompareResult{failingCompare}
			f.seats.RunFn = func(seatengine.Table) (seatengine.Result, error) {
				return seatengine.Result{}, errors.New("the agent died")
			}
			if _, _, err := f.call(); err == nil || !strings.Contains(err.Error(), "the agent died") {
				t.Fatalf("Call() error = %v; want the agent's error", err)
			}
			if open, _ := AgentSessionOpen(f.paths); open {
				t.Errorf("the agent-session mark reads open after the agent's error")
			}
		})
	})

	t.Run("under scope full the parent rules on the agent's acceptances", func(t *testing.T) {
		t.Parallel()
		haltFirst := func(t *testing.T) *verifyFixture {
			t.Helper()
			f := newVerifyFixture(t, full, nil)
			f.compares = []CompareResult{failingCompare, passingCompare}
			f.seats = gateLoopSeats(t, []func(){f.agentAccept()})
			f.deps.Seats = f.seats
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, true, "t1", "lym loom resume", "lym drift withdraw")
			if scope := f.scope(); !slices.Equal(scope.HaltNamed, []string{"t1"}) || scope.HaltNamedAt != markedAt {
				t.Fatalf("halt_named %v at %q; want t1 recorded at %q", scope.HaltNamed, scope.HaltNamedAt, markedAt)
			}
			f.compares = []CompareResult{passingCompare}
			f.compareCalls = 0
			return f
		}

		resumes := []struct {
			name    string
			history []shedengine.HistoryEntry
		}{
			{"a Stuck at or after the recorded time", []shedengine.HistoryEntry{{Producer: "Drift-Verify", Outcome: shedengine.Stuck, At: markedAt}}},
			{"a folded Stuck whose first time predates it and whose last follows it", []shedengine.HistoryEntry{{Producer: "Drift-Verify", Outcome: shedengine.Stuck, At: "2026-01-02T03:00:00Z", LastAt: "2026-01-02T03:04:06Z"}}},
		}
		for _, tt := range resumes {
			t.Run(tt.name+" rules the acceptance", func(t *testing.T) {
				t.Parallel()
				f := haltFirst(t)
				f.history = tt.history
				requireDone(f)
				scope := f.scope()
				if len(scope.Accepted) != 1 || scope.Accepted[0].Ruling != RulingRuled || scope.HaltNamed != nil || scope.HaltNamedAt != "" {
					t.Errorf("scope = %+v; want t1 ruled and the halt cleared", scope)
				}
				if f.seats.Calls != 1 {
					t.Errorf("agents = %d; want no new agent", f.seats.Calls)
				}
			})
		}

		nonResumes := []struct {
			name    string
			history []shedengine.HistoryEntry
		}{
			{"a last history entry of another row", []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Stuck, At: markedAt}}},
			{"a last history entry older than the recorded time", []shedengine.HistoryEntry{{Producer: "Drift-Verify", Outcome: shedengine.Stuck, At: "2026-01-02T03:04:04Z"}}},
		}
		for _, tt := range nonResumes {
			t.Run(tt.name+" rules nothing and halts again", func(t *testing.T) {
				t.Parallel()
				f := haltFirst(t)
				f.history = tt.history
				outcome, pointer, err := f.call()
				requireStuck(t, outcome, pointer, err, true, "t1")
				if f.compareCalls != 0 || f.scope().Accepted[0].Ruling != RulingUnruled {
					t.Errorf("compares %d, ruling %q; want no compare and t1 unruled", f.compareCalls, f.scope().Accepted[0].Ruling)
				}
			})
		}

		t.Run("a test withdrawn at the halt is not ruled and is compared again", func(t *testing.T) {
			t.Parallel()
			f := haltFirst(t)
			if err := Withdraw(f.paths, statusAt("Drift-Verify", shedengine.StateBlocked, shedadapters.DecisionHaltPrefix, false), "t1"); err != nil {
				t.Fatal(err)
			}
			f.stuckHistory(markedAt, "")
			requireDone(f)
			scope := f.scope()
			if len(scope.Accepted) != 0 || !slices.Equal(scope.Withdrawn, []string{"t1"}) || f.compareCalls != 1 {
				t.Errorf("scope = %+v, compares %d; want t1 withdrawn, not ruled, and compared again", scope, f.compareCalls)
			}
		})

		t.Run("an agent that accepts and then errors leaves the resumed entry halting before any compare", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t, full, nil)
			f.compares = []CompareResult{failingCompare}
			f.seats.RunFn = func(seatengine.Table) (seatengine.Result, error) {
				f.agentAccept()()
				return seatengine.Result{}, errors.New("the agent died")
			}
			if _, _, err := f.call(); err == nil {
				t.Fatal("Call() error = nil; want the agent's error")
			}
			compares := f.compareCalls
			f.stuckHistory(markedAt, "")
			outcome, pointer, err := f.call()
			requireStuck(t, outcome, pointer, err, true, "t1")
			if f.compareCalls != compares {
				t.Errorf("the resumed entry ran %d more compares; want it to halt before any compare", f.compareCalls-compares)
			}
		})
	})
}
