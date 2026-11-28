// landed.go declares the landed-commit file: the commit a run's landing put on its parent branch.
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

// LandedCommitFile returns the path of the ephemeral file holding runID's landed commit, under ScratchDir(l, runID).
func LandedCommitFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "landed-commit")
}

// WriteLandedCommit records commit as the landed commit of runID, replacing any recorded before.
func WriteLandedCommit(l *lymcwd.Location, runID, commit string) error {
	path := LandedCommitFile(l, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(commit+"\n"), 0o644)
}

// ReadLandedCommit returns the commit recorded for runID's landing, empty when none is recorded.
func ReadLandedCommit(l *lymcwd.Location, runID string) (string, error) {
	data, err := os.ReadFile(LandedCommitFile(l, runID))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
