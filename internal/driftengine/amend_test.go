// amend_test.go — tier 0 tests for the parent's amendment over the verify fixture's in-memory records store.

package driftengine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requireOneRecordsCommit checks the store grew by exactly one commit since before, carrying Drift-Records and no seal.
func requireOneRecordsCommit(t *testing.T, f *verifyFixture, before int) {
	t.Helper()
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	if len(f.store.commits) != before+1 {
		t.Fatalf("%d commits after the amendment, %d before; want exactly one more", len(f.store.commits), before)
	}
	trailers := f.store.commits[before].trailers
	hasKey := func(key string) bool {
		return slices.ContainsFunc(trailers, func(trailer fabricengine.Trailer) bool { return trailer.Key == key })
	}
	if !hasKey(RecordsTrailerKey) || hasKey(SealTrailerKey) {
		t.Errorf("amendment commit trailers = %v; want %s and no %s", trailers, RecordsTrailerKey, SealTrailerKey)
	}
}

// TestAmend covers each form of an amendment and its refusals, each refusal naming its way forward and writing nothing.
func TestAmend(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const amendedAt = "2026-01-02T03:04:05Z"
	blocked := statusAt(verifyRow, shedengine.StateBlocked, "")
	amend := func(t *testing.T, f *verifyFixture, req AmendRequest) {
		t.Helper()
		if err := Amend(f.deps, blocked, req); err != nil {
			t.Fatalf("Amend(%+v) = %v; want nil", req, err)
		}
	}
	check := func(t *testing.T, f *verifyFixture) CheckReport {
		t.Helper()
		report, err := Check(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("Check() = _, %v; want nil", err)
		}
		return report
	}
	sealFault := func(t *testing.T, f *verifyFixture) string {
		t.Helper()
		head, err := baselineHead(f.deps)
		if err != nil {
			t.Fatalf("baselineHead() = _, %v; want nil", err)
		}
		seal, err := CheckSeal(f.deps, f.records(), head)
		if err != nil {
			t.Fatalf("CheckSeal() = _, %v; want nil", err)
		}
		return seal.Fault
	}

	t.Run("an entry that covers a formerly unexpected cell makes it expected and drops its stored ruling", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsX)
		f.storeRuling(JudgedTuple{Case: "a.txt", Identity: Identity{Key: "y", N: 1}, Kind: CellChanged, Old: "2", New: "3", Ruling: RulingNoMatch, Reason: "wrong direction"})
		if unexpected := check(t, f).Unexpected(); len(unexpected) != 1 {
			t.Fatalf("%d unexpected cells before the amendment; want one", len(unexpected))
		}
		before := len(f.store.commits)

		amend(t, f, AmendRequest{By: AmendedByParent, Entries: expectsY})

		requireOneRecordsCommit(t, f, before)
		want := []Amendment{{By: AmendedByParent, At: amendedAt, Kind: amendmentKindEntries, Entries: expectsY}}
		if got := f.scope().Amendments; !slices.Equal(got, want) {
			t.Errorf("amendments = %+v; want %+v", got, want)
		}
		if len(f.records().Judged.Tuples) != 0 {
			t.Errorf("stored tuples = %+v; want the covered cell's ruling dropped", f.records().Judged.Tuples)
		}
		if report := check(t, f); len(report.Cells) != 1 || !report.Cells[0].Expected {
			t.Errorf("cells after the amendment = %+v; want the one cell expected", report.Cells)
		}
		if fault := sealFault(t, f); fault != "" {
			t.Errorf("seal fault after an entries amendment = %q; want none, since the seal commit is untouched", fault)
		}
	})

	t.Run("under command an entry naming a case drops that case's stored rulings", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedCommand()
		err := WriteRecords(f.deps, func(_ *ScopeRecord, judged *JudgedRecord) error {
			judged.Cases = []JudgedCase{
				{CodeHead: "code-head-0", Case: "a.txt", Ruling: RulingNoMatch, Reason: "r"},
				{CodeHead: "code-head-0", Case: "b.txt", Ruling: RulingUnexpected, Reason: "r"},
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WriteRecords() = %v; want nil", err)
		}

		amend(t, f, AmendRequest{By: AmendedByParent, Entries: expectsCommandCase})

		if got := f.records().Judged.Cases; len(got) != 1 || got[0].Case != "b.txt" {
			t.Errorf("stored cases = %+v; want only b.txt's ruling kept", got)
		}
	})

	t.Run("an entry the parser refuses is refused naming the way forward and writes nothing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			prepare func(f *verifyFixture)
			entries string
			want    string
		}{
			{"ACaseOutsideTheBaseline", func(f *verifyFixture) { f.seedChanged(expectsX) }, "cases:\n  nope.txt:\n    keys:\n      - key: y\n        why: w\n", `case "nope.txt"`},
			{"AKeysEntryUnderCommand", func(f *verifyFixture) { f.seedCommand() }, expectsY, "holds keys or lines"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				tt.prepare(f)
				before := len(f.store.commits)

				err := Amend(f.deps, blocked, AmendRequest{By: AmendedByParent, Entries: tt.entries})

				if err == nil {
					t.Fatalf("Amend() = nil; want a refusal")
				}
				for _, fragment := range []string{"drift amend refused: expected.yaml", tt.want, "correct the entry in the --entries file", "lym drift amend <slug> again"} {
					if !strings.Contains(err.Error(), fragment) {
						t.Errorf("refusal %q does not name %q", err.Error(), fragment)
					}
				}
				if len(f.store.commits) != before {
					t.Errorf("%d commits after a refused amendment; want none written", len(f.store.commits)-before)
				}
			})
		}
	})

	t.Run("a request with none or two forms is refused naming the three flags before anything is read", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			req  AmendRequest
		}{
			{"None", AmendRequest{By: AmendedByParent}},
			{"EntriesAndRestore", AmendRequest{By: AmendedByParent, Entries: expectsY, Restore: true}},
			{"RestoreAndDiscard", AmendRequest{By: AmendedByParent, Restore: true, Discard: true}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				err := Amend(Deps{}, blocked, tt.req)
				if err == nil {
					t.Fatalf("Amend() = nil; want a refusal")
				}
				for _, flag := range []string{"--entries", "--restore", "--discard"} {
					if !strings.Contains(err.Error(), flag) {
						t.Errorf("refusal %q does not name %q", err.Error(), flag)
					}
				}
			})
		}
	})

	t.Run("restore makes a seal check failed by an edited file pass", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		writeTree(t, filepath.Dir(f.paths.Expected), map[string]string{expectedFileName: expectsX})
		if sealFault(t, f) == "" {
			t.Fatalf("seal fault = none after an edit; want one")
		}
		before := len(f.store.commits)

		amend(t, f, AmendRequest{By: AmendedByParent, Restore: true})

		requireOneRecordsCommit(t, f, before)
		if data, err := os.ReadFile(f.paths.Expected); err != nil || string(data) != expectsY {
			t.Errorf("expected.yaml = %q, %v; want the sealed copy %q", data, err, expectsY)
		}
		if fault := sealFault(t, f); fault != "" {
			t.Errorf("seal fault after the restore = %q; want none", fault)
		}
		if got := f.scope().Amendments; len(got) != 1 || got[0].Kind != amendmentKindRestore {
			t.Errorf("amendments = %+v; want one restore", got)
		}
	})

	t.Run("restore writes back hand-edited records at Drift-Verify and at a Drift-Expect halt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			st   VerbStatus
		}{
			{"DriftVerify", blocked},
			{"DriftExpect", statusAt(expectRow, shedengine.StateBlocked, "")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				f.seedChanged(expectsY)
				if err := os.WriteFile(f.paths.Scope, []byte(`{"scope":"full"}`), 0o644); err != nil {
					t.Fatal(err)
				}
				if !f.records().Mismatch {
					t.Fatalf("records mismatch = false after a hand edit; want true")
				}

				if err := Amend(f.deps, tt.st, AmendRequest{By: AmendedByParent, Restore: true}); err != nil {
					t.Fatalf("Amend() = %v; want nil", err)
				}

				records := f.records()
				if records.Mismatch || len(records.Scope.Amendments) != 1 || records.Scope.Diff != DiffLines {
					t.Errorf("records = mismatch %v, amendments %+v, diff %q; want the committed copy back and one restore recorded", records.Mismatch, records.Scope.Amendments, records.Scope.Diff)
				}
			})
		}
	})

	t.Run("restore writes back hand-edited records when the scope is off", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		off := countedScope()
		off.Scope = ScopeOff
		f.seed(off, verifyBaseline)
		if err := os.WriteFile(f.paths.Scope, []byte(`{"scope":"off","awaited_at":"edited"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if !f.records().Mismatch {
			t.Fatalf("records mismatch = false after a hand edit; want true")
		}

		amend(t, f, AmendRequest{By: AmendedByParent, Restore: true})

		if records := f.records(); records.Mismatch || records.Scope.Scope != ScopeOff || records.Scope.AwaitedAt == "edited" {
			t.Errorf("records = mismatch %v, scope %q, awaited_at %q; want the committed copy back", records.Mismatch, records.Scope.Scope, records.Scope.AwaitedAt)
		}
	})

	t.Run("restore with nothing failed is refused", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged(expectsY)
		before := len(f.store.commits)

		err := Amend(f.deps, blocked, AmendRequest{By: AmendedByParent, Restore: true})

		if err == nil || !strings.Contains(err.Error(), "nothing to restore") || !strings.Contains(err.Error(), "lym loom status") {
			t.Errorf("Amend() = %v; want a refusal naming nothing to restore and lym loom status", err)
		}
		if len(f.store.commits) != before {
			t.Errorf("%d commits after a refused restore; want none", len(f.store.commits)-before)
		}
	})

	t.Run("discard clears a seal made at another code head, and the run is then checked against its amendments alone", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seed(countedScope(), verifyBaseline)
		f.moveHead("code-head-1")
		f.seal(expectsY)
		f.current = verifyChanged
		if sealFault(t, f) == "" {
			t.Fatalf("seal fault = none after a seal at another code head; want one")
		}
		before := len(f.store.commits)

		amend(t, f, AmendRequest{By: AmendedByOperator, Discard: true})

		requireOneRecordsCommit(t, f, before)
		report := check(t, f)
		if report.SealFault != "" || len(report.Unexpected()) != 1 {
			t.Errorf("report = seal fault %q, %d unexpected cells; want no fault and the change unexpected under an empty expectation", report.SealFault, len(report.Unexpected()))
		}

		amend(t, f, AmendRequest{By: AmendedByOperator, Entries: expectsY})

		if unexpected := check(t, f).Unexpected(); len(unexpected) != 0 {
			t.Errorf("%d unexpected cells after an entries amendment; want the amendments alone to cover the change", len(unexpected))
		}
	})

	t.Run("discard clears an expected.yaml no commit sealed", func(t *testing.T) {
		t.Parallel()
		f := newVerifyFixture(t)
		f.seedChanged("")
		writeTree(t, filepath.Dir(f.paths.Expected), map[string]string{expectedFileName: expectsY})
		if sealFault(t, f) == "" {
			t.Fatalf("seal fault = none beside an unsealed file; want one")
		}

		amend(t, f, AmendRequest{By: AmendedByParent, Discard: true})

		if fault := sealFault(t, f); fault != "" {
			t.Errorf("seal fault after the discard = %q; want none", fault)
		}
	})

	t.Run("an amendment is admitted at a blocked writer row, Drift-Verify and Drift-Recheck only, and at Drift-Expect for restore alone", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			st      VerbStatus
			wantErr []string
		}{
			{"ABlockedDriftExpectForDiscard", statusAt(expectRow, shedengine.StateBlocked, ""), []string{"Drift-Expect", "--restore"}},
			{"ABlockedDarnWrite", statusAt(darnWriteRow, shedengine.StateBlocked, ""), nil},
			{"ABlockedWebster", statusAt(websterRow, shedengine.StateBlocked, ""), nil},
			{"ABlockedDriftRecheck", statusAt(recheckRow, shedengine.StateBlocked, ""), nil},
			{"ARunningDriftVerify", statusAt(verifyRow, shedengine.StateRunning, ""), []string{"running", "lym loom status", "halt"}},
			{"ABlockedParentAlign", statusAt("Parent-Align", shedengine.StateBlocked, ""), []string{"Parent-Align", "Darn-Write", "Webster", "Drift-Verify", "Drift-Recheck"}},
			{"ARowBeforeDriftExpect", statusAt("Drift-Await", shedengine.StateBlocked, ""), []string{"Drift-Await", "Darn-Write", "lym loom status"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newVerifyFixture(t)
				f.seedChanged(expectsY)
				before := len(f.store.commits)

				err := Amend(f.deps, tt.st, AmendRequest{By: AmendedByParent, Discard: true})

				if tt.wantErr == nil {
					if err != nil {
						t.Fatalf("Amend() = %v; want nil", err)
					}
					requireOneRecordsCommit(t, f, before)
					return
				}
				if err == nil {
					t.Fatalf("Amend() = nil; want a refusal naming %q", tt.wantErr)
				}
				for _, fragment := range tt.wantErr {
					if !strings.Contains(err.Error(), fragment) {
						t.Errorf("refusal %q does not name %q", err.Error(), fragment)
					}
				}
				if len(f.store.commits) != before {
					t.Errorf("%d commits after a refused amendment; want none", len(f.store.commits)-before)
				}
			})
		}
	})
}
