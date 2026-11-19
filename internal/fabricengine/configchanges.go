// configchanges.go reports the per-worktree config files a task changed since its fork point and differ from its parent.

package fabricengine

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ConfigChanges is the set of per-worktree config files a task changed on its weft branch.
// A change counts only when the task's copy differs from both its fork point and its parent's current copy.
type ConfigChanges struct {
	// Files are the changed files, repository-relative and slash-separated, sorted.
	Files []string
	// Base is the merge-base SHA of the task's and the parent's weft branches, the task's fork point.
	Base string
	// Tip is the SHA of the task's weft branch tip.
	Tip string
	// ParentTip is the SHA of the parent's weft branch tip.
	ParentTip string
	// Contents holds, per file in Files, the file's bytes at each of the three revisions.
	Contents map[string]ConfigFileContents
}

// ConfigFileContents is one config file's bytes at the task's fork point, the task's tip and the parent's tip;
// a file absent at a revision is nil there.
type ConfigFileContents struct {
	Base, Tip, Parent []byte
}

// ReadConfigChanges reports which of configRels taskBranch's weft branch changed since its fork point with parentBranch;
// a file counts only when it also differs from the parent's current tip.
// configRels are anchor-relative paths the caller supplies.
// An empty configRels returns an empty ConfigChanges without running git.
// For each reported file it also reads the bytes at the fork point, the task tip and the parent tip,
// so the caller can compare the files key by key.
// An error names both branches compared.
// It writes nothing.
func ReadConfigChanges(l *lymcwd.Location, taskBranch, parentBranch string, configRels []string) (ConfigChanges, error) {
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

	contents := make(map[string]ConfigFileContents, len(files))
	for _, name := range files {
		var file ConfigFileContents
		for _, read := range []struct {
			rev string
			dst *[]byte
		}{{base, &file.Base}, {tip, &file.Tip}, {parentTip, &file.Parent}} {
			if *read.dst, err = showFile(weftRepoRoot, read.rev, name); err != nil {
				return fail("read "+name+" at "+read.rev, err)
			}
		}
		contents[name] = file
	}
	return ConfigChanges{Files: files, Base: base, Tip: tip, ParentTip: parentTip, Contents: contents}, nil
}

// showFile returns the bytes of path at rev in the repo at dir, or nil when rev holds no such file.
// rev is a full commit SHA or a name that resolves to a commit, such as HEAD or a full ref name.
// It reads the object store in-process, with no git process, and returns the blob's bytes unchanged.
func showFile(dir, rev, path string) ([]byte, error) {
	repo := gitrepo.New(dir)
	sha := rev
	if _, err := hex.DecodeString(rev); err != nil || (len(rev) != sha1HexLength && len(rev) != sha256HexLength) {
		if sha, err = repo.ResolveSHA(rev); err != nil {
			return nil, err
		}
	}
	data, err := repo.FileAtRevision(sha, path)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// The lengths of a full object name in hex, for a SHA-1 and a SHA-256 repository.
const (
	sha1HexLength   = 40
	sha256HexLength = 64
)

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
	return gitrepo.New(dir).RefSHA("refs/heads/" + branch)
}
