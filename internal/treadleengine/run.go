// run.go implements Engine.Run, the round loop that drives one treadle block to a terminal Result.
// The block starts from a fresh or resumed run dir.
// Each round makes a RoundRunner attempt with bounded retry, then the convergence gate, then the stuck ladder.
// State is persisted after every round, so a crash or an operator pause resumes from where the block left off.

package treadleengine

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrBlockBusy marks Run's fail-fast refusal when another invocation already holds the run dir's run.lock.
// It is a sentinel, matched with errors.Is.
// The caller must treat this refusal differently from every other hard error.
// The refused invocation wrote nothing to disk; the lock holder owns the block's state.
// The caller must therefore skip its block-exit bookkeeping, such as a fabric sync, for it.
// The sentinel's own message has no engine prefix; errf adds the engine name when Run wraps it.
var ErrBlockBusy = errors.New("block is already running")

// runLockName is the exclusive-lease file name inside a block's scratch dir.
// Run holds this lease for its entire duration.
// state.json.lock, by contrast, is held by internal/state only for one read or write.
// Without it, two concurrent invocations on one dir pair would both classify resume or fresh from state.json.
// Both would then drive rounds into the same dir:
// colliding artifact paths, clobbered state.json appends, and two round-runner agents editing the worktree at once.
const runLockName = "run.lock"

// roundOutcome holds what a round's retry loop produced once the runner reached a done outcome.
type roundOutcome struct {
	Attempts        int
	Verdict         Verdict
	BlockingCount   int
	ReviewPath      string
	FixerReportPath string
	SessionID       string
	Paths           roundArtifactPaths
}

// Run drives one treadle block's round loop for profile p, reading and persisting state under runDir.
// It validates p with structural checks only, and ensures runDir and the block's scratch dir exist.
// It resolves the resume point from the state file against p.ProfileHash, which the caller computed.
// Each round checks for a pause at its boundary only.
// It then makes a round-runner attempt with bounded non-done retry.
// It runs the convergence gate, and on a non-converged round it applies the stuck ladder.
// Every returned error is prefixed via e.errf.
// The returned Result mirrors the persisted state's rounds as RoundSummary values.
func (e *Engine) Run(p Profile, runDir string) (result Result, err error) {
	// scratchDir is resolved first.
	// The deferred terminal-clear closure, the run lock, and the entry-time clearPauseFlag call all need it.
	// An empty e.scratchDir falls back to runDir, so both MkdirAll calls name one directory.
	scratchDir := e.scratchDir
	if scratchDir == "" {
		scratchDir = runDir
	}

	// A pause requested during the final round can see a terminal, non-PAUSED outcome as the round settles.
	// The flag is checked only at the next round boundary, which never comes.
	// A stale flag must not linger in the scratch dir, where a finished block would carry it into the fabric commit.
	// Clearing it here covers every terminal return site at once.
	defer func() {
		if err == nil && result.Outcome != OutcomePaused {
			_ = clearPauseFlag(e.name, scratchDir)
		}
	}()

	if err := p.validate(e.name); err != nil {
		return Result{}, err
	}
	// Identity is p.ProfileHash exactly as the caller supplied it; treadle never computes it.
	// A caller's default-resolution change must not alter the identity of a block the operator never touched.
	// Such a change would invalidate that block's resume.
	hash := p.ProfileHash

	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Result{}, e.errf("create run dir %q: %w", runDir, err)
	}
	// state.json and every round artifact stay in runDir;
	// only the run lock, state.json.lock, and the pause flag live in scratchDir.
	// The run lock is acquired inside scratchDir, so it must exist first.
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return Result{}, e.errf("create scratch dir %q: %w", scratchDir, err)
	}

	// The lock is held for the whole call.
	// A second concurrent invocation on the same dir pair fails fast rather than interleaving rounds with this one.
	// The OS releases the lock on process exit or crash, even when Release never runs,
	// so a killed process cannot block a later resume.
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, runLockName))
	if err != nil {
		return Result{}, e.errf("acquire run lock for %q: %w", scratchDir, err)
	}
	if !locked {
		// Wrapped with %w so the caller can match ErrBlockBusy with errors.Is and skip its block-exit bookkeeping.
		return Result{}, e.errf("%w: %q (run.lock held); wait for it to finish or use a different --run-id", ErrBlockBusy, scratchDir)
	}
	defer runLock.Release()

	// A resumed block must not re-pause at once on a flag left by the run that requested the pause it resumes from.
	if err := clearPauseFlag(e.name, scratchDir); err != nil {
		return Result{}, err
	}

	st, resume, err := loadOrInitState(e.name, runDir, scratchDir, hash, p.RoundCaps)
	if err != nil {
		return Result{}, err
	}

	// Nil seams default here, once, at Run's entry.
	// The constructor stores Options' fields verbatim, nils included, so this file alone owns the fallback.
	pause := e.pauseRequested
	if pause == nil {
		pause = func() bool { return false }
	}
	runCommand := e.runCommand
	if runCommand == nil {
		runCommand = execGateCommand
	}

	// The ladder is the one stamped into state.json at block creation.
	// A fresh block stamps p's just-resolved RoundCaps, so the two agree there.
	// A resumed block reapplies the ladder it started with, even if the caller's default has since changed.
	// The identity hash deliberately does not cover the ladder.
	caps := st.RoundCaps
	if len(caps) == 0 {
		return Result{}, e.errf("state.json in %q records no round-caps ladder; the state file is corrupt", runDir)
	}
	hardCap := caps[len(caps)-1]

	for round := resume.NextRound; ; round++ {
		// A resume can land one round PAST the hard cap: the hard-cap round already ran and its record is persisted,
		// but a hard error interrupted its stuck classification before the block was marked terminal.
		// The interrupting error is a gate command that could not start.
		// Finalize it from the persisted state as STUCK with the hard-cap reason.
		// The last recorded round is the hard-cap round and did not converge.
		// Hard-cap is therefore the correct terminal reason.
		// Stopping here keeps the ladder's termination guarantee across a resume:
		// without it, round == hardCap never matches again, and the loop runs unbounded rounds past the ladder.
		if round > hardCap {
			st.Outcome = string(OutcomeStuck)
			st.StuckReason = string(StuckHardCap)
			if err := saveState(runDir, scratchDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomeStuck, StuckHardCap), nil
		}

		// Pause is checked only here, at the round boundary, never mid-round,
		// so a paused block always resumes at a clean round start.
		if pause() {
			if err := saveState(runDir, scratchDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomePaused, ""), nil
		}

		// Stale artifacts are moved aside once per round, for attempt 1, before pre-round targeting runs.
		// An interrupted prior attempt can leave a seed file at the path that targeting writes.
		// spec.validate rejects a pre-existing output file.
		// A later retry attempt's stale artifacts are handled inside runRound, per attempt.
		if err := moveStaleArtifacts(e.name, runDir, round, 1); err != nil {
			// The wrapped error carries only the file's name and path, so the log line adds the round and attempt.
			logger.Warn(e.name+": moving stale artifacts aside before round failed", "name", e.name, "round", round, "attempt", 1, "err", err)
			return Result{}, err
		}

		priorReviews, priorFixerReports := collectPriorHydration(st.Rounds)
		seedPath := e.runPreRoundTargeting(runDir, round, p, st.Rounds)

		outcome, err := e.runRound(runDir, round, p, priorReviews, priorFixerReports, seedPath)
		if err != nil {
			return Result{}, err
		}

		record := roundRecord{
			Round:           round,
			Attempts:        outcome.Attempts,
			ShuttleOutcome:  string(shuttleengine.OutcomeDone),
			Verdict:         string(outcome.Verdict),
			BlockingCount:   outcome.BlockingCount,
			ReviewPath:      outcome.ReviewPath,
			FixerReportPath: outcome.FixerReportPath,
			SeedPath:        seedPath,
			SessionID:       outcome.SessionID,
		}

		// The gate command runs after this round's fix phase, in command and both gate modes only.
		// llm-verdict never runs a command.
		// Both gate mode still requires the runner verdict too, so it does not ignore it.
		// Its cwd is always p.GateDir, the caller-supplied absolute path.
		// It is never the run dir, because the command exercises the repo's own build and test surface.
		if p.Gate.Mode == GateCommand || p.Gate.Mode == GateBoth {
			output, exitZero, err := runCommand(p.Gate.Command, p.GateDir, p.Gate.Timeout)
			if err != nil {
				// A gate command that cannot start is a hard error, but the round it follows completed.
				// Its record is persisted first, so a resume continues at the next round instead of re-buying it.
				// The record carries no gate result, so GatePassed is nil.
				// The loop reads nil as not converged, the safe direction.
				st.Rounds = append(st.Rounds, record)
				if saveErr := saveState(runDir, scratchDir, st); saveErr != nil {
					// Returning saveErr hides err, the real reason this round failed.
					// Execution never reaches the e.errf return below, so the Warn is the only place err surfaces.
					logger.Warn(e.name+": state persist failed after gate command error, original failure lost to caller", "runDir", runDir, "round", round, "err", err, "saveErr", saveErr)
					return Result{}, saveErr
				}
				return Result{}, e.errf("round %d gate command: %w", round, err)
			}
			// The output is written on pass and on fail, since the record is cheap.
			// Only a failing gate file is fed forward as hydration.
			if err := writeGateOutput(e.name, outcome.Paths.Gate, p.Gate.Command, output, exitZero); err != nil {
				return Result{}, err
			}
			record.GatePath = outcome.Paths.Gate
			record.GatePassed = &exitZero
		}

		if converged(p.Gate.Mode, outcome.Verdict, record.GatePassed) {
			st.Rounds = append(st.Rounds, record)
			st.Outcome = string(OutcomeApproved)
			if err := saveState(runDir, scratchDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomeApproved, ""), nil
		}

		// The stuck ladder runs only on a non-converged round.
		// Every trigger below is based on the runner verdict.
		// A round with VerdictApproved and a failing command (command or both gate mode) skips every rung.
		// It loops, bounded by the hard cap below and fed forward through the gate file.
		if round == hardCap {
			st.Rounds = append(st.Rounds, record)
			st.Outcome = string(OutcomeStuck)
			st.StuckReason = string(StuckHardCap)
			if err := saveState(runDir, scratchDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomeStuck, StuckHardCap), nil
		}

		if outcome.Verdict == VerdictBlocking {
			// The circling check never runs on the round right after an APPROVED round.
			// Command and both gate modes can reach that case, since an APPROVED round with a failing command does not converge.
			// The immediately prior review has zero blocking findings, so fresh findings here are new work.
			// They are not evidence of circling: a false CIRCLING verdict would be a permanent, wrong STUCK.
			// The milestone gate is not exempted: a rung asks about the whole trajectory, not recurrence against the prior round.
			prevRoundApproved := len(st.Rounds) > 0 &&
				st.Rounds[len(st.Rounds)-1].Verdict == string(VerdictApproved)

			// The read-set is computed inside each judge branch, not before the switch.
			// judgeReadSet reads and parses recorded handoff files on disk,
			// so a round that runs no judge must not pay that I/O or emit its corrupt-handoff Warns.
			// That covers round 1 and the round right after an APPROVED round.
			// The judge reasons over the review history, including this round's fresh review.
			// It reads reviews only: failed gate-command output files are excluded.
			// The judge compares blocking findings that recur across review files (the verdict-judge contract in doc.go),
			// and a gate transcript has no findings to compare.
			// judgeReadSet bounds the list to the latest valid handoff and the reviews it has not absorbed.
			switch {
			case isMilestoneRung(caps, round):
				judgeReviews, prevHandoffPath := judgeReadSet(e.name, st.Rounds, outcome.ReviewPath)
				// The milestone gate replaces the circling check for this round.
				// A rung round issues exactly one judge call.
				jv, _, judgeOK := runMilestone(e.shuttle, e.name, judgeInputs{
					Round:               round,
					HardCap:             hardCap,
					PriorReviews:        judgeReviews,
					VerdictPath:         outcome.Paths.Judge,
					PreviousHandoffPath: prevHandoffPath,
					HandoffPath:         outcome.Paths.Handoff,
					Choice:              p.Judge,
					StencilsDir:         e.stencilsDir,
					ParentName:          e.parentName,
				})
				// Only a real verdict is recorded.
				// A fail-safe fallback (judgeOK false) leaves the judge fields empty,
				// so an operator reading state.json can tell a genuine CONTINUE from a judge failure that never answered.
				// The Warn logged inside the call is the only trace of that failure.
				// A failed judge call records no handoff either.
				if judgeOK {
					record.JudgePath = outcome.Paths.Judge
					record.JudgeVerdict = string(jv)
					record.HandoffPath = recordHandoffIfValid(e.name, outcome.Paths.Handoff, round, "milestone judge")
				}
				if jv == JudgeStop {
					st.Rounds = append(st.Rounds, record)
					st.Outcome = string(OutcomeStuck)
					st.StuckReason = string(StuckMilestoneStop)
					if err := saveState(runDir, scratchDir, st); err != nil {
						return Result{}, err
					}
					return resultFromState(st, OutcomeStuck, StuckMilestoneStop), nil
				}
				// JudgeContinue / JudgeUncertain: fall through and loop.
			case round >= 2 && !prevRoundApproved:
				judgeReviews, prevHandoffPath := judgeReadSet(e.name, st.Rounds, outcome.ReviewPath)
				jv, _, judgeOK := runCircling(e.shuttle, e.name, judgeInputs{
					Round:               round,
					PriorReviews:        judgeReviews,
					VerdictPath:         outcome.Paths.Judge,
					PreviousHandoffPath: prevHandoffPath,
					HandoffPath:         outcome.Paths.Handoff,
					Choice:              p.Judge,
					StencilsDir:         e.stencilsDir,
					ParentName:          e.parentName,
				})
				// The fail-safe fallback is never recorded, and a failed judge call records no handoff.
				if judgeOK {
					record.JudgePath = outcome.Paths.Judge
					record.JudgeVerdict = string(jv)
					record.HandoffPath = recordHandoffIfValid(e.name, outcome.Paths.Handoff, round, "circling judge")
				}
				if jv == JudgeCircling {
					st.Rounds = append(st.Rounds, record)
					st.Outcome = string(OutcomeStuck)
					st.StuckReason = string(StuckCircling)
					if err := saveState(runDir, scratchDir, st); err != nil {
						return Result{}, err
					}
					return resultFromState(st, OutcomeStuck, StuckCircling), nil
				}
			}
			// Round 1 with a blocking verdict runs no judge, since there is no prior round to compare against.
			// Neither does a blocking round right after an APPROVED round.
		}
		// A VerdictApproved round that does not converge runs no judge and continues to the next round.

		st.Rounds = append(st.Rounds, record)
		if err := saveState(runDir, scratchDir, st); err != nil {
			return Result{}, err
		}
	}
}

// runPreRoundTargeting runs this round's pre-round targeting call when p.PreRoundTargeting is set.
// It returns the seed path that every attempt of round threads into its AttemptInput.SeedPath.
// It returns "" when targeting is off or when the targeting call fails.
// It also returns "" when no valid handoff exists yet to target from:
// round 1, or a round where every recorded handoff failed to read or parse.
// The seed path is resolved once per round, at attempt 1's token via artifactPaths.
// It is never recomputed per attempt, so a same-round retry reuses the identical path.
// This matches the round-scoped priorReviews and priorFixerReports hydration.
// Callers must clear any leftover seed file at that path (moveStaleArtifacts) before calling this,
// so a re-run's targeting call never trips the pre-existing-output-file rejection.
func (e *Engine) runPreRoundTargeting(runDir string, round int, p Profile, rounds []roundRecord) string {
	if !p.PreRoundTargeting {
		return ""
	}
	handoffPath, _, ok := latestValidHandoff(e.name, rounds)
	if !ok {
		// Nothing to target from yet: round 1, or no handoff has ever survived a valid parse.
		// This is not a failure, so nothing is logged.
		// It mirrors the round-1-runs-no-judge posture in the main loop below.
		return ""
	}
	seedPath := artifactPaths(runDir, round, 1).Seed
	if _, ok := runTargeting(e.stencilsDir, e.parentName, e.shuttle, e.name, round, handoffPath, seedPath, p.Judge); !ok {
		return ""
	}
	return seedPath
}

// runRound drives one round through up to two attempts:
// a fresh attempt, then one deterministic retry after a died or timeout outcome.
// priorReviews and priorFixerReports are the hydration accumulated from every completed round.
// Both attempts of a round reuse the same hydration and seedPath, since a retry completes no new round.
// seedPath is the pre-round-targeting seed, or "" when targeting produced none.
// A second consecutive non-done attempt is an infrastructure error, not `OutcomeStuck`:
// the machinery failed twice, which says nothing about whether the artifact will converge.
func (e *Engine) runRound(runDir string, round int, p Profile, priorReviews, priorFixerReports []string, seedPath string) (roundOutcome, error) {
	for attempt := 1; attempt <= 2; attempt++ {
		// The caller clears attempt 1's stale artifacts, seed file included, before pre-round targeting runs.
		// Clearing them again here would move aside the seed file that targeting just wrote.
		// A retry's stale artifacts from an earlier interrupted resume still need clearing here.
		if attempt > 1 {
			if err := moveStaleArtifacts(e.name, runDir, round, attempt); err != nil {
				// The wrapped error names the file but not the round or attempt; the log line below carries them.
				logger.Warn(e.name+": moving stale artifacts aside before retry attempt failed", "name", e.name, "round", round, "attempt", attempt, "err", err)
				return roundOutcome{}, err
			}
		}
		paths := artifactPaths(runDir, round, attempt)

		result, err := e.runner.RunAttempt(AttemptInput{
			RunDir:            runDir,
			Round:             round,
			Attempt:           attempt,
			RoundToken:        roundToken(round, attempt),
			ReviewPath:        paths.Review,
			FixerReportPath:   paths.FixerReport,
			SeedPath:          seedPath,
			PriorReviews:      priorReviews,
			PriorFixerReports: priorFixerReports,
			Choice:            p.Attempt,
			Timeout:           p.Timeout,
		})
		if err != nil {
			return roundOutcome{}, e.errf("round %d attempt run: %w", round, err)
		}

		if result.Outcome == shuttleengine.OutcomeDone {
			return roundOutcome{
				Attempts:        attempt,
				Verdict:         result.Verdict,
				BlockingCount:   result.BlockingCount,
				ReviewPath:      result.ReviewPath,
				FixerReportPath: result.FixerReportPath,
				SessionID:       result.SessionID,
				Paths:           paths,
			}, nil
		}

		// A died or timed-out attempt gets one cheap deterministic retry, since these are nearly always environmental.
		if attempt == 2 {
			return roundOutcome{}, e.errf("round %d failed twice (%s); session %s, kept run dir %s", round, result.Outcome, result.SessionID, result.RunDir)
		}
		// Log the retry so an operator sees it happening, not only its eventual outcome.
		// A second failure returns an error with full context instead.
		logger.Warn(e.name+": round attempt died or timed out, retrying", "round", round, "outcome", result.Outcome, "sessionID", result.SessionID)
	}
	// Unreachable: attempt 2 always returns from inside the loop.
	return roundOutcome{}, e.errf("round %d exhausted its bounded retries without a terminal outcome", round)
}

// collectPriorHydration builds the priorReviews and priorFixerReports lists a fresh round's attempt is seeded with.
// It draws them from every completed round in rounds.
// Each round's ReviewPath and FixerReportPath are always included.
// A round's GatePath joins priorReviews only when that round's GatePassed is false:
// passing-gate output is never fed forward, since a clean command run has nothing for the next round to learn from.
func collectPriorHydration(rounds []roundRecord) (priorReviews, priorFixerReports []string) {
	for _, r := range rounds {
		priorReviews = append(priorReviews, r.ReviewPath)
		if r.GatePassed != nil && !*r.GatePassed {
			priorReviews = append(priorReviews, r.GatePath)
		}
		priorFixerReports = append(priorFixerReports, r.FixerReportPath)
	}
	return priorReviews, priorFixerReports
}

// collectJudgeReviews builds the review-file list a progress-judge call reads:
// every completed round's ReviewPath in order, plus the current round's fresh review.
// Gate-command output is left out:
// the judge compares blocking findings across reviews, and a gate transcript is not a review.
func collectJudgeReviews(rounds []roundRecord, currentReviewPath string) []string {
	reviews := make([]string, 0, len(rounds)+1)
	for _, r := range rounds {
		reviews = append(reviews, r.ReviewPath)
	}
	return append(reviews, currentReviewPath)
}

// recordHandoffIfValid returns path when the handoff file at path is readable and parses, and "" otherwise.
// On failure it logs a warning, prefixed with name, that names label (such as "milestone judge") and round.
// It never fails the run: the verdict the handoff came with stands either way.
func recordHandoffIfValid(name string, path string, round int, label string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		logger.Warn(name+": "+label+" handoff file unreadable, leaving no recorded handoff", "round", round, "cause", err)
		return ""
	}
	if _, err := ParseHandoff(content); err != nil {
		logger.Warn(name+": "+label+" handoff file unparseable, leaving no recorded handoff", "round", round, "cause", err)
		return ""
	}
	return path
}

// isMilestoneRung reports whether round is one of caps' milestone rungs.
// Every entry but the last is a milestone rung; the last is the hard cap and is not judge-gated.
func isMilestoneRung(caps []int, round int) bool {
	for _, c := range caps[:len(caps)-1] {
		if c == round {
			return true
		}
	}
	return false
}

// resultFromState builds the Result a run returns from st, with each persisted round as a RoundSummary.
func resultFromState(st runState, outcome Outcome, stuckReason StuckReason) Result {
	rounds := make([]RoundSummary, 0, len(st.Rounds))
	for _, r := range st.Rounds {
		rounds = append(rounds, RoundSummary{
			Round:           r.Round,
			Attempts:        r.Attempts,
			Verdict:         Verdict(r.Verdict),
			BlockingCount:   r.BlockingCount,
			ReviewPath:      r.ReviewPath,
			FixerReportPath: r.FixerReportPath,
			JudgePath:       r.JudgePath,
			GatePath:        r.GatePath,
			JudgeVerdict:    r.JudgeVerdict,
			GatePassed:      r.GatePassed,
		})
	}
	return Result{
		Outcome:     outcome,
		StuckReason: stuckReason,
		RoundsRun:   len(st.Rounds),
		Rounds:      rounds,
	}
}
