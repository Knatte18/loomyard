// recordsguard_test.go — tier 0 tests for the records guard, over the in-memory commitStore.

package driftengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// setFull is a records change that records scope full and one stored ruling.
func setFull(scope *ScopeRecord, judged *JudgedRecord) error {
	scope.Scope = ScopeFull
	judged.Tuples = []JudgedTuple{{Case: "a.txt", Ruling: RulingMatches, Reason: "fits"}}
	return nil
}

// readFileString returns a file's content as a string, failing the test when it cannot be read.
func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// writeFileString writes content to path, creating its directory.
func writeFileString(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRecords covers the guard: a write's single commit and its digest, a read of the committed copies, a hand edit setting Mismatch, a restore, an unguarded read before the first commit, an extra trailer and an unknown key surviving a write.
func TestRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("WriteMakesOneCommitWhoseTrailerIsTheDigest", func(t *testing.T) {
		t.Parallel()
		store := newCommitStore(t)
		d := store.deps(NewPaths(store.root))
		if err := WriteRecords(d, setFull); err != nil {
			t.Fatalf("WriteRecords() = %v; want nil", err)
		}
		if len(store.commits) != 1 {
			t.Fatalf("WriteRecords() made %d commits; want 1", len(store.commits))
		}
		wantDigest := RecordsDigest([]byte(readFileString(t, d.Paths.Scope)), []byte(readFileString(t, d.Paths.Judged)), true, true)
		wantTrailers := []fabricengine.Trailer{{Key: RecordsTrailerKey, Value: wantDigest}}
		if diff := cmp.Diff(wantTrailers, store.commits[0].trailers); diff != "" {
			t.Errorf("commit trailers mismatch (-want +got):\n%s", diff)
		}
		for _, name := range []string{scopeFileName, judgedFileName} {
			if _, found := store.commits[0].files[durableRel(name)]; !found {
				t.Errorf("the commit lacks %s", name)
			}
		}

		records, err := ReadRecords(d)
		if err != nil {
			t.Fatalf("ReadRecords() = %v; want nil", err)
		}
		if !records.Committed || records.Mismatch || !records.ScopeFound || !records.JudgedFound {
			t.Errorf("ReadRecords() = %+v; want committed, found and no mismatch", records)
		}
		if records.Scope.Scope != ScopeFull || len(records.Judged.Tuples) != 1 {
			t.Errorf("ReadRecords() returned scope %q and %d tuples; want the written ones", records.Scope.Scope, len(records.Judged.Tuples))
		}
	})

	t.Run("HandEditSetsMismatchAndTheReadKeepsTheCommittedCopy", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			edit func(t *testing.T, d Deps)
		}{
			{"AmendmentAddedToScope", func(t *testing.T, d Deps) {
				err := UpdateScope(d.Paths, func(cur ScopeRecord, _ bool) (ScopeRecord, error) {
					cur.Amendments = append(cur.Amendments, Amendment{By: "hand", Kind: "discard"})
					return cur, nil
				})
				if err != nil {
					t.Fatalf("UpdateScope() = %v; want nil", err)
				}
			}},
			{"TupleAddedToJudged", func(t *testing.T, d Deps) {
				writeFileString(t, d.Paths.Judged, `{"tuples":[{"case":"a.txt"},{"case":"hand-added"}]}`)
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				store := newCommitStore(t)
				d := store.deps(NewPaths(store.root))
				if err := WriteRecords(d, setFull); err != nil {
					t.Fatalf("WriteRecords() = %v; want nil", err)
				}
				tt.edit(t, d)

				records, err := ReadRecords(d)
				if err != nil {
					t.Fatalf("ReadRecords() = %v; want nil", err)
				}
				if !records.Mismatch {
					t.Errorf("ReadRecords().Mismatch = false after a hand edit; want true")
				}
				if len(records.Scope.Amendments) != 0 || len(records.Judged.Tuples) != 1 || records.Judged.Tuples[0].Case != "a.txt" {
					t.Errorf("ReadRecords() returned %+v; want the committed copy", records)
				}

				err = WriteRecords(d, func(*ScopeRecord, *JudgedRecord) error { return nil })
				if err == nil || !strings.Contains(err.Error(), "--restore") {
					t.Errorf("WriteRecords() over a mismatch = %v; want a refusal naming --restore", err)
				}
				if len(store.commits) != 1 {
					t.Errorf("a refused write made %d commits in all; want the 1 from before", len(store.commits))
				}

				if err := RestoreRecords(d); err != nil {
					t.Fatalf("RestoreRecords() = %v; want nil", err)
				}
				if len(store.commits) != 1 {
					t.Errorf("RestoreRecords() changed the history to %d commits; want 1", len(store.commits))
				}
				if records, err := ReadRecords(d); err != nil || records.Mismatch {
					t.Errorf("ReadRecords() after RestoreRecords() = %+v, %v; want no mismatch", records, err)
				}
			})
		}
	})

	t.Run("RestoreRemovesAFileTheCommitLacks", func(t *testing.T) {
		t.Parallel()
		store := newCommitStore(t)
		d := store.deps(NewPaths(store.root))
		scope := `{"scope":"full"}`
		writeFileString(t, d.Paths.Scope, scope)
		digest := RecordsDigest([]byte(scope), nil, true, false)
		if err := d.CommitWithTrailers([]string{durableRel(scopeFileName)}, []fabricengine.Trailer{{Key: RecordsTrailerKey, Value: digest}}); err != nil {
			t.Fatalf("commit the scope alone: %v", err)
		}
		writeFileString(t, d.Paths.Judged, `{}`)
		writeFileString(t, d.Paths.Scope, `{"scope":"off"}`)
		if records, err := ReadRecords(d); err != nil || !records.Mismatch {
			t.Fatalf("ReadRecords() = %+v, %v; want a mismatch", records, err)
		}

		if err := RestoreRecords(d); err != nil {
			t.Fatalf("RestoreRecords() = %v; want nil", err)
		}
		if _, err := os.Stat(d.Paths.Judged); !os.IsNotExist(err) {
			t.Errorf("judged.json after RestoreRecords() = %v; want it removed", err)
		}
		if got := readFileString(t, d.Paths.Scope); got != scope {
			t.Errorf("scope.json after RestoreRecords() = %q; want %q", got, scope)
		}
	})

	t.Run("NoCommitReadsTheFilesAsTheyAreUnguarded", func(t *testing.T) {
		t.Parallel()
		store := newCommitStore(t)
		d := store.deps(NewPaths(store.root))
		writeFileString(t, d.Paths.Scope, `{"scope":"partial"}`)

		records, err := ReadRecords(d)
		if err != nil {
			t.Fatalf("ReadRecords() = %v; want nil", err)
		}
		if records.Committed || records.Mismatch || !records.ScopeFound || records.JudgedFound || records.Scope.Scope != ScopePartial {
			t.Errorf("ReadRecords() = %+v; want the file as it is, uncommitted, with no mismatch and no judged file", records)
		}
	})

	t.Run("ExtraTrailerRidesOnTheCommit", func(t *testing.T) {
		t.Parallel()
		store := newCommitStore(t)
		d := store.deps(NewPaths(store.root))
		extra := fabricengine.Trailer{Key: "Drift-Baseline", Value: "abc"}
		if err := WriteRecords(d, setFull, extra); err != nil {
			t.Fatalf("WriteRecords() = %v; want nil", err)
		}
		trailers := store.commits[0].trailers
		if len(trailers) != 2 || trailers[0].Key != RecordsTrailerKey || trailers[1] != extra {
			t.Errorf("commit trailers = %v; want %s then %v", trailers, RecordsTrailerKey, extra)
		}
	})

	t.Run("UnknownKeySurvivesAWrite", func(t *testing.T) {
		t.Parallel()
		store := newCommitStore(t)
		d := store.deps(NewPaths(store.root))
		writeFileString(t, d.Paths.Scope, `{"scope":"full","future_key":7}`)
		for round := range 2 {
			err := WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
				scope.AwaitedAt = "2026-01-01T00:00:00Z"
				return nil
			})
			if err != nil {
				t.Fatalf("WriteRecords() round %d = %v; want nil", round, err)
			}
			if got := readFileString(t, d.Paths.Scope); !strings.Contains(got, `"future_key": 7`) {
				t.Errorf("scope.json after write %d = %s; want future_key kept", round, got)
			}
		}
	})
}
