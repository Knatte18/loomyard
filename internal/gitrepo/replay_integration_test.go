//go:build integration

// replay_integration_test.go covers the primitives a caller needs to replay one commit onto another history: UpstreamSHA, CommitDetail, CherryPick and CherryPickAbort, against real git repositories built under t.TempDir(), reusing the package's newRepo, newBareRemote, writeFile and commitAll fixture helpers.

package gitrepo_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestUpstreamSHA drives UpstreamSHA through a repository with no upstream, a first push that establishes tracking, and a remote that moves ahead and is fetched.
// The steps run serially over one clone and one bare remote.
func TestUpstreamSHA(t *testing.T) {
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, clonePath, "a.txt", "one")
	commitAll(t, clonePath, "first")

	if _, err := repo.UpstreamSHA(); err != gitrepo.ErrNoUpstream {
		t.Fatalf("UpstreamSHA() with no upstream error = %v; want gitrepo.ErrNoUpstream bare", err)
	}

	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	pushed := requireCurrentSHA(t, repo)
	got, err := repo.UpstreamSHA()
	if err != nil {
		t.Fatalf("UpstreamSHA() on a tracking branch error = %v; want nil", err)
	}
	if got != pushed {
		t.Errorf("UpstreamSHA() = %q; want the pushed tip %q", got, pushed)
	}

	pushFromOtherClone(t, container, bareRemote)
	if got, _ := repo.UpstreamSHA(); got != pushed {
		t.Errorf("UpstreamSHA() before a fetch = %q; want the unmoved %q", got, pushed)
	}
	if err := repo.Fetch(); err != nil {
		t.Fatalf("Fetch() error = %v; want nil", err)
	}
	moved := remoteBranchSHA(t, bareRemote, "main")
	got, err = repo.UpstreamSHA()
	if err != nil {
		t.Fatalf("UpstreamSHA() after a fetch error = %v; want nil", err)
	}
	if got != moved {
		t.Errorf("UpstreamSHA() after a fetch = %q; want the moved remote tip %q", got, moved)
	}
}

// TestCommitDetail covers a commit's full message, its changed paths against the first parent (added, modified and deleted, sorted), a root commit listing every path, and an invalid SHA.
func TestCommitDetail(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "one")
	if err := os.MkdirAll(filepath.Join(dir, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	writeFile(t, dir, "dir/b.txt", "two")
	commitAll(t, dir, "root")
	root := requireCurrentSHA(t, repo)

	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "c.txt", "added")
	if err := os.Remove(filepath.Join(dir, "dir", "b.txt")); err != nil {
		t.Fatalf("remove dir/b.txt: %v", err)
	}
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-m", "subject line", "-m", "body line")
	second := requireCurrentSHA(t, repo)

	tests := []struct {
		name        string
		sha         string
		wantMessage string
		wantPaths   []string
		wantErr     error
	}{
		{"changes against the first parent", second, "subject line\n\nbody line\n", []string{"a.txt", "c.txt", "dir/b.txt"}, nil},
		{"root commit lists every path", root, "root\n", []string{"a.txt", "dir/b.txt"}, nil},
		{"invalid SHA", "--all", "", nil, gitrepo.ErrInvalidSHA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repo.CommitDetail(tt.sha)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CommitDetail(%q) error = %v; want %v", tt.sha, err, tt.wantErr)
			}
			if got.Message != tt.wantMessage {
				t.Errorf("CommitDetail(%q).Message = %q; want %q", tt.sha, got.Message, tt.wantMessage)
			}
			if !reflect.DeepEqual(got.Paths, tt.wantPaths) {
				t.Errorf("CommitDetail(%q).Paths = %v; want %v", tt.sha, got.Paths, tt.wantPaths)
			}
		})
	}
}

// TestCherryPick drives CherryPick and CherryPickAbort over one repository: a clean pick lands the commit, a pick whose change the branch already carries succeeds with HEAD unmoved, a conflicting pick returns git's error and the abort restores HEAD, an abort with nothing in progress succeeds, and an option-shaped SHA is refused.
// The steps run serially, each building on the history the one before left.
func TestCherryPick(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "base")
	commitAll(t, dir, "base")
	gitkit.Git(t, dir, "checkout", "-b", "side")
	writeFile(t, dir, "b.txt", "from side")
	commitAll(t, dir, "side adds b")
	pickable := requireCurrentSHA(t, repo)
	writeFile(t, dir, "c.txt", "side version")
	commitAll(t, dir, "side adds c")
	conflicting := requireCurrentSHA(t, repo)
	gitkit.Git(t, dir, "checkout", "main")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"a clean pick lands the commit", func(t *testing.T) {
			before := requireCurrentSHA(t, repo)
			if err := repo.CherryPick(pickable); err != nil {
				t.Fatalf("CherryPick(%q) error = %v; want nil", pickable, err)
			}
			if after := requireCurrentSHA(t, repo); after == before {
				t.Errorf("HEAD after a clean pick = %q; want it moved", after)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "b.txt")); err != nil || string(got) != "from side" {
				t.Errorf("b.txt after the pick = %q, %v; want the picked content", got, err)
			}
			if subject := strings.TrimSpace(gitkit.Git(t, dir, "log", "-1", "--format=%s")); subject != "side adds b" {
				t.Errorf("HEAD subject after the pick = %q; want the picked commit's", subject)
			}
		}},
		{"a pick whose change is already carried leaves HEAD unmoved", func(t *testing.T) {
			before := requireCurrentSHA(t, repo)
			if err := repo.CherryPick(pickable); err != nil {
				t.Fatalf("CherryPick(%q) of an already-carried change error = %v; want nil", pickable, err)
			}
			if after := requireCurrentSHA(t, repo); after != before {
				t.Errorf("HEAD after an already-carried pick = %q; want unmoved %q", after, before)
			}
		}},
		{"a conflicting pick fails and the abort restores HEAD", func(t *testing.T) {
			writeFile(t, dir, "c.txt", "main version")
			commitAll(t, dir, "main adds c")
			before := requireCurrentSHA(t, repo)

			err := repo.CherryPick(conflicting)
			var gitErr *gitexec.GitError
			if !errors.As(err, &gitErr) {
				t.Fatalf("CherryPick(%q) with a conflict error = %v; want a wrapped *gitexec.GitError", conflicting, err)
			}
			if err := repo.CherryPickAbort(); err != nil {
				t.Fatalf("CherryPickAbort() error = %v; want nil", err)
			}
			if after := requireCurrentSHA(t, repo); after != before {
				t.Errorf("HEAD after the abort = %q; want restored %q", after, before)
			}
			if status := statusOf(t, dir); strings.TrimSpace(status) != "" {
				t.Errorf("git status --porcelain after the abort = %q; want a clean tree", status)
			}
		}},
		{"an abort with nothing in progress succeeds", func(t *testing.T) {
			if err := repo.CherryPickAbort(); err != nil {
				t.Errorf("CherryPickAbort() with no cherry-pick in progress error = %v; want nil", err)
			}
		}},
		{"an option-shaped SHA is refused", func(t *testing.T) {
			if err := repo.CherryPick("--abort"); !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("CherryPick(\"--abort\") error = %v; want gitrepo.ErrInvalidSHA", err)
			}
		}},
	}
	for _, step := range steps {
		t.Run(step.name, step.run)
	}
}
