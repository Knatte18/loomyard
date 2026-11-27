// handover.go holds the git steps of handing a run over to the machine of a takeover entry.
// It moves the code tip the run's last transition names onto origin.
// It archives the local tips under owner archive tags.
// It resets a parent branch the run's own merge put ahead of its upstream, and writes this machine's hand-over mark.
// The caller ends the run's processes and removes the local pair, since fabric owns neither.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// owedTipPushedHook is nil in production.
// When set, it runs right after the owed code tip's push, before the records branch is read again.
var owedTipPushedHook func()

// HandOverRequest names what a hand-over covers.
type HandOverRequest struct {
	// Slug is the run handed over.
	Slug string
	// Pair is the run's task pair, nil for a run that has none here.
	Pair *lymcwd.Location
	// Parent is the parent pair the run's parent-side merge went into, nil when there is none.
	// It matters only together with Pair.
	Parent *lymcwd.Location
	// Entry is the takeover entry the hand-over answers.
	Entry shedengine.Takeover
	// DroppedIntents names the pause or cancel intents that never reached origin, for the notice.
	DroppedIntents []string
	// PrimeRecords is the prime whose records hold the run's unpushed transitions, nil when there is none to archive.
	// A set one has its local records tip pushed under an owner archive tag named for the run.
	PrimeRecords *lymcwd.Location
}

// HandOverResult reports what a hand-over did.
type HandOverResult struct {
	// Tags lists the owner archive tags it pushed.
	Tags []string
	// TipPushed reports whether it pushed the owed code tip and the tip stands on origin.
	// A tip the run gave up between the push and the re-read is moved off origin again and reports false.
	// When the push created the branch, the branch is deleted instead.
	TipPushed bool
}

// HandOverTips hands the run req names over to the machine of req.Entry, under the slug's teardown lock.
// First it pushes the code tip the run's last transition on origin names.
// It does so when origin's code branch lags behind that tip or is absent.
// Next it archives the pair's local code and records tips that origin lacks, uncommitted files included.
// Each goes under an owner archive tag pushed to origin.
// A parent branch that only the run's own merge puts ahead of its upstream is tagged the same way.
// That branch is then reset to the upstream.
// With req.PrimeRecords set, the prime's local records tip is archived the same way, under a tag named for the run.
// One notice goes through the installed push notifier, and the hand-over mark is written last, after every tag landed.
// A failure at any step returns it before the mark is written, so the next rejected push hands the run over again;
// a failed archive wraps ErrArchiveFailed.
// It pushes no branch but the owed code tip, and no tag but the owner archive tags.
func HandOverTips(prime *lymcwd.Location, req HandOverRequest) (HandOverResult, error) {
	hubPath := prime.HubPath
	if err := ensureTeardownLockDir(hubPath, req.Slug); err != nil {
		return HandOverResult{}, err
	}
	held, acquired, err := lock.AcquireWriteLockWithin(fabriclayout.TeardownLockPath(hubPath, req.Slug), fabriclayout.StatusPushLockWait)
	if err != nil {
		return HandOverResult{}, fmt.Errorf("take the teardown lock of the run %q: %w", req.Slug, err)
	}
	if !acquired {
		return HandOverResult{}, fmt.Errorf("a push or removal of the pair %q still holds its teardown lock: %w", req.Slug, ErrPairPushRunning)
	}
	defer func() { _ = held.Release() }()

	rec := NewMutations(hubPath)
	var result HandOverResult
	if req.Pair != nil {
		pushed, err := pushOwedCodeTip(rec, req.Pair)
		if err != nil {
			return result, err
		}
		result.TipPushed = pushed
		tags, err := archiveOwnerSides(rec, OwnerArchivePair{Slug: req.Slug, Pair: req.Pair, CommitUncommitted: true})
		result.Tags = append(result.Tags, tags...)
		if err != nil {
			return result, fmt.Errorf("%w: pair %s: %w", ErrArchiveFailed, req.Slug, err)
		}
		if req.Parent != nil {
			ownMerge, err := parentAheadByOwnMergeOnly(req.Parent.WorktreePath(), req.Pair.WorktreePath())
			if err != nil {
				return result, fmt.Errorf("%w: parent of %s: %w", ErrArchiveFailed, req.Slug, err)
			}
			if ownMerge {
				tag, err := archiveParentMerge(rec, req.Parent.WorktreePath(), req.Slug)
				if err != nil {
					return result, fmt.Errorf("%w: parent of %s: %w", ErrArchiveFailed, req.Slug, err)
				}
				if tag != "" {
					result.Tags = append(result.Tags, tag)
				}
			}
		}
	}

	if req.PrimeRecords != nil {
		tag, err := archiveOwnerTip(rec, RecordsWorktree(req.PrimeRecords), req.Slug, "records", false)
		if err != nil {
			return result, fmt.Errorf("%w: prime records of %s: %w", ErrArchiveFailed, req.Slug, err)
		}
		if tag != "" {
			result.Tags = append(result.Tags, tag)
		}
	}

	if notify := installedNotifier(); notify != nil {
		if _, err := notify(hubPath, handOverNotice(req, result.Tags)); err != nil {
			logger.Warn("fabricengine: the hand-over notice was not posted", "slug", req.Slug, "error", err)
		}
	}
	mark := fabriclayout.HandOverMark{Machine: req.Entry.Machine, At: req.Entry.At, Written: time.Now()}
	if err := WriteHandOverMark(hubPath, req.Slug, mark); err != nil {
		return result, err
	}
	return result, nil
}

// handOverNotice words the one notice a hand-over posts.
func handOverNotice(req HandOverRequest, tags []string) fabriclayout.PushNotice {
	line := fmt.Sprintf("the run %s was handed over to machine %s: its processes ended here and its local tips are archived", req.Slug, req.Entry.Machine)
	if len(tags) > 0 {
		line += " under " + strings.Join(tags, ", ")
	}
	if len(req.DroppedIntents) > 0 {
		line += fmt.Sprintf("; its %s never reached origin, so repeat each on machine %s", strings.Join(req.DroppedIntents, " and "), req.Entry.Machine)
	}
	return fabriclayout.PushNotice{Line: line, Topic: "hand-over/" + req.Slug}
}

// WriteHandOverMark writes this machine's hand-over mark of slug, replacing an earlier one.
func WriteHandOverMark(hubPath, slug string, mark fabriclayout.HandOverMark) error {
	path := fabriclayout.HandOverMarkPath(hubPath, slug)
	if err := state.WriteJSON(path, path+".lock", mark); err != nil {
		return fmt.Errorf("fabricengine: write the hand-over mark of %q: %w", slug, err)
	}
	return nil
}

// DropHandOverMark removes this machine's hand-over mark of slug.
// A mark of an earlier hand-over then never stops the pushes of a run this machine takes back.
// An absent mark is no error.
func DropHandOverMark(hubPath, slug string) error {
	markPath := fabriclayout.HandOverMarkPath(hubPath, slug)
	if !pathPresent(markPath) {
		return nil
	}
	req := pathRequest{
		what:      "drop this machine's hand-over mark of a run it takes back",
		container: filepath.Dir(markPath),
		target:    markPath,
		ownership: ownedHubScratch(fabriclayout.HubScratchDir(hubPath)),
		dirtiness: dirtinessNA("the mark is a marker fabric wrote for this machine, never operator content"),
		force:     false,
	}
	if err := removePath(NewMutations(hubPath), req); err != nil {
		return fmt.Errorf("fabricengine: drop the hand-over mark of %q: %w", slug, err)
	}
	return nil
}

// OriginParentRunStatus reads the run record of slug at the upstream tip of a parent pair's records branch.
// parent names the parent pair, and "" names the prime.
// It fetches that branch first, so the answer does not depend on an earlier fetch.
// found is false when the branch has no upstream or the tip holds no record.
func OriginParentRunStatus(prime *lymcwd.Location, slug, parent string) (status shedengine.Status, found bool, err error) {
	location := prime
	if parent != "" {
		location, err = lymcwd.NamedWorktree(prime.HubPath, parent)
		if err != nil {
			return shedengine.Status{}, false, err
		}
	}
	recordsWorktree := RecordsWorktree(location)
	if err := gitrepo.New(recordsWorktree).Fetch(); err != nil {
		return shedengine.Status{}, false, fmt.Errorf("fabricengine: fetch the records branch of the parent %q: %w", parent, err)
	}
	upstream, hasUpstream, err := upstreamSHAAt(recordsWorktree)
	if err != nil || !hasUpstream {
		return shedengine.Status{}, false, err
	}
	return originStatusAtTip(location, recordsWorktree, upstream, slug)
}

// OriginBattenStatus reads the batten run record of slug at the tip of the prime's records branch on origin.
// It fetches that branch first, so the answer does not depend on an earlier fetch.
// found is false when the branch has no upstream or the tip holds no record.
func OriginBattenStatus(prime *lymcwd.Location, slug string) (status shedengine.Status, found bool, err error) {
	recordsWorktree := RecordsWorktree(prime)
	if err := gitrepo.New(recordsWorktree).Fetch(); err != nil {
		return shedengine.Status{}, false, fmt.Errorf("fabricengine: fetch the prime's records branch: %w", err)
	}
	upstream, hasUpstream, err := upstreamSHAAt(recordsWorktree)
	if err != nil || !hasUpstream {
		return shedengine.Status{}, false, err
	}
	return originStatusAtTip(prime, recordsWorktree, upstream, slug)
}

// pushOwedCodeTip pushes the code tip the pair's last transition on origin names, when origin's code branch lags it.
// The tip is read from the records worktree's fetched upstream, and the push is leased at the origin tip read with it.
// It pushes nothing for a tip this repository lacks or a tip origin holds.
// It pushes nothing either when origin's branch has diverged from the tip.
// An origin with no code branch gets the branch created at the tip by a push that never forces.
// After the push the records branch is fetched again.
// When the last transition no longer names the tip, origin's code branch is reset to the value it had:
// a branch the push created is deleted, after the tip is saved under its owner archive tag.
// A given-up tip thus does not stay on origin.
// It reports whether the tip was pushed and stands.
func pushOwedCodeTip(rec *Mutations, pair *lymcwd.Location) (pushed bool, err error) {
	slug := pair.WorktreeName
	weftPath := RecordsWorktree(pair)
	upstream, hasUpstream, err := upstreamSHAAt(weftPath)
	if err != nil || !hasUpstream {
		return false, err
	}
	tip, err := lastTransitionCodeTip(weftPath, upstream, slug)
	if err != nil || tip == "" {
		return false, err
	}

	codeDir := pair.WorktreePath()
	repo := gitrepo.New(codeDir)
	branch, err := repo.CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("read the branch checked out at %s: %w", codeDir, err)
	}
	originTip, err := remoteHeadTip(codeDir, branch)
	if err != nil || originTip == tip {
		return false, err
	}
	held, err := holdsCommit(codeDir, tip)
	if err != nil || !held {
		return false, err
	}
	originAbsent := originTip == ""
	if !originAbsent {
		if _, err := repo.FetchWithArgs(originRemoteName, originTip); err != nil {
			return false, fmt.Errorf("fetch the tip %s of branch %q from %q: %w", originTip, branch, originRemoteName, err)
		}
		originHolds, err := isAncestorOrEqual(codeDir, tip, originTip)
		if err != nil || originHolds {
			return false, err
		}
		originLags, err := isAncestorOrEqual(codeDir, originTip, tip)
		if err != nil {
			return false, err
		}
		if !originLags {
			logger.Warn("fabricengine: origin's code branch diverged from the owed tip, so the tip is left to its archive tag", "branch", branch, "tip", tip, "origin", originTip)
			return false, nil
		}
	}

	move := func(sha, leaseSHA string) error {
		return updateRemoteBranch(rec, remoteBranchUpdateRequest{
			pathReq: pathRequest{
				what:      "move the code branch of the run's pair on origin",
				container: filepath.Dir(codeDir),
				target:    codeDir,
				ownership: ownedPairWarpCheckout(codeDir, weftPath, ""),
				dirtiness: dirtinessNA("a move of origin's branch touches no checkout file, and the worktree still holds the uncommitted files the archive keeps"),
			},
			repo:     repo,
			remote:   originRemoteName,
			branch:   branch,
			sha:      sha,
			leaseSHA: leaseSHA,
		})
	}
	if originAbsent {
		if err := repo.PushCommit(originRemoteName, tip, branch, gitrepo.RefNamespace{}); err != nil {
			return false, fmt.Errorf("create origin's code branch %s at the owed tip %s: %w; way forward: the next rejected push of the run hands it over again", branch, tip, err)
		}
		rec.AppendRef(fabriclayout.KindRemoteBranchUpdated, originRemoteName+"/"+branch, tip)
	} else if err := move(tip, originTip); err != nil {
		if errors.Is(err, gitrepo.ErrLeaseRejected) {
			return false, fmt.Errorf("origin's code branch %s moved during the hand-over, so the owed tip %s was not pushed; way forward: the next rejected push of the run hands it over again", branch, tip)
		}
		return false, err
	}
	if owedTipPushedHook != nil {
		owedTipPushedHook()
	}

	if err := gitrepo.New(weftPath).Fetch(); err != nil {
		return false, fmt.Errorf("fetch the records branch after the owed tip's push: %w", err)
	}
	upstream, hasUpstream, err = upstreamSHAAt(weftPath)
	if err != nil || !hasUpstream {
		return false, err
	}
	named, err := lastTransitionCodeTip(weftPath, upstream, slug)
	if err != nil {
		return false, err
	}
	if named == tip {
		return true, nil
	}
	if originAbsent {
		if err := deleteGivenUpCodeBranch(rec, codeDir, slug, branch, tip); err != nil {
			return false, fmt.Errorf("delete origin's code branch %s the given-up tip %s created: %w", branch, tip, err)
		}
		return false, nil
	}
	if err := move(originTip, tip); err != nil {
		return false, fmt.Errorf("move origin's code branch %s back off the given-up tip %s: %w", branch, tip, err)
	}
	return false, nil
}

// deleteGivenUpCodeBranch deletes branch on origin, which an owed tip's push created, under a lease on tip.
// The tip is first saved under its owner archive tag pushed to origin, so the delete loses no work.
func deleteGivenUpCodeBranch(rec *Mutations, codeDir, slug, branch, tip string) error {
	tag := fabriclayout.OwnerArchiveTagName(slug, tip)
	if err := createTagAt(codeDir, tag, tip, "code"); err != nil {
		return err
	}
	if err := pushArchiveTag(rec, codeDir, tag, nil); err != nil {
		return err
	}
	_, err := deleteRemoteBranch(rec, remoteBranchRequest{
		what:      "delete the code branch an owed tip's push created on origin",
		repoDir:   codeDir,
		remote:    originRemoteName,
		branch:    branch,
		ownership: ownedPairWarpBranch(branch, ""),
		dirtiness: dirtyArchivedOnRemote(tag),
		leaseSHA:  tip,
	})
	return err
}

// lastTransitionCodeTip returns the code tip the newest commit on ref that marks slug's transition names.
// A commit marks it when its Run-Transition trailer is slug; the tip is that commit's Warp-SHA trailer.
// Fabric's other records commits carry a Warp-SHA trailer too and are skipped, as is any other run's transition.
// It returns an empty tip when no commit on ref's first-parent line marks a transition of slug.
func lastTransitionCodeTip(weftPath, ref, slug string) (tip string, err error) {
	commit, found, err := lastTransitionCommit(weftPath, ref, slug)
	if err != nil || !found {
		return "", err
	}
	return trailerValue(commit.Message, fabriclayout.WarpSHATrailerKey), nil
}

// lastTransitionCommit returns the newest commit on ref's first-parent line whose Run-Transition trailer is slug.
// It reports found false when no commit on that line marks a transition of slug.
func lastTransitionCommit(weftPath, ref, slug string) (commit gitrepo.LogCommit, found bool, err error) {
	commits, err := gitrepo.New(weftPath).Log(ref, gitrepo.LogQuery{FirstParent: true})
	if err != nil {
		return gitrepo.LogCommit{}, false, fmt.Errorf("read the records history of %q at %s: %w", slug, ref, err)
	}
	for _, candidate := range commits {
		if trailerValue(candidate.Message, fabriclayout.TransitionTrailerKey) == slug {
			return candidate, true, nil
		}
	}
	return gitrepo.LogCommit{}, false, nil
}

// trailerValue returns the value of the last "<key>: <value>" line of message, or "" when it has none.
func trailerValue(message, key string) string {
	value := ""
	for _, line := range strings.Split(message, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), key+":"); found && strings.TrimSpace(rest) != "" {
			value = strings.TrimSpace(rest)
		}
	}
	return value
}

// holdsCommit reports whether the repository at dir holds the commit sha.
func holdsCommit(dir, sha string) (bool, error) {
	if _, err := gitexec.Run([]string{"cat-file", "-e", sha + "^{commit}"}, dir); err != nil {
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// parentAheadByOwnMergeOnly reports whether the run's own merge alone puts the parent branch ahead of its upstream.
// That holds when every commit the branch has beyond its upstream is one the pair's code tip reaches,
// or the one extra commit is the branch's tip, a merge.
// A parent branch with no upstream, or holding nothing beyond it, reports false.
func parentAheadByOwnMergeOnly(parentDir, pairDir string) (bool, error) {
	parent := gitrepo.New(parentDir)
	branch, err := parent.CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("read the branch checked out at %s: %w", parentDir, err)
	}
	tip, err := localBranchTip(parentDir, branch)
	if err != nil || tip == "" {
		return false, err
	}
	upstream, hasUpstream, err := parent.Upstream(branch)
	if err != nil || !hasUpstream || upstream == tip {
		return false, err
	}
	ahead, err := parent.CommitsNotIn(tip, upstream)
	if err != nil || len(ahead) == 0 {
		return false, err
	}
	pairTip, err := gitrepo.New(pairDir).CurrentSHA()
	if err != nil {
		return false, fmt.Errorf("read the tip of the pair at %s: %w", pairDir, err)
	}
	ownCommits, err := parent.CommitsNotIn(pairTip, upstream)
	if err != nil {
		return false, err
	}
	var others []string
	for _, commit := range ahead {
		if !slices.Contains(ownCommits, commit) {
			others = append(others, commit)
		}
	}
	switch len(others) {
	case 0:
		return true, nil
	case 1:
		parents, err := parent.CommitParents(others[0])
		if err != nil {
			return false, err
		}
		return others[0] == tip && len(parents) > 1, nil
	}
	return false, nil
}

// archiveParentMerge tags the local tip of the branch checked out at parentDir as archive-owner/<slug>/<sha12>.
// It pushes the tag, and resets the branch to its upstream with `git reset --keep`.
// It returns the tag, and none for a branch with no upstream or no local tip.
// The caller has judged that the run's own merge alone puts the branch ahead of its upstream.
func archiveParentMerge(rec *Mutations, parentDir, slug string) (tag string, err error) {
	repo := gitrepo.New(parentDir)
	branch, err := repo.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("read the branch checked out at %s: %w", parentDir, err)
	}
	tip, err := localBranchTip(parentDir, branch)
	if err != nil || tip == "" {
		return "", err
	}
	upstream, hasUpstream, err := repo.Upstream(branch)
	if err != nil || !hasUpstream {
		return "", err
	}
	tag = fabriclayout.OwnerArchiveTagName(slug, tip)
	if err := createTagAt(parentDir, tag, tip, "parent"); err != nil {
		return "", err
	}
	if err := pushArchiveTag(rec, parentDir, tag, nil); err != nil {
		return "", err
	}
	req := pathRequest{
		what:      "reset the parent branch to its upstream under an archive tag",
		container: filepath.Dir(parentDir),
		target:    parentDir,
		ownership: ownedWarpCheckout(parentDir),
		dirtiness: dirtinessNA("git reset --keep carries uncommitted changes across and refuses by itself where one touches a file the reset moves"),
	}
	if err := resetKeepUnderArchive(rec, req, repo, upstream, tag); err != nil {
		return "", fmt.Errorf("reset the parent branch %s to its upstream: %w", branch, err)
	}
	return tag, nil
}

// ArchiveParentMerge archives the local tip of the parent pair's branch and resets the branch to origin's tip.
// It is for a parent push origin rejected because origin's parent branch already holds the run's work.
// It fetches first, under the slug's teardown lock taken exclusive.
// A branch with no upstream, or one whose tip origin already holds, archives nothing and returns an empty tag.
// Otherwise the tip goes under an owner archive tag pushed to origin, and the branch is reset to origin's tip.
func ArchiveParentMerge(parent *lymcwd.Location, slug string) (tag string, err error) {
	hubPath := parent.HubPath
	if err := ensureTeardownLockDir(hubPath, slug); err != nil {
		return "", err
	}
	held, acquired, err := lock.AcquireWriteLockWithin(fabriclayout.TeardownLockPath(hubPath, slug), fabriclayout.StatusPushLockWait)
	if err != nil {
		return "", fmt.Errorf("take the teardown lock of the run %q: %w", slug, err)
	}
	if !acquired {
		return "", fmt.Errorf("a push or removal of the pair %q still holds its teardown lock: %w", slug, ErrPairPushRunning)
	}
	defer func() { _ = held.Release() }()

	parentDir := parent.WorktreePath()
	repo := gitrepo.New(parentDir)
	if err := repo.Fetch(); err != nil {
		return "", fmt.Errorf("fetch the parent branch at %s: %w", parentDir, err)
	}
	branch, err := repo.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("read the branch checked out at %s: %w", parentDir, err)
	}
	tip, err := localBranchTip(parentDir, branch)
	if err != nil || tip == "" {
		return "", err
	}
	upstream, hasUpstream, err := repo.Upstream(branch)
	if err != nil || !hasUpstream {
		return "", err
	}
	originHolds, err := isAncestorOrEqual(parentDir, tip, upstream)
	if err != nil || originHolds {
		return "", err
	}
	return archiveParentMerge(NewMutations(hubPath), parentDir, slug)
}
