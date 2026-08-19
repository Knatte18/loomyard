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
