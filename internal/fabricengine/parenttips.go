// parenttips.go implements the code-side reads a parent-merge needs — a branch's tips and their relation, the paths between two commits and the holds-work test — and FastForwardPair, the fast-forward-only move of a pair to its upstream.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

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

// ReadParentTips returns the tips of branch in the pair's code repository, fetching the repository's remotes first when fetch is true.
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

// ChangedPaths returns the sorted repository-relative paths that differ between from and to in the pair's code repository.
func (f *Fabric) ChangedPaths(from, to string) ([]string, error) {
	paths, err := diffNames(f.warpPath, from, to, nil)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: changed paths from %s to %s: %w", from, to, err)
	}
	return paths, nil
}

// CommitLog lists the commits reachable from to and not from from in the pair's code repository, newest first, each as a seven-character SHA prefix and the commit's subject.
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

// shortSHALength is how many leading characters of a SHA CommitLog prints.
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

// FastForwardRefusal names why FastForwardPair left the pair unmoved without an error.
type FastForwardRefusal string

const (
	// FastForwardDirty means the code worktree carries uncommitted tracked changes.
	FastForwardDirty FastForwardRefusal = "dirty"
	// FastForwardMergeInProgress means a fabric merge record exists on the pair.
	FastForwardMergeInProgress FastForwardRefusal = "merge-in-progress"
	// FastForwardNotFastForward means the local head is not an ancestor of the freshly fetched upstream.
	FastForwardNotFastForward FastForwardRefusal = "not-fast-forward"
)

// FastForwardResult reports what FastForwardPair did.
// It embeds MutationRecord, which is empty on a refusal.
type FastForwardResult struct {
	MutationRecord
	// Refusal is set when the pair was left unmoved, and is empty otherwise.
	Refusal FastForwardRefusal
	// NewHead is the upstream SHA the code side moved to, empty when it did not move.
	NewHead string
}

// FastForwardPair moves the pair to its upstream only as a fast-forward and never rewrites it.
// A merge record, a dirty code worktree and a local head that is not an ancestor of the freshly fetched upstream are refusals: nil error, nothing moved.
// Once none applies it pulls the weft first, where a failure there is logged and does not stop the code side, then resets the code side to the upstream the fetch returned.
// opts.SkipGit skips the weft side.
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
	dirty, err := f.warpWorktreeDirty()
	if err != nil {
		return FastForwardResult{}, err
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
		return FastForwardResult{Refusal: FastForwardNotFastForward}, nil
	}

	if !opts.SkipGit {
		f.fastForwardWeft(rec, opts)
	}

	if err := f.ResetHard(rec, upstreamSHA); err != nil {
		return FastForwardResult{}, err
	}
	f.recordWarpAdvance(rec, localHEAD)
	return FastForwardResult{NewHead: upstreamSHA}, nil
}

// fastForwardWeft pulls the weft the way Pull's weft arm does: a weft with no upstream is skipped, a failure is logged, and a weft HEAD that moved appends KindRepoAdvanced.
func (f *Fabric) fastForwardWeft(rec *Mutations, opts SyncOptions) {
	hasUpstream, err := f.weftHasUpstream()
	if err != nil {
		logger.Warn("fabricengine: fast-forward: resolve weft upstream failed, continuing to the code side", "weft", f.weftPath, "err", err)
		return
	}
	if !hasUpstream {
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
