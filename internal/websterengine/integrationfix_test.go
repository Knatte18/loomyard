//go:build integration

// integrationfix_test.go exercises checkFixCommits against real scratch git repositories, reusing gitwrap_test.go's helpers.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixCheckRepo returns a scratch repo with one base commit, its path and that commit's SHA.
func fixCheckRepo(t *testing.T) (dir, base string) {
	t.Helper()
	dir = gitwrapNewScratchRepo(t)
	base = gitwrapCommitFile(t, dir, "a.txt", "one", "base")
	return dir, base
}

func TestCheckFixCommits_OrdinaryCommitsAccepted(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	gitwrapCommitFile(t, dir, "b.txt", "two", "fix one")
	head := gitwrapCommitFile(t, dir, "sub/c.txt", "three", "fix two")

	warning, err := checkFixCommits(dir, base, head, filepath.Join(dir, "_lyx", "plan"), gitwrapParent)
	if err != nil || warning != "" {
		t.Fatalf("checkFixCommits() = (%q, %v); want (\"\", nil)", warning, err)
	}
}

func TestCheckFixCommits_NoCommitsAccepted(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)

	warning, err := checkFixCommits(dir, base, base, filepath.Join(dir, "_lyx", "plan"), gitwrapParent)
	if err != nil || warning != "" {
		t.Fatalf("checkFixCommits() = (%q, %v); want (\"\", nil)", warning, err)
	}
}

func TestCheckFixCommits_MergeCommitInRangeRefused(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	merge, _ := gitwrapMergeSide(t, dir, "side")

	_, err := checkFixCommits(dir, base, merge, filepath.Join(dir, "_lyx", "plan"), gitwrapParent)
	if err == nil || !strings.Contains(err.Error(), merge) {
		t.Fatalf("checkFixCommits() error = %v; want a refusal naming merge %s", err, merge)
	}
}

func TestCheckFixCommits_ForbiddenPathsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path string
	}{
		{"under _lyx", "_lyx/webster/state.json"},
		{"_lyx entry at the root", "_lyx"},
		{"plan directory", "plans/00-overview.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, base := fixCheckRepo(t)
			head := gitwrapCommitFile(t, dir, tc.path, "x", "forbidden")

			_, err := checkFixCommits(dir, base, head, filepath.Join(dir, "plans"), gitwrapParent)
			if err == nil || !strings.Contains(err.Error(), tc.path) || !strings.Contains(err.Error(), head) {
				t.Fatalf("checkFixCommits() error = %v; want a refusal naming commit %s and path %s", err, head, tc.path)
			}
		})
	}
}

func TestCheckFixCommits_DirtyWorktreeRefused(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	head := gitwrapCommitFile(t, dir, "b.txt", "two", "fix")
	planDir := filepath.Join(dir, "_lyx", "plan")

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := checkFixCommits(dir, base, head, planDir, gitwrapParent); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("uncommitted change: error = %v; want a dirty-worktree refusal", err)
	}

	gitwrapMustGit(t, dir, "checkout", "--", "a.txt")
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := checkFixCommits(dir, base, head, planDir, gitwrapParent); err == nil || !strings.Contains(err.Error(), "untracked") {
		t.Fatalf("untracked file: error = %v; want a dirty-worktree refusal", err)
	}
}

func TestCheckFixCommits_ReportHeadNotDescendingRefused(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	gitwrapMustGit(t, dir, "checkout", "-b", "other")
	other := gitwrapCommitFile(t, dir, "o.txt", "o", "other")
	gitwrapMustGit(t, dir, "checkout", "-")
	pre := gitwrapCommitFile(t, dir, "p.txt", "p", "pre-fix")
	_ = base

	_, err := checkFixCommits(dir, pre, other, filepath.Join(dir, "_lyx", "plan"), gitwrapParent)
	if err == nil || !strings.Contains(err.Error(), "descend") {
		t.Fatalf("checkFixCommits() error = %v; want a does-not-descend refusal", err)
	}
}

func TestCheckFixCommits_NonMergeCommitAfterReportRefused(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	report := gitwrapCommitFile(t, dir, "b.txt", "two", "fix")
	gitwrapCommitFile(t, dir, "c.txt", "three", "after the report")

	_, err := checkFixCommits(dir, base, report, filepath.Join(dir, "_lyx", "plan"), gitwrapParent)
	if err == nil || !strings.Contains(err.Error(), report) {
		t.Fatalf("checkFixCommits() error = %v; want a HEAD-mismatch refusal naming %s", err, report)
	}
}

func TestFixCommitsSince_ListsMergeButNotParentBranchCommits(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	fix := gitwrapCommitFile(t, dir, "b.txt", "two", "fix")
	merge, sideTip := gitwrapMergeSide(t, dir, gitwrapParentBranch)

	commits, err := fixCommitsSince(dir, base)
	if err != nil {
		t.Fatalf("fixCommitsSince() error = %v", err)
	}
	if len(commits) != 2 || commits[0] != fix || commits[1] != merge {
		t.Errorf("fixCommitsSince() = %v; want [%s %s] without the parent branch's %s", commits, fix, merge, sideTip)
	}
}

func TestCheckFixCommits_CleanParentMergeAfterReportWarns(t *testing.T) {
	t.Parallel()
	dir, base := fixCheckRepo(t)
	report := gitwrapCommitFile(t, dir, "b.txt", "two", "fix")
	merge, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

	warning, err := checkFixCommits(dir, base, report, filepath.Join(dir, "_lyx", "plan"), gitwrapParent)
	if err != nil {
		t.Fatalf("checkFixCommits() error = %v; want nil", err)
	}
	if !strings.Contains(warning, merge) {
		t.Errorf("warning %q; want it to name merge %s", warning, merge)
	}
}
