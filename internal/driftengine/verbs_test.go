// verbs_test.go — tier 0 tests for the halts Defer admits and refuses, over a fake run status and seeded scope records.

package driftengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// statusAt returns a VerbStatus at row in state, with an error text.
func statusAt(row string, state shedengine.State, errText string) VerbStatus {
	return VerbStatus{
		Status: shedengine.Status{CurrentProducer: row, State: state, Error: errText},
	}
}

// mustReadScope reads the scope record and its presence.
func mustReadScope(t *testing.T, p Paths) (ScopeRecord, bool) {
	t.Helper()
	rec, found, err := ReadScope(p)
	if err != nil {
		t.Fatalf("ReadScope() = %v", err)
	}
	return rec, found
}

// requireRefusal checks that err names every fragment of want and that the scope record still reads as it did before.
func requireRefusal(t *testing.T, err error, want []string, p Paths, before *ScopeRecord) {
	t.Helper()
	if err == nil {
		t.Fatalf("verb = nil; want a refusal naming %q", want)
	}
	for _, fragment := range want {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("refusal %q does not name %q", err.Error(), fragment)
		}
	}
	got, found := mustReadScope(t, p)
	if before == nil {
		if found {
			t.Errorf("a refused verb wrote the scope record %+v", got)
		}
		return
	}
	if !found || !reflect.DeepEqual(got, *before) {
		t.Errorf("a refused verb changed the scope record to %+v; want %+v", got, *before)
	}
}

// TestDefer covers the halts Defer admits and every refusal, each leaving the record as it was.
func TestDefer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	scoped := &ScopeRecord{Scope: ScopeFull}

	t.Run("Defer", func(t *testing.T) {
		t.Parallel()
		const decisionDefer = shedadapters.DecisionHaltPrefix + DeferHaltPrefix
		scopeOff := &ScopeRecord{Scope: ScopeOff}
		tests := []struct {
			name string
			st   VerbStatus
			// scope is the seeded scope record, scoped when nil, and baseline says whether the baseline directory exists.
			scope    *ScopeRecord
			baseline bool
			wantErr  []string
		}{
			{name: "blocked at Drift-Await", st: statusAt("Drift-Await", shedengine.StateBlocked, "")},
			{name: "blocked at a Drift-Baseline capture halt", st: statusAt("Drift-Baseline", shedengine.StateBlocked, CaptureHaltPrefix+DeferHaltPrefix+"exit 1")},
			{name: "blocked at a Drift-Expect halt whose head moved", st: statusAt(expectRow, shedengine.StateBlocked, decisionDefer+"the code HEAD moved")},
			{name: "blocked at a Drift-Verify halt whose capture never counted and whose current capture failed", st: statusAt(verifyRow, shedengine.StateBlocked, decisionDefer+"the current capture did not pass"), baseline: true},
			{name: "blocked at a Drift-Recheck halt with a missing baseline directory", st: statusAt(recheckRow, shedengine.StateBlocked, decisionDefer+"the baseline directory is missing")},
			{name: "blocked at a writer halt that names no defer, over a failed baseline check", st: statusAt(darnWriteRow, shedengine.StateBlocked, "something else")},
			{name: "blocked at another Drift-Baseline halt", st: statusAt("Drift-Baseline", shedengine.StateBlocked, "something else"), wantErr: []string{"lym loom status", "capture halt"}},
			{name: "running at Drift-Await", st: statusAt("Drift-Await", shedengine.StateRunning, ""), wantErr: []string{"lym loom status"}},
			{name: "blocked at a Drift-Verify halt on an unexpected cell", st: statusAt(verifyRow, shedengine.StateBlocked, shedadapters.DecisionHaltPrefix+"the code changed what the expectation does not cover"), baseline: true, wantErr: []string{"Drift-Await", "Drift-Verify", "lym loom status"}},
			{name: "blocked at Parent-Align", st: statusAt("Parent-Align", shedengine.StateBlocked, decisionDefer+"x"), baseline: true, wantErr: []string{"Drift-Await", "Drift-Baseline", "lym loom status"}},
			{name: "blocked at Darn-Write on a scope-off run", st: statusAt(darnWriteRow, shedengine.StateBlocked, "something else"), scope: scopeOff, wantErr: []string{"Darn-Write", "lym loom status"}},
			{name: "capture lock held at an admitted halt", st: VerbStatus{Status: shedengine.Status{CurrentProducer: "Drift-Await", State: shedengine.StateBlocked}, CaptureHeld: true}, wantErr: []string{"lym loom resume", "running capture", "lym drift defer again"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				store := newCommitStore(t)
				p := NewPaths(store.root)
				before := scoped
				if tt.scope != nil {
					before = tt.scope
				}
				if err := UpdateScope(p, func(ScopeRecord, bool) (ScopeRecord, error) { return *before, nil }); err != nil {
					t.Fatalf("seed the scope record: %v", err)
				}
				if tt.baseline {
					writeTree(t, p.BaselineDir, map[string]string{"a.txt": "x=1\n"})
				}

				err := Defer(store.deps(p), tt.st)

				if tt.wantErr != nil {
					requireRefusal(t, err, tt.wantErr, p, before)
					return
				}
				if err != nil {
					t.Fatalf("Defer() = %v; want nil", err)
				}
				got, _ := mustReadScope(t, p)
				if !got.Deferred || got.AwaitedAt == "" {
					t.Errorf("record = %+v; want deferred with awaited_at set", got)
				}
				if _, found, err := store.trailerCommit(RecordsTrailerKey, true); err != nil || !found {
					t.Errorf("records commit = %v, %v; want the deferral committed with %s", found, err, RecordsTrailerKey)
				}
			})
		}
	})
}
