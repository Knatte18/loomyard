// changedlines_test.go tests the line diff that marks the lines of a file a diff adds or changes.

package testreach_test

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

func TestChangedLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// A rewritten stretch one line longer than the table cap's square root holds one line both sides share.
	rewritten := int(math.Sqrt(testreach.MaxLineDiffCells)) + 1
	var bigBase, bigHead strings.Builder
	var everyRewrittenLine []int
	for i := range rewritten {
		if i == rewritten/2 {
			bigBase.WriteString("shared\n")
			bigHead.WriteString("shared\n")
		} else {
			fmt.Fprintf(&bigBase, "base%d\n", i)
			fmt.Fprintf(&bigHead, "head%d\n", i)
		}
		everyRewrittenLine = append(everyRewrittenLine, i+1)
	}

	tests := []struct {
		name string
		base string
		head string
		want []int
	}{
		{name: "AStretchOverTheTableCapMarksEveryLineOfIt", base: bigBase.String(), head: bigHead.String(), want: everyRewrittenLine},
		{name: "AnInsertedLineIsMarked", base: "a\nb\n", head: "a\nx\nb\n", want: []int{2}},
		{name: "AChangedLineIsMarked", base: "a\nb\nc\n", head: "a\nB\nc\n", want: []int{2}},
		{name: "ADeletionMarksNoLine", base: "a\nb\nc\n", head: "a\nc\n", want: nil},
		{name: "AnEmptyBaseMarksEveryLine", base: "", head: "a\nb\n", want: []int{1, 2}},
		{name: "SeveralHunksAreMarkedByHeadLine", base: "a\nb\nc\nd\n", head: "a\nB\nc\nD\nE\n", want: []int{2, 4, 5}},
		{name: "ATextWithNoFinalLineFeedHasNoEmptyLastLine", base: "a\nb", head: "a\nb\nc", want: []int{3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := testreach.ChangedLines([]byte(tt.base), []byte(tt.head)); !slices.Equal(got, tt.want) {
				t.Errorf("ChangedLines(%s) = %v; want %v", tt.name, got, tt.want)
			}
		})
	}
}
