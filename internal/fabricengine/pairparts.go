// pairparts.go holds the read-only reads of what is left of a pair.
// It reports which local parts remain and whether the task branch is on origin.
// It also lists the task branches a slug prefix names.

package fabricengine

import (
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PairParts reports which parts of slug's pair are present, read locally.
func (t *Topology) PairParts(l *lymcwd.Location, slug string) (fabriclayout.PairParts, error) {
	state, err := t.inspectPair(l, slug)
	if err != nil {
		return fabriclayout.PairParts{}, err
	}
	return fabriclayout.PairParts{
		TaskWorktree:    state.taskWorktree,
		RecordsWorktree: state.siblingWorktree,
		RecordsBranch:   state.siblingBranch,
		TaskBranch:      state.taskBranch,
		StrayPath:       state.strayPath,
	}, nil
}

// TaskBranchOnOrigin reports whether slug's task branch exists on the task repository's origin.
// It matches the branch under the current branch prefix, or the bare slug of a pair made before the prefix was set.
// Failing both, it matches the one branch another machine's prefix names, whose records branch the weft origin holds;
// more than one such branch is an error.
// A repository with no origin remote reads false, and a failed lookup on a configured origin is returned.
func (t *Topology) TaskBranchOnOrigin(l *lymcwd.Location, slug string) (bool, error) {
	found, err := t.originTaskBranch(l, slug)
	return found != "", err
}

// TaskBranchSlugs lists the slugs starting with prefix whose task branch exists in the prime's warp repository.
// A task branch counts as a local branch or as an origin remote-tracking branch.
// The result is sorted and has no duplicates.
// A branch carrying the configured branch prefix names the slug after it.
// A branch lacking it names itself, as a pair made before the prefix was set does.
// It reads the branch prefix from the fabric config and the refs in-process,
// so the remote-tracking branches are those the last fetch left, and no network call is made.
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
