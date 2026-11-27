// rowgate_test.go covers WrapRowGate over a scripted fixer and scripted inner gate closures.
// The closures run against a temp visit directory.

package gatefixer

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeFixer is a Fixer scripted by its fields.
// It records its spawns and stops and answers running a set number of times before it ends.
// It can fail its spawn, its wait or its stop.
type fakeFixer struct {
	mu sync.Mutex
	// runningFirst is how many waits answer running before the terminal answer.
	runningFirst int
	// reply is the reply of a fixer that ends done; a non-empty deadReason makes it end dead instead.
	reply      string
	deadReason string
	spawnErr   error
	awaitErr   error
	stopErr    error

	// onAwait, when set, runs at the start of every wait, outside the fixer's lock.
	onAwait func()

	specs  []Spec
	stops  []string
	awaits int
	polls  int
}

func (f *fakeFixer) SpawnOrAttach(store Store, spec Spec) (*Record, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.spawnErr != nil {
		return nil, false, f.spawnErr
	}
	f.specs = append(f.specs, spec)
	rec := &Record{StrandGUID: fmt.Sprintf("fix-%d", len(f.specs)), SpawnedAt: "1970-01-01T00:00:00Z", ReportPath: spec.ReportPath, ReplyPath: spec.ReplyPath}
	return rec, true, store.Save(rec)
}

func (f *fakeFixer) Await(rec *Record, timeout, wait time.Duration) (*Result, error) {
	if f.onAwait != nil {
		f.onAwait()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.awaits++
	if f.awaitErr != nil {
		return nil, f.awaitErr
	}
	f.polls++
	if f.polls <= f.runningFirst {
		return &Result{Status: StatusRunning}, nil
	}
	f.polls = 0
	if f.deadReason != "" {
		return &Result{Status: StatusDead, DeadReason: f.deadReason}, nil
	}
	return &Result{Status: StatusDone, Reply: f.reply}, nil
}

func (f *fakeFixer) Stop(rec *Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, rec.StrandGUID)
	return f.stopErr
}

// scriptedGate answers each call with the next scripted result, the last one repeating, and counts its calls.
type scriptedGate struct {
	results []shuttleengine.GateResult
	err     error
	calls   int
}

func (g *scriptedGate) Gate() (shuttleengine.GateResult, error) {
	result := g.results[min(g.calls, len(g.results)-1)]
	g.calls++
	return result, g.err
}

func passing() shuttleengine.GateResult { return shuttleengine.GateResult{Passed: true} }

func failing(findings string) shuttleengine.GateResult {
	return shuttleengine.GateResult{Findings: findings}
}

// rowHarness is a row fixer over a fakeFixer and a temp visit directory, for the entry named rowEntry.
type rowHarness struct {
	fixer    *fakeFixer
	rf       RowFixer
	visitDir string
	// visitErr, when set, is what VisitDir returns.
	visitErr error
}

const rowEntry = "verify"

func newRowHarness(t *testing.T) *rowHarness {
	t.Helper()
	h := &rowHarness{fixer: &fakeFixer{reply: "fixed it"}, visitDir: t.TempDir()}
	h.rf = RowFixer{
		Fixer: h.fixer,
		VisitDir: func() (string, error) {
			return h.visitDir, h.visitErr
		},
		Spec: func(reportPath, replyPath string) (Spec, error) {
			return Spec{ReportPath: reportPath, ReplyPath: replyPath, Role: RoleGateFixer}, nil
		},
		Timeout: time.Minute,
	}
	return h
}

func (h *rowHarness) store() rowStore { return newRowStore(h.visitDir, rowEntry) }

// seed saves records into the entry's set.
func (h *rowHarness) seed(t *testing.T, records ...Record) {
	t.Helper()
	for i := range records {
		if err := h.store().Save(&records[i]); err != nil {
			t.Fatalf("seed record %q: %v", records[i].StrandGUID, err)
		}
	}
}

// statuses returns the status of every record of the entry's set, in order.
func (h *rowHarness) statuses(t *testing.T) []string {
	t.Helper()
	records, err := h.store().load()
	if err != nil {
		t.Fatalf("load records: %v", err)
	}
	var statuses []string
	for _, rec := range records {
		statuses = append(statuses, rec.Status)
	}
	return statuses
}

// wrap wraps a one-entry spec around entry and returns the wrapped entry.
func (h *rowHarness) wrap(t *testing.T, entry shuttleengine.GateEntry) shuttleengine.GateEntry {
	t.Helper()
	wrapped, err := WrapRowGate(shuttleengine.GateSpec{entry}, entry.Name, h.rf)
	if err != nil {
		t.Fatalf("WrapRowGate() error = %v; want none", err)
	}
	return wrapped[0]
}

func verifyEntry(gate *scriptedGate, attempts int) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{Name: rowEntry, Gate: gate.Gate, Attempts: attempts}
}

// TestWrapRowGate pins the fixer hand-off of the entry that carries the fixer, the wait of every other entry and the wrap's refusals.
func TestWrapRowGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("InnerOutcomesThatSpawnNothing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			gate         scriptedGate
			wantPassed   bool
			wantFindings string
			wantErr      error
		}{
			{"Passing", scriptedGate{results: []shuttleengine.GateResult{passing()}}, true, "", nil},
			{"TerminalFailure", scriptedGate{results: []shuttleengine.GateResult{{Findings: "stop", Terminal: true}}}, false, "stop", nil},
			{"Error", scriptedGate{results: []shuttleengine.GateResult{{}}, err: errors.New("infra")}, false, "", errors.New("infra")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				entry := h.wrap(t, verifyEntry(&tt.gate, 2))

				got, err := entry.Gate()
				if (err == nil) != (tt.wantErr == nil) || (err != nil && err.Error() != tt.wantErr.Error()) {
					t.Fatalf("Gate() error = %v; want %v", err, tt.wantErr)
				}
				if got.Passed != tt.wantPassed || got.Findings != tt.wantFindings {
					t.Errorf("Gate() = %+v; want passed %v and findings %q", got, tt.wantPassed, tt.wantFindings)
				}
				if len(h.fixer.specs) != 0 || tt.gate.calls != 1 {
					t.Errorf("spawns = %d, inner calls = %d; want none and 1", len(h.fixer.specs), tt.gate.calls)
				}
			})
		}
	})

	t.Run("FailureSpawnsOneFixerWaitsAndJudgesAgain", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.fixer.runningFirst = 2
		gate := &scriptedGate{results: []shuttleengine.GateResult{failing("lint broke"), passing()}}
		entry := h.wrap(t, verifyEntry(gate, 2))

		got, err := entry.Gate()
		if err != nil || !got.Passed {
			t.Fatalf("Gate() = %+v, %v; want a pass", got, err)
		}
		if len(h.fixer.specs) != 1 || gate.calls != 2 || h.fixer.awaits != 3 {
			t.Errorf("spawns = %d, inner calls = %d, waits = %d; want 1, 2 and 3", len(h.fixer.specs), gate.calls, h.fixer.awaits)
		}
		report, err := os.ReadFile(h.fixer.specs[0].ReportPath)
		if err != nil || string(report) != "lint broke" {
			t.Errorf("report = %q, %v; want the inner findings", report, err)
		}
		if got := h.statuses(t); len(got) != 1 || got[0] != StatusDone {
			t.Errorf("record statuses = %v; want one done record", got)
		}
	})

	t.Run("ReRunFailureCarriesNewFindingsAndTheFixersAnswer", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			deadReason string
			wantNote   string
		}{
			{"Reply", "", "fixed it"},
			{"DeadFixer", DeadReasonTimeout, "ended without a reply (timeout); any change it left uncommitted is in the worktree"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				h.fixer.deadReason = tt.deadReason
				gate := &scriptedGate{results: []shuttleengine.GateResult{failing("first"), failing("second")}}
				entry := h.wrap(t, verifyEntry(gate, 2))

				got, err := entry.Gate()
				if err != nil || got.Passed {
					t.Fatalf("Gate() = %+v, %v; want a failure", got, err)
				}
				if !strings.HasPrefix(got.Findings, "second") || strings.Contains(got.Findings, "first") || !strings.Contains(got.Findings, "## Cold fixer reply") || !strings.Contains(got.Findings, tt.wantNote) {
					t.Errorf("Gate().Findings = %q; want the new findings, then a reply section holding %q", got.Findings, tt.wantNote)
				}
				wantStatus := StatusDone
				if tt.deadReason != "" {
					wantStatus = StatusDead
				}
				if got := h.statuses(t); len(got) != 1 || got[0] != wantStatus {
					t.Errorf("record statuses = %v; want one %s record", got, wantStatus)
				}
			})
		}
	})

	t.Run("ForkedFailureSpawnsUnlessSkipForkedHoldsIt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			skip       bool
			wantSpawns int
		}{
			{"SkipForkedFalse", false, 1},
			{"SkipForkedTrue", true, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				h.rf.SkipForked = func(findings string) bool { return tt.skip && findings == "forked broke" }
				plain := &scriptedGate{results: []shuttleengine.GateResult{passing()}}
				forked := &scriptedGate{results: []shuttleengine.GateResult{failing("forked broke"), passing()}}
				original := verifyEntry(plain, 2)
				original.Forked = forked.Gate
				entry := h.wrap(t, original)

				got, err := entry.Forked()
				if err != nil {
					t.Fatalf("Forked() error = %v; want none", err)
				}
				if len(h.fixer.specs) != tt.wantSpawns || plain.calls != 0 {
					t.Errorf("spawns = %d, plain calls = %d; want %d and 0", len(h.fixer.specs), plain.calls, tt.wantSpawns)
				}
				if wantPassed := tt.wantSpawns == 1; got.Passed != wantPassed {
					t.Errorf("Forked().Passed = %v; want %v", got.Passed, wantPassed)
				}
			})
		}
	})

	t.Run("SpawnErrorLeavesTheInnerFailureStandingWithNoRecord", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.fixer.spawnErr = errors.New("no provider")
		gate := &scriptedGate{results: []shuttleengine.GateResult{failing("lint broke")}}
		entry := h.wrap(t, verifyEntry(gate, 2))

		got, err := entry.Gate()
		if err != nil || got.Passed {
			t.Fatalf("Gate() = %+v, %v; want a failure and no error", got, err)
		}
		if !strings.HasPrefix(got.Findings, "lint broke") || !strings.Contains(got.Findings, "no provider") || !strings.Contains(got.Findings, "no fixer ran") {
			t.Errorf("Gate().Findings = %q; want the inner findings and a line naming the spawn error and that no fixer ran", got.Findings)
		}
		if count, _ := h.store().count(); count != 0 || gate.calls != 1 {
			t.Errorf("records = %d, inner calls = %d; want none and 1", count, gate.calls)
		}
	})

	t.Run("ASetAtAttemptsPlusOneSpawnsNone", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.seed(t, Record{StrandGUID: "a", Status: StatusDone}, Record{StrandGUID: "b", Status: StatusDead}, Record{StrandGUID: "c", Status: StatusDone})
		gate := &scriptedGate{results: []shuttleengine.GateResult{failing("lint broke")}}
		entry := h.wrap(t, verifyEntry(gate, 2))

		got, err := entry.Gate()
		if err != nil || got.Passed || got.Findings != "lint broke" || len(h.fixer.specs) != 0 {
			t.Errorf("Gate() = %+v, %v with %d spawns; want the unchanged failure and no spawn", got, err, len(h.fixer.specs))
		}
	})

	t.Run("ALiveRecordIsWaitedOnAndTheEntryJudgesOnce", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			results      []shuttleengine.GateResult
			wantPassed   bool
			wantFindings string
		}{
			{"Passing", []shuttleengine.GateResult{passing()}, true, ""},
			{"FailingFoldsTheReply", []shuttleengine.GateResult{failing("still broken")}, false, "still broken\n\n## Cold fixer reply\n\nfixed it"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				h.fixer.runningFirst = 1
				h.seed(t, Record{StrandGUID: "left-live"})
				gate := &scriptedGate{results: tt.results}
				entry := h.wrap(t, verifyEntry(gate, 2))

				got, err := entry.Gate()
				if err != nil || got.Passed != tt.wantPassed || got.Findings != tt.wantFindings {
					t.Errorf("Gate() = %+v, %v; want passed %v and findings %q", got, err, tt.wantPassed, tt.wantFindings)
				}
				if len(h.fixer.specs) != 0 || gate.calls != 1 || h.fixer.awaits != 2 {
					t.Errorf("spawns = %d, inner calls = %d, waits = %d; want none, 1 and 2", len(h.fixer.specs), gate.calls, h.fixer.awaits)
				}
				if got := h.statuses(t); len(got) != 1 || got[0] != StatusDone {
					t.Errorf("record statuses = %v; want the left-live record marked done", got)
				}
			})
		}
	})

	t.Run("AnEarlierEntryThatPassesLeavesTheRecordLiveForTheFixerEntry", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.seed(t, Record{StrandGUID: "left-live"})
		spec := shuttleengine.GateSpec{
			{Name: "clean", Gate: (&scriptedGate{results: []shuttleengine.GateResult{passing()}}).Gate, Attempts: 1},
			verifyEntry(&scriptedGate{results: []shuttleengine.GateResult{failing("verify broke")}}, 2),
		}
		wrapped, err := WrapRowGate(spec, rowEntry, h.rf)
		if err != nil {
			t.Fatalf("WrapRowGate() error = %v; want none", err)
		}

		if got, err := wrapped[0].Gate(); err != nil || !got.Passed {
			t.Fatalf("clean Gate() = %+v, %v; want a pass", got, err)
		}
		if got := h.statuses(t); len(got) != 1 || got[0] != "" {
			t.Errorf("record statuses after the passing entry = %v; want the record still live", got)
		}
		if expired, err := wrapped[1].Expired(); err != nil || !expired {
			t.Errorf("Expired() = %v, %v between the entries; want true for a restarted driver", expired, err)
		}

		got, err := wrapped[1].Gate()
		if err != nil || got.Passed || got.Findings != "verify broke\n\n## Cold fixer reply\n\nfixed it" {
			t.Errorf("verify Gate() = %+v, %v; want the failure carrying the fixer's reply", got, err)
		}
		if len(h.fixer.specs) != 0 {
			t.Errorf("spawns = %d; want none, since the fixer already ran", len(h.fixer.specs))
		}
		if got := h.statuses(t); len(got) != 1 || got[0] != StatusDone {
			t.Errorf("record statuses after the failing entry = %v; want the record terminal", got)
		}
	})

	t.Run("AnEarlierEntryThatIsPendingLeavesTheRecordLive", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.seed(t, Record{StrandGUID: "left-live"})
		spec := shuttleengine.GateSpec{
			{Name: "clean", Gate: (&scriptedGate{results: []shuttleengine.GateResult{{Pending: true}}}).Gate, Attempts: 1},
			verifyEntry(&scriptedGate{results: []shuttleengine.GateResult{passing()}}, 2),
		}
		wrapped, err := WrapRowGate(spec, rowEntry, h.rf)
		if err != nil {
			t.Fatalf("WrapRowGate() error = %v; want none", err)
		}

		if got, err := wrapped[0].Gate(); err != nil || !got.Pending {
			t.Fatalf("clean Gate() = %+v, %v; want pending", got, err)
		}
		if got := h.statuses(t); len(got) != 1 || got[0] != "" {
			t.Errorf("record statuses = %v; want the record still live", got)
		}
	})

	t.Run("AnInnerErrorAfterTheWaitLeavesTheRecordLive", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.seed(t, Record{StrandGUID: "left-live"})
		gate := &scriptedGate{results: []shuttleengine.GateResult{passing()}, err: errors.New("infra")}
		entry := h.wrap(t, verifyEntry(gate, 2))

		if _, err := entry.Gate(); err == nil || !strings.Contains(err.Error(), "infra") {
			t.Fatalf("Gate() error = %v; want the inner error", err)
		}
		if got := h.statuses(t); len(got) != 1 || got[0] != "" {
			t.Errorf("record statuses = %v; want the record still live", got)
		}
	})

	t.Run("AWaitErrorStopsTheStrandAndMarksItDead", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			stopErr      error
			wantErr      bool
			wantStatuses []string
			wantCalls    int
		}{
			{"StopSucceeds", nil, false, []string{StatusDead}, 1},
			{"StopFails", errors.New("reed gone"), true, []string{""}, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				h.fixer.awaitErr = errors.New("events unreadable")
				h.fixer.stopErr = tt.stopErr
				h.seed(t, Record{StrandGUID: "left-live"})
				gate := &scriptedGate{results: []shuttleengine.GateResult{passing()}}
				entry := h.wrap(t, verifyEntry(gate, 2))

				_, err := entry.Gate()
				if (err != nil) != tt.wantErr || (err != nil && (!strings.Contains(err.Error(), "events unreadable") || !strings.Contains(err.Error(), "reed gone"))) {
					t.Fatalf("Gate() error = %v; want an error joining the wait and stop errors: %v", err, tt.wantErr)
				}
				if len(h.fixer.stops) != 1 || gate.calls != tt.wantCalls {
					t.Errorf("stops = %v, inner calls = %d; want one stop and %d calls", h.fixer.stops, gate.calls, tt.wantCalls)
				}
				statuses := h.statuses(t)
				if len(statuses) != 1 || statuses[0] != tt.wantStatuses[0] {
					t.Errorf("record statuses = %v; want %v", statuses, tt.wantStatuses)
				}
				if records, _ := h.store().load(); !tt.wantErr && records[0].DeadReason != DeadReasonDied {
					t.Errorf("dead reason = %q; want %q", records[0].DeadReason, DeadReasonDied)
				}
			})
		}
	})

	t.Run("FinalNeverSpawnsAndWaitsOnlyForALiveRecord", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			forked     bool
			live       bool
			wantPlain  int
			wantForked int
		}{
			{"NoLiveRecordRunsGateOnce", false, false, 1, 0},
			{"LiveRecordWithPlainGate", false, true, 1, 0},
			{"LiveRecordWithForkedGate", true, true, 0, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				h.rf.Forked = tt.forked
				if tt.live {
					h.seed(t, Record{StrandGUID: "left-live"})
				}
				plain := &scriptedGate{results: []shuttleengine.GateResult{failing("plain broke")}}
				forked := &scriptedGate{results: []shuttleengine.GateResult{failing("forked broke")}}
				original := verifyEntry(plain, 2)
				original.Forked = forked.Gate
				entry := h.wrap(t, original)

				if !entry.MayHold {
					t.Error("MayHold = false; want true, since shuttle accepts a Final only on a holding entry")
				}
				got, err := entry.Final()
				if err != nil || got.Passed {
					t.Fatalf("Final() = %+v, %v; want a failure", got, err)
				}
				if plain.calls != tt.wantPlain || forked.calls != tt.wantForked || len(h.fixer.specs) != 0 {
					t.Errorf("plain calls = %d, forked calls = %d, spawns = %d; want %d, %d and none", plain.calls, forked.calls, len(h.fixer.specs), tt.wantPlain, tt.wantForked)
				}
				if folded := strings.Contains(got.Findings, "## Cold fixer reply"); folded != tt.live {
					t.Errorf("Final().Findings = %q; want the reply folded in: %v", got.Findings, tt.live)
				}
			})
		}
	})

	t.Run("OtherEntriesWaitForALiveFixerBeforeJudging", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			deadReason   string
			live         bool
			wantFindings string
			wantWaits    int
		}{
			{"NoLiveRecordRunsAtOnce", "", false, "other broke", 0},
			{"LiveRecordFoldsTheReply", "", true, "other broke\n\n## Cold fixer reply\n\nfixed it", 1},
			{"LiveRecordFoldsADeadNote", DeadReasonAsking, true, "other broke\n\n## Cold fixer reply\n\nThe cold fixer ended without a reply (asking); any change it left uncommitted is in the worktree.", 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newRowHarness(t)
				h.fixer.deadReason = tt.deadReason
				if tt.live {
					h.seed(t, Record{StrandGUID: "left-live"})
				}
				other := &scriptedGate{results: []shuttleengine.GateResult{failing("other broke")}}
				spec := shuttleengine.GateSpec{
					{Name: "other", Gate: other.Gate, Attempts: 1},
					verifyEntry(&scriptedGate{results: []shuttleengine.GateResult{passing()}}, 2),
				}
				wrapped, err := WrapRowGate(spec, rowEntry, h.rf)
				if err != nil {
					t.Fatalf("WrapRowGate() error = %v; want none", err)
				}

				got, err := wrapped[0].Gate()
				if err != nil || got.Findings != tt.wantFindings {
					t.Errorf("Gate() = %+v, %v; want findings %q", got, err, tt.wantFindings)
				}
				if h.fixer.awaits < tt.wantWaits || other.calls != 1 {
					t.Errorf("waits = %d, inner calls = %d; want at least %d and 1", h.fixer.awaits, other.calls, tt.wantWaits)
				}
				if wrapped[0].Forked != nil {
					t.Error("Forked set on an entry that had none; want it left nil")
				}
			})
		}
	})

	t.Run("ExpiredIsTrueOnlyForALiveRecordAFreshWrapDidNotSpawn", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		h.seed(t, Record{StrandGUID: "left-live"})
		fresh := h.wrap(t, verifyEntry(&scriptedGate{results: []shuttleengine.GateResult{passing()}}, 2))
		if expired, err := fresh.Expired(); err != nil || !expired {
			t.Errorf("Expired() = %v, %v with a live record from an earlier driver; want true", expired, err)
		}

		if _, err := fresh.Gate(); err != nil {
			t.Fatalf("Gate() error = %v; want none", err)
		}
		if expired, err := fresh.Expired(); err != nil || expired {
			t.Errorf("Expired() = %v, %v once the record is terminal; want false", expired, err)
		}

		spawning := newRowHarness(t)
		spawnGate := &scriptedGate{results: []shuttleengine.GateResult{failing("broke"), passing()}}
		spawningEntry := spawning.wrap(t, verifyEntry(spawnGate, 2))
		var expiredWhileWaiting []bool
		spawning.fixer.onAwait = func() {
			expired, err := spawningEntry.Expired()
			if err != nil {
				t.Errorf("Expired() error = %v; want none", err)
			}
			expiredWhileWaiting = append(expiredWhileWaiting, expired)
		}
		if _, err := spawningEntry.Gate(); err != nil {
			t.Fatalf("Gate() error = %v; want none", err)
		}
		if len(expiredWhileWaiting) == 0 || slices.Contains(expiredWhileWaiting, true) {
			t.Errorf("Expired() while waiting on the fixer this wrap spawned = %v; want only false", expiredWhileWaiting)
		}
	})

	t.Run("EntriesKeepTheirOtherFields", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		original := shuttleengine.GateEntry{
			Name: rowEntry, Gate: (&scriptedGate{results: []shuttleengine.GateResult{passing()}}).Gate, Attempts: 4, PassOnCap: true,
			Reprompt: func(path string) string { return "fix " + path },
		}
		entry := h.wrap(t, original)
		if entry.Name != rowEntry || entry.Attempts != 4 || !entry.PassOnCap || entry.Reprompt("x") != "fix x" {
			t.Errorf("wrapped entry = %+v; want name, attempts, pass-on-cap and re-prompt kept", entry)
		}
	})

	t.Run("RefusalsAndInfrastructureErrors", func(t *testing.T) {
		t.Parallel()
		h := newRowHarness(t)
		gate := &scriptedGate{results: []shuttleengine.GateResult{passing()}}
		if _, err := WrapRowGate(shuttleengine.GateSpec{verifyEntry(gate, 2)}, "missing", h.rf); err == nil || !strings.Contains(err.Error(), `"missing"`) {
			t.Errorf("WrapRowGate(unknown entry) error = %v; want one naming the entry", err)
		}

		h.visitErr = errors.New("no visit")
		entry := h.wrap(t, verifyEntry(gate, 2))
		if _, err := entry.Gate(); err == nil || !strings.Contains(err.Error(), "no visit") {
			t.Errorf("Gate() error = %v; want the visit directory error", err)
		}
	})
}
