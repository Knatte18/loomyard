//go:build integration

// merge_integration_test.go covers the merge primitives in merge.go against real git repositories,
// reusing gitrepo_test.go's fixture helpers (newRepo, writeFile, commitAll, runGit) and, for the
// remote-tracking-ref ResolveSHA case, push_test.go's newBareRemote/cloneFromBare helpers.

package gitrepo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// checkoutNewBranch creates and checks out a new branch named name from the current HEAD.
func checkoutNewBranch(t *testing.T, dir, name string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-b", name)
}

// checkoutBranch switches to an existing branch named name.
func checkoutBranch(t *testing.T, dir, name string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", name)
}

// mergeHeadPresentOnDisk reports whether MERGE_HEAD exists, checked directly via git rather than
// through the Repo under test, so ConflictedFiles/MergeHeadPresent fixture setup can be asserted
// independently of the methods being tested.
func mergeHeadPresentOnDisk(t *testing.T, dir string) bool {
	t.Helper()

	_, _, code, err := runGit(t, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	if err != nil {
		t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD error = %v", err)
	}
	return code == 0
}

// TestMergeConflictedFiles_ManufacturedConflict asserts ConflictedFiles returns the conflicted path
// after two branches edit the same line.
func TestMergeConflictedFiles_ManufacturedConflict(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "shared.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "shared.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "shared.txt", "main\n")
	commitAll(t, dir, "main edit")

	outcome, err := repo.MergeStart("feature", false)
	if err != nil {
		t.Fatalf("MergeStart(feature, false) error = %v; want nil (conflict is a result, not an error)", err)
	}
	if outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(feature, false) outcome = %v; want MergeConflicted", outcome)
	}

	got, err := repo.ConflictedFiles()
	if err != nil {
		t.Fatalf("ConflictedFiles() error = %v; want nil", err)
	}
	want := []string{"shared.txt"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("ConflictedFiles() = %v; want %v", got, want)
	}
}

// TestMergeConflictedFiles_NonASCIIPathIsRawNeverQuoted asserts ConflictedFiles returns the raw
// path bytes for a conflicted filename outside core.quotepath's default ASCII set, never git's
// C-quoted form (`"\303\244.txt"`, quotes included) that `--name-only` without `-z` emits — the
// quoted form is not a real worktree path, and fabricengine's visible-tree mapping misclassified
// a mappable conflict as unmergeable on it.
func TestMergeConflictedFiles_NonASCIIPathIsRawNeverQuoted(t *testing.T) {
	dir, repo := newRepo(t)
	const conflictedName = "ä-nöte.txt"
	writeFile(t, dir, conflictedName, "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, conflictedName, "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, conflictedName, "main\n")
	commitAll(t, dir, "main edit")

	outcome, err := repo.MergeStart("feature", false)
	if err != nil {
		t.Fatalf("MergeStart(feature, false) error = %v; want nil (conflict is a result, not an error)", err)
	}
	if outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(feature, false) outcome = %v; want MergeConflicted", outcome)
	}

	got, err := repo.ConflictedFiles()
	if err != nil {
		t.Fatalf("ConflictedFiles() error = %v; want nil", err)
	}
	if len(got) != 1 || got[0] != conflictedName {
		t.Errorf("ConflictedFiles() = %q; want [%q] — the raw bytes, not git's C-quoted rendering", got, conflictedName)
	}
}

// TestMergeConflictedFiles_CleanTreeReturnsEmptyNeverNil asserts ConflictedFiles returns an empty,
// non-nil slice on a clean tree.
func TestMergeConflictedFiles_CleanTreeReturnsEmptyNeverNil(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	got, err := repo.ConflictedFiles()
	if err != nil {
		t.Fatalf("ConflictedFiles() error = %v; want nil", err)
	}
	if got == nil {
		t.Error("ConflictedFiles() = nil; want an empty, non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("ConflictedFiles() = %v; want empty", got)
	}
}

// TestMergeStart_CleanStaged_UncommittedWithHeadUnchanged asserts a non-conflicting, non-fast-forward
// merge stages the change and leaves HEAD unmoved and the merge uncommitted.
func TestMergeStart_CleanStaged_UncommittedWithHeadUnchanged(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feat.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "base.txt", "base\nmain edit\n")
	commitAll(t, dir, "main edit")

	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	outcome, err := repo.MergeStart("feature", false)
	if err != nil {
		t.Fatalf("MergeStart(feature, false) error = %v; want nil", err)
	}
	if outcome != gitrepo.MergeStaged {
		t.Fatalf("MergeStart(feature, false) outcome = %v; want MergeStaged", outcome)
	}

	if _, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet"); code != 1 {
		t.Errorf("git diff --cached --quiet exit = %d; want 1 (something staged)", code)
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("CurrentSHA() after MergeStart(clean-staged) = %q; want unchanged %q", headAfter, headBefore)
	}
}

// TestMergeStart_Conflicted_ReturnsNilErrorWithUnmergedEntries asserts the conflicted outcome carries
// a nil error and leaves unmerged index entries in place.
func TestMergeStart_Conflicted_ReturnsNilErrorWithUnmergedEntries(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "shared.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "shared.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "shared.txt", "main\n")
	commitAll(t, dir, "main edit")

	outcome, err := repo.MergeStart("feature", false)
	if err != nil {
		t.Fatalf("MergeStart(feature, false) error = %v; want nil", err)
	}
	if outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(feature, false) outcome = %v; want MergeConflicted", outcome)
	}

	stdout, _, code, err := runGit(t, dir, "ls-files", "--unmerged")
	if err != nil {
		t.Fatalf("git ls-files --unmerged error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git ls-files --unmerged exited %d", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("git ls-files --unmerged = \"\"; want unmerged entries present")
	}
}

// TestMergeStart_FastForwarded_HeadMovedNothingStagedNoMergeHead pins the documented ff-defeats-
// --no-commit behaviour: a fast-forward-eligible merge moves HEAD, stages nothing, and leaves no
// MERGE_HEAD, even under --no-commit.
func TestMergeStart_FastForwarded_HeadMovedNothingStagedNoMergeHead(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")
	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feat.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	featureSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	checkoutBranch(t, dir, "main")

	outcome, err := repo.MergeStart("feature", false)
	if err != nil {
		t.Fatalf("MergeStart(feature, false) error = %v; want nil", err)
	}
	if outcome != gitrepo.MergeFastForwarded {
		t.Fatalf("MergeStart(feature, false) outcome = %v; want MergeFastForwarded", outcome)
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != featureSHA {
		t.Errorf("CurrentSHA() after fast-forward = %q; want %q (feature's tip)", headAfter, featureSHA)
	}
	if headAfter == headBefore {
		t.Error("CurrentSHA() after fast-forward is unchanged; want HEAD to have moved")
	}

	if _, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet"); code != 0 {
		t.Errorf("git diff --cached --quiet exit = %d; want 0 (nothing staged)", code)
	}
	if mergeHeadPresentOnDisk(t, dir) {
		t.Error("MERGE_HEAD present after a fast-forward; want none")
	}
}

// TestMergeStart_AlreadyUpToDate_HeadUnmovedNothingStaged asserts merging a ref that is already an
// ancestor of HEAD reports MergeAlreadyUpToDate, leaving HEAD unmoved and nothing staged.
func TestMergeStart_AlreadyUpToDate_HeadUnmovedNothingStaged(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	checkoutBranch(t, dir, "main")

	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	outcome, err := repo.MergeStart("feature", false)
	if err != nil {
		t.Fatalf("MergeStart(feature, false) error = %v; want nil", err)
	}
	if outcome != gitrepo.MergeAlreadyUpToDate {
		t.Fatalf("MergeStart(feature, false) outcome = %v; want MergeAlreadyUpToDate", outcome)
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("CurrentSHA() after already-up-to-date merge = %q; want unchanged %q", headAfter, headBefore)
	}
	if _, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet"); code != 0 {
		t.Errorf("git diff --cached --quiet exit = %d; want 0 (nothing staged)", code)
	}
}

// TestMergeStart_Squash_NoMergeHeadStagedUncommitted asserts a squash merge of a diverging branch
// leaves no MERGE_HEAD and stages the change uncommitted.
func TestMergeStart_Squash_NoMergeHeadStagedUncommitted(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feat.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "base.txt", "base\nmain edit\n")
	commitAll(t, dir, "main edit")

	outcome, err := repo.MergeStart("feature", true)
	if err != nil {
		t.Fatalf("MergeStart(feature, true) error = %v; want nil", err)
	}
	if outcome != gitrepo.MergeStaged {
		t.Fatalf("MergeStart(feature, true) outcome = %v; want MergeStaged", outcome)
	}

	if mergeHeadPresentOnDisk(t, dir) {
		t.Error("MERGE_HEAD present after a squash merge; want none (squash never sets MERGE_HEAD)")
	}
	if _, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet"); code != 1 {
		t.Errorf("git diff --cached --quiet exit = %d; want 1 (something staged)", code)
	}
}

// TestMergeStart_SquashFastForwardable_StagesWithoutMovingHead asserts squash on a fast-forward-
// eligible source still stages without moving HEAD, classified MergeStaged — squash never fast-
// forwards, regardless of ff eligibility.
func TestMergeStart_SquashFastForwardable_StagesWithoutMovingHead(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feat.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")

	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	outcome, err := repo.MergeStart("feature", true)
	if err != nil {
		t.Fatalf("MergeStart(feature, true) error = %v; want nil", err)
	}
	if outcome != gitrepo.MergeStaged {
		t.Fatalf("MergeStart(feature, true) outcome = %v; want MergeStaged", outcome)
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("CurrentSHA() after squash-ff-eligible merge = %q; want unchanged %q", headAfter, headBefore)
	}
	if _, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet"); code != 1 {
		t.Errorf("git diff --cached --quiet exit = %d; want 1 (something staged)", code)
	}
}

// TestMergeStart_RejectsLeadingDashRef asserts a ref with a leading '-' is rejected with
// ErrInvalidSHA before any git spawn.
func TestMergeStart_RejectsLeadingDashRef(t *testing.T) {
	_, repo := newRepo(t)

	_, err := repo.MergeStart("--squash", false)
	if !errors.Is(err, gitrepo.ErrInvalidSHA) {
		t.Fatalf("MergeStart(--squash, false) error = %v; want errors.Is(err, ErrInvalidSHA)", err)
	}
}

// TestMergeConclude_ExplicitMessage asserts MergeConclude with a non-empty message commits with
// exactly that message.
func TestMergeConclude_ExplicitMessage(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feat.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "base.txt", "base\nmain edit\n")
	commitAll(t, dir, "main edit")

	if outcome, err := repo.MergeStart("feature", false); err != nil || outcome != gitrepo.MergeStaged {
		t.Fatalf("MergeStart(feature, false) = (%v, %v); want (MergeStaged, nil)", outcome, err)
	}

	const msg = "explicit merge message"
	if err := repo.MergeConclude(msg); err != nil {
		t.Fatalf("MergeConclude(%q) error = %v; want nil", msg, err)
	}

	got, _, code, err := runGit(t, dir, "log", "-1", "--format=%s")
	if err != nil {
		t.Fatalf("git log -1 --format=%%s error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git log -1 --format=%%s exited %d", code)
	}
	if strings.TrimSpace(got) != msg {
		t.Errorf("commit message after MergeConclude(%q) = %q; want %q", msg, strings.TrimSpace(got), msg)
	}
}

// TestMergeConclude_EmptyMessageUsesPreparedMessage asserts MergeConclude with an empty message
// takes git's own prepared MERGE_MSG rather than opening an editor.
func TestMergeConclude_EmptyMessageUsesPreparedMessage(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feat.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "base.txt", "base\nmain edit\n")
	commitAll(t, dir, "main edit")

	if outcome, err := repo.MergeStart("feature", false); err != nil || outcome != gitrepo.MergeStaged {
		t.Fatalf("MergeStart(feature, false) = (%v, %v); want (MergeStaged, nil)", outcome, err)
	}

	if err := repo.MergeConclude(""); err != nil {
		t.Fatalf("MergeConclude(\"\") error = %v; want nil", err)
	}

	got, _, code, err := runGit(t, dir, "log", "-1", "--format=%s")
	if err != nil {
		t.Fatalf("git log -1 --format=%%s error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git log -1 --format=%%s exited %d", code)
	}
	if !strings.Contains(strings.TrimSpace(got), "Merge branch 'feature'") {
		t.Errorf("commit message after MergeConclude(\"\") = %q; want git's prepared MERGE_MSG (mentioning the merged branch)", strings.TrimSpace(got))
	}
}

// TestMergeResetHard_ClearsInProgressMergeState asserts ResetHard to the pre-merge SHA clears both
// MergeHeadPresent and ConflictedFiles — the abort mechanism's load-bearing property.
func TestMergeResetHard_ClearsInProgressMergeState(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "shared.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "shared.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "shared.txt", "main\n")
	commitAll(t, dir, "main edit")

	preMergeSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	if outcome, err := repo.MergeStart("feature", false); err != nil || outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(feature, false) = (%v, %v); want (MergeConflicted, nil)", outcome, err)
	}

	if err := repo.ResetHard(preMergeSHA); err != nil {
		t.Fatalf("ResetHard(%q) error = %v; want nil", preMergeSHA, err)
	}

	present, err := repo.MergeHeadPresent()
	if err != nil {
		t.Fatalf("MergeHeadPresent() error = %v; want nil", err)
	}
	if present {
		t.Error("MergeHeadPresent() after ResetHard() = true; want false")
	}

	conflicted, err := repo.ConflictedFiles()
	if err != nil {
		t.Fatalf("ConflictedFiles() error = %v; want nil", err)
	}
	if len(conflicted) != 0 {
		t.Errorf("ConflictedFiles() after ResetHard() = %v; want empty", conflicted)
	}
}

// TestMergeHeadPresent_TrueAfterConflict_FalseAfterResetAndConclude asserts MergeHeadPresent is true
// once a conflicted non-squash merge starts, and false both after ResetHard and after MergeConclude
// on a separate clean merge.
func TestMergeHeadPresent_TrueAfterConflict_FalseAfterResetAndConclude(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "shared.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "shared.txt", "feature\n")
	commitAll(t, dir, "feature edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "shared.txt", "main\n")
	commitAll(t, dir, "main edit")
	preMergeSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	if outcome, err := repo.MergeStart("feature", false); err != nil || outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(feature, false) = (%v, %v); want (MergeConflicted, nil)", outcome, err)
	}
	present, err := repo.MergeHeadPresent()
	if err != nil {
		t.Fatalf("MergeHeadPresent() error = %v; want nil", err)
	}
	if !present {
		t.Error("MergeHeadPresent() during a conflicted non-squash merge = false; want true")
	}

	if err := repo.ResetHard(preMergeSHA); err != nil {
		t.Fatalf("ResetHard(%q) error = %v; want nil", preMergeSHA, err)
	}
	present, err = repo.MergeHeadPresent()
	if err != nil {
		t.Fatalf("MergeHeadPresent() error = %v; want nil", err)
	}
	if present {
		t.Error("MergeHeadPresent() after ResetHard() = true; want false")
	}

	// A second, independent, non-conflicting merge (main <- another branch) is concluded to prove
	// MergeHeadPresent also goes false after MergeConclude.
	checkoutNewBranch(t, dir, "another")
	writeFile(t, dir, "other.txt", "another\n")
	commitAll(t, dir, "another edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "base2.txt", "main only\n")
	commitAll(t, dir, "main-only edit")

	if outcome, err := repo.MergeStart("another", false); err != nil || outcome != gitrepo.MergeStaged {
		t.Fatalf("MergeStart(another, false) = (%v, %v); want (MergeStaged, nil)", outcome, err)
	}
	if err := repo.MergeConclude("conclude another"); err != nil {
		t.Fatalf("MergeConclude() error = %v; want nil", err)
	}
	present, err = repo.MergeHeadPresent()
	if err != nil {
		t.Fatalf("MergeHeadPresent() error = %v; want nil", err)
	}
	if present {
		t.Error("MergeHeadPresent() after MergeConclude() = true; want false")
	}
}

// TestMergeHeads_EnumeratesEveryHeadOfAnOctopus is the reason MergeHeads reads MERGE_HEAD rather
// than shelling `git rev-parse --verify --quiet MERGE_HEAD` a second time: for a two-head merge,
// rev-parse reports only the FIRST head, so a caller comparing that single answer against an
// expected SHA accepts `git merge --no-commit <expected> <decoy>` as if it were `git merge <expected>`.
// The test asserts BOTH halves — the full list MergeHeads returns, and the truncated answer the
// rev-parse spelling gives for the same state — so rewriting MergeHeads onto rev-parse fails here
// rather than silently reintroducing the first-entry-only read.
func TestMergeHeads_EnumeratesEveryHeadOfAnOctopus(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "first")
	writeFile(t, dir, "first.txt", "first\n")
	commitAll(t, dir, "first")
	checkoutBranch(t, dir, "main")

	checkoutNewBranch(t, dir, "second")
	writeFile(t, dir, "second.txt", "second\n")
	commitAll(t, dir, "second")
	checkoutBranch(t, dir, "main")

	writeFile(t, dir, "main.txt", "main\n")
	commitAll(t, dir, "main advances")

	firstSHA := resolveForTest(t, repo, "first")
	secondSHA := resolveForTest(t, repo, "second")
	gitkit.MustRun(t, dir, "git", "merge", "--no-commit", "--no-ff", "first", "second")

	// Precondition, asserted rather than assumed: the rev-parse spelling really does truncate here,
	// or this test would pass against the very implementation it exists to forbid.
	stdout, _, code, err := runGit(t, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	if err != nil || code != 0 {
		t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD = (code %d, %v); want it to succeed on the octopus", code, err)
	}
	if got := strings.TrimSpace(stdout); got != firstSHA {
		t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD = %q; want the FIRST head %q — the truncation this test is built on is not present", got, firstSHA)
	}

	heads, err := repo.MergeHeads()
	if err != nil {
		t.Fatalf("MergeHeads() error = %v; want nil", err)
	}
	if len(heads) != 2 || heads[0] != firstSHA || heads[1] != secondSHA {
		t.Errorf("MergeHeads() = %v; want [%s %s] — every head, in git's own order", heads, firstSHA, secondSHA)
	}
}

// TestMergeHeads_NoMergeLiveReturnsEmptyNeverNil asserts the no-merge answer is an empty slice and
// not an error, matching ConflictedFiles' own empty-never-nil contract, and that a single-head merge
// reports exactly the one SHA it is merging.
func TestMergeHeads_NoMergeLiveReturnsEmptyNeverNil(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	heads, err := repo.MergeHeads()
	if err != nil {
		t.Fatalf("MergeHeads() on a clean checkout error = %v; want nil", err)
	}
	if heads == nil || len(heads) != 0 {
		t.Errorf("MergeHeads() on a clean checkout = %v (nil: %t); want an empty, non-nil slice", heads, heads == nil)
	}

	checkoutNewBranch(t, dir, "feature")
	writeFile(t, dir, "feature.txt", "feature\n")
	commitAll(t, dir, "feature")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "main.txt", "main\n")
	commitAll(t, dir, "main advances")

	featureSHA := resolveForTest(t, repo, "feature")
	if _, err := repo.MergeStart(featureSHA, false); err != nil {
		t.Fatalf("MergeStart(%s, false) error = %v", featureSHA, err)
	}

	heads, err = repo.MergeHeads()
	if err != nil {
		t.Fatalf("MergeHeads() mid-merge error = %v; want nil", err)
	}
	if len(heads) != 1 || heads[0] != featureSHA {
		t.Errorf("MergeHeads() mid-merge = %v; want exactly [%s]", heads, featureSHA)
	}
}

// resolveForTest resolves ref through the Repo under test, failing the test on error — the fixture
// helper the MergeHeads tests name their expected SHAs with.
func resolveForTest(t *testing.T, repo *gitrepo.Repo, ref string) string {
	t.Helper()

	sha, err := repo.ResolveSHA(ref)
	if err != nil {
		t.Fatalf("ResolveSHA(%s) error = %v", ref, err)
	}
	return sha
}

// TestMergeFFOnly_AdvancesBehindCheckout asserts MergeFFOnly advances a behind checkout to its
// fast-forward-eligible target.
func TestMergeFFOnly_AdvancesBehindCheckout(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "ahead")
	writeFile(t, dir, "feat.txt", "ahead\n")
	commitAll(t, dir, "ahead edit")
	aheadSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	checkoutBranch(t, dir, "main")

	if err := repo.MergeFFOnly("ahead"); err != nil {
		t.Fatalf("MergeFFOnly(ahead) error = %v; want nil", err)
	}

	got, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if got != aheadSHA {
		t.Errorf("CurrentSHA() after MergeFFOnly(ahead) = %q; want %q", got, aheadSHA)
	}
}

// TestMergeFFOnly_FailsLoudlyOnDivergedPair asserts MergeFFOnly returns a non-nil error and leaves
// HEAD unmoved when the target has genuinely diverged — never silently discarding local commits the
// way reset --hard would.
func TestMergeFFOnly_FailsLoudlyOnDivergedPair(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	commitAll(t, dir, "base")

	checkoutNewBranch(t, dir, "diverged")
	writeFile(t, dir, "diverged.txt", "diverged\n")
	commitAll(t, dir, "diverged edit")
	checkoutBranch(t, dir, "main")
	writeFile(t, dir, "main-only.txt", "main only\n")
	commitAll(t, dir, "main-only edit")

	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	if err := repo.MergeFFOnly("diverged"); err == nil {
		t.Fatal("MergeFFOnly(diverged) on a genuinely diverged pair error = nil; want an error")
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("CurrentSHA() after failed MergeFFOnly() = %q; want unchanged %q", headAfter, headBefore)
	}
}

// TestMergeResolveSHA_BranchOriginAndFullSHA asserts ResolveSHA resolves a branch name, an
// origin/<branch> remote-tracking ref, and a full SHA all to the same 40-character SHA.
func TestMergeResolveSHA_BranchOriginAndFullSHA(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	seedPath, seedRepo := newRepoWithRemote(t, container, "seed", bareRemote)
	writeFile(t, seedPath, "a.txt", "content")
	commitAll(t, seedPath, "init")
	if err := seedRepo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	_, cloneRepo := cloneFromBare(t, container, "clone", bareRemote)

	branchSHA, err := cloneRepo.ResolveSHA("main")
	if err != nil {
		t.Fatalf("ResolveSHA(main) error = %v; want nil", err)
	}
	if len(branchSHA) != 40 {
		t.Errorf("ResolveSHA(main) = %q; want a 40-char SHA", branchSHA)
	}

	originSHA, err := cloneRepo.ResolveSHA("origin/main")
	if err != nil {
		t.Fatalf("ResolveSHA(origin/main) error = %v; want nil", err)
	}
	if originSHA != branchSHA {
		t.Errorf("ResolveSHA(origin/main) = %q; want %q (same as ResolveSHA(main))", originSHA, branchSHA)
	}

	shaSHA, err := cloneRepo.ResolveSHA(branchSHA)
	if err != nil {
		t.Fatalf("ResolveSHA(%q) error = %v; want nil", branchSHA, err)
	}
	if shaSHA != branchSHA {
		t.Errorf("ResolveSHA(%q) = %q; want %q", branchSHA, shaSHA, branchSHA)
	}
}

// TestMergeResolveSHA_UnknownRefReturnsError asserts ResolveSHA returns an error for a ref that
// resolves nowhere.
func TestMergeResolveSHA_UnknownRefReturnsError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	_, err := repo.ResolveSHA("no-such-ref-anywhere")
	if err == nil {
		t.Fatal("ResolveSHA(no-such-ref-anywhere) error = nil; want an error")
	}
}

// TestMergeStart_HostileMergeFFConfig pins finding F4: an operator's own `merge.ff` setting must not
// change what MergeStart does.
// With `merge.ff = only` the plain `git merge --no-commit <ref>` MergeStart used to run aborted every
// non-fast-forward merge with `fatal: Not possible to fast-forward`, which MergeStart classified as a
// genuine error, so every fabric merge into a target that had moved self-aborted and failed. With
// `merge.ff = false` the reverse holds: a fast-forward would fabricate a merge commit and be
// classified MergeStaged instead of MergeFastForwarded.
func TestMergeStart_HostileMergeFFConfig(t *testing.T) {
	tests := []struct {
		name        string
		mergeFF     string
		diverge     bool
		wantOutcome gitrepo.MergeOutcome
	}{
		{name: "FFOnlyDoesNotBreakARealMerge", mergeFF: "only", diverge: true, wantOutcome: gitrepo.MergeStaged},
		{name: "FFFalseDoesNotSuppressAFastForward", mergeFF: "false", diverge: false, wantOutcome: gitrepo.MergeFastForwarded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, repo := newRepo(t)
			writeFile(t, dir, "base.txt", "base\n")
			commitAll(t, dir, "base")

			checkoutNewBranch(t, dir, "feature")
			writeFile(t, dir, "feature.txt", "feature\n")
			commitAll(t, dir, "feature edit")
			checkoutBranch(t, dir, "main")
			if tt.diverge {
				writeFile(t, dir, "main.txt", "main\n")
				commitAll(t, dir, "main edit")
			}

			gitkit.MustRun(t, dir, "git", "config", "merge.ff", tt.mergeFF)

			outcome, err := repo.MergeStart("feature", false)
			if err != nil {
				t.Fatalf("MergeStart(feature, false) with merge.ff=%s error = %v; want nil — fabric pins --ff so the operator's config cannot reach it", tt.mergeFF, err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("MergeStart(feature, false) with merge.ff=%s outcome = %v; want %v", tt.mergeFF, outcome, tt.wantOutcome)
			}
		})
	}
}

// TestMergeStart_EmptyResultTree_ClassifiedStagedNotAlreadyUpToDate pins crucible round
// opus-medium-r2's finding R1: a real, non-fast-forward merge whose result tree happens to equal
// HEAD's own tree must classify as MergeStaged, not MergeAlreadyUpToDate.
// The fixture is the everyday shape a cherry-pick, backport, or duplicated hand-edit produces: the
// source branch and the current branch each reach the same content independently, so the source is
// not an ancestor of HEAD, yet merging it stages nothing and moves no HEAD. Before the fix,
// classification read only those two signals and returned MergeAlreadyUpToDate while git had
// written a live MERGE_HEAD -- so fabric reported a clean no-op, deleted its merge-state record,
// and abandoned a merge in progress that no fabric verb could then clear.
// The squash subtest is the companion direction: squash writes no MERGE_HEAD and genuinely has
// nothing to commit, so it must keep classifying as MergeAlreadyUpToDate.
func TestMergeStart_EmptyResultTree_ClassifiedStagedNotAlreadyUpToDate(t *testing.T) {
	tests := []struct {
		name           string
		squash         bool
		wantOutcome    gitrepo.MergeOutcome
		wantMergeHead  bool
		wantConcludeOK bool
	}{
		{name: "NormalMergeIsStagedWithLiveMergeHead", squash: false, wantOutcome: gitrepo.MergeStaged, wantMergeHead: true, wantConcludeOK: true},
		{name: "SquashHasNothingToDoAndStaysAlreadyUpToDate", squash: true, wantOutcome: gitrepo.MergeAlreadyUpToDate, wantMergeHead: false, wantConcludeOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, repo := newRepo(t)
			writeFile(t, dir, "shared.txt", "base\n")
			commitAll(t, dir, "base")

			checkoutNewBranch(t, dir, "feature")
			writeFile(t, dir, "shared.txt", "same change\n")
			commitAll(t, dir, "feature: same change")
			checkoutBranch(t, dir, "main")
			writeFile(t, dir, "shared.txt", "same change\n")
			commitAll(t, dir, "main: same change, reached independently")

			// The fixture is only meaningful if the source is genuinely not an ancestor: an
			// ancestor would be a real already-up-to-date merge and prove nothing.
			featureSHA, err := repo.ResolveSHA("feature")
			if err != nil {
				t.Fatalf("ResolveSHA(feature) error = %v", err)
			}
			headSHA, err := repo.CurrentSHA()
			if err != nil {
				t.Fatalf("CurrentSHA() error = %v", err)
			}
			ancestor, err := repo.IsAncestor(featureSHA, headSHA)
			if err != nil {
				t.Fatalf("IsAncestor(feature, HEAD) error = %v", err)
			}
			if ancestor {
				t.Fatal("fixture broken: feature is an ancestor of HEAD, so this is a genuine already-up-to-date merge")
			}

			headBefore, err := repo.CurrentSHA()
			if err != nil {
				t.Fatalf("CurrentSHA() error = %v", err)
			}

			outcome, err := repo.MergeStart("feature", tt.squash)
			if err != nil {
				t.Fatalf("MergeStart(feature, %t) error = %v; want nil", tt.squash, err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("MergeStart(feature, %t) outcome = %v; want %v", tt.squash, outcome, tt.wantOutcome)
			}
			if got := mergeHeadPresentOnDisk(t, dir); got != tt.wantMergeHead {
				t.Errorf("MERGE_HEAD present = %t; want %t", got, tt.wantMergeHead)
			}
			if _, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet"); code != 0 {
				t.Errorf("git diff --cached --quiet exit = %d; want 0 — this fixture's whole point is that the merge stages nothing", code)
			}
			if got, _ := repo.CurrentSHA(); got != headBefore {
				t.Errorf("CurrentSHA() = %q; want unchanged %q — this fixture's whole point is that HEAD does not move", got, headBefore)
			}

			// A MergeStaged classification is only honest if the merge really is concludable.
			if !tt.wantConcludeOK {
				return
			}
			if err := repo.MergeConclude(""); err != nil {
				t.Fatalf("MergeConclude(\"\") error = %v; want nil — a MergeStaged outcome must be concludable", err)
			}
			if mergeHeadPresentOnDisk(t, dir) {
				t.Error("MERGE_HEAD still present after MergeConclude; the merge was not concluded")
			}
			parents, _, _, err := runGit(t, dir, "rev-list", "--parents", "-n1", "HEAD")
			if err != nil {
				t.Fatalf("git rev-list --parents error = %v", err)
			}
			if got := len(strings.Fields(parents)); got != 3 {
				t.Errorf("conclude commit has %d parents (rev-list --parents fields = %d); want a two-parent merge commit", got-1, got)
			}
		})
	}
}
