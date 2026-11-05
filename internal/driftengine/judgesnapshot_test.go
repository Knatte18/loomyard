// judgesnapshot_test.go — tier 0 tests for the judge snapshot, over fake checkouts that are temporary directories.

package driftengine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The records paths loom writes during a row, relative to the records checkout's root.
const (
	testRunDirRel      = "_lym/shed/run1"
	testStatusRel      = testRunDirRel + "/status.json"
	testFrictionRel    = "_lym/loom/friction"
	testFrictionPrefix = testFrictionRel + "-"
	testReviewRel      = "_lym/loom/reviews/review-1.md"
	testJudgementRel   = "_lym/drift/judgement.yaml"
)

// judgeFixture holds two fake checkouts, a scratch directory and the records history a test changes between the snapshot and the comparison.
type judgeFixture struct {
	code, records fabricengine.CheckoutState
	// recordsChanged is what RecordsChangedSince answers.
	recordsChanged []string
	// sinceRevs records each revision RecordsChangedSince was asked about.
	sinceRevs []string
	scratch   string
	deps      Deps
}

func newJudgeFixture(t *testing.T) *judgeFixture {
	t.Helper()
	f := &judgeFixture{
		code:    fabricengine.CheckoutState{Root: t.TempDir(), Head: "code-1"},
		records: fabricengine.CheckoutState{Root: t.TempDir(), Head: "records-1"},
		scratch: t.TempDir(),
	}
	f.deps = Deps{
		Paths:            NewPaths(f.code.Root),
		WorktreeRoot:     f.code.Root,
		JudgeSnapshotDir: filepath.Join(t.TempDir(), "snapshot"),
		LoomRecordsDirs:  []string{testRunDirRel, testFrictionRel, testFrictionPrefix},
		Checkouts: func() (fabricengine.CheckoutState, fabricengine.CheckoutState, error) {
			return f.code, f.records, nil
		},
		RecordsChangedSince: func(rev string) ([]string, error) {
			f.sinceRevs = append(f.sinceRevs, rev)
			return f.recordsChanged, nil
		},
	}
	return f
}

// writeTestFile writes content to rel under root, creating its directory.
func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// removeTestFile removes rel under root.
func removeTestFile(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

// TestJudgeSnapshot covers what the snapshot names as the judge's change and what it leaves out, and the snapshot's record on disk.
func TestJudgeSnapshot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		before func(t *testing.T, f *judgeFixture)
		after  func(t *testing.T, f *judgeFixture)
		want   []string
	}{
		{
			"ModifiedTrackedFile",
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.code.Root, "a.go", "one")
				f.code.Changed = []string{"a.go"}
			},
			func(t *testing.T, f *judgeFixture) { writeTestFile(t, f.code.Root, "a.go", "two") },
			[]string{"code: a.go was changed"},
		},
		{
			"ModifiedUntrackedFile",
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.code.Root, "u.txt", "one")
				f.code.Untracked = []string{"u.txt"}
			},
			func(t *testing.T, f *judgeFixture) { writeTestFile(t, f.code.Root, "u.txt", "two") },
			[]string{"code: u.txt was changed"},
		},
		{
			"DeletedUntrackedFile",
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.code.Root, "u.txt", "one")
				f.code.Untracked = []string{"u.txt"}
			},
			func(t *testing.T, f *judgeFixture) { removeTestFile(t, f.code.Root, "u.txt") },
			[]string{"code: u.txt was deleted"},
		},
		{
			"AddedUntrackedFile",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.code.Root, "new.txt", "one")
				f.code.Untracked = []string{"new.txt"}
			},
			[]string{"code: new.txt was added"},
		},
		{
			"MovedCodeHead",
			func(*testing.T, *judgeFixture) {},
			func(_ *testing.T, f *judgeFixture) { f.code.Head = "code-2" },
			[]string{`code: HEAD moved from "code-1" to "code-2"`},
		},
		{
			"RecordsCommitOfAnotherPath",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, "notes.md", "one")
				f.records.Head, f.recordsChanged = "records-2", []string{"notes.md"}
			},
			[]string{"records: notes.md was changed"},
		},
		{
			"ChangedScratchFile",
			func(t *testing.T, f *judgeFixture) { writeTestFile(t, f.scratch, "case.txt", "one") },
			func(t *testing.T, f *judgeFixture) { writeTestFile(t, f.scratch, "case.txt", "two") },
			[]string{"scratch: case.txt was changed"},
		},
		{
			"ChangeBeforeTheSnapshotIsNotNamed",
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.code.Root, "a.go", "one")
				f.code.Changed = []string{"a.go"}
			},
			func(*testing.T, *judgeFixture) {},
			nil,
		},
		{
			"JudgementFileIsExempt",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testJudgementRel, "spawn_id: s")
				f.records.Untracked = []string{testJudgementRel}
			},
			nil,
		},
		{
			"StatusCommitIsNoChange",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testStatusRel, "{}")
				f.records.Head, f.recordsChanged = "records-2", []string{testStatusRel}
			},
			nil,
		},
		{
			"FrictionNoteCommittedIsNoChange",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testFrictionRel+"/halt.md", "halt")
				f.records.Head, f.recordsChanged = "records-2", []string{testFrictionRel + "/halt.md"}
			},
			nil,
		},
		{
			"FrictionNoteUntrackedIsNoChange",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testFrictionRel+"/halt.md", "halt")
				f.records.Untracked = []string{testFrictionRel + "/halt.md"}
			},
			nil,
		},
		{
			"FrictionArchiveSiblingIsNoChange",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testFrictionPrefix+"20260101/halt.md", "halt")
				f.records.Untracked = []string{testFrictionPrefix + "20260101/halt.md"}
			},
			nil,
		},
		{
			"CommitAlsoChangingAnotherPathNamesIt",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testStatusRel, "{}")
				writeTestFile(t, f.records.Root, "src/x.go", "one")
				f.records.Head, f.recordsChanged = "records-2", []string{testStatusRel, "src/x.go"}
			},
			[]string{"records: src/x.go was changed"},
		},
		{
			"UntrackedFileCommittedWithTheSameContentIsNoChange",
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testReviewRel, "review")
				f.records.Untracked = []string{testReviewRel}
			},
			func(_ *testing.T, f *judgeFixture) {
				f.records.Head, f.records.Untracked, f.recordsChanged = "records-2", nil, []string{testReviewRel}
			},
			nil,
		},
		{
			"UntrackedFileCommittedWithOtherContentIsNamed",
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testReviewRel, "review")
				f.records.Untracked = []string{testReviewRel}
			},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, testReviewRel, "edited")
				f.records.Head, f.records.Untracked, f.recordsChanged = "records-2", nil, []string{testReviewRel}
			},
			[]string{"records: " + testReviewRel + " was changed"},
		},
		{
			"NewCommittedPathBesideTheReviewsIsNamed",
			func(*testing.T, *judgeFixture) {},
			func(t *testing.T, f *judgeFixture) {
				writeTestFile(t, f.records.Root, "_lym/loom/reviews/review-2.md", "new")
				f.records.Head, f.recordsChanged = "records-2", []string{"_lym/loom/reviews/review-2.md"}
			},
			[]string{"records: _lym/loom/reviews/review-2.md was changed"},
		},
		{
			"DeletionCommittedAfterwardsIsNoChange",
			func(_ *testing.T, f *judgeFixture) { f.records.Changed = []string{testReviewRel} },
			func(_ *testing.T, f *judgeFixture) {
				f.records.Head, f.records.Changed, f.recordsChanged = "records-2", nil, []string{testReviewRel}
			},
			nil,
		},
		{
			"DeletedScratchDirectoryIsLeftOut",
			func(t *testing.T, f *judgeFixture) { writeTestFile(t, f.scratch, "case.txt", "one") },
			func(t *testing.T, f *judgeFixture) {
				if err := os.RemoveAll(f.scratch); err != nil {
					t.Fatal(err)
				}
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newJudgeFixture(t)
			tt.before(t, f)
			if _, err := TakeJudgeSnapshot(f.deps, f.scratch); err != nil {
				t.Fatalf("TakeJudgeSnapshot() = _, %v; want nil error", err)
			}
			snapshot, found, err := ReadJudgeSnapshot(f.deps)
			if err != nil || !found {
				t.Fatalf("ReadJudgeSnapshot() = _, %v, %v; want the snapshot", found, err)
			}
			tt.after(t, f)

			got, err := snapshot.Changes(f.deps)
			if err != nil {
				t.Fatalf("Changes() = _, %v; want nil error", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Changes() = %q; want %q", got, tt.want)
			}
			for _, rev := range f.sinceRevs {
				if rev != "records-1" {
					t.Errorf("RecordsChangedSince asked about %q; want the snapshot's records head records-1", rev)
				}
			}
		})
	}

	t.Run("the record keeps a copy of each untracked file and a gone snapshot reads found false", func(t *testing.T) {
		t.Parallel()
		f := newJudgeFixture(t)
		if _, found, err := ReadJudgeSnapshot(f.deps); err != nil || found {
			t.Fatalf("ReadJudgeSnapshot() before any snapshot = _, %v, %v; want found false", found, err)
		}

		writeTestFile(t, f.code.Root, "u.txt", "kept")
		f.code.Untracked = []string{"u.txt"}
		if _, err := TakeJudgeSnapshot(f.deps, f.scratch); err != nil {
			t.Fatalf("TakeJudgeSnapshot() = _, %v; want nil error", err)
		}
		copied, err := os.ReadFile(filepath.Join(f.deps.JudgeSnapshotDir, "untracked", "code", "u.txt"))
		if err != nil || string(copied) != "kept" {
			t.Errorf("the untracked copy = %q, %v; want %q", copied, err, "kept")
		}
		snapshot, found, err := ReadJudgeSnapshot(f.deps)
		if err != nil || !found || snapshot.ScratchDir != f.scratch || snapshot.Code.Head != "code-1" {
			t.Errorf("ReadJudgeSnapshot() = %+v, %v, %v; want the snapshot just taken", snapshot, found, err)
		}

		if err := os.RemoveAll(f.deps.JudgeSnapshotDir); err != nil {
			t.Fatal(err)
		}
		if _, found, err := ReadJudgeSnapshot(f.deps); err != nil || found {
			t.Errorf("ReadJudgeSnapshot() after the directory is gone = _, %v, %v; want found false", found, err)
		}
	})
}
