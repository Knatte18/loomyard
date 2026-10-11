// burler.go holds what the Burler review row and fix row share about a segment's run directory: the round file names, the complete-round predicate, the highest-complete-round scan, the prior-round hydration paths and the one-line description of a half.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// roundReviewFilePrefix and roundReviewFileSuffix bound the exact filename shape ParseRoundReviewName recognizes -- the literal text either side of a round's decimal token.
const (
	roundReviewFilePrefix = "round-"
	roundReviewFileSuffix = "-review.md"
)

// roundReviewPath returns round n's review file path under runDir, joined as "round-<n>-review.md"
// with n rendered as a plain positive decimal integer carrying no leading zeros and no attempt
// suffix -- the attempt-distinguishing token lives on RunOpts.Round, never on this path.
func roundReviewPath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-review.md", n))
}

// roundFixerReportPath returns round n's fixer-report file path under runDir, joined as
// "round-<n>-fixer-report.md" using the same rendering rule as roundReviewPath.
func roundFixerReportPath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-fixer-report.md", n))
}

// roundComplete reports whether round n's review, fixer-report and acceptance record all exist in runDir.
// The triple is the single completion test: a review-only orphan (a process killed between the review row and the fix row) and a review and fixer-report without an accepted-review record both read as incomplete, never as complete.
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

// ParseRoundReviewName reports whether name matches the exact round-<n>-review.md shape -- the
// literal prefix "round-", the literal suffix "-review.md", a non-empty run of decimal digits with
// no leading zero in between, and a parsed value of at least 1 -- returning n and true on a match.
// A stamped archive sibling (round-2-review-20260820T101500Z.md), an attempt-suffixed token
// (round-3b-review.md), a zero-padded token, or any other unrelated name fails the match and is
// ignored: never adopted, never deleted.
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

// highestCompleteRound scans runDir's directory entries and returns the highest n for which roundComplete holds.
// It returns 0 (and a nil error) when runDir is absent, which is not an error, and wraps any other os.ReadDir failure.
// Its name-matching discipline is exact-shape via ParseRoundReviewName.
// The completion predicate is the triple, never the review alone, and why:
// a producer process killed between the review row and the fix row leaves a review with no fixer report beside it and no exit path ever ran to clean it up.
// Under a review-only predicate the next call would advance and hydrate a fixer report that burlerengine's requireExistingPaths rejects fail-loud, wedging the segment permanently.
// Under the triple predicate the orphan simply means the round is incomplete and is re-run.
// The segment's Bouncer does NOT run the same test, and saying so matters: it resolves its round through ResolveRound, which stats the review file alone.
// The asymmetry is safe today only because of where an orphan can appear:
// a process killed in that window leaves current_producer naming the review or fix row, so that row re-resolves the same round and archives the orphan before the Bouncer is ever routed to.
// It is not safe by construction, and a future change that lets the Bouncer be entered with an orphaned review present would have it judge a review no fixer round stands behind.
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

// hydrationPaths returns, for every round n from 1 up to but excluding current for which
// roundComplete holds, the review path and the fixer-report path in ascending round order --
// using the same predicate as highestCompleteRound, so hydration can never name a fixer
// report that does not exist.
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

// describeHalf names one half by its session and run directory, by its start error when it never came up, or as not started when it carries nothing.
func describeHalf(label string, half burlerengine.Half) string {
	switch {
	case half.StartError != "":
		return fmt.Sprintf("%s never started (%s)", label, half.StartError)
	case half == (burlerengine.Half{}):
		return label + " not started"
	}
	return fmt.Sprintf("%s session %s run dir %s", label, half.SessionID, half.RunDir)
}
