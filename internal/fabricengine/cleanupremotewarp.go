// cleanupremotewarp.go implements Topology.CleanupRemoteWarp, the engine half of cleanup's sweep of leftover task branches on the warp repo's origin.
// It is a sibling of Cleanup, which sweeps weft branches, and knows nothing of GitHub: the caller hands it the set of open pull request heads.

package fabricengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// RemoteWarpBranchEntry describes the fate of one branch on the warp origin under CleanupRemoteWarp.
type RemoteWarpBranchEntry struct {
	// Branch is the branch name on origin.
	Branch string `json:"branch"`
	// Candidate reports whether every deletion condition held, whether or not apply ran.
	Candidate bool `json:"candidate"`
	// Deleted reports whether the branch was observably deleted from origin.
	Deleted bool `json:"deleted"`
	// Reason names the condition that kept a non-candidate; empty for a candidate.
	Reason string `json:"reason,omitempty"`
	// Error is non-empty when apply tried to delete a candidate and the gate refused or the lease failed.
	Error string `json:"error,omitempty"`
}

// RemoteWarpCleanupResult is CleanupRemoteWarp's result: one entry per branch on the warp origin, and the mutation record of the call.
type RemoteWarpCleanupResult struct {
	MutationRecord
	// Entries lists every branch on origin and its disposition; always an array.
	Entries []RemoteWarpBranchEntry `json:"entries"`
	// SkippedReason is non-empty when the sweep did not run, today only a warp repo with no origin remote.
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// CleanupRemoteWarp classifies every branch on the warp repo's origin and, with apply, deletes the leftover task branches among them.
// A branch is fabric-managed when the weft origin holds an archive/<slug>/* tag or a WeftBranchName(branch) branch for it,
// where slug is the branch without BranchPrefix; with a non-empty BranchPrefix, a branch lacking it is unmanaged.
// A managed branch is a deletion candidate unless it is origin's default branch, is checked out in a hub worktree, is in openPRHeads, or carries work not landed on the default branch.
// Every other entry carries the reason it was kept.
// Without apply nothing is deleted; with apply each candidate goes through the destructive gate, leased to the tip observed at enumeration,
// and a refusal or a lost lease fills that entry's Error while the sweep continues.
// Cleanup is untouched and fabricengine imports no GitHub client, so the caller composes the two.
// A nil openPRHeads means the caller could not establish the open pull request set: every deletion is then refused and entries are returned only.
// Callers that can establish it pass a non-nil map, empty when no pull request is open.
// There is no force parameter, so nothing answers a kept branch.
func (t *Topology) CleanupRemoteWarp(l *lyxcwd.Location, apply bool, openPRHeads map[string]bool) (RemoteWarpCleanupResult, error) {
	return t.cleanupRemoteWarp(l, apply, openPRHeads, nil)
}

// cleanupRemoteWarp is CleanupRemoteWarp with afterEnumerate, a hook run once the tips are observed and before any deletion, so a test can move a tip in between.
func (t *Topology) cleanupRemoteWarp(l *lyxcwd.Location, apply bool, openPRHeads map[string]bool, afterEnumerate func()) (res RemoteWarpCleanupResult, err error) {
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

	weftRoot, err := WeftRepoRoot(l)
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

	worktrees, err := List(repoDir)
	if err != nil {
		return res, fmt.Errorf("list warp worktrees: %w", err)
	}
	checkedOut := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		checkedOut[wt.Branch] = true
	}

	// One fetch makes every observed tip's objects local and brings the default branch's tracking ref up to date, which is the parent the landed check compares against.
	if _, err := gitexec.Run([]string{"fetch", "--no-tags", originRemoteName}, repoDir); err != nil {
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
		deleted, delErr := deleteRemoteBranch(rec, remoteBranchRequest{
			what:      "delete leftover task branch on remote",
			repoDir:   repoDir,
			remote:    originRemoteName,
			branch:    entry.Branch,
			ownership: ownedPairWarpBranch(entry.Branch, defaultBranch),
			dirtiness: dirtyUnlandedRemoteTip(parent),
			leaseSHA:  tips[entry.Branch],
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
	return res, nil
}

// keptReason names the first deletion condition branch fails before the landed check, or "" when it passes them all.
func (t *Topology) keptReason(branch, defaultBranch string, weftBranches map[string]string, archiveTagRefs []string, checkedOut map[string]bool, openPRHeads map[string]bool) string {
	if branch == defaultBranch {
		return "origin's default branch"
	}
	prefix := t.cfg.BranchPrefix
	if prefix != "" && !strings.HasPrefix(branch, prefix) {
		return fmt.Sprintf("not fabric-managed: lacks the branch prefix %q", prefix)
	}
	slug := strings.TrimPrefix(branch, prefix)
	if slug == "" {
		return "not fabric-managed: empty slug"
	}
	if !hasWeftEvidence(slug, WeftBranchName(branch), weftBranches, archiveTagRefs) {
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
	tagPrefix := "refs/tags/archive/" + slug + "/"
	for _, ref := range archiveTagRefs {
		if strings.HasPrefix(ref, tagPrefix) {
			return true
		}
	}
	return false
}

// unlandedReason runs the landed check against the tip and returns why it failed, or "" when the tip's work is landed on parent.
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

// remoteDefaultBranch returns the branch origin's HEAD points at.
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
