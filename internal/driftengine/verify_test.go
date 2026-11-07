// verify_test.go — tier 0 tests for the Drift-Verify producer over the in-memory records store.
// It uses two fake checkouts, a scripted capture, and a judge that runs the judgement gate as the run loop does.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// gateLoopSeats returns a seat runner whose run and resume evaluate the table's gate entries in order.
// It does so as the run loop does at each turn end.
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

var (
	// verifyBaseline and verifyChanged are a capture's files before and after the task changes the value of the key y.
	verifyBaseline = map[string]string{"a.txt": "x=1\ny=2\n"}
	verifyChanged  = map[string]string{"a.txt": "x=1\ny=3\n"}

	// darnRows and tapestryRows are the row orders a run of each recipe holds around the Drift rows.
	darnRows     = []string{"Record-Write", "Drift-Baseline", "Drift-Await", expectRow, darnWriteRow, "Drift-Verify", "Drift-Recheck"}
	tapestryRows = []string{"Drift-Baseline", "Drift-Await", expectRow, websterRow, "Parent-Align", "Drift-Verify", "Code-Bouncer", "Drift-Recheck"}

	passingCompare = CompareResult{Passed: true}
	failingCompare = CompareResult{ExitCode: 1, Log: "compare-1.log", Reason: "exit 1; log compare-1.log"}
)

// expectsX is an expectation covering the key `x` of `a.txt` alone, which leaves a change of `y` uncovered.
const expectsX = "cases:\n  a.txt:\n    keys:\n      - key: x\n        why: x is expected to move\n"

// verifyFixture is the fake world one DriftVerify pass runs in.
type verifyFixture struct {
	t     *testing.T
	store *commitStore
	paths Paths
	deps  Deps
	seats *shedfake.SeatRunner

	// head is the code HEAD, which moveHead keeps equal to the store's.
	head string
	// row is the row the Status seam reports.
	row    string
	config Config
	// current holds the next capture's files; captureDir is the last capture's directory, captureFails fails the next.
	current      map[string]string
	captureDir   string
	captureFails bool
	captures     int
	compares     int
	// compare is what the Compare seam returns; caseRulings is the ruling the judge gives each failing case by name.
	compare     CompareResult
	caseRulings map[string]Ruling
	// endedRoles holds the roles of each call of EndRoles.
	endedRoles [][]string
	// liveChair is whether a probe finds the judge's chair.
	liveChair bool
	// codeCheckout and recordsCheckout are the two fake checkouts, and recordsChanged is what RecordsChangedSince answers.
	codeCheckout, recordsCheckout fabricengine.CheckoutState
	// checkoutsErr, when set, is what the checkouts read fails with, so taking the judge snapshot fails.
	checkoutsErr   error
	recordsChanged []string
	// rulings maps a cell's "<case> <key>" to its ruling, which defaults to matching; rangeChecked is the open-range note.
	rulings      map[string]Ruling
	rangeChecked string
	// onArrival runs at each judge arrival before the judgement is written; badJudgement makes it unparsable.
	onArrival    func()
	badJudgement bool
	// requests, judgementPresent, and marksAtArrival record each arrival's request, file presence and session mark.
	requests         []JudgeRequest
	judgementPresent []bool
	marksAtArrival   []SessionMark
}

// newVerifyFixture returns a fixture at Drift-Verify under darn's rows.
// It uses the lines mode, the key=value pattern, and a judge that rules every cell matching.
func newVerifyFixture(t *testing.T) *verifyFixture {
	t.Helper()
	store := newCommitStore(t)
	f := &verifyFixture{
		t:               t,
		store:           store,
		paths:           NewPaths(store.root),
		head:            store.codeHead,
		row:             "Drift-Verify",
		config:          Config{Enabled: true, Diff: DiffLines, KeyPatterns: []string{keyValuePattern}},
		rangeChecked:    "read what the range covers",
		compare:         passingCompare,
		codeCheckout:    fabricengine.CheckoutState{Root: t.TempDir(), Head: "code-1"},
		recordsCheckout: fabricengine.CheckoutState{Root: t.TempDir(), Head: "records-1"},
	}

	actions := make([]func(), 12)
	for i := range actions {
		actions[i] = f.judgeArrives
	}
	f.seats = gateLoopSeats(t, actions)
	f.seats.ProbeFn = func(seatengine.Table) (seatengine.LiveTable, error) {
		if f.liveChair {
			return seatengine.LiveTable{Chair: fakeChair{}}, nil
		}
		return seatengine.LiveTable{}, nil
	}

	d := store.deps(f.paths)
	d.Rows = darnRows
	d.Head = func() (string, error) { return f.head, nil }
	d.Config = func() (Config, error) { return f.config, nil }
	d.Status = func() (shedengine.Status, error) { return shedengine.Status{CurrentProducer: f.row}, nil }
	d.Seats = f.seats
	d.Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 700_000_000, time.UTC) }
	d.GateFindingsFile = filepath.Join(t.TempDir(), "gate-findings.md")
	d.CaptureCurrent = func(context.Context) (CurrentCapture, error) {
		f.captures++
		if f.captureFails {
			return CurrentCapture{Log: "/logs/current.log", Reason: "exit 3"}, nil
		}
		f.captureDir = t.TempDir()
		writeTree(t, f.captureDir, f.current)
		return CurrentCapture{Dir: f.captureDir, Log: "/logs/current.log", Passed: true}, nil
	}
	d.Compare = func(context.Context) (CompareResult, error) {
		f.compares++
		return f.compare, nil
	}
	d.JudgeTable = func(string) (seatengine.Table, error) {
		return seatengine.Table{Seats: []seatengine.Seat{{Outputs: []string{f.paths.Judgement}}}}, nil
	}
	d.JudgeSnapshotDir = filepath.Join(t.TempDir(), "snapshot")
	d.LoomRecordsDirs = []string{testRunDirRel, testFrictionRel, testFrictionPrefix}
	d.Checkouts = func() (fabricengine.CheckoutState, fabricengine.CheckoutState, error) {
		return f.codeCheckout, f.recordsCheckout, f.checkoutsErr
	}
	d.RecordsChangedSince = func(string) ([]string, error) { return f.recordsChanged, nil }
	d.EndRoles = func(roles []string) error {
		f.endedRoles = append(f.endedRoles, slices.Clone(roles))
		return nil
	}
	f.deps = d
	return f
}

// moveHead moves the code HEAD of the fixture and of its records store.
func (f *verifyFixture) moveHead(head string) {
	f.head = head
	f.store.codeHead = head
}

// judgeArrives is the scripted judge's action at each arrival.
// It reads the request and writes a judgement that rules every cell by rulings and notes every open range.
func (f *verifyFixture) judgeArrives() {
	_, statErr := os.Stat(f.paths.Judgement)
	f.judgementPresent = append(f.judgementPresent, statErr == nil)
	request, found, err := readJudgeRequest(f.paths)
	if err != nil || !found {
		f.t.Fatalf("the judge arrived with no readable request: found %v, error %v", found, err)
	}
	f.requests = append(f.requests, request)
	f.marksAtArrival = append(f.marksAtArrival, f.records().Scope.JudgeSession)
	if f.onArrival != nil {
		f.onArrival()
	}
	if f.badJudgement {
		writeTree(f.t, filepath.Dir(f.paths.Judgement), map[string]string{judgementFileName: "spawn_id: not-this-spawn\n"})
		return
	}

	if request.Mode == DiffCommand {
		var text strings.Builder
		fmt.Fprintf(&text, "spawn_id: %s\ncases:\n", request.SpawnID)
		for _, name := range slices.Sorted(maps.Keys(f.caseRulings)) {
			fmt.Fprintf(&text, "  - case: %s\n    ruling: %s\n    reason: compared the old and the new value\n", name, f.caseRulings[name])
		}
		writeTree(f.t, filepath.Dir(f.paths.Judgement), map[string]string{judgementFileName: text.String()})
		return
	}

	var text strings.Builder
	fmt.Fprintf(&text, "spawn_id: %s\ncells:\n", request.SpawnID)
	for _, cell := range request.Cells {
		ruling := RulingMatches
		if set, ok := f.rulings[cell.Case+" "+cell.Identity.Key]; ok {
			ruling = set
		}
		fmt.Fprintf(&text, "  - id: %s\n    ruling: %s\n    reason: compared the old and the new value\n", cell.ID, ruling)
	}
	text.WriteString("open_ranges:\n")
	for _, open := range request.OpenRanges {
		fmt.Fprintf(&text, "  - case: %s\n    range: %q\n    checked: %s\n", open.Case, open.Range, f.rangeChecked)
	}
	writeTree(f.t, filepath.Dir(f.paths.Judgement), map[string]string{judgementFileName: text.String()})
}

// seed writes the baseline files and a passed capture record at the code HEAD.
// It then commits the scope record with the baseline's digest in one records commit.
func (f *verifyFixture) seed(scope ScopeRecord, baseline map[string]string) {
	f.t.Helper()
	writeTree(f.t, f.paths.BaselineDir, baseline)
	digest, err := BaselineDigest(f.paths.BaselineDir)
	if err != nil {
		f.t.Fatalf("BaselineDigest() = _, %v; want nil", err)
	}
	if err := WriteCapture(f.paths, CaptureRecord{State: CapturePassed, Head: f.head}); err != nil {
		f.t.Fatalf("WriteCapture() = %v; want nil", err)
	}
	change := func(s *ScopeRecord, _ *JudgedRecord) error { *s = scope; return nil }
	if err := WriteRecords(f.deps, change, fabricengine.Trailer{Key: BaselineTrailerKey, Value: digest}); err != nil {
		f.t.Fatalf("WriteRecords() = %v; want nil", err)
	}
}

// seal writes expected.yaml and commits it as the seal.
func (f *verifyFixture) seal(expected string) {
	f.t.Helper()
	writeTree(f.t, filepath.Dir(f.paths.Expected), map[string]string{expectedFileName: expected})
	if err := SealExpectation(f.deps); err != nil {
		f.t.Fatalf("SealExpectation() = %v; want nil", err)
	}
}

// seedChanged seeds the baseline and seals expected unless it is empty.
// It also sets a current capture in which y changed from 2 to 3.
func (f *verifyFixture) seedChanged(expected string) {
	f.t.Helper()
	f.seed(countedScope(), verifyBaseline)
	if expected != "" {
		f.seal(expected)
	}
	f.current = verifyChanged
}

// seedCommand seeds the baseline under diff command, the sealed expectation of the case a.txt and a failing compare.
func (f *verifyFixture) seedCommand() {
	f.t.Helper()
	scope := countedScope()
	scope.Diff = DiffCommand
	f.seed(scope, verifyBaseline)
	f.seal(expectsCommandCase)
	f.compare = failingCompare
}

// seedMark opens the judge-session mark under spawnID, and takes the snapshot of the fake checkouts when snapshot is set.
func (f *verifyFixture) seedMark(spawnID string, snapshot bool) {
	f.t.Helper()
	err := WriteRecords(f.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.JudgeSession = SessionMark{Open: true, OpenedAt: "2026-01-01T00:00:00Z", SpawnID: spawnID}
		return nil
	})
	if err != nil {
		f.t.Fatalf("WriteRecords() = %v; want nil", err)
	}
	if snapshot {
		if _, err := TakeJudgeSnapshot(f.deps, ""); err != nil {
			f.t.Fatalf("TakeJudgeSnapshot() = _, %v; want nil", err)
		}
	}
}

// storeRuling stores tuple in judged.json through the records guard.
func (f *verifyFixture) storeRuling(tuple JudgedTuple) {
	f.t.Helper()
	err := WriteRecords(f.deps, func(_ *ScopeRecord, judged *JudgedRecord) error {
		judged.Tuples = append(judged.Tuples, tuple)
		return nil
	})
	if err != nil {
		f.t.Fatalf("WriteRecords() = %v; want nil", err)
	}
}

// call runs one DriftVerify pass with a judgement entry of three attempts.
func (f *verifyFixture) call() (shedengine.Outcome, shedengine.OutputPointer, error) {
	entry := shuttleengine.GateEntry{Name: "judgement", Attempts: 3, Gate: NewJudgementGate(f.deps)}
	return NewDriftVerify(f.row, Row{}, entry, f.deps).Call(context.Background())
}

// records reads the two records as the guard returns them.
func (f *verifyFixture) records() Records {
	f.t.Helper()
	records, err := ReadRecords(f.deps)
	if err != nil {
		f.t.Fatalf("ReadRecords() = _, %v; want nil", err)
	}
	return records
}

// scope reads the scope record.
func (f *verifyFixture) scope() ScopeRecord {
	f.t.Helper()
	return f.records().Scope
}

// untouched reports whether the seat runner was never asked to probe, run or resume the judge.
func (f *verifyFixture) untouched() bool {
	return f.seats.Calls+f.seats.ProbeCalls+f.seats.ResumeCalls == 0
}

// judgementGone reports whether judgement.yaml is absent.
func (f *verifyFixture) judgementGone() bool {
	_, err := os.Stat(f.paths.Judgement)
	return os.IsNotExist(err)
}

// commit is the action of the judge committing on the code side.
func (f *verifyFixture) commit() func() { return func() { f.codeCheckout.Head = "code-2" } }

// requireStuck checks the outcome is Stuck and the reason names every fragment;
// prefixed says whether the reason opens with the decision halt prefix.
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

// requireDeferPrefix checks the reason carries DeferHaltPrefix right after the decision halt prefix exactly when deferred is set.
func requireDeferPrefix(t *testing.T, reason string, deferred bool) {
	t.Helper()
	got := strings.HasPrefix(reason, shedadapters.DecisionHaltPrefix+DeferHaltPrefix)
	if got != deferred || (!deferred && strings.Contains(reason, DeferHaltPrefix)) {
		t.Errorf("reason %q carries the defer prefix after the decision prefix = %v; want %v", reason, got, deferred)
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

// TestDriftVerify covers the passes that need no judge and the Stuck passes that stop before it.
// It covers the stored rulings and the judge with its request and rulings.
// It also covers the session an earlier entry left open and the snapshot guard around it.
func TestDriftVerify(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const checkedAt = "2026-01-02T03:04:05Z"

	t.Run("a pass that needs no judge is Done with no capture", func(t *testing.T) {
		t.Parallel()
		withDiff := func(mutate func(*ScopeRecord)) ScopeRecord {
			scope := countedScope()
			mutate(&scope)
			return scope
		}
		tests := []struct {
			name       string
			scope      *ScopeRecord
			wantReason string
		}{
			{"NoScopeRecord", nil, "no scope is recorded"},
			{"ScopeOffNamingItsSource", ptr(withDiff(func(s *ScopeRecord) { s.Scope, s.ScopeSource = ScopeOff, "drift.scope" })), "set by drift.scope"},
			{"ADeferredCapture", ptr(withDiff(func(s *ScopeRecord) { s.Deferred = true })), "deferred"},
			{"ADeferredCaptureUnderCommand", ptr(withDiff(func(s *ScopeRecord) { s.Deferred, s.Diff = true, DiffCommand })), "deferred"},
			{"AWaiver", ptr(withDiff(func(s *ScopeRecord) { s.WaivedAt, s.WaivedReason = checkedAt, "no profile" })), "no profile"},
			{"TheHeadOfTheLastPassWithNoAmendmentSince", ptr(withDiff(func(s *ScopeRecord) { s.LastPassHead = "code-head-0" })), "code-head-0"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				if tt.scope != nil {
					f.seed(*tt.scope, verifyBaseline)
				}
				f.current = verifyChanged

				outcome, pointer, err := f.call()

				if err != nil || outcome != shedengine.Done || !strings.Contains(pointer.Reason, tt.wantReason) {
					t.Fatalf("Call() = %q, %v (%s); want Done naming %q", outcome, err, pointer.Reason, tt.wantReason)
				}
				if f.captures != 0 || f.compares != 0 || !f.untouched() {
					t.Errorf("%d captures, %d compares, judge untouched %v; want no capture, no compare and no judge", f.captures, f.compares, f.untouched())
				}
			})
		}
	})

	t.Run("drift switched off is Done at once with no chair ended and no commit", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		commits := len(f.store.commits)
		f.config.Enabled = false

		outcome, pointer, err := f.call()

		if err != nil || outcome != shedengine.Done || !strings.Contains(pointer.Reason, "drift.enabled = false") {
			t.Fatalf("Call() = %q, %v (%s); want Done naming the switch", outcome, err, pointer.Reason)
		}
		if len(f.endedRoles) != 0 || len(f.store.commits) != commits || f.captures != 0 {
			t.Errorf("ended %v, %d new commits, %d captures; want none", f.endedRoles, len(f.store.commits)-commits, f.captures)
		}
	})

	t.Run("a pass stops for the parent before any judge", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			rows      []string
			prepare   func(f *verifyFixture)
			fragments []string
			// absent holds text the reason must not name.
			absent []string
			// deferred says the reason carries DeferHaltPrefix after the decision halt prefix.
			deferred bool
			check    func(t *testing.T, f *verifyFixture)
		}{
			{
				name: "ARecordsMismatch",
				prepare: func(f *verifyFixture) {
					f.seedChanged(expectsY)
					if err := os.WriteFile(f.paths.Scope, []byte(`{"scope":"full","awaited_at":"2026-01-01T00:00:00Z","diff":"lines"}`), 0o644); err != nil {
						f.t.Fatal(err)
					}
				},
				fragments: []string{"differ from their newest committed copy", "lym drift amend <slug> --restore", "lym loom resume"},
			},
			{
				name: "ACaptureThatNeverCounted",
				prepare: func(f *verifyFixture) {
					scope := countedScope()
					scope.AwaitedAt = ""
					f.seed(scope, verifyBaseline)
				},
				fragments: []string{"never counted", "lym drift defer", "lym loom resume"},
				deferred:  true,
			},
			{
				name: "AMissingBaselineWithNoCaptureRecord",
				prepare: func(f *verifyFixture) {
					f.seedChanged(expectsY)
					if err := os.RemoveAll(f.paths.BaselineDir); err != nil {
						f.t.Fatal(err)
					}
					if err := os.Remove(f.paths.Capture); err != nil {
						f.t.Fatal(err)
					}
				},
				fragments: []string{"baseline directory", "lym drift defer", "lym loom resume"},
				deferred:  true,
			},
			{
				name: "AnEditedBaseline",
				prepare: func(f *verifyFixture) {
					f.seedChanged(expectsY)
					writeTree(f.t, f.paths.BaselineDir, map[string]string{"a.txt": "x=1\ny=9\n"})
				},
				fragments: []string{"no longer matches", "lym drift defer"},
				deferred:  true,
			},
			{
				name: "AnExpectationWithNoSealCommit",
				prepare: func(f *verifyFixture) {
					f.seedChanged("")
					writeTree(f.t, filepath.Dir(f.paths.Expected), map[string]string{expectedFileName: expectsY})
				},
				fragments: []string{"no commit sealed it", "lym drift amend <slug> --discard"},
				check: func(t *testing.T, f *verifyFixture) {
					if f.scope().Unsealed {
						t.Errorf("the scope record reads unsealed beside an expected.yaml; want it left unmarked")
					}
				},
			},
			{
				name: "AFailedCurrentCapture",
				prepare: func(f *verifyFixture) {
					f.seedChanged(expectsY)
					f.captureFails = true
				},
				fragments: []string{"exit 3", "/logs/current.log", "lym loom resume", "lym loom goto --to Darn-Write then lym loom resume", "lym drift defer"},
				deferred:  true,
			},
			{
				name: "AnUnexpectedCellOfAnUnsealedRunUnderDarnsRows",
				prepare: func(f *verifyFixture) {
					f.seedChanged("")
				},
				fragments: []string{`changed in case "a.txt"`, `key "y"`, `old "2"`, `new "3"`, "lym drift amend <slug>", "lym loom goto --to Darn-Write then lym loom resume"},
				check: func(t *testing.T, f *verifyFixture) {
					scope := f.scope()
					if !scope.Unsealed || scope.LastCheck.Passed || scope.LastCheck.Unexpected != 1 || scope.LastCheck.Changed != 1 {
						t.Errorf("scope = unsealed %v, last check %+v; want an unsealed run whose last check found one unexpected cell", scope.Unsealed, scope.LastCheck)
					}
				},
			},
			{
				name: "AnUnexpectedCellUnderTapestrysRows",
				rows: tapestryRows,
				prepare: func(f *verifyFixture) {
					f.seedChanged(expectsX)
				},
				fragments: []string{`key "y"`, "lym drift amend <slug>", "lym loom goto --to Webster then lym loom resume"},
				absent:    []string{"Darn-Write"},
			},
			{
				name: "AStoredRulingOfDoesNotMatch",
				prepare: func(f *verifyFixture) {
					f.seedChanged(expectsY)
					f.storeRuling(JudgedTuple{Case: "a.txt", Identity: Identity{Key: "y", N: 1}, Kind: CellChanged, Old: "2", New: "3", Ruling: RulingNoMatch, Reason: "wrong direction"})
				},
				fragments: []string{`key "y"`, "wrong direction", "lym drift amend <slug>", "lym loom goto --to Darn-Write then lym loom resume"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				if tt.rows != nil {
					f.deps.Rows = tt.rows
				}
				tt.prepare(f)

				outcome, pointer, err := f.call()

				requireStuck(t, outcome, pointer, err, true, tt.fragments...)
				requireDeferPrefix(t, pointer.Reason, tt.deferred)
				for _, absent := range tt.absent {
					if strings.Contains(pointer.Reason, absent) {
						t.Errorf("reason %q names %q; want it left out", pointer.Reason, absent)
					}
				}
				if !f.untouched() || f.compares != 0 {
					t.Errorf("judge untouched %v, %d compares; want no judge and no compare", f.untouched(), f.compares)
				}
				if tt.check != nil {
					tt.check(t, f)
				}
			})
		}
	})

	t.Run("every cell expected passes with stored rulings and judges a changed value again", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			current    map[string]string
			wantJudged bool
		}{
			{"AReproducedTupleReusesItsRuling", verifyChanged, false},
			{"AChangedNewValueIsJudgedAgain", map[string]string{"a.txt": "x=1\ny=4\n"}, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				f.seedChanged(expectsY)
				f.current = tt.current
				f.storeRuling(JudgedTuple{Case: "a.txt", Identity: Identity{Key: "y", N: 1}, Kind: CellChanged, Old: "2", New: "3", Ruling: RulingMatches, Reason: "seeded"})

				requireDone(f)

				if (f.seats.Calls == 1) != tt.wantJudged {
					t.Errorf("%d judge runs; want a run = %v", f.seats.Calls, tt.wantJudged)
				}
				scope, judged := f.records().Scope, f.records().Judged
				if !scope.LastCheck.Passed || scope.LastPassHead != "code-head-0" || scope.LastPassAmendments != 0 || scope.LastCheck.At != checkedAt {
					t.Errorf("scope = last check %+v, last pass %q at %d amendments; want a pass at code-head-0 with none", scope.LastCheck, scope.LastPassHead, scope.LastPassAmendments)
				}
				wantTuples, wantReused := 1, 1
				if tt.wantJudged {
					wantTuples, wantReused = 2, 0
				}
				if len(judged.Tuples) != wantTuples || scope.LastCheck.Reused != wantReused || scope.LastCheck.Expected != 1 {
					t.Errorf("%d stored tuples and %d reused in %+v; want %d and %d", len(judged.Tuples), scope.LastCheck.Reused, scope.LastCheck, wantTuples, wantReused)
				}
				if !reflect.DeepEqual(f.endedRoles, [][]string{{"drift-baseline", "drift-verify"}}) {
					t.Errorf("ended roles = %v; want the removed agents' roles once", f.endedRoles)
				}
			})
		}
	})

	t.Run("a cell with no stored ruling spawns one judge whose request lists it", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		writeTree(t, filepath.Dir(f.paths.Judgement), map[string]string{judgementFileName: "spawn_id: stale\n"})

		requireDone(f)

		if len(f.requests) != 1 || f.seats.Calls != 1 {
			t.Fatalf("%d requests and %d runs; want one judge", len(f.requests), f.seats.Calls)
		}
		request := f.requests[0]
		wantCell := JudgeCell{ID: "c1", Case: "a.txt", Identity: JudgeIdentity{Key: "y", N: 1}, Kind: CellChanged, Old: "2", New: "3", Why: "tuned the setting"}
		if !strings.HasPrefix(request.SpawnID, "judge-") || request.Mode != DiffLines || !slices.Equal(request.Cells, []JudgeCell{wantCell}) || request.Expectation != expectsY || request.Amendments != "" {
			t.Errorf("request = %+v; want one lines-mode cell %+v under a fresh spawn id with the sealed expectation and no amendments", request, wantCell)
		}
		if f.compares != 0 {
			t.Errorf("%d compares under diff lines; want none", f.compares)
		}
		if f.judgementPresent[0] {
			t.Errorf("a stale judgement.yaml was still there when the judge arrived; want it removed before the spawn")
		}
		if mark := f.marksAtArrival[0]; !mark.Open || mark.SpawnID != request.SpawnID {
			t.Errorf("session mark at the judge's arrival = %+v; want it open under the request's spawn id %q", mark, request.SpawnID)
		}
		if _, found, err := ReadJudgeSnapshot(f.deps); err != nil || !found {
			t.Errorf("ReadJudgeSnapshot() = _, %v, %v; want the snapshot taken before the spawn", found, err)
		}
		if gate := f.seats.GotTables[0].Gate; len(gate) != 1 || gate[0].Name != "judgement" {
			t.Errorf("judge table gate = %+v; want the one judgement entry", gate)
		}

		scope, judged := f.records().Scope, f.records().Judged
		wantTuple := JudgedTuple{Case: "a.txt", Identity: Identity{Key: "y", N: 1}, Kind: CellChanged, Old: "2", New: "3", Ruling: RulingMatches, Reason: "compared the old and the new value", At: checkedAt}
		if !slices.Equal(judged.Tuples, []JudgedTuple{wantTuple}) {
			t.Errorf("stored tuples = %+v; want %+v", judged.Tuples, wantTuple)
		}
		if scope.JudgeSession.Open || !scope.LastCheck.Passed || scope.LastCheck.Matched != 1 || scope.LastPassHead != "code-head-0" {
			t.Errorf("scope = mark %+v, last check %+v, last pass %q; want a closed mark and a pass with one matched cell", scope.JudgeSession, scope.LastCheck, scope.LastPassHead)
		}
		if !f.judgementGone() {
			t.Errorf("judgement.yaml is still there after the Done; want it removed once the rulings are stored")
		}
	})

	t.Run("a snapshot that fails on a fresh judge session leaves no session mark open", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		f.checkoutsErr = errors.New("checkouts unreadable")

		_, _, err := f.call()

		if err == nil {
			t.Fatalf("Call() error = nil; want the snapshot failure")
		}
		if f.scope().JudgeSession.Open || f.seats.Calls != 0 {
			t.Errorf("mark %+v and %d runs after the failed snapshot; want no open mark and no judge", f.scope().JudgeSession, f.seats.Calls)
		}
	})

	t.Run("a judge ruling does not match stops naming the cell and stores the ruling", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		f.rulings = map[string]Ruling{"a.txt y": RulingNoMatch}

		outcome, pointer, err := f.call()

		requireStuck(t, outcome, pointer, err, true, `key "y"`, "compared the old and the new value", "lym drift amend <slug>", "lym loom goto --to Darn-Write then lym loom resume")
		requireDeferPrefix(t, pointer.Reason, false)
		scope, judged := f.records().Scope, f.records().Judged
		if len(judged.Tuples) != 1 || judged.Tuples[0].Ruling != RulingNoMatch {
			t.Errorf("stored tuples = %+v; want the one ruling of does not match", judged.Tuples)
		}
		if scope.JudgeSession.Open || scope.LastCheck.Passed || scope.LastPassHead != "" {
			t.Errorf("scope = mark %+v, last check %+v, last pass %q; want a closed mark and no pass", scope.JudgeSession, scope.LastCheck, scope.LastPassHead)
		}
		if !f.judgementGone() {
			t.Errorf("judgement.yaml is still there after the does-not-match Stuck; want it removed once the rulings are stored")
		}
	})

	t.Run("a cell of an added case names the case's file in the current capture", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seed(countedScope(), map[string]string{"a.txt": "x=1\n"})
		f.seal("added:\n  - case: b.txt\n    why: the task adds b\n")
		f.current = map[string]string{"a.txt": "x=1\n", "b.txt": "x=1\n"}

		requireDone(f)

		want := JudgeCell{ID: "c1", Case: "b.txt", Kind: CellAddedCase, Why: "the task adds b", File: filepath.Join(f.captureDir, "b.txt")}
		if len(f.requests) != 1 || !slices.Equal(f.requests[0].Cells, []JudgeCell{want}) {
			t.Errorf("requests = %+v; want the one added-case cell %+v", f.requests, want)
		}
	})

	t.Run("an open range is stored with its why and the judge's sentence, and a later listing replaces it", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seed(countedScope(), map[string]string{"a.txt": "x=1\nfree\nmore\n"})
		f.seal("cases:\n  a.txt:\n    lines:\n      - range: \"1-\"\n        why: the free text is rewritten\n")
		f.current = map[string]string{"a.txt": "x=1\nfree2\nmore2\n"}
		f.rangeChecked = "first look"

		requireDone(f)

		wantOpen := []JudgeOpenRange{{Case: "a.txt", Range: "1-", Why: "the free text is rewritten"}}
		if len(f.requests) != 1 || !slices.Equal(f.requests[0].OpenRanges, wantOpen) {
			t.Fatalf("requests = %+v; want the one open range %+v listed once for both cells", f.requests, wantOpen)
		}
		wantRange := JudgedRange{Case: "a.txt", Range: "1-", Why: "the free text is rewritten", Checked: "first look", At: checkedAt}
		if got := f.records().Judged.Ranges; !slices.Equal(got, []JudgedRange{wantRange}) {
			t.Errorf("stored ranges = %+v; want %+v", got, wantRange)
		}

		f.moveHead("code-head-1")
		f.current = map[string]string{"a.txt": "x=1\nfree3\nmore3\n"}
		f.rangeChecked = "second look"

		requireDone(f)

		wantRange.Checked = "second look"
		if got := f.records().Judged.Ranges; !slices.Equal(got, []JudgedRange{wantRange}) {
			t.Errorf("stored ranges after a later judge = %+v; want the one range replaced by %+v", got, wantRange)
		}
	})

	t.Run("a record with no diff records the config's, and a later config change moves neither", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		scope := countedScope()
		scope.Diff, scope.KeyPatterns = "", nil
		f.seed(scope, verifyBaseline)
		f.current = verifyBaseline
		f.config.Volatile = Volatile{Files: []string{"_source.txt"}}

		requireDone(f)

		recorded := f.scope()
		if recorded.Diff != DiffLines || !slices.Equal(recorded.KeyPatterns, []string{keyValuePattern}) || !slices.Equal(recorded.Volatile.Files, f.config.Volatile.Files) {
			t.Fatalf("scope diff %q, patterns %q and volatile %+v; want the config's %q, %q and %+v", recorded.Diff, recorded.KeyPatterns, recorded.Volatile, DiffLines, keyValuePattern, f.config.Volatile)
		}

		f.config.Diff, f.config.KeyPatterns, f.config.Volatile = DiffCommand, []string{`^(?P<key>\w+):(?P<value>.*)$`}, Volatile{}
		f.moveHead("code-head-1")
		requireDone(f)

		if again := f.scope(); again.Diff != DiffLines || !slices.Equal(again.KeyPatterns, recorded.KeyPatterns) || !slices.Equal(again.Volatile.Files, recorded.Volatile.Files) {
			t.Errorf("scope diff %q, patterns %q and volatile %+v after the config changed; want %q, %q and %+v kept", again.Diff, again.KeyPatterns, again.Volatile, DiffLines, recorded.KeyPatterns, recorded.Volatile)
		}
	})

	t.Run("under command the compare decides and the judge rules each case", func(t *testing.T) {
		t.Parallel()

		t.Run("a passing compare is Done with no judge and no capture", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedCommand()
			f.compare = passingCompare

			requireDone(f)

			scope := f.scope()
			if f.compares != 1 || f.captures != 0 || !f.untouched() {
				t.Errorf("%d compares, %d captures, judge untouched %v; want one compare, no capture and no judge", f.compares, f.captures, f.untouched())
			}
			if !scope.LastCheck.Passed || scope.LastCheck.Mode != DiffCommand || scope.LastPassHead != "code-head-0" {
				t.Errorf("last check %+v, last pass %q; want a command-mode pass at code-head-0", scope.LastCheck, scope.LastPassHead)
			}
		})

		t.Run("a failing compare spawns one judge whose request holds the log, the expectation and the amendments, and all matches records the pass", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedCommand()
			const amended = "cases:\n  b.txt:\n    why: the amendment explains b\n"
			err := WriteRecords(f.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
				scope.Amendments = []Amendment{{By: "parent", At: "2026-01-01T12:00:00Z", Kind: amendmentKindEntries, Entries: amended}}
				return nil
			})
			if err != nil {
				t.Fatalf("WriteRecords() = %v; want nil", err)
			}
			f.caseRulings = map[string]Ruling{"a.txt": RulingMatches}

			requireDone(f)

			if len(f.requests) != 1 {
				t.Fatalf("%d requests; want one judge", len(f.requests))
			}
			request := f.requests[0]
			wantAmendments := "# amendment entries by parent at 2026-01-01T12:00:00Z\n" + amended
			if request.Mode != DiffCommand || request.CompareLog != failingCompare.Log || request.Expectation != expectsCommandCase || request.Amendments != wantAmendments || len(request.Cells) != 0 {
				t.Errorf("request = %+v; want the command mode, the compare's log, the sealed expectation, the amendments as text and no cells", request)
			}
			scope := f.scope()
			if len(f.records().Judged.Cases) != 0 || scope.JudgeSession.Open || !scope.LastCheck.Passed || scope.LastCheck.CompareExit != 1 || scope.LastCheck.Matched != 1 {
				t.Errorf("cases %+v, mark %+v, last check %+v; want no stored ruling, a closed mark and a pass with one matched case", f.records().Judged.Cases, scope.JudgeSession, scope.LastCheck)
			}
			if !f.judgementGone() {
				t.Errorf("judgement.yaml is still there after the Done; want it removed once the rulings are stored")
			}

			requireDone(f)

			if f.compares != 1 || f.seats.Calls != 1 {
				t.Errorf("%d compares and %d judge runs after a second entry at the same head; want the pass reused with no compare and no judge", f.compares, f.seats.Calls)
			}
		})

		t.Run("a ruling of unexpected or does not match stops naming the case and is stored", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name   string
				ruling Ruling
			}{
				{"Unexpected", RulingUnexpected},
				{"DoesNotMatch", RulingNoMatch},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					f := newVerifyFixture(t)
					f.seedCommand()
					f.caseRulings = map[string]Ruling{"a.txt": tt.ruling}

					outcome, pointer, err := f.call()

					requireStuck(t, outcome, pointer, err, true, `case "a.txt"`, string(tt.ruling), "compared the old and the new value", "lym drift amend <slug>", "lym loom goto --to Darn-Write then lym loom resume")
					requireDeferPrefix(t, pointer.Reason, false)
					want := JudgedCase{CodeHead: "code-head-0", Case: "a.txt", Ruling: tt.ruling, Reason: "compared the old and the new value", At: checkedAt}
					if got := f.records().Judged.Cases; !slices.Equal(got, []JudgedCase{want}) {
						t.Errorf("stored cases = %+v; want %+v", got, want)
					}
					scope := f.scope()
					if scope.JudgeSession.Open || scope.LastCheck.Passed || scope.LastPassHead != "" {
						t.Errorf("scope = mark %+v, last check %+v, last pass %q; want a closed mark and no pass", scope.JudgeSession, scope.LastCheck, scope.LastPassHead)
					}
					if !f.judgementGone() {
						t.Errorf("judgement.yaml is still there after the Stuck; want it removed once the rulings are stored")
					}
				})
			}
		})

		t.Run("a stored ruling stops a resumed row with no judge, and an amendment recorded since judges the case again", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedCommand()
			err := WriteRecords(f.deps, func(_ *ScopeRecord, judged *JudgedRecord) error {
				judged.Cases = []JudgedCase{{CodeHead: "code-head-0", Case: "a.txt", Ruling: RulingNoMatch, Reason: "wrong direction", At: "2026-01-01T06:00:00Z"}}
				return nil
			})
			if err != nil {
				t.Fatalf("WriteRecords() = %v; want nil", err)
			}

			outcome, pointer, err := f.call()

			requireStuck(t, outcome, pointer, err, true, `case "a.txt"`, "wrong direction", "lym drift amend <slug>")
			if !f.untouched() || f.compares != 1 {
				t.Fatalf("judge untouched %v, %d compares; want the compare run once and no judge", f.untouched(), f.compares)
			}

			err = WriteRecords(f.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
				scope.Amendments = []Amendment{{By: "parent", At: "2026-01-02T00:00:00Z", Kind: amendmentKindEntries, Entries: expectsCommandCase}}
				return nil
			})
			if err != nil {
				t.Fatalf("WriteRecords() = %v; want nil", err)
			}
			f.caseRulings = map[string]Ruling{"a.txt": RulingMatches}

			requireDone(f)

			if f.seats.Calls != 1 || len(f.records().Judged.Cases) != 0 {
				t.Errorf("%d judge runs and stored cases %+v; want one judge and the old ruling dropped once the case matches", f.seats.Calls, f.records().Judged.Cases)
			}
		})
	})

	t.Run("the Drift-Recheck row runs the same pass", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.row = "Drift-Recheck"
		f.seedChanged(expectsY)

		requireDone(f)

		if f.seats.Calls != 1 || !f.records().Scope.LastCheck.Passed {
			t.Errorf("%d judge runs, last check %+v; want one judge and a pass", f.seats.Calls, f.records().Scope.LastCheck)
		}
	})

	t.Run("a session an earlier entry left open", func(t *testing.T) {
		t.Parallel()

		t.Run("a live chair is resumed and none is respawned, each under the recorded spawn id", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name        string
				liveChair   bool
				wantRuns    int
				wantResumes int
			}{
				{"ALiveChairIsResumed", true, 0, 1},
				{"NoLiveChairRespawns", false, 1, 0},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					f := newVerifyFixture(t)
					f.seedChanged(expectsY)
					f.seedMark("judge-recorded", true)
					f.liveChair = tt.liveChair

					requireDone(f)

					if f.seats.Calls != tt.wantRuns || f.seats.ResumeCalls != tt.wantResumes || f.seats.ProbeCalls == 0 {
						t.Errorf("%d runs, %d resumes and %d probes; want %d runs, %d resumes and a probe", f.seats.Calls, f.seats.ResumeCalls, f.seats.ProbeCalls, tt.wantRuns, tt.wantResumes)
					}
					if len(f.requests) != 1 || f.requests[0].SpawnID != "judge-recorded" {
						t.Errorf("requests = %+v; want the one judge under the recorded spawn id", f.requests)
					}
					if f.scope().JudgeSession.Open || len(f.records().Judged.Tuples) != 1 {
						t.Errorf("scope mark %+v and %d tuples; want the mark closed and the ruling stored", f.scope().JudgeSession, len(f.records().Judged.Tuples))
					}
				})
			}
		})

		t.Run("a change loom's own commits made is no judge edit", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name    string
				changed string
			}{
				{"TheRowsStatusCommit", testStatusRel},
				{"AFrictionNoteCommit", testFrictionRel + "/halt.md"},
				{"AScopeRecordCommit", "_lym/drift/scope.json"},
				{"AJudgedRecordCommit", "_lym/drift/judged.json"},
				{"ACompareLog", "_lym/drift/logs/compare-1.log"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					f := newVerifyFixture(t)
					f.seedChanged(expectsY)
					f.seedMark("judge-recorded", true)
					writeTestFile(t, f.recordsCheckout.Root, tt.changed, "{}")
					f.recordsCheckout.Head, f.recordsChanged = "records-2", []string{tt.changed}

					requireDone(f)

					if f.seats.Calls != 1 {
						t.Errorf("%d judge runs; want the judge resumed", f.seats.Calls)
					}
				})
			}
		})

		t.Run("a gone snapshot with no live chair takes a new one and goes on under the recorded spawn id", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedChanged(expectsY)
			f.seedMark("judge-recorded", false)
			f.recordsCheckout.Root = f.store.root
			f.onArrival = func() {
				f.recordsCheckout.Head, f.recordsChanged = "records-2", []string{"_lym/drift/scope.json", "_lym/drift/judged.json"}
			}

			requireDone(f)

			if _, found, err := ReadJudgeSnapshot(f.deps); err != nil || !found {
				t.Errorf("ReadJudgeSnapshot() = _, %v, %v; want the new snapshot", found, err)
			}
			if f.scope().SnapshotLost || f.scope().JudgeSession.Open || len(f.requests) != 1 || f.requests[0].SpawnID != "judge-recorded" {
				t.Errorf("scope = lost %v, mark %+v; requests %+v; want the flag cleared, the mark closed and the recorded spawn id kept", f.scope().SnapshotLost, f.scope().JudgeSession, f.requests)
			}
		})

		t.Run("a gone snapshot under a live chair stops recording it lost, and the next entry takes a new snapshot and resumes", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedChanged(expectsY)
			f.seedMark("judge-recorded", false)
			f.liveChair = true

			outcome, pointer, err := f.call()

			requireStuck(t, outcome, pointer, err, true, "while its session was live", "cannot be told", "lym loom resume")
			requireDeferPrefix(t, pointer.Reason, false)
			if !f.scope().SnapshotLost || f.seats.Calls+f.seats.ResumeCalls != 0 {
				t.Fatalf("snapshot lost %v, %d runs and %d resumes; want it recorded lost and no judge", f.scope().SnapshotLost, f.seats.Calls, f.seats.ResumeCalls)
			}

			requireDone(f)

			if f.seats.ResumeCalls != 1 || f.scope().SnapshotLost || f.scope().JudgeSession.Open {
				t.Errorf("%d resumes, lost %v, mark %+v; want the chair resumed, the flag cleared and the mark closed", f.seats.ResumeCalls, f.scope().SnapshotLost, f.scope().JudgeSession)
			}
		})

		t.Run("a snapshot gone while the judge ran stops, and the next entry takes a new one", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedChanged(expectsY)
			f.onArrival = func() {
				if err := os.RemoveAll(f.deps.JudgeSnapshotDir); err != nil {
					t.Fatal(err)
				}
			}

			outcome, pointer, err := f.call()

			requireStuck(t, outcome, pointer, err, true, "while its session was live", "lym loom resume")
			if !f.scope().SnapshotLost || !f.scope().JudgeSession.Open {
				t.Fatalf("snapshot lost %v, mark %+v; want it recorded lost and the mark left open", f.scope().SnapshotLost, f.scope().JudgeSession)
			}

			f.onArrival = nil
			requireDone(f)

			if f.scope().SnapshotLost || f.scope().JudgeSession.Open {
				t.Errorf("lost %v, mark %+v; want the flag cleared and the mark closed", f.scope().SnapshotLost, f.scope().JudgeSession)
			}
		})
	})

	t.Run("a change the judge session makes stops the pass, and a resume stops again until it is undone", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			edit     func(t *testing.T, f *verifyFixture)
			undo     func(t *testing.T, f *verifyFixture)
			fragment string
		}{
			{
				name: "ATrackedFile",
				edit: func(t *testing.T, f *verifyFixture) {
					writeTestFile(t, f.codeCheckout.Root, "a.go", "edited")
					f.codeCheckout.Changed = []string{"a.go"}
				},
				undo: func(t *testing.T, f *verifyFixture) {
					removeTestFile(t, f.codeCheckout.Root, "a.go")
					f.codeCheckout.Changed = nil
				},
				fragment: "code: a.go was changed",
			},
			{
				name: "AnAddedUntrackedFile",
				edit: func(t *testing.T, f *verifyFixture) {
					writeTestFile(t, f.codeCheckout.Root, "new.txt", "added")
					f.codeCheckout.Untracked = []string{"new.txt"}
				},
				undo: func(t *testing.T, f *verifyFixture) {
					removeTestFile(t, f.codeCheckout.Root, "new.txt")
					f.codeCheckout.Untracked = nil
				},
				fragment: "code: new.txt was added",
			},
			{
				name:     "ACommitOnTheCodeSide",
				edit:     func(_ *testing.T, f *verifyFixture) { f.commit()() },
				undo:     func(_ *testing.T, f *verifyFixture) { f.codeCheckout.Head = "code-1" },
				fragment: `code: HEAD moved from "code-1" to "code-2"`,
			},
			{
				name: "ACommitOnTheRecordsSide",
				edit: func(t *testing.T, f *verifyFixture) {
					writeTestFile(t, f.recordsCheckout.Root, "notes.md", "added")
					f.recordsCheckout.Head, f.recordsChanged = "records-2", []string{"notes.md"}
				},
				undo: func(t *testing.T, f *verifyFixture) {
					removeTestFile(t, f.recordsCheckout.Root, "notes.md")
					f.recordsCheckout.Head, f.recordsChanged = "records-1", nil
				},
				fragment: "records: notes.md was changed",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				f.seedChanged(expectsY)
				f.onArrival = func() { tt.edit(t, f) }

				outcome, pointer, err := f.call()

				requireStuck(t, outcome, pointer, err, true, tt.fragment, f.deps.JudgeSnapshotDir, f.captureDir, "keeps the snapshot directory", "lym loom resume")
				requireDeferPrefix(t, pointer.Reason, false)
				if !f.scope().JudgeSession.Open || len(f.records().Judged.Tuples) != 0 {
					t.Fatalf("scope mark %+v and %d tuples; want the mark left open and nothing stored", f.scope().JudgeSession, len(f.records().Judged.Tuples))
				}
				spawn := f.requests[0].SpawnID

				outcome, pointer, err = f.call()

				requireStuck(t, outcome, pointer, err, true, tt.fragment)
				if f.seats.Calls != 1 {
					t.Fatalf("%d judge runs after a resume with the edit still there; want the guard to stop it with no spawn", f.seats.Calls)
				}

				f.onArrival = nil
				tt.undo(t, f)
				requireDone(f)

				if f.seats.Calls != 2 || f.requests[1].SpawnID != spawn || f.scope().JudgeSession.Open {
					t.Errorf("%d runs, resumed spawn %q, mark %+v; want a respawn under %q and a closed mark", f.seats.Calls, f.requests[1].SpawnID, f.scope().JudgeSession, spawn)
				}
			})
		}

		for _, name := range []string{scopeFileName, judgedFileName} {
			t.Run("AHandEditOf_"+name+"_StopsAndClearsWithARestore", func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				f.seedChanged(expectsY)
				rel := durableRel(name)
				f.onArrival = func() {
					// An empty judged record serializes as {}, so the edit must differ from that to be a hand edit.
					writeTestFile(t, f.store.root, rel, `{"hand_edit": true}`)
					writeTestFile(t, f.recordsCheckout.Root, rel, `{"hand_edit": true}`)
					f.recordsCheckout.Changed = []string{rel}
				}

				outcome, pointer, err := f.call()

				requireStuck(t, outcome, pointer, err, true, "records: "+rel+" was changed")

				f.onArrival = nil
				outcome, pointer, err = f.call()

				requireStuck(t, outcome, pointer, err, true, "lym drift amend <slug> --restore")

				if err := RestoreRecords(f.deps); err != nil {
					t.Fatalf("RestoreRecords() = %v; want nil", err)
				}
				removeTestFile(t, f.recordsCheckout.Root, rel)
				f.recordsCheckout.Changed = nil
				requireDone(f)
			})
		}

		t.Run("AChangeMadeBeforeTheSpawnIsNoJudgeEdit", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedChanged(expectsY)
			writeTestFile(t, f.codeCheckout.Root, "a.go", "before")
			f.codeCheckout.Changed = []string{"a.go"}

			requireDone(f)
		})

		t.Run("ABaselineEditedDuringTheSessionStopsNamingDefer", func(t *testing.T) {
			t.Parallel()
			f := newVerifyFixture(t)
			f.seedChanged(expectsY)
			f.onArrival = func() { writeTree(t, f.paths.BaselineDir, map[string]string{"a.txt": "x=1\ny=9\n"}) }

			outcome, pointer, err := f.call()

			requireStuck(t, outcome, pointer, err, true, "lym drift defer")
			requireDeferPrefix(t, pointer.Reason, true)
			if !f.scope().JudgeSession.Open || len(f.records().Judged.Tuples) != 0 {
				t.Errorf("scope mark %+v and %d tuples; want the mark left open and nothing stored", f.scope().JudgeSession, len(f.records().Judged.Tuples))
			}
		})
	})

	t.Run("a judgement entry that fails every attempt ends Stuck and the next entry respawns the judge", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		f.badJudgement = true

		outcome, pointer, err := f.call()

		if err != nil || outcome != shedengine.Stuck || strings.HasPrefix(pointer.Reason, shedadapters.DecisionHaltPrefix) {
			t.Fatalf("Call() = %q, %v (%s); want the shed's Stuck with no decision prefix", outcome, err, pointer.Reason)
		}
		if !f.scope().JudgeSession.Open || len(f.records().Judged.Tuples) != 0 {
			t.Fatalf("scope mark %+v and %d tuples; want the mark left open and nothing stored", f.scope().JudgeSession, len(f.records().Judged.Tuples))
		}

		f.badJudgement = false
		requireDone(f)

		if f.seats.Calls != 2 || f.requests[len(f.requests)-1].SpawnID != f.requests[0].SpawnID {
			t.Errorf("%d runs, spawn ids %q and %q; want a respawn under the same spawn id", f.seats.Calls, f.requests[0].SpawnID, f.requests[len(f.requests)-1].SpawnID)
		}
	})
}

// ptr returns a pointer to a copy of value.
func ptr[T any](value T) *T { return &value }
