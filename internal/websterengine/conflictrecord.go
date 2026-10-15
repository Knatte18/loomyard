// conflictrecord.go declares the never-tracked plan-conflict record begin-batch leaves in webster's scratch directory, and its writers and removers.
package websterengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

const (
	// conflictRecordName is the file begin-batch writes when it refuses with a plan conflict.
	conflictRecordName = "plan-conflict.md"
	// routedConflictName is the file holding the findings of the conflict last routed to plan review.
	routedConflictName = "plan-conflict-routed.md"
)

// ConflictRecordRel returns the conflict record's forward-slashed path relative to the anchor, as recipe text names it.
func ConflictRecordRel() string {
	return path.Join(lyxdirs.DotLyxDirName, websterDirName, conflictRecordName)
}

// ConflictRecordPath returns the conflict record's path under the told scratchDir.
func ConflictRecordPath(scratchDir string) string {
	return filepath.Join(scratchDir, conflictRecordName)
}

// RoutedConflictPath returns the last-routed conflict's path under the told scratchDir.
func RoutedConflictPath(scratchDir string) string {
	return filepath.Join(scratchDir, routedConflictName)
}

// WriteConflictRecord writes the conflict record naming conflict's batch, cause and blocking findings.
// The scratch directory must already exist: every caller holds a lock file in it.
func WriteConflictRecord(scratchDir string, conflict *PlanConflictError) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan conflict at begin-batch %02d\ncause: %s\n\n", conflict.Batch, conflict.Cause)
	for _, finding := range conflict.Findings {
		b.WriteString("- " + finding + "\n")
	}
	if err := os.WriteFile(ConflictRecordPath(scratchDir), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: write plan-conflict record: %w", err)
	}
	return nil
}

// RemoveConflictRecord removes the conflict record, and is a no-op when it is absent.
func RemoveConflictRecord(scratchDir string) error {
	return removeIfPresent(ConflictRecordPath(scratchDir))
}

// RemoveRoutedConflict removes the last-routed conflict, and is a no-op when it is absent.
func RemoveRoutedConflict(scratchDir string) error {
	return removeIfPresent(RoutedConflictPath(scratchDir))
}

// removeIfPresent removes the file at filePath and returns nil when there is none.
func removeIfPresent(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("webster: remove %s: %w", filePath, err)
	}
	return nil
}
