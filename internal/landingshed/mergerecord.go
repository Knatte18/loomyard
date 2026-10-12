// mergerecord.go — the merge record Merge-In writes and the merge review reads.
//
// The record and the reviewed-tip file live in the run's told scratch directory, so they stay under `.lyx` and are never committed.
// This package derives no path of its own beyond the two fixed file names joined onto that directory.

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

const (
	mergeRecordFile   = "merge-record.yaml"
	mergeReviewedFile = "merge-reviewed"
)

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
	// ReviewedTip is the parent tip the last finished merge review reached, the fork point before any.
	ReviewedTip string `yaml:"reviewed_tip"`
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

// MergeReviewedPath is the reviewed-tip file's path in scratchDir.
func MergeReviewedPath(scratchDir string) string {
	return filepath.Join(scratchDir, mergeReviewedFile)
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

// ReadReviewedTip reads the one-line reviewed tip at path.
// An absent file returns found == false with a nil error.
func ReadReviewedTip(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("landingshed: read reviewed tip: %w", err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

// RecordMergeReviewed copies the merge record's merged_tip into the reviewed-tip file atomically.
// A missing merge record writes nothing and returns nil, so the next range still starts at the earlier reviewed tip.
func RecordMergeReviewed(recordPath, reviewedPath string) error {
	rec, found, err := ReadMergeRecord(recordPath)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return writeBytesAtomic(reviewedPath, "reviewed tip", []byte(rec.MergedTip+"\n"))
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

// SkipsReview reports whether the merge needs no review: an up-to-date merge, or one whose overlap lists are all empty.
// Bound: it skips only when the git-computed sides share no file, no Go package and no PATTERN change.
func (r MergeRecord) SkipsReview() bool {
	return r.UpToDate || (len(r.Overlap.Files) == 0 && len(r.Overlap.Packages) == 0 && len(r.Overlap.Pattern) == 0)
}

// MergeReviewNote renders the merge review's subject as the markdown note the rubric's merge_review marker carries.
// A missing or unreadable record at recordPath renders a note telling the reviewer to review the whole merge-in diff of the latest merge of the parent into the task.
func MergeReviewNote(recordPath string) string {
	rec, found, err := ReadMergeRecord(recordPath)
	if err != nil || !found {
		return "No merge record is available.\n" +
			"Review the whole merge-in diff of the latest merge of the parent into the task.\n"
	}

	mergeCommit := rec.MergeCommit
	if mergeCommit == "" {
		mergeCommit = "none (the merge was up to date)"
	}
	var note strings.Builder
	fmt.Fprintf(&note, "Review how the task's code interacts with what `%s` gained in this merge.\n\n", rec.ParentBranch)
	fmt.Fprintf(&note, "- Parent range: `%s..%s`\n", rec.ReviewedTip, rec.MergedTip)
	fmt.Fprintf(&note, "- Task range: `%s..%s`\n", rec.TaskBase, rec.TaskHead)
	fmt.Fprintf(&note, "- Merge commit: %s\n", mergeCommit)
	fmt.Fprintf(&note, "- Overlapping files: %s\n", noteList(rec.Overlap.Files))
	fmt.Fprintf(&note, "- Overlapping packages: %s\n", noteList(rec.Overlap.Packages))
	fmt.Fprintf(&note, "- Changed pattern files: %s\n", noteList(rec.Overlap.Pattern))
	return note.String()
}

// noteList renders list as comma-separated code spans, or "none" when it is empty.
func noteList(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	quoted := make([]string, len(list))
	for i, item := range list {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, ", ")
}
