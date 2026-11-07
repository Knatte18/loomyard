// burler.go holds what the Burler review row and the fix row share about a segment's run directory.
// It holds the round file paths, the complete-round predicate, and the highest-complete-round scan.
// It also holds the prior-round hydration paths and the one-line description of a half.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// roundReviewFilePrefix and roundReviewFileSuffix are the literal text on either side of a round's decimal token.
const (
	roundReviewFilePrefix = "round-"
	roundReviewFileSuffix = "-review.md"
)

// roundReviewPath returns round n's review file path under runDir, joined as "round-<n>-review.md".
// n is rendered as a plain positive decimal with no leading zeros and no attempt suffix.
// The attempt-distinguishing token is carried by the run options, never by this path.
func roundReviewPath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-review.md", n))
}

// roundFixerReportPath returns round n's fixer-report file path under runDir, joined as "round-<n>-fixer-report.md".
// n is rendered as a plain decimal with no leading zeros.
func roundFixerReportPath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-fixer-report.md", n))
}

// roundComplete reports whether round n's review, fixer-report and acceptance record all exist in runDir.
// The triple is the single completion test: a review missing its fixer report or acceptance record reads as incomplete.
func roundComplete(runDir string, n int) bool {
	if _, err := os.Stat(roundReviewPath(runDir, n)); err != nil {
		return false
	}
	if _, err := os.Stat(roundFixerReportPath(runDir, n)); err != nil {
		return false
	}
	_, accepted, err := readAcceptance(runDir, n)
	return err == nil && accepted
}

// ParseRoundReviewName reports whether name has the exact round-<n>-review.md shape, and returns n on a match.
// The digits between the affixes must be a non-empty run with no leading zero and a value of at least 1.
// A stamped archive sibling (round-2-review-20260820T101500Z.md) fails the match.
// So does an attempt-suffixed token (round-3b-review.md), a zero-padded token, or any other unrelated name.
// Such names are ignored: never adopted, never deleted.
func ParseRoundReviewName(name string) (int, bool) {
	if !strings.HasPrefix(name, roundReviewFilePrefix) || !strings.HasSuffix(name, roundReviewFileSuffix) {
		return 0, false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, roundReviewFilePrefix), roundReviewFileSuffix)
	if mid == "" || (len(mid) > 1 && mid[0] == '0') {
		return 0, false
	}
	for _, r := range mid {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(mid)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// highestCompleteRound scans runDir's directory entries and returns the highest n whose round is complete.
// It returns 0 (and a nil error) when runDir is absent, which is not an error, and wraps any other os.ReadDir failure.
// Only names of the exact round-<n>-review.md shape are considered.
// The triple is the completion test, never the review alone.
// A process killed between the review row and the fix row leaves a review with no fixer report beside it,
// and no exit path runs to clean it up.
// Under a review-only test the next call would hydrate a fixer report that burlerengine's requireExistingPaths rejects.
// The segment would then be wedged permanently.
// Under the triple, the orphan means the round is incomplete and is re-run.
// The segment's Bouncer does not run this test: it resolves its round from the review file alone.
// That is safe only because a killed process leaves current_producer naming the review or fix row,
// so that row re-resolves the same round and archives the orphan before the Bouncer runs.
// The arrangement is not safe by construction.
// A change that lets the Bouncer run with an orphaned review present would judge a review no fixer round stands behind.
func highestCompleteRound(runDir string) (int, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("shedadapters: read run dir %q: %w", runDir, err)
	}

	highest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		n, ok := ParseRoundReviewName(entry.Name())
		if !ok {
			continue
		}
		if n > highest && roundComplete(runDir, n) {
			highest = n
		}
	}
	return highest, nil
}

// hydrationPaths returns the review path and the fixer-report path of every complete round below current.
// The paths come in ascending round order.
// Only complete rounds count, so hydration never names a fixer report that does not exist.
func hydrationPaths(runDir string, current int) (reviews []string, fixerReports []string, err error) {
	for n := 1; n < current; n++ {
		if !roundComplete(runDir, n) {
			continue
		}
		reviews = append(reviews, roundReviewPath(runDir, n))
		fixerReports = append(fixerReports, roundFixerReportPath(runDir, n))
	}
	return reviews, fixerReports, nil
}

// describeHalf names one half by its session and run directory.
// It names the half by its start error when it never came up, and as not started when it carries nothing.
func describeHalf(label string, half burlerengine.Half) string {
	switch {
	case half.StartError != "":
		return fmt.Sprintf("%s never started (%s)", label, half.StartError)
	case half == (burlerengine.Half{}):
		return label + " not started"
	}
	return fmt.Sprintf("%s session %s run dir %s", label, half.SessionID, half.RunDir)
}
