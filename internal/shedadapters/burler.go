// burler.go implements BurlerProducer, the shedadapters adapter over one burlerengine round: it
// resolves the round to run from disk via the review/fixer-report pair predicate, hydrates prior
// rounds into the profile, runs one burlerengine round with a bounded retry on died/timeout, and
// maps its outcome onto the shedengine.ShedProducer contract as a routine Stuck hand-off to the
// segment's Bouncer -- never Done.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/burlermarker"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// BurlerRunner is the narrow seam BurlerProducer drives one burler round through:
// Run spawns a round, ProbeRound reports what became of a previous session's two halves, and Resume completes a round whose halves are both live.
type BurlerRunner interface {
	Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error)
	ProbeRound(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.LiveRound, error)
	Resume(p burlerengine.Profile, opts burlerengine.RunOpts, live burlerengine.LiveRound) (burlerengine.Result, error)
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
	remover burlerengine.StrandRemover
	models  burlerengine.RoundModels
	anchor  string
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
	runDir  string
	now     func() time.Time
}

// BurlerDeps are the told collaborators and settings of a BurlerProducer.
type BurlerDeps struct {
	// Runner drives one round.
	Runner BurlerRunner
	// Remover stops the live half of a round that has only one live half, required.
	// It is the same remover the engine is told, so a half is stopped one way on every path.
	Remover burlerengine.StrandRemover
	// Models holds the per-round review and fix model lists; each round runs on the pick for its number.
	Models burlerengine.RoundModels
	// AnchorPath is the absolute anchor the round's ready marker is derived under, through burlermarker.Path.
	AnchorPath string
}

// NewBurlerProducer returns a BurlerProducer identified as name, driving profile through deps.Runner under opts, with round artifacts under runDir.
// profile is a template whose ReviewPath, FixerReportPath, FocusDirective, PriorReviews, PriorFixerReports, and ClusterExclude fields are overwritten per round;
// opts is a template whose Round, Review and Fix fields are overwritten per attempt, the last two from deps.Models.
// A nil now defaults to time.Now, and the injected clock resolves only the archive filename's
// same-second collision suffix.
// profile's ReadyMarkerPath is overwritten per round too, from deps.AnchorPath.
// It returns a distinct error for each of: a nil runner, a nil remover, an empty name, an empty
// runDir, a runDir that is not absolute per filepath.IsAbs, and an anchor path that is not absolute.
// NewBurlerProducer never stats, creates, or otherwise touches runDir -- creating it is Call's job.
//
// deps.Remover is required rather than optional.
// A round this producer respawns beside a still-live half produces two concurrent sessions writing the same review or fixer-report file.
// On a fix-scope: source row, that is two sessions holding commit authority over the same branch.
// Accepting a nil seam would make that outcome reachable again through a wiring slip, silently, which is exactly how it shipped the first time.
func NewBurlerProducer(name string, deps BurlerDeps, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time) (*BurlerProducer, error) {
	if deps.Runner == nil {
		return nil, fmt.Errorf("shedadapters: %s (%s): runner must not be nil", name, burlerEngineLabel)
	}
	if deps.Remover == nil {
		return nil, fmt.Errorf("shedadapters: %s (%s): remover must not be nil", name, burlerEngineLabel)
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
	if !filepath.IsAbs(deps.AnchorPath) {
		return nil, fmt.Errorf("shedadapters: %s (%s): anchor path %q is not absolute", name, burlerEngineLabel, deps.AnchorPath)
	}
	if now == nil {
		now = time.Now
	}
	return &BurlerProducer{
		name:    name,
		runner:  deps.Runner,
		remover: deps.Remover,
		models:  deps.Models,
		anchor:  deps.AnchorPath,
		profile: profile,
		opts:    opts,
		runDir:  runDir,
		now:     now,
	}, nil
}

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

// highestCompleteRound scans runDir's directory entries and returns the highest n for which
// roundComplete holds, returning 0 (and a nil error) when runDir is absent -- which is not an
// error -- and wrapping any other os.ReadDir failure.
// Its name-matching discipline is exact-shape via ParseRoundReviewName.
// The completion predicate is the pair, never the review alone, and why: a producer process killed
// in the phase-A-written/phase-B-pending window leaves a review with no fixer report beside it and
// no exit path ever ran to clean it up, so under a review-only predicate the next call would
// advance and hydrate a fixer report that burlerengine's requireExistingPaths rejects fail-loud,
// wedging the segment permanently, whereas under the pair predicate the orphan simply means the
// round is incomplete and is re-run.
// The segment's Bouncer does NOT run the same test, and saying so matters: it resolves its round
// through ResolveRound, which stats the review file alone. The asymmetry is safe today only because
// of where an orphan can appear -- a process killed in that window leaves current_producer naming
// this row, so this producer re-resolves the same round and archives the orphan before the Bouncer
// is ever routed to. It is not safe by construction, and a future change that lets the Bouncer be
// entered with an orphaned review present would have it judge a review no fixer round stands behind.
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

// Call runs one BurlerProducer round: resolve the round to run from disk, hand control back
// unspent when the highest complete round has not been judged yet, build a fresh per-round copy of
// the stored template profile, run at most two attempts (a second only on a died/timeout first
// attempt), and map the outcome onto shedengine's contract.
//
// Advance rule: the round to run is the highest COMPLETE round's successor only when that round
// also carries a parsing Bouncer verdict and ledger; otherwise this call returns Stuck without
// spawning anything, so the segment's Bouncer judges the round that is still owed a verdict rather
// than this row paying for a fresh review round over an unjudged one -- see the comment at the check
// itself for why a degraded judge makes that case ordinary rather than exotic.
//
// Archive rule: every return in which the round did not produce a usable review archives both
// round paths first, keyed on that fact rather than on whether the return is an error -- this
// covers a runner error (regardless of the Result.Outcome it carries), a
// second consecutive died/timeout, an unrecognized outcome, a gate-failed round, and a
// cancellation detected between attempts. Two carve-outs leave the round's files in place: the
// success return, and a cancellation detected after the round already completed and parsed --
// that return is an error, but its artifacts survive so the next call advances to the following
// round instead of re-running this one.
//
// Gate-failed exit: a done round whose Result.Gate is non-nil and failing maps to Stuck with an
// empty Path (the cause rides on Reason), archived exactly like every other non-success exit --
// the empty Path is what tells the segment's Bouncer there is no round artifact to judge, the
// same signal the deleted validate producers used for exactly this meaning (see the "the two
// producers' output pointers mean different things" decision). It consumes no
// attempt-1/attempt-2 retry: that retry is for OutcomeDied/OutcomeTimeout, infrastructure faults,
// while gate exhaustion is a determinate verdict the gate already re-prompted its whole budget over
// inside the session.
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

	// Completing a round is not what earns the next one: being judged is. The segment's Bouncer
	// routes to this row on EVERY Stuck it returns, and its degraded exits -- an unreadable rubric,
	// a judge spawn that died, a verdict that did not parse -- are Stuck too, so a single transient
	// judge fault arrives here indistinguishable from a BLOCKING verdict unless the verdict itself
	// is consulted. Advancing on that would cost a whole extra fixer round (a real LLM session) for
	// a review nobody has judged, and would break the ledger chain besides: round N+1's judge looks
	// for round N's ledger, finds none, and silently falls back to "(none)", resetting the
	// finding-identity carry-forward it uses to tell a recurring finding from a new one.
	//
	// So an unjudged highest round hands control straight back to the Bouncer instead, spawning
	// nothing, archiving nothing, and pointing at the review still waiting for a verdict. That
	// hand-back is the cheapest correct move rather than a re-run of round N, whose artifacts are
	// exactly what the Bouncer must judge. It cannot ping-pong forever: the Bouncer's next call is a
	// genuine judge retry, and each hand-back spends one unit of this row's own bounce budget, so a
	// judge that never recovers halts the run for a human rather than looping.
	if highest > 0 {
		if _, judged := recordedVerdict(p.runDir, highest); !judged {
			if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			logger.Warn("shedadapters: burler round producer reached with the highest complete round unjudged; handing back for judgment instead of running a fresh round", "producer", p.name, "engine", burlerEngineLabel, "round", highest)
			return shedengine.Stuck, shedengine.OutputPointer{Path: roundReviewPath(p.runDir, highest), Reason: fmt.Sprintf("round %d is complete but unjudged; handing back for judgment", highest)}, nil
		}
	}

	round := highest + 1

	reviewPath := roundReviewPath(p.runDir, round)
	fixerReportPath := roundFixerReportPath(p.runDir, round)
	readyMarkerPath, err := burlermarker.Path(p.anchor, p.anchor, reviewPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: ready marker path: %w", p.name, burlerEngineLabel, round, err)
	}

	focus := readRoundFocus(p.name, p.runDir, round)
	priorReviews, priorFixerReports, err := hydrationPaths(p.runDir, round)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): resolve hydration: %w", p.name, burlerEngineLabel, err)
	}

	// A fresh copy of the stored template, built per round:
	// every round must carry its own ReviewPath, FixerReportPath, ReadyMarkerPath, FocusDirective, PriorReviews, PriorFixerReports, and ClusterExclude values,
	// and a reused copy would leak the previous round's values into the next one.
	// The stored template itself is never mutated -- every slice field set below is a freshly allocated slice, never an in-place append onto p.profile's own backing array.
	profile := p.profile
	profile.ReviewPath = reviewPath
	profile.FocusDirective = focus.DirectivePath
	profile.FixerReportPath = fixerReportPath
	profile.ReadyMarkerPath = readyMarkerPath
	profile.PriorReviews = append(append([]string{}, p.profile.PriorReviews...), priorReviews...)
	profile.PriorFixerReports = append(append([]string{}, p.profile.PriorFixerReports...), priorFixerReports...)
	profile.ClusterExclude = nil
	if p.profile.ClusterFan != "" {
		profile.ClusterExclude = focus.ExcludeLenses
	} else if len(focus.ExcludeLenses) > 0 {
		// The Bouncer only asks for excludes when its own cluster_excludes key says this round
		// has a fan, so this branch fires only on a breach: a judge writing exclude_lenses
		// unprompted, or a recipe whose Bouncer and BurlerRound rows disagree. That is why it
		// stays a WARN, and it never becomes a validate hard error downstream -- the fan is
		// authoritative config, the focus file is an advisory, LLM-authored directive.
		logger.Warn("shedadapters: focus file names cluster excludes but this round's profile has no cluster fan; dropping them", "producer", p.name, "engine", burlerEngineLabel, "round", round, "lenses", focus.ExcludeLenses)
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

	// Probe before archive, exactly as SingleLLMProducer.Call does and for the identical reason:
	// archiving renames the very two files a live round is about to write.
	// Shuttle's Wait polls for bare existence at those paths, so archiving ahead of the probe would make a resumed round unable to ever classify done -- in precisely the case the probe exists to protect.
	if attachedOutcome, attachedPtr, attachedErr, handled := p.probeLiveRound(ctx, round, profile, archiveRound, failureExit); handled {
		return attachedOutcome, attachedPtr, attachedErr
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
		//
		// Routed through failureExit rather than returned bare, so it consults cancelErr first like
		// every other non-success exit in this function. An archive failure is not a success verdict,
		// and this package's shared cancellation rule admits no exception for it -- returning bare
		// here reported an infrastructure error for a run an operator had already cancelled.
		// It skips failureExit's own archive step by construction: archiveRound calls the very
		// helper that just failed, so retrying it could only fail again.
		if err := archiveStaleOutputs([]string{reviewPath, fixerReportPath}, p.now); err != nil {
			if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
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
		attemptOpts.Review, attemptOpts.Fix = p.models.Pick(round)
		// p.runDir's base is the segment's own run_subdir recipe value (webster, plan, discussion),
		// so this stem distinguishes Plan-Burler round 3 from Webster-Burler round 3 rather than
		// letting them collide in one friction directory.
		attemptOpts.NoteID = "burler-" + filepath.Base(p.runDir) + "-r" + strconv.Itoa(round)

		result, runErr := p.runner.Run(profile, attemptOpts)
		if errors.Is(runErr, burlerengine.ErrHalfNotStopped) {
			// Returned bare, without archiving or the retry, like the live-round probe's errors:
			// a half that could not be stopped may still be writing the round's files.
			if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: run: %w", p.name, burlerEngineLabel, round, attemptToken, runErr)
		}
		if runErr != nil {
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: run: %w", p.name, burlerEngineLabel, round, attemptToken, runErr))
		}

		switch result.Outcome {
		case shuttleengine.OutcomeDone:
			return p.doneExit(ctx, round, result, archiveRound)

		case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
			if attempt == 1 {
				logger.Warn("shedadapters: burler round attempt died or timed out, retrying", "producer", p.name, "engine", burlerEngineLabel, "round", round, "attempt", attemptToken, "outcome", result.Outcome, "halves", describeHalves(result))
				priorResult = result
				priorToken = attemptToken
				continue
			}
			if result.NotStarted {
				return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d: two consecutive died/timeout outcomes (attempt %s outcome %s %s; attempt %s outcome %s %s): %w", p.name, burlerEngineLabel, round, priorToken, priorResult.Outcome, describeHalves(priorResult), attemptToken, result.Outcome, describeHalves(result), shuttleengine.ErrNotStarted))
			}
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d: two consecutive died/timeout outcomes (attempt %s outcome %s %s; attempt %s outcome %s %s)", p.name, burlerEngineLabel, round, priorToken, priorResult.Outcome, describeHalves(priorResult), attemptToken, result.Outcome, describeHalves(result)))

		default:
			return failureExit(fmt.Errorf("shedadapters: %s (%s): round %d attempt %s: unrecognized burler outcome %q", p.name, burlerEngineLabel, round, attemptToken, result.Outcome))
		}
	}

	// Unreachable: every path through the loop above returns.
	return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): round %d: attempt loop exited without a verdict", p.name, burlerEngineLabel, round)
}

// doneExit maps a round that reached OutcomeDone onto Call's contract, for a spawned and a resumed round alike.
//
// A failed gate must not consume or trigger the attempt-1/attempt-2 retry: that retry
// exists for OutcomeDied/OutcomeTimeout, which are infrastructure, while gate
// exhaustion is a determinate verdict the gate already re-prompted its whole budget
// over inside the session, and a second full round on the same input would re-spend
// an LLM generation to reach the same answer. Archiving is what keeps the hand-back
// honest: a gate-failed round's review file must not be left for the Bouncer to judge
// -- a Go validator already proved it invalid.
//
// A genuine success verdict survives cancellation only up to the moment the round
// completed and parsed; a cancellation observed after that point still yields an
// error (internal/shedengine binds every implementation to surface cancellation as a
// non-nil error, never as Stuck), but the already-complete artifacts survive.
func (p *BurlerProducer) doneExit(ctx context.Context, round int, result burlerengine.Result, archiveRound func()) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if result.Gate != nil && !result.Gate.Passed {
		archiveRound()
		if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: burler round's gate did not pass", "producer", p.name, "engine", burlerEngineLabel, "round", round, "attempts", result.Gate.Attempts, "findingsPath", result.Gate.FindingsPath)
		return shedengine.Stuck, shedengine.OutputPointer{GateAttempts: gateAttemptsPointer(result.Gate), Reason: gateFailedReason(result.Gate)}, nil
	}
	if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Stuck, shedengine.OutputPointer{Path: roundReviewPath(p.runDir, round), GateAttempts: gateAttemptsPointer(result.Gate), BudgetExempt: p.roundBudgetExempt(round)}, nil
}

// describeHalves names both halves of a round for a log line or an error: each half's session and run directory,
// or the start error of a half that never started.
func describeHalves(result burlerengine.Result) string {
	return describeHalf("review", result.Review) + ", " + describeHalf("fix", result.Fix)
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

// roundBudgetExempt reports whether the Stuck that hands completed round N back to the Bouncer is exempt from the bounce budget:
// round N-1 carries a recorded continue decision whose cause is budget, which grants exactly this one more round.
// The exemption is tied to that one decision file, so each further round needs its own decision.
// A malformed decision file is warned about and grants no exemption.
func (p *BurlerProducer) roundBudgetExempt(round int) bool {
	if round < 2 {
		return false
	}
	decision, cause, _, exists, err := readCirclingDecision(p.runDir, round-1)
	if err != nil {
		logger.Warn("shedadapters: unreadable circling decision; the round's Stuck stays counted against the budget", "producer", p.name, "engine", burlerEngineLabel, "round", round-1, "error", err)
		return false
	}
	return exists && decision == CirclingContinue && cause == EscalationBudget
}

// probeLiveRound asks the runner what became of a previous session's two halves of this round and resumes the round when both are still live.
//
// It reports handled=false whenever the caller should proceed to its ordinary archive-then-spawn path:
// neither half is live, exactly one half is live and was stopped, or the resumed round had already died or timed out.
// Every other case reports handled=true along with the three values Call must return.
//
// With both halves live it calls Resume and maps the result through the same outcome switch as a spawned attempt, except that a died or timeout result falls through to a fresh attempt 1.
// The bounded retry then applies to that spawn from its own attempt 1, deliberately: the attached round was not this producer's attempt, and counting it would silently halve the retry budget of every resumed round.
// With exactly one half live that half's strand is removed through the remover and the round respawns, so a live fixer beside a finished review is stopped and re-run rather than attached;
// its target edits stay in the worktree and the next attempt's reviewer reviews them.
// With no half live, whatever the other halves' files hold, nothing is removed.
// The respawn's archive then renames the round's outputs and the engine removes the ready marker.
//
// A probe error, a failed removal and a Resume error wrapping burlerengine.ErrHalfNotStopped are returned bare rather than through failureExit:
// failureExit archives the round's two paths, and a half that may still be live is the one situation where archiving is most dangerous, since it may be mid-write on them.
// A failed removal wraps ErrHalfNotStopped and ends with the way forward, like the engine's own failed stop.
func (p *BurlerProducer) probeLiveRound(
	ctx context.Context,
	round int,
	profile burlerengine.Profile,
	archiveRound func(),
	failureExit func(error) (shedengine.Outcome, shedengine.OutputPointer, error),
) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	probeToken := strconv.Itoa(round)
	opts := p.opts
	opts.Round = probeToken
	opts.Review, opts.Fix = p.models.Pick(round)

	live, err := p.runner.ProbeRound(profile, opts)
	if err != nil {
		return p.liveRoundErrorExit(ctx, fmt.Errorf("shedadapters: %s (%s): round %d: probe the round's halves: %w", p.name, burlerEngineLabel, round, err))
	}

	reviewLive := live.Review.State == burlerengine.HalfLive
	fixLive := live.Fix.State == burlerengine.HalfLive
	switch {
	case reviewLive && fixLive:
		return p.resumeLiveRound(ctx, round, profile, opts, live, archiveRound, failureExit)
	case reviewLive:
		return p.stopLiveHalf(ctx, round, live.Review.Handle)
	case fixLive:
		return p.stopLiveHalf(ctx, round, live.Fix.Handle)
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// stopLiveHalf removes the strand of the one live half of a round, so the respawn that follows runs beside no live half.
// A removal that fails is the round's error, wrapping burlerengine.ErrHalfNotStopped and ending with the way forward.
func (p *BurlerProducer) stopLiveHalf(ctx context.Context, round int, half burlerengine.Handle) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	guid := half.StrandGUID()
	logger.Warn("shedadapters: stopping the one live half of a burler round before respawning it", "producer", p.name, "engine", burlerEngineLabel, "round", round, "strandGUID", guid)
	if err := p.remover.RemoveStrandIfLive(guid); err != nil {
		return p.liveRoundErrorExit(ctx, fmt.Errorf("shedadapters: %s (%s): round %d: %w: strand %s: %v; way forward: run \"lyx reed remove %s\", then re-step the row", p.name, burlerEngineLabel, round, burlerengine.ErrHalfNotStopped, guid, err, guid))
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// resumeLiveRound resumes a round whose halves are both live and maps its result onto Call's contract.
func (p *BurlerProducer) resumeLiveRound(
	ctx context.Context,
	round int,
	profile burlerengine.Profile,
	opts burlerengine.RunOpts,
	live burlerengine.LiveRound,
	archiveRound func(),
	failureExit func(error) (shedengine.Outcome, shedengine.OutputPointer, error),
) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	result, err := p.runner.Resume(profile, opts, live)
	if errors.Is(err, burlerengine.ErrHalfNotStopped) {
		return p.liveRoundErrorExit(ctx, fmt.Errorf("shedadapters: %s (%s): round %d: resume: %w", p.name, burlerEngineLabel, round, err))
	}
	if err != nil {
		outcome, ptr, exitErr := failureExit(fmt.Errorf("shedadapters: %s (%s): round %d: resume: %w", p.name, burlerEngineLabel, round, err))
		return outcome, ptr, exitErr, true
	}

	logger.Info("shedadapters: resumed a live burler round instead of respawning", "producer", p.name, "engine", burlerEngineLabel, "round", round, "halves", describeHalves(result))

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		outcome, ptr, exitErr := p.doneExit(ctx, round, result, archiveRound)
		return outcome, ptr, exitErr, true

	case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
		logger.Warn("shedadapters: resumed burler round had already died or timed out; respawning", "producer", p.name, "engine", burlerEngineLabel, "round", round, "outcome", result.Outcome, "halves", describeHalves(result))
		return "", shedengine.OutputPointer{}, nil, false

	default:
		outcome, ptr, exitErr := failureExit(fmt.Errorf("shedadapters: %s (%s): round %d resumed run reported unrecognized outcome %q", p.name, burlerEngineLabel, round, result.Outcome))
		return outcome, ptr, exitErr, true
	}
}

// liveRoundErrorExit returns err without archiving, unless the context was cancelled, whose error takes precedence.
func (p *BurlerProducer) liveRoundErrorExit(ctx context.Context, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if cerr := cancelErr(ctx, p.name, burlerEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr, true
	}
	return "", shedengine.OutputPointer{}, err, true
}
