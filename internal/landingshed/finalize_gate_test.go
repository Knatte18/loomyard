//go:build tier0

// finalize_gate_test.go drives Finalize's landing gate and its finalize-mode merge fixer over the gate fixture, a fake seat runner, a fake parent merger and fake line-change reads.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// reviewPath is the report the fix review session writes for Finalize.
func (fx *finalizeGateFixture) reviewPath() string {
	return filepath.Join(fx.deps.ScratchDir, finalizeName+"-fix-review.md")
}

// review scripts a fix review turn that writes content as its report.
func (fx *finalizeGateFixture) review(t *testing.T, content string) func() {
	t.Helper()
	return func() {
		if err := os.WriteFile(fx.reviewPath(), []byte(content), 0o644); err != nil {
			t.Errorf("write fix review report: %v", err)
		}
	}
}

const (
	approvedReview = "verdict: approved\n\n## Findings\n"
	rejectedReview = "verdict: rejected\n\n## Findings\n- f1 loosens the assertion in a_test.go\n"
)

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
		const treeScoped = "lym gate test --wait --tier 0 --tree-scoped"
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

	t.Run("two turns each under the line cap but past it together are over the cap, measured from the entry's baseline", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.reviewed = true
		fx.lineChange = fabricengine.LineChange{Lines: 60, Paths: []string{"a/a.go"}}
		fx.scriptGate(failing, failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1"), fx.turn(t, fixedReport, "f2")}
		fx.seats.reviewTurns = []func(){fx.review(t, rejectedReview)}

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

		requireHalt(t, outcome, ptr, path, "way forward: remove "+path, "`lym loom resume`")
		if fx.gate.fake.verifyCalls != 0 || len(fx.merger.calls) != 0 {
			t.Errorf("verify calls = %d, parent-side merges = %d; want none over an undecodable record", fx.gate.fake.verifyCalls, len(fx.merger.calls))
		}
	})

	t.Run("an entry whose last turn ended without a note is resumed with no fixer spawn", func(t *testing.T) {
		tests := []struct {
			name     string
			gate     verifytree.Result
			change   fabricengine.LineChange
			reviewed bool
			// review is the fix review's report, written when the row reaches one.
			review     string
			wantLand   bool
			wantReason []string
		}{
			{name: "a failing gate reroutes as a spent cap does", gate: failing, reviewed: true, wantReason: []string{"spent every attempt", "verify-", "fixer commits: t2..f1"}},
			{name: "a failing gate halts where no review follows the reroute", gate: failing, wantReason: []string{"spent every attempt", "verify-", "fixer commits: t2..f1"}},
			{name: "a passing gate over a change within the caps proceeds", gate: passing, change: small, reviewed: true, wantLand: true},
			{name: "a passing gate over a change past the caps reroutes on the fix review's rejection", gate: passing, change: fabricengine.LineChange{Lines: 500}, reviewed: true, review: rejectedReview, wantReason: []string{"over the cap of 50", "the fix review rejected it", "fixer commits: t2..f1"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				fx := newFinalizeGateFixture(t)
				fx.head = "f1"
				fx.reviewed = tt.reviewed
				fx.lineChange = tt.change
				fx.seed(t, FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2"})
				fx.scriptGate(tt.gate)
				if tt.review != "" {
					fx.seats.reviewTurns = []func(){fx.review(t, tt.review)}
				}

				outcome, ptr := fx.call(t)

				wantSessions := 0
				if tt.review != "" {
					wantSessions = 1
				}
				if len(fx.seats.tables) != wantSessions {
					t.Fatalf("sessions = %d; want %d", len(fx.seats.tables), wantSessions)
				}
				for _, table := range fx.seats.tables {
					if table.RolePrefix != fixReviewRole {
						t.Errorf("session role = %q; want no fixer spawned", table.RolePrefix)
					}
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

	t.Run("a fix over the caps goes to the fix review over the fixer's commits, and a rejection reroutes with the commits kept, whatever makes it over", func(t *testing.T) {
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
				fx.seats.reviewTurns = []func(){fx.review(t, rejectedReview)}

				outcome, ptr := fx.call(t)

				fx.requireReroute(t, outcome, ptr, tt.wantReason, "the fix review rejected it: f1 loosens the assertion", fx.reviewPath(), "fixer commits: t2..f1")
				if len(fx.seats.tables) != 2 {
					t.Fatalf("sessions = %d; want the fixer's and the fix review's", len(fx.seats.tables))
				}
				review := fx.seats.tables[1]
				if review.RolePrefix != fixReviewRole || review.Values["baseline"] != "t2" || review.Values["head"] != "f1" || !strings.Contains(review.Values["cap_reason"], tt.wantReason) {
					t.Errorf("fix review table = %q, %v; want role %q over t2..f1 naming %q", review.RolePrefix, review.Values, fixReviewRole, tt.wantReason)
				}
				if got := review.Values["fixer_report_path"]; got != fx.reportPath() {
					t.Errorf("fix review fixer_report_path = %q; want %q", got, fx.reportPath())
				}
				if want := time.Duration(fx.deps.Config.FixerTimeoutMin) * time.Minute; review.Timeout != want {
					t.Errorf("fix review timeout = %v; want fixer_timeout_min, %v", review.Timeout, want)
				}
				if got, want := review.Seats[0].ModelChoice, fx.seats.tables[0].Seats[0].ModelChoice; got != want {
					t.Errorf("fix review model choice = %+v; want the fixer's %+v", got, want)
				}
			})
		}
	})

	t.Run("the fix review's end decides where an over-cap change goes", func(t *testing.T) {
		tests := []struct {
			name string
			// review scripts the fix review's one turn, given the fixture.
			review     func(t *testing.T, fx *finalizeGateFixture) func()
			wantLand   bool
			wantHalt   bool
			wantReason []string
			// rePrompted is the report the re-prompt's turn writes, when the first turn's report does not parse.
			rePrompted string
			// reviewTimeout makes the review session end on its timeout.
			reviewTimeout bool
			// absentReason is a fragment the halt reason must not hold.
			absentReason string
		}{
			{
				name:     "an approval lands",
				review:   func(t *testing.T, fx *finalizeGateFixture) func() { return fx.review(t, approvedReview) },
				wantLand: true,
			},
			{
				name:          "a session past its timeout reroutes naming the timeout key",
				review:        func(*testing.T, *finalizeGateFixture) func() { return func() {} },
				reviewTimeout: true,
				wantReason:    []string{"the fix review gave no verdict", "fixer_timeout_min", "session log: " + fakeFixerRunDir, "fixer commits: t2..f1"},
			},
			{
				name:       "an unparseable report is re-prompted once, and the report that follows decides",
				review:     func(t *testing.T, fx *finalizeGateFixture) func() { return fx.review(t, "verdict: maybe\n") },
				rePrompted: approvedReview,
				wantLand:   true,
			},
			{
				name:       "a session that leaves no report reroutes naming the missing report",
				review:     func(*testing.T, *finalizeGateFixture) func() { return func() {} },
				wantReason: []string{"the fix review gave no verdict", "session log: " + fakeFixerRunDir, "fixer commits: t2..f1"},
			},
			{
				name:       "a report with no known verdict reroutes naming the line",
				review:     func(t *testing.T, fx *finalizeGateFixture) func() { return fx.review(t, "verdict: maybe\n") },
				wantReason: []string{"the fix review gave no verdict", "is no known verdict"},
			},
			{
				name: "a session that dirties the tree halts naming the way forward",
				review: func(t *testing.T, fx *finalizeGateFixture) func() {
					return func() {
						fx.review(t, approvedReview)()
						fx.gate.fake.dirtyAt(fx.gate.fake.dirtyCalls, "a/a.go")
					}
				},
				wantHalt:     true,
				wantReason:   []string{"the fix review session of Finalize changed the task branch", "a/a.go", "way forward: restore the task branch to the fixer's commits t2..f1", "`lym loom resume`"},
				absentReason: "commit or remove them",
			},
			{
				name: "a session that commits halts naming the moved head",
				review: func(t *testing.T, fx *finalizeGateFixture) func() {
					return func() {
						fx.review(t, approvedReview)()
						fx.head = "r1"
					}
				},
				wantHalt:   true,
				wantReason: []string{"the task head moved from f1 to r1", "way forward: restore the task branch"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				fx := newFinalizeGateFixture(t)
				fx.reviewed = true
				fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
				fx.scriptGate(failing, passing)
				fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}
				fx.seats.reviewTurns = []func(){tt.review(t, fx)}
				if tt.rePrompted != "" {
					fx.seats.reviewTurns = append(fx.seats.reviewTurns, fx.review(t, tt.rePrompted))
				}
				fx.seats.reviewTimeout = tt.reviewTimeout

				outcome, ptr := fx.call(t)

				switch {
				case tt.wantLand:
					fx.requireLanded(t, outcome, ptr)
				case tt.wantHalt:
					requireHalt(t, outcome, ptr, tt.wantReason...)
					if tt.absentReason != "" && strings.Contains(ptr.Reason, tt.absentReason) {
						t.Errorf("reason = %q; want it without %q", ptr.Reason, tt.absentReason)
					}
					if len(fx.merger.calls) != 0 {
						t.Errorf("parent-side merges = %d; want none after a halt", len(fx.merger.calls))
					}
				default:
					fx.requireReroute(t, outcome, ptr, tt.wantReason...)
				}
			})
		}
	})

	t.Run("an approval is kept in the attempt record as the HEAD it approved", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.reviewed = true
		fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
		fx.merger.pushErr = errors.New("push refused")
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}
		fx.seats.reviewTurns = []func(){fx.review(t, approvedReview)}

		// The refused push stops the landing after the merge, which keeps the record the landing would otherwise remove.
		if _, _, err := fx.fz.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if len(fx.merger.calls) != 1 {
			t.Fatalf("parent-side merges = %d; want the landing to reach its merge after the approval", len(fx.merger.calls))
		}
		if rec, found := fx.attempts(t); !found || rec.Reviewed != "f1" {
			t.Errorf("attempt record = %+v, %v; want Reviewed f1, the HEAD the review approved", rec, found)
		}
	})

	t.Run("a pass at a HEAD the fix review approved lands with no second review, and at any other HEAD is reviewed again", func(t *testing.T) {
		tests := []struct {
			name     string
			reviewed string
			// review is the fix review's report, written when a review runs.
			review       string
			wantSessions int
			wantLand     bool
		}{
			{name: "the approved HEAD", reviewed: "f1", wantLand: true},
			{name: "a HEAD that moved since the approval", reviewed: "e9", review: rejectedReview, wantSessions: 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				fx := newFinalizeGateFixture(t)
				fx.reviewed = true
				fx.head = "f1"
				fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
				fx.seed(t, FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", Reviewed: tt.reviewed})
				fx.scriptGate(passing)
				if tt.review != "" {
					fx.seats.reviewTurns = []func(){fx.review(t, tt.review)}
				}

				outcome, ptr := fx.call(t)

				if tt.wantLand {
					fx.requireLanded(t, outcome, ptr)
				} else {
					fx.requireReroute(t, outcome, ptr, "the fix review rejected it")
				}
				if len(fx.seats.tables) != tt.wantSessions {
					t.Errorf("sessions = %d; want %d", len(fx.seats.tables), tt.wantSessions)
				}
				for _, table := range fx.seats.tables {
					if table.RolePrefix != fixReviewRole {
						t.Errorf("session role = %q; want only fix review sessions", table.RolePrefix)
					}
				}
			})
		}
	})

	t.Run("a fix review session that fails to start returns a transient error and lands nothing", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.reviewed = true
		fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}
		fx.seats.reviewStartErr = fmt.Errorf("seat chair: %w", shuttleengine.ErrNotStarted)

		_, _, err := fx.fz.Call(context.Background())

		if err == nil || shedengine.TransientOf(err) != shedengine.TransientAgentStart {
			t.Fatalf("Call() error = %v; want one the transient mark classifies as %q", err, shedengine.TransientAgentStart)
		}
		if len(fx.merger.calls) != 0 {
			t.Errorf("parent-side merges = %d; want none after a failed start", len(fx.merger.calls))
		}
	})

	t.Run("with no review after the reroute an over-cap change halts naming its commits and noting over-cap, with no fix review", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a_test.go"}}
		fx.scriptGate(failing, passing)
		fx.seats.turns = []func(){fx.turn(t, fixedReport, "f1")}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, "the test file a/a_test.go", "fixer commits: t2..f1", "way forward: review and fix on the task branch", "`lym loom resume`")
		if rec, _ := fx.attempts(t); rec.Spent != 1 || rec.Halt != fixerHaltOverCap {
			t.Errorf("attempt record = %+v; want 1 of the 2 attempts spent and the halt noted as %q", rec, fixerHaltOverCap)
		}
		if len(fx.merger.calls) != 0 {
			t.Errorf("parent-side merges = %d; want none after a halt", len(fx.merger.calls))
		}
		if len(fx.seats.tables) != 1 {
			t.Errorf("sessions = %d; want the fixer's alone", len(fx.seats.tables))
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

		requireHalt(t, outcome, ptr, fx.reportPath(), "decision D3")
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
		if strings.Contains(ptr.Reason, "`lym loom decision add`") {
			t.Errorf("reason = %q; want no decision verb on a run that does not judge an escalate first", ptr.Reason)
		}
		if rec, _ := fx.attempts(t); rec.Spent != 0 || rec.Halt != "" {
			t.Errorf("attempt record = %+v; want no spent turn and no halt noted", rec)
		}
	})

	t.Run("an escalate report on a run that judges it first halts with the escalate prefix, no hand-back", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.fz.deps.EscalateJudgedFirst = true
		fx.scriptGate(failing)
		fx.seats.turns = []func(){fx.turn(t, escalateReport, "f1")}

		outcome, ptr := fx.call(t)

		requireEscalateHalt(t, outcome, ptr, finalizeName, fx.reportPath(), "decision D3", "`lym loom decision add`")
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
