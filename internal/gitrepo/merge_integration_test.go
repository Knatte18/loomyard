//go:build integration

// merge_integration_test.go covers the merge primitives in merge.go against real git repositories,
// reusing gitrepo_test.go's fixture helpers (newRepo, writeFile, commitAll, runGit) and, for the
// remote-tracking-ref ResolveSHA case, push_test.go's newBareRemote/cloneFromBare helpers.

package gitrepo_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// nonASCIIName is a filename outside core.quotepath's default ASCII set.
const nonASCIIName = "ä-nöte.txt"

// commitOnNewBranch creates branch from the current HEAD, commits content to file on it, and returns to main.
func commitOnNewBranch(t *testing.T, dir, branch, file, content string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-b", branch)
	writeFile(t, dir, file, content)
	commitAll(t, dir, branch+" edit")
	gitkit.MustRun(t, dir, "git", "checkout", "main")
}

// commitOnMain commits content to file on main.
func commitOnMain(t *testing.T, dir, file, content string) {
	t.Helper()

	writeFile(t, dir, file, content)
	commitAll(t, dir, "main edit of "+file)
}

// stagedChanges reports whether the index differs from HEAD.
func stagedChanges(t *testing.T, dir string) bool {
	t.Helper()

	_, _, code, _ := runGit(t, dir, "diff", "--cached", "--quiet")
	return code == 1
}

// TestMergePrimitives drives MergeStart, MergeConclude, MergeFFOnly, ConflictedFiles, MergeHeadPresent and MergeHeads through one repository.
// The repository carries a base commit with base.txt, shared.txt and a non-ASCII file.
// Every step starts by returning main to that base commit, clearing any merge state and untracked files, and forks branches with names of its own, so the steps run serially in one order but depend on nothing an earlier step leaves behind.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestMergePrimitives(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	writeFile(t, dir, "shared.txt", "base\n")
	writeFile(t, dir, nonASCIIName, "base\n")
	commitAll(t, dir, "base")
	baseSHA := requireCurrentSHA(t, repo)

	// toBase returns main to the base commit with a clean tree and the default merge.ff.
	toBase := func(t *testing.T) {
		t.Helper()

		gitkit.MustRun(t, dir, "git", "checkout", "-f", "main")
		gitkit.MustRun(t, dir, "git", "reset", "--hard", baseSHA)
		gitkit.MustRun(t, dir, "git", "clean", "-fdq")
		gitkit.MustRun(t, dir, "git", "config", "merge.ff", "true")
	}

	branchCount := 0
	newBranchName := func() string {
		branchCount++
		return fmt.Sprintf("feature-%d", branchCount)
	}

	// conflict forks a branch and main that both edit file, leaving main checked out.
	conflict := func(t *testing.T, file string) string {
		t.Helper()

		branch := newBranchName()
		commitOnNewBranch(t, dir, branch, file, "feature\n")
		commitOnMain(t, dir, file, "main\n")
		return branch
	}

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"MergeStart rejects a leading-dash ref before any git spawn", func(t *testing.T) {
			_, err := repo.MergeStart("--squash", false)
			if !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Fatalf("MergeStart(--squash, false) error = %v; want errors.Is(err, ErrInvalidSHA)", err)
			}
		}},
		{"MergeBase of two diverged branches is their fork commit", func(t *testing.T) {
			toBase(t)
			branch := conflict(t, "shared.txt")

			got, err := repo.MergeBase(resolveForTest(t, repo, branch), requireCurrentSHA(t, repo))
			if err != nil {
				t.Fatalf("MergeBase(%s, main) error = %v; want nil", branch, err)
			}
			if got != baseSHA {
				t.Errorf("MergeBase(%s, main) = %s; want the fork commit %s", branch, got, baseSHA)
			}
		}},
		{"ConflictedFiles and MergeHeads return an empty non-nil slice on a clean tree", func(t *testing.T) {
			toBase(t)

			conflicted, err := repo.ConflictedFiles()
			if err != nil {
				t.Fatalf("ConflictedFiles() error = %v; want nil", err)
			}
			if conflicted == nil || len(conflicted) != 0 {
				t.Errorf("ConflictedFiles() = %#v; want an empty, non-nil slice", conflicted)
			}

			heads, err := repo.MergeHeads()
			if err != nil {
				t.Fatalf("MergeHeads() on a clean checkout error = %v; want nil", err)
			}
			if heads == nil || len(heads) != 0 {
				t.Errorf("MergeHeads() on a clean checkout = %v (nil: %t); want an empty, non-nil slice", heads, heads == nil)
			}
		}},
		// Two branches edit the same line: the conflicted outcome carries a nil error (a conflict is a result, not an error), leaves unmerged index entries and a live MERGE_HEAD, and ConflictedFiles names the path.
		// ResetHard to the pre-merge SHA is the abort mechanism: it must clear both MergeHeadPresent and ConflictedFiles.
		{"a conflicted merge reports its conflict and is aborted by ResetHard", func(t *testing.T) {
			toBase(t)
			branch := conflict(t, "shared.txt")
			preMergeSHA := requireCurrentSHA(t, repo)

			outcome, err := repo.MergeStart(branch, false)
			if err != nil {
				t.Fatalf("MergeStart(%s, false) error = %v; want nil (conflict is a result, not an error)", branch, err)
			}
			if outcome != gitrepo.MergeConflicted {
				t.Fatalf("MergeStart(%s, false) outcome = %v; want MergeConflicted", branch, outcome)
			}

			got, err := repo.ConflictedFiles()
			if err != nil {
				t.Fatalf("ConflictedFiles() error = %v; want nil", err)
			}
			if len(got) != 1 || got[0] != "shared.txt" {
				t.Errorf("ConflictedFiles() = %v; want [shared.txt]", got)
			}

			stdout, _, code, err := runGit(t, dir, "ls-files", "--unmerged")
			if err != nil || code != 0 {
				t.Fatalf("git ls-files --unmerged = (code %d, %v)", code, err)
			}
			if strings.TrimSpace(stdout) == "" {
				t.Error("git ls-files --unmerged = \"\"; want unmerged entries present")
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
			conflicted, err := repo.ConflictedFiles()
			if err != nil {
				t.Fatalf("ConflictedFiles() error = %v; want nil", err)
			}
			if len(conflicted) != 0 {
				t.Errorf("ConflictedFiles() after ResetHard() = %v; want empty", conflicted)
			}
		}},
		// ConflictedFiles returns the raw path bytes for a conflicted filename outside core.quotepath's default ASCII set, never git's C-quoted form (`"\303\244.txt"`, quotes included) that `--name-only` without `-z` emits — the quoted form is not a real worktree path, and fabricengine's visible-tree mapping misclassified a mappable conflict as unmergeable on it.
		{"ConflictedFiles returns a non-ASCII path raw, never quoted", func(t *testing.T) {
			toBase(t)
			branch := conflict(t, nonASCIIName)

			outcome, err := repo.MergeStart(branch, false)
			if err != nil || outcome != gitrepo.MergeConflicted {
				t.Fatalf("MergeStart(%s, false) = (%v, %v); want (MergeConflicted, nil)", branch, outcome, err)
			}

			got, err := repo.ConflictedFiles()
			if err != nil {
				t.Fatalf("ConflictedFiles() error = %v; want nil", err)
			}
			if len(got) != 1 || got[0] != nonASCIIName {
				t.Errorf("ConflictedFiles() = %q; want [%q] — the raw bytes, not git's C-quoted rendering", got, nonASCIIName)
			}
		}},
		// A non-conflicting, non-fast-forward merge stages the change and leaves HEAD unmoved and the merge uncommitted; MergeConclude with a non-empty message commits with exactly that message, and MergeHeadPresent is false afterwards.
		{"a clean merge stages uncommitted and MergeConclude commits the explicit message", func(t *testing.T) {
			toBase(t)
			branch := newBranchName()
			commitOnNewBranch(t, dir, branch, "feat.txt", "feature\n")
			commitOnMain(t, dir, "base.txt", "base\nmain edit\n")
			headBefore := requireCurrentSHA(t, repo)

			if outcome, err := repo.MergeStart(branch, false); err != nil || outcome != gitrepo.MergeStaged {
				t.Fatalf("MergeStart(%s, false) = (%v, %v); want (MergeStaged, nil)", branch, outcome, err)
			}
			if !stagedChanges(t, dir) {
				t.Error("git diff --cached --quiet exit != 1; want something staged")
			}
			if headAfter := requireCurrentSHA(t, repo); headAfter != headBefore {
				t.Errorf("CurrentSHA() after MergeStart(clean-staged) = %q; want unchanged %q", headAfter, headBefore)
			}

			const msg = "explicit merge message"
			if err := repo.MergeConclude(msg); err != nil {
				t.Fatalf("MergeConclude(%q) error = %v; want nil", msg, err)
			}
			got, _, code, err := runGit(t, dir, "log", "-1", "--format=%s")
			if err != nil || code != 0 {
				t.Fatalf("git log -1 --format=%%s = (code %d, %v)", code, err)
			}
			if strings.TrimSpace(got) != msg {
				t.Errorf("commit message after MergeConclude(%q) = %q; want %q", msg, strings.TrimSpace(got), msg)
			}
			present, err := repo.MergeHeadPresent()
			if err != nil {
				t.Fatalf("MergeHeadPresent() error = %v; want nil", err)
			}
			if present {
				t.Error("MergeHeadPresent() after MergeConclude() = true; want false")
			}
		}},
		// MergeConclude with an empty message takes git's own prepared MERGE_MSG rather than opening an editor.
		{"MergeConclude with an empty message uses the prepared message", func(t *testing.T) {
			toBase(t)
			branch := newBranchName()
			commitOnNewBranch(t, dir, branch, "feat.txt", "feature\n")
			commitOnMain(t, dir, "base.txt", "base\nmain edit\n")

			if outcome, err := repo.MergeStart(branch, false); err != nil || outcome != gitrepo.MergeStaged {
				t.Fatalf("MergeStart(%s, false) = (%v, %v); want (MergeStaged, nil)", branch, outcome, err)
			}
			if err := repo.MergeConclude(""); err != nil {
				t.Fatalf("MergeConclude(\"\") error = %v; want nil", err)
			}

			got, _, code, err := runGit(t, dir, "log", "-1", "--format=%s")
			if err != nil || code != 0 {
				t.Fatalf("git log -1 --format=%%s = (code %d, %v)", code, err)
			}
			if !strings.Contains(strings.TrimSpace(got), "Merge branch '"+branch+"'") {
				t.Errorf("commit message after MergeConclude(\"\") = %q; want git's prepared MERGE_MSG (mentioning the merged branch)", strings.TrimSpace(got))
			}
		}},
		{"MergeStart outcomes", func(t *testing.T) {
			// An operator's own `merge.ff` setting must not change what MergeStart does.
			// With `merge.ff = only` the plain `git merge --no-commit <ref>` MergeStart used to run aborted every non-fast-forward merge with `fatal: Not possible to fast-forward`, which MergeStart classified as a genuine error, so every fabric merge into a target that had moved self-aborted and failed.
			// With `merge.ff = false` the reverse holds: a fast-forward would fabricate a merge commit and be classified MergeStaged instead of MergeFastForwarded.
			tests := []struct {
				name          string
				squash        bool
				featureEdits  bool
				mainEdits     bool
				mergeFF       string
				wantOutcome   gitrepo.MergeOutcome
				wantHeadMoved bool
				wantStaged    bool
				wantMergeHead bool
			}{
				{name: "a clean merge stages without moving HEAD", featureEdits: true, mainEdits: true, wantOutcome: gitrepo.MergeStaged, wantStaged: true, wantMergeHead: true},
				// The documented ff-defeats---no-commit behaviour: a fast-forward-eligible merge moves HEAD, stages nothing and leaves no MERGE_HEAD, even under --no-commit.
				{name: "a fast-forward moves HEAD and stages nothing", featureEdits: true, wantOutcome: gitrepo.MergeFastForwarded, wantHeadMoved: true},
				{name: "an ancestor source is already up to date", wantOutcome: gitrepo.MergeAlreadyUpToDate},
				{name: "a squash of a diverging branch stages without MERGE_HEAD", squash: true, featureEdits: true, mainEdits: true, wantOutcome: gitrepo.MergeStaged, wantStaged: true},
				// Squash never fast-forwards, regardless of ff eligibility.
				{name: "a squash of a fast-forwardable branch stages without moving HEAD", squash: true, featureEdits: true, wantOutcome: gitrepo.MergeStaged, wantStaged: true},
				{name: "merge.ff=only does not break a real merge", featureEdits: true, mainEdits: true, mergeFF: "only", wantOutcome: gitrepo.MergeStaged, wantStaged: true, wantMergeHead: true},
				{name: "merge.ff=false does not suppress a fast-forward", featureEdits: true, mergeFF: "false", wantOutcome: gitrepo.MergeFastForwarded, wantHeadMoved: true},
			}
			for _, tt := range tests {
				if !t.Run(tt.name, func(t *testing.T) {
					toBase(t)
					branch := newBranchName()
					if tt.featureEdits {
						commitOnNewBranch(t, dir, branch, "feature.txt", "feature\n")
					} else {
						gitkit.MustRun(t, dir, "git", "branch", branch)
					}
					if tt.mainEdits {
						commitOnMain(t, dir, "main.txt", "main\n")
					}
					if tt.mergeFF != "" {
						gitkit.MustRun(t, dir, "git", "config", "merge.ff", tt.mergeFF)
					}
					headBefore := requireCurrentSHA(t, repo)
					featureSHA := resolveForTest(t, repo, branch)

					outcome, err := repo.MergeStart(branch, tt.squash)
					if err != nil {
						t.Fatalf("MergeStart(%s, %t) with merge.ff=%q error = %v; want nil — fabric pins --ff so the operator's config cannot reach it", branch, tt.squash, tt.mergeFF, err)
					}
					if outcome != tt.wantOutcome {
						t.Fatalf("MergeStart(%s, %t) with merge.ff=%q outcome = %v; want %v", branch, tt.squash, tt.mergeFF, outcome, tt.wantOutcome)
					}

					headAfter := requireCurrentSHA(t, repo)
					if tt.wantHeadMoved {
						if headAfter != featureSHA {
							t.Errorf("CurrentSHA() after fast-forward = %q; want %q (feature's tip)", headAfter, featureSHA)
						}
					} else if headAfter != headBefore {
						t.Errorf("CurrentSHA() after the merge = %q; want unchanged %q", headAfter, headBefore)
					}
					if got := stagedChanges(t, dir); got != tt.wantStaged {
						t.Errorf("staged changes = %t; want %t", got, tt.wantStaged)
					}
					if got := mergeHeadPresent(t, dir); got != tt.wantMergeHead {
						t.Errorf("MERGE_HEAD present = %t; want %t (a squash never sets MERGE_HEAD)", got, tt.wantMergeHead)
					}
				}) {
					return
				}
			}
		}},
		// A real, non-fast-forward merge whose result tree happens to equal HEAD's own tree must classify as MergeStaged, not MergeAlreadyUpToDate.
		// The fixture is the everyday shape a cherry-pick, backport, or duplicated hand-edit produces: the source branch and the current branch each reach the same content independently, so the source is not an ancestor of HEAD, yet merging it stages nothing and moves no HEAD.
		// Before the fix, classification read only those two signals and returned MergeAlreadyUpToDate while git had written a live MERGE_HEAD -- so fabric reported a clean no-op, deleted its merge-state record, and abandoned a merge in progress that no fabric verb could then clear.
		// The squash row is the companion direction: squash writes no MERGE_HEAD and genuinely has nothing to commit, so it must keep classifying as MergeAlreadyUpToDate.
		{"an empty result tree is classified staged, not already up to date", func(t *testing.T) {
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
				if !t.Run(tt.name, func(t *testing.T) {
					toBase(t)
					branch := newBranchName()
					commitOnNewBranch(t, dir, branch, "shared.txt", "same change\n")
					commitOnMain(t, dir, "shared.txt", "same change\n")

					// The fixture is only meaningful if the source is genuinely not an ancestor: an
					// ancestor would be a real already-up-to-date merge and prove nothing.
					featureSHA := resolveForTest(t, repo, branch)
					headBefore := requireCurrentSHA(t, repo)
					ancestor, err := repo.IsAncestor(featureSHA, headBefore)
					if err != nil {
						t.Fatalf("IsAncestor(feature, HEAD) error = %v", err)
					}
					if ancestor {
						t.Fatal("fixture broken: feature is an ancestor of HEAD, so this is a genuine already-up-to-date merge")
					}

					outcome, err := repo.MergeStart(branch, tt.squash)
					if err != nil {
						t.Fatalf("MergeStart(%s, %t) error = %v; want nil", branch, tt.squash, err)
					}
					if outcome != tt.wantOutcome {
						t.Errorf("MergeStart(%s, %t) outcome = %v; want %v", branch, tt.squash, outcome, tt.wantOutcome)
					}
					if got := mergeHeadPresent(t, dir); got != tt.wantMergeHead {
						t.Errorf("MERGE_HEAD present = %t; want %t", got, tt.wantMergeHead)
					}
					if stagedChanges(t, dir) {
						t.Error("something staged; want nothing — this fixture's whole point is that the merge stages nothing")
					}
					if got := requireCurrentSHA(t, repo); got != headBefore {
						t.Errorf("CurrentSHA() = %q; want unchanged %q — this fixture's whole point is that HEAD does not move", got, headBefore)
					}

					// A MergeStaged classification is only honest if the merge really is concludable.
					if !tt.wantConcludeOK {
						return
					}
					if err := repo.MergeConclude(""); err != nil {
						t.Fatalf("MergeConclude(\"\") error = %v; want nil — a MergeStaged outcome must be concludable", err)
					}
					if mergeHeadPresent(t, dir) {
						t.Error("MERGE_HEAD still present after MergeConclude; the merge was not concluded")
					}
					parents, _, _, err := runGit(t, dir, "rev-list", "--parents", "-n1", "HEAD")
					if err != nil {
						t.Fatalf("git rev-list --parents error = %v", err)
					}
					if got := len(strings.Fields(parents)); got != 3 {
						t.Errorf("conclude commit has %d parents (rev-list --parents fields = %d); want a two-parent merge commit", got-1, got)
					}
				}) {
					return
				}
			}
		}},
		// A single-head merge reports exactly the one SHA it is merging.
		{"MergeHeads reports the one head of a single-head merge", func(t *testing.T) {
			toBase(t)
			branch := newBranchName()
			commitOnNewBranch(t, dir, branch, "feature.txt", "feature\n")
			commitOnMain(t, dir, "main.txt", "main\n")

			featureSHA := resolveForTest(t, repo, branch)
			if _, err := repo.MergeStart(featureSHA, false); err != nil {
				t.Fatalf("MergeStart(%s, false) error = %v", featureSHA, err)
			}

			heads, err := repo.MergeHeads()
			if err != nil {
				t.Fatalf("MergeHeads() mid-merge error = %v; want nil", err)
			}
			if len(heads) != 1 || heads[0] != featureSHA {
				t.Errorf("MergeHeads() mid-merge = %v; want exactly [%s]", heads, featureSHA)
			}
		}},
		// The reason MergeHeads reads MERGE_HEAD rather than shelling `git rev-parse --verify --quiet MERGE_HEAD` a second time: for a two-head merge, rev-parse reports only the FIRST head, so a caller comparing that single answer against an expected SHA accepts `git merge --no-commit <expected> <decoy>` as if it were `git merge <expected>`.
		// The step asserts BOTH halves — the full list MergeHeads returns, and the truncated answer the rev-parse spelling gives for the same state — so rewriting MergeHeads onto rev-parse fails here rather than silently reintroducing the first-entry-only read.
		{"MergeHeads enumerates every head of an octopus", func(t *testing.T) {
			toBase(t)
			first, second := newBranchName(), newBranchName()
			commitOnNewBranch(t, dir, first, "first.txt", "first\n")
			commitOnNewBranch(t, dir, second, "second.txt", "second\n")
			commitOnMain(t, dir, "main.txt", "main\n")

			firstSHA := resolveForTest(t, repo, first)
			secondSHA := resolveForTest(t, repo, second)
			gitkit.MustRun(t, dir, "git", "merge", "--no-commit", "--no-ff", first, second)

			// Precondition, asserted rather than assumed: the rev-parse spelling really does truncate here, or this step would pass against the very implementation it exists to forbid.
			stdout, _, code, err := runGit(t, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
			if err != nil || code != 0 {
				t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD = (code %d, %v); want it to succeed on the octopus", code, err)
			}
			if got := strings.TrimSpace(stdout); got != firstSHA {
				t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD = %q; want the FIRST head %q — the truncation this step is built on is not present", got, firstSHA)
			}

			heads, err := repo.MergeHeads()
			if err != nil {
				t.Fatalf("MergeHeads() error = %v; want nil", err)
			}
			if len(heads) != 2 || heads[0] != firstSHA || heads[1] != secondSHA {
				t.Errorf("MergeHeads() = %v; want [%s %s] — every head, in git's own order", heads, firstSHA, secondSHA)
			}
		}},
		{"MergeFFOnly advances a behind checkout", func(t *testing.T) {
			toBase(t)
			branch := newBranchName()
			commitOnNewBranch(t, dir, branch, "feat.txt", "ahead\n")
			aheadSHA := resolveForTest(t, repo, branch)

			if err := repo.MergeFFOnly(branch); err != nil {
				t.Fatalf("MergeFFOnly(%s) error = %v; want nil", branch, err)
			}
			if got := requireCurrentSHA(t, repo); got != aheadSHA {
				t.Errorf("CurrentSHA() after MergeFFOnly(%s) = %q; want %q", branch, got, aheadSHA)
			}
		}},
		// MergeFFOnly returns a non-nil error and leaves HEAD unmoved when the target has genuinely diverged — never silently discarding local commits the way reset --hard would.
		{"MergeFFOnly fails loudly on a diverged pair", func(t *testing.T) {
			toBase(t)
			branch := newBranchName()
			commitOnNewBranch(t, dir, branch, "diverged.txt", "diverged\n")
			commitOnMain(t, dir, "main-only.txt", "main only\n")
			headBefore := requireCurrentSHA(t, repo)

			if err := repo.MergeFFOnly(branch); err == nil {
				t.Fatalf("MergeFFOnly(%s) on a genuinely diverged pair error = nil; want an error", branch)
			}
			if headAfter := requireCurrentSHA(t, repo); headAfter != headBefore {
				t.Errorf("CurrentSHA() after failed MergeFFOnly() = %q; want unchanged %q", headAfter, headBefore)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// TestMergeResolveSHA covers ResolveSHA against a clone of a bare remote: a branch name, an origin/<branch> remote-tracking ref and a full SHA all resolve to the same 40-character SHA, and a ref that resolves nowhere returns an error.
func TestMergeResolveSHA(t *testing.T) {
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	seedPath, seedRepo := newRepoWithRemote(t, container, "seed", bareRemote)
	writeFile(t, seedPath, "a.txt", "content")
	commitAll(t, seedPath, "init")
	if err := seedRepo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	_, cloneRepo := cloneFromBare(t, container, "clone", bareRemote)

	t.Run("a branch, an origin ref and a full sha resolve to the same sha", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("an unknown ref returns an error", func(t *testing.T) {
		t.Parallel()

		if _, err := cloneRepo.ResolveSHA("no-such-ref-anywhere"); err == nil {
			t.Fatal("ResolveSHA(no-such-ref-anywhere) error = nil; want an error")
		}
	})
}

// resolveForTest resolves ref through the Repo under test, failing the test on error — the fixture helper the merge steps name their expected SHAs with.
func resolveForTest(t *testing.T, repo *gitrepo.Repo, ref string) string {
	t.Helper()

	sha, err := repo.ResolveSHA(ref)
	if err != nil {
		t.Fatalf("ResolveSHA(%s) error = %v", ref, err)
	}
	return sha
}
