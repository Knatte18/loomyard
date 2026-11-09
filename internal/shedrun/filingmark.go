// filingmark.go declares the filing mark, the record a raddle filing leaves among a run's records.
// A filing writes it once it turned the run's own board entry into the project of the family it filed.

package shedrun

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// filingMarkFileName is the filename of the filing mark, whose path FilingMarkFile returns.
const filingMarkFileName = "filing-mark"

// FilingMarkFile returns the path of the durable filing mark of runID, under RunDir(l, runID).
func FilingMarkFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), filingMarkFileName)
}

// FilingMarkRel returns the worktree-anchor-relative form of FilingMarkFile's path, for a records commit's pathspec.
func FilingMarkRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), filingMarkFileName)
}

// WriteFilingMark records that runID's filing turned its board entry into the project of the family it filed.
// While the mark stands, the run's status writes leave that entry alone.
// Writing it again changes nothing.
func WriteFilingMark(l *lymcwd.Location, runID string) error {
	path := FilingMarkFile(l, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, nil, 0o644)
}

// FilingMarked reports whether runID's filing mark is on disk.
func FilingMarked(l *lymcwd.Location, runID string) (bool, error) {
	_, err := os.Stat(FilingMarkFile(l, runID))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
