// mergerecord_test.go covers the merge record codec, the overlap computation and the reviewed-tip hand-over.

package landingshed

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func sampleMergeRecord() MergeRecord {
	return MergeRecord{
		ParentBranch: "main",
		MergedTip:    "aaa111",
		ReviewedTip:  "bbb222",
		TaskBase:     "ccc333",
		TaskHead:     "ddd444",
		MergeCommit:  "eee555",
		Overlap: Overlap{
			Files:    []string{"internal/x/a.go"},
			Packages: []string{"internal/x"},
			Pattern:  []string{"PATTERN.md"},
		},
	}
}

func TestMergeRecord_RoundTripAndUndecodable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recordPath := MergeRecordPath(filepath.Join(dir, "scratch"))
	if got := filepath.Dir(recordPath); got != filepath.Join(dir, "scratch") {
		t.Fatalf("MergeRecordPath dir = %q; want the told scratch directory", got)
	}

	if _, found, err := ReadMergeRecord(recordPath); found || err != nil {
		t.Fatalf("ReadMergeRecord(absent) = found %v, err %v; want not found, nil", found, err)
	}

	want := sampleMergeRecord()
	if err := WriteMergeRecord(recordPath, want); err != nil {
		t.Fatalf("WriteMergeRecord() error = %v", err)
	}
	got, found, err := ReadMergeRecord(recordPath)
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadMergeRecord() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}

	if err := os.WriteFile(recordPath, []byte("parent_branch: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ReadMergeRecord(recordPath); found || err == nil {
		t.Fatalf("ReadMergeRecord(undecodable) = found %v, err %v; want not found and an error", found, err)
	}
}

func TestComputeOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		parent, task []string
		want         Overlap
	}{
		{
			name:   "a shared file is a file overlap and, being Go, a package overlap",
			parent: []string{"internal/x/a.go", "docs/only-parent.md"},
			task:   []string{"internal/x/a.go", "docs/only-task.md"},
			want:   Overlap{Files: []string{"internal/x/a.go"}, Packages: []string{"internal/x"}},
		},
		{
			name:   "different Go files in one directory overlap by package only",
			parent: []string{"internal/x/b.go", "internal/x/c.go"},
			task:   []string{"internal/x/a.go"},
			want:   Overlap{Packages: []string{"internal/x"}},
		},
		{
			name:   "a parent change to PATTERN.md is a pattern overlap",
			parent: []string{"PATTERN.md"},
			task:   []string{"internal/y/a.go"},
			want:   Overlap{Pattern: []string{"PATTERN.md"}},
		},
		{
			name:   "a parent change under pattern/ is a pattern overlap",
			parent: []string{"pattern/PATTERN-x.md", "patterns/other.md"},
			task:   nil,
			want:   Overlap{Pattern: []string{"pattern/PATTERN-x.md"}},
		},
		{
			name:   "disjoint sides overlap nowhere",
			parent: []string{"a/one.go"},
			task:   []string{"b/two.go"},
			want:   Overlap{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := computeOverlap(tt.parent, tt.task); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("computeOverlap(%v, %v) = %+v; want %+v", tt.parent, tt.task, got, tt.want)
			}
		})
	}
}

func TestRecordMergeReviewed(t *testing.T) {
	t.Parallel()

	t.Run("copies the merged tip into the reviewed file", func(t *testing.T) {
		t.Parallel()
		scratch := t.TempDir()
		if err := WriteMergeRecord(MergeRecordPath(scratch), sampleMergeRecord()); err != nil {
			t.Fatal(err)
		}
		if err := RecordMergeReviewed(MergeRecordPath(scratch), MergeReviewedPath(scratch)); err != nil {
			t.Fatalf("RecordMergeReviewed() error = %v", err)
		}
		got, found, err := ReadReviewedTip(MergeReviewedPath(scratch))
		if err != nil || !found || got != "aaa111" {
			t.Errorf("ReadReviewedTip() = %q, %v, %v; want aaa111, true, nil", got, found, err)
		}
	})

	t.Run("a missing merge record writes nothing and keeps the earlier tip", func(t *testing.T) {
		t.Parallel()
		scratch := t.TempDir()
		reviewedPath := MergeReviewedPath(scratch)
		if err := os.WriteFile(reviewedPath, []byte("earlier\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RecordMergeReviewed(MergeRecordPath(scratch), reviewedPath); err != nil {
			t.Fatalf("RecordMergeReviewed() error = %v; want nil", err)
		}
		if got, _, err := ReadReviewedTip(reviewedPath); err != nil || got != "earlier" {
			t.Errorf("ReadReviewedTip() = %q, %v; want the earlier tip kept", got, err)
		}
		if _, found, err := ReadReviewedTip(filepath.Join(scratch, "none")); found || err != nil {
			t.Errorf("ReadReviewedTip(absent) = found %v, err %v; want not found, nil", found, err)
		}
	})
}
