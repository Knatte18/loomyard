// changedlines.go finds the lines of a file that a diff adds or changes, from the file's text on both sides.

package testreach

import "strings"

// maxLineDiffCells caps the cells of the table the line diff fills, one per pair of base and head lines it compares.
const maxLineDiffCells = 4_000_000

// ChangedLines returns the sorted 1-based line numbers of head that a line diff of base and head marks added or changed.
// The diff keeps a longest common subsequence of lines, so a line outside it is marked.
// A line only base holds marks none.
// An empty base marks every line of head.
// Lines the two sides share at their start and end are never marked.
// A rewritten stretch between them can be too large for the diff's table.
// That is so when its base and head lines pair up in more than maxLineDiffCells ways.
// Every head line of such a stretch is marked.
func ChangedLines(base, head []byte) []int {
	baseLines, headLines := splitLines(base), splitLines(head)
	prefix := 0
	for prefix < len(baseLines) && prefix < len(headLines) && baseLines[prefix] == headLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(baseLines)-prefix && suffix < len(headLines)-prefix &&
		baseLines[len(baseLines)-1-suffix] == headLines[len(headLines)-1-suffix] {
		suffix++
	}
	baseMiddle := baseLines[prefix : len(baseLines)-suffix]
	headMiddle := headLines[prefix : len(headLines)-suffix]

	if len(baseMiddle)*len(headMiddle) > maxLineDiffCells {
		changed := make([]int, len(headMiddle))
		for j := range changed {
			changed[j] = prefix + j + 1
		}
		return changed
	}

	// common[i][j] is the length of the longest common subsequence of baseMiddle[i:] and headMiddle[j:].
	common := make([][]int, len(baseMiddle)+1)
	for i := range common {
		common[i] = make([]int, len(headMiddle)+1)
	}
	for i := len(baseMiddle) - 1; i >= 0; i-- {
		for j := len(headMiddle) - 1; j >= 0; j-- {
			if baseMiddle[i] == headMiddle[j] {
				common[i][j] = common[i+1][j+1] + 1
			} else {
				common[i][j] = max(common[i+1][j], common[i][j+1])
			}
		}
	}

	var changed []int
	i, j := 0, 0
	for j < len(headMiddle) {
		switch {
		case i < len(baseMiddle) && baseMiddle[i] == headMiddle[j]:
			i++
			j++
		case i < len(baseMiddle) && common[i+1][j] >= common[i][j+1]:
			i++
		default:
			changed = append(changed, prefix+j+1)
			j++
		}
	}
	return changed
}

// splitLines returns the lines of data without their line feeds; a final line feed ends the last line and opens none.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
