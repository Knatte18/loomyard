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

// ConfigChanges is the set of per-worktree config files a task changed on its weft branch since it forked from its parent.
type ConfigChanges struct {
	// Files are the changed files, repository-relative and slash-separated, sorted.
	Files []string
	// Base is the merge-base SHA of the task's and the parent's weft branches, the task's fork point.
	Base string
	// Tip is the SHA of the task's weft branch tip.
	Tip string
}

// ReadConfigChanges reports which of configRels changed on taskBranch's weft branch since it forked from parentBranch's weft branch.
// configRels are anchor-relative paths the caller supplies; an empty configRels returns an empty ConfigChanges without running git.
// The diff runs from the merge-base to the task tip, so a commit on the parent's side after the fork never appears.
// A failure is wrapped naming both branches compared.
// It writes nothing.
func ReadConfigChanges(l *lyxcwd.Location, taskBranch, parentBranch string, configRels []string) (ConfigChanges, error) {
	if len(configRels) == 0 {
		return ConfigChanges{}, nil
	}

	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return ConfigChanges{}, fmt.Errorf("read config changes of %q against %q: %w", taskBranch, parentBranch, err)
	}

	taskWeft := WeftBranchName(taskBranch)
	parentWeft := WeftBranchName(parentBranch)
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

	args := []string{"diff", "--name-only", base, tip, "--"}
	for _, spec := range ScopedPathspec(l.AnchorRel, configRels) {
		args = append(args, filepath.ToSlash(spec))
	}
	out, err := gitexec.Run(args, weftRepoRoot)
	if err != nil {
		return fail("diff", err)
	}

	var files []string
	if changed := strings.TrimSpace(out); changed != "" {
		files = strings.Split(changed, "\n")
		slices.Sort(files)
	}
	return ConfigChanges{Files: files, Base: base, Tip: tip}, nil
}

// revParseBranch resolves refs/heads/<branch> in the repo at dir to its SHA.
func revParseBranch(dir, branch string) (string, error) {
	out, err := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + branch}, dir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
