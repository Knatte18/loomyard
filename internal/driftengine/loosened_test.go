// loosened_test.go covers the pure helpers of loosened.go: the trailer lines a ruling key depends on.

package driftengine

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoosenedTrailerLines asserts which Loosened-Check lines the messages of a range yield.
// Only the last paragraph of each message counts, and the lines come out trimmed, sorted and without duplicates.
func TestLoosenedTrailerLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const first = "Loosened-Check: TestA — the limit from 5 to 50 — measured 40"
	const second = "Loosened-Check: TestB — the timeout from 1s to 5s — measured 3s"
	tests := []struct {
		name     string
		messages []string
		want     []string
	}{
		{"TrailerInAMiddleParagraphIsIgnored", []string{"subject\n\n" + first + "\n\nSigned-off-by: someone\n"}, nil},
		{"TrailerInTheLastParagraphCounts", []string{"subject\n\n" + first + "\n"}, []string{first}},
		{"OtherLinesOfTheLastParagraphAreIgnored", []string{"subject\n\nSigned-off-by: someone\n" + first + "\n"}, []string{first}},
		{"SameTrailerInTwoMessagesYieldsItOnce", []string{"one\n\n" + first + "\n", "two\n\n" + first + "\n"}, []string{first}},
		{"UnsortedTrailersComeOutSorted", []string{"subject\n\n" + second + "\n" + first + "\n"}, []string{first, second}},
		{"CrlfMessageCutsAtItsLastParagraph", []string{"subject\r\n\r\n" + first + "\r\n\r\nSigned-off-by: someone\r\n"}, nil},
		{"CrlfTrailerComesOutTrimmed", []string{"subject\r\n\r\n" + first + "\r\n"}, []string{first}},
		{"MessageWithNoTrailerYieldsNone", []string{"subject only\n"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := loosenedTrailerLines(tt.messages); !slices.Equal(got, tt.want) {
				t.Errorf("loosenedTrailerLines(%q) = %q; want %q", tt.messages, got, tt.want)
			}
		})
	}
}
