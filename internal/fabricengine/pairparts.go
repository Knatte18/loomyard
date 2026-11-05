// pairparts.go holds the read-only reads of what is left of a pair: its local parts, whether its task branch is on origin, and which task branches a slug prefix names.

package fabricengine

import (
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PairParts reports which parts of a pair are present, read locally with no network call.
type PairParts struct {
	// TaskWorktree reports a registered linked worktree at the pair's location.
	TaskWorktree bool
	// RecordsWorktree reports a directory at the pair's records worktree path.
	RecordsWorktree bool
	// RecordsBranch reports the local sibling branch.
	RecordsBranch bool
	// TaskBranch reports the local task branch.
	TaskBranch bool
	// Launchers reports the pair's launcher directory.
	Launchers bool
	// StrayPath reports a path at the pair's task worktree location that is no registered linked worktree, such as a directory a late writer refilled after a removal.
	StrayPath bool
}

// Any reports whether any part of the pair is present.
func (p PairParts) Any() bool {
	return p.TaskWorktree || p.RecordsWorktree || p.RecordsBranch || p.TaskBranch || p.Launchers || p.StrayPath
}

// PairParts reads which parts of slug's pair are present locally, by the same reading Remove makes of what is left.
func (t *Topology) PairParts(l *lymcwd.Location, slug string) (PairParts, error) {
	state, err := t.inspectPair(l, slug)
	if err != nil {
		return PairParts{}, err
	}
	return PairParts{
		TaskWorktree:    state.taskWorktree,
		RecordsWorktree: state.siblingWorktree,
		RecordsBranch:   state.siblingBranch,
		TaskBranch:      state.taskBranch,
		Launchers:       state.launchers,
		StrayPath:       state.strayPath,
	}, nil
}

// TaskBranchOnOrigin reports whether slug's task branch exists on the task repository's origin, under the current branch prefix or as the bare slug of a pair made before the prefix was set.
// A repository with no origin remote reads false, and a failed lookup on a configured origin is returned.
func (t *Topology) TaskBranchOnOrigin(l *lymcwd.Location, slug string) (bool, error) {
	found, err := t.originTaskBranch(l, slug)
	return found != "", err
}

// TaskBranchSlugs lists, sorted and without duplicates, the slugs starting with prefix whose task branch exists in the prime's warp repository, as a local branch or as an origin remote-tracking branch.
// A branch carrying the configured branch prefix names the slug after it, and one lacking it names itself, as a pair made before the prefix was set does.
// It reads the branch prefix from the fabric config and the refs in-process, so the remote-tracking branches are the ones the last fetch left and no network call is made.
// An empty result is no error.
func TaskBranchSlugs(prime *lymcwd.Location, prefix string) ([]string, error) {
	cfg, err := LoadConfig(ConfigDirs(prime))
	if err != nil {
		return nil, err
	}
	repo := gitrepo.New(prime.WorktreePath())
	seen := map[string]bool{}
	for _, remote := range []string{"", originRemoteName} {
		branches, err := repo.BranchNames(remote)
		if err != nil {
			return nil, err
		}
		for _, branch := range branches {
			if slug := slugOfBranch(cfg, branch); strings.HasPrefix(slug, prefix) {
				seen[slug] = true
			}
		}
	}
	slugs := make([]string, 0, len(seen))
	for slug := range seen {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs, nil
}
