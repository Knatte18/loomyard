//go:build tier0

// mergerecord_test.go covers the merge record codec and the overlap computation.

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
		AlignedTip:   "bbb222",
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

	rawPath := filepath.Join(dir, "scratch", "raw", "merge-record.yaml")
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, []byte("parent_branch: main\nmerged_tip: aaa111\nreviewed_tip: bbb222\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old, found, err := ReadMergeRecord(rawPath)
	if err != nil || !found || old.MergedTip != "aaa111" || old.AlignedTip != "" {
		t.Fatalf("ReadMergeRecord(old reviewed_tip record) = %+v, %v, %v; want merged_tip kept and an empty aligned tip", old, found, err)
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
