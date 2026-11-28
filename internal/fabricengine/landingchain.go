// landingchain.go holds the git steps of landing several branches at once:
// the chain of landing commits, the clean-merge read, the guarded move onto the chain and the newest-landing read.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ChainLink is one branch to land on a chain.
type ChainLink struct {
	// Slug names the run that lands.
	Slug string
	// TaskHead is the commit whose work the link lands.
	TaskHead string
	// Message is the whole landing commit message, its landing trailer included.
	Message string
	// TrueMerge keeps TaskHead as a second parent of the landing commit; otherwise the landing squashes.
	TrueMerge bool
}

// ChainCommit is the landing commit of one run on a chain.
type ChainCommit struct {
	Slug   string
	Commit string
}

// ChainResult is a built landing chain and the links it left out, each list in link order.
type ChainResult struct {
	Commits []ChainCommit
	// Conflicted are the slugs whose work does not merge cleanly onto the chain.
	Conflicted []string
	// Held are the slugs whose work the chain already holds.
	Held []string
}

// BuildLandingChain writes one landing commit per link, each on the one before it and the first on base.
// A link whose work the chain already holds, or which does not merge cleanly, adds no commit.
// The next link then merges onto the same commit.
// It writes commit objects alone, moving no ref and touching neither worktree nor index.
// base and every task head are resolved before the first commit is written.
func (f *Fabric) BuildLandingChain(base string, links []ChainLink) (ChainResult, error) {
	previous, err := f.code.ResolveSHA(base)
	if err != nil {
		return ChainResult{}, fmt.Errorf("fabricengine: landing chain: resolve base %s: %w", base, err)
	}
	heads := make([]string, len(links))
	for i, link := range links {
		if heads[i], err = f.code.ResolveSHA(link.TaskHead); err != nil {
			return ChainResult{}, fmt.Errorf("fabricengine: landing chain: resolve the task head of %s: %w", link.Slug, err)
		}
	}

	var result ChainResult
	for i, link := range links {
		fail := func(err error) (ChainResult, error) {
			return ChainResult{}, fmt.Errorf("fabricengine: landing chain: land %s: %w", link.Slug, err)
		}
		held, err := f.HoldsWork(previous, heads[i])
		if err != nil {
			return fail(err)
		}
		if held {
			result.Held = append(result.Held, link.Slug)
			continue
		}
		tree, clean, err := f.code.MergeTree(previous, heads[i])
		if err != nil {
			return fail(err)
		}
		if !clean {
			result.Conflicted = append(result.Conflicted, link.Slug)
			continue
		}
		parents := []string{previous}
		if link.TrueMerge {
			parents = append(parents, heads[i])
		}
		commit, err := f.code.WriteCommitTree(tree, parents, link.Message)
		if err != nil {
			return fail(err)
		}
		result.Commits = append(result.Commits, ChainCommit{Slug: link.Slug, Commit: commit})
		previous = commit
	}
	return result, nil
}

// MergesClean reports whether merging source into base stops on no conflict, without changing anything.
// Each may be a branch or a SHA, and one that does not resolve is an error.
func (f *Fabric) MergesClean(base, source string) (bool, error) {
	baseSHA, err := f.code.ResolveSHA(base)
	if err != nil {
		return false, fmt.Errorf("fabricengine: clean-merge test: %w", err)
	}
	sourceSHA, err := f.code.ResolveSHA(source)
	if err != nil {
		return false, fmt.Errorf("fabricengine: clean-merge test: %w", err)
	}
	_, clean, err := f.code.MergeTree(baseSHA, sourceSHA)
	if err != nil {
		return false, fmt.Errorf("fabricengine: clean-merge test: %w", err)
	}
	return clean, nil
}

// MoveCodeTo fast-forwards the checked-out code branch and its worktree from base to tip, and reports whether it did.
// A branch whose head is not base is left as it is.
// A detached HEAD, or tracked uncommitted changes, are refused with a *MergeGuardError.
// A tip that does not descend from base is an error.
// The records side is never moved.
func (f *Fabric) MoveCodeTo(base, tip string) (bool, error) {
	fail := func(err error) (bool, error) {
		return false, fmt.Errorf("fabricengine: move the code branch from %s to %s: %w", base, tip, err)
	}
	baseSHA, err := f.code.ResolveSHA(base)
	if err != nil {
		return fail(err)
	}
	tipSHA, err := f.code.ResolveSHA(tip)
	if err != nil {
		return fail(err)
	}

	reasons, err := detachedHeadReason(f)
	if err != nil {
		return fail(err)
	}
	if len(reasons) == 0 {
		head, err := f.code.CurrentSHA()
		if err != nil {
			return fail(err)
		}
		if head != baseSHA {
			return false, nil
		}
	}
	dirtyReasons, dirtyPaths, err := pairDirtyReason(f)
	if err != nil {
		return fail(err)
	}
	if reasons = append(reasons, dirtyReasons...); len(reasons) > 0 {
		return false, newMergeGuardError(reasons).withDirt(f.warpPath, dirtyPaths)
	}

	descends, err := f.code.IsAncestor(baseSHA, tipSHA)
	if err != nil {
		return fail(err)
	}
	if !descends {
		return false, fmt.Errorf("fabricengine: move the code branch: %s does not descend from %s", tip, base)
	}
	if err := f.code.MergeFFOnly(tipSHA); err != nil {
		return fail(err)
	}
	return true, nil
}

// NewestLandingOf returns the newest commit on branch's first-parent history whose landing trailer names slug.
// The boolean reports whether such a commit was found.
// It reads the repository of the worktree l, commits origin holds included.
func NewestLandingOf(l *lymcwd.Location, branch, slug string) (string, bool, error) {
	out, err := gitexec.Run([]string{"log", "--first-parent", "-z", "--format=%H%n%B", "refs/heads/" + branch, "--"}, l.WorktreePath())
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: find the landing of %s on %s: %w", slug, branch, err)
	}
	for _, entry := range strings.Split(out, "\x00") {
		sha, message, _ := strings.Cut(strings.TrimLeft(entry, "\n"), "\n")
		if sha != "" && trailerValue(message, fabriclayout.LandingTrailerKey) == slug {
			return sha, true, nil
		}
	}
	return "", false, nil
}
