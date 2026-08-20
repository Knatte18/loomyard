// burler.go implements BurlerProducer, the shedadapters adapter over one burlerengine round: it
// resolves the round to run from disk via the review/fixer-report pair predicate, hydrates prior
// rounds into the profile, runs one burlerengine round with a bounded retry on died/timeout, and
// maps its outcome onto the shedengine.ShedProducer contract as a routine Stuck hand-off to the
// segment's Bouncer -- never Done.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// BurlerRunner is the narrow seam BurlerProducer drives one burler round through.
type BurlerRunner interface {
	Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error)
}

// Compile-time proof that *burlerengine.Engine satisfies BurlerRunner.
var _ BurlerRunner = (*burlerengine.Engine)(nil)

// BurlerProducer is the shedadapters adapter over one burlerengine round.
//
// Call returns shedengine.Stuck on every successful round and never shedengine.Done -- that Stuck
// is a routine hand-off signal to the segment's Bouncer via OnStuck, never a real stuck condition,
// so an operator reading a status file is never misled.
//
// A round that did not reach shuttleengine.OutcomeDone after the bounded retry is a hard error
// rather than Stuck, because the Bouncer tells its seed call from its judge call by the round
// artifacts on disk, and a failed round returning Stuck with no review written would be misread as
// a seed call.
//
// Because the producer never returns Done, its Shed bounce episode never resets, so its
// effectiveMaxBounces stops being a bounce-loop guard and becomes a cap on review rounds.
//
// That cap is a two-row relationship, not this row's own MaxBounces to raise: the segment's
// Bouncer row has the same unresetting property and is the segment's entry point, so its Stuck
// sequence runs one ahead of this producer's round count, and with equal budgets it exhausts
// first -- the segment's round cap is therefore the smaller of the two rows' budgets, the
// Bouncer's normally binds, and raising the cap means raising both rows together.
type BurlerProducer struct {
	name    string
	runner  BurlerRunner
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
	runDir  string
	now     func() time.Time
}

// NewBurlerProducer returns a BurlerProducer identified as name, driving profile through runner
// under opts, with round artifacts under runDir.
// profile is a template whose ReviewPath, FixerReportPath, PriorReviews, PriorFixerReports, and
// ClusterExclude fields are overwritten per round;
// opts is a template whose Round field is overwritten per attempt.
// A nil now defaults to time.Now, and the injected clock resolves only the archive filename's
// same-second collision suffix.
// It returns a distinct error for each of: a nil runner, an empty name, an empty runDir, and a
// runDir that is not absolute per filepath.IsAbs.
// NewBurlerProducer never stats, creates, or otherwise touches runDir -- creating it is Call's job.
func NewBurlerProducer(name string, runner BurlerRunner, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time) (*BurlerProducer, error) {
	if runner == nil {
		return nil, fmt.Errorf("shedadapters: %s (%s): runner must not be nil", name, burlerEngineLabel)
	}
	if name == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): name must not be empty", name, burlerEngineLabel)
	}
	if runDir == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): runDir must not be empty", name, burlerEngineLabel)
	}
	if !filepath.IsAbs(runDir) {
		return nil, fmt.Errorf("shedadapters: %s (%s): runDir %q is not absolute", name, burlerEngineLabel, runDir)
	}
	if now == nil {
		now = time.Now
	}
	return &BurlerProducer{
		name:    name,
		runner:  runner,
		profile: profile,
		opts:    opts,
		runDir:  runDir,
		now:     now,
	}, nil
}

// roundReviewFilePrefix and roundReviewFileSuffix bound the exact filename shape
// parseRoundReviewName recognizes -- the literal text either side of a round's decimal token.
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

// roundComplete reports whether both round n's review and fixer-report files exist in runDir.
// The pair is the single completion test: a review-only orphan (a process killed in the
// phase-A-written/phase-B-pending window) reads as incomplete, never as complete.
func roundComplete(runDir string, n int) bool {
	if _, err := os.Stat(roundReviewPath(runDir, n)); err != nil {
		return false
	}
	if _, err := os.Stat(roundFixerReportPath(runDir, n)); err != nil {
		return false
	}
	return true
}

// parseRoundReviewName reports whether name matches the exact round-<n>-review.md shape -- the
// literal prefix "round-", the literal suffix "-review.md", a non-empty run of decimal digits with
// no leading zero in between, and a parsed value of at least 1 -- returning n and true on a match.
// A stamped archive sibling (round-2-review-20260820T101500Z.md), an attempt-suffixed token
// (round-3b-review.md), a zero-padded token, or any other unrelated name fails the match and is
// ignored: never adopted, never deleted.
func parseRoundReviewName(name string) (int, bool) {
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

// highestCompleteRound scans runDir's directory entries and returns the highest n for which
// roundComplete holds, returning 0 (and a nil error) when runDir is absent -- which is not an
// error -- and wrapping any other os.ReadDir failure.
// Its name-matching discipline mirrors highestRunAttempt in perch.go and is exact-shape via
// parseRoundReviewName.
// The completion predicate is the pair, never the review alone, and why: a producer process killed
// in the phase-A-written/phase-B-pending window leaves a review with no fixer report beside it and
// no exit path ever ran to clean it up, so under a review-only predicate the next call would
// advance and hydrate a fixer report that burlerengine's requireExistingPaths rejects fail-loud,
// wedging the segment permanently, whereas under the pair predicate the orphan simply means the
// round is incomplete and is re-run.
// The same pair predicate is what the segment's Bouncer uses to tell its seed call from its judge
// call, so the two sides run the same test.
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
		n, ok := parseRoundReviewName(entry.Name())
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
// using the same pair predicate as highestCompleteRound, so hydration can never name a fixer
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
