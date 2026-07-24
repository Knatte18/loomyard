//go:build integration

// snapshot_test.go covers SnapshotSHA and SetSnapshotSHA against a bare
// remote shared by two clones — the fixture snapshot tracking is built
// around, since the refs it stores are pushed and must be visible across
// clones, not confined to one worktree. It reuses the newBareRemote,
// newRepoWithRemote, and cloneFromBare fixture builders from push_test.go.

package gitrepo_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
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

// TestSetSnapshotSHA_NoRemoteConfigured_SurfacesGitError asserts the third
// fixture discussion.md calls out: a repo with zero remotes configured at
// all. remoteName falls back to "origin" even though no such remote exists,
// so the push to it fails outright — a rejection that must not be confused
// with the adopt-on-conflict path (its stderr never matches
// rebaseRetryTriggers), so SetSnapshotSHA must return git's own error
// unchanged rather than silently swallowing it as if it were a conflict.
func TestSetSnapshotSHA_NoRemoteConfigured_SurfacesGitError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	sha, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	if err := repo.SetSnapshotSHA("mykey", sha); err == nil {
		t.Fatal("SetSnapshotSHA() with no remote configured error = nil; want an error")
	} else if !strings.Contains(err.Error(), "gitrepo: git push") {
		t.Errorf("SetSnapshotSHA() error = %q; want it to wrap git's own push error unchanged", err)
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

// TestSetSnapshotSHA_ConcurrentCreationRace_NewestValueWins races two clones'
// first-ever SetSnapshotSHA for the same key, where clone B's SHA strictly
// descends from clone A's. Whichever write lands first, the key must end at
// B's newer value: if A lands first, B's push is rejected by the remote-side
// creation race ("reference already exists") even though it is a pure
// fast-forward, and the adopt-then-retry-once path must recover it rather
// than silently adopting the older value. Several rounds are raced because a
// single round can resolve without contention at all (B simply lands first);
// the assertion itself is deterministic — every round must converge on B's
// value, whichever interleaving occurred.
func TestSetSnapshotSHA_ConcurrentCreationRace_NewestValueWins(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "commit 1")
	commitAll(t, cloneAPath, "commit 1")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	olderSHA, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "b.txt", "commit 2")
	commitAll(t, cloneBPath, "commit 2")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	newerSHA, err := repoB.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	const key = "racekey"
	ref := "refs/loomyard/snapshot/" + key
	for round := 0; round < 5; round++ {
		var wg sync.WaitGroup
		var errA, errB error
		wg.Add(2)
		go func() {
			defer wg.Done()
			errA = repoA.SetSnapshotSHA(key, olderSHA)
		}()
		go func() {
			defer wg.Done()
			errB = repoB.SetSnapshotSHA(key, newerSHA)
		}()
		wg.Wait()
		if errA != nil || errB != nil {
			t.Fatalf("round %d: SetSnapshotSHA errors = (%v, %v); want both nil", round, errA, errB)
		}

		remoteVal, stderr, code, err := runGit(t, bareRemote, "rev-parse", ref)
		if err != nil {
			t.Fatalf("round %d: git rev-parse (bare) error = %v", round, err)
		}
		if code != 0 {
			t.Fatalf("round %d: git rev-parse (bare) exited %d: %s", round, code, stderr)
		}
		if got := strings.TrimSpace(remoteVal); got != newerSHA {
			t.Fatalf("round %d: remote %s = %q; want %q (the strictly-newer value must never be dropped)", round, ref, got, newerSHA)
		}

		// Reset the key everywhere so the next round races the creation again.
		lyxtest.MustRun(t, bareRemote, "git", "update-ref", "-d", ref)
		for _, dir := range []string{cloneAPath, cloneBPath} {
			lyxtest.MustRun(t, dir, "git", "update-ref", "-d", ref)
		}
	}
}

// TestSetSnapshotSHA_ThreeWayCreationRace_NewestValueWins extends the
// two-clone creation race to three concurrent writers on one monotonic line of
// SHAs. Round 1's F3 fix retried a rejected creation push exactly once; under
// three or more writers that single retry can itself lose transient ref-lock
// contention, silently dropping the strictly-newest value. The bounded retry
// loop must instead converge on the newest value every round, whichever
// interleaving occurs. Several rounds are raced because a quiet interleaving
// can resolve without contention; the assertion is deterministic — the newest
// value must always end up on the remote.
func TestSetSnapshotSHA_ThreeWayCreationRace_NewestValueWins(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	// Build a three-commit monotonic line and push it, so each clone below
	// checks out the full history and can name any of the three SHAs.
	seedPath, seed := newRepoWithRemote(t, container, "seed", bareRemote)
	writeFile(t, seedPath, "f.txt", "c1")
	commitAll(t, seedPath, "commit 1")
	olderSHA, err := seed.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	writeFile(t, seedPath, "f.txt", "c2")
	commitAll(t, seedPath, "commit 2")
	midSHA, err := seed.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	writeFile(t, seedPath, "f.txt", "c3")
	commitAll(t, seedPath, "commit 3")
	newestSHA, err := seed.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if err := seed.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	// Three independent clones, each assigned a different SHA on the line; the
	// clone holding the strict tip (newestSHA) must always win the race.
	cloneOldPath, repoOld := cloneFromBare(t, container, "cloneOld", bareRemote)
	cloneMidPath, repoMid := cloneFromBare(t, container, "cloneMid", bareRemote)
	cloneNewPath, repoNew := cloneFromBare(t, container, "cloneNew", bareRemote)

	const key = "racekey"
	ref := "refs/loomyard/snapshot/" + key
	writers := []struct {
		repo *gitrepo.Repo
		sha  string
	}{
		{repoOld, olderSHA},
		{repoMid, midSHA},
		{repoNew, newestSHA},
	}

	for round := 0; round < 5; round++ {
		var wg sync.WaitGroup
		errs := make([]error, len(writers))
		for i := range writers {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				errs[i] = writers[i].repo.SetSnapshotSHA(key, writers[i].sha)
			}(i)
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: writer %d SetSnapshotSHA error = %v; want nil", round, i, err)
			}
		}

		remoteVal, stderr, code, err := runGit(t, bareRemote, "rev-parse", ref)
		if err != nil {
			t.Fatalf("round %d: git rev-parse (bare) error = %v", round, err)
		}
		if code != 0 {
			t.Fatalf("round %d: git rev-parse (bare) exited %d: %s", round, code, stderr)
		}
		if got := strings.TrimSpace(remoteVal); got != newestSHA {
			t.Fatalf("round %d: remote %s = %q; want %q (the strictly-newest value must never be dropped)", round, ref, got, newestSHA)
		}

		// Reset the key everywhere so the next round races the creation again.
		// Every writer creates its local ref before pushing (advanceAndPush's
		// update-ref) or adopts one on rejection, so all three clones hold it.
		lyxtest.MustRun(t, bareRemote, "git", "update-ref", "-d", ref)
		for _, dir := range []string{cloneOldPath, cloneMidPath, cloneNewPath} {
			lyxtest.MustRun(t, dir, "git", "update-ref", "-d", ref)
		}
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

// TestSetSnapshotSHA_OptionShapedSHA_RejectedAndRefUntouched asserts the
// SHA-injection guard: an option-shaped sha like "-d" (git update-ref's
// delete flag) must return ErrInvalidSHA before ever reaching git — without
// the guard, `git update-ref <ref> -d` deletes the ref instead of setting
// it, destroying local snapshot state.
func TestSetSnapshotSHA_OptionShapedSHA_RejectedAndRefUntouched(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, clonePath, "a.txt", "initial")
	commitAll(t, clonePath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	headSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if err := repo.SetSnapshotSHA("mykey", headSHA); err != nil {
		t.Fatalf("SetSnapshotSHA() error = %v; want nil", err)
	}

	for _, badSHA := range []string{"-d", "--stdin", "-O/dev/null", "HEAD"} {
		if err := repo.SetSnapshotSHA("mykey", badSHA); !errors.Is(err, gitrepo.ErrInvalidSHA) {
			t.Errorf("SetSnapshotSHA(mykey, %q) error = %v; want errors.Is(err, ErrInvalidSHA)", badSHA, err)
		}
	}
	if _, err := repo.ChangedFilesSince("-O/dev/null"); !errors.Is(err, gitrepo.ErrInvalidSHA) {
		t.Errorf("ChangedFilesSince(\"-O/dev/null\") error = %v; want errors.Is(err, ErrInvalidSHA)", err)
	}

	// The stored value must be untouched by any of the rejected calls.
	got, err := repo.SnapshotSHA("mykey")
	if err != nil {
		t.Fatalf("SnapshotSHA() error = %v", err)
	}
	if got != headSHA {
		t.Errorf("SnapshotSHA() after rejected option-shaped writes = %q; want %q (unchanged)", got, headSHA)
	}
}
