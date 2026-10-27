// mergerecord.go — the merge record Parent-Align writes.
//
// The record lives in the run's told scratch directory, so it stays under `.lym` and is never committed.
// This package derives no path of its own beyond the fixed file name joined onto that directory.

package landingshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const mergeRecordFile = "merge-record.yaml"

// Overlap is where the parent's changes and the task's changes meet.
type Overlap struct {
	// Files are the paths changed on both sides, sorted.
	Files []string `yaml:"files,omitempty"`
	// Packages are the directories in which both sides changed a Go file, sorted.
	Packages []string `yaml:"packages,omitempty"`
	// Pattern are the parent-side paths that are PATTERN.md or under pattern/, sorted.
	Pattern []string `yaml:"pattern,omitempty"`
}

// MergeRecord is one merge of the parent into the task.
type MergeRecord struct {
	// ParentBranch is the branch merged in.
	ParentBranch string `yaml:"parent_branch"`
	// MergedTip is the parent tip the merge brought in.
	MergedTip string `yaml:"merged_tip"`
	// AlignedTip is the parent tip up to which the merge fixer has read what the parent brought in, empty before the first alignment.
	AlignedTip string `yaml:"aligned_tip"`
	// TaskBase is the parent tip last merged into the task before this merge, the fork point before any merge-in.
	TaskBase string `yaml:"task_base"`
	// TaskHead is the task head before this merge.
	TaskHead string `yaml:"task_head"`
	// MergeCommit is the merge commit, empty for an up-to-date merge.
	MergeCommit string `yaml:"merge_commit"`
	// UpToDate reports that the task already held the parent's tip.
	UpToDate bool `yaml:"up_to_date"`
	// Overlap is where the two sides' changes meet.
	Overlap Overlap `yaml:"overlap"`
}

// MergeRecordPath is the merge record's path in scratchDir.
func MergeRecordPath(scratchDir string) string {
	return filepath.Join(scratchDir, mergeRecordFile)
}

// ReadMergeRecord reads the record at path.
// An absent file returns found == false with a nil error; unreadable or undecodable content is an error.
func ReadMergeRecord(path string) (MergeRecord, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return MergeRecord{}, false, nil
	}
	if err != nil {
		return MergeRecord{}, false, fmt.Errorf("landingshed: read merge record: %w", err)
	}
	var rec MergeRecord
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return MergeRecord{}, false, fmt.Errorf("landingshed: decode merge record %s: %w", path, err)
	}
	return rec, true, nil
}

// WriteMergeRecord writes rec to path, creating the parent directory and replacing any earlier record atomically.
func WriteMergeRecord(path string, rec MergeRecord) error {
	data, err := yaml.Marshal(rec)
	if err != nil {
		return fmt.Errorf("landingshed: encode merge record: %w", err)
	}
	return writeBytesAtomic(path, "merge-record", data)
}

// writeBytesAtomic writes data to path through a temp file in the same directory and a rename, creating the parent directory;
// kind names the record in error messages.
func writeBytesAtomic(path, kind string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("landingshed: create %s directory: %w", kind, err)
	}
	tmp, err := os.CreateTemp(dir, "."+kind+"-*.tmp")
	if err != nil {
		return fmt.Errorf("landingshed: create %s temp file: %w", kind, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("landingshed: write %s: %w", kind, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("landingshed: close %s temp file: %w", kind, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("landingshed: replace %s: %w", kind, err)
	}
	return nil
}

// computeOverlap returns where the parent's changed paths and the task's changed paths meet.
func computeOverlap(parentPaths, taskPaths []string) Overlap {
	var overlap Overlap
	taskSet := make(map[string]bool, len(taskPaths))
	taskGoDirs := make(map[string]bool)
	for _, p := range taskPaths {
		taskSet[p] = true
		if strings.HasSuffix(p, ".go") {
			taskGoDirs[path.Dir(p)] = true
		}
	}
	for _, p := range parentPaths {
		if taskSet[p] {
			overlap.Files = append(overlap.Files, p)
		}
		if strings.HasSuffix(p, ".go") && taskGoDirs[path.Dir(p)] {
			overlap.Packages = append(overlap.Packages, path.Dir(p))
		}
		if p == "PATTERN.md" || strings.HasPrefix(p, "pattern/") {
			overlap.Pattern = append(overlap.Pattern, p)
		}
	}
	overlap.Files = sortedUnique(overlap.Files)
	overlap.Packages = sortedUnique(overlap.Packages)
	overlap.Pattern = sortedUnique(overlap.Pattern)
	return overlap
}

// sortedUnique sorts list and drops repeats, returning nil for an empty list.
func sortedUnique(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	slices.Sort(list)
	return slices.Compact(list)
}
