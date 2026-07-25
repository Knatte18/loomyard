//go:build integration

// write_test.go holds the differential parity tests for gitnativepoc's
// write-surface methods in write.go: for each operation and fixture built by
// harness_test.go's builders, it runs the go-git-backed poc method alongside
// gitrepo.Repo's CLI-backed reference method and asserts the two agree on
// behaviour, per the plan's differential-oracle Shared Decision. Unlike
// read_test.go's parity tests, a write operation's own SHA is never
// byte-comparable across a separately-created poc fixture and cli fixture
// (each side creates its own new commit, and a commit hash embeds its author
// timestamp), so parity here is asserted as behavioural/content equivalence
// (same files changed, same final outcome, same conflict resolution) rather
// than raw SHA equality — each test's comment says which shape it uses. Every
// test carries its MIGRATE/CLI-BOUND verdict as a comment, per the
// cli-bound-is-a-recorded-outcome Shared Decision; TestPush_RebaseRetryOnNonFastForward
// is the pivotal one the batch exists to answer.

package gitnativepoc

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestStageAndCommit_EmptyFileList is MIGRATE: an empty files list is a pure
// no-op on both sides — ("", false, nil) with no git/go-git work performed at
// all — asserted against a shared fixture dir since neither side mutates it.
func TestStageAndCommit_EmptyFileList(t *testing.T) {
	dir := newRepoFixture(t)

	cliSHA, cliCommitted, cliErr := gitrepo.New(dir).StageAndCommit("noop", nil)
	if cliErr != nil {
		t.Fatalf("gitrepo StageAndCommit(nil) error = %v", cliErr)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocSHA, pocCommitted, pocErr := poc.StageAndCommit("noop", nil)
	if pocErr != nil {
		t.Fatalf("gitnativepoc StageAndCommit(nil) error = %v", pocErr)
	}

	assertParityBool(t, pocCommitted, cliCommitted)
	assertParitySHA(t, pocSHA, cliSHA)
	if cliCommitted || pocCommitted {
		t.Fatalf("StageAndCommit(nil) committed = (poc %v, cli %v), want both false", pocCommitted, cliCommitted)
	}
}

// TestStageAndCommit_NothingToCommit is MIGRATE: staging a file whose
// content already matches HEAD reports ("", false, nil) on both sides — the
// scoped equivalent of `diff --cached --quiet -- <files>` reporting exit 0 —
// asserted against a shared fixture dir since neither side ends up mutating
// it (the git-add each side performs re-stages identical content, which is
// itself a no-op).
func TestStageAndCommit_NothingToCommit(t *testing.T) {
	dir := newRepoFixture(t)

	cliSHA, cliCommitted, cliErr := gitrepo.New(dir).StageAndCommit("noop", []string{"a.txt"})
	if cliErr != nil {
		t.Fatalf("gitrepo StageAndCommit(a.txt unchanged) error = %v", cliErr)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocSHA, pocCommitted, pocErr := poc.StageAndCommit("noop", []string{"a.txt"})
	if pocErr != nil {
		t.Fatalf("gitnativepoc StageAndCommit(a.txt unchanged) error = %v", pocErr)
	}

	assertParityBool(t, pocCommitted, cliCommitted)
	assertParitySHA(t, pocSHA, cliSHA)
	if cliCommitted || pocCommitted {
		t.Fatalf("StageAndCommit(unchanged file) committed = (poc %v, cli %v), want both false", pocCommitted, cliCommitted)
	}
}

// TestStageAndCommit_RealCommit is MIGRATE: a genuine content change is
// staged and committed on both sides. Real commit SHA parity here means each
// side's returned sha is verified to actually be that fixture's new HEAD
// (self-consistency, checked independently per side via headSHA) rather than
// raw cross-fixture SHA equality (impossible for two independently-created
// commits — see the file comment); the shared behavioural check is that
// ChangedFilesSince(initial SHA) — read.go/card 6, already proven MIGRATE —
// reports the identical file list on both sides.
func TestStageAndCommit_RealCommit(t *testing.T) {
	cliDir := newRepoFixture(t)
	cliInitialSHA := firstCommitSHA(t, cliDir)
	writeFixtureFile(t, cliDir, "new.txt", "added content")
	cliRepo := gitrepo.New(cliDir)
	cliSHA, cliCommitted, cliErr := cliRepo.StageAndCommit("add new.txt", []string{"new.txt"})
	if cliErr != nil {
		t.Fatalf("gitrepo StageAndCommit(new.txt) error = %v", cliErr)
	}
	if !cliCommitted {
		t.Fatalf("gitrepo StageAndCommit(new.txt) committed = false, want true")
	}
	if got := headSHA(t, cliDir); got != cliSHA {
		t.Errorf("gitrepo StageAndCommit returned sha %q, but HEAD is %q", cliSHA, got)
	}

	pocDir := newRepoFixture(t)
	pocInitialSHA := firstCommitSHA(t, pocDir)
	writeFixtureFile(t, pocDir, "new.txt", "added content")
	poc, err := OpenRepo(pocDir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", pocDir, err)
	}
	pocSHA, pocCommitted, pocErr := poc.StageAndCommit("add new.txt", []string{"new.txt"})
	if pocErr != nil {
		t.Fatalf("gitnativepoc StageAndCommit(new.txt) error = %v", pocErr)
	}
	if !pocCommitted {
		t.Fatalf("gitnativepoc StageAndCommit(new.txt) committed = false, want true")
	}
	if got := headSHA(t, pocDir); got != pocSHA {
		t.Errorf("gitnativepoc StageAndCommit returned sha %q, but HEAD is %q", pocSHA, got)
	}

	assertParityBool(t, pocCommitted, cliCommitted)

	cliChanged, cliErr := cliRepo.ChangedFilesSince(cliInitialSHA)
	if cliErr != nil {
		t.Fatalf("gitrepo ChangedFilesSince() error = %v", cliErr)
	}
	pocChanged, pocErr := poc.ChangedFilesSince(pocInitialSHA)
	if pocErr != nil {
		t.Fatalf("gitnativepoc ChangedFilesSince() error = %v", pocErr)
	}
	assertParityFileList(t, pocChanged, cliChanged)
}

// TestStageAndCommit_ScopedCommitLeavesOtherStagedEntries is MIGRATE: a
// pathspec-scoped commit includes only the listed file and leaves any
// already-staged entry outside the pathspec staged-but-uncommitted, on both
// sides — see write.go's StageAndCommit doc comment for the synthetic-index
// mechanism go-git needs to reproduce this (Worktree.Commit alone has no
// pathspec option to do it directly).
func TestStageAndCommit_ScopedCommitLeavesOtherStagedEntries(t *testing.T) {
	setup := func(t *testing.T) string {
		dir := newRepoFixture(t)
		writeFixtureFile(t, dir, "b.txt", "staged elsewhere")
		lyxtest.MustRun(t, dir, "git", "add", "b.txt")
		writeFixtureFile(t, dir, "c.txt", "scoped commit target")
		return dir
	}

	cliDir := setup(t)
	cliRepo := gitrepo.New(cliDir)
	cliSHA, cliCommitted, cliErr := cliRepo.StageAndCommit("commit c only", []string{"c.txt"})
	if cliErr != nil {
		t.Fatalf("gitrepo StageAndCommit(c.txt) error = %v", cliErr)
	}
	if !cliCommitted {
		t.Fatalf("gitrepo StageAndCommit(c.txt) committed = false, want true")
	}

	pocDir := setup(t)
	poc, err := OpenRepo(pocDir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", pocDir, err)
	}
	pocSHA, pocCommitted, pocErr := poc.StageAndCommit("commit c only", []string{"c.txt"})
	if pocErr != nil {
		t.Fatalf("gitnativepoc StageAndCommit(c.txt) error = %v", pocErr)
	}
	if !pocCommitted {
		t.Fatalf("gitnativepoc StageAndCommit(c.txt) committed = false, want true")
	}

	assertParityBool(t, pocCommitted, cliCommitted)

	for _, tc := range []struct {
		label string
		dir   string
		sha   string
	}{
		{"gitrepo", cliDir, cliSHA},
		{"gitnativepoc", pocDir, pocSHA},
	} {
		// b.txt must never have entered the new commit's tree.
		if _, stderr, code, err := runGit(t, tc.dir, "show", tc.sha+":b.txt"); err != nil || code == 0 {
			t.Errorf("%s: git show %s:b.txt succeeded (stderr=%q), want it to fail — b.txt must not be part of the scoped commit", tc.label, tc.sha, stderr)
		}
		// c.txt must have been committed with its worktree content.
		stdout, stderr, code, err := runGit(t, tc.dir, "show", tc.sha+":c.txt")
		if err != nil {
			t.Fatalf("%s: git show %s:c.txt error = %v", tc.label, tc.sha, err)
		}
		if code != 0 {
			t.Fatalf("%s: git show %s:c.txt exited %d: %s", tc.label, tc.sha, code, stderr)
		}
		if strings.TrimSpace(stdout) != "scoped commit target" {
			t.Errorf("%s: git show %s:c.txt = %q, want %q", tc.label, tc.sha, strings.TrimSpace(stdout), "scoped commit target")
		}
		// b.txt must still be staged in the real index — untouched by the
		// scoped commit above.
		staged, stderr, code, err := runGit(t, tc.dir, "diff", "--cached", "--name-only")
		if err != nil {
			t.Fatalf("%s: git diff --cached --name-only error = %v", tc.label, err)
		}
		if code != 0 {
			t.Fatalf("%s: git diff --cached --name-only exited %d: %s", tc.label, code, stderr)
		}
		if !containsString(strings.Fields(staged), "b.txt") {
			t.Errorf("%s: git diff --cached --name-only = %q, want it to still list b.txt as staged", tc.label, staged)
		}
	}
}

// TestStageAllAndCommit_RealCommit is MIGRATE: `AddOptions{All: true}` plus
// Worktree.Commit stage and commit every working-tree change, matching
// gitrepo.StageAllAndCommit's `git add -A && git commit`. Parity is asserted
// the same content-equivalence way as TestStageAndCommit_RealCommit, via
// ChangedFilesSince.
func TestStageAllAndCommit_RealCommit(t *testing.T) {
	cliDir := newRepoFixture(t)
	cliInitialSHA := firstCommitSHA(t, cliDir)
	writeFixtureFile(t, cliDir, "new.txt", "added content")
	writeFixtureFile(t, cliDir, "other.txt", "also added")
	cliRepo := gitrepo.New(cliDir)
	cliSHA, cliCommitted, cliErr := cliRepo.StageAllAndCommit("add everything")
	if cliErr != nil {
		t.Fatalf("gitrepo StageAllAndCommit() error = %v", cliErr)
	}
	if !cliCommitted {
		t.Fatalf("gitrepo StageAllAndCommit() committed = false, want true")
	}

	pocDir := newRepoFixture(t)
	pocInitialSHA := firstCommitSHA(t, pocDir)
	writeFixtureFile(t, pocDir, "new.txt", "added content")
	writeFixtureFile(t, pocDir, "other.txt", "also added")
	poc, err := OpenRepo(pocDir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", pocDir, err)
	}
	pocSHA, pocCommitted, pocErr := poc.StageAllAndCommit("add everything")
	if pocErr != nil {
		t.Fatalf("gitnativepoc StageAllAndCommit() error = %v", pocErr)
	}
	if !pocCommitted {
		t.Fatalf("gitnativepoc StageAllAndCommit() committed = false, want true")
	}

	assertParityBool(t, pocCommitted, cliCommitted)
	if got := headSHA(t, cliDir); got != cliSHA {
		t.Errorf("gitrepo StageAllAndCommit returned sha %q, but HEAD is %q", cliSHA, got)
	}
	if got := headSHA(t, pocDir); got != pocSHA {
		t.Errorf("gitnativepoc StageAllAndCommit returned sha %q, but HEAD is %q", pocSHA, got)
	}

	cliChanged, cliErr := cliRepo.ChangedFilesSince(cliInitialSHA)
	if cliErr != nil {
		t.Fatalf("gitrepo ChangedFilesSince() error = %v", cliErr)
	}
	pocChanged, pocErr := poc.ChangedFilesSince(pocInitialSHA)
	if pocErr != nil {
		t.Fatalf("gitnativepoc ChangedFilesSince() error = %v", pocErr)
	}
	assertParityFileList(t, pocChanged, cliChanged)
}

// TestStageAllAndCommit_NothingToCommit is MIGRATE: a clean working tree
// after the wildcard add reports ("", false, nil) on both sides, asserted on
// a shared fixture dir since neither side mutates it.
func TestStageAllAndCommit_NothingToCommit(t *testing.T) {
	dir := newRepoFixture(t)

	cliSHA, cliCommitted, cliErr := gitrepo.New(dir).StageAllAndCommit("noop")
	if cliErr != nil {
		t.Fatalf("gitrepo StageAllAndCommit() error = %v", cliErr)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocSHA, pocCommitted, pocErr := poc.StageAllAndCommit("noop")
	if pocErr != nil {
		t.Fatalf("gitnativepoc StageAllAndCommit() error = %v", pocErr)
	}

	assertParityBool(t, pocCommitted, cliCommitted)
	assertParitySHA(t, pocSHA, cliSHA)
	if cliCommitted || pocCommitted {
		t.Fatalf("StageAllAndCommit() on a clean tree committed = (poc %v, cli %v), want both false", pocCommitted, cliCommitted)
	}
}

// TestPush_RebaseRetryOnNonFastForward is the pivotal test this batch exists
// to answer. VERDICT: CLI-BOUND. Evidence: the CLI oracle side
// (gitrepo.Push, exercised first below) genuinely recovers from the
// non-fast-forward rejection via `git pull --rebase` and its retried push
// lands on the remote. The poc side hits the identical rejection shape but
// returns ErrPushCLIBound instead of recovering, because go-git v5.19.1 ships
// no rebase implementation at all (grepping the entire module source for any
// Rebase type or function returns nothing) — there is no API this poc could
// call to replay CloneA's commit on top of the fetched remote history the
// way `git pull --rebase` does. This is the single most important divergence
// the write-surface spike found: a go-git migration would need to
// reimplement rebase from scratch (walking each local-only commit, replaying
// its tree changes onto the new base, one at a time) to reach parity here,
// which is out of scope for a client-library replacement. No Windows-hinged
// aspect: the divergence is go-git's own feature gap, not an OS-specific
// behaviour, so nothing here needs a Win11-pending marker.
func TestPush_RebaseRetryOnNonFastForward(t *testing.T) {
	// CLI oracle: gitrepo.Push recovers from the rejection.
	cliFixture := newBareRemoteFixture(t)
	writeFixtureFile(t, cliFixture.CloneB, "fromB.txt", "b's change")
	lyxtest.MustRun(t, cliFixture.CloneB, "git", "add", ".")
	lyxtest.MustRun(t, cliFixture.CloneB, "git", "commit", "-m", "b advances main")
	lyxtest.MustRun(t, cliFixture.CloneB, "git", "push")

	writeFixtureFile(t, cliFixture.CloneA, "fromA.txt", "a's change")
	lyxtest.MustRun(t, cliFixture.CloneA, "git", "add", ".")
	lyxtest.MustRun(t, cliFixture.CloneA, "git", "commit", "-m", "a advances main")

	if err := gitrepo.New(cliFixture.CloneA).Push(); err != nil {
		t.Fatalf("gitrepo Push() after non-fast-forward rejection error = %v, want the rebase-retry recovery to succeed", err)
	}
	remoteSHA := remoteMainSHA(t, cliFixture.Bare)
	cloneASHA := headSHA(t, cliFixture.CloneA)
	if remoteSHA != cloneASHA {
		t.Fatalf("gitrepo Push() recovery did not land: remote main = %q, cloneA HEAD = %q", remoteSHA, cloneASHA)
	}

	// poc side: identical fixture shape, independent clones.
	pocFixture := newBareRemoteFixture(t)
	writeFixtureFile(t, pocFixture.CloneB, "fromB.txt", "b's change")
	lyxtest.MustRun(t, pocFixture.CloneB, "git", "add", ".")
	lyxtest.MustRun(t, pocFixture.CloneB, "git", "commit", "-m", "b advances main")
	lyxtest.MustRun(t, pocFixture.CloneB, "git", "push")

	writeFixtureFile(t, pocFixture.CloneA, "fromA.txt", "a's change")
	lyxtest.MustRun(t, pocFixture.CloneA, "git", "add", ".")
	lyxtest.MustRun(t, pocFixture.CloneA, "git", "commit", "-m", "a advances main")

	poc, err := OpenRepo(pocFixture.CloneA)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", pocFixture.CloneA, err)
	}
	pocErr := poc.Push()
	if !errors.Is(pocErr, ErrPushCLIBound) {
		t.Fatalf("gitnativepoc Push() after non-fast-forward rejection error = %v, want errors.Is(_, ErrPushCLIBound) — if this now succeeds, go-git gained rebase support and this test's verdict comment must be updated to MIGRATE", pocErr)
	}
}

// remoteMainSHA returns the SHA refs/heads/main currently points at on a
// bare remote.
func remoteMainSHA(t *testing.T, bare string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, bare, "rev-parse", "refs/heads/main")
	if err != nil {
		t.Fatalf("git rev-parse refs/heads/main error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse refs/heads/main exited %d: %s", code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// TestSetSnapshotSHA_FastForwardPush is MIGRATE: with no prior value on the
// remote, advancing and pushing refs/loomyard/snapshot/<key> succeeds on the
// first attempt on both sides — asserted per-side against that side's own
// sha (see the file comment on why a real write's sha cannot be compared
// byte-for-byte across two independently-created fixtures).
func TestSetSnapshotSHA_FastForwardPush(t *testing.T) {
	const key = "ff-only"

	cliFixture := newBareRemoteFixture(t)
	cliSHA := firstCommitSHA(t, cliFixture.CloneA)
	if err := gitrepo.New(cliFixture.CloneA).SetSnapshotSHA(key, cliSHA); err != nil {
		t.Fatalf("gitrepo SetSnapshotSHA(%q, %q) error = %v", key, cliSHA, err)
	}
	assertParitySHA(t, remoteRefValue(t, cliFixture.Bare, key), cliSHA)

	pocFixture := newBareRemoteFixture(t)
	pocSHA := firstCommitSHA(t, pocFixture.CloneA)
	poc, err := OpenRepo(pocFixture.CloneA)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", pocFixture.CloneA, err)
	}
	if err := poc.SetSnapshotSHA(key, pocSHA); err != nil {
		t.Fatalf("gitnativepoc SetSnapshotSHA(%q, %q) error = %v", key, pocSHA, err)
	}
	assertParitySHA(t, remoteRefValue(t, pocFixture.Bare, key), pocSHA)
}

// TestSetSnapshotSHA_AdoptOnConflict is MIGRATE, per gitrepo's own rejection
// detection posture: go-git offers no typed rejection either (see
// isNonFastForwardRejection's doc comment in write.go — a data point in
// itself, not a divergence), yet the adopt-on-conflict resolution still
// lands identically on both sides. A concurrent writer (CloneB) sets the key
// first, forcing CloneA's own attempt to be rejected (the remote already
// carries the key); since CloneA's sha strictly descends from what gets
// adopted, isStrictDescendant (read.go/card 7) correctly identifies it as
// genuinely further along, and the retry re-advances and lands — asserted
// per-side (see the file comment on cross-fixture SHA comparability) that
// the final remote value is the caller's strictly-newer sha, not the
// concurrent writer's stale one.
func TestSetSnapshotSHA_AdoptOnConflict(t *testing.T) {
	const key = "concurrent"

	setup := func(t *testing.T) (fixture bareRemoteFixture, newSHA string) {
		fixture = newBareRemoteFixture(t)
		initSHA := firstCommitSHA(t, fixture.CloneA)

		// The concurrent writer: CloneB sets the key to the baseline commit
		// first, landing on the remote before CloneA ever attempts to.
		lyxtest.MustRun(t, fixture.CloneB, "git", "update-ref", "refs/loomyard/snapshot/"+key, initSHA)
		lyxtest.MustRun(t, fixture.CloneB, "git", "push", "origin", "refs/loomyard/snapshot/"+key)

		// CloneA advances past initSHA with a commit of its own before
		// attempting to set the same key, so its sha genuinely descends from
		// what is now on the remote.
		writeFixtureFile(t, fixture.CloneA, "further.txt", "cloneA advances")
		lyxtest.MustRun(t, fixture.CloneA, "git", "add", ".")
		lyxtest.MustRun(t, fixture.CloneA, "git", "commit", "-m", "cloneA advances")
		return fixture, headSHA(t, fixture.CloneA)
	}

	cliFixture, cliNewSHA := setup(t)
	if err := gitrepo.New(cliFixture.CloneA).SetSnapshotSHA(key, cliNewSHA); err != nil {
		t.Fatalf("gitrepo SetSnapshotSHA(%q, %q) error = %v, want the adopt-then-retry path to resolve without error", key, cliNewSHA, err)
	}
	if got := remoteRefValue(t, cliFixture.Bare, key); got != cliNewSHA {
		t.Errorf("gitrepo: remote refs/loomyard/snapshot/%s = %q, want %q (the strictly-newer write must win after adopting and retrying)", key, got, cliNewSHA)
	}

	pocFixture, pocNewSHA := setup(t)
	poc, err := OpenRepo(pocFixture.CloneA)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", pocFixture.CloneA, err)
	}
	if err := poc.SetSnapshotSHA(key, pocNewSHA); err != nil {
		t.Fatalf("gitnativepoc SetSnapshotSHA(%q, %q) error = %v, want the adopt-then-retry path to resolve without error", key, pocNewSHA, err)
	}
	if got := remoteRefValue(t, pocFixture.Bare, key); got != pocNewSHA {
		t.Errorf("gitnativepoc: remote refs/loomyard/snapshot/%s = %q, want %q (the strictly-newer write must win after adopting and retrying)", key, got, pocNewSHA)
	}
}

// remoteRefValue returns the SHA refs/loomyard/snapshot/<key> currently
// points at on a bare remote.
func remoteRefValue(t *testing.T, bare, key string) string {
	t.Helper()

	ref := "refs/loomyard/snapshot/" + key
	stdout, stderr, code, err := runGit(t, bare, "rev-parse", ref)
	if err != nil {
		t.Fatalf("git rev-parse %s error = %v", ref, err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse %s exited %d: %s", ref, code, stderr)
	}
	return strings.TrimSpace(stdout)
}
