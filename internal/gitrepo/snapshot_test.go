//go:build integration

// snapshot_test.go covers SnapshotSHA and SetSnapshotSHA against a bare
// remote shared by two clones — the fixture snapshot tracking is built
// around, since the refs it stores are pushed and must be visible across
// clones, not confined to one worktree. It reuses the newBareRemote,
// newRepoWithRemote, and cloneFromBare fixture builders from push_test.go.

package gitrepo_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestSnapshotSHA_ReturnsEmptyBeforeAnySet asserts SnapshotSHA's documented
// ("", nil) result when a key has never been set — an absent ref is a
// normal state, not a failure.
func TestSnapshotSHA_ReturnsEmptyBeforeAnySet(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, clonePath, "a.txt", "initial")
	commitAll(t, clonePath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	got, err := repo.SnapshotSHA("nokey")
	if err != nil {
		t.Fatalf("SnapshotSHA() error = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("SnapshotSHA() = %q; want \"\" before any SetSnapshotSHA", got)
	}
}

// TestSnapshotSHA_SetInCloneA_VisibleInCloneB asserts the core round-trip:
// a value set by one clone and pushed to the shared remote is visible to a
// second clone's SnapshotSHA, which fetches the snapshot namespace first.
func TestSnapshotSHA_SetInCloneA_VisibleInCloneB(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "initial")
	commitAll(t, cloneAPath, "init")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	headSHA, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	if err := repoA.SetSnapshotSHA("mykey", headSHA); err != nil {
		t.Fatalf("SetSnapshotSHA() error = %v; want nil", err)
	}

	// Clone B checks out the bare remote after the ref push, so its
	// SnapshotSHA read must fetch the snapshot namespace to see it.
	_, repoB := cloneFromBare(t, container, "cloneB", bareRemote)

	got, err := repoB.SnapshotSHA("mykey")
	if err != nil {
		t.Fatalf("SnapshotSHA() error = %v; want nil", err)
	}
	if got != headSHA {
		t.Errorf("SnapshotSHA() = %q; want %q (value set by clone A)", got, headSHA)
	}
}

// TestSetSnapshotSHA_AdoptsOnFastForwardConflict asserts the
// fast-forward-only adopt-on-conflict path: once clone B has advanced a key
// past the value clone A still holds locally, clone A's own attempt to set
// that key back to an older, non-descendant SHA is rejected by the remote
// and silently adopts clone B's value rather than returning an error.
func TestSetSnapshotSHA_AdoptsOnFastForwardConflict(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "commit 1")
	commitAll(t, cloneAPath, "commit 1")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	headSHA1, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "b.txt", "commit 2")
	commitAll(t, cloneBPath, "commit 2")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	headSHA2, err := repoB.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	// Clone B advances the key first and pushes it — headSHA2 is a
	// descendant of headSHA1, so this push is a clean fast-forward (the
	// key had no prior value at all).
	if err := repoB.SetSnapshotSHA("mykey", headSHA2); err != nil {
		t.Fatalf("SetSnapshotSHA() (clone B) error = %v; want nil", err)
	}

	// Clone A never fetched the snapshot namespace, so its attempt to set
	// the same key to the older headSHA1 is a non-fast-forward push
	// against the remote's current value (headSHA2, ahead of headSHA1) —
	// it must be rejected and silently adopt headSHA2 rather than
	// returning an error.
	if err := repoA.SetSnapshotSHA("mykey", headSHA1); err != nil {
		t.Fatalf("SetSnapshotSHA() (clone A, non-descendant) error = %v; want nil (adopt-on-conflict)", err)
	}

	got, err := repoA.SnapshotSHA("mykey")
	if err != nil {
		t.Fatalf("SnapshotSHA() error = %v; want nil", err)
	}
	if got != headSHA2 {
		t.Errorf("SnapshotSHA() (clone A after adopt) = %q; want %q (clone B's advanced value)", got, headSHA2)
	}
}

// TestSnapshotMethods_InvalidKey_ReturnsErrInvalidSnapshotKey asserts both
// SnapshotSHA and SetSnapshotSHA reject a ref-illegal key before it ever
// reaches git.
func TestSnapshotMethods_InvalidKey_ReturnsErrInvalidSnapshotKey(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, clonePath, "a.txt", "initial")
	commitAll(t, clonePath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	const badKey = "bad key"

	if _, err := repo.SnapshotSHA(badKey); !errors.Is(err, gitrepo.ErrInvalidSnapshotKey) {
		t.Errorf("SnapshotSHA(%q) error = %v; want errors.Is(err, ErrInvalidSnapshotKey)", badKey, err)
	}

	if err := repo.SetSnapshotSHA(badKey, "0123456789abcdef0123456789abcdef01234567"); !errors.Is(err, gitrepo.ErrInvalidSnapshotKey) {
		t.Errorf("SetSnapshotSHA(%q, ...) error = %v; want errors.Is(err, ErrInvalidSnapshotKey)", badKey, err)
	}
}
