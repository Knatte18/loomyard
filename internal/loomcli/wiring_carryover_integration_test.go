//go:build integration

// wiring_carryover_integration_test.go drives writeCarryOver over a real fabric pair,
// so the entry lands in the decision record and the record is committed alone.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestWriteCarryOver_Real_CommitsTheRecordAlone asserts writeCarryOver puts the segment's entry into the decision record and commits that record and nothing else.
// Another dirty file of the discussion directory stays uncommitted, and writing the same entry again commits nothing.
func TestWriteCarryOver_Real_CommitsTheRecordAlone(t *testing.T) {
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "carryoverrecord"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	recordsSibling := hub.PairRecordsSibling(slug)

	recordPath := loomengine.DiscussionDecisionRecord(location)
	supportLogPath := loomengine.DiscussionSupportLog(location)
	writeTestFile(t, recordPath, "# Record\n\n## Goal\n\ng.\n\n## Decisions\n\nd.\n\n## Open risks\n\nnone yet.\n\n## Acceptance criteria\n\na.\n")
	writeTestFile(t, supportLogPath, "log\n")
	if _, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, "discussion", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit discussion: %v", err)
	}
	writeTestFile(t, supportLogPath, "log, edited and not yet committed\n")

	entry := discussionparser.CarryOver{
		Segment:         "Plan-Review",
		Round:           2,
		Closing:         discussionparser.CarryOverConverged,
		ReviewPath:      "reviews/plan/round-2-review.md",
		FixerReportPath: "reviews/plan/round-2-fixer-report.md",
		Findings:        []discussionparser.CarryOverFinding{{Key: "cache-key", Class: "design", Severity: "MEDIUM", Rounds: []int{1, 2}}},
	}
	before := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD")
	if err := writeCarryOver(location, entry); err != nil {
		t.Fatalf("writeCarryOver() = %v; want nil", err)
	}

	recordBytes, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read decision record: %v", err)
	}
	record := string(recordBytes)
	for _, want := range []string{"<!-- lyx:carry-over Plan-Review -->", "cache-key (class design, severity MEDIUM, open in rounds 1, 2)", "<!-- /lyx:carry-over Plan-Review -->"} {
		if !strings.Contains(record, want) {
			t.Errorf("decision record does not contain %q:\n%s", want, record)
		}
	}
	after := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD")
	if want := atoiOrFail(t, before) + 1; atoiOrFail(t, after) != want {
		t.Fatalf("records commit count %s -> %s; want exactly one commit", strings.TrimSpace(before), strings.TrimSpace(after))
	}
	if subject := strings.TrimSpace(gitkit.Git(t, recordsSibling, "log", "-1", "--format=%s")); !strings.HasPrefix(subject, "loom: review carry-over for ") {
		t.Errorf("commit subject = %q; want the loom: review carry-over message", subject)
	}
	changed := strings.Fields(gitkit.Git(t, recordsSibling, "show", "--no-renames", "--name-only", "--format=", "HEAD"))
	if len(changed) != 1 || filepath.Base(changed[0]) != filepath.Base(loomengine.DiscussionDecisionRecordRel()) {
		t.Errorf("commit changed %v; want the decision record alone", changed)
	}
	if status := gitkit.Git(t, recordsSibling, "status", "--porcelain"); !strings.Contains(status, "support-log.md") || strings.Contains(status, "decision-record.md") {
		t.Errorf("git status = %q; want only the support log left dirty", status)
	}

	if err := writeCarryOver(location, entry); err != nil {
		t.Fatalf("second writeCarryOver() = %v; want nil", err)
	}
	if again := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD"); atoiOrFail(t, again) != atoiOrFail(t, after) {
		t.Errorf("records commit count after an unchanged write = %s; want %s", strings.TrimSpace(again), strings.TrimSpace(after))
	}
}
