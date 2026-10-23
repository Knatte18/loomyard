// linechange.go implements LineChange, the read of how much one commit range changes the pair's code repository: its paths, its line count and the text of the lines it adds.

package fabricengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// LineChange is the tree difference between two commits of a pair's code repository.
type LineChange struct {
	// Paths are the sorted changed paths, a rename counted at both its old and its new path.
	Paths []string
	// Lines is the added plus the deleted lines.
	// A pure rename adds none, and a binary file counts as one line, so it is never free.
	Lines int
	// AddedLines is the text of every added line, without its leading plus.
	AddedLines []string
}

// LineChange reads the tree difference from from to to in the pair's code repository.
// A failed read is an error naming both commits.
func (f *Fabric) LineChange(from, to string) (LineChange, error) {
	paths, err := f.ChangedPathsBothSides(from, to)
	if err != nil {
		return LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	numstat, err := gitexec.Run([]string{"diff", "--numstat", "-M", from, to}, f.warpPath)
	if err != nil {
		return LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	lines, err := countNumstatLines(numstat)
	if err != nil {
		return LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	patch, err := gitexec.Run([]string{"diff", "--unified=0", "--no-color", "-M", from, to}, f.warpPath)
	if err != nil {
		return LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	return LineChange{Paths: paths, Lines: lines, AddedLines: addedPatchLines(patch)}, nil
}

// countNumstatLines sums the added and deleted counts of `git diff --numstat` output, counting a binary file's "-" pair as one line.
func countNumstatLines(numstat string) (int, error) {
	total := 0
	for _, line := range strings.Split(numstat, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			return 0, fmt.Errorf("numstat line %q has no counts", line)
		}
		if fields[0] == "-" && fields[1] == "-" {
			total++
			continue
		}
		for _, count := range fields[:2] {
			n, err := strconv.Atoi(count)
			if err != nil {
				return 0, fmt.Errorf("numstat line %q: %w", line, err)
			}
			total += n
		}
	}
	return total, nil
}

// addedPatchLines returns the text of every added line of a zero-context unified diff.
// A line is added only inside a hunk, so a file header's "+++" line is never read as one.
func addedPatchLines(patch string) []string {
	var added []string
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			added = append(added, line[1:])
		}
	}
	return added
}
