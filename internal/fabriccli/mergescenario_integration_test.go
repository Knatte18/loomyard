//go:build integration

// mergescenario_integration_test.go drives the "lyx fabric merge", "merge-in", "merge-stage" and "status" verbs end-to-end against one real hubforge pair, through fabriccli.RunCLIIn, as an ordered scenario.
// It is the CLI-boundary counterpart to internal/fabricengine's own merge integration tests, which cover the same matrix at the Go-API layer:
// it asserts the JSON envelope shape at the CLI boundary — exit codes, the sorted "conflicts" array, "partial" staying false on a conflict envelope, the fixed error text a pinned typed error surfaces through — and the optional "warnings" key that reports a mid-run parent merge while webster state sits under the anchor.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// commitOnBranchCLI checks out branch in dir — creating it off whatever is currently checked out when
// it does not exist yet — writes filename with content, commits msg, then switches back to whatever
// branch was checked out before.
func commitOnBranchCLI(t *testing.T, dir, branch, filename, content, msg string) {
	t.Helper()

	current := strings.TrimSpace(gitOutputCLI(t, dir, "branch", "--show-current"))
	if gitkit.BranchExists(t, dir, branch) {
		gitkit.Git(t, dir, "checkout", "-q", branch)
	} else {
		gitkit.Git(t, dir, "checkout", "-q", "-b", branch)
	}

	gitkit.CommitFile(t, dir, filename, content, msg)

	gitkit.Git(t, dir, "checkout", "-q", current)
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

	gitkit.CommitFile(t, dir, filename, "seed content\n", "seed "+filename)
	commitOnBranchCLI(t, dir, branch, filename, "branch content\n", "diverge "+filename+" on "+branch)
	gitkit.CommitFile(t, dir, filename, "current content\n", "diverge "+filename+" on current")
}

// divergeConflicting makes merging branch into the prime pair conflict on filename, and cuts the weft counterpart of branch at the weft's current HEAD.
func divergeConflicting(t *testing.T, h *hubforge.Hub, branch, filename string) {
	t.Helper()

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), branch, filename)
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), branch+"-weft")
}

// divergeCleanly gives branch and the prime pair's current branch each a commit on a different file, and cuts the weft counterpart of branch at the weft's current HEAD.
func divergeCleanly(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	commitOnBranchCLI(t, h.PrimeWorktree(), branch, branch+".txt", "feature\n", "feature: add file")
	gitkit.CommitFile(t, h.PrimeWorktree(), "main-side-"+branch+".txt", "main\n", "main: add file")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), branch+"-weft")
}

// mergeInExpectingConflict runs "merge-in branch" from the prime worktree, requires the conflict exit code and returns the decoded envelope.
func mergeInExpectingConflict(t *testing.T, h *hubforge.Hub, branch string) envelope.Envelope {
	t.Helper()

	code, output := runFabric(t, h.PrimeWorktree(), "merge-in", branch)
	if code != 1 {
		t.Fatalf("RunCLI(merge-in %s) = %d; want 1 (a conflict envelope)\noutput: %s", branch, code, output)
	}
	return envelope.Decode(t, output)
}

// abortParkedMerge runs "merge --abort" from the prime worktree so the next step starts with no merge parked.
func abortParkedMerge(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	if code, output := runFabric(t, h.PrimeWorktree(), "merge", "--abort"); code != 0 {
		t.Fatalf("RunCLI(merge --abort) = %d; want 0\noutput: %s", code, output)
	}
}

// resolveConflictFile overwrites filename in the prime worktree with content.
func resolveConflictFile(t *testing.T, h *hubforge.Hub, filename, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), filename), []byte(content), 0o644); err != nil {
		t.Fatalf("write resolution %s: %v", filename, err)
	}
}

// stringSliceField reads fields[key] as a JSON array of strings, failing the test when it is absent
// or not an array of strings.
func stringSliceField(t *testing.T, fields map[string]any, key string) []string {
	t.Helper()

	raw, ok := fields[key].([]any)
	if !ok {
		t.Fatalf("envelope[%q] = %v; want an array", key, fields[key])
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

// seedWebsterState writes state.json, plus outcome.yaml when outcome is non-empty, under the hub's webster directory and commits it on the prime weft,
// so the pair is clean when the weft feature branch is cut.
func seedWebsterState(t *testing.T, h *hubforge.Hub, outcome string) {
	t.Helper()

	dir := websterengine.Dir(h.Location.AnchorPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(state.json): %v", err)
	}
	if outcome != "" {
		if err := os.WriteFile(filepath.Join(dir, "outcome.yaml"), []byte(outcome), 0o644); err != nil {
			t.Fatalf("WriteFile(outcome.yaml): %v", err)
		}
	}
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-q", "-m", "seed webster state")
}

// runMergeIn runs "merge-in branch" from the prime worktree and returns the exit code and decoded envelope.
func runMergeIn(t *testing.T, h *hubforge.Hub, branch string) (int, envelope.Envelope) {
	t.Helper()

	code, output := runFabric(t, h.PrimeWorktree(), "merge-in", branch)
	return code, envelope.Decode(t, output)
}

func assertOneWebsterWarning(t *testing.T, env envelope.Envelope) {
	t.Helper()

	raw, present := env.Raw["warnings"]
	if !present {
		t.Fatalf("envelope has no warnings key: %v", env)
	}
	warnings, ok := raw.([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("warnings = %v; want exactly one entry", raw)
	}
	s, _ := warnings[0].(string)
	for _, want := range []string{"Webster is mid-run in this worktree", "lyx webster record-batch"} {
		if !strings.Contains(s, want) {
			t.Errorf("warnings[0] = %q; want it to contain %q", s, want)
		}
	}
}

func assertNoWarnings(t *testing.T, env envelope.Envelope) {
	t.Helper()

	if _, present := env.Raw["warnings"]; present {
		t.Errorf("envelope carries a warnings key = %v; want none", env.Raw["warnings"])
	}
}

// TestRunCLI_MergeScenario runs every merge-family check over one hub.
// Each step cuts its own branches and conflict files, and a step that parks a merge aborts it before returning, so steps do not disturb each other.
// The webster-warning steps come last and run in the order of the webster state they need:
// none, then a state file alone, then a finished outcome, then a malformed outcome; the state only accumulates.
// The scenario calls t.Parallel as a whole; no step does, because they share the one hub.
func TestRunCLI_MergeScenario(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"MergeInConflictThenContinueConcludes", func(t *testing.T) {
			// merge-in into a warp-side conflict yields the failure envelope's sorted "conflicts" array and "partial": false; resolving in the worktree and "merge --continue" concludes it.
			divergeConflicting(t, h, "continue-feature", "continue-conflict.txt")

			mergeInEnv := mergeInExpectingConflict(t, h, "continue-feature")
			if mergeInEnv.OK {
				t.Errorf("RunCLI(merge-in) ok = true; want false on a conflict envelope")
			}
			if mergeInEnv.Partial == nil || *mergeInEnv.Partial {
				t.Errorf("RunCLI(merge-in) partial = %v; want present and false", mergeInEnv.Partial)
			}
			conflictsRaw, ok := mergeInEnv.Raw["conflicts"].([]any)
			if !ok || len(conflictsRaw) == 0 {
				t.Fatalf("RunCLI(merge-in) conflicts = %v; want a non-empty array", mergeInEnv.Raw["conflicts"])
			}
			conflicts := make([]string, len(conflictsRaw))
			for i, c := range conflictsRaw {
				conflicts[i], _ = c.(string)
			}
			if !sort.StringsAreSorted(conflicts) {
				t.Errorf("RunCLI(merge-in) conflicts = %v; want lexically sorted", conflicts)
			}
			if _, present := mergeInEnv.Raw["mutations"]; !present {
				t.Errorf("RunCLI(merge-in) output missing 'mutations' key")
			}

			resolveConflictFile(t, h, "continue-conflict.txt", "resolved content\n")
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "continue-conflict.txt")

			code, output := runFabric(t, h.PrimeWorktree(), "merge", "--continue")
			if code != 0 {
				t.Fatalf("RunCLI(merge --continue) = %d; want 0\noutput: %s", code, output)
			}
			continueEnv := envelope.RequireOK(t, output)
			if committed, _ := continueEnv.Raw["committed"].(bool); !committed {
				t.Errorf("RunCLI(merge --continue) committed = %v; want true", continueEnv.Raw["committed"])
			}
		}},
		{"MergeInThenMergeAbortRestoresPair", func(t *testing.T) {
			// merge-in into a conflict, then "merge --abort", leaves both sides at their exact pre-merge SHAs.
			divergeConflicting(t, h, "abort-feature", "abort-conflict.txt")

			warpStartSHA := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "rev-parse", "HEAD"))
			weftStartSHA := strings.TrimSpace(gitOutputCLI(t, h.PrimeWeft(), "rev-parse", "HEAD"))

			mergeInExpectingConflict(t, h, "abort-feature")

			code, output := runFabric(t, h.PrimeWorktree(), "merge", "--abort")
			if code != 0 {
				t.Fatalf("RunCLI(merge --abort) = %d; want 0\noutput: %s", code, output)
			}
			envelope.RequireOK(t, output)

			if got := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "rev-parse", "HEAD")); got != warpStartSHA {
				t.Errorf("warp HEAD after merge --abort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
			}
			if got := strings.TrimSpace(gitOutputCLI(t, h.PrimeWeft(), "rev-parse", "HEAD")); got != weftStartSHA {
				t.Errorf("weft HEAD after merge --abort = %q; want restored pre-merge SHA %q", got, weftStartSHA)
			}
		}},
		{"MergeContinueAcceptsMessage", func(t *testing.T) {
			// -m stays meaningful for --continue, so the rejection of "merge --abort -m" is scoped to --abort alone.
			divergeConflicting(t, h, "message-feature", "message-conflict.txt")
			mergeInExpectingConflict(t, h, "message-feature")

			resolveConflictFile(t, h, "message-conflict.txt", "resolved\n")
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "message-conflict.txt")

			const wantSubject = "crucible: chosen merge message"
			if code, output := runFabric(t, h.PrimeWorktree(), "merge", "--continue", "-m", wantSubject); code != 0 {
				t.Fatalf("RunCLI(merge --continue -m) = %d; want 0\noutput: %s", code, output)
			}
			if got := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "log", "-1", "--format=%s")); got != wantSubject {
				t.Errorf("conclude-commit subject = %q; want %q — -m must still reach MergeContinue", got, wantSubject)
			}
		}},
		{"MergeStageResolvesAWarpSideConflict", func(t *testing.T) {
			// The merge-stage -> merge --continue flow end-to-end against an ordinary warp-side conflict:
			// resolve the content, prove "merge --continue" still refuses before staging, stage with "merge-stage", and conclude.
			const conflictPath = "merge-stage-clash.txt"
			divergeConflicting(t, h, "stage-feature", conflictPath)

			mergeInEnv := mergeInExpectingConflict(t, h, "stage-feature")
			conflictsRaw, ok := mergeInEnv.Raw["conflicts"].([]any)
			if !ok || len(conflictsRaw) != 1 {
				t.Fatalf("RunCLI(merge-in) conflicts = %v; want exactly the one conflict this fixture creates", mergeInEnv.Raw["conflicts"])
			}
			reportedPath, _ := conflictsRaw[0].(string)
			if reportedPath != conflictPath {
				t.Fatalf("RunCLI(merge-in) conflicts = [%q]; want [%q]", reportedPath, conflictPath)
			}

			resolveConflictFile(t, h, reportedPath, "resolved content\n")

			// MergeContinue still refuses, because content edits do not clear an index entry.
			earlyCode, earlyOutput := runFabric(t, h.PrimeWorktree(), "merge", "--continue")
			if earlyCode != 1 {
				t.Fatalf("RunCLI(merge --continue) before staging = %d; want 1\noutput: %s", earlyCode, earlyOutput)
			}
			if !strings.Contains(earlyOutput, "unresolved conflicts remain") {
				t.Errorf("RunCLI(merge --continue) before staging output = %q; want the unresolved-conflicts guard reason", earlyOutput)
			}

			stageCode, stageOutput := runFabric(t, h.PrimeWorktree(), "merge-stage", reportedPath)
			if stageCode != 0 {
				t.Fatalf("RunCLI(merge-stage %s) = %d; want 0\noutput: %s", reportedPath, stageCode, stageOutput)
			}
			stageEnv := envelope.RequireOK(t, stageOutput)
			if _, present := stageEnv.Raw["mutations"]; !present {
				t.Errorf("RunCLI(merge-stage) output missing 'mutations' key; every mutating verb's envelope carries it")
			}
			if stageEnv.Partial == nil || *stageEnv.Partial {
				t.Errorf("RunCLI(merge-stage) partial = %v; want present and false", stageEnv.Partial)
			}

			continueCode, continueOutput := runFabric(t, h.PrimeWorktree(), "merge", "--continue")
			if continueCode != 0 {
				t.Fatalf("RunCLI(merge --continue) after merge-stage = %d; want 0\noutput: %s", continueCode, continueOutput)
			}
			continueEnv := envelope.Decode(t, continueOutput)
			if committed, _ := continueEnv.Raw["committed"].(bool); !committed {
				t.Errorf("RunCLI(merge --continue) committed = %v; want true", continueEnv.Raw["committed"])
			}
		}},
		{"MergeStageRejectsAPathThatIsNotConflicted", func(t *testing.T) {
			// A path conflicted on neither side is an error rather than a silent skip, and nothing is staged, so a typo cannot leave the merge half-resolved.
			divergeConflicting(t, h, "reject-feature", "reject-conflict.txt")
			mergeInExpectingConflict(t, h, "reject-feature")
			defer abortParkedMerge(t, h)

			const bogusPath = "not-conflicted-at-all.txt"
			code, output := runFabric(t, h.PrimeWorktree(), "merge-stage", "reject-conflict.txt", bogusPath)
			if code != 1 {
				t.Fatalf("RunCLI(merge-stage reject-conflict.txt %s) = %d; want 1\noutput: %s", bogusPath, code, output)
			}
			if !strings.Contains(output, bogusPath) {
				t.Errorf("RunCLI(merge-stage) error output = %q; want it to name %q", output, bogusPath)
			}

			// The good path in the same call must NOT have been staged: the verb partitions every path before staging anything, so one bad path fails the whole call.
			statusCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
			statusCmd.Dir = h.PrimeWorktree()
			out, err := statusCmd.Output()
			if err != nil {
				t.Fatalf("git diff --name-only --diff-filter=U: %v", err)
			}
			if !strings.Contains(string(out), "reject-conflict.txt") {
				t.Errorf("reject-conflict.txt is no longer unmerged after a refused merge-stage; want the whole call to have staged nothing")
			}
		}},
		{"MergeContinuePartialStagingListsTheRemainingPaths", func(t *testing.T) {
			// The operator staged SOME of the reported paths and not all of them: the unresolved-conflicts refusal reports the remaining paths under "unresolved" and carries no "conflicts" key at all, since "conflicts" is the documented discriminator between a conflict result and a hard failure.
			// Both conflicting paths are warp-side: the weft side is not a merge participant.
			setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "partial-feature", "partial-a.txt")
			setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "partial-feature", "partial-b.txt")
			branchAtCurrentHEADCLI(t, h.PrimeWeft(), "partial-feature-weft")

			mergeInEnv := mergeInExpectingConflict(t, h, "partial-feature")
			defer abortParkedMerge(t, h)
			if got := stringSliceField(t, mergeInEnv.Raw, "conflicts"); len(got) != 2 {
				t.Fatalf("RunCLI(merge-in) conflicts = %v; want both warp-side paths conflicted, or this test cannot stage a strict subset", got)
			}

			// Resolve and stage partial-a.txt only, leaving partial-b.txt outstanding.
			resolveConflictFile(t, h, "partial-a.txt", "resolved\n")
			if code, output := runFabric(t, h.PrimeWorktree(), "merge-stage", "partial-a.txt"); code != 0 {
				t.Fatalf("RunCLI(merge-stage partial-a.txt) = %d; want 0\noutput: %s", code, output)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "merge", "--continue")
			if code != 1 {
				t.Fatalf("RunCLI(merge --continue) with one path still unstaged = %d; want 1\noutput: %s", code, output)
			}
			continueEnv := envelope.RequireErr(t, output, "unresolved conflicts remain")
			unresolved := stringSliceField(t, continueEnv.Raw, "unresolved")
			if len(unresolved) != 1 || unresolved[0] != "partial-b.txt" {
				t.Errorf("RunCLI(merge --continue) unresolved = %v; want exactly [partial-b.txt] — the path the operator has left to resolve", unresolved)
			}
			if _, present := continueEnv.Raw["conflicts"]; present {
				t.Errorf("RunCLI(merge --continue) carries a %q key; want the remaining paths under \"unresolved\" only, so \"conflicts\" stays the conflict-result discriminator", "conflicts")
			}
		}},
		{"MergeStageEchoesEachPathOnce", func(t *testing.T) {
			// A path passed twice in one call stages fine (the engine tolerates the duplicate) but appears ONCE in "staged": an envelope claiming two stagings for one path reports something that did not happen twice.
			divergeConflicting(t, h, "echo-feature", "echo-conflict.txt")
			mergeInExpectingConflict(t, h, "echo-feature")
			defer abortParkedMerge(t, h)
			resolveConflictFile(t, h, "echo-conflict.txt", "resolved\n")

			code, output := runFabric(t, h.PrimeWorktree(), "merge-stage", "echo-conflict.txt", "echo-conflict.txt")
			if code != 0 {
				t.Fatalf("RunCLI(merge-stage echo-conflict.txt echo-conflict.txt) = %d; want 0\noutput: %s", code, output)
			}
			stageEnv := envelope.Decode(t, output)
			staged, _ := stageEnv.Raw["staged"].([]any)
			if len(staged) != 1 {
				t.Fatalf("RunCLI(merge-stage) staged = %v; want exactly one entry for the duplicated path", stageEnv.Raw["staged"])
			}
			if got, _ := staged[0].(string); got != "echo-conflict.txt" {
				t.Errorf("RunCLI(merge-stage) staged[0] = %q; want %q", got, "echo-conflict.txt")
			}
		}},
		{"StatusReportsMergeInProgressWhileAMergeIsParked", func(t *testing.T) {
			// status run from the pair holding a parked conflicted merge reports "merge_in_progress" present and true — the assertion that proves the field reads the real record rather than a hardcoded constant.
			divergeConflicting(t, h, "status-feature", "status-conflict.txt")
			mergeInExpectingConflict(t, h, "status-feature")
			defer abortParkedMerge(t, h)

			code, output := runFabric(t, h.PrimeWorktree(), "status")
			if code != 0 {
				t.Fatalf("RunCLI(status) = %d; want 0\noutput: %s", code, output)
			}

			result := envelope.Decode(t, output)
			inProgress, present := result.Raw["merge_in_progress"]
			if !present {
				t.Fatalf("RunCLI(status) output missing 'merge_in_progress' key; got %v", result)
			}
			if inProgress != true {
				t.Errorf("RunCLI(status) merge_in_progress = %v; want true", inProgress)
			}
		}},
		{"MergeInAlreadyUpToDate", func(t *testing.T) {
			// merge-in against a branch already an ancestor of both sides' HEADs exits 0 with "already_up_to_date": true.
			branchAtCurrentHEADCLI(t, h.PrimeWorktree(), "uptodate-feature")
			branchAtCurrentHEADCLI(t, h.PrimeWeft(), "uptodate-feature-weft")

			code, output := runFabric(t, h.PrimeWorktree(), "merge-in", "uptodate-feature")
			if code != 0 {
				t.Fatalf("RunCLI(merge-in) [already up to date] = %d; want 0\noutput: %s", code, output)
			}

			result := envelope.RequireOK(t, output)
			if alreadyUpToDate, _ := result.Raw["already_up_to_date"].(bool); !alreadyUpToDate {
				t.Errorf("RunCLI(merge-in) [already up to date] already_up_to_date = %v; want true", result.Raw["already_up_to_date"])
			}
		}},
		{"MergeCleanSquashFromTargetPair", func(t *testing.T) {
			// A clean "merge <branch> --squash" from a second, unrelated pair's worktree, the target-pair Merge verb's own shape: the source pair's branches are seeded on the prime pair, and Merge is invoked with cwd on the target pair instead.
			hubforge.AddPair(t, h, "squash-target")

			commitOnBranchCLI(t, h.PrimeWorktree(), "squash-feature", "warp-feature.txt", "warp feature\n", "warp: add feature")
			commitOnBranchCLI(t, h.PrimeWeft(), "squash-feature-weft", "weft-feature.txt", "weft feature\n", "weft: add feature")

			code, output := runFabric(t, h.PairWarpWorktree("squash-target"), "merge", "squash-feature", "--squash")
			if code != 0 {
				t.Fatalf("RunCLI(merge squash-feature --squash) = %d; want 0\noutput: %s", code, output)
			}

			result := envelope.RequireOK(t, output)
			if committed, _ := result.Raw["committed"].(bool); !committed {
				t.Errorf("RunCLI(merge --squash) committed = %v; want true", result.Raw["committed"])
			}
		}},
		{"MergeConflictSelfAbortsWithErrMergeInRequired", func(t *testing.T) {
			// A "merge <branch>" on a target pair that would conflict reports the fixed ErrMergeInRequired message and leaves the target pair unchanged.
			hubforge.AddPair(t, h, "abort-target")

			gitkit.CommitFile(t, h.PairWarpWorktree("abort-target"), "selfabort-conflict.txt", "target content\n", "target: seed selfabort-conflict.txt")
			commitOnBranchCLI(t, h.PrimeWorktree(), "selfabort-feature", "selfabort-conflict.txt", "feature content\n", "feature: diverge selfabort-conflict.txt")
			commitOnBranchCLI(t, h.PrimeWeft(), "selfabort-feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")

			warpBefore := strings.TrimSpace(gitOutputCLI(t, h.PairWarpWorktree("abort-target"), "rev-parse", "HEAD"))
			weftBefore := strings.TrimSpace(gitOutputCLI(t, h.PairWeftSibling("abort-target"), "rev-parse", "HEAD"))

			code, output := runFabric(t, h.PairWarpWorktree("abort-target"), "merge", "selfabort-feature")
			if code != 1 {
				t.Fatalf("RunCLI(merge selfabort-feature) [would conflict] = %d; want 1\noutput: %s", code, output)
			}

			result := envelope.RequireErr(t, output, "")
			wantErr := `fabricengine: merge produced conflicts and was aborted; run "lyx fabric merge-in" in the source branch's own worktree first, then retry`
			if result.Error != wantErr {
				t.Errorf("RunCLI(merge) [would conflict] error = %q; want %q", result.Error, wantErr)
			}

			if got := strings.TrimSpace(gitOutputCLI(t, h.PairWarpWorktree("abort-target"), "rev-parse", "HEAD")); got != warpBefore {
				t.Errorf("target warp HEAD after self-aborted merge = %q; want unchanged %q", got, warpBefore)
			}
			if got := strings.TrimSpace(gitOutputCLI(t, h.PairWeftSibling("abort-target"), "rev-parse", "HEAD")); got != weftBefore {
				t.Errorf("target weft HEAD after self-aborted merge = %q; want unchanged %q", got, weftBefore)
			}
		}},
		{"MergeIn_NoWarningsWithoutWebsterState", func(t *testing.T) {
			divergeCleanly(t, h, "nowebster-feature")

			code, env := runMergeIn(t, h, "nowebster-feature")
			if code != 0 {
				t.Fatalf("merge-in = %d; want 0\nenvelope: %v", code, env)
			}
			assertNoWarnings(t, env)
		}},
		{"MergeIn_WarnsWhileWebsterInFlight", func(t *testing.T) {
			// Seeds webster state with no outcome; the following webster steps rely on it staying in place.
			seedWebsterState(t, h, "")
			divergeCleanly(t, h, "inflight-feature")

			code, env := runMergeIn(t, h, "inflight-feature")
			if code != 0 {
				t.Fatalf("merge-in = %d; want 0\nenvelope: %v", code, env)
			}
			if committed, _ := env.Raw["committed"].(bool); !committed {
				t.Errorf("committed = %v; want true", env.Raw["committed"])
			}
			assertOneWebsterWarning(t, env)
		}},
		{"MergeIn_ConflictWarnsWhileWebsterInFlight", func(t *testing.T) {
			divergeConflicting(t, h, "inflight-conflict-feature", "inflight-conflict.txt")
			defer abortParkedMerge(t, h)

			code, env := runMergeIn(t, h, "inflight-conflict-feature")
			if code != 1 {
				t.Fatalf("merge-in = %d; want 1\nenvelope: %v", code, env)
			}
			if conflicts, _ := env.Raw["conflicts"].([]any); len(conflicts) == 0 {
				t.Errorf("conflicts = %v; want a non-empty array", env.Raw["conflicts"])
			}
			assertOneWebsterWarning(t, env)
		}},
		{"MergeIn_NoWarningsWhenAlreadyUpToDate", func(t *testing.T) {
			branchAtCurrentHEADCLI(t, h.PrimeWorktree(), "inflight-uptodate-feature")
			branchAtCurrentHEADCLI(t, h.PrimeWeft(), "inflight-uptodate-feature-weft")

			code, env := runMergeIn(t, h, "inflight-uptodate-feature")
			if code != 0 {
				t.Fatalf("merge-in = %d; want 0\nenvelope: %v", code, env)
			}
			if upToDate, _ := env.Raw["already_up_to_date"].(bool); !upToDate {
				t.Errorf("already_up_to_date = %v; want true", env.Raw["already_up_to_date"])
			}
			assertNoWarnings(t, env)
		}},
		{"MergeIn_NoWarningsWhenWebsterDone", func(t *testing.T) {
			seedWebsterState(t, h, "outcome: done\nstuck_reason: null\nbatches_done: 3\n")
			divergeCleanly(t, h, "done-feature")

			code, env := runMergeIn(t, h, "done-feature")
			if code != 0 {
				t.Fatalf("merge-in = %d; want 0\nenvelope: %v", code, env)
			}
			assertNoWarnings(t, env)
		}},
		{"MergeIn_MalformedOutcomeDegradesToNoWarning", func(t *testing.T) {
			seedWebsterState(t, h, "outcome: [unterminated\n")
			divergeCleanly(t, h, "malformed-feature")

			code, env := runMergeIn(t, h, "malformed-feature")
			if code != 0 {
				t.Fatalf("merge-in = %d; want 0\nenvelope: %v", code, env)
			}
			if committed, _ := env.Raw["committed"].(bool); !committed {
				t.Errorf("committed = %v; want true", env.Raw["committed"])
			}
			assertNoWarnings(t, env)
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
