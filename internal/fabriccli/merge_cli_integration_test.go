//go:build integration

// merge_cli_integration_test.go drives the "lyx fabric merge"/"merge-in" verbs end-to-end against a
// real hubforge pair, through fabriccli.RunCLIIn — the CLI-boundary counterpart to
// internal/fabricengine's own mergein_integration_test.go and merge_target_integration_test.go, which
// cover the same scenario matrix at the Go-API layer. This file asserts the JSON envelope shape at the
// CLI boundary: exit codes, the sorted "conflicts" array, "partial" staying false on a conflict
// envelope, and the fixed error text a pinned typed error surfaces through.

package fabriccli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// commitOnCurrentBranchCLI writes filename with content in dir, stages it, and commits msg on
// whatever branch is currently checked out. Named distinctly from internal/fabricengine's identical
// helper since the two packages share no test code.
func commitOnCurrentBranchCLI(t *testing.T, dir, filename, content, msg string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", filename, err)
	}
	gitkit.MustRun(t, dir, "git", "add", filename)
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", msg)
}

// commitOnBranchCLI checks out branch in dir — creating it off whatever is currently checked out when
// it does not exist yet — writes filename with content, commits msg, then switches back to whatever
// branch was checked out before.
func commitOnBranchCLI(t *testing.T, dir, branch, filename, content, msg string) {
	t.Helper()

	current := strings.TrimSpace(gitOutputCLI(t, dir, "branch", "--show-current"))
	if branchExistsLocallyCLI(t, dir, branch) {
		gitkit.MustRun(t, dir, "git", "checkout", "-q", branch)
	} else {
		gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", branch)
	}

	commitOnCurrentBranchCLI(t, dir, filename, content, msg)

	gitkit.MustRun(t, dir, "git", "checkout", "-q", current)
}

// branchExistsLocallyCLI reports whether branch already exists as a local ref in dir. Uses plain
// exec.Command rather than gitOutputCLI, which fails the test on a non-zero exit — a non-zero exit
// here is exactly the "does not exist" case, not a test failure.
func branchExistsLocallyCLI(t *testing.T, dir, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// branchAtCurrentHEADCLI creates branch in dir pointing at whatever is currently checked out, without
// checking it out or adding any commit — the already-up-to-date fixture shape.
func branchAtCurrentHEADCLI(t *testing.T, dir, branch string) {
	t.Helper()
	gitkit.MustRun(t, dir, "git", "branch", branch)
}

// setupConflictingDivergenceCLI seeds filename on dir's current branch, branches off, diverges the
// branch's copy, then diverges the current branch's own copy again — so merging branch into the
// current branch conflicts on filename.
func setupConflictingDivergenceCLI(t *testing.T, dir, branch, filename string) {
	t.Helper()

	commitOnCurrentBranchCLI(t, dir, filename, "seed content\n", "seed "+filename)
	commitOnBranchCLI(t, dir, branch, filename, "branch content\n", "diverge "+filename+" on "+branch)
	commitOnCurrentBranchCLI(t, dir, filename, "current content\n", "diverge "+filename+" on current")
}

// TestRunCLI_MergeInConflictThenContinueConcludes drives "merge-in" into a warp-side conflict,
// asserting the failure envelope's sorted "conflicts" array and "partial": false, then resolves the
// conflict in the worktree and drives "merge --continue" to conclude it.
func TestRunCLI_MergeInConflictThenContinueConcludes(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1\noutput: %s", exitCode, mergeInOut.String())
	}

	envelope := decodeResult(t, &mergeInOut)
	if ok, _ := envelope["ok"].(bool); ok {
		t.Errorf("RunCLI(merge-in) ok = true; want false on a conflict envelope")
	}
	if partial, present := envelope["partial"]; !present || partial != false {
		t.Errorf("RunCLI(merge-in) partial = %v (present=%v); want false", envelope["partial"], present)
	}
	conflictsRaw, ok := envelope["conflicts"].([]any)
	if !ok || len(conflictsRaw) == 0 {
		t.Fatalf("RunCLI(merge-in) conflicts = %v; want a non-empty array", envelope["conflicts"])
	}
	conflicts := make([]string, len(conflictsRaw))
	for i, c := range conflictsRaw {
		conflicts[i], _ = c.(string)
	}
	if !sort.StringsAreSorted(conflicts) {
		t.Errorf("RunCLI(merge-in) conflicts = %v; want lexically sorted", conflicts)
	}
	if _, present := envelope["mutations"]; !present {
		t.Errorf("RunCLI(merge-in) output missing 'mutations' key")
	}

	resolvedPath := filepath.Join(h.PrimeWorktree(), "conflict.txt")
	if err := os.WriteFile(resolvedPath, []byte("resolved content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict.txt): %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "conflict.txt")

	var continueOut bytes.Buffer
	exitCode = fabriccli.RunCLIIn(h.PrimeWorktree(), &continueOut, []string{"merge", "--continue"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(merge --continue) = %d; want 0\noutput: %s", exitCode, continueOut.String())
	}
	continueEnvelope := decodeResult(t, &continueOut)
	if ok, _ := continueEnvelope["ok"].(bool); !ok {
		t.Errorf("RunCLI(merge --continue) ok = %v; want true", continueEnvelope["ok"])
	}
	if committed, _ := continueEnvelope["committed"].(bool); !committed {
		t.Errorf("RunCLI(merge --continue) committed = %v; want true", continueEnvelope["committed"])
	}
}

// TestRunCLI_MergeInThenMergeAbortRestoresPair drives "merge-in" into a conflict, then "merge --abort",
// asserting exit 0 and both sides restored to their exact pre-merge SHAs.
func TestRunCLI_MergeInThenMergeAbortRestoresPair(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	warpStartSHA := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "rev-parse", "HEAD"))
	weftStartSHA := strings.TrimSpace(gitOutputCLI(t, h.PrimeWeft(), "rev-parse", "HEAD"))

	var mergeInOut bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1\noutput: %s", exitCode, mergeInOut.String())
	}

	var abortOut bytes.Buffer
	exitCode = fabriccli.RunCLIIn(h.PrimeWorktree(), &abortOut, []string{"merge", "--abort"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(merge --abort) = %d; want 0\noutput: %s", exitCode, abortOut.String())
	}
	abortEnvelope := decodeResult(t, &abortOut)
	if ok, _ := abortEnvelope["ok"].(bool); !ok {
		t.Errorf("RunCLI(merge --abort) ok = %v; want true", abortEnvelope["ok"])
	}

	if got := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "rev-parse", "HEAD")); got != warpStartSHA {
		t.Errorf("warp HEAD after merge --abort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := strings.TrimSpace(gitOutputCLI(t, h.PrimeWeft(), "rev-parse", "HEAD")); got != weftStartSHA {
		t.Errorf("weft HEAD after merge --abort = %q; want restored pre-merge SHA %q", got, weftStartSHA)
	}
}

// TestRunCLI_MergeCleanSquashFromTargetPair drives a clean "merge <branch> --squash" from a second,
// unrelated pair's worktree (hubforge.AddPair), the target-pair Merge verb's own shape: the source
// pair's "feature"/"feature-weft" branches are seeded on the prime pair, and Merge is invoked with cwd
// on the "target" pair instead.
func TestRunCLI_MergeCleanSquashFromTargetPair(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "target")

	commitOnBranchCLI(t, h.PrimeWorktree(), "feature", "warp-feature.txt", "warp feature\n", "warp: add feature")
	commitOnBranchCLI(t, h.PrimeWeft(), "feature-weft", "weft-feature.txt", "weft feature\n", "weft: add feature")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PairWarpWorktree("target"), &out, []string{"merge", "feature", "--squash"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(merge feature --squash) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	envelope := decodeResult(t, &out)
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Errorf("RunCLI(merge --squash) ok = %v; want true", envelope["ok"])
	}
	if committed, _ := envelope["committed"].(bool); !committed {
		t.Errorf("RunCLI(merge --squash) committed = %v; want true", envelope["committed"])
	}
}

// TestRunCLI_MergeConflictSelfAbortsWithErrMergeInRequired drives a "merge <branch>" on a target pair
// that would conflict, asserting the envelope's error text is *fabricengine.ErrMergeInRequired's
// fixed message and the target pair is left unchanged.
func TestRunCLI_MergeConflictSelfAbortsWithErrMergeInRequired(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "target")

	commitOnCurrentBranchCLI(t, h.PairWarpWorktree("target"), "conflict.txt", "target content\n", "target: seed conflict.txt")
	commitOnBranchCLI(t, h.PrimeWorktree(), "feature", "conflict.txt", "feature content\n", "feature: diverge conflict.txt")
	commitOnBranchCLI(t, h.PrimeWeft(), "feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")

	warpBefore := strings.TrimSpace(gitOutputCLI(t, h.PairWarpWorktree("target"), "rev-parse", "HEAD"))
	weftBefore := strings.TrimSpace(gitOutputCLI(t, h.PairWeftSibling("target"), "rev-parse", "HEAD"))

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PairWarpWorktree("target"), &out, []string{"merge", "feature"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge feature) [would conflict] = %d; want 1\noutput: %s", exitCode, out.String())
	}

	envelope := decodeResult(t, &out)
	errMsg, _ := envelope["error"].(string)
	wantErr := `fabricengine: merge produced conflicts and was aborted; run "lyx fabric merge-in" in the source branch's own worktree first, then retry`
	if errMsg != wantErr {
		t.Errorf("RunCLI(merge) [would conflict] error = %q; want %q", errMsg, wantErr)
	}

	if got := strings.TrimSpace(gitOutputCLI(t, h.PairWarpWorktree("target"), "rev-parse", "HEAD")); got != warpBefore {
		t.Errorf("target warp HEAD after self-aborted merge = %q; want unchanged %q", got, warpBefore)
	}
	if got := strings.TrimSpace(gitOutputCLI(t, h.PairWeftSibling("target"), "rev-parse", "HEAD")); got != weftBefore {
		t.Errorf("target weft HEAD after self-aborted merge = %q; want unchanged %q", got, weftBefore)
	}
}

// TestRunCLI_MergeNonexistentBranchReportsSourceNotFound drives "merge nonexistent-branch",
// asserting the aggregated *fabricengine.MergeGuardError text names the
// source-not-found reason — neither a local "nonexistent-branch" nor "nonexistent-branch-weft"
// exists on a freshly built hub.
// mergeReasonNotFabricManaged is no longer reachable from resolveMergeSources (see its doc comment
// in mergeguards.go): the weft side is not a merge participant, so an absent weft counterpart no
// longer contributes a second reason to the aggregated error.
func TestRunCLI_MergeNonexistentBranchReportsSourceNotFound(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge", "nonexistent-branch"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge nonexistent-branch) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	envelope := decodeResult(t, &out)
	errMsg, _ := envelope["error"].(string)
	if !strings.Contains(errMsg, "source branch not found") {
		t.Errorf("RunCLI(merge nonexistent-branch) error = %q; want substring %q", errMsg, "source branch not found")
	}
	if strings.Contains(errMsg, "source branch is not fabric-managed") {
		t.Errorf("RunCLI(merge nonexistent-branch) error = %q; must not claim the weft counterpart is not fabric-managed (unreachable now the weft side is not a merge participant)", errMsg)
	}
}

// TestRunCLI_MergeInAlreadyUpToDate drives "merge-in" against a branch already an ancestor of both
// sides' HEADs, asserting exit 0 and "already_up_to_date": true.
func TestRunCLI_MergeInAlreadyUpToDate(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	branchAtCurrentHEADCLI(t, h.PrimeWorktree(), "feature")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge-in", "feature"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(merge-in feature) [already up to date] = %d; want 0\noutput: %s", exitCode, out.String())
	}

	envelope := decodeResult(t, &out)
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Errorf("RunCLI(merge-in) [already up to date] ok = %v; want true", envelope["ok"])
	}
	if alreadyUpToDate, _ := envelope["already_up_to_date"].(bool); !alreadyUpToDate {
		t.Errorf("RunCLI(merge-in) [already up to date] already_up_to_date = %v; want true", envelope["already_up_to_date"])
	}
}

// TestRunCLI_MergeRejectsFlagsItWouldOtherwiseIgnore pins finding F8: "merge --abort -m <msg>" used
// to be accepted and the message silently discarded, since -m is MergeAbort's no-op while being
// MergeContinue's real message override. A caller cannot tell a discarded message from a landed one,
// so the pre-flight rejects it the same way it already rejects --squash alongside --abort.
// Driven through RunCLIIn against a real pair rather than a bare cobra tree, so the check is proven
// to sit ahead of the engine call.
func TestRunCLI_MergeRejectsFlagsItWouldOtherwiseIgnore(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "MessageWithAbort",
			args:    []string{"merge", "--abort", "-m", "ignored message"},
			wantErr: "usage: -m cannot be combined with --abort",
		},
		{
			name:    "SquashWithAbort",
			args:    []string{"merge", "--abort", "--squash"},
			wantErr: "usage: --squash cannot be combined with --continue or --abort",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.NewHub(t, ".")

			var out bytes.Buffer
			exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, tt.args)
			if exitCode != 1 {
				t.Fatalf("RunCLI(%v) = %d; want 1\noutput: %s", tt.args, exitCode, out.String())
			}
			envelope := decodeResult(t, &out)
			if ok, _ := envelope["ok"].(bool); ok {
				t.Errorf("RunCLI(%v) ok = true; want false", tt.args)
			}
			if got, _ := envelope["error"].(string); got != tt.wantErr {
				t.Errorf("RunCLI(%v) error = %q; want %q", tt.args, got, tt.wantErr)
			}
		})
	}
}

// TestRunCLI_MergeContinueAcceptsMessage pins the other half of finding F8's rule: -m stays
// meaningful for --continue, so the rejection above must be scoped to --abort alone.
func TestRunCLI_MergeContinueAcceptsMessage(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1 (conflict)\noutput: %s", exitCode, mergeInOut.String())
	}

	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "conflict.txt")

	const wantSubject = "crucible: chosen merge message"
	var continueOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &continueOut, []string{"merge", "--continue", "-m", wantSubject}); exitCode != 0 {
		t.Fatalf("RunCLI(merge --continue -m) = %d; want 0\noutput: %s", exitCode, continueOut.String())
	}
	if got := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "log", "-1", "--format=%s")); got != wantSubject {
		t.Errorf("conclude-commit subject = %q; want %q — -m must still reach MergeContinue", got, wantSubject)
	}
}

// TestRunCLI_MergeStageResolvesAWarpSideConflict pins the merge-stage -> merge --continue flow
// end-to-end against an ordinary warp-side conflict.
//
// This supersedes an earlier version of this test that conflicted on a path behind a wired junction
// (living in the weft checkout, reachable from the visible worktree only through the junction, where
// plain `git add` refuses to stage it): that premise no longer holds, since the weft side is not a
// merge participant (mergeguards.go's resolveMergeSources doc comment) and can no longer contribute
// a conflicting path to merge-in at all. merge-stage's junction-staging behavior is therefore
// unreachable in practice for a real merge conflict — see the conclude-and-conflict-plumbing-stays
// Shared Decision — and this test instead pins the ordinary, still-reachable flow: resolve the
// content, prove `merge --continue` still refuses before staging, stage with `merge-stage`, and
// conclude.
func TestRunCLI_MergeStageResolvesAWarpSideConflict(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	const conflictPath = "merge-stage-clash.txt"
	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", conflictPath)
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1 (a conflict envelope)\noutput: %s", exitCode, mergeInOut.String())
	}
	envelope := decodeResult(t, &mergeInOut)
	conflictsRaw, ok := envelope["conflicts"].([]any)
	if !ok || len(conflictsRaw) != 1 {
		t.Fatalf("RunCLI(merge-in) conflicts = %v; want exactly the one conflict this fixture creates", envelope["conflicts"])
	}
	reportedPath, _ := conflictsRaw[0].(string)
	if reportedPath != conflictPath {
		t.Fatalf("RunCLI(merge-in) conflicts = [%q]; want [%q]", reportedPath, conflictPath)
	}

	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), reportedPath), []byte("resolved content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", reportedPath, err)
	}

	// MergeContinue still refuses, because content edits do not clear an index entry.
	var earlyContinueOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &earlyContinueOut, []string{"merge", "--continue"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge --continue) before staging = %d; want 1\noutput: %s", exitCode, earlyContinueOut.String())
	}
	if got := earlyContinueOut.String(); !strings.Contains(got, "unresolved conflicts remain") {
		t.Errorf("RunCLI(merge --continue) before staging output = %q; want the unresolved-conflicts guard reason", got)
	}

	var stageOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &stageOut, []string{"merge-stage", reportedPath}); exitCode != 0 {
		t.Fatalf("RunCLI(merge-stage %s) = %d; want 0\noutput: %s", reportedPath, exitCode, stageOut.String())
	}
	stageEnvelope := decodeResult(t, &stageOut)
	if ok, _ := stageEnvelope["ok"].(bool); !ok {
		t.Errorf("RunCLI(merge-stage) ok = %v; want true", stageEnvelope["ok"])
	}
	if _, present := stageEnvelope["mutations"]; !present {
		t.Errorf("RunCLI(merge-stage) output missing 'mutations' key; every mutating verb's envelope carries it")
	}
	if partial, present := stageEnvelope["partial"]; !present || partial != false {
		t.Errorf("RunCLI(merge-stage) partial = %v (present=%v); want false", stageEnvelope["partial"], present)
	}

	var continueOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &continueOut, []string{"merge", "--continue"}); exitCode != 0 {
		t.Fatalf("RunCLI(merge --continue) after merge-stage = %d; want 0\noutput: %s", exitCode, continueOut.String())
	}
	continueEnvelope := decodeResult(t, &continueOut)
	if committed, _ := continueEnvelope["committed"].(bool); !committed {
		t.Errorf("RunCLI(merge --continue) committed = %v; want true", continueEnvelope["committed"])
	}
}

// TestRunCLI_MergeStageRejectsAPathThatIsNotConflicted covers merge-stage's refusal path at the CLI
// boundary: a path conflicted on neither side is an error rather than a silent skip, and nothing is
// staged, so a typo cannot leave the merge half-resolved.
func TestRunCLI_MergeStageRejectsAPathThatIsNotConflicted(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1 (a conflict envelope)\noutput: %s", exitCode, mergeInOut.String())
	}

	const bogusPath = "not-conflicted-at-all.txt"
	var stageOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &stageOut, []string{"merge-stage", "conflict.txt", bogusPath}); exitCode != 1 {
		t.Fatalf("RunCLI(merge-stage conflict.txt %s) = %d; want 1\noutput: %s", bogusPath, exitCode, stageOut.String())
	}
	if got := stageOut.String(); !strings.Contains(got, bogusPath) {
		t.Errorf("RunCLI(merge-stage) error output = %q; want it to name %q", got, bogusPath)
	}

	// The good path in the same call must NOT have been staged: the verb partitions every path before
	// staging anything, so one bad path fails the whole call.
	statusCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	statusCmd.Dir = h.PrimeWorktree()
	out, err := statusCmd.Output()
	if err != nil {
		t.Fatalf("git diff --name-only --diff-filter=U: %v", err)
	}
	if !strings.Contains(string(out), "conflict.txt") {
		t.Errorf("conflict.txt is no longer unmerged after a refused merge-stage; want the whole call to have staged nothing")
	}
}

// TestRunCLI_MergeContinuePartialStagingListsTheRemainingPaths pins the one flow where the
// unresolved-conflicts refusal has to say more than that some conflict exists: the operator staged
// SOME of the reported paths and not all of them.
// Both conflicting paths are warp-side here — the weft side is not a merge participant
// (mergeguards.go's resolveMergeSources doc comment) and can no longer contribute a conflicting path
// to merge-in at all, so a strict-subset staging scenario needs two warp-side conflicts rather than
// one warp-side and one weft-side path.
// The test also pins the key SEPARATION, not merely the presence of the list: "conflicts" is the
// documented discriminator between a conflict result and a hard failure, so a refusal must report the
// remaining paths under "unresolved" and must NOT carry a "conflicts" key at all.
func TestRunCLI_MergeContinuePartialStagingListsTheRemainingPaths(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict-a.txt")
	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict-b.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1 (a conflict envelope)\noutput: %s", exitCode, mergeInOut.String())
	}
	mergeInEnvelope := decodeResult(t, &mergeInOut)
	if got := stringSliceField(t, mergeInEnvelope, "conflicts"); len(got) != 2 {
		t.Fatalf("RunCLI(merge-in) conflicts = %v; want both warp-side paths conflicted, or this test cannot stage a strict subset", got)
	}

	// Resolve and stage conflict-a.txt only, leaving conflict-b.txt outstanding.
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "conflict-a.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution: %v", err)
	}
	var stageOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &stageOut, []string{"merge-stage", "conflict-a.txt"}); exitCode != 0 {
		t.Fatalf("RunCLI(merge-stage conflict-a.txt) = %d; want 0\noutput: %s", exitCode, stageOut.String())
	}

	var continueOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &continueOut, []string{"merge", "--continue"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge --continue) with one path still unstaged = %d; want 1\noutput: %s", exitCode, continueOut.String())
	}
	continueEnvelope := decodeResult(t, &continueOut)
	if errMsg, _ := continueEnvelope["error"].(string); !strings.Contains(errMsg, "unresolved conflicts remain") {
		t.Fatalf("RunCLI(merge --continue) error = %q; want the unresolved-conflicts refusal", errMsg)
	}
	unresolved := stringSliceField(t, continueEnvelope, "unresolved")
	if len(unresolved) != 1 || unresolved[0] != "conflict-b.txt" {
		t.Errorf("RunCLI(merge --continue) unresolved = %v; want exactly [conflict-b.txt] — the path the operator has left to resolve", unresolved)
	}
	if _, present := continueEnvelope["conflicts"]; present {
		t.Errorf("RunCLI(merge --continue) carries a %q key; want the remaining paths under \"unresolved\" only, so \"conflicts\" stays the conflict-result discriminator", "conflicts")
	}
}

// stringSliceField reads envelope[key] as a JSON array of strings, failing the test when it is absent
// or not an array of strings.
func stringSliceField(t *testing.T, envelope map[string]any, key string) []string {
	t.Helper()

	raw, ok := envelope[key].([]any)
	if !ok {
		t.Fatalf("envelope[%q] = %v; want an array", key, envelope[key])
	}
	values := make([]string, len(raw))
	for i, entry := range raw {
		values[i], ok = entry.(string)
		if !ok {
			t.Fatalf("envelope[%q][%d] = %v; want a string", key, i, entry)
		}
	}
	return values
}

// TestRunCLI_MergeStageRequiresAtLeastOnePath pins the verb's arity at the CLI boundary: with no
// paths it must refuse rather than succeed vacuously, since a caller that passed nothing meant to
// pass something.
func TestRunCLI_MergeStageRequiresAtLeastOnePath(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge-stage"}); exitCode == 0 {
		t.Errorf("RunCLI(merge-stage) with no paths = 0; want a refusal\noutput: %s", out.String())
	}
}

// TestRunCLI_MergeStageEchoesEachPathOnce pins the success envelope's staged echo: a path passed
// twice in one call stages fine (the engine tolerates the duplicate) but must appear ONCE in
// "staged" — an envelope claiming two stagings for one path reports something that did not happen
// twice.
func TestRunCLI_MergeStageEchoesEachPathOnce(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"}); exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1 (a conflict envelope)\noutput: %s", exitCode, mergeInOut.String())
	}
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution: %v", err)
	}

	var stageOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &stageOut, []string{"merge-stage", "conflict.txt", "conflict.txt"}); exitCode != 0 {
		t.Fatalf("RunCLI(merge-stage conflict.txt conflict.txt) = %d; want 0\noutput: %s", exitCode, stageOut.String())
	}
	envelope := decodeResult(t, &stageOut)
	staged, _ := envelope["staged"].([]any)
	if len(staged) != 1 {
		t.Errorf("RunCLI(merge-stage) staged = %v; want exactly one entry for the duplicated path", envelope["staged"])
	}
	if got, _ := staged[0].(string); len(staged) == 1 && got != "conflict.txt" {
		t.Errorf("RunCLI(merge-stage) staged[0] = %q; want %q", got, "conflict.txt")
	}
}
