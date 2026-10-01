// gitwrap.go implements webster's own git-query helpers over internal/gitrepo: headSHA captures a
// batch's start-SHA and the report cross-check's actual HEAD, and dirty is the half-done-work
// signal.
// refuseMidMerge and reconcileReportHead are the read-only probes record-batch and recover-batch share:
// the first refuses while a git merge is in progress, the second accepts a HEAD that is only clean parent merges past the report's head_sha.
// The suspect-path probes (ignoredPath, worktreePathDiffers, worktreeBlob, commitBlob, treePathsWithBlob) and otherWorktrees are the read-only evidence queries accept-audit, recovery and fresh runs share;
// shaExists and isAncestor are the evidence-base probes runEvidenceBases picks its commits with.
// Per the Shared Decision git-verification-via-gitrepo, every helper here goes through gitrepo.Repo except dirty and the read-only probes gitrepo has no method for (all the suspect-path probes and otherWorktrees).
// gitrepo exposes no method for them, so they wrap the checked gitexec.Run directly, the carved-out exception the decision names;
// they are kept in this one file so no other webster file runs git.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// headSHA returns worktree's current HEAD commit SHA via gitrepo.Repo.
// An unborn HEAD surfaces as gitrepo.ErrNoCommits.
func headSHA(worktree string) (string, error) {
	sha, err := gitrepo.New(worktree).CurrentSHA()
	if err != nil {
		return "", fmt.Errorf("websterengine: head sha in %s: %w", worktree, err)
	}
	return sha, nil
}

// refuseMidMerge returns an error when worktree has a git merge in progress, and nil otherwise.
// It asks git whether MERGE_HEAD resolves rather than statting a file, because a linked worktree keeps MERGE_HEAD in its per-worktree git dir.
func refuseMidMerge(worktree string) error {
	present, err := gitrepo.New(worktree).MergeHeadPresent()
	if err != nil {
		return fmt.Errorf("websterengine: probe merge in progress in %s: %w", worktree, err)
	}
	if present {
		return fmt.Errorf("webster: worktree %s has a git merge in progress; conclude it first "+
			"(`lyx fabric merge --continue` / `lyx fabric merge --abort` in a hub, "+
			"`git merge --continue` / `git merge --abort` for a standalone run)", worktree)
	}
	return nil
}

// ParentBranchFunc names the branch the run merges its parent in from, resolved only when a merge commit needs checking.
// A nil ParentBranchFunc means the run has no parent branch, so no merge commit is ever accepted.
type ParentBranchFunc func() (string, error)

// reconcileReportHead cross-checks a report's head_sha against worktree's actual HEAD, tolerating clean parent merges only.
// HEAD equal to reportHead is the unchanged fast path and returns an empty warning.
// Otherwise it walks HEAD's first-parent chain, comparing each commit with reportHead BEFORE looking at its parents —
// so a reportHead that is itself a merge commit is accepted, not stepped past.
// A commit that is not reportHead must be a clean parent merge to be walked over:
// exactly two parents, the second reachable from parentBranch's local or origin tip, and a tree equal to the conflict-free merge of the two.
// A non-merge commit, the root, or a merge failing any check ends the walk in refusal, and so does any error while checking.
// The rule keeps the audit sound: the batch is recorded at reportHead, so content a merge adds beyond a clean parent merge would bypass the audited delta.
// On acceptance the warning names subject, both heads and every walked merge SHA in walk order.
// A refusal's way forward is worded for record-batch and recover-batch (reportHeadRefusal).
func reconcileReportHead(worktree, reportHead, subject string, parentBranch ParentBranchFunc) (warning string, err error) {
	return reconcileHead(worktree, reportHead, subject, parentBranch, reportHeadRefusal)
}

// headRefusal words a reconcileHead refusal's way forward for one caller.
type headRefusal struct {
	// head names the commit HEAD must move back to, such as "the report's head_sha".
	head string
	// rerun is what to re-run once HEAD is back on it, such as "re-run this verb".
	rerun string
	// redoMerge is when to redo a parent merge-in that did not qualify, such as "after the batch is recorded".
	redoMerge string
}

// reportHeadRefusal is the wording for a verb that records a batch at a report's head_sha.
var reportHeadRefusal = headRefusal{head: "the report's head_sha", rerun: "re-run this verb", redoMerge: "after the batch is recorded"}

// reconcileHead is reconcileReportHead with the refusal's way forward worded by refusal.
func reconcileHead(worktree, reportHead, subject string, parentBranch ParentBranchFunc, refusal headRefusal) (warning string, err error) {
	head, err := headSHA(worktree)
	if err != nil {
		return "", err
	}
	if head == reportHead {
		return "", nil
	}

	repo := gitrepo.New(worktree)
	var merges []string
	var parentTips []string
	for cur := head; ; {
		if cur == reportHead {
			return fmt.Sprintf("webster: %s: head_sha %q differs from the worktree's HEAD %q; only merge commits (%s) sit between them, so the batch is recorded at the report's head_sha %q",
				subject, reportHead, head, strings.Join(merges, ", "), reportHead), nil
		}
		parents, err := repo.CommitParents(cur)
		if err != nil {
			return "", fmt.Errorf("websterengine: walk first-parent chain from %s in %s: %w", head, worktree, err)
		}
		if len(parents) < 2 {
			return "", fmt.Errorf("webster: %s: head_sha %q does not match the worktree's actual HEAD %q; "+
				"only merge commits, such as a parent merge-in, may sit between a fork's reported head and HEAD; "+
				"way forward: move HEAD back to %s %s with git, then %s",
				subject, reportHead, head, refusal.head, reportHead, refusal.rerun)
		}
		// The parent tips are resolved once, on the first merge the walk meets.
		if parentTips == nil {
			if parentTips, err = resolveParentTips(repo, parentBranch); err != nil {
				return "", parentMergeRefusal(subject, reportHead, head, cur, err.Error(), refusal)
			}
		}
		if reason := parentMergeRejection(repo, cur, parents, parentTips); reason != "" {
			return "", parentMergeRefusal(subject, reportHead, head, cur, reason, refusal)
		}
		merges = append(merges, cur)
		cur = parents[0]
	}
}

// resolveParentTips resolves the run's parent branch to every tip a parent merge-in may have merged from:
// the local branch and its origin remote-tracking ref, whichever exist, mirroring fabric merge-in's own source resolution.
// A nil parentBranch, an empty branch name, or a branch with neither ref resolving is an error, so no merge is accepted.
func resolveParentTips(repo *gitrepo.Repo, parentBranch ParentBranchFunc) ([]string, error) {
	if parentBranch == nil {
		return nil, fmt.Errorf("the run has no known parent branch")
	}
	branch, err := parentBranch()
	if err != nil {
		return nil, fmt.Errorf("resolve the run's parent branch: %v", err)
	}
	if branch == "" {
		return nil, fmt.Errorf("the run has no known parent branch")
	}
	var tips []string
	for _, ref := range []string{branch, "origin/" + branch} {
		if sha, err := repo.ResolveSHA(ref); err == nil {
			tips = append(tips, sha)
		}
	}
	if len(tips) == 0 {
		return nil, fmt.Errorf("the run's parent branch %q resolves neither locally nor on origin", branch)
	}
	return tips, nil
}

// parentMergeRejection returns why merge commit sha does not qualify as a clean parent merge, or "" when it does.
// It qualifies only when it has exactly two parents, its second parent is reachable from one of parentTips,
// and its tree equals the conflict-free merge of its two parents.
// The first parent needs no check here: the caller's walk steps to parents[0] itself.
// Any git error is itself a rejection, so the check fails closed.
func parentMergeRejection(repo *gitrepo.Repo, sha string, parents, parentTips []string) string {
	if len(parents) != 2 {
		return fmt.Sprintf("it has %d parents, and only a two-parent merge qualifies", len(parents))
	}
	onParent := false
	for _, tip := range parentTips {
		ok, err := repo.IsAncestor(parents[1], tip)
		if err != nil {
			return fmt.Sprintf("checking its merged-in parent %s against the parent branch failed: %v", parents[1], err)
		}
		if ok {
			onParent = true
			break
		}
	}
	if !onParent {
		return fmt.Sprintf("it merged %s, which is not on the run's parent branch", parents[1])
	}
	cleanTree, clean, err := repo.MergeTree(parents[0], parents[1])
	if err != nil {
		return fmt.Sprintf("computing the clean merge of its parents failed: %v", err)
	}
	if !clean {
		return "its parents do not merge cleanly, so it carries a hand-made conflict resolution"
	}
	tree, err := repo.CommitTree(sha)
	if err != nil {
		return fmt.Sprintf("reading its tree failed: %v", err)
	}
	if tree != cleanTree {
		return "its tree differs from the clean merge of its parents, so it carries changes beyond a clean merge"
	}
	return ""
}

// parentMergeRefusal builds the head_sha mismatch refusal for a walked merge commit that is not a clean parent merge, its way forward worded by refusal.
func parentMergeRefusal(subject, reportHead, head, merge, reason string, refusal headRefusal) error {
	return fmt.Errorf("webster: %s: head_sha %q does not match the worktree's actual HEAD %q; "+
		"only merge commits that cleanly merge the run's parent branch may sit between a fork's reported head and HEAD, "+
		"and merge commit %s does not qualify: %s; "+
		"way forward: move HEAD back to %s %s, %s, and redo the parent merge-in %s",
		subject, reportHead, head, merge, reason, refusal.head, reportHead, refusal.rerun, refusal.redoMerge)
}

// ignoredPath reports whether git ignores path in worktree.
// A path that does not exist and is not ignored reads as not ignored, the conservative answer.
// It wraps gitexec.Run directly for the same reason dirty does.
func ignoredPath(worktree, path string) (bool, error) {
	stdout, err := gitexec.Run([]string{"status", "--porcelain", "--ignored", "--", path}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git status --ignored %s in %s: %w", path, worktree, err)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "!! ") {
			return true, nil
		}
	}
	return false, nil
}

// dirty reports whether worktree has any uncommitted or untracked changes.
// It wraps gitexec.Run directly since gitrepo.Repo exposes no porcelain/status method.
func dirty(worktree string) (bool, error) {
	stdout, err := gitexec.Run([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s: %w", worktree, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// otherWorktrees returns the canonical root of every worktree of worktree's repository except worktree itself.
// It reads the repository's own worktree list, so websterengine derives no hub path.
func otherWorktrees(worktree string) ([]string, error) {
	self, err := canonicalPath(worktree)
	if err != nil {
		return nil, err
	}
	stdout, err := gitexec.Run([]string{"worktree", "list", "--porcelain"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: list worktrees of %s: %w", worktree, err)
	}
	var others []string
	for _, line := range strings.Split(stdout, "\n") {
		path, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "worktree ")
		if !ok {
			continue
		}
		canon, err := canonicalPath(path)
		if err != nil {
			return nil, err
		}
		if canon != self {
			others = append(others, canon)
		}
	}
	return others, nil
}

// worktreePathDiffers reports whether path differs from base in worktree: a changed tracked file or an untracked new one.
// `git diff --quiet` exits 1 to answer "differs", so that exit is the answer rather than an error;
// any other nonzero exit is a failure.
func worktreePathDiffers(worktree, base, path string) (bool, error) {
	_, err := gitexec.Run([]string{"diff", "--quiet", base, "--", path}, worktree)
	if code, ok := gitExitCode(err); ok && code == 1 {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("websterengine: git diff %s in %s: %w", path, worktree, err)
	}
	stdout, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "--", path}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git ls-files %s in %s: %w", path, worktree, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// worktreeBlob returns the git blob id of path's content in the worktree, or "" when the file is absent.
func worktreeBlob(worktree, path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("websterengine: stat %s: %w", path, err)
	}
	stdout, err := gitexec.Run([]string{"hash-object", "--", path}, worktree)
	if err != nil {
		return "", fmt.Errorf("websterengine: git hash-object %s in %s: %w", path, worktree, err)
	}
	return strings.TrimSpace(stdout), nil
}

// commitBlob returns the git blob id of path at commit, or "" when the commit does not hold the path.
// `git rev-parse --verify --quiet` exits 1 with no output for a name that does not resolve, so that exit is the "" answer;
// any other nonzero exit is a failure.
func commitBlob(worktree, commit, path string) (string, error) {
	stdout, err := gitexec.Run([]string{"rev-parse", "--verify", "--quiet", commit + ":" + path}, worktree)
	if code, ok := gitExitCode(err); ok && code == 1 {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("websterengine: git rev-parse %s:%s in %s: %w", commit, path, worktree, err)
	}
	return strings.TrimSpace(stdout), nil
}

// treePathsWithBlob returns, sorted, every path in commit's whole tree whose object id is blob.
func treePathsWithBlob(worktree, commit, blob string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"ls-tree", "-r", commit}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: git ls-tree -r %s in %s: %w", commit, worktree, err)
	}
	var paths []string
	for _, line := range strings.Split(stdout, "\n") {
		meta, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) == 3 && fields[2] == blob {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// shaExists reports whether sha names a commit in worktree's repository.
func shaExists(worktree, sha string) bool {
	return gitrepo.New(worktree).SHAExists(sha)
}

// isAncestor reports whether sha is an ancestor of ref in worktree's repository.
func isAncestor(worktree, sha, ref string) (bool, error) {
	return gitrepo.New(worktree).IsAncestor(sha, ref)
}

// gitExitCode returns the exit code of a git command gitexec.Run reports as rejected, and false for nil or an exec-level failure.
func gitExitCode(err error) (int, bool) {
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		return gitErr.ExitCode, true
	}
	return 0, false
}
