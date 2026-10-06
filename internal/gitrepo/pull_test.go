//go:build integration

// pull_test.go covers Repo.Pull against a real bare remote and clones, reusing push_test.go's bare-remote/clone fixtures (newBareRemote, newRepoWithRemote, cloneFromBare) since a fast-forward pull needs the same bare-remote-plus-clones shape the push rebase-retry tests already build.

package gitrepo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPull drives Pull through one bare remote and two clones.
// Pull() first fast-forwards the local branch when the remote has commits this clone lacks.
// It then enforces the fast-forward-only contract: a local branch with its own unpushed commit, pulling from a remote that has diverged from underneath it, is refused with an error rather than folded into a merge commit — and local history is left untouched.
// The steps run serially in that order and share the remote and both clones: the diverged step relies on clone A having pulled everything the fast-forward step pushed.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestPull(t *testing.T) {
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "shared.txt", "base\n")
	commitAll(t, cloneAPath, "init")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}
	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)

	if !t.Run("a remote that advanced is fast-forwarded in", func(t *testing.T) {
		writeFile(t, cloneBPath, "b.txt", "from B")
		commitAll(t, cloneBPath, "commit from B")
		if err := repoB.Push(); err != nil {
			t.Fatalf("Push() from clone B error = %v; want nil", err)
		}

		// Clone A is now behind the remote; Pull() must fast-forward it to include B's commit.
		if err := repoA.Pull(); err != nil {
			t.Fatalf("Pull() error = %v; want nil", err)
		}

		got, err := os.ReadFile(filepath.Join(cloneAPath, "b.txt"))
		if err != nil {
			t.Fatalf("read b.txt after Pull() error = %v; want the file fast-forwarded in", err)
		}
		if string(got) != "from B" {
			t.Errorf("b.txt content after Pull() = %q; want %q", got, "from B")
		}
	}) {
		return
	}

	t.Run("a diverged local branch is refused and left untouched", func(t *testing.T) {
		writeFile(t, cloneBPath, "shared.txt", "from B\n")
		commitAll(t, cloneBPath, "conflicting commit from B")
		if err := repoB.Push(); err != nil {
			t.Fatalf("Push() from clone B error = %v; want nil", err)
		}

		// Clone A commits its own conflicting local change without pushing, diverging from the remote it is about to try to Pull from.
		writeFile(t, cloneAPath, "shared.txt", "from A\n")
		commitAll(t, cloneAPath, "commit from A")
		localHead := requireCurrentSHA(t, repoA)

		err := repoA.Pull()
		if err == nil {
			t.Fatal("Pull() on a diverged branch error = nil; want an error (fast-forward-only refuses to merge)")
		}
		if strings.Contains(err.Error(), "fatal:") {
			t.Errorf("Pull() error = %q; must not leak raw git stderr", err)
		}
		if !strings.Contains(err.Error(), "git -C") {
			t.Errorf("Pull() error = %q; want it to name the reproducing `git -C ... pull` command so a nonzero exit stays diagnosable", err)
		}

		if headAfter := requireCurrentSHA(t, repoA); headAfter != localHead {
			t.Errorf("HEAD after refused Pull() = %q; want unchanged %q (local history must not move)", headAfter, localHead)
		}
	})
}
