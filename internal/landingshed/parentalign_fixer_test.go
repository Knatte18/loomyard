//go:build tier0

// parentalign_fixer_test.go drives Parent-Align's alignment through the merge fixer loop over the Parent-Align fixture's fake seat runner, tips, gate and line-change reads.

package landingshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// fixerReportPath is the report the fixer session writes for Merge-In.
func (fx *mergeInFixture) fixerReportPath() string {
	return filepath.Join(fx.m.deps.ScratchDir, parentAlignName+"-fixer-report.md")
}

// fixerTurn scripts a turn that writes content as its report and moves the task head to head, as a turn that commits does.
func (fx *mergeInFixture) fixerTurn(t *testing.T, content, head string) func() {
	t.Helper()
	return func() {
		if err := os.WriteFile(fx.fixerReportPath(), []byte(content), 0o644); err != nil {
			t.Errorf("write fixer report: %v", err)
		}
		fx.head = head
	}
}

// scriptGate makes the n-th verify answer results[n], and every later one the last result.
func (fx *mergeInFixture) scriptGate(results ...verifytree.Result) {
	fx.gate.fake.onVerify = func() {
		fx.events = append(fx.events, "verify")
		fx.gate.fake.result = results[min(fx.gate.fake.verifyCalls-1, len(results)-1)]
	}
}

// seedAttempts writes rec as the entry's attempt record.
func (fx *mergeInFixture) seedAttempts(t *testing.T, rec FixerAttempts) {
	t.Helper()
	if err := WriteFixerAttempts(FixerAttemptsPath(fx.m.deps.ScratchDir, parentAlignName), rec); err != nil {
		t.Fatal(err)
	}
}

// attemptsOnDisk returns the attempt record for the fixture's entry and whether the file exists.
func (fx *mergeInFixture) attemptsOnDisk(t *testing.T) (FixerAttempts, bool) {
	t.Helper()
	path := FixerAttemptsPath(fx.m.deps.ScratchDir, parentAlignName)
	if _, err := os.Stat(path); err != nil {
		return FixerAttempts{}, false
	}
	rec, err := ReadFixerAttempts(path, fx.entry)
	if err != nil {
		t.Fatalf("ReadFixerAttempts() error = %v", err)
	}
	return rec, true
}

// requireHalt asserts the call ended in a halting Stuck whose reason opens with LandingStopPrefix and holds every fragment, and returns the reason.
func requireHalt(t *testing.T, outcome shedengine.Outcome, ptr shedengine.OutputPointer, fragments ...string) string {
	t.Helper()
	if outcome != shedengine.Stuck || !ptr.Halt || !strings.HasPrefix(ptr.Reason, LandingStopPrefix) {
		t.Fatalf("Call() = %q, %+v; want a halting Stuck opening with %q", outcome, ptr, LandingStopPrefix)
	}
	for _, want := range fragments {
		if !strings.Contains(ptr.Reason, want) {
			t.Errorf("reason = %q; want it to contain %q", ptr.Reason, want)
		}
	}
	return ptr.Reason
}

// resumeAfterMerge readies the fixture for a resumed Merge-In whose merge is already up to date, so the merge base is now the merged tip.
func (fx *mergeInFixture) resumeAfterMerge() {
	fx.mergeBase = "p1"
	fx.resolver.result.AlreadyUpToDate = true
}

// TestMergeIn_Fixer pins Merge-In's alignment: when the merge fixer spawns, how its attempts are kept across resumes, and where its halts and caps lead.
func TestMergeIn_Fixer(t *testing.T) {
	t.Parallel()

	failing := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	passing := verifytree.Result{Status: verifytree.StatusPassed}
	// brought marks the parent's tip as adding commits over the task's fork point.
	brought := func(fx *mergeInFixture) { fx.holds["b0..p1"] = false }
	spentEntry := FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2", AlignFrom: "b0"}

	t.Run("no parent commit and a passing gate spawn nothing and advance the aligned tip", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)

		outcome, ptr := fx.call(t)

		if outcome != shedengine.Done || len(fx.seats.tables) != 0 {
			t.Fatalf("Call() = %q, %+v with %d fixer session(s); want Done and none", outcome, ptr, len(fx.seats.tables))
		}
		if got := fx.readRecord(t).AlignedTip; got != "p1" {
			t.Errorf("aligned_tip = %q; want the merged tip p1", got)
		}
		if _, found := fx.attemptsOnDisk(t); found {
			t.Error("the attempt record outlived a Done")
		}
	})

	t.Run("parent commits and a passing gate spawn the merge-in fixer from the fork point, the first Merge-In included", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}

		outcome, ptr := fx.call(t)

		if outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %+v; want Done", outcome, ptr)
		}
		if len(fx.seats.tables) != 1 {
			t.Fatalf("fixer sessions = %d; want 1", len(fx.seats.tables))
		}
		values := fx.seats.tables[0].Values
		if values["mode"] != string(FixerModeParentAlign) || values["aligned_tip"] != "b0" || values["merged_tip"] != "p1" {
			t.Errorf("fixer table values = %v; want merge-in mode over the fork point b0 and the merged tip p1", values)
		}
		if got := fx.readRecord(t).AlignedTip; got != "p1" {
			t.Errorf("aligned_tip = %q; want the merged tip p1", got)
		}
		if len(fx.lineChangeCalls) != 0 {
			t.Errorf("LineChange calls = %v; want none with a Code-Review budget above 0", fx.lineChangeCalls)
		}
	})

	t.Run("a failing gate re-prompts until it passes or the attempts are spent, then halts naming the log", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		writeVerifyLog(t, fx.gate, "FAIL")
		fx.scriptGate(failing, failing, passing)
		fix := fx.fixerTurn(t, fixedReport, "f1")
		fx.seats.turns = []func(){fix, fix}

		if outcome, ptr := fx.call(t); outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %+v; want Done once the gate passes", outcome, ptr)
		}
		if got := fx.seats.tables[0].Gate[0].Attempts; got != 2 {
			t.Errorf("gate attempts = %d; want the 2 turns the entry has", got)
		}
		// One call for the row's own gate and one per fixer turn, all with the spawning row's key.
		if want := []string{"parent-align", "parent-align", "parent-align"}; !slices.Equal(fx.gate.closureKeys, want) {
			t.Errorf("VerifyCommand keys = %v; want %v", fx.gate.closureKeys, want)
		}

		spent := newMergeInFixture(t)
		writeVerifyLog(t, spent.gate, "FAIL")
		spent.scriptGate(failing)
		fix = spent.fixerTurn(t, fixedReport, "f1")
		spent.seats.turns = []func(){fix, fix}

		outcome, ptr := spent.call(t)

		requireHalt(t, outcome, ptr, "spent every attempt", "verify-")
		rec, found := spent.attemptsOnDisk(t)
		if !found || rec.Spent != 2 || rec.Halt != fixerHaltSpent {
			t.Errorf("attempt record = %+v, %v; want 2 spent turns and the halt noted as %q", rec, found, fixerHaltSpent)
		}
	})

	t.Run("escalate on a run's first Merge-In halts naming the report, and the resumed entry still reads the fork point", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.seats.turns = []func(){fx.fixerTurn(t, escalateReport, "t2")}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, fx.fixerReportPath(), "decision D3")
		if rec, _ := fx.attemptsOnDisk(t); rec.Spent != 0 || rec.AlignFrom != "b0" {
			t.Errorf("attempt record = %+v; want no spent turn and the fork point b0", rec)
		}

		fx.resumeAfterMerge()
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}
		if outcome, ptr := fx.call(t); outcome != shedengine.Done {
			t.Fatalf("resumed Call() = %q, %+v; want Done", outcome, ptr)
		}
		if len(fx.seats.tables) != 2 || fx.seats.tables[1].Values["aligned_tip"] != "b0" {
			t.Errorf("fixer sessions = %d; want the resumed entry's second one over the fork point b0", len(fx.seats.tables))
		}
	})

	t.Run("a first Merge-In that stops with an error after the merge still spawns the fixer when resumed", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.gate.fake.verifyErr = errors.New("shell gone")

		if _, _, err := fx.m.Call(t.Context()); err == nil || !strings.Contains(err.Error(), "shell gone") {
			t.Fatalf("Call() error = %v; want the gate's infrastructure error", err)
		}

		fx.gate.fake.verifyErr = nil
		fx.resumeAfterMerge()
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}
		if outcome, ptr := fx.call(t); outcome != shedengine.Done {
			t.Fatalf("resumed Call() = %q, %+v; want Done", outcome, ptr)
		}
		if len(fx.seats.tables) != 1 || fx.seats.tables[0].Values["aligned_tip"] != "b0" {
			t.Errorf("fixer sessions = %d; want one over the fork point b0", len(fx.seats.tables))
		}
	})

	t.Run("a failed spawn is an error stop that spends no attempt", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.seats.startErr = fmt.Errorf("seat chair: %w", shuttleengine.ErrNotStarted)

		if _, _, err := fx.m.Call(t.Context()); err == nil {
			t.Fatal("Call() error = nil; want the failed start")
		}

		if rec, _ := fx.attemptsOnDisk(t); rec.Spent != 0 {
			t.Errorf("attempt record = %+v; want no spent turn", rec)
		}
	})

	t.Run("a resumed entry with no attempts left spawns nothing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			seed       FixerAttempts
			gate       verifytree.Result
			reviewOff  bool
			lines      int
			wantOK     bool
			wantReason []string
		}{
			{name: "a spent cap over a passing gate reads the operator's fix as the alignment", seed: FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2", AlignFrom: "b0", Halt: fixerHaltSpent}, gate: passing, wantOK: true},
			{name: "a spent cap over a failing gate halts again naming the log", seed: FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2", AlignFrom: "b0", Halt: fixerHaltSpent}, gate: failing, wantReason: []string{"spent every attempt", "verify-"}},
			{name: "a last turn that ended without a note is Done over a passing gate", seed: spentEntry, gate: passing, reviewOff: true, wantOK: true},
			{name: "a last turn that ended without a note halts over-cap when its commits put the change past the caps", seed: spentEntry, gate: passing, reviewOff: true, lines: 500, wantReason: []string{"over the caps", "t2..f1"}},
			{name: "a last turn that ended without a note halts naming the log over a failing gate", seed: spentEntry, gate: failing, reviewOff: true, wantReason: []string{"spent every attempt", "verify-"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := newMergeInFixture(t)
				brought(fx)
				fx.mergeHead = "f1"
				fx.reviewOff = tt.reviewOff
				fx.lineChange = fabricengine.LineChange{Lines: tt.lines}
				fx.seedAttempts(t, tt.seed)
				writeVerifyLog(t, fx.gate, "FAIL")
				fx.scriptGate(tt.gate)

				outcome, ptr := fx.call(t)

				if len(fx.seats.tables) != 0 {
					t.Errorf("fixer sessions = %d; want none", len(fx.seats.tables))
				}
				if tt.wantOK {
					if outcome != shedengine.Done {
						t.Fatalf("Call() = %q, %+v; want Done", outcome, ptr)
					}
					return
				}
				requireHalt(t, outcome, ptr, tt.wantReason...)
			})
		}
	})

	t.Run("a nil CodeReviewOff applies the caps", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.m.deps.CodeReviewOff = nil
		fx.mergeHead = "f1"
		fx.lineChange = fabricengine.LineChange{Lines: 500}
		fx.seedAttempts(t, spentEntry)

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, "over the caps")
	})

	t.Run("under CodeReviewOff the fixer's change is judged from the baseline after the merge", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			change     fabricengine.LineChange
			wantReason []string
		}{
			{name: "a change past finalize_fix_max_lines halts naming its commits", change: fabricengine.LineChange{Lines: 500}, wantReason: []string{"over the cap of 100", "t2..f1"}},
			{name: "a change that edits a test file halts naming its commits", change: fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}, wantReason: []string{"the test file a/a_test.go", "t2..f1"}},
			{name: "a merge that exceeds the caps followed by a change within them is Done", change: fabricengine.LineChange{Lines: 3, Paths: []string{"a/a.go"}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := newMergeInFixture(t)
				brought(fx)
				fx.reviewOff = true
				fx.lineChange = tt.change
				fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}

				outcome, ptr := fx.call(t)

				if len(tt.wantReason) == 0 {
					if outcome != shedengine.Done {
						t.Fatalf("Call() = %q, %+v; want Done", outcome, ptr)
					}
				} else {
					requireHalt(t, outcome, ptr, tt.wantReason...)
					if rec, _ := fx.attemptsOnDisk(t); rec.Halt != fixerHaltOverCap {
						t.Errorf("attempt record halt = %q; want %q", rec.Halt, fixerHaltOverCap)
					}
				}
				if want := []string{"t2..f1"}; strings.Join(fx.lineChangeCalls, ",") != strings.Join(want, ",") {
					t.Errorf("LineChange calls = %v; want %v, from the HEAD after the merge and never the pre-merge HEAD", fx.lineChangeCalls, want)
				}
			})
		}
	})

	t.Run("with a budget above 0 a fixed change over a passing gate goes to the Code-Perch unjudged", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.lineChange = fabricengine.LineChange{Lines: 500}
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}

		if outcome, ptr := fx.call(t); outcome != shedengine.Done || len(fx.lineChangeCalls) != 0 {
			t.Fatalf("Call() = %q, %+v with LineChange calls %v; want Done and no judgement", outcome, ptr, fx.lineChangeCalls)
		}
	})

	t.Run("a resume after an error stop judges from the baseline the entry already holds", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.reviewOff = true
		fx.lineChangeErr = errors.New("git exploded")
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}

		if _, _, err := fx.m.Call(t.Context()); err == nil || !strings.Contains(err.Error(), "git exploded") {
			t.Fatalf("Call() error = %v; want the line-change read's error", err)
		}

		fx.lineChangeErr = nil
		fx.resumeAfterMerge()
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f2")}
		if outcome, ptr := fx.call(t); outcome != shedengine.Done {
			t.Fatalf("resumed Call() = %q, %+v; want Done", outcome, ptr)
		}
		if want := "t2..f1,t2..f2"; strings.Join(fx.lineChangeCalls, ",") != want {
			t.Errorf("LineChange calls = %v; want %q, both from the baseline of the first turn", fx.lineChangeCalls, want)
		}
	})

	t.Run("a resume after an over-cap halt writes the baseline afresh and reads the operator's review as the alignment", func(t *testing.T) {
		t.Parallel()
		overCap := func(spent int) FixerAttempts {
			return FixerAttempts{Entry: 1, Spent: spent, Baseline: "t2", AlignFrom: "b0", Halt: fixerHaltOverCap}
		}
		tests := []struct {
			name       string
			seed       FixerAttempts
			gate       verifytree.Result
			turns      func(t *testing.T, fx *mergeInFixture) []func()
			wantDone   bool
			wantTables int
			wantReason []string
		}{
			{name: "a passing gate is Done with no spawn", seed: overCap(1), gate: passing, wantDone: true},
			{
				name: "a failing gate runs the fixer on the attempts left", seed: overCap(1), gate: failing, wantTables: 1,
				turns: func(t *testing.T, fx *mergeInFixture) []func() {
					return []func(){fx.fixerTurn(t, escalateReport, "h5")}
				},
				wantReason: []string{"decision D3"},
			},
			{name: "a failing gate with none left halts naming the log", seed: overCap(2), gate: failing, wantReason: []string{"spent every attempt", "verify-"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := newMergeInFixture(t)
				brought(fx)
				fx.reviewOff = true
				fx.mergeHead = "h5"
				fx.lineChange = fabricengine.LineChange{Lines: 500}
				fx.seedAttempts(t, tt.seed)
				writeVerifyLog(t, fx.gate, "FAIL")
				fx.scriptGate(tt.gate)
				if tt.turns != nil {
					fx.seats.turns = tt.turns(t, fx)
				}

				outcome, ptr := fx.call(t)

				if len(fx.seats.tables) != tt.wantTables {
					t.Errorf("fixer sessions = %d; want %d", len(fx.seats.tables), tt.wantTables)
				}
				if len(fx.lineChangeCalls) != 0 {
					t.Errorf("LineChange calls = %v; want none, the operator's review being the alignment", fx.lineChangeCalls)
				}
				if tt.wantDone {
					if outcome != shedengine.Done {
						t.Fatalf("Call() = %q, %+v; want Done", outcome, ptr)
					}
					return
				}
				requireHalt(t, outcome, ptr, tt.wantReason...)
				if rec, _ := fx.attemptsOnDisk(t); rec.Baseline != "h5" || rec.Halt != "" {
					t.Errorf("attempt record = %+v; want the baseline rewritten at h5 and the halt cleared", rec)
				}
				if tt.wantTables == 1 {
					if got := fx.seats.tables[0].Gate[0].Attempts; got != 1 {
						t.Errorf("gate attempts = %d; want the 1 turn the entry has left", got)
					}
				}
			})
		}
	})

	t.Run("an escalated turn of a resumed over-cap entry still counts toward the change judged at the next resume", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		brought(fx)
		fx.reviewOff = true
		fx.mergeHead = "h5"
		fx.seedAttempts(t, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", AlignFrom: "b0", Halt: fixerHaltOverCap})
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.fixerTurn(t, escalateReport, "h6")}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, "decision D3")

		fx.resumeAfterMerge()
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "h7")}
		if outcome, ptr := fx.call(t); outcome != shedengine.Done {
			t.Fatalf("resumed Call() = %q, %+v; want Done", outcome, ptr)
		}
		if want := "h5..h7"; strings.Join(fx.lineChangeCalls, ",") != want {
			t.Errorf("LineChange calls = %v; want %q, from the baseline written at the first resume", fx.lineChangeCalls, want)
		}
	})

	t.Run("a re-entry from another row starts at zero attempts while a resume keeps the spent ones", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			entry        int
			wantAttempts int
		}{
			{"re-entry", 2, 2},
			{"resume", 1, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := newMergeInFixture(t)
				fx.entry = tt.entry
				fx.seedAttempts(t, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", AlignFrom: "b0"})
				fx.scriptGate(failing)
				fx.seats.turns = []func(){fx.fixerTurn(t, escalateReport, "t2")}

				outcome, ptr := fx.call(t)

				requireHalt(t, outcome, ptr)
				if len(fx.seats.tables) != 1 || fx.seats.tables[0].Gate[0].Attempts != tt.wantAttempts {
					t.Errorf("fixer sessions = %d; want one holding %d attempt(s)", len(fx.seats.tables), tt.wantAttempts)
				}
			})
		}
	})

	t.Run("an undecodable attempt record halts naming the file and how to remove it", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		path := FixerAttemptsPath(fx.m.deps.ScratchDir, parentAlignName)
		if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
			t.Fatal(err)
		}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, path, "way forward: remove "+path, "`lym loom resume`")
		if strings.Contains(strings.Join(fx.events, ","), "merge main") {
			t.Errorf("a merge ran over an undecodable attempt record: %v", fx.events)
		}
	})

	retiredAttemptsPath := func(fx *mergeInFixture) string {
		return FixerAttemptsPath(fx.m.deps.ScratchDir, retiredMergeInName)
	}
	seedRetiredAttempts := func(t *testing.T, fx *mergeInFixture, rec FixerAttempts) {
		t.Helper()
		if err := WriteFixerAttempts(retiredAttemptsPath(fx), rec); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("an attempt record under the retired row name is read with its spent turns and fork point", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.holds["x0..p1"] = false
		fx.seats.turns = []func(){fx.fixerTurn(t, fixedReport, "f1")}
		seedRetiredAttempts(t, fx, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", AlignFrom: "x0"})

		outcome, ptr := fx.call(t)

		if outcome != shedengine.Done || len(fx.seats.tables) != 1 {
			t.Fatalf("Call() = %q, %+v with %d fixer session(s); want Done and one", outcome, ptr, len(fx.seats.tables))
		}
		if got := fx.seats.tables[0].Gate[0].Attempts; got != 1 {
			t.Errorf("gate attempts = %d; want the 1 turn the retired record leaves the entry", got)
		}
		if got := fx.seats.tables[0].Values["aligned_tip"]; got != "x0" {
			t.Errorf("fixer aligned_tip = %q; want the retired record's fork point x0", got)
		}
	})

	t.Run("the spent halt a retired-name record holds still halts without a fixer", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		writeVerifyLog(t, fx.gate, "FAIL")
		fx.scriptGate(failing)
		seedRetiredAttempts(t, fx, FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2", AlignFrom: "b0", Halt: fixerHaltSpent})

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, "spent every attempt")
		if len(fx.seats.tables) != 0 {
			t.Errorf("fixer sessions = %d; want none over the spent attempts the retired record carries", len(fx.seats.tables))
		}
	})

	t.Run("an undecodable attempt record under the retired row name halts naming the old file", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		path := retiredAttemptsPath(fx)
		if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
			t.Fatal(err)
		}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, path, "way forward: remove "+path, "`lym loom resume`")
	})

	t.Run("a Done removes the retired row name's attempt record on the success path and after the align path", func(t *testing.T) {
		t.Parallel()
		success := newMergeInFixture(t)
		seedRetiredAttempts(t, success, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", AlignFrom: "b0"})
		if outcome, ptr := success.call(t); outcome != shedengine.Done {
			t.Fatalf("success path: Call() = %q, %+v; want Done", outcome, ptr)
		}
		if _, err := os.Stat(retiredAttemptsPath(success)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("success path: stat of the retired attempt record = %v; want it removed", err)
		}

		aligned := newMergeInFixture(t)
		writeVerifyLog(t, aligned.gate, "FAIL")
		aligned.scriptGate(failing, passing)
		aligned.seats.turns = []func(){aligned.fixerTurn(t, fixedReport, "f1")}
		seedRetiredAttempts(t, aligned, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", AlignFrom: "b0"})
		if outcome, ptr := aligned.call(t); outcome != shedengine.Done {
			t.Fatalf("align path: Call() = %q, %+v; want Done", outcome, ptr)
		}
		if _, err := os.Stat(retiredAttemptsPath(aligned)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("align path: stat of the retired attempt record = %v; want it removed", err)
		}
		if _, found := aligned.attemptsOnDisk(t); found {
			t.Error("align path: the attempt record outlived a Done")
		}
	})
}
