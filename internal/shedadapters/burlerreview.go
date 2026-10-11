// burlerreview.go implements BurlerReviewProducer, the shedadapters adapter over the review half of a burlerengine round:
// it resolves the round to review from disk, hydrates prior rounds into the profile, and runs one reviewer session with a bounded retry on died/timeout.
// It records the accepted review in the round's acceptance record and hands off to the segment's fix row as a routine Stuck -- never Done.
// It also declares BurlerHalfRunner and BurlerHalfDeps, the seam and collaborators both halves' producers share.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// burlerReviewEngineLabel and burlerFixEngineLabel name the two producers in logs and errors.
const (
	burlerReviewEngineLabel = "burler-review"
	burlerFixEngineLabel    = "burler-fix"
)

// BurlerHalfRunner is the narrow seam the review and fix producers drive one half of a burler round through:
// Review and Fix spawn a half, ProbeReview and ProbeFix report what became of a previous session's half, and ResumeReview and ResumeFix complete a half that is still live.
type BurlerHalfRunner interface {
	Review(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.ReviewResult, error)
	ProbeReview(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.LiveHalf, error)
	ResumeReview(p burlerengine.Profile, opts burlerengine.RunOpts, live burlerengine.LiveHalf) (burlerengine.ReviewResult, error)
	Fix(p burlerengine.Profile, opts burlerengine.RunOpts, digest string) (burlerengine.FixResult, error)
	ProbeFix(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.LiveHalf, error)
	ResumeFix(p burlerengine.Profile, opts burlerengine.RunOpts, digest string, live burlerengine.LiveHalf) (burlerengine.FixResult, error)
}

// Compile-time proof that *burlerengine.Engine satisfies BurlerHalfRunner.
var _ BurlerHalfRunner = (*burlerengine.Engine)(nil)

// BurlerHalfDeps are the told collaborators and settings of a review or fix producer.
type BurlerHalfDeps struct {
	// Runner drives one half.
	Runner BurlerHalfRunner
	// Models holds the per-round review and fix model lists; each round runs on the pick for its number.
	Models burlerengine.RoundModels
}

// checkHalfProducerArgs returns the constructor error of a half producer named name under label, or nil.
// It is a distinct error for each of: a nil runner, an empty name, an empty runDir and a runDir that is not absolute.
func checkHalfProducerArgs(name, label string, haveRunner bool, runDir string) error {
	switch {
	case !haveRunner:
		return fmt.Errorf("shedadapters: %s (%s): runner must not be nil", name, label)
	case name == "":
		return fmt.Errorf("shedadapters: %s (%s): name must not be empty", name, label)
	case runDir == "":
		return fmt.Errorf("shedadapters: %s (%s): runDir must not be empty", name, label)
	case !filepath.IsAbs(runDir):
		return fmt.Errorf("shedadapters: %s (%s): runDir %q is not absolute", name, label, runDir)
	}
	return nil
}

// BurlerReviewProducer is the shedadapters adapter over the review half of a burlerengine round.
//
// Call returns shedengine.Stuck on every successful review and never shedengine.Done -- that Stuck is a routine hand-off to the segment's fix row via OnStuck, never a real stuck condition.
// A review that was not accepted after the bounded retry is a hard error rather than Stuck.
// The Bouncer tells its seed call from its judge call by the round artifacts on disk, so a failed review returning Stuck with no review written would be misread as a seed call.
//
// Every hand-back is BudgetExempt: the segment's budget on judged rounds is the Bouncer row's own check, and this row has no counted exit.
type BurlerReviewProducer struct {
	name    string
	runner  BurlerHalfRunner
	models  burlerengine.RoundModels
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
	runDir  string
	now     func() time.Time
}

// Compile-time proof that *BurlerReviewProducer satisfies shedengine.ShedProducer.
var _ shedengine.ShedProducer = (*BurlerReviewProducer)(nil)

// NewBurlerReviewProducer returns a BurlerReviewProducer identified as name, driving profile through deps.Runner under opts, with round artifacts under runDir.
// profile is a template whose ReviewPath, FixerReportPath, FocusDirective, PriorReviews, PriorFixerReports and ClusterExclude fields are overwritten per round;
// opts is a template whose Round, Review and Fix fields are overwritten per attempt, the last two from deps.Models.
// A nil now defaults to time.Now, and the injected clock resolves only the archive filename's same-second collision suffix.
// It returns a distinct error for each of: a nil runner, an empty name, an empty runDir and a runDir that is not absolute.
// It never stats, creates or otherwise touches runDir -- creating it is Call's job.
func NewBurlerReviewProducer(name string, deps BurlerHalfDeps, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time) (*BurlerReviewProducer, error) {
	if err := checkHalfProducerArgs(name, burlerReviewEngineLabel, deps.Runner != nil, runDir); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &BurlerReviewProducer{name: name, runner: deps.Runner, models: deps.Models, profile: profile, opts: opts, runDir: runDir, now: now}, nil
}

// roundProfile returns a fresh per-round copy of the stored template profile for round:
// the round's own review and fixer-report paths, focus directive, hydration of earlier complete rounds and, from round 2 on, the cluster exclusions.
// The stored template itself is never mutated -- every slice field set is a freshly allocated slice.
func (p *BurlerReviewProducer) roundProfile(round int) burlerengine.Profile {
	focus := readRoundFocus(p.name, p.runDir, round)
	priorReviews, priorFixerReports, _ := hydrationPaths(p.runDir, round)

	profile := p.profile
	profile.ReviewPath = roundReviewPath(p.runDir, round)
	profile.FixerReportPath = roundFixerReportPath(p.runDir, round)
	profile.FocusDirective = focus.DirectivePath
	profile.PriorReviews = append(append([]string{}, p.profile.PriorReviews...), priorReviews...)
	profile.PriorFixerReports = append(append([]string{}, p.profile.PriorFixerReports...), priorFixerReports...)
	profile.ClusterExclude = nil
	profile.ClusterExcludeHeld = nil
	switch {
	case p.profile.ClusterFan != "" && round >= 2:
		var dropped []string
		profile.ClusterExclude, dropped = excludedLenses(p.name, p.runDir, round, p.profile.ClusterFan)
		profile.ClusterExcludeHeld, _ = excludedLenses(p.name, p.runDir, round-1, p.profile.ClusterFan)
		if len(dropped) > 0 {
			logger.Warn("shedadapters: focus file excludes lenses the previous round did not run under this fan; dropping them", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "lenses", dropped)
		}
	case p.profile.ClusterFan != "" && len(focus.ExcludeLenses) > 0:
		logger.Warn("shedadapters: round 1 focus file names cluster excludes but the whole fan runs in round 1; dropping them", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "lenses", focus.ExcludeLenses)
	case len(focus.ExcludeLenses) > 0:
		logger.Warn("shedadapters: focus file names cluster excludes but this round's profile has no cluster fan; dropping them", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "lenses", focus.ExcludeLenses)
	}
	return profile
}

// Call runs one review: resolve the round to review from disk and hand control on without spawning when the round is already reviewed or still unjudged.
// Otherwise it builds the round's profile, runs at most two attempts (a second only on a died/timeout first attempt), and maps the outcome onto shedengine's contract.
//
// The round to review is the highest complete round's successor only when that round carries a parsing Bouncer verdict and ledger.
// Otherwise this call hands the review of the round still owed a verdict straight on, BudgetExempt, spawning nothing, so the fix row passes it to the Bouncer.
// A round whose review already carries an acceptance record is handed on the same way, so a restart with the fixer pending resumes at the fix row without re-running the reviewer.
//
// A live reviewer is probed before anything is archived, because archiving renames the very file a live reviewer is about to write.
// Every return in which the review was not accepted archives the round's review, fixer report and acceptance record first.
// Two exits archive nothing: a reviewer that could not be stopped, whose files a live session may still be writing, and a cancellation observed after the review was accepted and recorded.
//
// No mid-run cancellation bridge is installed, because burlerengine exposes no pause seam: a cancel is observed only once the review reaches a terminal outcome or its own timeout elapses.
func (p *BurlerReviewProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, burlerReviewEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := os.MkdirAll(p.runDir, 0o755); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): create run dir %q: %w", p.name, burlerReviewEngineLabel, p.runDir, err)
	}

	highest, err := highestCompleteRound(p.runDir)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve round: %w", p.name, burlerReviewEngineLabel, err)
	}
	if highest > 0 {
		if _, _, judged := recordedVerdict(p.runDir, highest); !judged {
			logger.Warn("shedadapters: burler review producer reached with the highest complete round unjudged; handing on for judgment instead of running a fresh review", "producer", p.name, "engine", burlerReviewEngineLabel, "round", highest)
			return p.handOn(ctx, roundReviewPath(p.runDir, highest), fmt.Sprintf("round %d is complete but unjudged; handing on for judgment", highest))
		}
	}

	round := highest + 1
	reviewPath := roundReviewPath(p.runDir, round)
	_, accepted, err := readAcceptance(p.runDir, round)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: %w", p.name, burlerReviewEngineLabel, round, err)
	}
	if _, statErr := os.Stat(reviewPath); accepted && statErr == nil {
		return p.handOn(ctx, reviewPath, fmt.Sprintf("round %d review is accepted; handing on to the fixer", round))
	}

	profile := p.roundProfile(round)
	paths := []string{profile.ReviewPath, profile.FixerReportPath, roundAcceptancePath(p.runDir, round)}
	archiveRound := func() {
		if archErr := archiveStaleOutputs(paths, p.now); archErr != nil {
			logger.Warn("shedadapters: archive round outputs on exit failed", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", archErr)
		}
	}
	failureExit := func(primaryErr error) (shedengine.Outcome, shedengine.OutputPointer, error) {
		archiveRound()
		if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, primaryErr
	}

	if outcome, ptr, exitErr, handled := p.probeLiveReview(ctx, round, profile, archiveRound); handled {
		return outcome, ptr, exitErr
	}

	var priorResult burlerengine.ReviewResult
	var priorToken string
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt == 2 {
			if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
				archiveRound()
				return "", shedengine.OutputPointer{}, cerr
			}
		}

		// A leftover file at the round's own paths is renamed to a stamped sibling rather than being passed to a run whose spec validation rejects a pre-existing output file.
		if err := archiveStaleOutputs(paths, p.now); err != nil {
			if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: archive stale outputs before attempt %d: %w", p.name, burlerReviewEngineLabel, round, attempt, err)
		}

		attemptToken := strconv.Itoa(round)
		if attempt == 2 {
			attemptToken += "b"
		}
		attemptOpts := p.opts
		attemptOpts.Round = attemptToken
		attemptOpts.Review, attemptOpts.Fix = p.models.Pick(round)

		result, runErr := p.runner.Review(profile, attemptOpts)
		if errors.Is(runErr, burlerengine.ErrHalfNotStopped) {
			return p.unstoppedExit(ctx, fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: review: %w", p.name, burlerReviewEngineLabel, round, attemptToken, runErr))
		}
		if runErr != nil {
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: review: %w", p.name, burlerReviewEngineLabel, round, attemptToken, runErr))
		}

		switch result.Outcome {
		case shuttleengine.OutcomeDone:
			return p.acceptedExit(ctx, round, result, archiveRound)

		case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
			if attempt == 1 {
				logger.Warn("shedadapters: burler review attempt died or timed out, retrying", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "attempt", attemptToken, "outcome", result.Outcome, "review", describeHalf("review", result.Review))
				priorResult, priorToken = result, attemptToken
				continue
			}
			twice := fmt.Errorf("shedadapters: %s (%s): round %d: two consecutive died/timeout outcomes (attempt %s outcome %s %s; attempt %s outcome %s %s)", p.name, burlerReviewEngineLabel, round, priorToken, priorResult.Outcome, describeHalf("review", priorResult.Review), attemptToken, result.Outcome, describeHalf("review", result.Review))
			if result.NotStarted {
				twice = fmt.Errorf("%w: %w", twice, shuttleengine.ErrNotStarted)
			}
			return failureExit(twice)

		default:
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: unrecognized burler outcome %q", p.name, burlerReviewEngineLabel, round, attemptToken, result.Outcome))
		}
	}

	// Unreachable: every path through the loop above returns.
	return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: attempt loop exited without a verdict", p.name, burlerReviewEngineLabel, round)
}

// handOn returns the routine Stuck that hands the review at path on without spawning anything, BudgetExempt.
// A cancelled context takes precedence.
func (p *BurlerReviewProducer) handOn(ctx context.Context, path, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Stuck, shedengine.OutputPointer{Path: path, Reason: reason, BudgetExempt: true}, nil
}

// unstoppedExit returns err without archiving, unless the context was cancelled, whose error takes precedence.
// A reviewer that could not be stopped may still be writing the round's files.
func (p *BurlerReviewProducer) unstoppedExit(ctx context.Context, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return "", shedengine.OutputPointer{}, err
}

// acceptedExit maps a review that reached OutcomeDone onto Call's contract, for a spawned and a resumed review alike.
// It records the accepted review's digest in the round's acceptance record and merges the review half's usage into the round's usage record.
// It returns Stuck with the review as Path, BudgetExempt.
// A failed acceptance write is a hard error with the round's paths archived, since a review with no record would be reviewed again on resume.
// A cancellation observed after the record was written still yields an error, but the accepted review and its record survive.
func (p *BurlerReviewProducer) acceptedExit(ctx context.Context, round int, result burlerengine.ReviewResult, archiveRound func()) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := writeAcceptance(p.runDir, round, result.Digest); err != nil {
		archiveRound()
		if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: %w", p.name, burlerReviewEngineLabel, round, err)
	}
	if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	review, _ := p.models.Pick(round)
	half := reviewHalfUsage(review, result.Review)
	err := mergeRoundUsage(p.runDir, round, p.now, func(u *roundUsage) {
		u.Fan, u.Lenses, u.Review = p.profile.ClusterFan, result.Lenses, &half
	})
	if err != nil {
		logger.Warn("shedadapters: burler review's usage record was not written", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
	}
	return shedengine.Stuck, shedengine.OutputPointer{Path: roundReviewPath(p.runDir, round), BudgetExempt: true}, nil
}

// reviewHalfUsage builds the review half's usage record from the model it ran on and the half's identity, with the reviewer's fork reading when it is known.
func reviewHalfUsage(model burlerengine.ModelChoice, half burlerengine.Half) halfUsage {
	record := newHalfUsage(model, half)
	if usage := half.Usage; usage.Known {
		forks := usage.Forks
		record.Forks = &forks
		record.ForkFreshTokens = &usage.ForkFresh
		record.ForkCacheReadTokens = &usage.ForkCacheRead
		if usage.Fresh > 0 {
			share := float64(usage.ForkFresh) / float64(usage.Fresh)
			record.ForkFreshShare = &share
		}
	}
	return record
}

// probeLiveReview asks the runner what became of a previous session's reviewer for this round and resumes it when it is still live.
//
// It reports handled=false whenever the caller should proceed to its ordinary archive-then-spawn path:
// the reviewer is not live, or the resumed reviewer had already died or timed out, which falls through to a fresh attempt 1.
// Every other case reports handled=true along with the three values Call must return.
// A live reviewer is resumed through ResumeReview and mapped like a spawned one.
//
// A probe error and a resume error wrapping burlerengine.ErrHalfNotStopped are returned bare rather than through the archiving failure exit:
// a reviewer that may still be live may be mid-write on the round's files, so archiving them is most dangerous there.
func (p *BurlerReviewProducer) probeLiveReview(ctx context.Context, round int, profile burlerengine.Profile, archiveRound func()) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	opts := p.opts
	opts.Round = strconv.Itoa(round)
	opts.Review, opts.Fix = p.models.Pick(round)

	live, err := p.runner.ProbeReview(profile, opts)
	if err != nil {
		outcome, ptr, exitErr := p.unstoppedExit(ctx, fmt.Errorf("shedadapters: %s (%s): round %d: probe the reviewer: %w", p.name, burlerReviewEngineLabel, round, err))
		return outcome, ptr, exitErr, true
	}
	if live.State != burlerengine.HalfLive {
		return "", shedengine.OutputPointer{}, nil, false
	}

	result, err := p.runner.ResumeReview(profile, opts, live)
	if err != nil {
		wrapped := fmt.Errorf("shedadapters: %s (%s): round %d: resume: %w", p.name, burlerReviewEngineLabel, round, err)
		if errors.Is(err, burlerengine.ErrHalfNotStopped) {
			outcome, ptr, exitErr := p.unstoppedExit(ctx, wrapped)
			return outcome, ptr, exitErr, true
		}
		archiveRound()
		if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, wrapped, true
	}

	logger.Info("shedadapters: resumed a live burler reviewer instead of respawning", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "review", describeHalf("review", result.Review))

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		outcome, ptr, exitErr := p.acceptedExit(ctx, round, result, archiveRound)
		return outcome, ptr, exitErr, true

	case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
		logger.Warn("shedadapters: resumed burler reviewer had already died or timed out; respawning", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "outcome", result.Outcome)
		return "", shedengine.OutputPointer{}, nil, false

	default:
		archiveRound()
		if cerr := cancelErr(ctx, p.name, burlerReviewEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d resumed run reported unrecognized outcome %q", p.name, burlerReviewEngineLabel, round, result.Outcome), true
	}
}
