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
	wantErr := "fabricengine: merge produced conflicts and was aborted; run MergeIn in the source branch's worktree first, then retry"
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

// TestRunCLI_MergeNonexistentBranchReportsAggregatedGuardError drives "merge nonexistent-branch",
// asserting the aggregated *fabricengine.MergeGuardError text names both the source-not-found and
// not-fabric-managed reasons — neither a local "nonexistent-branch" nor "nonexistent-branch-weft"
// exists on a freshly built hub.
func TestRunCLI_MergeNonexistentBranchReportsAggregatedGuardError(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge", "nonexistent-branch"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge nonexistent-branch) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	envelope := decodeResult(t, &out)
	errMsg, _ := envelope["error"].(string)
	for _, reason := range []string{"source branch not found", "source branch is not fabric-managed"} {
		if !strings.Contains(errMsg, reason) {
			t.Errorf("RunCLI(merge nonexistent-branch) error = %q; want substring %q", errMsg, reason)
		}
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
