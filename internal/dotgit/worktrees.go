// worktrees.go lists a repository's worktrees from its common dir, as `git worktree list --porcelain` reports them.
// It reads the admin entries under `worktrees/`, the loose refs and `packed-refs`, and the repository's `config`.

package dotgit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Worktree is one worktree of a repository, as `git worktree list --porcelain` reports it.
type Worktree struct {
	// Path is the worktree's root directory.
	Path string
	// Head is the commit HEAD resolves to.
	// It is all zeros when the HEAD file is missing or names a branch with no commit yet.
	Head string
	// Branch is the full ref HEAD names, such as `refs/heads/main`, and empty when HEAD is detached or missing.
	Branch string
	// Locked reports whether a linked worktree carries a lock.
	Locked bool
	// Prunable reports whether an unlocked linked worktree's `.git` no longer exists.
	Prunable bool
}

// ErrRefStorage is returned for a repository whose `extensions.refStorage` is not `files`.
// It is wrapped with the common dir and its backend.
// Such a repository's refs are no files this package can read.
var ErrRefStorage = errors.New("dotgit: ref storage is not files")

// maxSymrefDepth bounds a chain of symbolic refs, as git bounds it.
const maxSymrefDepth = 5

// asciiSpace is the set of characters git trims from the end of a `gitdir` file.
const asciiSpace = " \t\n\v\f\r"

// ListWorktrees lists the worktrees of the repository holding dir, as `git worktree list --porcelain` does.
// dir must be absolute and may be any directory inside a worktree; the repository is found by walking up from it.
// The main worktree comes first, its path resolved through symlinks.
// The linked worktrees follow, sorted by path, compared case-folded when the repository's `core.ignorecase` is true.
// A linked worktree whose `gitdir` file is missing, unreadable or empty is left out, as git leaves it out.
// A dir that does not exist is an error with no walk-up, and so are a bare repository and a dir in no repository.
// A repository whose refs are not files is ErrRefStorage, so a caller can ask git instead.
func ListWorktrees(dir string) ([]Worktree, error) {
	if err := requireDir(dir); err != nil {
		return nil, fmt.Errorf("dotgit: list worktrees from %s: %w", dir, err)
	}
	commonDir, err := findCommonDir(dir)
	if err != nil {
		return nil, err
	}
	config, err := readRepositoryConfig(commonDir)
	if err != nil {
		return nil, err
	}
	switch {
	case config.refStorage != "" && config.refStorage != "files":
		return nil, fmt.Errorf("%w: %s uses %q", ErrRefStorage, commonDir, config.refStorage)
	case config.bare:
		return nil, fmt.Errorf("dotgit: %s: bare repositories are not supported", commonDir)
	}

	refs := &refReader{commonDir: commonDir, zero: config.zeroSHA()}
	main, err := readMainWorktree(commonDir, refs)
	if err != nil {
		return nil, err
	}
	linked, err := readLinkedWorktrees(commonDir, refs)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(linked, func(a, b Worktree) int { return comparePaths(a.Path, b.Path, config.ignoreCase) })
	return append([]Worktree{main}, linked...), nil
}

// findCommonDir returns the common dir of the repository holding dir.
// dir is that common dir itself when it is a repository's git dir, as a bare repository's directory is.
func findCommonDir(dir string) (string, error) {
	if IsRepository(Geometry{GitDir: dir, CommonDir: dir}) {
		return dir, nil
	}
	_, g, err := FindRoot(dir)
	if err != nil {
		return "", err
	}
	return g.CommonDir, nil
}

// readMainWorktree reads the main worktree: the common dir resolved through symlinks, less its `.git`.
func readMainWorktree(commonDir string, refs *refReader) (Worktree, error) {
	resolved, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		return Worktree{}, fmt.Errorf("dotgit: resolve %s: %w", commonDir, err)
	}
	head, branch, err := refs.readHead(commonDir)
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{Path: stripDotGit(resolved), Head: head, Branch: branch}, nil
}

// readLinkedWorktrees reads each linked worktree from its admin entry under the common dir's `worktrees`, unsorted.
func readLinkedWorktrees(commonDir string, refs *refReader) ([]Worktree, error) {
	adminRoot := filepath.Join(commonDir, "worktrees")
	entries, err := os.ReadDir(adminRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dotgit: read %s: %w", adminRoot, err)
	}

	var worktrees []Worktree
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		adminDir := filepath.Join(adminRoot, entry.Name())
		dotGit, ok := readAdminGitdir(adminDir)
		if !ok {
			continue
		}
		head, branch, err := refs.readHead(adminDir)
		if err != nil {
			return nil, err
		}
		locked := exists(filepath.Join(adminDir, "locked"))
		worktrees = append(worktrees, Worktree{
			Path:     stripDotGit(dotGit),
			Head:     head,
			Branch:   branch,
			Locked:   locked,
			Prunable: !locked && !exists(dotGit),
		})
	}
	return worktrees, nil
}

// readAdminGitdir returns the `.git` path an admin entry's `gitdir` file names, and false when git would skip the entry.
// An absolute path is taken as written.
// A relative one, which git 2.48 and later write, resolves against the admin entry as git resolves it:
// symlinks are resolved as far as the path exists.
func readAdminGitdir(adminDir string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
	if err != nil {
		return "", false
	}
	dotGit := strings.TrimRight(string(content), asciiSpace)
	if dotGit == "" {
		return "", false
	}
	if filepath.IsAbs(dotGit) {
		return dotGit, true
	}
	base, err := filepath.EvalSymlinks(adminDir)
	if err != nil {
		base = adminDir
	}
	return resolveExistingPrefix(filepath.Join(base, dotGit)), true
}

// resolveExistingPrefix returns path with symlinks resolved in its longest existing prefix and the rest appended.
func resolveExistingPrefix(path string) string {
	var tail []string
	current := filepath.Clean(path)
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path)
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}
}

// stripDotGit removes a trailing `.git` component from path, as git does to name a worktree from its `.git`.
func stripDotGit(path string) string {
	if trimmed, ok := strings.CutSuffix(path, "/.git"); ok {
		return trimmed
	}
	return strings.TrimSuffix(path, string(filepath.Separator)+".git")
}

// comparePaths orders two worktree paths as git's path compare does: bytewise, or ASCII case-folded under ignoreCase.
func comparePaths(a, b string, ignoreCase bool) int {
	if !ignoreCase {
		return strings.Compare(a, b)
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if diff := int(lowerASCII(a[i])) - int(lowerASCII(b[i])); diff != 0 {
			return diff
		}
	}
	return len(a) - len(b)
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// refReader resolves refs from a common dir's loose ref files and its `packed-refs`, reading `packed-refs` once.
type refReader struct {
	commonDir string
	// zero is the all-zero commit of the repository's object format.
	zero   string
	packed map[string]string
}

// readHead returns the commit and the branch ref of the HEAD file in gitDir.
// A missing HEAD file is detached at the all-zero commit, as git lists a worktree still being added.
func (r *refReader) readHead(gitDir string) (head, branch string, err error) {
	path := filepath.Join(gitDir, "HEAD")
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return r.zero, "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("dotgit: read %s: %w", path, err)
	}
	value := strings.TrimSpace(string(content))
	target, symbolic := strings.CutPrefix(value, "ref:")
	if !symbolic {
		return value, "", nil
	}
	return r.resolve(strings.TrimSpace(target))
}

// resolve follows ref through symbolic refs and returns its commit and the last ref of the chain.
// A ref that does not exist resolves to the all-zero commit.
func (r *refReader) resolve(ref string) (commit, finalRef string, err error) {
	for range maxSymrefDepth {
		value, found, err := r.readLoose(ref)
		if err != nil {
			return "", "", err
		}
		if !found {
			packed, err := r.readPacked(ref)
			if err != nil {
				return "", "", err
			}
			if packed == "" {
				return r.zero, ref, nil
			}
			return packed, ref, nil
		}
		target, symbolic := strings.CutPrefix(value, "ref:")
		if !symbolic {
			return value, ref, nil
		}
		ref = strings.TrimSpace(target)
	}
	return "", "", fmt.Errorf("dotgit: %s: symbolic refs nest deeper than %d", ref, maxSymrefDepth)
}

// readLoose returns the content of ref's loose file in the common dir, and false when there is no such file.
func (r *refReader) readLoose(ref string) (value string, found bool, err error) {
	path := filepath.Join(r.commonDir, filepath.FromSlash(ref))
	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		return strings.TrimSpace(string(content)), true, nil
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.EISDIR):
		return "", false, nil
	}
	return "", false, fmt.Errorf("dotgit: read ref %s: %w", path, err)
}

// readPacked returns ref's commit in the common dir's `packed-refs`, or "" when it holds none.
func (r *refReader) readPacked(ref string) (string, error) {
	if r.packed == nil {
		packed, err := parsePackedRefs(filepath.Join(r.commonDir, "packed-refs"))
		if err != nil {
			return "", err
		}
		r.packed = packed
	}
	return r.packed[ref], nil
}

// parsePackedRefs maps each ref in the `packed-refs` file at path to its commit; a missing file maps none.
// The header and the peeled `^` lines carry no ref and are skipped.
func parsePackedRefs(path string) (map[string]string, error) {
	packed := map[string]string{}
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return packed, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dotgit: read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || line[0] == '#' || line[0] == '^' {
			continue
		}
		if commit, ref, ok := strings.Cut(line, " "); ok {
			packed[ref] = commit
		}
	}
	return packed, nil
}

// repositoryConfig holds the settings of a repository's `config` file that decide how its worktrees are read.
type repositoryConfig struct {
	bare         bool
	ignoreCase   bool
	refStorage   string
	objectFormat string
}

// zeroSHA returns the all-zero commit of the repository's object format.
func (c repositoryConfig) zeroSHA() string {
	if c.objectFormat == "sha256" {
		return strings.Repeat("0", 64)
	}
	return strings.Repeat("0", 40)
}

// readRepositoryConfig reads the settings repositoryConfig holds from commonDir's `config` file.
// A missing file leaves every setting unset.
// It reads `key = value` lines under `[core]` and `[extensions]`, the last assignment winning, and follows no include.
func readRepositoryConfig(commonDir string) (repositoryConfig, error) {
	var config repositoryConfig
	path := filepath.Join(commonDir, "config")
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("dotgit: read %s: %w", path, err)
	}

	section := ""
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(stripConfigComment(line))
		if rest, ok := strings.CutPrefix(line, "["); ok {
			header, after, closed := strings.Cut(rest, "]")
			if !closed {
				section = ""
				continue
			}
			// A subsection header such as `remote "origin"` keeps its quoted part, so it names no section read here.
			section = strings.ToLower(strings.TrimSpace(header))
			line = strings.TrimSpace(after)
		}
		if line == "" {
			continue
		}
		key, value, hasValue := strings.Cut(line, "=")
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
		switch section + "." + strings.ToLower(strings.TrimSpace(key)) {
		case "core.bare":
			config.bare = configBool(value, hasValue)
		case "core.ignorecase":
			config.ignoreCase = configBool(value, hasValue)
		case "extensions.refstorage":
			config.refStorage = value
		case "extensions.objectformat":
			config.objectFormat = value
		}
	}
	return config, nil
}

// stripConfigComment cuts line at the first `#` or `;` outside double quotes.
func stripConfigComment(line string) string {
	quoted := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			quoted = !quoted
		case '#', ';':
			if !quoted {
				return line[:i]
			}
		}
	}
	return line
}

// configBool reads a lower-cased git boolean; a key with no `=` is true, as git reads it.
func configBool(value string, hasValue bool) bool {
	if !hasValue {
		return true
	}
	switch value {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}
