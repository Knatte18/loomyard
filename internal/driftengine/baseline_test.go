// baseline_test.go — tier 0 tests for the Drift-Baseline producer over fake Deps and the in-memory records store.

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

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// baselineFixture is the fake world one DriftBaseline pass runs in, with counters of what the pass did.
type baselineFixture struct {
	t     *testing.T
	store *commitStore
	paths Paths
	deps  Deps

	// head, branchPoint and dirty are what the git seams read.
	head, branchPoint string
	dirty             []string
	// declared is whether drift.yaml declares a capture command.
	declared bool
	// disabled switches drift off, and configErr fails the config load.
	disabled  bool
	configErr error
	// configScope is the drift config's scope key, and configWasPartial says the config read it from a stored partial.
	configScope      string
	configWasPartial bool
	// startErr is what StartCapture answers, and runRecord what RunCapture answers.
	startErr  error
	runRecord CaptureRecord
	// onSleep runs at each Sleep with the 1-based call number.
	onSleep func(call int)

	// seats must stay untouched, since no row of the baseline spawns an agent.
	seats   *shedfake.SeatRunner
	starts  int
	runs    int
	sleeps  []time.Duration
	commits [][]string
	// endedRoles holds the roles of each call of EndRoles, and scopeAtEnd whether a scope was recorded when it was called.
	endedRoles [][]string
	scopeAtEnd []bool
}

// newBaselineFixture returns a fixture whose HEAD is on the branch point and whose capture is declared, over a fresh records store.
func newBaselineFixture(t *testing.T) *baselineFixture {
	t.Helper()
	store := newCommitStore(t)
	f := &baselineFixture{t: t, store: store, paths: NewPaths(store.root), head: "aaa", branchPoint: "aaa", declared: true, seats: &shedfake.SeatRunner{}}
	f.runRecord = CaptureRecord{State: CapturePassed}

	d := store.deps(f.paths)
	d.Config = func() (Config, error) {
		if f.configErr != nil {
			return Config{}, f.configErr
		}
		cfg := Config{Enabled: !f.disabled, Scope: f.configScope, ScopeWasPartial: f.configWasPartial, Diff: DiffLines, KeyPatterns: []string{keyValuePattern}, Volatile: Volatile{Keys: []string{"captured_at"}}}
		if f.declared {
			cfg.Capture = CommandStep{Command: "capture {baseline_dir}", Dir: ".", TimeoutMin: 1}
		}
		return cfg, nil
	}
	d.Status = func() (shedengine.Status, error) {
		return shedengine.Status{CurrentProducer: "Drift-Baseline", State: shedengine.StateRunning}, nil
	}
	d.Head = func() (string, error) { return f.head, nil }
	d.BranchPoint = func() (string, error) { return f.branchPoint, nil }
	d.DirtyPaths = func() ([]string, error) { return f.dirty, nil }
	d.Commit = func(rel []string) error {
		f.commits = append(f.commits, slices.Clone(rel))
		return nil
	}
	d.Seats = f.seats
	d.EndRoles = func(roles []string) error {
		f.endedRoles = append(f.endedRoles, slices.Clone(roles))
		_, found, err := ReadScope(f.paths)
		f.scopeAtEnd = append(f.scopeAtEnd, found)
		return err
	}
	d.StartCapture = func() error {
		f.starts++
		return f.startErr
	}
	d.RunCapture = func(context.Context) (CaptureRecord, error) {
		f.runs++
		return f.runRecord, nil
	}
	d.Sleep = func(_ context.Context, delay time.Duration) error {
		f.sleeps = append(f.sleeps, delay)
		if f.onSleep != nil {
			f.onSleep(len(f.sleeps))
		}
		return nil
	}
	f.deps = d
	return f
}

// seedScopeRecord writes rec as the scope record, as a run seeded before the records guard existed holds it.
func (f *baselineFixture) seedScopeRecord(rec ScopeRecord) {
	f.t.Helper()
	if err := UpdateScope(f.paths, func(ScopeRecord, bool) (ScopeRecord, error) { return rec, nil }); err != nil {
		f.t.Fatal(err)
	}
}

// seedBaseline writes a baseline directory with one file, which Drift-Await digests when it counts a capture.
func (f *baselineFixture) seedBaseline() {
	f.t.Helper()
	writeTree(f.t, f.paths.BaselineDir, map[string]string{"a.txt": "x=1\n"})
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

// checkRecordedScope fails t unless the scope record holds want's scope, source, diff mode, key patterns and volatile keys.
func checkRecordedScope(t *testing.T, f *baselineFixture, want ScopeRecord) {
	t.Helper()
	got, found, err := ReadScope(f.paths)
	if err != nil || !found || got.Scope != want.Scope || got.ScopeSource != want.ScopeSource || got.Diff != want.Diff || !slices.Equal(got.KeyPatterns, want.KeyPatterns) || !slices.Equal(got.Volatile.Keys, want.Volatile.Keys) {
		t.Errorf("ReadScope() = %+v, %v, %v; want scope %q from %q, diff %q, patterns %q, volatile keys %q", got, found, err, want.Scope, want.ScopeSource, want.Diff, want.KeyPatterns, want.Volatile.Keys)
	}
}

// requireRecordsCommits fails t unless the records store holds exactly n commits and each carries the Drift-Records trailer.
func requireRecordsCommits(t *testing.T, f *baselineFixture, n int) {
	t.Helper()
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	if len(f.store.commits) != n {
		t.Errorf("%d records commits; want %d", len(f.store.commits), n)
	}
	for _, commit := range f.store.commits {
		if !slices.ContainsFunc(commit.trailers, func(trailer fabricengine.Trailer) bool { return trailer.Key == RecordsTrailerKey }) {
			t.Errorf("commit %s carries trailers %v; want %s", commit.sha, commit.trailers, RecordsTrailerKey)
		}
	}
}

// TestDriftBaseline covers the scope the row records from config, the capture start and refusal, the pass with drift switched off, and the paths it commits on every way out.
func TestDriftBaseline(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	whole := []string{DirRel()}
	haltWays := []string{CaptureHaltPrefix + DeferHaltPrefix, "lym drift capture", "lym drift defer", "lym loom resume"}
	fullFromConfig := ScopeRecord{Scope: ScopeFull, Diff: DiffLines, KeyPatterns: []string{keyValuePattern}, Volatile: Volatile{Keys: []string{"captured_at"}}}

	tests := []struct {
		name       string
		setup      func(f *baselineFixture)
		want       shedengine.Outcome
		wantErr    string
		wantReason []string
		// wantNoReason lists text the reason must not hold.
		wantNoReason []string
		wantStarts   int
		wantRuns     int
		// wantCommit nil wants no commit through Deps.Commit at all.
		wantCommit []string
		// wantDeferrable is true for a Stuck that lym drift defer settles.
		wantDeferrable bool
		check          func(t *testing.T, f *baselineFixture)
	}{
		{
			name:  "drift.scope off records off with its source and takes no capture",
			setup: func(f *baselineFixture) { f.configScope = ScopeOff },
			want:  shedengine.Done,
			check: func(t *testing.T, f *baselineFixture) {
				checkRecordedScope(t, f, ScopeRecord{Scope: ScopeOff, ScopeSource: driftScopeKey})
				requireRecordsCommits(t, f, 1)
			},
		},
		{
			name:       "an unset scope records full with the config's diff and patterns and starts the capture",
			want:       shedengine.Done,
			wantStarts: 1,
			check: func(t *testing.T, f *baselineFixture) {
				checkRecordedScope(t, f, fullFromConfig)
				requireRecordsCommits(t, f, 1)
			},
		},
		{
			name:       "a config scope read from partial is recorded full",
			setup:      func(f *baselineFixture) { f.configScope, f.configWasPartial = ScopeFull, true },
			want:       shedengine.Done,
			wantStarts: 1,
			check: func(t *testing.T, f *baselineFixture) {
				checkRecordedScope(t, f, fullFromConfig)
			},
		},
		{
			name: "a recorded scope survives a config change, and so do its mode and patterns",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull, Diff: DiffCommand, KeyPatterns: []string{"other"}})
				f.configScope = ScopeOff
			},
			want:       shedengine.Done,
			wantStarts: 1,
			check: func(t *testing.T, f *baselineFixture) {
				checkRecordedScope(t, f, ScopeRecord{Scope: ScopeFull, Diff: DiffCommand, KeyPatterns: []string{"other"}})
				requireRecordsCommits(t, f, 0)
			},
		},
		{
			name: "an old recorded partial is kept and captured",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopePartial, Skip: []string{"slow"}})
			},
			want:       shedengine.Done,
			wantStarts: 1,
			check: func(t *testing.T, f *baselineFixture) {
				checkRecordedScope(t, f, ScopeRecord{Scope: ScopePartial})
			},
		},
		{
			name: "a live chair of either removed role is ended before the scope is recorded",
			want: shedengine.Done, wantStarts: 1,
			check: func(t *testing.T, f *baselineFixture) {
				if want := [][]string{{"drift-baseline", "drift-verify"}}; !slices.EqualFunc(f.endedRoles, want, slices.Equal) {
					t.Errorf("ended roles = %v; want %v", f.endedRoles, want)
				}
				if !slices.Equal(f.scopeAtEnd, []bool{false}) {
					t.Errorf("scope recorded when the chairs were ended = %v; want none yet", f.scopeAtEnd)
				}
			},
		},
		{
			name: "awaited_at set takes no capture",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull, AwaitedAt: "2026-01-01T00:00:00Z", Deferred: true})
			},
			want: shedengine.Done,
		},
		{
			name: "a held lock is left running",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.holdCaptureLock()
			},
			want: shedengine.Done,
			check: func(t *testing.T, f *baselineFixture) {
				if len(f.sleeps) != 0 {
					t.Errorf("sleeps = %v; want none, since the capture is left running", f.sleeps)
				}
			},
		},
		{
			name: "a passed record is left alone",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				if err := WriteCapture(f.paths, CaptureRecord{State: CapturePassed}); err != nil {
					f.t.Fatal(err)
				}
			},
			want: shedengine.Done,
		},
		{
			name: "a running record with a free lock captures again",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				if err := WriteCapture(f.paths, CaptureRecord{State: CaptureRunning, Pid: 1}); err != nil {
					f.t.Fatal(err)
				}
			},
			want:       shedengine.Done,
			wantStarts: 1,
		},
		{
			name: "a failed record captures again",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				if err := WriteCapture(f.paths, CaptureRecord{State: CaptureFailed}); err != nil {
					f.t.Fatal(err)
				}
			},
			want:       shedengine.Done,
			wantStarts: 1,
		},
		{
			name:       "a moved HEAD is Stuck naming defer",
			setup:      func(f *baselineFixture) { f.head = "bbb" },
			want:       shedengine.Stuck,
			wantReason: []string{CaptureHaltPrefix + DeferHaltPrefix, "lym drift defer", "lym loom resume"},
		},
		{
			name:       "a dirty tree on the branch point names restoring or stashing, resume and defer",
			setup:      func(f *baselineFixture) { f.dirty = []string{"a.go", "b.go"} },
			want:       shedengine.Stuck,
			wantReason: []string{CaptureHaltPrefix + DeferHaltPrefix, "a.go, b.go", "restore those paths to HEAD or stash them", "lym loom resume", "lym drift defer"},
		},
		{
			name:       "a breakaway refusal falls back to the foreground capture",
			setup:      func(f *baselineFixture) { f.startErr = ErrBreakawayRefused },
			want:       shedengine.Done,
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
			wantStarts: 1,
			wantRuns:   1,
			wantCommit: whole,
		},
		{
			name: "a child that did not record its start is a capture halt naming the child's log, which lym drift defer settles",
			setup: func(f *baselineFixture) {
				f.startErr = startFailure(SpawnRequest{}, "child.log", "ended before it recorded its start")
			},
			want:           shedengine.Stuck,
			wantReason:     []string{CaptureHaltPrefix + DeferHaltPrefix, "child.log", "lym loom resume", "lym drift defer"},
			wantStarts:     1,
			wantDeferrable: true,
		},
		{
			name: "a child that ended on an error a resume cannot change is a capture halt offering only defer and a fix",
			setup: func(f *baselineFixture) {
				f.startErr = childNoRecordError{text: "drift: the capture child ended before it recorded its start: cwd /w does not equal the anchored directory /w/backend; its log is child.log", lasting: true}
			},
			want:           shedengine.Stuck,
			wantReason:     []string{CaptureHaltPrefix + DeferHaltPrefix, "does not equal the anchored directory", "child.log", "lym loom resume cannot change this error", "lym drift defer followed by lym loom resume", "fix of the error's cause"},
			wantNoReason:   []string{"lym loom resume to capture again"},
			wantStarts:     1,
			wantDeferrable: true,
		},
		{
			name:       "an environment fault starting the capture is an error that offers no defer",
			setup:      func(f *baselineFixture) { f.startErr = errors.New("drift: resolve this binary: no such file") },
			wantErr:    "resolve this binary",
			wantStarts: 1,
		},
		{
			name:       "a held capture lock at the start keeps its own refusal",
			setup:      func(f *baselineFixture) { f.startErr = lockHeldRefusal("capture.lock") },
			wantErr:    "a capture already holds capture.lock",
			wantStarts: 1,
		},
		{
			name: "drift switched off passes a run halted on a failed capture, with no capture or commit",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureFailed})
				f.disabled = true
			},
			want:       shedengine.Done,
			wantReason: []string{"drift.enabled = false"},
			check: func(t *testing.T, f *baselineFixture) {
				if len(f.endedRoles) != 0 {
					t.Errorf("ended roles = %v; want none while drift is off", f.endedRoles)
				}
			},
		},
		{
			name:       "a drift config that fails to load is Stuck naming lym loom resume",
			setup:      func(f *baselineFixture) { f.configErr = errors.New(`drift config key "capture.timeout_min": missing`) },
			want:       shedengine.Stuck,
			wantReason: []string{"capture.timeout_min", "lym loom resume"},
		},
		{
			name:       "an undeclared capture names lym config drift",
			setup:      func(f *baselineFixture) { f.declared = false },
			want:       shedengine.Stuck,
			wantReason: []string{"lym config drift", "lym loom resume"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newBaselineFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}

			outcome, pointer, err := NewDriftBaseline("Drift-Baseline", Row{}, f.deps).Call(context.Background())
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
			for _, unwanted := range tt.wantNoReason {
				if strings.Contains(pointer.Reason, unwanted) {
					t.Errorf("Call() reason = %q; want it without %q", pointer.Reason, unwanted)
				}
			}
			if strings.HasPrefix(pointer.Reason, CaptureHaltPrefix) && !strings.HasPrefix(pointer.Reason, CaptureHaltPrefix+DeferHaltPrefix) {
				t.Errorf("Call() reason = %q; want a capture halt to carry the defer prefix right after the capture prefix", pointer.Reason)
			}
			if strings.Contains(pointer.Reason, "remov") {
				t.Errorf("Call() reason = %q; want no removing offered as a way forward", pointer.Reason)
			}
			if f.seats.Calls+f.seats.ProbeCalls+f.seats.ResumeCalls != 0 {
				t.Errorf("seat runner calls = %d runs, %d probes, %d resumes; want no agent", f.seats.Calls, f.seats.ProbeCalls, f.seats.ResumeCalls)
			}
			if f.starts != tt.wantStarts || f.runs != tt.wantRuns {
				t.Errorf("starts/runs = %d/%d; want %d/%d", f.starts, f.runs, tt.wantStarts, tt.wantRuns)
			}
			if tt.wantCommit == nil {
				if len(f.commits) != 0 {
					t.Errorf("commits = %v; want none through Deps.Commit", f.commits)
				}
			} else if len(f.commits) != 1 || !slices.Equal(f.commits[0], tt.wantCommit) {
				t.Errorf("commits = %v; want one commit of %v", f.commits, tt.wantCommit)
			}
			if tt.wantDeferrable {
				if err := Defer(f.deps, statusAt("Drift-Baseline", shedengine.StateBlocked, pointer.Reason)); err != nil {
					t.Errorf("Defer() at the halt = %v; want it admitted", err)
				}
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}
