// leftovers.go removes the files a merge leaves behind in a directory it deleted.
// The ignore rules of the commit the merge started from named those files,
// and no rule names them once the merge took those rules away.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// ignoreFileName is git's per-directory ignore file.
const ignoreFileName = ".gitignore"

// RemoveIgnoredLeftovers removes the leftover files of the directories a merge emptied in l's worktree.
// Such a directory held tracked files at the commit base and holds none now.
// It removes only an untracked file that base's per-directory ignore files ignore;
// no exclude file of the user or of the repository counts.
// A symlink and anything inside a nested repository stay.
// It returns the removed files as sorted slash-separated worktree-relative paths, also beside an error.
func RemoveIgnoredLeftovers(l *lymcwd.Location, base string) ([]string, error) {
	worktree := l.WorktreePath()
	emptied, err := emptiedDirs(worktree, base)
	if err != nil || len(emptied) == 0 {
		return nil, err
	}
	candidates, err := leftoverFiles(worktree, emptied)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	ignored, err := ignoredUnderRulesAt(l.HubPath, worktree, base, emptied, candidates)
	if err != nil {
		return nil, err
	}
	slices.Sort(ignored)

	targets := make([]string, len(ignored))
	for i, rel := range ignored {
		targets[i] = filepath.Join(worktree, filepath.FromSlash(rel))
	}
	rec := NewMutations(l.HubPath)
	for i, target := range targets {
		err := removePath(rec, pathRequest{
			what:      "remove a merge leftover",
			container: worktree,
			target:    target,
			ownership: ownedIgnoredLeftover(worktree, targets),
			dirtiness: dirtinessNA("an untracked file that git ignores under the rules of the commit the merge started from"),
		})
		if err != nil {
			return ignored[:i], fmt.Errorf("fabricengine: %w", err)
		}
	}
	return ignored, nil
}

// emptiedDirs returns the topmost directories of worktree that lost a file since base and hold no tracked file any more.
func emptiedDirs(worktree, base string) ([]string, error) {
	deleted, err := gitexec.Run([]string{"diff", "--no-renames", "--diff-filter=D", "--name-only", "-z", base, "HEAD", "--"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list the files deleted since %s in %s: %w", base, worktree, err)
	}
	tracked, err := gitexec.Run([]string{"ls-files", "-z"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list the tracked files of %s: %w", worktree, err)
	}
	holdsTracked := map[string]bool{}
	for _, file := range nulFields(tracked) {
		for dir := path.Dir(file); dir != "."; dir = path.Dir(dir) {
			holdsTracked[dir] = true
		}
	}

	var emptied []string
	for _, file := range nulFields(deleted) {
		dir := path.Dir(file)
		if dir == "." {
			continue
		}
		prefix := ""
		for _, segment := range strings.Split(dir, "/") {
			prefix = path.Join(prefix, segment)
			if !holdsTracked[prefix] {
				emptied = append(emptied, prefix)
				break
			}
		}
	}
	slices.Sort(emptied)
	return slices.Compact(emptied), nil
}

// leftoverFiles returns the regular files under each of dirs, as slash-separated worktree-relative paths.
// A directory holding a `.git` entry is a repository of its own and is passed over with everything in it.
func leftoverFiles(worktree string, dirs []string) ([]string, error) {
	var files []string
	for _, dir := range dirs {
		root := filepath.Join(worktree, filepath.FromSlash(dir))
		info, err := os.Lstat(root)
		if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("fabricengine: read the leftover directory %s: %w", root, err)
		}
		err = filepath.WalkDir(root, func(p string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if pathPresent(filepath.Join(p, ".git")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(worktree, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("fabricengine: list the files under %s: %w", root, err)
		}
	}
	return files, nil
}

// ignoredUnderRulesAt returns those of candidates that git ignores under the ignore files the commit base holds for dirs.
// It asks git over a fresh scratch repository that holds those ignore files alone.
// The repository lies under the hub's scratch directory and is removed again.
// Each call has its own, so two calls at once never read each other's rules.
func ignoredUnderRulesAt(hubPath, worktree, base string, dirs, candidates []string) ([]string, error) {
	hubScratch := fabriclayout.HubScratchDir(hubPath)
	parent := filepath.Join(hubScratch, lymdirs.IgnoreCheckDirName)
	hubRoot, err := os.OpenRoot(hubPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open hub root %s: %w", hubPath, err)
	}
	defer hubRoot.Close()
	parentRel, err := hubRel(hubPath, parent)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: %w", err)
	}
	if err := hubRoot.MkdirAll(parentRel, 0o755); err != nil {
		return nil, fmt.Errorf("fabricengine: create the ignore-check directory: %w", err)
	}
	scratch, err := os.MkdirTemp(parent, "check-*")
	if err != nil {
		return nil, fmt.Errorf("fabricengine: create an ignore-check scratch directory: %w", err)
	}
	defer func() {
		err := removePath(NewMutations(hubPath), pathRequest{
			what:      "remove an ignore-check scratch directory",
			container: hubScratch,
			target:    scratch,
			ownership: ownedHubScratch(hubScratch),
			dirtiness: dirtinessNA("a scratch repository this call created"),
		})
		if err != nil {
			logger.Warn("fabricengine: an ignore-check scratch directory could not be removed", "path", scratch, "cause", err)
		}
	}()

	scratchRoot, err := os.OpenRoot(scratch)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open the ignore-check scratch directory: %w", err)
	}
	// Deferred after the removal above, so the root is closed before the directory is removed.
	defer scratchRoot.Close()

	const repoName = "repo"
	repo := filepath.Join(scratch, repoName)
	if err := scratchRoot.Mkdir(repoName, 0o755); err != nil {
		return nil, fmt.Errorf("fabricengine: create the ignore-check repository: %w", err)
	}
	if _, err := gitexec.Run([]string{"init", "-q", "--template="}, repo); err != nil {
		return nil, fmt.Errorf("fabricengine: initialize the ignore-check repository: %w", err)
	}
	const excludesName = "excludes"
	excludes := filepath.Join(scratch, excludesName)
	for _, empty := range []string{excludesName, path.Join(repoName, ".git", "info", "exclude")} {
		if err := scratchRoot.MkdirAll(path.Dir(empty), 0o755); err != nil {
			return nil, fmt.Errorf("fabricengine: create the directory of %s: %w", empty, err)
		}
		if err := scratchRoot.WriteFile(empty, nil, 0o644); err != nil {
			return nil, fmt.Errorf("fabricengine: write the empty exclude file %s: %w", empty, err)
		}
	}
	repoRoot, err := scratchRoot.OpenRoot(repoName)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open the ignore-check repository: %w", err)
	}
	defer repoRoot.Close()
	if err := writeIgnoreFilesAt(repoRoot, worktree, base, dirs); err != nil {
		return nil, err
	}

	stdout, err := gitexec.RunStdin([]string{"-c", "core.excludesFile=" + excludes, "check-ignore", "--no-index", "-z", "--stdin"}, repo, strings.Join(candidates, "\x00")+"\x00")
	if err != nil {
		// check-ignore exits 1 when it ignores none of the paths.
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("fabricengine: check the ignore rules of %s: %w", base, err)
	}
	return nulFields(stdout), nil
}

// writeIgnoreFilesAt writes into repo the ignore files the commit base holds for dirs.
// Those are the files on the path chain of one of dirs, or under it.
func writeIgnoreFilesAt(repo *os.Root, worktree, base string, dirs []string) error {
	listed, err := gitexec.Run([]string{"ls-tree", "-r", "-z", "--full-tree", base}, worktree)
	if err != nil {
		return fmt.Errorf("fabricengine: list the tree of %s: %w", base, err)
	}
	for _, entry := range nulFields(listed) {
		// An entry reads "<mode> <type> <object>\t<path>".
		meta, file, found := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		// A symlinked ignore file has mode 120000, and git reads no rule through it.
		if !found || len(fields) != 3 || !strings.HasPrefix(fields[0], "100") || path.Base(file) != ignoreFileName {
			continue
		}
		holder := path.Dir(file)
		onChain := holder == "." || slices.ContainsFunc(dirs, func(dir string) bool {
			return holder == dir || strings.HasPrefix(dir, holder+"/") || strings.HasPrefix(holder, dir+"/")
		})
		if !onChain {
			continue
		}
		content, err := gitexec.Run([]string{"cat-file", "blob", fields[2]}, worktree)
		if err != nil {
			return fmt.Errorf("fabricengine: read %s at %s: %w", file, base, err)
		}
		if err := repo.MkdirAll(path.Dir(file), 0o755); err != nil {
			return fmt.Errorf("fabricengine: create the directory of %s: %w", file, err)
		}
		if err := repo.WriteFile(file, []byte(content), 0o644); err != nil {
			return fmt.Errorf("fabricengine: write %s: %w", file, err)
		}
	}
	return nil
}

// nulFields splits git's NUL-terminated output into its non-empty fields.
func nulFields(output string) []string {
	return strings.FieldsFunc(output, func(r rune) bool { return r == 0 })
}
