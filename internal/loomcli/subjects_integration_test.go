//go:build integration

// subjects_integration_test.go drives codeSubject against a real fabric pair and real git, since its identity is the task's diff against the merge base the pair's history leaves.

package loomcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestCodeSubject_Real pins what a code subject follows: commits of the run records,the run's scratch directory and the harness's own file leave it unchanged, an untracked task file changes it, and a location with no recorded parent branch answers not found.
func TestCodeSubject_Real(t *testing.T) {
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "codesubject"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	read := func(t *testing.T) shedadapters.Subject {
		t.Helper()
		subject, found, err := codeSubject(location, shedrun.SelfRunID)
		if err != nil || !found {
			t.Fatalf("codeSubject() = %+v, %v, %v; want a subject, found, nil", subject, found, err)
		}
		if subject.Kind != shedadapters.SubjectCode || subject.MergeBase == "" || subject.Head == "" || subject.FullHash == "" {
			t.Fatalf("codeSubject() = %+v; want a code subject carrying its merge base, head and full hash", subject)
		}
		return subject
	}
	before := read(t)

	// A status-like commit and a carry-over commit land on the records side, which no diff of the task's code holds.
	writeTestFile(t, loomengine.DiscussionDecisionRecord(location), "# Record\n\n## Decisions\n\nd.\n\n## Open risks\n\nnone yet.\n")
	if _, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, "discussion", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit discussion: %v", err)
	}
	entry := discussionparser.CarryOver{Segment: "Webster-Review", Round: 1, Closing: discussionparser.CarryOverConverged, ReviewPath: "r.md", FixerReportPath: "f.md"}
	if err := writeCarryOver(location, entry); err != nil {
		t.Fatalf("writeCarryOver() = %v; want nil", err)
	}
	if after := read(t); !sameCodeSubject(after, before) {
		t.Errorf("subject after commits of the run records = %+v; want the unchanged %+v", after, before)
	}

	// The harness's own file and a file under the run's scratch directory are exempt while untracked.
	writeTestFile(t, filepath.Join(location.WorktreePath(), ".scratch_task.json"), "{}\n")
	writeTestFile(t, filepath.Join(shedrun.ScratchDir(location, shedrun.SelfRunID), "note.txt"), "scratch\n")
	if after := read(t); !sameCodeSubject(after, before) {
		t.Errorf("subject after the harness file and a scratch file = %+v; want the unchanged %+v", after, before)
	}

	// An untracked file outside the exclusion set is part of the task's change.
	writeTestFile(t, filepath.Join(location.WorktreePath(), "task-notes.txt"), "a task file\n")
	if after := read(t); after.FullHash == before.FullHash {
		t.Errorf("subject after an untracked task file = %+v; want a changed full hash", after)
	}

	// The prime worktree records no parent branch, so its code segments write no records.
	prime, err := lyxcwd.ResolveWorktree(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree(prime) error = %v; want nil", err)
	}
	if subject, found, err := codeSubject(prime, shedrun.SelfRunID); err != nil || found {
		t.Errorf("codeSubject(prime) = %+v, %v, %v; want not found, nil", subject, found, err)
	}
}

// sameCodeSubject reports whether a and b carry the same merge base, head, full hash and patch-id.
func sameCodeSubject(a, b shedadapters.Subject) bool {
	return a.MergeBase == b.MergeBase && a.Head == b.Head && a.FullHash == b.FullHash && a.PatchID == b.PatchID
}
