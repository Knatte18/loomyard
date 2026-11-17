// importrecord.go records the files the codex-file render imported from the repository's CLAUDE.md.
// A take-in of one of them re-renders AGENTS.override.md, which carries their text.

package heddleengine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// importRecordFile is the name of the import record, in the scratch directory's heddle directory.
	importRecordFile = "codex-file-imports.json"
	// importRecordLockSuffix ends the name of the lock file beside the import record.
	importRecordLockSuffix = ".lock"
)

// importRecord is the content of the import record.
// Paths are the slash-separated, worktree-root-relative paths of the imported files that lie inside the worktree, sorted.
type importRecord struct {
	Paths []string `json:"paths"`
}

// importRecordPath returns the path of the import record under scratchDir.
func importRecordPath(scratchDir string) string {
	return filepath.Join(scratchDir, lymdirs.HeddleDirName, importRecordFile)
}

// recordImports writes the import record of req's worktree from imports, the absolute paths the render imported.
// An import outside req.Root is left out: it raises no worktree event, so the record could not match it.
// The record is rewritten whole, so an empty imports list clears a dropped import.
func recordImports(req RenderRequest, imports []string) error {
	paths := []string{}
	for _, imported := range imports {
		rel, err := filepath.Rel(req.Root, imported)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		paths = append(paths, rel)
	}
	slices.Sort(paths)
	path := importRecordPath(req.ScratchDir)
	if err := state.WriteJSON(path, path+importRecordLockSuffix, importRecord{Paths: paths}); err != nil {
		return fmt.Errorf("the import record %s was not written, so an edit to an imported file re-renders nothing: %w", path, err)
	}
	return nil
}

// recordedImports returns the root-relative paths the last codex-file render under scratchDir imported.
// An absent record gives none.
// An unreadable one gives none with a Warn log naming the file and the render verb that rewrites it.
func recordedImports(scratchDir string) []string {
	path := importRecordPath(scratchDir)
	record, _, err := state.ReadJSON[importRecord](path, path+importRecordLockSuffix)
	if err != nil {
		logger.Warn("heddleengine: the import record is unreadable (non-fatal)", "file", path, "error", err, "way_forward", "run `"+renderVerb+"`, which rewrites it")
		return nil
	}
	return record.Paths
}
