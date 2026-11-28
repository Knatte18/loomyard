// cleanupremotewarp.go implements the sweep of leftover task branches on the warp repo's origin.
// It imports no GitHub client: the caller passes in the set of open pull request heads.

package fabricengine

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

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
// The landed check counts a local tag as holding the tip's commits.
// Before each delete, the sweep therefore pushes the slug's archive tags that origin lacks.
// A failed tag push fills that entry's Error and keeps the branch.
// The sweep pushes no branch.
// A candidate whose tip's work only another local branch then holds is therefore kept with a Reason.
// A refusal or a lost lease fills that entry's Error, and the sweep continues.
// A candidate whose lock is held, or that a worktree has checked out under the lock, is kept with a Reason.
// Any other lock error fills its Error.
// A nil openPRHeads means the caller could not establish the open pull request set:
// every deletion is then refused and only entries are returned.
// Callers that can establish it pass a non-nil map, empty when no pull request is open.
// There is no force parameter: nothing overrides a kept branch.
func (t *Topology) CleanupRemoteWarp(l *lymcwd.Location, apply bool, openPRHeads map[string]bool) (fabriclayout.RemoteWarpCleanupResult, error) {
	return t.cleanupRemoteWarp(l, apply, openPRHeads, nil)
}

// cleanupRemoteWarp classifies the warp origin's branches and, with apply, deletes the leftover ones.
// afterEnumerate, when not nil, runs once the tips are observed and before any deletion;
// a test uses it to move a tip in between.
func (t *Topology) cleanupRemoteWarp(l *lymcwd.Location, apply bool, openPRHeads map[string]bool, afterEnumerate func()) (res fabriclayout.RemoteWarpCleanupResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()
	res.Entries = []fabriclayout.RemoteWarpBranchEntry{}

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
	archiveTagRefs, err := remoteTagRefs(weftRoot, fabriclayout.ArchiveTagPrefix+"*")
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
		entry := fabriclayout.RemoteWarpBranchEntry{Branch: branch}
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
// It then pushes the slug's local archive tags that origin lacks, since one of them may be all that holds the tip's work;
// a failed push fills Error and deletes nothing.
// The landed check then runs with local branches left out, and a tip that fails it keeps its branch with a Reason.
func (t *Topology) deleteLeftoverUnderLock(rec *Mutations, l *lymcwd.Location, entry *fabriclayout.RemoteWarpBranchEntry, defaultBranch, parent, tip string) {
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
	if err := pushLocalArchiveTags(rec, repoDir, slug, "", t.archiveSleep); err != nil {
		entry.Error = err.Error()
		return
	}
	// A local branch holds nothing on origin, so the delete below would take origin's only copy of work such a branch holds.
	if err := checkUnlandedRemoteTip(remoteBranchRequest{
		what:                   "delete leftover task branch on remote",
		repoDir:                repoDir,
		remote:                 originRemoteName,
		branch:                 entry.Branch,
		dirtiness:              dirtyUnlandedRemoteTip(parent),
		leaseSHA:               tip,
		localBranchesDoNotHold: true,
	}); err != nil {
		reason := err.Error()
		if refusal, ok := RefusalOf(err); ok {
			reason = refusal.Reason
		}
		entry.Reason = "kept: with local branches left out, " + reason
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
	// An archive tag is evidence only for a branch this machine's prefix names:
	// the slug of any other branch is a guess at where its prefix stops, so a tag of that slug proves nothing of it.
	evidenceTagRefs := archiveTagRefs
	if !slices.Contains(pairBranchNames(t.cfg, slug), branch) {
		evidenceTagRefs = nil
	}
	if !hasWeftEvidence(slug, fabriclayout.RecordsBranchName(branch), weftBranches, evidenceTagRefs) {
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
	tagPrefix := "refs/tags/" + fabriclayout.ArchiveTagPrefix + slug + "/"
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

// remoteBranchTips maps every branch of dir's records namespace on originRemoteName to its tip SHA.
// A branch outside the namespace, such as a code branch in a one-repository hub, is not listed.
func remoteBranchTips(dir string) (map[string]string, error) {
	ns, err := originNamespaceAt(dir)
	if err != nil {
		return nil, err
	}
	out, err := gitexec.Run([]string{"ls-remote", originRemoteName, ns.Branch("*")}, dir)
	if err != nil {
		return nil, fmt.Errorf("list branches on %q: %w", originRemoteName, err)
	}
	tips := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if local, mapped := ns.LocalRef(ref); mapped && strings.HasPrefix(local, "refs/heads/") {
			tips[strings.TrimPrefix(local, "refs/heads/")] = sha
		}
	}
	return tips, nil
}

// remoteTagRefs lists the local refs/tags/ names of the tags on originRemoteName that match pattern.
// pattern is relative to the tags prefix, so the same pattern serves a code store and a one-repository records store.
func remoteTagRefs(dir, pattern string) ([]string, error) {
	ns, err := originNamespaceAt(dir)
	if err != nil {
		return nil, err
	}
	out, err := gitexec.Run([]string{"ls-remote", originRemoteName, ns.Tag(pattern)}, dir)
	if err != nil {
		return nil, fmt.Errorf("list tags on %q: %w", originRemoteName, err)
	}
	var refs []string
	for _, line := range strings.Split(out, "\n") {
		_, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		name, _ := strings.CutSuffix(ref, "^{}")
		if local, mapped := ns.LocalRef(name); mapped && strings.HasPrefix(local, "refs/tags/") {
			refs = append(refs, local+strings.TrimPrefix(ref, name))
		}
	}
	return refs, nil
}

// PlainArchiveTagsOnOrigin lists the archive and owner archive tags under origin's plain refs/tags/.
// It reads a one-repository hub.
// Each tag carries the commands that move it under the code store's archive prefix and delete the plain ref.
// The commands run from the code worktree at l, and the prefix sits below the records namespace root.
// The push of both is atomic, so a failed move deletes nothing.
// A two-repository hub, and a code store with no origin remote, list none.
// A code store whose origin URL cannot be read for any other reason is an error.
// The list is sorted by ref, and empty rather than nil.
func PlainArchiveTagsOnOrigin(l *lymcwd.Location) ([]fabriclayout.PlainArchiveTag, error) {
	tags := []fabriclayout.PlainArchiveTag{}
	recordsRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return tags, fmt.Errorf("resolve weft repo root: %w", err)
	}
	ns, err := originNamespaceAt(recordsRoot)
	if err != nil || ns == (gitrepo.RefNamespace{}) {
		return tags, err
	}
	codeDir := l.WorktreePath()
	if _, urlErr := gitrepo.New(codeDir).RemoteURL(originRemoteName); urlErr != nil {
		if errors.Is(urlErr, gitrepo.ErrRemoteNotConfigured) {
			return tags, nil
		}
		return tags, fmt.Errorf("read the origin URL of the code store at %s: %w", codeDir, urlErr)
	}

	prefixes := []string{"refs/tags/" + fabriclayout.ArchiveTagPrefix, "refs/tags/" + fabriclayout.OwnerArchiveTagPrefix}
	out, err := gitexec.Run([]string{"ls-remote", originRemoteName, prefixes[0] + "*", prefixes[1] + "*"}, codeDir)
	if err != nil {
		return tags, fmt.Errorf("list the archive tags under %q's plain refs/tags/: %w", originRemoteName, err)
	}
	codeTags := fabriclayout.RecordsNamespaceRoot + fabriclayout.RecordsNamespaceCodeTags
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || strings.HasSuffix(ref, "^{}") || !slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(ref, p) }) {
			continue
		}
		name := strings.TrimPrefix(ref, "refs/tags/")
		tags = append(tags, fabriclayout.PlainArchiveTag{Ref: ref, SHA: sha, WayForward: fmt.Sprintf(
			"move it under %s with `git -C %s fetch --no-tags %s %s`, then `git -C %s push --atomic %s %s:%s%s :%s`",
			codeTags, codeDir, originRemoteName, ref, codeDir, originRemoteName, sha, codeTags, name, ref)})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Ref < tags[j].Ref })
	return tags, nil
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
