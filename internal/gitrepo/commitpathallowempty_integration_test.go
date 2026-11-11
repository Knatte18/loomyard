// commitpathallowempty_integration_test.go tests Repo.CommitPathAllowEmpty against a real git repository.
// It uses these fixture helpers of gitrepo_test.go and commitempty_integration_test.go:
// newRepo, writeFile, requireCurrentSHA and treeSHA.

package gitrepo_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCommitPathAllowEmpty(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name        string
		stageOther  bool
		editTarget  bool
		wantContent string
	}{
		{"UnchangedPathNothingElseStaged", false, false, "initial"},
		{"UnchangedPathWithAnotherPathStaged", true, false, "initial"},
		{"ChangedPathCommitsTheChange", false, true, "changed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir, repo := newRepo(t)
			gitkit.CommitFile(t, dir, "target.txt", "initial", "init")
			parent := requireCurrentSHA(t, repo)

			if tt.stageOther {
				writeFile(t, dir, "other.txt", "staged only")
				gitkit.Git(t, dir, "add", "other.txt")
			}
			if tt.editTarget {
				writeFile(t, dir, "target.txt", "changed")
			}

			sha, err := repo.CommitPathAllowEmpty("path commit", "target.txt")
			if err != nil {
				t.Fatalf("CommitPathAllowEmpty() error = %v; want nil", err)
			}
			if sha == parent {
				t.Fatalf("CommitPathAllowEmpty() = %q; want a new commit", sha)
			}
			if head := requireCurrentSHA(t, repo); head != sha {
				t.Errorf("HEAD = %q; want the returned %q", head, sha)
			}
			if got := gitkit.Git(t, dir, "log", "-1", "--format=%s", sha); got != "path commit" {
				t.Errorf("commit message = %q; want %q", got, "path commit")
			}
			if got := gitkit.Git(t, dir, "show", sha+":target.txt"); got != tt.wantContent {
				t.Errorf("target.txt in the commit = %q; want %q", got, tt.wantContent)
			}
			if !tt.editTarget {
				if got, want := treeSHA(t, dir, sha), treeSHA(t, dir, parent); got != want {
					t.Errorf("commit tree = %q; want parent's tree %q", got, want)
				}
			}

			wantStaged := ""
			if tt.stageOther {
				wantStaged = "other.txt"
			}
			if got := gitkit.Git(t, dir, "diff", "--cached", "--name-only"); got != wantStaged {
				t.Errorf("staged paths after the commit = %q; want %q", got, wantStaged)
			}
		})
	}

	t.Run("PathMatchingNothingIsRefused", func(t *testing.T) {
		t.Parallel()

		dir, repo := newRepo(t)
		gitkit.CommitFile(t, dir, "target.txt", "initial", "init")
		parent := requireCurrentSHA(t, repo)

		sha, err := repo.CommitPathAllowEmpty("must not happen", "missing.txt")
		if err == nil {
			t.Fatal("CommitPathAllowEmpty() error = nil; want git's refusal")
		}
		if sha != "" {
			t.Errorf("CommitPathAllowEmpty() sha = %q; want \"\"", sha)
		}
		if head := requireCurrentSHA(t, repo); head != parent {
			t.Errorf("HEAD = %q; want unchanged %q", head, parent)
		}
	})
}
