//go:build tier0

// finalize_gate_test.go drives Finalize's landing gate and its finalize-mode merge fixer over the gate fixture, a fake seat runner, a fake parent merger and fake line-change reads.

package landingshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// finalizeGateFixture is a Finalize over the gate fixture, a fake seat runner and fake line-change reads.
type finalizeGateFixture struct {
	fz     *Finalize
	deps   Deps
	gate   *gateFixture
	seats  *fakeFixerSeats
	merger *recordingParentMerger
	// head is the task head, which each fixer turn that commits moves on.
	head string
	// entry is the row entry RowEntry answers.
	entry int
	// reviewed is what RerouteReviewed answers.
	reviewed bool
	// lineChange is what LineChange answers, and lineChangeCalls records each "from..to".
	lineChange      fabricengine.LineChange
	lineChangeCalls []string
}

func newFinalizeGateFixture(t *testing.T) *finalizeGateFixture {
	t.Helper()
	deps := newTestDeps(t)
	stencilkit.SeedInto(t, deps.StencilsDir)
	fx := &finalizeGateFixture{
		gate:   newGateFixture(t, "finalize", "go test ./...", nil),
		seats:  &fakeFixerSeats{},
		merger: &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}},
		head:   "t2",
		entry:  1,
	}
	deps.Slug = "me"
	deps.Config.Fixer = "claude:opus"
	deps.Config.FixerTimeoutMin = 5
	deps.Config.FinalizeFixAttempts = 2
	deps.Config.FinalizeFixMaxLines = 50
	deps.Registry = modelspec.Registry{}
	deps.FixerSeats = fx.seats
	deps.DecisionRecordPath = filepath.Join(deps.ScratchDir, "decision-record.md")
	deps.TaskRemoteTip = func() (string, error) { return "remote-1", nil }
	deps.IsTestFile = func(path string) bool { return strings.HasSuffix(path, "_test.go") }
	deps.RowEntry = func(string) (int, error) { return fx.entry, nil }
	deps.RerouteReviewed = func() (bool, error) { return fx.reviewed, nil }
	deps.TaskHead = func() (string, error) { return fx.head, nil }
	deps.LineChange = func(from, to string) (fabricengine.LineChange, error) {
		fx.lineChangeCalls = append(fx.lineChangeCalls, from+".."+to)
		return fx.lineChange, nil
	}
	if err := os.MkdirAll(fx.gate.paths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeVerifyLog(t, fx.gate, "FAIL")
	fx.gate.gate.recorder = &failureRecorder{head: deps.TaskHead}
	fx.deps = deps
	fx.fz = &Finalize{
		deps:         deps,
		resolver:     &recordingResolver{result: resolved(false)},
		gate:         fx.gate.gate,
		parentOpener: func() (parentMerger, error) { return fx.merger, nil },
	}
	return fx
}

func (fx *finalizeGateFixture) call(t *testing.T) (shedengine.Outcome, shedengine.OutputPointer) {
	t.Helper()
	outcome, ptr, err := fx.fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	return outcome, ptr
}

// reportPath is the report the fixer session writes for Finalize.
func (fx *finalizeGateFixture) reportPath() string {
	return filepath.Join(fx.deps.ScratchDir, finalizeName+"-fixer-report.md")
}

// turn scripts a fixer turn that writes content as its report and moves the task head to head, as a turn that commits does.
func (fx *finalizeGateFixture) turn(t *testing.T, content, head string) func() {
	t.Helper()
	return func() {
		if err := os.WriteFile(fx.reportPath(), []byte(content), 0o644); err != nil {
			t.Errorf("write fixer report: %v", err)
		}
		fx.head = head
	}
}

// scriptGate makes the n-th verify from now answer results[n], and every later one the last result.
func (fx *finalizeGateFixture) scriptGate(results ...verifytree.Result) {
	already := fx.gate.fake.verifyCalls
	fx.gate.fake.onVerify = func() {
		fx.gate.fake.result = results[min(fx.gate.fake.verifyCalls-already-1, len(results)-1)]
	}
}

// seed writes rec as the entry's attempt record.
func (fx *finalizeGateFixture) seed(t *testing.T, rec FixerAttempts) {
	t.Helper()
	if err := WriteFixerAttempts(FixerAttemptsPath(fx.deps.ScratchDir, finalizeName), rec); err != nil {
		t.Fatal(err)
	}
}

// attempts returns the attempt record for the fixture's entry and whether the file exists.
func (fx *finalizeGateFixture) attempts(t *testing.T) (FixerAttempts, bool) {
	t.Helper()
	path := FixerAttemptsPath(fx.deps.ScratchDir, finalizeName)
	if _, err := os.Stat(path); err != nil {
		return FixerAttempts{}, false
	}
	rec, err := ReadFixerAttempts(path, fx.entry)
	if err != nil {
		t.Fatalf("ReadFixerAttempts() error = %v", err)
	}
	return rec, true
}

// requireReroute asserts the call rerouted with no landing and returns the reason after checking it holds every fragment.
func (fx *finalizeGateFixture) requireReroute(t *testing.T, outcome shedengine.Outcome, ptr shedengine.OutputPointer, fragments ...string) {
	t.Helper()
	if outcome != shedengine.Stuck || !ptr.Reroute || ptr.Halt {
		t.Fatalf("Call() = %q, %+v; want a reroute", outcome, ptr)
	}
	for _, want := range fragments {
		if !strings.Contains(ptr.Reason, want) {
			t.Errorf("reason = %q; want it to contain %q", ptr.Reason, want)
		}
	}
	if len(fx.merger.calls) != 0 {
		t.Errorf("parent-side merges = %d; want none after a reroute", len(fx.merger.calls))
	}
}

// requireLanded asserts the call ended Done with one parent-side merge, and that the entry's attempt record and the landing failure record are gone.
func (fx *finalizeGateFixture) requireLanded(t *testing.T, outcome shedengine.Outcome, ptr shedengine.OutputPointer) {
	t.Helper()
	if outcome != shedengine.Done || len(fx.merger.calls) != 1 {
		t.Fatalf("Call() = %q, %+v with %d parent-side merge(s); want Done after one", outcome, ptr, len(fx.merger.calls))
	}
	if _, found := fx.attempts(t); found {
		t.Error("the attempt record outlived a Done")
	}
	if _, found, _ := verifytree.ReadLandingFailure(fx.gate.paths); found {
		t.Error("the landing failure record outlived a passing gate")
	}
}

// TestFinalize_GateAndFixer pins Finalize's landing gate and its finalize-mode fixer:
// when the fixer spawns, how its attempts and its Baseline are kept across resumes, and where a spent cap, an over-cap change and an escalate report lead.
func TestFinalize_GateAndFixer(t *testing.T) {
	failing := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	passing := verifytree.Result{Status: verifytree.StatusPassed}
	small := fabricengine.LineChange{Lines: 3, Paths: []string{"a/a.go"}}

	t.Run("a darn Finalize gates through the command its verify source answers", func(t *testing.T) {
		const treeScoped = "lyx gate test --wait --tier 0 --tree-scoped"
		fx := newFinalizeGateFixture(t)
		fx.fz.gate.command = func(string) (string, error) { return treeScoped, nil }

		outcome, ptr := fx.call(t)

		fx.requireLanded(t, outcome, ptr)
		if got := strings.Join(fx.gate.fake.commands, ","); got != treeScoped {
			t.Errorf("verify commands = %q; want only %q", got, treeScoped)
		}
	})

	t.Run("a failing gate spawns the finalize fixer, and a fixed change within the caps proceeds to the landing", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.lineChange = small
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}

		outcome, ptr := fx.call(t)

		fx.requireLanded(t, outcome, ptr)
		if len(fx.seats.tables) != 1 {
			t.Fatalf("fixer sessions = %d; want 1", len(fx.seats.tables))
		}
		values := fx.seats.tables[0].Values
		if values["mode"] != string(FixerModeFinalize) || values["merged_tip"] != testParentTip {
			t.Errorf("fixer table values = %v; want finalize mode over the merged tip %s", values, testParentTip)
		}
		if got := strings.Join(fx.lineChangeCalls, ","); got != "t2..f1" {
			t.Errorf("LineChange calls = %q; want t2..f1, from the HEAD before the first turn", got)
		}
	})

	t.Run("a passing gate spawns nothing", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)

		outcome, ptr := fx.call(t)

		fx.requireLanded(t, outcome, ptr)
		if len(fx.seats.tables) != 0 || len(fx.lineChangeCalls) != 0 {
			t.Errorf("fixer sessions = %d, LineChange calls = %v; want none", len(fx.seats.tables), fx.lineChangeCalls)
		}
	})

	t.Run("two turns each under the line cap but past it together reroute as over the cap, measured from the entry's baseline", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.reviewed = true
		fx.lineChange = fabricengine.LineChange{Lines: 60, Paths: []string{"a/a.go"}}
		fx.scriptGate(failing, failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1"), fx.turn(t, fixedReport, "f2")}

		outcome, ptr := fx.call(t)

		fx.requireReroute(t, outcome, ptr, "over the cap of 50", "fixer commits: t2..f2")
		if got := strings.Join(fx.lineChangeCalls, ","); got != "t2..f2" {
			t.Errorf("LineChange calls = %q; want one over t2..f2, the entry's baseline to HEAD", got)
		}
		if rec, found := fx.attempts(t); !found || rec.Spent != 2 || rec.Baseline != "t2" || rec.Halt != "" {
			t.Errorf("attempt record = %+v, %v; want 2 spent turns from baseline t2, left as a reroute leaves it", rec, found)
		}
	})

	t.Run("an attempt record that does not decode halts naming the file and how to remove it", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		path := FixerAttemptsPath(fx.deps.ScratchDir, finalizeName)
		if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
			t.Fatal(err)
		}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, path, "way forward: remove "+path, "`lyx loom resume`")
		if fx.gate.fake.verifyCalls != 0 || len(fx.merger.calls) != 0 {
			t.Errorf("verify calls = %d, parent-side merges = %d; want none over an undecodable record", fx.gate.fake.verifyCalls, len(fx.merger.calls))
		}
	})

	t.Run("an entry whose last turn ended without a note is resumed with no spawn", func(t *testing.T) {
		tests := []struct {
			name       string
			gate       verifytree.Result
			change     fabricengine.LineChange
			reviewed   bool
			wantLand   bool
			wantReason []string
		}{
			{name: "a failing gate reroutes as a spent cap does", gate: failing, reviewed: true, wantReason: []string{"spent every attempt", "verify-", "fixer commits: t2..f1"}},
			{name: "a failing gate halts where no review follows the reroute", gate: failing, wantReason: []string{"spent every attempt", "verify-", "fixer commits: t2..f1"}},
			{name: "a passing gate over a change within the caps proceeds", gate: passing, change: small, reviewed: true, wantLand: true},
			{name: "a passing gate over a change past the caps reroutes", gate: passing, change: fabricengine.LineChange{Lines: 500}, reviewed: true, wantReason: []string{"over the cap of 50", "fixer commits: t2..f1"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				fx := newFinalizeGateFixture(t)
				fx.head = "f1"
				fx.reviewed = tt.reviewed
				fx.lineChange = tt.change
				fx.seed(t, FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2"})
				fx.scriptGate(tt.gate)

				outcome, ptr := fx.call(t)

				if len(fx.seats.tables) != 0 {
					t.Errorf("fixer sessions = %d; want none", len(fx.seats.tables))
				}
				switch {
				case tt.wantLand:
					fx.requireLanded(t, outcome, ptr)
				case tt.reviewed:
					fx.requireReroute(t, outcome, ptr, tt.wantReason...)
				default:
					requireHalt(t, outcome, ptr, tt.wantReason...)
					if rec, _ := fx.attempts(t); rec.Halt != fixerHaltSpent {
						t.Errorf("attempt record halt = %q; want %q", rec.Halt, fixerHaltSpent)
					}
				}
			})
		}
	})

	t.Run("a fix over the caps reroutes with its commits kept, whatever makes it over", func(t *testing.T) {
		tests := []struct {
			name       string
			change     fabricengine.LineChange
			isTestFile func(path string) bool
			wantReason string
		}{
			{name: "past finalize_fix_max_lines", change: fabricengine.LineChange{Lines: 500, Paths: []string{"a/a.go"}}, wantReason: "over the cap of 50"},
			{name: "a three-line change editing a test file under a Go matcher", change: fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}, wantReason: "the test file a/a_test.go"},
			{
				name: "a three-line change editing a C# test file under the darn union", change: fabricengine.LineChange{Lines: 3, Paths: []string{"a/FooTests.cs"}},
				isTestFile: func(path string) bool {
					return strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "Tests.cs")
				},
				wantReason: "the test file a/FooTests.cs",
			},
			{name: "a small fix to any file under a none plan", change: small, isTestFile: func(string) bool { return true }, wantReason: "the test file a/a.go"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				fx := newFinalizeGateFixture(t)
				fx.reviewed = true
				fx.lineChange = tt.change
				if tt.isTestFile != nil {
					fx.fz.deps.IsTestFile = tt.isTestFile
				}
				fx.scriptGate(failing, passing)
				fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}

				outcome, ptr := fx.call(t)

				fx.requireReroute(t, outcome, ptr, tt.wantReason, "fixer commits: t2..f1")
			})
		}
	})

	t.Run("with no review after the reroute an over-cap change halts naming its commits and noting over-cap", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, "the test file a/a_test.go", "fixer commits: t2..f1", "way forward: review and fix on the task branch", "`lyx loom resume`")
		if rec, _ := fx.attempts(t); rec.Spent != 1 || rec.Halt != fixerHaltOverCap {
			t.Errorf("attempt record = %+v; want 1 of the 2 attempts spent and the halt noted as %q", rec, fixerHaltOverCap)
		}
		if len(fx.merger.calls) != 0 {
			t.Errorf("parent-side merges = %d; want none after a halt", len(fx.merger.calls))
		}
	})

	t.Run("a resume after an over-cap halt reads the operator's review as acceptance of the commits", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
		fx.scriptGate(failing, failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1"), fx.turn(t, fixedReport, "f2")}
		outcome, ptr := fx.call(t)
		requireHalt(t, outcome, ptr, "the test file a/a_test.go")
		fx.lineChangeCalls = nil

		fx.head = "h5"
		fx.scriptGate(passing)
		outcome, ptr = fx.call(t)

		fx.requireLanded(t, outcome, ptr)
		if len(fx.lineChangeCalls) != 0 || len(fx.seats.tables) != 1 {
			t.Errorf("LineChange calls = %v, fixer sessions = %d; want none and the first run's one", fx.lineChangeCalls, len(fx.seats.tables))
		}
	})

	t.Run("a resume over a failing gate runs the fixer on the attempt left from a baseline written afresh, and keeps that baseline across an escalate", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}
		outcome, ptr := fx.call(t)
		requireHalt(t, outcome, ptr, "the test file a/a_test.go")
		if rec, _ := fx.attempts(t); rec.Spent != 1 || rec.Halt != fixerHaltOverCap {
			t.Fatalf("attempt record = %+v; want 1 spent turn and over-cap noted", rec)
		}

		fx.head = "h5"
		fx.scriptGate(failing)
		fx.seats.turns = []func(){fx.turn(t, escalateReport, "h6")}
		outcome, ptr = fx.call(t)

		requireHalt(t, outcome, ptr, fx.reportPath(), "decision D3", "`lyx loom decision add`")
		if got := fx.seats.tables[1].Gate[0].Attempts; got != 1 {
			t.Errorf("gate attempts = %d; want the 1 turn the entry has left", got)
		}
		if rec, _ := fx.attempts(t); rec.Baseline != "h5" || rec.Halt != "" || rec.Spent != 1 {
			t.Errorf("attempt record = %+v; want the baseline rewritten at h5, the note cleared and the escalated turn taken back", rec)
		}

		fx.lineChange = small
		fx.lineChangeCalls = nil
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "h7")}
		outcome, ptr = fx.call(t)

		fx.requireLanded(t, outcome, ptr)
		if got := strings.Join(fx.lineChangeCalls, ","); got != "h5..h7" {
			t.Errorf("LineChange calls = %q; want h5..h7, from the baseline the first resume wrote and never from the HEAD this resume started at", got)
		}
	})

	t.Run("a two-turn fix halted over the caps and resumed over a failing gate spawns nothing and halts naming the commits", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.lineChange = fabricengine.LineChange{Lines: 500}
		fx.scriptGate(failing, failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1"), fx.turn(t, fixedReport, "f2")}
		outcome, ptr := fx.call(t)
		requireHalt(t, outcome, ptr, "over the cap of 50")

		fx.head = "h5"
		fx.scriptGate(failing)
		outcome, ptr = fx.call(t)

		requireHalt(t, outcome, ptr, "spent every attempt", "fixer commits:", "way forward: review and fix on the task branch")
		if len(fx.seats.tables) != 1 {
			t.Errorf("fixer sessions = %d; want only the first run's", len(fx.seats.tables))
		}
	})

	t.Run("an escalate report halts naming the report, keeps the commits and spends no attempt", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.scriptGate(failing)
		fx.seats.turns = []func(){fx.turn(t, escalateReport, "f1")}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, fx.reportPath(), "decision D3")
		if rec, _ := fx.attempts(t); rec.Spent != 0 || rec.Halt != "" {
			t.Errorf("attempt record = %+v; want no spent turn and no halt noted", rec)
		}
	})

	t.Run("a re-entry from another row starts at zero attempts while a resume keeps the spent ones", func(t *testing.T) {
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
				fx := newFinalizeGateFixture(t)
				fx.entry = tt.entry
				fx.seed(t, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2"})
				fx.scriptGate(failing)
				fx.seats.turns = []func(){fx.turn(t, escalateReport, "t2")}

				outcome, ptr := fx.call(t)

				requireHalt(t, outcome, ptr)
				if len(fx.seats.tables) != 1 || fx.seats.tables[0].Gate[0].Attempts != tt.wantAttempts {
					t.Errorf("fixer sessions = %d; want one holding %d attempt(s)", len(fx.seats.tables), tt.wantAttempts)
				}
			})
		}
	})
}
