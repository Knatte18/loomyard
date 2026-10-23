// overlapnote.go implements the overlap note a darn Finalize writes when a large merge reroutes the run, which the re-entered review perch reads as its review input.

package landingshed

import (
	"fmt"
	"path/filepath"
	"strings"
)

const overlapNoteFile = "overlap-note.md"

// OverlapNotePath is the overlap note's path in scratchDir.
func OverlapNotePath(scratchDir string) string {
	return filepath.Join(scratchDir, overlapNoteFile)
}

// WriteOverlapNote writes the note naming the parent's changes to the overlapping files at path, creating its directory and replacing an earlier note atomically.
// rec.ParentBranch is the branch the changes are on and rec.MergedTip the parent tip they are read since.
// files are the paths both sides changed, and the note gives the command that reads the parent's changes to them.
func WriteOverlapNote(path string, rec MergeRecord, files []string) error {
	var note strings.Builder
	fmt.Fprintf(&note, "# Parent changes to review\n\nThe merge of parent branch %q was large, so the run went back to review.\n", rec.ParentBranch)
	if len(files) == 0 {
		fmt.Fprintf(&note, "The merge needed the conflict session; the parent's changes since %s are the ones to review.\n", rec.MergedTip)
		return writeBytesAtomic(path, "overlap-note", []byte(note.String()))
	}
	fmt.Fprintf(&note, "The parent changed these files the task changed too, since its tip %s:\n\n", rec.MergedTip)
	for _, file := range files {
		fmt.Fprintf(&note, "- %s\n", file)
	}
	fmt.Fprintf(&note, "\nRead the parent's changes to them with `git diff %s %s -- %s`.\n", rec.MergedTip, rec.ParentBranch, strings.Join(files, " "))
	return writeBytesAtomic(path, "overlap-note", []byte(note.String()))
}
