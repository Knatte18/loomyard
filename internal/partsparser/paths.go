// paths.go declares the parts directory's name and the paths under it.

package partsparser

import (
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// DirName is the name of the parts directory, which sits under the worktree's `_lym` directory.
const DirName = "parts"

// ArchivePrefix starts the name of every archive directory under the parts directory.
const ArchivePrefix = "archive-"

const (
	fileName         = "parts.md"
	overviewFileName = "00-overview.md"
	joinRecordName   = "join.md"
)

// Dir returns the parts directory of the worktree at anchorPath.
func Dir(anchorPath string) string {
	return filepath.Join(anchorPath, lymdirs.LymDirName, DirName)
}

// DirRel returns the worktree-relative parts directory token, `_lym/parts`, always forward-slash.
func DirRel() string {
	return path.Join(lymdirs.LymDirName, DirName)
}

// File returns the parts file's path.
func File(anchorPath string) string {
	return filepath.Join(Dir(anchorPath), fileName)
}

// PartDir returns the directory holding the plan of the part named part.
func PartDir(anchorPath, part string) string {
	return filepath.Join(Dir(anchorPath), part)
}

// PartOverview returns the overview file of the part named part's plan.
func PartOverview(anchorPath, part string) string {
	return filepath.Join(PartDir(anchorPath, part), overviewFileName)
}

// JoinRecord returns the path of the record the chair's join writes.
func JoinRecord(anchorPath string) string {
	return filepath.Join(Dir(anchorPath), joinRecordName)
}
