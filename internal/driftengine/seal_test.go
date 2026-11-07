// seal_test.go — tier 0 tests for the expectation's seal, the baseline digest and the row order, over the in-memory commitStore.

package driftengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// sealHead is the code head the store reports while a test seals or commits.
const sealHead = "code-head-1"

// sealDeps returns a store and its Deps with the status at row current and a recipe order that holds Drift-Expect.
func sealDeps(t *testing.T, current string) (*commitStore, Deps) {
	t.Helper()
	store := newCommitStore(t)
	store.codeHead = sealHead
	d := store.deps(NewPaths(store.root))
	d.Rows = []string{"Drift-Baseline", "Drift-Await", "Drift-Expect", "Darn-Write", "Drift-Verify"}
	d.Status = func() (shedengine.Status, error) { return shedengine.Status{CurrentProducer: current}, nil }
	return store, d
}

// seedBaseline writes the baseline directory's files.
func seedBaseline(t *testing.T, d Deps) {
	t.Helper()
	writeFileString(t, filepath.Join(d.Paths.BaselineDir, "a.txt"), "one\n")
	writeFileString(t, filepath.Join(d.Paths.BaselineDir, "sub", "b.txt"), "two\n")
}

// commitBaselineDigest commits the baseline's current digest under the baseline trailer.
func commitBaselineDigest(t *testing.T, d Deps) {
	t.Helper()
	digest, err := BaselineDigest(d.Paths.BaselineDir)
	if err != nil {
		t.Fatalf("BaselineDigest() = %v; want nil", err)
	}
	if err := d.CommitWithTrailers(nil, []fabricengine.Trailer{{Key: BaselineTrailerKey, Value: digest}}); err != nil {
		t.Fatalf("commit the baseline digest: %v", err)
	}
}

// TestSealAndBaseline covers each fault CheckSeal reports, the baseline digest read from its commit, and the row order.
func TestSealAndBaseline(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const expected = "cases: {}\n"
	sealed := func(t *testing.T, _ *commitStore, d Deps) Records {
		t.Helper()
		writeFileString(t, d.Paths.Expected, expected)
		if err := SealExpectation(d); err != nil {
			t.Fatalf("SealExpectation() = %v; want nil", err)
		}
		return Records{}
	}

	sealTests := []struct {
		name         string
		current      string
		baselineHead string
		setup        func(t *testing.T, store *commitStore, d Deps) Records
		want         SealState
		wantFault    string
	}{
		{name: "ValidSealReadsItsBytes", current: "Drift-Verify", setup: sealed, want: SealState{Sealed: []byte(expected)}},
		{
			name: "EditedFileFaultsRestoreAndKeepsTheSealedCopy", current: "Drift-Verify",
			setup: func(t *testing.T, store *commitStore, d Deps) Records {
				rec := sealed(t, store, d)
				writeFileString(t, d.Paths.Expected, "cases: {x: {}}\n")
				return rec
			},
			want: SealState{Sealed: []byte(expected)}, wantFault: "--restore",
		},
		{name: "SealMadeAtAnotherCodeHeadFaultsDiscard", current: "Drift-Verify", baselineHead: "another-head", setup: sealed, wantFault: "--discard"},
		{
			name: "SealCopyFailingItsOwnHashFaultsDiscard", current: "Drift-Verify",
			setup: func(t *testing.T, _ *commitStore, d Deps) Records {
				writeFileString(t, d.Paths.Expected, expected)
				if err := d.CommitWithTrailers([]string{durableRel(expectedFileName)}, []fabricengine.Trailer{{Key: SealTrailerKey, Value: "not-the-hash"}}); err != nil {
					t.Fatalf("commit the bad seal: %v", err)
				}
				return Records{}
			},
			wantFault: "--discard",
		},
		{
			name: "FileWithNoSealPastDriftExpectFaultsDiscard", current: "Drift-Verify",
			setup: func(t *testing.T, _ *commitStore, d Deps) Records {
				writeFileString(t, d.Paths.Expected, expected)
				return Records{}
			},
			wantFault: "--discard",
		},
		{name: "NoFileAndNoSealPastDriftExpectIsEmpty", current: "Drift-Verify", setup: func(*testing.T, *commitStore, Deps) Records { return Records{} }, want: SealState{Empty: true}},
		{name: "RowBeforeDriftExpectIsNotYet", current: "Drift-Await", setup: sealed, want: SealState{NotYet: true}},
		{
			name: "UnsealedRecordIgnoresALaterSeal", current: "Drift-Verify",
			setup: func(t *testing.T, store *commitStore, d Deps) Records {
				rec := sealed(t, store, d)
				rec.Scope.Unsealed = true
				return rec
			},
			want: SealState{Empty: true},
		},
		{
			name: "DiscardAmendmentSkipsTheCheck", current: "Drift-Verify",
			setup: func(t *testing.T, _ *commitStore, d Deps) Records {
				writeFileString(t, d.Paths.Expected, expected)
				return Records{Scope: ScopeRecord{Amendments: []Amendment{{Kind: amendmentKindDiscard}}}}
			},
			want: SealState{Empty: true},
		},
	}
	for _, tt := range sealTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store, d := sealDeps(t, tt.current)
			rec := tt.setup(t, store, d)
			baselineHead := tt.baselineHead
			if baselineHead == "" {
				baselineHead = sealHead
			}
			got, err := CheckSeal(d, rec, baselineHead)
			if err != nil {
				t.Fatalf("CheckSeal() = %v; want nil", err)
			}
			if tt.wantFault != "" {
				if !strings.Contains(got.Fault, tt.wantFault) {
					t.Fatalf("CheckSeal().Fault = %q; want it to name %q", got.Fault, tt.wantFault)
				}
				got.Fault = ""
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("CheckSeal() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("SealCommitsTheFileAloneWithAnExtraTrailer", func(t *testing.T) {
		t.Parallel()
		store, d := sealDeps(t, "Drift-Verify")
		writeFileString(t, d.Paths.Expected, expected)
		extra := fabricengine.Trailer{Key: BaselineTrailerKey, Value: "digest"}
		if err := SealExpectation(d, extra); err != nil {
			t.Fatalf("SealExpectation() = %v; want nil", err)
		}
		if len(store.commits) != 1 {
			t.Fatalf("SealExpectation() made %d commits; want 1", len(store.commits))
		}
		if _, found := store.commits[0].files[durableRel(expectedFileName)]; !found || len(store.commits[0].files) != 1 {
			t.Fatalf("SealExpectation() committed %d files; want the expectation alone", len(store.commits[0].files))
		}
		want := []fabricengine.Trailer{{Key: SealTrailerKey, Value: sealDigest([]byte(expected))}, extra}
		if diff := cmp.Diff(want, store.commits[0].trailers); diff != "" {
			t.Errorf("commit trailers mismatch (-want +got):\n%s", diff)
		}
	})

	baselineTests := []struct {
		name  string
		setup func(t *testing.T, d Deps)
		want  BaselineState
		fault bool
	}{
		{name: "IntactBaselineHolds", setup: func(t *testing.T, d Deps) { seedBaseline(t, d); commitBaselineDigest(t, d) }},
		{
			name: "EditedBaselineFileFaults",
			setup: func(t *testing.T, d Deps) {
				seedBaseline(t, d)
				commitBaselineDigest(t, d)
				writeFileString(t, filepath.Join(d.Paths.BaselineDir, "a.txt"), "edited\n")
			},
			fault: true,
		},
		{
			name: "MissingBaselineDirectoryFaults",
			setup: func(t *testing.T, d Deps) {
				seedBaseline(t, d)
				commitBaselineDigest(t, d)
				if err := os.RemoveAll(d.Paths.BaselineDir); err != nil {
					t.Fatalf("remove the baseline: %v", err)
				}
			},
			fault: true,
		},
		{name: "NoDigestCommitIsUnguardedWithNoFault", setup: seedBaseline, want: BaselineState{Unguarded: true}},
		{name: "MissingDirectoryWithNoDigestCommitFaults", setup: func(*testing.T, Deps) {}, want: BaselineState{Unguarded: true}, fault: true},
	}
	for _, tt := range baselineTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, d := sealDeps(t, "Drift-Verify")
			tt.setup(t, d)
			got, err := CheckBaseline(d)
			if err != nil {
				t.Fatalf("CheckBaseline() = %v; want nil", err)
			}
			if tt.fault {
				if !strings.HasPrefix(got.Fault, DeferHaltPrefix) || !strings.Contains(got.Fault, "lym drift defer") {
					t.Fatalf("CheckBaseline().Fault = %q; want it to open with %q and name lym drift defer", got.Fault, DeferHaltPrefix)
				}
				got.Fault = ""
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("CheckBaseline() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("PastRow", func(t *testing.T) {
		t.Parallel()
		rows := []string{"a", "b", "c"}
		tests := []struct {
			name         string
			current, row string
			want         bool
		}{
			{"CurrentAfterRow", "c", "b", true},
			{"CurrentIsRow", "b", "b", false},
			{"CurrentBeforeRow", "a", "b", false},
			{"RowAbsentIsNotPast", "c", "x", false},
			{"CurrentAbsentIsNotPast", "x", "a", false},
		}
		for _, tt := range tests {
			if got := PastRow(rows, tt.current, tt.row); got != tt.want {
				t.Errorf("%s: PastRow(%v, %q, %q) = %v; want %v", tt.name, rows, tt.current, tt.row, got, tt.want)
			}
		}
	})
}
