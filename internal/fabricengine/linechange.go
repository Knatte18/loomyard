// linechange.go reads how a commit range changes a pair's code repository.
// It holds LineChange, which lists the changed paths, the line count and the added lines;
// DiffSize totals the added and deleted lines and the changed files.

package fabricengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
)

// LineChange reads the tree difference from from to to in the pair's code repository.
// A failed read is an error naming both commits.
func (f *Fabric) LineChange(from, to string) (fabriclayout.LineChange, error) {
	paths, err := f.ChangedPathsBothSides(from, to)
	if err != nil {
		return fabriclayout.LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	numstat, err := gitexec.Run([]string{"diff", "--numstat", "-M", from, to}, f.warpPath)
	if err != nil {
		return fabriclayout.LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	lines, err := countNumstatLines(numstat)
	if err != nil {
		return fabriclayout.LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	patch, err := gitexec.Run([]string{"diff", "--unified=0", "--no-color", "-M", from, to}, f.warpPath)
	if err != nil {
		return fabriclayout.LineChange{}, fmt.Errorf("fabricengine: line change from %s to %s: %w", from, to, err)
	}
	return fabriclayout.LineChange{Paths: paths, Lines: lines, AddedLines: addedPatchLines(patch)}, nil
}

// DiffSize reads the size of the tree difference from from to to in the pair's code repository.
// A failed read is an error naming both commits.
func (f *Fabric) DiffSize(from, to string) (fabriclayout.DiffSize, error) {
	numstat, err := gitexec.Run([]string{"diff", "--numstat", "-M", from, to}, f.warpPath)
	if err != nil {
		return fabriclayout.DiffSize{}, fmt.Errorf("fabricengine: diff size from %s to %s: %w", from, to, err)
	}
	size, err := diffSizeOfNumstat(numstat)
	if err != nil {
		return fabriclayout.DiffSize{}, fmt.Errorf("fabricengine: diff size from %s to %s: %w", from, to, err)
	}
	return size, nil
}

// diffSizeOfNumstat totals `git diff --numstat` output, which has one file per line.
// A binary file's "-" pair adds no lines.
func diffSizeOfNumstat(numstat string) (fabriclayout.DiffSize, error) {
	var size fabriclayout.DiffSize
	for _, line := range strings.Split(numstat, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			return fabriclayout.DiffSize{}, fmt.Errorf("numstat line %q has no counts", line)
		}
		size.FilesChanged++
		if fields[0] == "-" && fields[1] == "-" {
			continue
		}
		added, err := strconv.Atoi(fields[0])
		if err != nil {
			return fabriclayout.DiffSize{}, fmt.Errorf("numstat line %q: %w", line, err)
		}
		deleted, err := strconv.Atoi(fields[1])
		if err != nil {
			return fabriclayout.DiffSize{}, fmt.Errorf("numstat line %q: %w", line, err)
		}
		size.LinesAdded += added
		size.LinesDeleted += deleted
	}
	return size, nil
}

// countNumstatLines sums the added and deleted counts of `git diff --numstat` output;
// a binary file's "-" pair counts as one line.
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
