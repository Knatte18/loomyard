//go:build tier0

// await_test.go — tier 0 tests for the Drift-Await producer over the fake Deps the baseline tests use.

package driftengine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestDriftAwait covers each Done and Stuck way out of the row, the wait on a held capture lock, awaited_at written on a passed capture, a deferral and the commit of the durable directory on every pass.
func TestDriftAwait(t *testing.T) {
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
		check       func(t *testing.T, f *baselineFixture)
	}{
		{name: "no scope record is Done", want: shedengine.Done},
		{
			name:  "scope off is Done",
			setup: func(f *baselineFixture) { f.seedScopeRecord(ScopeRecord{Scope: ScopeOff}, false) },
			want:  shedengine.Done,
		},
		{
			name: "awaited_at set is Done with HEAD moved",
			setup: func(f *baselineFixture) {
				f.head = "ccc"
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull, AwaitedAt: awaited}, false)
			},
			want: shedengine.Done,
		},
		{
			name: "a missing record under partial is Stuck naming capture and defer",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopePartial, Skip: []string{"s"}}, false)
			},
			want:       shedengine.Stuck,
			wantReason: []string{"no baseline capture", capturePath, deferPath},
		},
		{
			name: "a failed record is Stuck naming the log",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
				f.writeCapture(CaptureRecord{State: CaptureFailed, Reason: "exit 1", Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"ended failed (exit 1)", "log capture.log", capturePath},
		},
		{
			name: "a timed-out record is Stuck naming the log",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
				f.writeCapture(CaptureRecord{State: CaptureFailed, Reason: "timed out after 30m", Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"timed out after 30m", "log capture.log"},
		},
		{
			name: "a running record with a free lock is Stuck naming the log",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
				f.writeCapture(CaptureRecord{State: CaptureRunning, Pid: 1, Log: "capture.log"})
			},
			want:       shedengine.Stuck,
			wantReason: []string{"reads running", "log capture.log"},
		},
		{
			name: "a held lock is waited on and the passed record is Done with awaited_at written",
			setup: func(f *baselineFixture) {
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
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
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
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
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
				f.writeCapture(passedAtHead)
			},
			want:        shedengine.Done,
			wantAwaited: true,
		},
		{
			name: "a passed record with HEAD moved before the first await is Stuck naming only defer",
			setup: func(f *baselineFixture) {
				f.head = "bbb"
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
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
				f.seedScopeRecord(ScopeRecord{Scope: ScopeFull}, false)
				blocked := statusAt("Drift-Await", shedengine.StateBlocked, "", false)
				if err := Defer(f.paths, blocked); err != nil {
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
			if len(f.commits) == 0 || !slices.Equal(f.commits[0], []string{DirRel()}) {
				t.Errorf("commits = %v; want the whole durable directory", f.commits)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}
