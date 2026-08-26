// burler.go implements BurlerProducer, the shedadapters adapter over one burlerengine round: it
// resolves the round to run from disk via the review/fixer-report pair predicate, hydrates prior
// rounds into the profile, runs one burlerengine round with a bounded retry on died/timeout, and
// maps its outcome onto the shedengine.ShedProducer contract as a routine Stuck hand-off to the
// segment's Bouncer -- never Done.

package shedadapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
// That cap is a two-row relationship, not this row's own MaxBounces to raise, and the two rows are
// no longer symmetric once a segment can be re-entered after settling: the segment's Bouncer row
// returns Done on approval, so its own episode resets at that Done, while this row's never does.
// The Bouncer's budget binds in a segment's first generation -- its Stuck sequence runs one ahead
// of this producer's round count, and with equal budgets it exhausts first. In any later
// generation, though, this row's episode has kept counting since the segment's very first round,
// so the Burler's leftover budget binds instead, not the Bouncer's fresh one.
//
// That is a real, accepted limitation, not a hypothetical: with equal max_bounces budgets, a first
// generation that approves on round k leaves this row max_bounces-k units for every later
// generation, and a first generation that approves on the last round leaves none -- the segment's
// very first Burler hand-off in a second generation then halts the run on a bounce-budget-exhausted
// escalation to a human. It is accepted because it fails safe (a halt and a human escalation, never
// an unjudged artifact passing the gate), and because compensating for it means changing
// shedengine's shared episode/budget model, a design change well past a single row's own doc
// comment. Raising the cap for either generation means raising both rows' budgets together.
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
// Its name-matching discipline is exact-shape via
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

// Compile-time proof that *BurlerProducer satisfies shedengine.ShedProducer.
var _ shedengine.ShedProducer = (*BurlerProducer)(nil)

// Call runs one BurlerProducer round: resolve the round to run from disk, build a fresh per-round
// copy of the stored template profile, run at most two attempts (a second only on a died/timeout
// first attempt), and map the outcome onto shedengine's contract.
//
// Archive rule: every return in which the round did not produce a usable review archives both
// round paths first, keyed on that fact rather than on whether the return is an error -- this
// covers a runner error (regardless of the Result.Outcome it carries), an asking hard error, a
// second consecutive died/timeout, an unrecognized outcome, and a cancellation detected between
// attempts. Two carve-outs leave the round's files in place: the success return, and a
// cancellation detected after the round already completed and parsed -- that return is an error,
// but its artifacts survive so the next call advances to the following round instead of re-running
// this one.
//
// No mid-run cancellation bridge is installed, because internal/burlerengine exposes no pause
// seam: a cancel is observed only once the round reaches a terminal outcome or its own
// RunOpts.Timeout elapses.
func (p *BurlerProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, burlerEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if err := os.MkdirAll(p.runDir, 0o755); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): create run dir %q: %w", p.name, burlerEngineLabel, p.runDir, err)
	}

	highest, err := highestCompleteRound(p.runDir)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve round: %w", p.name, burlerEngineLabel, err)
	}
	round := highest + 1

	reviewPath := roundReviewPath(p.runDir, round)
	fixerReportPath := roundFixerReportPath(p.runDir, round)

	focus := readRoundFocus(p.name, p.runDir, round)
	priorReviews, priorFixerReports, err := hydrationPaths(p.runDir, round)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve hydration: %w", p.name, burlerEngineLabel, err)
	}

	// A fresh copy of the stored template, built per round: every round must carry its own
	// ReviewPath, FixerReportPath, PriorReviews, PriorFixerReports, and ClusterExclude values, and
	// a reused copy would leak the previous round's values into the next one. The stored template
	// itself is never mutated -- every slice field set below is a freshly allocated slice, never an
	// in-place append onto p.profile's own backing array.
	profile := p.profile
	profile.ReviewPath = reviewPath
	profile.FixerReportPath = fixerReportPath
	profile.PriorReviews = append(append([]string{}, p.profile.PriorReviews...), priorReviews...)
	profile.PriorReviews = append(profile.PriorReviews, focus.Hydrate...)
	profile.PriorFixerReports = append(append([]string{}, p.profile.PriorFixerReports...), priorFixerReports...)
	profile.ClusterExclude = nil
	if p.profile.ClusterFan != "" {
		profile.ClusterExclude = focus.ExcludeLenses
	} else if len(focus.ExcludeLenses) > 0 {
		// A well-formed but unusable directive must never become a validate hard error
		// downstream -- the fan is authoritative config, the focus file is an advisory,
		// LLM-authored directive.
		logger.Warn("shedadapters: focus directive names cluster excludes but the template profile has no cluster fan; dropping", "producer", p.name, "engine", burlerEngineLabel, "round", round)
	}

	// archiveRound renames both of this round's own paths to stamped siblings, logging (but never
	// masking) a failure -- every caller below is on a failure exit whose round did not produce a
	// usable review.
	archiveRound := func() {
		if archErr := archiveStaleOutputs([]string{reviewPath, fixerReportPath}, p.now); archErr != nil {
			logger.Warn("shedadapters: archive round outputs on exit failed", "producer", p.name, "engine", burlerEngineLabel, "round", round, "error", archErr)
		}
	}

	// failureExit is the shared tail of every non-success return: cancelErr is consulted first,
	// exactly as the other adapters do, then the round's paths are archived before returning
	// whichever error takes precedence.
	failureExit := func(primaryErr error) (shedengine.Outcome, shedengine.OutputPointer, error) {
		if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
			archiveRound()
			return "", shedengine.OutputPointer{}, cerr
		}
		archiveRound()
		return "", shedengine.OutputPointer{}, primaryErr
	}

	var priorResult burlerengine.Result
	var priorToken string
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt == 2 {
			// Re-check ctx.Err() before spawning a fresh, expensive round.
			if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
				archiveRound()
				return "", shedengine.OutputPointer{}, cerr
			}
		}

		// Before every attempt, including attempt 1: a leftover file at the round's own paths is
		// renamed to a stamped sibling rather than being passed through to a run whose spec
		// validation rejects a pre-existing output file.
		if err := archiveStaleOutputs([]string{reviewPath, fixerReportPath}, p.now); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: archive stale outputs before attempt %d: %w", p.name, burlerEngineLabel, round, attempt, err)
		}

		// The attempt-distinguishing token belongs on RunOpts.Round, which names the shuttle run,
		// and never on the artifact paths, which stay canonical per round.
		attemptToken := strconv.Itoa(round)
		if attempt == 2 {
			attemptToken += "b"
		}
		attemptOpts := p.opts
		attemptOpts.Round = attemptToken

		result, runErr := p.runner.Run(profile, attemptOpts)
		if runErr != nil {
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: run: %w", p.name, burlerEngineLabel, round, attemptToken, runErr))
		}

		switch result.Outcome {
		case shuttleengine.OutcomeDone:
			// A genuine success verdict survives cancellation only up to the moment the round
			// completed and parsed; a cancellation observed after that point still yields an
			// error (internal/shedengine binds every implementation to surface cancellation as a
			// non-nil error, never as Stuck), but the already-complete artifacts survive.
			if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return shedengine.Stuck, shedengine.OutputPointer{Path: reviewPath}, nil

		case shuttleengine.OutcomeAsking:
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: shuttle run is asking: %s", p.name, burlerEngineLabel, round, attemptToken, result.LastAssistantMessage))

		case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
			if attempt == 1 {
				logger.Warn("shedadapters: burler round attempt died or timed out, retrying", "producer", p.name, "engine", burlerEngineLabel, "round", round, "attempt", attemptToken, "outcome", result.Outcome, "sessionID", result.SessionID)
				priorResult = result
				priorToken = attemptToken
				continue
			}
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d: two consecutive died/timeout outcomes (attempt %s outcome %s session %s run dir %s; attempt %s outcome %s session %s run dir %s)", p.name, burlerEngineLabel, round, priorToken, priorResult.Outcome, priorResult.SessionID, priorResult.RunDir, attemptToken, result.Outcome, result.SessionID, result.RunDir))

		default:
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: unrecognized burler outcome %q", p.name, burlerEngineLabel, round, attemptToken, result.Outcome))
		}
	}

	// Unreachable: every path through the loop above returns.
	return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: attempt loop exited without a verdict", p.name, burlerEngineLabel, round)
}
