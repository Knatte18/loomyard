// landed.go declares the landed-commit file.
// It holds the commit a run's landing put on its parent branch and whether that landing's diff was empty.
// The landing writes it, and the dobby that drives the run reads it.

package shedrun

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// emptyDiffMark is the landed-commit file's second line when the landing's diff was empty.
const emptyDiffMark = "empty-diff"

// LandedCommit is a run's landing as its landing recorded it.
type LandedCommit struct {
	// Commit is the commit the landing put on the parent branch, empty when none is recorded.
	Commit string
	// EmptyDiff reports that the landing changed no code path on the parent branch, so none of the run's work reached it.
	EmptyDiff bool
}

// LandedCommitFile returns the path of the ephemeral file holding runID's landed commit, under ScratchDir(l, runID).
func LandedCommitFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "landed-commit")
}

// WriteLandedCommit records landed as the landing of runID, replacing any recorded before.
func WriteLandedCommit(l *lymcwd.Location, runID string, landed LandedCommit) error {
	path := LandedCommitFile(l, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := landed.Commit + "\n"
	if landed.EmptyDiff {
		content += emptyDiffMark + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// RecordLandedCommit records landed as the landing of runID.
// A landing already recorded for the same commit keeps its EmptyDiff,
// so a landing step run again over a parent that already holds the work does not reclassify the first landing.
func RecordLandedCommit(l *lymcwd.Location, runID string, landed LandedCommit) error {
	recorded, err := ReadLandedCommit(l, runID)
	if err != nil {
		return err
	}
	if recorded.Commit != "" && recorded.Commit == landed.Commit {
		landed.EmptyDiff = recorded.EmptyDiff
	}
	return WriteLandedCommit(l, runID, landed)
}

// ReadLandedCommit returns the landing recorded for runID, the zero LandedCommit when none is recorded.
// A file holding the commit alone reads as a landing whose diff was not empty.
func ReadLandedCommit(l *lymcwd.Location, runID string) (LandedCommit, error) {
	data, err := os.ReadFile(LandedCommitFile(l, runID))
	if errors.Is(err, fs.ErrNotExist) {
		return LandedCommit{}, nil
	}
	if err != nil {
		return LandedCommit{}, err
	}
	commit, rest, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	return LandedCommit{Commit: strings.TrimSpace(commit), EmptyDiff: strings.TrimSpace(rest) == emptyDiffMark}, nil
}
