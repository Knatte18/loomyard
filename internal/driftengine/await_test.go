// await_test.go — tier 0 tests for the Drift-Await producer over the fake Deps the baseline tests use.

package driftengine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDriftAwait covers every way out of the row: Done, Stuck, a deferral, and the pass with drift switched off.
// It also checks the wait on a held capture lock, the awaited_at stamp, and the durable directory commit on every pass.
func TestDriftAwait(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const awaited = "2026-01-01T00:00:00Z"
	passedAtHead := CaptureRecord{State: CapturePassed, Head: "aaa"}
	capturePath := "lym drift capture followed by lym loom resume"
	deferPath := "lym drift defer followed by lym loom resume"

	tests := []struct {
		name        string
		setup       func(f *baselineFixture)
		want        shedengine.Outcome
		wantReason  []string
		wantNoWays  []string
		wantAwaited bool
		// wantNoCommit is true for a pass that commits nothing.
		wantNoCommit bool
		check        func(t *testing.T, f *baselineFixture)
	}{
		{name: "no scope record is Done", want: shedengine.Done},
		{
			name: "drift switched off is Done with no commit, whatever the capture",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureFailed, Reason: "exit 1"})
				f.disabled = true
			},
			want:         shedengine.Done,
			wantReason:   []string{"drift.enabled = false"},
			wantNoCommit: true,
		},
		{
			name:  "scope off is Done",
			setup: func(f *baselineFixture) { f.seedScopeRecord(ScopeRecord{Scope: ScopeOff}) },
			want:  shedengine.Done,
		},
		{
			name: "awaited_at set is Done with HEAD moved",
			setup: func(f *baselineFixture) {
				f.head = "ccc"
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull, AwaitedAt: awaited})
			},
			want: shedengine.Done,
		},
		{
			name: "a missing record under partial is Stuck naming capture and defer",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopePartial, Skip: []string{"s"}})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"no baseline capture", capturePath, deferPath},
		},
		{
			name: "a failed record is Stuck naming the log",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureFailed, Reason: "exit 1", Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"ended failed (exit 1)", "log capture.log", capturePath},
		},
		{
			name: "a record that wrote no file names the plain capture way",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureFailed, WroteNoFile: true, Reason: "exit 0 but wrote no file to baseline", Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"wrote no file", capturePath, deferPath},
			wantNoWays: []string{"--scope"},
		},
		{
			name: "a timed-out record is Stuck naming the log",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureFailed, Reason: "timed out after 30m", Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"timed out after 30m", "log capture.log"},
		},
		{
			name: "a running record with a free lock is Stuck naming the log",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureRunning, Pid: 1, Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"reads running", "log capture.log"},
		},
		{
			name: "a held lock is waited on and the passed record is Done with awaited_at written",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				release := f.holdCaptureLock()
				f.onSleep = func(int) {
					f.writeCapture(passedAtHead)
					release()
				}
			},
			want:        shedengine.Done,
			wantAwaited: true,
			check: func(t *testing.T, f *baselineFixture) {
				if len(f.sleeps) != 1 {
					t.Errorf("sleeps = %v; want one, through the fake Sleep that released the lock", f.sleeps)
				}
			},
		},
		{
			name: "a record flipped from running to passed as the lock is released is read as passed",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(CaptureRecord{State: CaptureRunning, Pid: 1})
				release := f.holdCaptureLock()
				f.onSleep = func(int) {
					f.writeCapture(passedAtHead)
					release()
				}
			},
			want:        shedengine.Done,
			wantAwaited: true,
		},
		{
			name: "a passed record at HEAD is Done with awaited_at written",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(passedAtHead)
			},
			want:        shedengine.Done,
			wantAwaited: true,
		},
		{
			name: "the counted capture's commit carries the baseline directory's digest",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(passedAtHead)
			},
			want:        shedengine.Done,
			wantAwaited: true,
			check: func(t *testing.T, f *baselineFixture) {
				digest, err := BaselineDigest(f.paths.BaselineDir)
				if err != nil {
					t.Fatalf("BaselineDigest() = _, %v; want nil", err)
				}
				commit, found, err := f.store.trailerCommit(BaselineTrailerKey, false)
				if err != nil || !found || commit.Value != digest {
					t.Errorf("%s commit = %+v, %v, %v; want one carrying the digest %q", BaselineTrailerKey, commit, found, err, digest)
				}
				requireRecordsCommits(t, f, 1)
			},
		},
		{
			name: "a passed record with HEAD moved before the first await is Stuck naming only defer",
			setup: func(f *baselineFixture) {
				f.head = "bbb"
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				f.writeCapture(passedAtHead)
			},
			want:       shedengine.Stuck,
			wantReason: []string{"moved off", deferPath},
			wantNoWays: []string{"lym drift capture"},
		},
		{
			name: "after Defer the row is Done on every later entry",
			setup: func(f *baselineFixture) {
				f.head = "bbb"
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull})
				blocked := statusAt("Drift-Await", shedengine.StateBlocked, "")
				if err := Defer(f.deps, blocked); err != nil {
					f.t.Fatal(err)
				}
			},
			want: shedengine.Done,
			check: func(t *testing.T, f *baselineFixture) {
				for range 2 {
					if outcome, _, err := NewDriftAwait("Drift-Await", f.deps).Call(context.Background()); err != nil || outcome != shedengine.Done {
						t.Errorf("a later Call() = %q, %v; want Done after the deferral", outcome, err)
					}
				}
				if len(f.commits) != 3 {
					t.Errorf("commits = %d; want one per Call", len(f.commits))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newBaselineFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			if tt.wantAwaited {
				f.seedBaseline()
			}

			before, _, _ := ReadScope(f.paths)
			outcome, pointer, err := NewDriftAwait("Drift-Await", f.deps).Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != tt.want {
				t.Errorf("Call() outcome = %q (%s); want %q", outcome, pointer.Reason, tt.want)
			}
			for _, want := range tt.wantReason {
				if !strings.Contains(pointer.Reason, want) {
					t.Errorf("Call() reason = %q; want it to name %q", pointer.Reason, want)
				}
			}
			for _, banned := range tt.wantNoWays {
				if strings.Contains(pointer.Reason, banned) {
					t.Errorf("Call() reason = %q; want it to omit %q", pointer.Reason, banned)
				}
			}
			if strings.Contains(pointer.Reason, "decision halt") {
				t.Errorf("Call() reason = %q; want no decision halt prefix, so the arbiter may settle it", pointer.Reason)
			}
			if after, _, _ := ReadScope(f.paths); (after.AwaitedAt != "" && after.AwaitedAt != before.AwaitedAt) != tt.wantAwaited {
				t.Errorf("awaited_at %q after %q; want written = %v", after.AwaitedAt, before.AwaitedAt, tt.wantAwaited)
			}
			if tt.wantNoCommit {
				if len(f.commits) != 0 {
					t.Errorf("commits = %v; want none", f.commits)
				}
			} else if len(f.commits) == 0 || !slices.Equal(f.commits[0], []string{DirRel()}) {
				t.Errorf("commits = %v; want the whole durable directory", f.commits)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}
