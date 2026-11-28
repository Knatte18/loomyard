// cleanupremotewarp.go implements the sweep of leftover task branches on the warp repo's origin.
// It imports no GitHub client: the caller passes in the set of open pull request heads.

package fabricengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// RemoteWarpBranchEntry describes the fate of one branch on the warp origin in a sweep.
type RemoteWarpBranchEntry struct {
	// Branch is the branch name on origin.
	Branch string `json:"branch"`
	// Candidate reports whether every deletion condition held, whether or not apply ran.
	Candidate bool `json:"candidate"`
	// Deleted reports whether the branch was observably deleted from origin.
	Deleted bool `json:"deleted"`
	// Reason names the condition that kept a non-candidate; empty for a candidate.
	// Under apply a kept candidate's Reason also names a held teardown lock or a hub checkout.
	Reason string `json:"reason,omitempty"`
	// Error is set when apply's delete of a candidate was refused, its lease was lost, or its teardown lock failed.
	Error string `json:"error,omitempty"`
}

// RemoteWarpCleanupResult reports a sweep of the warp origin: one entry per branch, and the mutations the sweep made.
type RemoteWarpCleanupResult struct {
	MutationRecord
	// Entries lists every branch on origin and its disposition; always an array.
	Entries []RemoteWarpBranchEntry `json:"entries"`
	// SkippedReason is non-empty when the sweep did not run, such as when the warp repo has no origin remote.
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// CleanupRemoteWarp classifies the warp origin's branches and, with apply, deletes the leftover task branches.
// A branch is fabric-managed when the weft origin holds one of two things:
// an archive/<slug>/* tag, or a RecordsBranchName(branch) branch for it.
// Here slug is the branch without the current branch prefix.
// A branch lacking the prefix is read as the bare slug of a pair made before the prefix was set,
// and the weft evidence alone decides whether it is managed.
// A managed branch is a deletion candidate unless one of these holds:
// it is origin's default branch;
// a hub worktree has it checked out;
// it heads an open pull request;
// or it carries work not landed on the default branch.
// Every other entry carries the reason it was kept.
// Without apply nothing is deleted.
// With apply, each candidate goes through the destructive gate, leased to the tip observed at enumeration.
// It is tried only when two things hold:
// the slug's teardown lock can be taken on one try, and no hub worktree has it checked out under that lock.
// A refusal or a lost lease fills that entry's Error, and the sweep continues.
// A candidate whose lock is held, or that a worktree has checked out under the lock, is kept with a Reason.
// Any other lock error fills its Error.
// A nil openPRHeads means the caller could not establish the open pull request set:
// every deletion is then refused and only entries are returned.
// Callers that can establish it pass a non-nil map, empty when no pull request is open.
// There is no force parameter: nothing overrides a kept branch.
func (t *Topology) CleanupRemoteWarp(l *lymcwd.Location, apply bool, openPRHeads map[string]bool) (RemoteWarpCleanupResult, error) {
	return t.cleanupRemoteWarp(l, apply, openPRHeads, nil)
}

// cleanupRemoteWarp classifies the warp origin's branches and, with apply, deletes the leftover ones.
// afterEnumerate, when not nil, runs once the tips are observed and before any deletion;
// a test uses it to move a tip in between.
func (t *Topology) cleanupRemoteWarp(l *lymcwd.Location, apply bool, openPRHeads map[string]bool, afterEnumerate func()) (res RemoteWarpCleanupResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()
	res.Entries = []RemoteWarpBranchEntry{}

	repoDir := l.WorktreePath()
	if _, urlErr := gitrepo.New(repoDir).RemoteURL(originRemoteName); urlErr != nil {
		res.SkippedReason = fmt.Sprintf("no remote sweep attempted: the warp repo has no %q remote configured: %v", originRemoteName, urlErr)
		return res, nil
	}

	tips, err := remoteBranchTips(repoDir)
	if err != nil {
		return res, err
	}
	defaultBranch, err := remoteDefaultBranch(repoDir)
	if err != nil {
		return res, err
	}

	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return res, fmt.Errorf("resolve weft repo root: %w", err)
	}
	weftBranches, err := remoteBranchTips(weftRoot)
	if err != nil {
		return res, err
	}
	archiveTagRefs, err := remoteTagRefs(weftRoot, "refs/tags/archive/*")
	if err != nil {
		return res, err
	}

	checkedOut, err := checkedOutBranches(repoDir)
	if err != nil {
		return res, err
	}

	// One fetch makes every observed tip's objects local and brings the default branch's tracking ref up to date;
	// the landed check compares against that ref.
	if _, err := gitrepo.New(repoDir).FetchWithArgs("--no-tags", originRemoteName); err != nil {
		return res, fmt.Errorf("fetch %q: %w", originRemoteName, err)
	}
	parent := originRemoteName + "/" + defaultBranch

	for _, branch := range sortedKeys(tips) {
		entry := RemoteWarpBranchEntry{Branch: branch}
		entry.Reason = t.keptReason(branch, defaultBranch, weftBranches, archiveTagRefs, checkedOut, openPRHeads)
		if entry.Reason == "" {
			entry.Reason = unlandedReason(repoDir, branch, tips[branch], parent)
		}
		entry.Candidate = entry.Reason == ""
		res.Entries = append(res.Entries, entry)
	}

	if afterEnumerate != nil {
		afterEnumerate()
	}
	if !apply || openPRHeads == nil {
		return res, nil
	}

	for i := range res.Entries {
		entry := &res.Entries[i]
		if !entry.Candidate {
			continue
		}
		t.deleteLeftoverUnderLock(rec, l, entry, defaultBranch, parent, tips[entry.Branch])
	}
	return res, nil
}

// checkedOutBranches lists the branches some worktree of the repository at repoDir has checked out.
func checkedOutBranches(repoDir string) (map[string]bool, error) {
	worktrees, err := List(repoDir)
	if err != nil {
		return nil, fmt.Errorf("list warp worktrees: %w", err)
	}
	checkedOut := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		checkedOut[wt.Branch] = true
	}
	return checkedOut, nil
}

// deleteLeftoverUnderLock deletes the candidate entry's branch on origin, leased to tip.
// It takes the slug's teardown lock on one try:
// a held lock keeps the candidate with a Reason, and any other lock error fills Error.
// Under the lock it re-reads whether a hub worktree has the branch checked out.
// The sweep's earlier read may predate an Add of the slug;
// a checkout found now keeps the candidate with a Reason.
func (t *Topology) deleteLeftoverUnderLock(rec *Mutations, l *lymcwd.Location, entry *RemoteWarpBranchEntry, defaultBranch, parent, tip string) {
	repoDir := l.WorktreePath()
	slug := slugOfBranch(t.cfg, entry.Branch)
	// The lock file is the sweep's scratch state, not a mutation of the branch, so it joins no record.
	release, lockErr := t.lockPairTeardown(NewMutations(l.HubPath), l, slug, true, cleanupSweepRerun)
	switch {
	case errors.Is(lockErr, ErrPairBusy):
		entry.Reason = teardownLockHeldReason(slug)
		return
	case lockErr != nil:
		entry.Error = fmt.Sprintf("take the teardown lock of the pair %q failed: %v", slug, lockErr)
		return
	}
	defer release()

	checkedOut, err := checkedOutBranches(repoDir)
	if err != nil {
		entry.Error = err.Error()
		return
	}
	if checkedOut[entry.Branch] {
		entry.Reason = "kept: a hub worktree now has the branch checked out, so its pair is live"
		return
	}

	deleted, delErr := deleteRemoteBranch(rec, remoteBranchRequest{
		what:      "delete leftover task branch on remote",
		repoDir:   repoDir,
		remote:    originRemoteName,
		branch:    entry.Branch,
		ownership: ownedPairWarpBranch(entry.Branch, defaultBranch),
		dirtiness: dirtyUnlandedRemoteTip(parent),
		leaseSHA:  tip,
		force:     false,
	})
	switch {
	case delErr == nil:
		entry.Deleted = deleted
	default:
		if refusal, ok := RefusalOf(delErr); ok {
			entry.Error = refusal.Reason
		} else {
			entry.Error = fmt.Sprintf("delete leftover task branch %s on %s: %v", entry.Branch, originRemoteName, delErr)
		}
	}
}

// keptReason names the first deletion condition the branch fails before the landed check.
// It returns the empty string when the branch passes them all.
func (t *Topology) keptReason(branch, defaultBranch string, weftBranches map[string]string, archiveTagRefs []string, checkedOut map[string]bool, openPRHeads map[string]bool) string {
	if branch == defaultBranch {
		return "origin's default branch"
	}
	slug := slugOfBranch(t.cfg, branch)
	if slug == "" {
		return "not fabric-managed: empty slug"
	}
	if !hasWeftEvidence(slug, RecordsBranchName(branch), weftBranches, archiveTagRefs) {
		return "not fabric-managed: the weft origin holds neither a weft branch nor an archive tag for it"
	}
	if checkedOut[branch] {
		return "checked out in a hub worktree"
	}
	if openPRHeads == nil {
		return "open pull requests unknown; nothing is deleted"
	}
	if openPRHeads[branch] {
		return "head of an open pull request"
	}
	return ""
}

// hasWeftEvidence reports whether the weft origin holds weftBranch or an archive/<slug>/* tag.
func hasWeftEvidence(slug, weftBranch string, weftBranches map[string]string, archiveTagRefs []string) bool {
	if _, ok := weftBranches[weftBranch]; ok {
		return true
	}
	tagPrefix := "refs/tags/" + ArchiveTagPrefix + slug + "/"
	for _, ref := range archiveTagRefs {
		if strings.HasPrefix(ref, tagPrefix) {
			return true
		}
	}
	return false
}

// unlandedReason runs the landed check of tip against parent and returns why it failed.
// It returns the empty string when the tip's work is landed on parent.
func unlandedReason(repoDir, branch, tip, parent string) string {
	err := checkUnlandedRemoteTip(remoteBranchRequest{
		what:      "classify leftover task branch on remote",
		repoDir:   repoDir,
		remote:    originRemoteName,
		branch:    branch,
		dirtiness: dirtyUnlandedRemoteTip(parent),
		leaseSHA:  tip,
	})
	if err == nil {
		return ""
	}
	var refusal *destructiveRefusal
	if errors.As(err, &refusal) {
		return refusal.Reason
	}
	return err.Error()
}

// remoteBranchTips maps every branch on originRemoteName, as seen from dir, to its tip SHA.
func remoteBranchTips(dir string) (map[string]string, error) {
	out, err := gitexec.Run([]string{"ls-remote", "--heads", originRemoteName}, dir)
	if err != nil {
		return nil, fmt.Errorf("list branches on %q: %w", originRemoteName, err)
	}
	tips := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && strings.HasPrefix(ref, "refs/heads/") {
			tips[strings.TrimPrefix(ref, "refs/heads/")] = sha
		}
	}
	return tips, nil
}

// remoteTagRefs lists the full ref names on originRemoteName matching pattern.
func remoteTagRefs(dir, pattern string) ([]string, error) {
	out, err := gitexec.Run([]string{"ls-remote", "--tags", originRemoteName, pattern}, dir)
	if err != nil {
		return nil, fmt.Errorf("list tags on %q: %w", originRemoteName, err)
	}
	var refs []string
	for _, line := range strings.Split(out, "\n") {
		_, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// remoteDefaultBranch returns the branch that the HEAD of originRemoteName points at.
func remoteDefaultBranch(dir string) (string, error) {
	out, err := gitexec.Run([]string{"ls-remote", "--symref", originRemoteName, "HEAD"}, dir)
	if err != nil {
		return "", fmt.Errorf("read the default branch of %q: %w", originRemoteName, err)
	}
	for _, line := range strings.Split(out, "\n") {
		target, ok := strings.CutPrefix(strings.TrimSpace(line), "ref: refs/heads/")
		if !ok {
			continue
		}
		if name, _, found := strings.Cut(target, "\t"); found && name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("cannot determine the default branch of %q", originRemoteName)
}

// sortedKeys returns m's keys in ascending order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
