// diff.go lists the `.go` files that differ between the base and the new side, and reads both sides of each.

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

// changedGoFiles lists the `.go` files of worktree that differ from baseRevision, sorted by path.
// With workingTree it diffs the working tree against baseRevision and adds the untracked files;
// otherwise it diffs HEAD against baseRevision.
// A deleted file is not listed, and a rename keeps its old path as the base side.
// An untracked file that baseRevision holds keeps its own path as the base side.
// Only the lines it changed count as new.
func changedGoFiles(worktree, baseRevision string, workingTree bool) ([]fileChange, error) {
	diffArgs := []string{"diff", "--name-status", "-z", "--find-renames", baseRevision}
	if !workingTree {
		diffArgs = append(diffArgs, "HEAD")
	}
	diffOutput, err := gitexec.Run(diffArgs, worktree)
	if err != nil {
		return nil, fmt.Errorf("commentlint: list the changed files: %w", err)
	}
	changes := parseNameStatus(diffOutput)

	if workingTree {
		untrackedOutput, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "-z"}, worktree)
		if err != nil {
			return nil, fmt.Errorf("commentlint: list the untracked files: %w", err)
		}
		for _, path := range strings.Split(untrackedOutput, "\x00") {
			if !strings.HasSuffix(path, ".go") {
				continue
			}
			//gitexec:raw cat-file -e signals a missing path through its exit code, which is the answer sought here
			_, _, exitCode, err := gitexec.RunGit([]string{"cat-file", "-e", baseRevision + ":" + path}, worktree)
			if err != nil {
				return nil, fmt.Errorf("commentlint: look for %s at %s: %w", path, baseRevision, err)
			}
			change := fileChange{path: path}
			if exitCode == 0 {
				change.basePath = path
			}
			changes = append(changes, change)
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
// The base side comes from git at baseRevision;
// the new side comes from the working tree with workingTree, and from HEAD otherwise.
// A file with no base side has an empty base text.
func readChange(worktree, baseRevision string, workingTree bool, change fileChange) (baseText, newText string, err error) {
	if change.basePath != "" {
		baseText, err = gitexec.Run([]string{"show", baseRevision + ":" + change.basePath}, worktree)
		if err != nil {
			return "", "", fmt.Errorf("commentlint: read %s at %s: %w", change.basePath, baseRevision, err)
		}
	}
	if workingTree {
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
