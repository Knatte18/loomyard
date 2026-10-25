//go:build tier0

// baseline_test.go — tier 0 tests for the Drift-Baseline producer over fake Deps and a scripted seat runner.

package driftengine

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// baselineFixture is the fake world one DriftBaseline pass runs in, with counters of what the pass did.
type baselineFixture struct {
	t     *testing.T
	paths Paths
	deps  Deps

	// agentScope and agentSkip are what the scripted agent records through RecordScope; an empty agentScope records none.
	agentScope string
	agentSkip  []string
	// head, branchPoint and dirty are what the git seams read.
	head, branchPoint string
	dirty             []string
	// declared is whether drift.yaml declares a capture command.
	declared bool
	// startErr is what StartCapture answers, and runRecord what RunCapture answers.
	startErr  error
	runRecord CaptureRecord
	// onSleep runs at each Sleep with the 1-based call number.
	onSleep func(call int)

	seats   *shedfake.SeatRunner
	notes   []string
	starts  int
	runs    int
	sleeps  []time.Duration
	commits [][]string
}

// newBaselineFixture returns a fixture whose agent records a full scope, whose HEAD is on the branch point and whose capture is declared.
func newBaselineFixture(t *testing.T) *baselineFixture {
	t.Helper()
	f := &baselineFixture{t: t, paths: NewPaths(t.TempDir()), agentScope: ScopeFull, head: "aaa", branchPoint: "aaa", declared: true}
	f.runRecord = CaptureRecord{State: CapturePassed}
	f.seats = &shedfake.SeatRunner{RunFn: func(seatengine.Table) (seatengine.Result, error) {
		if err := os.MkdirAll(filepath.Dir(f.paths.BaselineReport), 0o755); err != nil {
			return seatengine.Result{}, err
		}
		if err := os.WriteFile(f.paths.BaselineReport, []byte("expected drift"), 0o644); err != nil {
			return seatengine.Result{}, err
		}
		// The real agent's second `lyx drift scope` is refused and it goes on, so the fake records only the first.
		if _, recorded, err := ReadScope(f.paths); err != nil {
			return seatengine.Result{}, err
		} else if f.agentScope != "" && !recorded {
			if err := RecordScope(f.paths, statusAt("Drift-Baseline", shedengine.StateRunning, "", false), f.agentScope, f.agentSkip); err != nil {
				return seatengine.Result{}, err
			}
		}
		return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: []string{f.paths.BaselineReport}}, nil
	}}
	f.deps = Deps{
		Paths:        f.paths,
		WorktreeRoot: t.TempDir(),
		Config: func() (Config, error) {
			if !f.declared {
				return Config{}, nil
			}
			return Config{Capture: CommandStep{Command: "capture {baseline_dir}", Dir: ".", TimeoutMin: 1}}, nil
		},
		Status: func() (shedengine.Status, error) {
			return shedengine.Status{CurrentProducer: "Drift-Baseline", State: shedengine.StateRunning}, nil
		},
		AwaitRow:    true,
		Head:        func() (string, error) { return f.head, nil },
		BranchPoint: func() (string, error) { return f.branchPoint, nil },
		DirtyPaths:  func() ([]string, error) { return f.dirty, nil },
		Commit: func(rel []string) error {
			f.commits = append(f.commits, slices.Clone(rel))
			return nil
		},
		Seats: f.seats,
		BaselineTable: func(note string) (seatengine.Table, error) {
			f.notes = append(f.notes, note)
			return seatengine.Table{Seats: []seatengine.Seat{{Outputs: []string{f.paths.BaselineReport}}}}, nil
		},
		StartCapture: func() error {
			f.starts++
			return f.startErr
		},
		RunCapture: func(context.Context) (CaptureRecord, error) {
			f.runs++
			return f.runRecord, nil
		},
		Sleep: func(_ context.Context, d time.Duration) error {
			f.sleeps = append(f.sleeps, d)
			if f.onSleep != nil {
				f.onSleep(len(f.sleeps))
			}
			return nil
		},
	}
	return f
}

// seedScopeRecord writes rec as the scope record, and report as baseline.md when true.
func (f *baselineFixture) seedScopeRecord(rec ScopeRecord, report bool) {
	f.t.Helper()
	if err := UpdateScope(f.paths, func(ScopeRecord, bool) (ScopeRecord, error) { return rec, nil }); err != nil {
		f.t.Fatal(err)
	}
	if report {
		if err := os.WriteFile(f.paths.BaselineReport, []byte("expected drift"), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
}

// writeCapture writes rec as the capture record.
func (f *baselineFixture) writeCapture(rec CaptureRecord) {
	f.t.Helper()
	if err := WriteCapture(f.paths, rec); err != nil {
		f.t.Fatal(err)
	}
}

// holdCaptureLock holds the capture lock until the returned release runs.
func (f *baselineFixture) holdCaptureLock() (release func()) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.paths.CaptureLock), 0o755); err != nil {
		f.t.Fatal(err)
	}
	held, taken, err := lock.TryAcquireWriteLock(f.paths.CaptureLock)
	if err != nil || !taken {
		f.t.Fatalf("hold the capture lock: taken %v, err %v", taken, err)
	}
	released := false
	release = func() {
		if !released {
			released = true
			_ = held.Release()
		}
	}
	f.t.Cleanup(release)
	return release
}

// TestDriftBaseline covers the producer's way through each scope, capture mode and refusal, and the paths it commits on every way out.
func TestDriftBaseline(t *testing.T) {
	t.Parallel()

	scopeAndReport := []string{path.Join(DirRel(), "scope.json"), path.Join(DirRel(), "baseline.md")}
	whole := []string{DirRel()}
	haltWays := []string{CaptureHaltPrefix, "lyx drift capture", "lyx drift defer", "lyx loom resume"}

	tests := []struct {
		name       string
		row        Row
		setup      func(f *baselineFixture)
		want       shedengine.Outcome
		wantErr    string
		wantReason []string
		wantAgents int
		wantStarts int
		wantRuns   int
		wantCommit []string
		check      func(t *testing.T, f *baselineFixture)
	}{
		{
			name:       "scope off starts no capture",
			setup:      func(f *baselineFixture) { f.agentScope = ScopeOff },
			want:       shedengine.Done,
			wantAgents: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "partial starts the detached capture",
			setup:      func(f *baselineFixture) { f.agentScope, f.agentSkip = ScopePartial, []string{"slow"} },
			want:       shedengine.Done,
			wantAgents: 1,
			wantStarts: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "a waited capture that passes is Done",
			row:        Row{WaitCapture: true},
			want:       shedengine.Done,
			wantAgents: 1,
			wantRuns:   1,
			wantCommit: whole,
		},
		{
			name: "a waited capture that fails is Stuck naming capture and defer",
			row:  Row{WaitCapture: true},
			setup: func(f *baselineFixture) {
				f.runRecord = CaptureRecord{State: CaptureFailed, Reason: "exit 1", Log: "capture.log"}
			},
			want:       shedengine.Stuck,
			wantReason: append([]string{"capture.log"}, haltWays...),
			wantAgents: 1,
			wantRuns:   1,
			wantCommit: whole,
		},
		{
			name: "a scope record without baseline.md spawns the agent with the scope note",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
			},
			want:       shedengine.Done,
			wantAgents: 1,
			wantStarts: 1,
			wantCommit: scopeAndReport,
			check: func(t *testing.T, f *baselineFixture) {
				if len(f.notes) != 1 || !strings.Contains(f.notes[0], "already recorded as full") {
					t.Errorf("scope notes = %q; want one saying the scope is already recorded as full", f.notes)
				}
			},
		},
		{
			name: "a scope record with baseline.md spawns no agent",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, true)
			},
			want:       shedengine.Done,
			wantStarts: 1,
			wantCommit: scopeAndReport,
		},
		{
			name: "awaited_at set takes no capture",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull, AwaitedAt: "2026-01-01T00:00:00Z", Deferred: true}, true)
			},
			want:       shedengine.Done,
			wantCommit: scopeAndReport,
		},
		{
			name: "a held lock is waited on under WaitCapture until released, then the passed record is left alone",
			row:  Row{WaitCapture: true},
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, true)
				release := f.holdCaptureLock()
				f.onSleep = func(call int) {
					if call == 3 {
						if err := WriteCapture(f.paths, CaptureRecord{State: CapturePassed}); err != nil {
							f.t.Fatal(err)
						}
						release()
					}
				}
			},
			want:       shedengine.Done,
			wantCommit: whole,
			check: func(t *testing.T, f *baselineFixture) {
				if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !slices.Equal(f.sleeps, want) {
					t.Errorf("sleeps = %v; want %v, doubling from one second", f.sleeps, want)
				}
			},
		},
		{
			name: "a held lock is left running without WaitCapture",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, true)
				f.holdCaptureLock()
			},
			want:       shedengine.Done,
			wantCommit: scopeAndReport,
			check: func(t *testing.T, f *baselineFixture) {
				if len(f.sleeps) != 0 {
					t.Errorf("sleeps = %v; want none, since the capture is left running", f.sleeps)
				}
			},
		},
		{
			name: "a passed record is left alone",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, true)
				if err := WriteCapture(f.paths, CaptureRecord{State: CapturePassed}); err != nil {
					f.t.Fatal(err)
				}
			},
			want:       shedengine.Done,
			wantCommit: scopeAndReport,
		},
		{
			name: "a running record with a free lock captures again",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, true)
				if err := WriteCapture(f.paths, CaptureRecord{State: CaptureRunning, Pid: 1}); err != nil {
					f.t.Fatal(err)
				}
			},
			want:       shedengine.Done,
			wantStarts: 1,
			wantCommit: scopeAndReport,
		},
		{
			name: "a failed record captures again",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, true)
				if err := WriteCapture(f.paths, CaptureRecord{State: CaptureFailed}); err != nil {
					f.t.Fatal(err)
				}
			},
			want:       shedengine.Done,
			wantStarts: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "a moved HEAD is Stuck naming defer",
			setup:      func(f *baselineFixture) { f.head = "bbb" },
			want:       shedengine.Stuck,
			wantReason: []string{CaptureHaltPrefix, "lyx drift defer", "lyx loom resume"},
			wantAgents: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "a dirty tree on the branch point names restoring or stashing, resume and defer",
			setup:      func(f *baselineFixture) { f.dirty = []string{"a.go", "b.go"} },
			want:       shedengine.Stuck,
			wantReason: []string{CaptureHaltPrefix, "a.go, b.go", "restore those paths to HEAD or stash them", "lyx loom resume", "lyx drift defer"},
			wantAgents: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "a breakaway refusal falls back to the foreground capture",
			setup:      func(f *baselineFixture) { f.startErr = ErrBreakawayRefused },
			want:       shedengine.Done,
			wantAgents: 1,
			wantStarts: 1,
			wantRuns:   1,
			wantCommit: whole,
		},
		{
			name: "a failed foreground fallback is Stuck naming capture and defer",
			setup: func(f *baselineFixture) {
				f.startErr = ErrBreakawayRefused
				f.runRecord = CaptureRecord{State: CaptureFailed, Reason: "exit 2", Log: "capture.log"}
			},
			want:       shedengine.Stuck,
			wantReason: haltWays,
			wantAgents: 1,
			wantStarts: 1,
			wantRuns:   1,
			wantCommit: whole,
		},
		{
			name:       "another start error is returned naming the child's log",
			setup:      func(f *baselineFixture) { f.startErr = errors.New("the child ended before its record; read child.log") },
			wantErr:    "child.log",
			wantAgents: 1,
			wantStarts: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "an undeclared capture names lyx config drift",
			setup:      func(f *baselineFixture) { f.declared = false },
			want:       shedengine.Stuck,
			wantReason: []string{"lyx config drift", "lyx loom resume"},
			wantAgents: 1,
			wantCommit: scopeAndReport,
		},
		{
			name:       "an agent that records no scope is Stuck",
			setup:      func(f *baselineFixture) { f.agentScope = "" },
			want:       shedengine.Stuck,
			wantReason: []string{"recorded no scope", "lyx loom resume"},
			wantAgents: 1,
			wantCommit: scopeAndReport,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newBaselineFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}

			outcome, pointer, err := NewDriftBaseline("Drift-Baseline", tt.row, f.deps).Call(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Call() error = %v; want one naming %q", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != tt.want {
					t.Errorf("Call() outcome = %q (%s); want %q", outcome, pointer.Reason, tt.want)
				}
			}
			for _, want := range tt.wantReason {
				if !strings.Contains(pointer.Reason, want) {
					t.Errorf("Call() reason = %q; want it to name %q", pointer.Reason, want)
				}
			}
			if strings.Contains(pointer.Reason, "remov") {
				t.Errorf("Call() reason = %q; want no removing offered as a way forward", pointer.Reason)
			}
			if f.seats.Calls != tt.wantAgents || f.starts != tt.wantStarts || f.runs != tt.wantRuns {
				t.Errorf("agents/starts/runs = %d/%d/%d; want %d/%d/%d", f.seats.Calls, f.starts, f.runs, tt.wantAgents, tt.wantStarts, tt.wantRuns)
			}
			if len(f.commits) != 1 || !slices.Equal(f.commits[0], tt.wantCommit) {
				t.Errorf("commits = %v; want one commit of %v", f.commits, tt.wantCommit)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}
