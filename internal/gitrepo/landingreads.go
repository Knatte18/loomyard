// landingreads.go adds the two reads that attribute a landed fix to the code it repaired: PreImageLines lists the lines of a commit's first parent that the commit removes or replaces, and BlameLines names the first-parent commit that brought each of a file's lines.

package gitrepo

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// PreImageLines diffs sha's tree against its first parent through go-git and returns, per slash-separated path the commit modifies or deletes, the ascending 1-based line numbers of the first parent's version that the commit removes or replaces.
// A path the commit only adds has no entry, a binary file has no entry, and a root commit returns an empty map.
// An invalid sha is ErrInvalidSHA.
func (r *Repo) PreImageLines(sha string) (map[string][]int, error) {
	if !validSHA(sha) {
		return nil, ErrInvalidSHA
	}

	return readGoGit(r, func(repo *git.Repository) (map[string][]int, error) {
		lines := map[string][]int{}
		commit, err := commitByHash(repo, sha)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: resolve commit %s: %w", sha, err)
		}
		if commit.NumParents() == 0 {
			return lines, nil
		}

		tree, err := commit.Tree()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read tree of %s: %w", sha, err)
		}
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read first parent of %s: %w", sha, err)
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read tree of first parent of %s: %w", sha, err)
		}
		changes, err := parentTree.Diff(tree)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: diff %s against its first parent: %w", sha, err)
		}

		for _, change := range changes {
			action, err := change.Action()
			if err != nil {
				return nil, fmt.Errorf("gitrepo: classify a change of %s: %w", sha, err)
			}
			if action == merkletrie.Insert {
				continue
			}
			patch, err := change.Patch()
			if err != nil {
				return nil, fmt.Errorf("gitrepo: patch %s in %s: %w", change.From.Name, sha, err)
			}
			for _, filePatch := range patch.FilePatches() {
				if filePatch.IsBinary() {
					continue
				}
				if removed := removedLines(filePatch.Chunks()); len(removed) > 0 {
					lines[change.From.Name] = removed
				}
			}
		}
		return lines, nil
	})
}

// removedLines returns the ascending 1-based line numbers of the old version that chunks delete.
func removedLines(chunks []diff.Chunk) []int {
	var removed []int
	line := 1
	for _, chunk := range chunks {
		count := lineCount(chunk.Content())
		switch chunk.Type() {
		case diff.Equal:
			line += count
		case diff.Delete:
			for i := 0; i < count; i++ {
				removed = append(removed, line+i)
			}
			line += count
		}
	}
	return removed
}

// lineCount counts the lines in content, a last line without a newline included.
func lineCount(content string) int {
	count := strings.Count(content, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

// blameHeaderPattern matches the header line git blame --porcelain prints before each blamed line: the commit, the line in that commit and the line in the blamed file.
var blameHeaderPattern = regexp.MustCompile(`^([0-9a-f]{40}) \d+ (\d+)(?: \d+)?$`)

// BlameLines runs git blame --porcelain --first-parent over relPath at rev and returns the attributing commit SHA of each requested 1-based line, with one -L range per run of consecutive lines.
// It is CLI-bound: go-git's blame walks the whole history of the file per call and has no first-parent mode, which attributing a merged branch's lines to the merge that landed them needs.
// rev must be a valid hex object name and relPath must not start with '-', else ErrInvalidSHA; no lines returns an empty map without running git.
func (r *Repo) BlameLines(rev, relPath string, lines []int) (map[int]string, error) {
	if !validSHA(rev) || strings.HasPrefix(relPath, "-") {
		return nil, ErrInvalidSHA
	}
	attributed := map[int]string{}
	if len(lines) == 0 {
		return attributed, nil
	}

	args := []string{"blame", "--porcelain", "--first-parent"}
	for _, span := range consecutiveRuns(lines) {
		args = append(args, "-L", fmt.Sprintf("%d,%d", span[0], span[1]))
	}
	args = append(args, rev, "--", relPath)
	stdout, err := r.runChecked(args...)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: blame %s at %s in %s: %w", relPath, rev, r.path, err)
	}

	for _, text := range strings.Split(stdout, "\n") {
		match := blameHeaderPattern.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		line, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, fmt.Errorf("gitrepo: blame header %q: %w", text, err)
		}
		attributed[line] = match[1]
	}
	return attributed, nil
}

// consecutiveRuns returns the first and last line of each run of consecutive numbers in lines, ascending.
func consecutiveRuns(lines []int) [][2]int {
	sorted := slices.Clone(lines)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)

	var runs [][2]int
	for _, line := range sorted {
		if n := len(runs); n > 0 && runs[n-1][1] == line-1 {
			runs[n-1][1] = line
			continue
		}
		runs = append(runs, [2]int{line, line})
	}
	return runs
}
