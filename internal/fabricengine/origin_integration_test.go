// origin_integration_test.go covers the origin record that Add writes, end-to-end.
// It checks the record's anchor-relative placement and its adoption on an existing weft branch.
// It also checks the mutation-record entries.
//
// It is package fabricengine_test so it can import hubforge, which an in-package test cannot import.
// It shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAdd_RecordsParentBranch_SubpathAnchoredHub proves the record lands at the anchor-relative path
// inside the new pair's weft worktree, and not at the weft worktree root.
// A "." anchor would put both paths at the root, so only a subdirectory anchor tells them apart.
//
//testtiming:keep the origin record landing at the anchor-relative path and not at the weft root, with parent_branch main; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_RecordsParentBranch_SubpathAnchoredHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend"})
	l := h.Location
	const slug = "subpath-parent"

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	wantPath := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, fabricengine.OriginRecordRel())
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("origin record missing at the anchor-relative path %s: %v", wantPath, err)
	}

	rootPath := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), fabricengine.OriginRecordRel())
	if _, err := os.Stat(rootPath); err == nil {
		t.Errorf("origin record also landed at the weft worktree root %s; want it only at the anchor-relative path", rootPath)
	}

	pairLayout, err := lymcwd.Resolve(filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(pair warp worktree): %v", err)
	}
	origin, ok, err := fabricengine.ReadOrigin(pairLayout)
	if err != nil {
		t.Fatalf("ReadOrigin() error = %v", err)
	}
	if !ok {
		t.Fatalf("ReadOrigin() ok = false; want a record written by Add")
	}
	if origin.ParentBranch != "main" {
		t.Errorf("ReadOrigin().ParentBranch = %q; want %q", origin.ParentBranch, "main")
	}
}

// TestAdd_AdoptedWeftKeepsItsOriginRecord adopts a live pair's weft branch from a worktree on another branch.
// The branch already carries the origin record, which keeps its recorded parent and gets no new record commit,
// and the new weft worktree still gets its .weft lock directory.
//
//testtiming:keep no file_written entry for the origin record and no commit_created entry on an adopted weft branch; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_AdoptedWeftKeepsItsOriginRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "moved-pair"
	weftBranch := fabricengine.RecordsBranchName(slug)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	recordedTip := gitkit.RevParse(t, mustRecordsRepoRoot(t, l), "refs/remotes/origin/"+weftBranch)
	if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "branch", weftBranch, recordedTip)
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "another-parent")

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	pairLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(pair warp worktree): %v", err)
	}
	origin, ok, err := fabricengine.ReadOrigin(pairLayout)
	if err != nil || !ok {
		t.Fatalf("ReadOrigin() = (ok %v, err %v); want the adopted branch's own record", ok, err)
	}
	if origin.ParentBranch != "main" {
		t.Errorf("ReadOrigin().ParentBranch = %q; want the recorded %q, not the acting worktree's branch", origin.ParentBranch, "main")
	}
	if got := gitkit.RevParse(t, fabricengine.RecordsWorktreePath(l, slug), "HEAD"); got != recordedTip {
		t.Errorf("adopted weft HEAD = %s; want the recorded tip %s (no record commit)", got, recordedTip)
	}
	if _, err := os.Stat(filepath.Join(fabricengine.RecordsWorktreePath(l, slug), ".weft")); err != nil {
		t.Errorf("adopted weft worktree lacks its .weft lock directory: %v", err)
	}
	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	for _, m := range res.Mutations.Entries() {
		if (m.Kind == fabricengine.KindFileWritten && strings.Contains(recordPath, m.Target)) || m.Kind == fabricengine.KindCommitCreated {
			t.Errorf("record carries %s at %s; want the origin record neither rewritten nor committed", m.Kind, m.Target)
		}
	}
}

// TestAdd_OriginRecordMutationEntries asserts the mutation snapshot of a successful Add.
// It holds exactly one KindFileWritten entry for the record and one KindCommitCreated entry for its commit.
// The commit entry's target is the new pair's weft worktree, and its detail is the sha the commit landed at.
// It is the only guard on the commit:
// AssertRecordMatchesDiff classifies KindCommitCreated as a git-state kind and exempts it from the commission direction.
//
//testtiming:keep exactly one file_written entry for the origin record and one commit_created entry targeting the weft worktree with the landed sha; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_OriginRecordMutationEntries(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "record-mutation-entries"

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	weftBranch := fabricengine.RecordsBranchName(slug)
	wantSHA := gitkit.RevParse(t, weftPath, "refs/heads/"+weftBranch)

	var fileWrittenCount, commitCreatedCount int
	for _, m := range res.Mutated().Entries() {
		switch m.Kind {
		case fabricengine.KindFileWritten:
			if strings.HasSuffix(recordPath, m.Target) || strings.Contains(recordPath, m.Target) {
				fileWrittenCount++
			}
		case fabricengine.KindCommitCreated:
			if strings.Contains(weftPath, m.Target) || strings.Contains(m.Target, filepath.Base(weftPath)) {
				commitCreatedCount++
				if m.Detail != wantSHA {
					t.Errorf("KindCommitCreated entry detail = %q; want the record commit sha %q", m.Detail, wantSHA)
				}
			}
		}
	}
	if fileWrittenCount != 1 {
		t.Errorf("KindFileWritten entries naming the origin record = %d; want exactly 1", fileWrittenCount)
	}
	if commitCreatedCount != 1 {
		t.Errorf("KindCommitCreated entries naming the new pair's weft worktree = %d; want exactly 1", commitCreatedCount)
	}
}

// TestAdd_SkipGitWritesRecordWithoutCommit asserts that an Add run with opts.SkipGit set writes the origin record.
// The run still carries its KindFileWritten entry, but it lands no commit and carries no KindCommitCreated entry.
// The test guards that no commit is claimed on a run that performs none.
//
//testtiming:keep a file_written entry for the origin record and no commit_created entry under SkipGit; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_SkipGitWritesRecordWithoutCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "skip-git-record-only"

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true, SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("origin record missing at %s: %v", recordPath, err)
	}

	var fileWrittenCount, commitCreatedCount int
	for _, m := range res.Mutated().Entries() {
		switch m.Kind {
		case fabricengine.KindFileWritten:
			if strings.Contains(recordPath, m.Target) {
				fileWrittenCount++
			}
		case fabricengine.KindCommitCreated:
			commitCreatedCount++
		}
	}
	if fileWrittenCount != 1 {
		t.Errorf("KindFileWritten entries naming the origin record = %d; want exactly 1 even under SkipGit", fileWrittenCount)
	}
	if commitCreatedCount != 0 {
		t.Errorf("KindCommitCreated entries = %d; want 0 under SkipGit (CommitRecordsPaths must return before staging or recording anything)", commitCreatedCount)
	}
}
