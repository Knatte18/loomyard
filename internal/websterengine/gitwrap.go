// gitwrap.go holds webster's git queries: HEAD, merge-state, parent-merge and suspect-path checks.
// Helpers here go through gitrepo.Repo, except dirty and the probes gitrepo has no method for.
// Those wrap the checked gitexec.Run directly, and they stay in this file so no other webster file runs git.
// The worktree list is read through dotgit, and through git only for a repository whose refs are not files.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// headSHA returns the SHA of worktree's HEAD commit.
// An unborn HEAD returns an error that wraps gitrepo.ErrNoCommits.
func headSHA(worktree string) (string, error) {
	sha, err := gitrepo.New(worktree).CurrentSHA()
	if err != nil {
		return "", fmt.Errorf("websterengine: head sha in %s: %w", worktree, err)
	}
	return sha, nil
}

// refuseMidMerge returns an error when worktree has a git merge in progress, and nil otherwise.
// It asks git whether MERGE_HEAD resolves rather than statting a file.
// A linked worktree keeps MERGE_HEAD in its per-worktree git dir, so a file stat would miss it.
func refuseMidMerge(git Git, worktree string) error {
	present, err := git.MergeInProgress(worktree)
	if err != nil {
		return fmt.Errorf("websterengine: probe merge in progress in %s: %w", worktree, err)
	}
	if present {
		return fmt.Errorf("webster: worktree %s has a git merge in progress; conclude it first "+
			"(`lym fabric merge --continue` / `lym fabric merge --abort` in a hub, "+
			"`git merge --continue` / `git merge --abort` for a standalone run); "+
			"a session lym refuses the verb from reports status: FAILED and the orch runs it", worktree)
	}
	return nil
}

// mergeInProgress reports whether worktree has a git merge in progress.
func mergeInProgress(worktree string) (bool, error) {
	return gitrepo.New(worktree).MergeHeadPresent()
}

// commitParents returns the parent SHAs of commit in worktree's repository.
func commitParents(worktree, commit string) ([]string, error) {
	return gitrepo.New(worktree).CommitParents(commit)
}

// mergeRejection returns why commit does not qualify as a clean merge of the run's parent branch, or "" when it does.
// A parentBranch that cannot be resolved is itself a rejection, so no merge is accepted without one.
func mergeRejection(worktree, commit string, parents []string, parentBranch ParentBranchFunc) string {
	repo := gitrepo.New(worktree)
	tips, err := resolveParentTips(repo, parentBranch)
	if err != nil {
		return err.Error()
	}
	return parentMergeRejection(repo, commit, parents, tips)
}

// ParentBranchFunc names the branch the run merges its parent in from, resolved only when a merge commit needs checking.
// A nil ParentBranchFunc means the run has no parent branch, so no merge commit is ever accepted.
type ParentBranchFunc func() (string, error)

// reconcileReportHead cross-checks a report's head_sha against worktree's HEAD.
// It tolerates only clean parent merges between them.
// HEAD equal to reportHead returns an empty warning.
// Otherwise it walks HEAD's first-parent chain.
// It compares each commit with reportHead before it looks at that commit's parents,
// so a reportHead that is itself a merge commit is accepted, not stepped past.
// A commit other than reportHead is walked over only when it is a clean parent merge.
// It has exactly two parents.
// Its second parent is reachable from parentBranch's local or origin tip.
// Its tree equals the conflict-free merge of the two parents.
// A non-merge commit, the root, or a merge failing any check ends the walk in refusal.
// Any error while checking also ends it in refusal.
// The rule keeps the audit sound.
// The batch is recorded at reportHead.
// Content a merge adds beyond a clean parent merge would bypass the audited delta.
// On acceptance the warning names subject, both heads and every walked merge SHA in walk order.
// A refusal's way forward is worded for record-batch and recover-batch of the given batch number.
func reconcileReportHead(git Git, worktree, reportHead, subject string, parentBranch ParentBranchFunc, batch int) (warning string, err error) {
	return reconcileHead(git, worktree, reportHead, subject, parentBranch, reportHeadRefusal(batch))
}

// headRefusal holds the wording of a refusal's way forward for one verb.
type headRefusal struct {
	// resetStep is the reset that moves HEAD back onto the commit the check expects.
	resetStep string
	// rerun is what to re-run once HEAD is back on it, such as "re-run this verb".
	rerun string
	// redoMerge is when to redo a parent merge-in that did not qualify, such as "after the batch is recorded".
	redoMerge string
}

// reportHeadRefusal words the way forward for a verb that records a batch at a report's head_sha:
// reset to the report's head with `reset --to report-head`.
func reportHeadRefusal(batch int) headRefusal {
	return headRefusal{resetStep: resetVerb(ResetToReportHead, batch), rerun: "re-run this verb", redoMerge: "after the batch is recorded"}
}

// reconcileHead cross-checks reportHead against worktree's HEAD, tolerating only clean parent merges between them.
// refusal sets the wording of the way forward in each refusal it returns.
func reconcileHead(git Git, worktree, reportHead, subject string, parentBranch ParentBranchFunc, refusal headRefusal) (warning string, err error) {
	head, err := git.HeadSHA(worktree)
	if err != nil {
		return "", err
	}
	if head == reportHead {
		return "", nil
	}

	var merges []string
	for cur := head; ; {
		if cur == reportHead {
			return fmt.Sprintf("webster: %s: head_sha %q differs from the worktree's HEAD %q; only merge commits (%s) sit between them, so the batch is recorded at the report's head_sha %q",
				subject, reportHead, head, strings.Join(merges, ", "), reportHead), nil
		}
		parents, err := git.CommitParents(worktree, cur)
		if err != nil {
			return "", fmt.Errorf("websterengine: walk first-parent chain from %s in %s: %w", head, worktree, err)
		}
		if len(parents) < 2 {
			return "", fmt.Errorf("webster: %s: head_sha %q does not match the worktree's actual HEAD %q; "+
				"only merge commits, such as a parent merge-in, may sit between a fork's reported head and HEAD; "+
				"%s",
				subject, reportHead, head, wayForwardSteps(refusal.resetStep, refusal.rerun))
		}
		if reason := git.MergeRejection(worktree, cur, parents, parentBranch); reason != "" {
			return "", parentMergeRefusal(subject, reportHead, head, cur, reason, refusal)
		}
		merges = append(merges, cur)
		cur = parents[0]
	}
}

// resolveParentTips returns every tip a parent merge-in may merge from:
// the local branch and its origin remote-tracking ref, whichever exist.
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
// The first parent is not checked here; the walk that calls this continues from it.
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

// parentMergeRefusal builds the head_sha mismatch refusal for a walked merge commit that is not a clean parent merge.
// Its way forward is worded by refusal.
func parentMergeRefusal(subject, reportHead, head, merge, reason string, refusal headRefusal) error {
	return fmt.Errorf("webster: %s: head_sha %q does not match the worktree's actual HEAD %q; "+
		"only merge commits that cleanly merge the run's parent branch may sit between a fork's reported head and HEAD, "+
		"and merge commit %s does not qualify: %s; "+
		"%s",
		subject, reportHead, head, merge, reason,
		wayForwardSteps(refusal.resetStep, refusal.rerun, "redo the parent merge-in "+refusal.redoMerge))
}

// ignoredPath reports whether git ignores path in worktree.
// A path that does not exist and is not ignored reads as not ignored, the conservative answer.
// It calls gitexec.Run directly, since gitrepo.Repo exposes no porcelain status method.
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

// commitsBetween returns the commits on head's first-parent chain that base cannot reach, newest first.
// A merge commit in that range is listed, but the commits it brought in from its other parent are not,
// so a merged parent branch's own history never reads as commits made on this branch.
func commitsBetween(worktree, base, head string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"rev-list", "--first-parent", base + ".." + head}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: git rev-list %s..%s in %s: %w", base, head, worktree, err)
	}
	return strings.Fields(stdout), nil
}

// commitsSince returns the commits on HEAD's first-parent chain that base cannot reach, oldest first.
// A merge commit in that range is listed, but the commits it brought in from its other parent are not.
func commitsSince(worktree, base string) ([]string, error) {
	newestFirst, err := commitsBetween(worktree, base, "HEAD")
	if err != nil {
		return nil, err
	}
	slices.Reverse(newestFirst)
	return newestFirst, nil
}

// fixCommitRejection walks the commits from base to HEAD and returns the first that does not qualify.
// A commit qualifies when it is a non-merge commit or a clean parent merge.
// The reason says why the returned commit fails.
// It returns empty strings when every commit qualifies.
// The parent tips are resolved once, on the first merge the walk meets.
// A resolution error rejects that merge with the error as the reason, so a nil parentBranch accepts no merge.
// An error is a failure to read the commits, never a rejection.
func fixCommitRejection(worktree, base string, parentBranch ParentBranchFunc) (commit, reason string, err error) {
	commits, err := commitsSince(worktree, base)
	if err != nil {
		return "", "", err
	}
	repo := gitrepo.New(worktree)
	var parentTips []string
	var tipsErr error
	resolved := false
	for _, c := range commits {
		parents, err := repo.CommitParents(c)
		if err != nil {
			return "", "", fmt.Errorf("websterengine: parents of %s in %s: %w", c, worktree, err)
		}
		if len(parents) < 2 {
			continue
		}
		if !resolved {
			parentTips, tipsErr = resolveParentTips(repo, parentBranch)
			resolved = true
		}
		if tipsErr != nil {
			return c, tipsErr.Error(), nil
		}
		if rejection := parentMergeRejection(repo, c, parents, parentTips); rejection != "" {
			return c, rejection, nil
		}
	}
	return "", "", nil
}

// commitChangedPaths returns the slash-separated repository-relative paths a non-merge commit changes, including a root commit's.
func commitChangedPaths(worktree, commit string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"diff-tree", "--no-commit-id", "--name-only", "-r", "--root", "-z", commit}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: git diff-tree %s in %s: %w", commit, worktree, err)
	}
	var paths []string
	for _, p := range strings.Split(stdout, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// otherWorktrees returns the canonical root of every worktree of worktree's repository except worktree itself.
// It reads the repository's own worktree list, so websterengine derives no hub path.
func otherWorktrees(worktree string) ([]string, error) {
	self, err := canonicalPath(worktree)
	if err != nil {
		return nil, err
	}
	roots, err := worktreeRoots(worktree)
	if err != nil {
		return nil, err
	}
	var others []string
	for _, root := range roots {
		canon, err := canonicalPath(root)
		if err != nil {
			return nil, err
		}
		if canon != self {
			others = append(others, canon)
		}
	}
	return others, nil
}

// worktreeRoots returns the root of every worktree of worktree's repository, read in-process with no git process.
// A repository whose refs are not files is read from `git worktree list --porcelain` instead.
func worktreeRoots(worktree string) ([]string, error) {
	worktrees, err := dotgit.ListWorktrees(worktree)
	if errors.Is(err, dotgit.ErrRefStorage) {
		return porcelainWorktreeRoots(worktree)
	}
	if err != nil {
		return nil, fmt.Errorf("websterengine: list worktrees of %s: %w", worktree, err)
	}
	roots := make([]string, len(worktrees))
	for i, listed := range worktrees {
		roots[i] = listed.Path
	}
	return roots, nil
}

// porcelainWorktreeRoots returns the path of each `worktree` line of `git worktree list --porcelain` run in worktree.
func porcelainWorktreeRoots(worktree string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"worktree", "list", "--porcelain"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: list worktrees of %s: %w", worktree, err)
	}
	var roots []string
	for _, line := range strings.Split(stdout, "\n") {
		if path, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "worktree "); ok {
			roots = append(roots, path)
		}
	}
	return roots, nil
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
// `git rev-parse --verify --quiet` exits 1 with no output for a name that does not resolve,
// so that exit is the empty answer;
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

// nonMergeCommitsBetween reports whether the first-parent range base..head holds a commit that is not a merge.
// A base the repository does not hold is an error, as is any other failed git call.
// It wraps gitexec.Run directly since gitrepo.Repo exposes no first-parent range walk.
func nonMergeCommitsBetween(worktree, base, head string) (bool, error) {
	stdout, err := gitexec.Run([]string{"rev-list", "--first-parent", "--no-merges", "--max-count=1", base + ".." + head}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git rev-list --first-parent --no-merges %s..%s in %s: %w", base, head, worktree, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// departureTrailerKey is the trailer key a fork's commit message declares a PATTERN departure under.
const departureTrailerKey = "Pattern-Departure:"

// commitLog returns, oldest first, the non-merge commits on head's first-parent line after base.
// It walks head's first-parent line through gitrepo and stops at base.
// A walk that reaches the root without meeting base is an error.
func commitLog(worktree, base, head string) ([]CommitLogEntry, error) {
	walked, err := gitrepo.New(worktree).Log(head, gitrepo.LogQuery{FirstParent: true})
	if err != nil {
		return nil, fmt.Errorf("websterengine: log of %s in %s: %w", head, worktree, err)
	}
	var entries []CommitLogEntry
	metBase := false
	for _, commit := range walked {
		if commit.SHA == base {
			metBase = true
			break
		}
		if len(commit.Parents) >= 2 {
			continue
		}
		subject, _, _ := strings.Cut(commit.Message, "\n")
		entries = append(entries, CommitLogEntry{
			SHA:        commit.SHA,
			Subject:    strings.TrimSpace(subject),
			Departures: departureTrailers(commit.Message),
		})
	}
	if !metBase {
		return nil, fmt.Errorf("websterengine: %s is not on the first-parent line of %s in %s", base, head, worktree)
	}
	slices.Reverse(entries)
	return entries, nil
}

// departureTrailers returns the values of the departure trailer lines in message's last paragraph.
// Git reads trailers from that paragraph.
// Each value is trimmed and otherwise verbatim.
// A message of one paragraph has no trailers.
func departureTrailers(message string) []string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(message, "\r\n", "\n"))
	cut := strings.LastIndex(trimmed, "\n\n")
	if cut < 0 {
		return nil
	}
	var values []string
	for _, line := range strings.Split(trimmed[cut+2:], "\n") {
		if len(line) >= len(departureTrailerKey) && strings.EqualFold(line[:len(departureTrailerKey)], departureTrailerKey) {
			values = append(values, strings.TrimSpace(line[len(departureTrailerKey):]))
		}
	}
	return values
}

// dirtyTrackedPaths returns the sorted worktree-relative paths of tracked files whose content differs from HEAD.
// The paths use slashes, and a change counts whether staged or not.
// Untracked files are not listed.
// It calls gitexec.Run directly.
func dirtyTrackedPaths(worktree string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"diff", "--name-only", "-z", "HEAD", "--"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: git diff --name-only HEAD in %s: %w", worktree, err)
	}
	var paths []string
	for _, p := range strings.Split(stdout, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// octopusMergeBase returns the best common ancestor of shas, read-only.
// It wraps gitexec.Run directly since gitrepo.Repo exposes no merge-base method.
// An error includes the case where the commits share no common ancestor.
func octopusMergeBase(worktree string, shas []string) (string, error) {
	stdout, err := gitexec.Run(append([]string{"merge-base", "--octopus"}, shas...), worktree)
	if err != nil {
		return "", fmt.Errorf("websterengine: git merge-base --octopus in %s: %w", worktree, err)
	}
	return strings.TrimSpace(stdout), nil
}

// gitExitCode returns the exit code of a git command that gitexec.Run reports as rejected.
// The bool is false for a nil error or an exec-level failure.
func gitExitCode(err error) (int, bool) {
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		return gitErr.ExitCode, true
	}
	return 0, false
}

// ErrHeadSHAUnresolved is the sentinel error for a report's abbreviated head_sha that names no commit or more than one.
var ErrHeadSHAUnresolved = errors.New("webster: report head_sha unresolved")

// commitsNamedBy returns, sorted, the full SHA of each commit whose object name starts with prefix.
// The search covers worktree's repository, and only object names match, never ref names.
// It returns an empty slice when none does.
// `git rev-parse --disambiguate` lists every object the prefix names,
// and it prints nothing, exiting 0, for a prefix that names none.
// The listed objects are classified in one `git cat-file --batch-check` run, and only the commits are kept.
func commitsNamedBy(worktree, prefix string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"rev-parse", "--disambiguate=" + prefix}, worktree)
	if err != nil {
		return nil, fmt.Errorf("websterengine: git rev-parse --disambiguate=%s in %s: %w", prefix, worktree, err)
	}
	named := strings.Fields(stdout)
	if len(named) == 0 {
		return nil, nil
	}
	checked, err := gitexec.RunStdin([]string{"cat-file", "--batch-check"}, worktree, strings.Join(named, "\n")+"\n")
	if err != nil {
		return nil, fmt.Errorf("websterengine: git cat-file --batch-check in %s: %w", worktree, err)
	}
	commits, err := commitsFromBatchCheck(checked)
	if err != nil {
		return nil, fmt.Errorf("websterengine: git cat-file --batch-check in %s: %w", worktree, err)
	}
	sort.Strings(commits)
	return commits, nil
}

// commitsFromBatchCheck returns the object names typed commit in `git cat-file --batch-check` output.
// Each output line is `<name> <type> <size>`, one per object.
// A `<name> missing` line is an error naming the object, since the caller just listed it,
// and any other malformed line is an error too.
func commitsFromBatchCheck(output string) ([]string, error) {
	var commits []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) == 2 && fields[1] == "missing":
			return nil, fmt.Errorf("object %s is missing", fields[0])
		case len(fields) != 3:
			return nil, fmt.Errorf("malformed batch-check line %q", line)
		case fields[1] == "commit":
			commits = append(commits, fields[0])
		}
	}
	return commits, nil
}

// minAbbreviatedSHA is git's minimum abbreviation length: a head_sha shorter than this is not resolved.
const minAbbreviatedSHA = 4

// fullSHALength is the length of a full SHA-1 commit id in hex digits.
const fullSHALength = 40

// resolveReportHead resolves a report's head_sha to the one full commit SHA it names.
// A value of exactly 40 hex digits returns unchanged with no git call.
// A hex value of 4 to 39 digits, of either case, is lowercased and resolved against the repository's commits;
// no commit or more than one is an error wrapping ErrHeadSHAUnresolved that names reportPath and the way forward.
// Any other value returns unchanged, so it meets the refusal at its consume site.
// A git failure is returned as an error that does not wrap ErrHeadSHAUnresolved.
func resolveReportHead(git Git, worktree, reportPath, headSHA string) (string, error) {
	if len(headSHA) < minAbbreviatedSHA || len(headSHA) >= fullSHALength || !isHex(headSHA) {
		return headSHA, nil
	}
	prefix := strings.ToLower(headSHA)
	commits, err := git.CommitsNamedBy(worktree, prefix)
	if err != nil {
		return "", err
	}
	if len(commits) == 1 {
		return commits[0], nil
	}
	named := "names no commit"
	if len(commits) > 1 {
		named = fmt.Sprintf("names %d commits", len(commits))
	}
	return "", fmt.Errorf("%w: head_sha %q in %s %s; "+
		"way forward: have the fork rewrite head_sha in %s as the output of `git rev-parse HEAD`, then re-run this verb",
		ErrHeadSHAUnresolved, headSHA, reportPath, named, reportPath)
}

// isHex reports whether s is non-empty and holds only hex digits of either case.
func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return s != ""
}
