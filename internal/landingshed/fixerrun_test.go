//go:build tier0

// fixerrun_test.go drives the merge fixer loop over a fake seat runner, the gate fixture's fake verify tree and fake seams.
// fakeFixerSeats is the shared fake the Merge-In and Finalize fixer tests drive too.

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

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// fakeFixerSeats is a fixerSeats whose session runs a scripted callback per turn and then calls the table's gate closure, up to the gate's attempts.
type fakeFixerSeats struct {
	// turns scripts each turn by its index: it may write the report, commit, dirty the tree, move the remote tip, advance the gate's clock or release a lock record.
	// A turn past the script does nothing.
	turns []func()
	// startErr, when set, is returned by Run before any turn, as a seat that never started.
	startErr error
	// sessionTimeout makes the session end on the table's timeout without reaching the gate closure.
	sessionTimeout bool
	// hangAfter, when above zero, makes the turn at that index run its callback and then end the session with hangOutcome, never reaching the gate closure.
	hangAfter   int
	hangOutcome shuttleengine.Outcome

	// tables records every table Run was given.
	tables []seatengine.Table
	// turnsRun counts the turns Run ran.
	turnsRun int
}

const fakeFixerRunDir = "/runs/fixer"

// Run runs the scripted turns until the gate passes, ends the run, or its re-prompts are spent.
func (f *fakeFixerSeats) Run(table seatengine.Table) (seatengine.Result, error) {
	f.tables = append(f.tables, table)
	if f.startErr != nil {
		return seatengine.Result{}, f.startErr
	}
	if f.sessionTimeout {
		return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout, RunDir: fakeFixerRunDir}}, nil
	}
	entry := table.Gate[0]
	for turn := 0; turn <= entry.Attempts; turn++ {
		f.turnsRun++
		if turn < len(f.turns) {
			f.turns[turn]()
		}
		if f.hangAfter > 0 && turn == f.hangAfter {
			return seatengine.Result{Chair: shuttleengine.Result{Outcome: f.hangOutcome, RunDir: fakeFixerRunDir}}, nil
		}
		result, err := entry.Gate()
		if err != nil {
			return seatengine.Result{}, err
		}
		if result.Passed || result.Terminal {
			break
		}
	}
	return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, RunDir: fakeFixerRunDir}}, nil
}

// fixerRunFixture is a fixerRun over the gate fixture, a fake seat runner, a fake clock and a fake remote tip.
type fixerRunFixture struct {
	run       fixerRun
	gate      *gateFixture
	seats     *fakeFixerSeats
	deps      Deps
	clock     time.Time
	remoteTip string
}

const (
	fixerTestRow   = "Parent-Align"
	fixerTestCap   = 2
	fixerTestLimit = 5 * time.Minute
)

func newFixerRunFixture(t *testing.T) *fixerRunFixture {
	t.Helper()
	fx := &fixerRunFixture{
		gate:      newGateFixture(t, "parent-align", "go test ./...", nil),
		seats:     &fakeFixerSeats{},
		clock:     gateMarkStart,
		remoteTip: "remote-1",
	}
	scratch := t.TempDir()
	stencilsDir := t.TempDir()
	stencilkit.SeedInto(t, stencilsDir)
	if err := os.MkdirAll(fx.gate.paths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fx.gate.gate.now = func() time.Time { return fx.clock }
	fx.gate.gate.recorder = &failureRecorder{head: func() (string, error) { return "head-1", nil }}
	fx.gate.fake.writeLog = func(path string) {
		if err := os.WriteFile(path, []byte("FAIL\n"), 0o644); err != nil {
			t.Errorf("write verify log: %v", err)
		}
	}
	fx.deps = Deps{
		GateKey:            "parent-align",
		ScratchDir:         scratch,
		StencilsDir:        stencilsDir,
		DecisionRecordPath: filepath.Join(scratch, "decision-record.md"),
		MergeRecordPath:    MergeRecordPath(scratch),
		TaskBranch:         "task-branch",
		ParentBranch:       "main",
		Slug:               "my-slug",
		Config:             Config{Fixer: "claude:opus", FixerTimeoutMin: int(fixerTestLimit / time.Minute)},
		Registry:           modelspec.Registry{},
		FixerSeats:         fx.seats,
		TaskRemoteTip:      func() (string, error) { return fx.remoteTip, nil },
	}
	fx.run = newFixerRun(fx.deps, fixerTestRow, fx.gate.gate)
	return fx
}

func (fx *fixerRunFixture) reportPath() string {
	return filepath.Join(fx.deps.ScratchDir, fixerTestRow+"-fixer-report.md")
}

func (fx *fixerRunFixture) writeReport(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(fx.reportPath(), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixer report: %v", err)
	}
}

func (fx *fixerRunFixture) execute(t *testing.T, rec FixerAttempts) (fixerOutcome, FixerAttempts, error) {
	t.Helper()
	return fx.run.run(context.Background(), FixerModeParentAlign, rec, fixerTestCap, [2]string{"aligned", "merged"})
}

const (
	fixedReport    = "verdict: fixed\n\n## Parent changes\n\n## Fixes\n- c1 fixed the failing test\n"
	escalateReport = "verdict: escalate\n\n## Parent changes\n\n## Fixes\n\n## Conflict\nThe parent renamed X against decision D3.\n"
)

func TestFixerRun_Outcomes(t *testing.T) {
	t.Parallel()

	failing := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	tests := []struct {
		name       string
		start      FixerAttempts
		setup      func(t *testing.T, fx *fixerRunFixture)
		wantEnd    fixerEnd
		wantSpent  int
		wantTurns  int
		wantReason []string
	}{
		{
			name: "fixed over a passing gate with an unmoved remote tip",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.seats.turns = []func(){func() { fx.writeReport(t, fixedReport) }}
			},
			wantEnd: fixerEndFixed, wantSpent: 1, wantTurns: 1,
		},
		{
			name: "escalate spends no attempt",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.seats.turns = []func(){func() { fx.writeReport(t, escalateReport) }}
			},
			wantEnd: fixerEndEscalate, wantSpent: 0, wantTurns: 1,
			wantReason: []string{LandingStopPrefix, "decision D3", "`lyx loom decision add`", "`lyx loom resume`"},
		},
		{
			name: "a failing gate re-prompts, and the last turn's failure is spent",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.gate.fake.result = failing
				report := func() { fx.writeReport(t, fixedReport) }
				fx.seats.turns = []func(){report, report}
			},
			wantEnd: fixerEndSpent, wantSpent: fixerTestCap, wantTurns: 2,
			wantReason: []string{LandingStopPrefix, "verify-", "`lyx loom resume`"},
		},
		{
			name:  "a resumed record with one turn left runs exactly one turn",
			start: FixerAttempts{Entry: 1, Spent: fixerTestCap - 1},
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.gate.fake.result = failing
				fx.seats.turns = []func(){func() { fx.writeReport(t, fixedReport) }, func() { fx.writeReport(t, fixedReport) }}
			},
			wantEnd: fixerEndSpent, wantSpent: fixerTestCap, wantTurns: 1,
		},
		{
			name: "a session that outruns its bound without returning to the gate is the timeout end, its turn spent",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.seats.sessionTimeout = true
			},
			wantEnd: fixerEndTimeout, wantSpent: 1,
			wantReason: []string{LandingStopPrefix, fakeFixerRunDir, "`lyx loom resume`"},
		},
		{
			name: "a later turn that hangs after a failing gate is the timeout end, not fixed",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.gate.fake.result = failing
				fx.seats.turns = []func(){func() { fx.writeReport(t, fixedReport) }}
				fx.seats.hangAfter = 1
				fx.seats.hangOutcome = shuttleengine.OutcomeTimeout
			},
			wantEnd: fixerEndTimeout, wantSpent: fixerTestCap, wantTurns: 2,
			wantReason: []string{LandingStopPrefix, fakeFixerRunDir, "`lyx loom resume`"},
		},
		{
			name: "a later turn that dies after a failing gate is the no-report end, not fixed",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.gate.fake.result = failing
				fx.seats.turns = []func(){func() { fx.writeReport(t, fixedReport) }}
				fx.seats.hangAfter = 1
				fx.seats.hangOutcome = shuttleengine.OutcomeDone
			},
			wantEnd: fixerEndNoReport, wantSpent: fixerTestCap, wantTurns: 2,
			wantReason: []string{LandingStopPrefix, fakeFixerRunDir, "`lyx loom resume`"},
		},
		{
			name: "a turn that returns past fixer_timeout_min is the timeout end",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.seats.turns = []func(){func() {
					fx.writeReport(t, fixedReport)
					fx.clock = fx.clock.Add(fixerTestLimit + time.Second)
				}}
			},
			wantEnd: fixerEndTimeout, wantSpent: 1, wantTurns: 1,
		},
		{
			name: "a stale report is archived, so a turn that writes none is the no-report end",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.writeReport(t, fixedReport)
			},
			wantEnd: fixerEndNoReport, wantSpent: 1, wantTurns: 1,
			wantReason: []string{LandingStopPrefix, fakeFixerRunDir, "`lyx loom resume`"},
		},
		{
			name: "an unparseable report is the no-report end",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.seats.turns = []func(){func() { fx.writeReport(t, "no verdict here\n") }}
			},
			wantEnd: fixerEndNoReport, wantSpent: 1, wantTurns: 1,
		},
		{
			name: "a dirty tree after a turn is the dirty end",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.gate.fake.dirtyAt(0, "a.go")
				fx.seats.turns = []func(){func() { fx.writeReport(t, fixedReport) }}
			},
			wantEnd: fixerEndDirty, wantSpent: 1, wantTurns: 1,
			wantReason: []string{LandingStopPrefix, "`lyx loom resume`"},
		},
		{
			name: "a remote tip that moved during the session is the pushed end",
			setup: func(t *testing.T, fx *fixerRunFixture) {
				fx.seats.turns = []func(){func() {
					fx.writeReport(t, fixedReport)
					fx.remoteTip = "remote-2"
				}}
			},
			wantEnd: fixerEndPushed, wantSpent: 1, wantTurns: 1,
			wantReason: []string{LandingStopPrefix, "origin/task-branch at remote-2", "check the pushed commits are the task's", "`lyx loom resume`"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixerRunFixture(t)
			start := tt.start
			if start.Entry == 0 {
				start = FixerAttempts{Entry: 1}
			}
			tt.setup(t, fx)

			outcome, rec, err := fx.execute(t, start)
			if err != nil {
				t.Fatalf("run() error = %v; want nil", err)
			}
			if outcome.end != tt.wantEnd {
				t.Errorf("run() end = %q; want %q", outcome.end, tt.wantEnd)
			}
			if rec.Spent != tt.wantSpent {
				t.Errorf("run() record Spent = %d; want %d", rec.Spent, tt.wantSpent)
			}
			onDisk, err := ReadFixerAttempts(FixerAttemptsPath(fx.deps.ScratchDir, fixerTestRow), start.Entry)
			if err != nil || onDisk != rec {
				t.Errorf("attempt record on disk = %+v, %v; want the returned %+v", onDisk, err, rec)
			}
			if tt.wantTurns != 0 && fx.seats.turnsRun != tt.wantTurns {
				t.Errorf("turns run = %d; want %d", fx.seats.turnsRun, tt.wantTurns)
			}
			if outcome.reportPath != fx.reportPath() {
				t.Errorf("run() report path = %q; want %q", outcome.reportPath, fx.reportPath())
			}
			reason := outcome.haltReason(fixerTestRow)
			if tt.wantEnd == fixerEndFixed {
				if reason != "" {
					t.Errorf("haltReason() = %q for a fixed run; want none", reason)
				}
				return
			}
			for _, want := range tt.wantReason {
				if !strings.Contains(reason, want) {
					t.Errorf("haltReason() = %q; want it to contain %q", reason, want)
				}
			}
			if !strings.HasPrefix(reason, LandingStopPrefix) || !strings.Contains(reason, "way forward: ") {
				t.Errorf("haltReason() = %q; want the prefix %q and a way forward", reason, LandingStopPrefix)
			}
		})
	}

	t.Run("the table's gate holds the turns left and its timeout bounds all of them", func(t *testing.T) {
		t.Parallel()
		fx := newFixerRunFixture(t)
		fx.seats.turns = []func(){func() { fx.writeReport(t, fixedReport) }}

		if _, _, err := fx.execute(t, FixerAttempts{Entry: 1, Spent: 1}); err != nil {
			t.Fatalf("run() error = %v; want nil", err)
		}
		table := fx.seats.tables[0]
		if table.Gate[0].Name != "landing" || table.Gate[0].Attempts != 1 {
			t.Errorf("gate entry = %q with %d attempts; want landing with 1", table.Gate[0].Name, table.Gate[0].Attempts)
		}
		if table.Timeout != fixerTestLimit {
			t.Errorf("table timeout = %v; want %v for the one turn left", table.Timeout, fixerTestLimit)
		}
		if table.Values["aligned_tip"] != "aligned" || table.Values["merged_tip"] != "merged" {
			t.Errorf("table tips = %q, %q; want aligned, merged", table.Values["aligned_tip"], table.Values["merged_tip"])
		}
	})

	t.Run("a failed start leaves the record as it was and returns a transient error", func(t *testing.T) {
		t.Parallel()
		fx := newFixerRunFixture(t)
		fx.seats.startErr = fmt.Errorf("seat chair: %w", shuttleengine.ErrNotStarted)
		start := FixerAttempts{Entry: 1, Spent: 1, Baseline: "b0"}

		outcome, rec, err := fx.execute(t, start)
		if err == nil || shedengine.TransientOf(err) != shedengine.TransientAgentStart {
			t.Fatalf("run() error = %v; want one the transient mark classifies as %q", err, shedengine.TransientAgentStart)
		}
		if rec != start || outcome != (fixerOutcome{}) {
			t.Errorf("run() = %+v, %+v; want the record as it was and no outcome", outcome, rec)
		}
		onDisk, readErr := ReadFixerAttempts(FixerAttemptsPath(fx.deps.ScratchDir, fixerTestRow), 1)
		if readErr != nil || onDisk != start {
			t.Errorf("attempt record on disk = %+v, %v; want %+v", onDisk, readErr, start)
		}
	})

	t.Run("an error that is no failed start keeps the turn spent", func(t *testing.T) {
		t.Parallel()
		fx := newFixerRunFixture(t)
		fx.seats.startErr = errors.New("seat engine broke")

		_, rec, err := fx.execute(t, FixerAttempts{Entry: 1})
		if err == nil || shedengine.TransientOf(err) != "" {
			t.Fatalf("run() error = %v; want an unmarked error", err)
		}
		if rec.Spent != 1 {
			t.Errorf("run() record Spent = %d; want the turn kept spent", rec.Spent)
		}
	})
}
