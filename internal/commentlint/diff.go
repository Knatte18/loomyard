// diff.go lists the `.go` files that differ between the base and the worktree, and reads each side of a changed file through git.

package commentlint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// fileChange is one changed `.go` file.
type fileChange struct {
	// path is the slash-separated path of the new side, relative to the worktree.
	path string
	// basePath is the path of the base side, empty when the file has no base side.
	basePath string
}

// changedGoFiles lists the `.go` files of worktree that differ from the base, sorted by path.
// An empty base diffs the working tree against HEAD and adds the untracked files; otherwise it diffs base against HEAD.
// A deleted file is not listed, and a rename keeps its old path as the base side.
func changedGoFiles(worktree, base string) ([]fileChange, error) {
	diffArgs := []string{"diff", "--name-status", "-z", "--find-renames", "HEAD"}
	if base != "" {
		diffArgs = []string{"diff", "--name-status", "-z", "--find-renames", base, "HEAD"}
	}
	diffOutput, err := gitexec.Run(diffArgs, worktree)
	if err != nil {
		return nil, fmt.Errorf("commentlint: list the changed files: %w", err)
	}
	changes := parseNameStatus(diffOutput)

	if base == "" {
		untrackedOutput, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "-z"}, worktree)
		if err != nil {
			return nil, fmt.Errorf("commentlint: list the untracked files: %w", err)
		}
		for _, path := range strings.Split(untrackedOutput, "\x00") {
			if path != "" {
				changes = append(changes, fileChange{path: path})
			}
		}
	}

	var goChanges []fileChange
	for _, change := range changes {
		if strings.HasSuffix(change.path, ".go") {
			goChanges = append(goChanges, change)
		}
	}
	sort.Slice(goChanges, func(i, j int) bool { return goChanges[i].path < goChanges[j].path })
	return goChanges, nil
}

// parseNameStatus reads `git diff --name-status -z` output: a status, then one path, or two for a rename or copy.
func parseNameStatus(output string) []fileChange {
	fields := strings.Split(output, "\x00")
	var changes []fileChange
	for i := 0; i < len(fields) && fields[i] != ""; {
		status := fields[i][0]
		switch status {
		case 'R', 'C':
			if i+2 >= len(fields) {
				return changes
			}
			changes = append(changes, fileChange{path: fields[i+2], basePath: fields[i+1]})
			i += 3
		case 'D':
			i += 2
		case 'A':
			if i+1 >= len(fields) {
				return changes
			}
			changes = append(changes, fileChange{path: fields[i+1]})
			i += 2
		default:
			if i+1 >= len(fields) {
				return changes
			}
			changes = append(changes, fileChange{path: fields[i+1], basePath: fields[i+1]})
			i += 2
		}
	}
	return changes
}

// readChange returns the base text and the new text of one changed file.
// The base side comes from git at the base commit, or at HEAD when the base is empty; the new side comes from the working tree when the base is empty, and from HEAD otherwise.
// A file with no base side has an empty base text.
func readChange(worktree, base string, change fileChange) (baseText, newText string, err error) {
	baseRevision := base
	if base == "" {
		baseRevision = "HEAD"
	}
	if change.basePath != "" {
		baseText, err = gitexec.Run([]string{"show", baseRevision + ":" + change.basePath}, worktree)
		if err != nil {
			return "", "", fmt.Errorf("commentlint: read %s at %s: %w", change.basePath, baseRevision, err)
		}
	}
	if base == "" {
		data, err := os.ReadFile(filepath.Join(worktree, filepath.FromSlash(change.path)))
		if err != nil {
			return "", "", fmt.Errorf("commentlint: read %s: %w", change.path, err)
		}
		return baseText, string(data), nil
	}
	newText, err = gitexec.Run([]string{"show", "HEAD:" + change.path}, worktree)
	if err != nil {
		return "", "", fmt.Errorf("commentlint: read %s at HEAD: %w", change.path, err)
	}
	return baseText, newText, nil
}
