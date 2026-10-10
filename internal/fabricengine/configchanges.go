// configchanges.go implements ReadConfigChanges, the read-only diff of a task's per-worktree config files since the task's fork point.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ConfigChanges is the set of per-worktree config files a task changed on its weft branch away from both its fork point and its parent's current copy.
type ConfigChanges struct {
	// Files are the changed files, repository-relative and slash-separated, sorted.
	Files []string
	// Base is the merge-base SHA of the task's and the parent's weft branches, the task's fork point.
	Base string
	// Tip is the SHA of the task's weft branch tip.
	Tip string
	// ParentTip is the SHA of the parent's weft branch tip.
	ParentTip string
}

// ReadConfigChanges reports which of configRels changed on taskBranch's weft branch away from both its fork point from parentBranch's weft branch and the parent's current tip.
// configRels are anchor-relative paths the caller supplies;
// an empty configRels returns an empty ConfigChanges without running git.
// A file is reported only when the task tip differs from the merge-base and from the parent's tip,
// so a parent-side change the task never took, one it took through a sync, and a task edit the parent holds identically all drop out.
// A failure is wrapped naming both branches compared.
// It writes nothing.
func ReadConfigChanges(l *lyxcwd.Location, taskBranch, parentBranch string, configRels []string) (ConfigChanges, error) {
	if len(configRels) == 0 {
		return ConfigChanges{}, nil
	}

	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return ConfigChanges{}, fmt.Errorf("read config changes of %q against %q: %w", taskBranch, parentBranch, err)
	}

	taskWeft := RecordsBranchName(taskBranch)
	parentWeft := RecordsBranchName(parentBranch)
	fail := func(step string, err error) (ConfigChanges, error) {
		return ConfigChanges{}, fmt.Errorf("read config changes of weft branch %q against %q: %s: %w", taskWeft, parentWeft, step, err)
	}

	tip, err := revParseBranch(weftRepoRoot, taskWeft)
	if err != nil {
		return fail("resolve task branch", err)
	}
	parentTip, err := revParseBranch(weftRepoRoot, parentWeft)
	if err != nil {
		return fail("resolve parent branch", err)
	}
	base, err := gitexec.Run([]string{"merge-base", parentTip, tip}, weftRepoRoot)
	if err != nil {
		return fail("find merge-base", err)
	}
	base = strings.TrimSpace(base)

	pathspec := []string{"--"}
	for _, spec := range ScopedPathspec(l.AnchorRel, configRels) {
		pathspec = append(pathspec, filepath.ToSlash(spec))
	}
	sinceFork, err := diffNames(weftRepoRoot, base, tip, pathspec)
	if err != nil {
		return fail("diff against the fork point", err)
	}
	fromParent, err := diffNames(weftRepoRoot, parentTip, tip, pathspec)
	if err != nil {
		return fail("diff against the parent tip", err)
	}

	var files []string
	for _, name := range sinceFork {
		if slices.Contains(fromParent, name) {
			files = append(files, name)
		}
	}
	return ConfigChanges{Files: files, Base: base, Tip: tip, ParentTip: parentTip}, nil
}

// diffNames lists the sorted paths `git diff --name-only from to` reports in the repo at dir, limited by pathspec.
func diffNames(dir, from, to string, pathspec []string) ([]string, error) {
	out, err := gitexec.Run(append([]string{"diff", "--name-only", from, to}, pathspec...), dir)
	if err != nil {
		return nil, err
	}
	var names []string
	if changed := strings.TrimSpace(out); changed != "" {
		names = strings.Split(changed, "\n")
		slices.Sort(names)
	}
	return names, nil
}

// revParseBranch resolves refs/heads/<branch> in the repo at dir to its SHA.
func revParseBranch(dir, branch string) (string, error) {
	out, err := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + branch}, dir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
