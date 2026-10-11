// burlerfix.go implements BurlerFixProducer, the shedadapters adapter over the fix half of a burlerengine round:
// it reads the round's acceptance record and runs one fixer session over the accepted review with a bounded retry on died/timeout.
// It hands off to the segment's Bouncer as a routine Stuck -- never Done.

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

// BurlerFixProducer is the shedadapters adapter over the fix half of a burlerengine round.
//
// Call returns shedengine.Stuck on every successful fix and never shedengine.Done -- that Stuck is a routine hand-off to the segment's Bouncer via OnStuck, never a real stuck condition.
// A fixer that did not finish after the bounded retry is a hard error rather than Stuck.
//
// Both hand-backs for judgment, a finished fix and the highest complete round found still unjudged, are BudgetExempt.
// Only the gate-failed exit is counted against this row, so its MaxBounces caps the gate-failed rounds of one generation.
type BurlerFixProducer struct {
	name      string
	reviewRow string
	runner    BurlerHalfRunner
	models    burlerengine.RoundModels
	profile   burlerengine.Profile
	opts      burlerengine.RunOpts
	runDir    string
	now       func() time.Time
}

// Compile-time proof that *BurlerFixProducer satisfies shedengine.ShedProducer.
var _ shedengine.ShedProducer = (*BurlerFixProducer)(nil)

// NewBurlerFixProducer returns a BurlerFixProducer identified as name, driving profile through deps.Runner under opts, with round artifacts under runDir.
// reviewRow names the review row of the same segment, which the refusals' way forward sends the operator back to.
// profile is a template whose ReviewPath and FixerReportPath fields are overwritten per round;
// opts is a template whose Round, Review, Fix and NoteID fields are overwritten per attempt, the first three from the attempt and deps.Models, and whose Gate gates the fixer.
// A nil now defaults to time.Now, and the injected clock resolves only the archive filename's same-second collision suffix.
// It returns a distinct error for each of: a nil runner, an empty name, an empty reviewRow, an empty runDir and a runDir that is not absolute.
// It never stats, creates or otherwise touches runDir -- creating it is Call's job.
func NewBurlerFixProducer(name, reviewRow string, deps BurlerHalfDeps, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time) (*BurlerFixProducer, error) {
	if err := checkHalfProducerArgs(name, burlerFixEngineLabel, deps.Runner != nil, runDir); err != nil {
		return nil, err
	}
	if reviewRow == "" {
		return nil, fmt.Errorf("shedadapters: %s (%s): review row must not be empty", name, burlerFixEngineLabel)
	}
	if now == nil {
		now = time.Now
	}
	return &BurlerFixProducer{name: name, reviewRow: reviewRow, runner: deps.Runner, models: deps.Models, profile: profile, opts: opts, runDir: runDir, now: now}, nil
}

// wayForward is the way forward every refusal that needs the round reviewed again ends with.
func (p *BurlerFixProducer) wayForward() string {
	return fmt.Sprintf("way forward: run \"lyx loom goto --to %s\", then \"lyx loom resume\", in the task worktree", p.reviewRow)
}

// archive renames the files of round that exist to stamped siblings, logging but never masking a failure.
// Every caller is on an exit whose round did not finish.
func (p *BurlerFixProducer) archive(round int, files []string) {
	if err := archiveStaleOutputs(files, p.now); err != nil {
		logger.Warn("shedadapters: archive round outputs on exit failed", "producer", p.name, "engine", burlerFixEngineLabel, "round", round, "error", err)
	}
}

// archiveWholeRound archives round's review, fixer report and acceptance record together, so no record outlives its review.
func (p *BurlerFixProducer) archiveWholeRound(round int) {
	p.archive(round, []string{roundReviewPath(p.runDir, round), roundFixerReportPath(p.runDir, round), roundAcceptancePath(p.runDir, round)})
}

// archiveReport archives round's fixer report alone, keeping the accepted review and its record so a resume re-spawns the fixer without re-running the reviewer.
func (p *BurlerFixProducer) archiveReport(round int) {
	p.archive(round, []string{roundFixerReportPath(p.runDir, round)})
}

// Call runs one fix: resolve the round to fix from disk and hand control on without spawning when the highest complete round is still unjudged.
// It refuses a round with no accepted review, runs at most two attempts over the accepted review (a second only on a died/timeout first attempt), and maps the outcome onto shedengine's contract.
//
// A live fixer is probed before anything is archived, because archiving renames the very file a live fixer is about to write.
// Only a stale fixer report is archived before an attempt.
// A fixer that finished with a failed gate, and a review that changed since it was accepted, archive the review, fixer report and acceptance record together.
// The Bouncer's next round, or the operator's goto to the review row, then reviews afresh.
// Every other failure archives the fixer report alone.
// A fixer that could not be stopped returns its error with nothing archived, since a live session may still be writing the round's files.
func (p *BurlerFixProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, burlerFixEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := os.MkdirAll(p.runDir, 0o755); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): create run dir %q: %w", p.name, burlerFixEngineLabel, p.runDir, err)
	}

	highest, err := highestCompleteRound(p.runDir)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve round: %w", p.name, burlerFixEngineLabel, err)
	}
	if highest > 0 {
		if _, _, judged := recordedVerdict(p.runDir, highest); !judged {
			if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			logger.Warn("shedadapters: burler fix producer reached with the highest complete round unjudged; handing on for judgment instead of running a fixer", "producer", p.name, "engine", burlerFixEngineLabel, "round", highest)
			return shedengine.Stuck, shedengine.OutputPointer{Path: roundReviewPath(p.runDir, highest), Reason: fmt.Sprintf("round %d is complete but unjudged; handing on for judgment", highest), BudgetExempt: true}, nil
		}
	}

	round := highest + 1
	reviewPath := roundReviewPath(p.runDir, round)
	digest, accepted, err := readAcceptance(p.runDir, round)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: %w", p.name, burlerFixEngineLabel, round, err)
	}
	if _, statErr := os.Stat(reviewPath); !accepted || statErr != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d has no accepted review; %s", p.name, burlerFixEngineLabel, round, p.wayForward())
	}

	profile := p.profile
	profile.ReviewPath = reviewPath
	profile.FixerReportPath = roundFixerReportPath(p.runDir, round)
	archiveRound := func() { p.archiveWholeRound(round) }

	if outcome, ptr, exitErr, handled := p.probeLiveFix(ctx, round, profile, digest, archiveRound); handled {
		return outcome, ptr, exitErr
	}

	var priorResult burlerengine.FixResult
	var priorToken string
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt == 2 {
			if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
				p.archiveReport(round)
				return "", shedengine.OutputPointer{}, cerr
			}
		}

		if err := archiveStaleOutputs([]string{profile.FixerReportPath}, p.now); err != nil {
			if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: archive stale outputs before attempt %d: %w", p.name, burlerFixEngineLabel, round, attempt, err)
		}

		attemptToken := strconv.Itoa(round)
		if attempt == 2 {
			attemptToken += "b"
		}
		attemptOpts := p.opts
		attemptOpts.Round = attemptToken
		attemptOpts.Review, attemptOpts.Fix = p.models.Pick(round)
		// The run directory's base is the segment's own run_subdir recipe value, so this stem distinguishes one segment's round from another's in one friction directory.
		attemptOpts.NoteID = "burler-" + filepath.Base(p.runDir) + "-r" + strconv.Itoa(round)

		result, runErr := p.runner.Fix(profile, attemptOpts, digest)
		if runErr != nil {
			return p.errorExit(ctx, round, attemptToken, "fix", runErr)
		}

		switch result.Outcome {
		case shuttleengine.OutcomeDone:
			return p.fixedExit(ctx, round, result, archiveRound)

		case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
			if attempt == 1 {
				logger.Warn("shedadapters: burler fix attempt died or timed out, retrying", "producer", p.name, "engine", burlerFixEngineLabel, "round", round, "attempt", attemptToken, "outcome", result.Outcome, "fix", describeHalf("fix", result.Fix))
				priorResult, priorToken = result, attemptToken
				continue
			}
			twice := fmt.Errorf("shedadapters: %s (%s): round %d: two consecutive died/timeout outcomes (attempt %s outcome %s %s; attempt %s outcome %s %s)", p.name, burlerFixEngineLabel, round, priorToken, priorResult.Outcome, describeHalf("fix", priorResult.Fix), attemptToken, result.Outcome, describeHalf("fix", result.Fix))
			if result.NotStarted {
				twice = fmt.Errorf("%w: %w", twice, shuttleengine.ErrNotStarted)
			}
			return p.reportFailureExit(ctx, round, twice)

		default:
			return p.reportFailureExit(ctx, round, fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: unrecognized burler outcome %q", p.name, burlerFixEngineLabel, round, attemptToken, result.Outcome))
		}
	}

	// Unreachable: every path through the loop above returns.
	return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: attempt loop exited without a verdict", p.name, burlerFixEngineLabel, round)
}

// reportFailureExit archives round's fixer report and returns err, unless the context was cancelled, whose error takes precedence.
func (p *BurlerFixProducer) reportFailureExit(ctx context.Context, round int, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.archiveReport(round)
	if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return "", shedengine.OutputPointer{}, err
}

// errorExit maps the error runErr of Fix or ResumeFix (verb names which) for attempt token onto Call's contract.
// A fixer that could not be stopped returns bare with nothing archived.
// A changed review archives the whole round and ends with the way forward back to the review row.
// Any other error archives the fixer report alone.
func (p *BurlerFixProducer) errorExit(ctx context.Context, round int, token, verb string, runErr error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	switch {
	case errors.Is(runErr, burlerengine.ErrHalfNotStopped):
		if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: %s: %w", p.name, burlerFixEngineLabel, round, token, verb, runErr)
	case errors.Is(runErr, burlerengine.ErrReviewChanged):
		p.archiveWholeRound(round)
		if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: %s: %w; %s", p.name, burlerFixEngineLabel, round, token, verb, runErr, p.wayForward())
	}
	return p.reportFailureExit(ctx, round, fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: %s: %w", p.name, burlerFixEngineLabel, round, token, verb, runErr))
}

// fixedExit maps a fix that reached OutcomeDone onto Call's contract, for a spawned, a resumed and a skipped fix alike.
//
// A failed fixer gate archives the review, fixer report and acceptance record and returns a counted Stuck with an empty Path, the gate's reason and attempts.
// The Bouncer's next round then reviews afresh; a gate exhaustion is a determinate verdict, so it consumes no retry.
// Otherwise it merges the fix half's usage into the round's usage record and returns Stuck with the review as Path, BudgetExempt.
// A cancellation observed after the fix finished still yields an error, but a finished round's artifacts survive.
func (p *BurlerFixProducer) fixedExit(ctx context.Context, round int, result burlerengine.FixResult, archiveRound func()) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if result.Gate != nil && !result.Gate.Passed {
		archiveRound()
		if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: burler fix's gate did not pass", "producer", p.name, "engine", burlerFixEngineLabel, "round", round, "attempts", result.Gate.Attempts, "findingsPath", result.Gate.FindingsPath)
		return shedengine.Stuck, shedengine.OutputPointer{GateAttempts: gateAttemptsPointer(result.Gate), Reason: gateFailedReason(result.Gate)}, nil
	}
	if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	_, fix := p.models.Pick(round)
	half := newHalfUsage(fix, result.Fix)
	if err := mergeRoundUsage(p.runDir, round, p.now, func(u *roundUsage) { u.Fix = &half }); err != nil {
		logger.Warn("shedadapters: burler fix's usage record was not written", "producer", p.name, "engine", burlerFixEngineLabel, "round", round, "error", err)
	}
	return shedengine.Stuck, shedengine.OutputPointer{Path: roundReviewPath(p.runDir, round), GateAttempts: gateAttemptsPointer(result.Gate), BudgetExempt: true}, nil
}

// probeLiveFix asks the runner what became of a previous session's fixer for this round and resumes it, against digest, when it is still live.
//
// It reports handled=false whenever the caller should proceed to its ordinary archive-then-spawn path:
// the fixer is not live, or the resumed fixer had already died or timed out, which falls through to a fresh attempt 1.
// Every other case reports handled=true along with the three values Call must return.
// A probe error and a resume error wrapping burlerengine.ErrHalfNotStopped are returned bare rather than through an archiving exit:
// a fixer that may still be live may be mid-write on the round's files.
func (p *BurlerFixProducer) probeLiveFix(ctx context.Context, round int, profile burlerengine.Profile, digest string, archiveRound func()) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	token := strconv.Itoa(round)
	opts := p.opts
	opts.Round = token
	opts.Review, opts.Fix = p.models.Pick(round)
	opts.NoteID = "burler-" + filepath.Base(p.runDir) + "-r" + token

	live, err := p.runner.ProbeFix(profile, opts)
	if err != nil {
		if cerr := cancelErr(ctx, p.name, burlerFixEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: probe the fixer: %w", p.name, burlerFixEngineLabel, round, err), true
	}
	if live.State != burlerengine.HalfLive {
		return "", shedengine.OutputPointer{}, nil, false
	}

	result, err := p.runner.ResumeFix(profile, opts, digest, live)
	if err != nil {
		outcome, ptr, exitErr := p.errorExit(ctx, round, token, "resume", err)
		return outcome, ptr, exitErr, true
	}

	logger.Info("shedadapters: resumed a live burler fixer instead of respawning", "producer", p.name, "engine", burlerFixEngineLabel, "round", round, "fix", describeHalf("fix", result.Fix))

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		outcome, ptr, exitErr := p.fixedExit(ctx, round, result, archiveRound)
		return outcome, ptr, exitErr, true

	case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
		logger.Warn("shedadapters: resumed burler fixer had already died or timed out; respawning", "producer", p.name, "engine", burlerFixEngineLabel, "round", round, "outcome", result.Outcome)
		return "", shedengine.OutputPointer{}, nil, false

	default:
		outcome, ptr, exitErr := p.reportFailureExit(ctx, round, fmt.Errorf("shedadapters: %s (%s): round %d resumed run reported unrecognized outcome %q", p.name, burlerFixEngineLabel, round, result.Outcome))
		return outcome, ptr, exitErr, true
	}
}
