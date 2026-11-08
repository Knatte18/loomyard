// remove_siblingdirty_integration_test.go pins Remove's sibling-dirty handling.
// An uncommitted run record in a pair's other worktree is committed and archived, not refused.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_UntrackedDriveReportIsCommittedAndArchived leaves a new file in an untracked drive-reports/ directory.
// The file is the uncommitted stop report the done-wait rests on.
// It asserts that a no-force Remove commits the file and the archive tag contains it;
// it raises no sibling-dirty refusal.
//
//testtiming:keep the untracked drive report carried into the archive tag on origin with its content
func TestRemove_UntrackedDriveReportIsCommittedAndArchived(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "sibling-dirty"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	reports := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), "_lym", "shed", slug, "drive-reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatalf("create %s: %v", reports, err)
	}
	if err := os.WriteFile(filepath.Join(reports, "stop.md"), []byte("stopped\n"), 0o644); err != nil {
		t.Fatalf("write stop report: %v", err)
	}

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(force=false) on a pair with an uncommitted drive report error = %v; want it committed and archived", err)
	}
	if res.ArchiveTag == "" {
		t.Fatalf("ArchiveTag is empty; want the tag covering the committed report")
	}
	rel := "_lym/shed/" + slug + "/drive-reports/stop.md"
	if got := showAtTag(t, h.RecordsBare, res.ArchiveTag, rel); got != "stopped\n" {
		t.Errorf("archived %s = %q; want the committed report", rel, got)
	}
}
