//go:build tier1

// subjects_integration_test.go drives codeSubject and taskMergeBase against a real fabric pair and real git, since their identity is the task's diff against the merge base the pair's history leaves.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestCodeSubject_Real pins what a code subject follows: commits of the run records,the run's scratch directory and the harness's own file leave it unchanged, an untracked task file changes it, and a location with no recorded parent branch answers not found.
func TestCodeSubject_Real(t *testing.T) {
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "codesubject"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
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

	// A parent tip the task merged but the local parent branch has not reached leaves the task's diff only once the merge record names it.
	forkPoint, mergedTip, parentFile := mergeUnreachedParentTip(t, hub, slug)
	if after := read(t); after.MergeBase != forkPoint || !bytes.Contains(after.Patch, []byte(parentFile)) {
		t.Errorf("subject without a merge record: merge base %q, patch holds the parent file = %v; want the local parent branch's merge base %q and the parent file in the patch", after.MergeBase, bytes.Contains(after.Patch, []byte(parentFile)), forkPoint)
	}
	writeMergeRecordNaming(t, location, mergedTip)
	if after := read(t); after.MergeBase != mergedTip || bytes.Contains(after.Patch, []byte(parentFile)) {
		t.Errorf("subject with a merge record: merge base %q, patch holds the parent file = %v; want the merged tip %q and no parent file in the patch", after.MergeBase, bytes.Contains(after.Patch, []byte(parentFile)), mergedTip)
	}

	// The prime worktree records no parent branch, so its code segments write no records.
	prime, err := lymcwd.ResolveWorktree(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree(prime) error = %v; want nil", err)
	}
	if subject, found, err := codeSubject(prime, shedrun.SelfRunID); err != nil || found {
		t.Errorf("codeSubject(prime) = %+v, %v, %v; want not found, nil", subject, found, err)
	}
}

// mergeUnreachedParentTip commits a file on the hub's parent branch, merges that commit into the slug pair's task branch, adds a task commit, and then rewinds the parent branch to the task's fork point.
// It returns the fork point, the merged tip and the file the parent brought, so the local parent branch lags the tip the task merged.
func mergeUnreachedParentTip(t *testing.T, hub *hubforge.Hub, slug string) (forkPoint, mergedTip, parentFile string) {
	t.Helper()
	worktree := hub.PairCodeWorktree(slug)
	prime := hub.PrimeWorktree()
	parentFile = "parent-progress.txt"
	forkPoint = gitkit.RevParse(t, worktree, "HEAD")
	mergedTip = gitkit.CommitFile(t, prime, parentFile, "parent progress\n", "parent: progress")
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "merge the parent tip", mergedTip)
	gitkit.CommitFile(t, worktree, "task-after-merge.txt", "task work\n", "task: work after the merge")
	gitkit.Git(t, prime, "reset", "--hard", forkPoint)
	return forkPoint, mergedTip, parentFile
}

// writeMergeRecordNaming writes the merge record of the run at location with mergedTip as the parent tip the task merged.
func writeMergeRecordNaming(t *testing.T, location *lymcwd.Location, mergedTip string) {
	t.Helper()
	recordPath := landingshed.MergeRecordPath(loomengine.LoomScratchDir(location))
	if err := landingshed.WriteMergeRecord(recordPath, landingshed.MergeRecord{MergedTip: mergedTip}); err != nil {
		t.Fatalf("WriteMergeRecord() = %v; want nil", err)
	}
}

// TestTaskMergeBase_FollowsTheMergedTip pins the base of the task's diff: the merge base with the parent tip the merge record names while the local parent branch lags it, and the merge base with the local parent branch when the record is absent or undecodable.
func TestTaskMergeBase_FollowsTheMergedTip(t *testing.T) {
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "taskmergebase"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	forkPoint, mergedTip, _ := mergeUnreachedParentTip(t, hub, slug)
	mergeBase := taskMergeBase(location)

	tests := []struct {
		name   string
		record func(t *testing.T)
		want   string
	}{
		{"no merge record", func(t *testing.T) {}, forkPoint},
		{"undecodable merge record", func(t *testing.T) {
			recordPath := landingshed.MergeRecordPath(loomengine.LoomScratchDir(location))
			writeTestFile(t, recordPath, "merged_tip: [unclosed\n")
		}, forkPoint},
		{"merge record naming the merged tip", func(t *testing.T) {
			if err := os.Remove(landingshed.MergeRecordPath(loomengine.LoomScratchDir(location))); err != nil {
				t.Fatalf("remove the undecodable record: %v", err)
			}
			writeMergeRecordNaming(t, location, mergedTip)
		}, mergedTip},
	}
	// The rows share one pair and run in order: each row replaces the record the row before left.
	for _, tt := range tests {
		tt.record(t)
		if got, err := mergeBase(); err != nil || got != tt.want {
			t.Errorf("taskMergeBase() with %s = %q, %v; want %q, nil", tt.name, got, err, tt.want)
		}
	}
}

// sameCodeSubject reports whether a and b carry the same merge base, head, full hash and patch-id.
func sameCodeSubject(a, b shedadapters.Subject) bool {
	return a.MergeBase == b.MergeBase && a.Head == b.Head && a.FullHash == b.FullHash && a.PatchID == b.PatchID
}
