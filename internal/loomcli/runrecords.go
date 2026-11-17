// runrecords.go wires the implementer run records loom moves and names:
// the PR rework archive seam over both implementers' run directories,
// and the run-record path Describe reads from the active implementer.

package loomcli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// archiveRunRecords returns the rework archive seam's closure.
// It archives Webster's run directory into the slot it is handed.
// It archives Weaver's into the weaver slot beside it, so the next generation of either implementer starts fresh.
func archiveRunRecords(webster websterengine.Geometry, weaver weaverengine.Geometry) func(dest string) error {
	return func(dest string) error {
		if err := websterengine.ArchiveRunRecord(webster, dest); err != nil {
			return err
		}
		return weaverengine.ArchiveRunRecord(weaver, filepath.Join(filepath.Dir(dest), loomshed.ReworkPriorWeaver))
	}
}

// activeRunRecordPath returns the `summary.md` under the active implementer's run directory.
// A refusal to name the active implementer is returned, never read as Webster's.
func activeRunRecordPath(active weaverengine.ActiveDeps) (string, error) {
	implementer, _, err := weaverengine.Active(active)
	if err != nil {
		return "", err
	}
	runDir, _ := weaverengine.RunDirs(implementer, active.AnchorRoot)
	return summaryparser.Path(runDir), nil
}
