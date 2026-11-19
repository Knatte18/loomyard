// parenttips.go holds the code-side reads a parent-merge needs.
// They are a branch's tips and their relation, the paths between two commits, and the holds-work test.
// It also holds FastForwardPair, the fast-forward-only move of a pair to its upstream.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ParentRelation names how a branch's local tip stands to its upstream tip.
type ParentRelation string

const (
	// ParentEqual means the local and upstream tips are the same commit.
	ParentEqual ParentRelation = "equal"
	// ParentBehind means local is an ancestor of upstream, so a fast-forward reaches upstream.
	ParentBehind ParentRelation = "behind"
	// ParentAhead means upstream is an ancestor of local.
	ParentAhead ParentRelation = "ahead"
	// ParentDiverged means each tip holds a commit the other lacks.
	ParentDiverged ParentRelation = "diverged"
	// ParentNoUpstream means the branch tracks no remote branch, or the tracked one was never fetched.
	ParentNoUpstream ParentRelation = "no-upstream"
)

// ParentTips is a branch's local and upstream tips and how they relate.
type ParentTips struct {
	// Local is the SHA of refs/heads/<branch>.
	Local string
	// Upstream is the SHA of the remote-tracking ref the branch tracks, empty when Relation is ParentNoUpstream.
	Upstream string
	// Relation is how Local stands to Upstream.
	Relation ParentRelation
}

// ReadParentTips returns the tips of branch in the pair's code repository.
// When fetch is true it fetches the repository's remotes first.
// A missing local branch is an error naming it; a failed fetch is an error naming the command that reproduces it.
// A branch with no configured or no fetched upstream returns ParentNoUpstream with an empty Upstream.
func (f *Fabric) ReadParentTips(branch string, fetch bool) (ParentTips, error) {
	if fetch {
		if err := f.code.Fetch(); err != nil {
			return ParentTips{}, fmt.Errorf("fabricengine: read tips of %q: %w", branch, err)
		}
	}

	local, err := f.code.RefSHA("refs/heads/" + branch)
	if err != nil {
		return ParentTips{}, fmt.Errorf("fabricengine: read tips of %q: local branch: %w", branch, err)
	}
	upstream, hasUpstream, err := f.code.Upstream(branch)
	if err != nil {
		return ParentTips{}, fmt.Errorf("fabricengine: read tips of %q: upstream: %w", branch, err)
	}
	if !hasUpstream {
		return ParentTips{Local: local, Relation: ParentNoUpstream}, nil
	}

	tips := ParentTips{Local: local, Upstream: upstream}
	if local == upstream {
		tips.Relation = ParentEqual
		return tips, nil
	}
	behind, err := f.code.IsAncestor(local, upstream)
	if err != nil {
		return ParentTips{}, fmt.Errorf("fabricengine: read tips of %q: classify: %w", branch, err)
	}
	if behind {
		tips.Relation = ParentBehind
		return tips, nil
	}
	ahead, err := f.code.IsAncestor(upstream, local)
	if err != nil {
		return ParentTips{}, fmt.Errorf("fabricengine: read tips of %q: classify: %w", branch, err)
	}
	if ahead {
		tips.Relation = ParentAhead
		return tips, nil
	}
	tips.Relation = ParentDiverged
	return tips, nil
}

// LocalOnlyCommits lists the SHAs of the local branch's commits that its tracking ref lacks, newest first.
// The tracking ref is as the last fetch left it.
// A branch level with, or behind, its tracking ref lists none.
// A branch with no tracking ref lists none either, since there is nothing to compare against.
// A missing local branch is an error naming it.
func (f *Fabric) LocalOnlyCommits(branch string) ([]string, error) {
	local, err := f.code.RefSHA("refs/heads/" + branch)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: local-only commits of %q: local branch: %w", branch, err)
	}
	upstream, hasUpstream, err := f.code.Upstream(branch)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: local-only commits of %q: upstream: %w", branch, err)
	}
	if !hasUpstream {
		return nil, nil
	}
	commits, err := f.code.CommitsNotIn(local, upstream)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: local-only commits of %q: %w", branch, err)
	}
	return commits, nil
}

// ChangedPaths returns the paths that differ between from and to in the pair's code repository.
// The paths are repository-relative and sorted.
func (f *Fabric) ChangedPaths(from, to string) ([]string, error) {
	paths, err := diffNames(f.warpPath, from, to, nil)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: changed paths from %s to %s: %w", from, to, err)
	}
	return paths, nil
}

// ChangedCodePaths returns the paths that differ between from and to in the pair's code repository, leaving out records.
// A path is left out when it is a records path of to or lies under one, as a one-repository hub keeps its records there.
// The paths are repository-relative and sorted.
func (f *Fabric) ChangedCodePaths(from, to string) ([]string, error) {
	paths, err := f.ChangedPaths(from, to)
	if err != nil {
		return nil, err
	}
	recordsPaths, err := RecordsPathsAt(f.warpPath, to)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: changed code paths from %s to %s: %w", from, to, err)
	}
	return slices.DeleteFunc(paths, func(path string) bool { return underRecordsPath(path, recordsPaths) }), nil
}

// ChangedPathsBothSides returns the paths that differ between from and to in the pair's code repository.
// The paths are repository-relative, sorted and deduplicated.
// A rename is reported by both its old and its new path, so a move out of a tree is seen by the tree it left.
// Each path is read from NUL-separated output and arrives verbatim, never in core.quotePath's C-quoted form.
func (f *Fabric) ChangedPathsBothSides(from, to string) ([]string, error) {
	out, err := gitexec.Run([]string{"diff", "--name-only", "-z", "--no-renames", from, to}, f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: changed paths from %s to %s: %w", from, to, err)
	}
	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// CommitLog lists the commits reachable from to and not from from in the pair's code repository, newest first.
// Each entry is a seven-character SHA prefix and the commit's subject.
// An empty range returns an empty slice, and a ref that does not resolve is an error naming it.
func (f *Fabric) CommitLog(from, to string) ([]string, error) {
	fromSHA, err := f.code.ResolveSHA(from)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: commit log from %s to %s: %w", from, to, err)
	}
	toSHA, err := f.code.ResolveSHA(to)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: commit log from %s to %s: %w", from, to, err)
	}
	shas, err := f.code.CommitsNotIn(toSHA, fromSHA)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: commit log from %s to %s: %w", from, to, err)
	}
	lines := make([]string, 0, len(shas))
	for _, sha := range shas {
		detail, err := f.code.CommitDetail(sha)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: commit log from %s to %s: %w", from, to, err)
		}
		subject, _, _ := strings.Cut(detail.Message, "\n")
		lines = append(lines, sha[:shortSHALength]+" "+subject)
	}
	return lines, nil
}

// CommitMessages lists the whole message of every commit reachable from to and not from from.
// The commits come from the pair's code repository, newest first.
// A message keeps its body and trailer paragraphs.
// An empty range returns an empty slice, and a ref that does not resolve is an error naming it.
func (f *Fabric) CommitMessages(from, to string) ([]string, error) {
	fromSHA, err := f.code.ResolveSHA(from)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: commit messages from %s to %s: %w", from, to, err)
	}
	toSHA, err := f.code.ResolveSHA(to)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: commit messages from %s to %s: %w", from, to, err)
	}
	shas, err := f.code.CommitsNotIn(toSHA, fromSHA)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: commit messages from %s to %s: %w", from, to, err)
	}
	messages := make([]string, 0, len(shas))
	for _, sha := range shas {
		detail, err := f.code.CommitDetail(sha)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: commit messages from %s to %s: %w", from, to, err)
		}
		messages = append(messages, detail.Message)
	}
	return messages, nil
}

// ErrBranchUnresolved marks a FirstParentCommits error whose branch resolves to no commit.
var ErrBranchUnresolved = errors.New("the branch resolves to no commit")

// ErrCommitUnresolved marks a FirstParentCommits error whose since commit resolves to no commit.
var ErrCommitUnresolved = errors.New("the commit resolves to no commit")

// FirstParentCommit is one commit of a branch's first-parent history with its first parent.
type FirstParentCommit struct {
	// SHA is the commit's full SHA.
	SHA string
	// Parent is the full SHA of the commit's first parent, empty for a root commit.
	Parent string
}

// FirstParentCommits returns the commits of branch's first-parent history newer than since, newest first.
// branch and since are each resolved to a commit in the pair's code repository.
// Each may be a branch, a remote-tracking ref or a SHA.
// The result is true when the walk met since, and holds the commits before it.
// The walk stops after limit+1 commits it has not met since in, so a longer range returns limit+1 commits and false.
// A history that ends without meeting since returns false with every commit it holds.
// A branch or since that resolves to no commit is an error wrapping ErrBranchUnresolved or ErrCommitUnresolved.
func (f *Fabric) FirstParentCommits(branch, since string, limit int) ([]FirstParentCommit, bool, error) {
	tip, err := f.code.ResolveSHA(branch)
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: first-parent commits of %q: %w: %v", branch, ErrBranchUnresolved, err)
	}
	sinceSHA, err := f.code.ResolveSHA(since)
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: first-parent commits since %q: %w: %v", since, ErrCommitUnresolved, err)
	}
	history, err := f.code.Log(tip, gitrepo.LogQuery{FirstParent: true, Limit: limit + 1})
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: first-parent commits of %q: %w", branch, err)
	}
	var commits []FirstParentCommit
	for _, entry := range history {
		if entry.SHA == sinceSHA {
			return commits, true, nil
		}
		commit := FirstParentCommit{SHA: entry.SHA}
		if len(entry.Parents) > 0 {
			commit.Parent = entry.Parents[0]
		}
		commits = append(commits, commit)
	}
	return commits, false, nil
}

// HeadHoldsCommitsNotIn reports whether the pair's code HEAD holds a commit that ref lacks.
// A ref that does not resolve is an error naming it.
func (f *Fabric) HeadHoldsCommitsNotIn(ref string) (bool, error) {
	refSHA, err := f.code.ResolveSHA(ref)
	if err != nil {
		return false, fmt.Errorf("fabricengine: HEAD commits not in %s: %w", ref, err)
	}
	headSHA, err := f.code.ResolveSHA("HEAD")
	if err != nil {
		return false, fmt.Errorf("fabricengine: HEAD commits not in %s: %w", ref, err)
	}
	shas, err := f.code.CommitsNotIn(headSHA, refSHA)
	if err != nil {
		return false, fmt.Errorf("fabricengine: HEAD commits not in %s: %w", ref, err)
	}
	return len(shas) > 0, nil
}

// shortSHALength is the number of leading SHA characters a commit log entry shows.
const shortSHALength = 7

// HoldsWork reports whether merging source into base adds nothing: the merge is clean and its tree equals base's tree.
// A conflicted merge holds nothing.
// It holds after a squash landing of source's work onto base, where ancestry does not.
func (f *Fabric) HoldsWork(base, source string) (bool, error) {
	baseSHA, err := f.code.ResolveSHA(base)
	if err != nil {
		return false, fmt.Errorf("fabricengine: holds-work test: %w", err)
	}
	sourceSHA, err := f.code.ResolveSHA(source)
	if err != nil {
		return false, fmt.Errorf("fabricengine: holds-work test: %w", err)
	}
	merged, clean, err := f.code.MergeTree(baseSHA, sourceSHA)
	if err != nil {
		return false, fmt.Errorf("fabricengine: holds-work test: %w", err)
	}
	if !clean {
		return false, nil
	}
	baseTree, err := f.code.CommitTree(baseSHA)
	if err != nil {
		return false, fmt.Errorf("fabricengine: holds-work test: %w", err)
	}
	return merged == baseTree, nil
}

// ConflictsWith reports whether merging ref into the pair's code HEAD stops on a conflict.
// The merge runs in memory and changes no worktree, index or ref.
// A ref that does not resolve is an error naming it.
func (f *Fabric) ConflictsWith(ref string) (bool, error) {
	refSHA, err := f.code.ResolveSHA(ref)
	if err != nil {
		return false, fmt.Errorf("fabricengine: conflict test against %s: %w", ref, err)
	}
	headSHA, err := f.code.ResolveSHA("HEAD")
	if err != nil {
		return false, fmt.Errorf("fabricengine: conflict test against %s: %w", ref, err)
	}
	_, clean, err := f.code.MergeTree(headSHA, refSHA)
	if err != nil {
		return false, fmt.Errorf("fabricengine: conflict test against %s: %w", ref, err)
	}
	return !clean, nil
}

// FastForwardRefusal names why a fast-forward left the pair unmoved without returning an error.
type FastForwardRefusal string

const (
	// FastForwardDirty means the code worktree carries uncommitted tracked changes.
	FastForwardDirty FastForwardRefusal = "dirty"
	// FastForwardMergeInProgress means a fabric merge record exists on the pair.
	FastForwardMergeInProgress FastForwardRefusal = "merge-in-progress"
	// FastForwardNotFastForward means the local head is not an ancestor of the freshly fetched upstream.
	FastForwardNotFastForward FastForwardRefusal = "not-fast-forward"
)

// FastForwardResult reports what a fast-forward did to the pair.
// It embeds MutationRecord, which is empty on a refusal.
type FastForwardResult struct {
	MutationRecord
	// Refusal is set when the pair was left unmoved, and is empty otherwise.
	Refusal FastForwardRefusal
	// NewHead is the upstream SHA the code side moved to, empty when it did not move.
	NewHead string
}

// FastForwardPair moves the pair to its upstream only as a fast-forward and never rewrites it.
// A merge record, a dirty code worktree, or a local head that is not an ancestor of the fetched upstream is a refusal.
// A refusal returns a nil error and moves nothing.
// Once none applies, it pulls the weft first; a failure there is logged and does not stop the code side.
// It then resets the code side to the upstream the fetch returned.
// opts.SkipGit skips the weft side.
// In a one-repository worktree the records are committed first under the records locks.
// Only code paths count as dirt.
// A local head that is not an ancestor of the upstream is then leveled with the upstream.
// The leveling replays its records-only commits onto the upstream.
// Only a local commit that changes a code path, or a replay that stops, is the not-fast-forward refusal.
func (f *Fabric) FastForwardPair(opts SyncOptions) (res FastForwardResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return FastForwardResult{}, err
	}
	if recordExists {
		return FastForwardResult{Refusal: FastForwardMergeInProgress}, nil
	}

	if err := f.code.Fetch(); err != nil {
		return FastForwardResult{}, err
	}
	oneRepo := oneRepositoryAt(f.warpPath)
	var dirty bool
	if oneRepo {
		release, err := f.lockWeftForReplay()
		if err != nil {
			return FastForwardResult{}, err
		}
		defer release()
		if err := f.commitReceivingRecords(rec); err != nil {
			return FastForwardResult{}, err
		}
		dirtyReasons, _, err := pairDirtyReason(f)
		if err != nil {
			return FastForwardResult{}, err
		}
		dirty = len(dirtyReasons) > 0
	} else {
		dirty, err = f.warpWorktreeDirty()
		if err != nil {
			return FastForwardResult{}, err
		}
	}
	if dirty {
		return FastForwardResult{Refusal: FastForwardDirty}, nil
	}

	upstreamSHA, err := f.warpUpstreamSHA()
	if err != nil {
		return FastForwardResult{}, err
	}
	localHEAD, err := f.code.CurrentSHA()
	if err != nil {
		return FastForwardResult{}, err
	}
	isFastForward, err := f.code.IsAncestor(localHEAD, upstreamSHA)
	if err != nil {
		return FastForwardResult{}, err
	}
	if !isFastForward {
		if !oneRepo {
			return FastForwardResult{Refusal: FastForwardNotFastForward}, nil
		}
		return f.levelOneRepositoryWithUpstream(rec, localHEAD, upstreamSHA)
	}

	if !opts.SkipGit && !oneRepo {
		f.fastForwardWeft(rec, opts)
	}

	if err := f.ResetHard(rec, upstreamSHA); err != nil {
		return FastForwardResult{}, err
	}
	f.recordWarpAdvance(rec, localHEAD)
	// The pair took in origin's commits; the worktree seam hears of them.
	// The call logs a failure and never fails the move.
	f.fireTookIn(localHEAD, upstreamSHA)
	return FastForwardResult{NewHead: upstreamSHA}, nil
}

// levelOneRepositoryWithUpstream brings a one-repository worktree whose head is no ancestor of upstreamSHA level with it.
// It replays the local records-only commits onto the upstream, under the records locks its caller holds.
// A head that is ahead of the upstream, a local commit that changes a code path and a stopped replay are refusals.
// A refusal returns a nil error and moves nothing.
func (f *Fabric) levelOneRepositoryWithUpstream(rec *Mutations, localHEAD, upstreamSHA string) (FastForwardResult, error) {
	ahead, err := f.code.IsAncestor(upstreamSHA, localHEAD)
	if err != nil {
		return FastForwardResult{}, err
	}
	if ahead {
		return FastForwardResult{Refusal: FastForwardNotFastForward}, nil
	}
	err = f.replayOneRepositoryRecords(rec, upstreamSHA)
	var codeStop *localCodeCommitsStop
	if errors.Is(err, ErrReplayConflict) || errors.Is(err, ErrReplayDirty) || errors.As(err, &codeStop) {
		return FastForwardResult{Refusal: FastForwardNotFastForward}, nil
	}
	if err != nil {
		return FastForwardResult{}, err
	}
	newHead, err := f.code.CurrentSHA()
	if err != nil {
		return FastForwardResult{}, err
	}
	f.fireTookIn(localHEAD, newHead)
	return FastForwardResult{NewHead: newHead}, nil
}

// fastForwardWeft pulls the weft during a fast-forward.
// A weft with no upstream is skipped.
// A failed pull is logged, and the code side goes on.
// A weft HEAD that moved appends KindRepoAdvanced.
// A weft that carries a move record or an empty own anchor marker is not pulled.
// Neither is a weft whose upstream carries another anchor marker.
// For such a weft, the warning names the way forward.
func (f *Fabric) fastForwardWeft(rec *Mutations, opts SyncOptions) {
	hasUpstream, err := f.weftHasUpstream()
	if err != nil {
		logger.Warn("fabricengine: fast-forward: resolve weft upstream failed, continuing to the code side", "weft", f.weftPath, "err", err)
		return
	}
	if !hasUpstream {
		return
	}
	stop, err := f.weftAnchorStopBeforePull()
	if err != nil {
		logger.Warn("fabricengine: fast-forward: weft anchor comparison failed, continuing to the code side", "weft", f.weftPath, "err", err)
		return
	}
	if stop != nil {
		logger.Warn("fabricengine: fast-forward: weft pull failed, continuing to the code side", "weft", f.weftPath, "err", stop.Error(), "way_forward", stop.WayForward)
		return
	}
	before, beforeErr := weftSHAOrEmpty(f)
	if err := f.PullWeft(opts); err != nil {
		logger.Warn("fabricengine: fast-forward: weft pull failed, continuing to the code side", "weft", f.weftPath, "err", err)
		return
	}
	if beforeErr != nil {
		return
	}
	if after, afterErr := weftSHAOrEmpty(f); afterErr == nil && after != before {
		rec.Append(KindRepoAdvanced, f.weftPath, after)
	}
}
