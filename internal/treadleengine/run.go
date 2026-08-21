// run.go implements Engine.Run, the deterministic round loop that drives one treadle block from a
// fresh or resumed run dir to a terminal Result: it validates the profile, resolves the block's
// resume point, then loops one round at a time through a RoundRunner attempt (with its bounded
// retry), the pluggable convergence gate, and the milestone-laddered stuck ladder, persisting state
// after every round so a crash or an operator pause can resume from exactly where the block left
// off.

package treadleengine

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrBlockBusy marks Run's fail-fast refusal when another invocation already holds the run dir's
// run.lock.
// It is a sentinel (matched via errors.Is) because the caller must treat this refusal differently
// from every other hard error: the losing invocation touched NOTHING on disk — the winner is
// mid-round and owns the block's state — so a loop owner must not run its block-exit bookkeeping
// (e.g. its own fabric sync) for it.
// The sentinel's own message is deliberately un-prefixed — the calling engine's name is applied at
// wrap time below, by errf, so the composed text still reads "<name>: block is already running:
// ..."
// exactly like the original loop's own literal message did before the extraction (see the
// name-parameterized-diagnostics shared decision).
var ErrBlockBusy = errors.New("block is already running")

// runLockName is the exclusive-lease file name inside a block's scratch dir,
// held for the ENTIRE duration of one Engine.Run call — distinct from
// state.json.lock, which internal/state only holds for the instant of one
// read or write. Without this, two concurrent invocations against the same
// run/scratch dir pair would each classify resume/fresh from state.json and
// then both drive rounds into the same dir: colliding artifact paths,
// clobbered state.json appends, and two round-runner agents editing the
// worktree at once.
const runLockName = "run.lock"

// roundOutcome captures what a round's retry loop produced when the runner
// reached a done outcome.
type roundOutcome struct {
	Attempts        int
	Verdict         Verdict
	BlockingCount   int
	ReviewPath      string
	FixerReportPath string
	TriagePath      string
	SessionID       string
	Paths           roundArtifactPaths
}

// Run drives one treadle block's round loop for Profile p, reading and persisting state at runDir.
// It validates p (structural checks only) via p.validate(e.name), ensures runDir and the block's
// scratch dir exist, resolves the block's resume point (loadOrInitState against p.ProfileHash,
// which the caller already computed), then loops one round at a time: a pause check at the round
// boundary only, a round-runner attempt with its bounded non-done retry, the pluggable convergence
// gate, and — on a non-converged round — the milestone-laddered stuck ladder.
// Every returned error is prefixed via e.errf;
// the returned Result mirrors the persisted state's rounds as RoundSummary values.
func (e *Engine) Run(p Profile, runDir string) (result Result, err error) {
	// Resolved first, before anything below reads it: the deferred
	// terminal-clear closure, the runDir MkdirAll, the run-lock acquisition,
	// and the entry-time clearPauseFlag call all need this value, and all
	// four precede the "Seam defaulting happens here, once" block further
	// down that defaults pause/runCommand. An empty e.scratchDir defaults to
	// runDir, the back-compat case where the two MkdirAll calls below name
	// the same directory (a harmless no-op on the second).
	scratchDir := e.scratchDir
	if scratchDir == "" {
		scratchDir = runDir
	}

	// A pause requested while the final round was still in flight can
	// observe a terminal, non-PAUSED outcome once that round settles on its
	// own (the pause flag is checked only at the NEXT round boundary, which
	// never arrives). The stale flag must not linger in the scratch dir (and
	// get fabric-committed alongside a finished block, in the back-compat
	// case where scratch and run coincide) once the block is done judging —
	// clearing it centrally here, once, covers every terminal return site
	// without duplicating the call at each one.
	defer func() {
		if err == nil && result.Outcome != OutcomePaused {
			_ = clearPauseFlag(e.name, scratchDir)
		}
	}()

	if err := p.validate(e.name); err != nil {
		return Result{}, err
	}
	// Identity is p.ProfileHash exactly as the caller supplied it — treadle
	// never computes this itself (see the treadle-owns-no-config shared
	// decision); a caller's own default-resolution change must never
	// silently change, or invalidate the resume of, a block whose profile
	// content the operator never touched.
	hash := p.ProfileHash

	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Result{}, e.errf("create run dir %q: %w", runDir, err)
	}
	// state.json and every round artifact still live in runDir; only the
	// run lock, state.json.lock, and the pause flag move to scratchDir — see
	// the told-never-derived-scratch-dir shared decision. The run lock below
	// is acquired inside scratchDir, so it must exist first.
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return Result{}, e.errf("create scratch dir %q: %w", scratchDir, err)
	}

	// Held for this entire call: a second concurrent invocation (or a
	// re-entrant caller) against the SAME run/scratch dir pair must fail
	// fast rather than silently interleave rounds with this one. Released
	// by the OS on process exit/crash even if this call never reaches
	// Release, so a killed process never bricks the run dir for a later
	// resume.
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, runLockName))
	if err != nil {
		return Result{}, e.errf("acquire run lock for %q: %w", scratchDir, err)
	}
	if !locked {
		// Wrapped with %w so the caller can errors.Is-match ErrBlockBusy and
		// skip its block-exit bookkeeping — see the sentinel's doc.
		return Result{}, e.errf("%w: %q (run.lock held); wait for it to finish or use a different --run-id", ErrBlockBusy, scratchDir)
	}
	defer runLock.Release()

	// A resumed block must never instantly re-pause on a flag left over
	// from the run that requested the pause it is now resuming from.
	if err := clearPauseFlag(e.name, scratchDir); err != nil {
		return Result{}, err
	}

	st, resume, err := loadOrInitState(e.name, runDir, scratchDir, hash, p.RoundCaps)
	if err != nil {
		return Result{}, err
	}

	// Seam defaulting happens here, once, at Run's entry: New (engine.go)
	// stores Options' fields verbatim (nils included) precisely so this
	// file — and only this file — owns the fallback behavior.
	pause := e.pauseRequested
	if pause == nil {
		pause = func() bool { return false }
	}
	runCommand := e.runCommand
	if runCommand == nil {
		runCommand = execGateCommand
	}

	// The ladder that governs this block is the one STAMPED into state.json
	// at block creation (a fresh block stamps p's just-resolved RoundCaps, so
	// the two agree there) — a resumed block re-applies the ladder it
	// actually started with even if the caller's own default changed in
	// between, which the identity hash above deliberately does not cover.
	caps := st.RoundCaps
	if len(caps) == 0 {
		return Result{}, e.errf("state.json in %q records no round-caps ladder; the state file is corrupt", runDir)
	}
	hardCap := caps[len(caps)-1]

	for round := resume.NextRound; ; round++ {
		// A resume can land one PAST the hard cap: the hard-cap round already
		// ran and its record is persisted, but a hard error (a could-not-start
		// gate command — see the persist-then-error path below) interrupted its
		// stuck classification before the block was marked terminal. Finalize it
		// as STUCK/hard-cap from the already-persisted state rather than spawning
		// rounds beyond the ladder: the last recorded round IS the hard-cap round
		// and it did not converge, so hard-cap is the correct terminal reason,
		// and stopping here preserves the ladder's guaranteed-termination
		// invariant across a resume (without this, round == hardCap never matches
		// again and the loop runs unbounded rounds past the ladder).
		if round > hardCap {
			st.Outcome = string(OutcomeStuck)
			st.StuckReason = string(StuckHardCap)
			if err := saveState(runDir, scratchDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomeStuck, StuckHardCap), nil
		}

		// Pause is checked ONLY here, at the round boundary — never
		// mid-round — so a paused block always resumes at a clean round
		// start rather than an in-progress one.
		if pause() {
			if err := saveState(runDir, scratchDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomePaused, ""), nil
		}

		// Cleared here, once per round, at attempt 1's token — BEFORE
		// pre-round targeting runs below, so a leftover seed file from an
		// interrupted prior attempt at this same round is moved aside before
		// targeting tries to write a fresh one at the same path (spec.validate
		// rejects a pre-existing output file). A later retry attempt's own
		// stale artifacts are still handled inside runRound, per attempt.
		if err := moveStaleArtifacts(e.name, runDir, round, 1); err != nil {
			// moveStaleIfExists already wraps with name/path context only (no
			// round/attempt) — a reader of the bare wrapped error knows which
			// file failed to move but not which round was in progress.
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
			TriagePath:      outcome.TriagePath,
			SeedPath:        seedPath,
			SessionID:       outcome.SessionID,
		}

		// The gate command runs after this round's fix phase, in the command
		// and both gate modes only (llm-verdict never runs a command; both
		// still requires the runner verdict too, so it does not "ignore" it);
		// its cwd is always p.GateDir (the caller-supplied absolute path —
		// see Profile.GateDir), never the run dir, since the command
		// exercises the repo's own build/test surface.
		if p.Gate.Mode == GateCommand || p.Gate.Mode == GateBoth {
			output, exitZero, err := runCommand(p.Gate.Command, p.GateDir, p.Gate.Timeout)
			if err != nil {
				// A could-not-start gate failure is a hard error, but the
				// round it follows COMPLETED — persist its record first so a
				// resume continues at the next round instead of re-buying
				// this one. The record carries no gate result (nil
				// GatePassed), which the loop reads as non-converged: the
				// safe direction.
				st.Rounds = append(st.Rounds, record)
				if saveErr := saveState(runDir, scratchDir, st); saveErr != nil {
					// saveErr is the value actually returned below, but err — the
					// real reason this round failed — is what gets lost: execution
					// never reaches the e.errf return two lines down, so without
					// this line the gate-command failure that triggered this whole
					// branch would never surface anywhere, masked entirely by the
					// unrelated persistence failure.
					logger.Warn(e.name+": state persist failed after gate command error, original failure lost to caller", "runDir", runDir, "round", round, "err", err, "saveErr", saveErr)
					return Result{}, saveErr
				}
				return Result{}, e.errf("round %d gate command: %w", round, err)
			}
			// Written on pass AND fail — the record is cheap — even though
			// only a failing gate file is ever fed forward as hydration.
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

		// The stuck ladder is reached only on a non-converged round. Every
		// trigger below is runner-verdict-based: a round with
		// VerdictApproved but a failing command (command/both gate modes)
		// skips every rung and simply loops, bounded by the hard cap below
		// and fed forward via the gate file.
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
			// The circling check never runs on the round immediately after an
			// APPROVED round (reachable in command/both gate modes, where an
			// APPROVED round with a failing command does not converge): the
			// immediately-prior review has zero blocking findings, so fresh
			// findings here are new work surfacing, not evidence of circling —
			// and a false CIRCLING verdict is a permanent, wrong STUCK. The
			// milestone gate is deliberately NOT exempted: a rung asks about
			// the whole trajectory, not recurrence against the prior round.
			prevRoundApproved := len(st.Rounds) > 0 &&
				st.Rounds[len(st.Rounds)-1].Verdict == string(VerdictApproved)

			// The read-set is computed INSIDE each judge branch, not before
			// the switch: judgeReadSet walks recorded handoff files on disk
			// (latestValidHandoff reads and parses them), and a judge-skipped
			// blocking round — round 1, or the round right after an APPROVED
			// round — must not pay that I/O or emit its corrupt-handoff Warns
			// for a judge call that never happens. The judge reasons over the
			// REVIEW history including this round's own fresh review — and
			// only reviews: unlike the round-attempt hydration in
			// priorReviews, failed gate-command output files are deliberately
			// excluded, since the judge's material is blocking findings
			// recurring across review files (doc.go's verdict-judge
			// contract), and a gate transcript has no findings to compare.
			// judgeReadSet bounds this list to {latest valid handoff +
			// reviews it has not absorbed} instead of every prior review —
			// see handoff.go.
			switch {
			case isMilestoneRung(caps, round):
				judgeReviews, prevHandoffPath := judgeReadSet(e.name, st.Rounds, outcome.ReviewPath)
				// The milestone gate REPLACES the circling check for this
				// round — a rung round issues exactly one judge call.
				jv, _, judgeOK := runMilestone(e.shuttle, e.name, judgeInputs{
					Round:               round,
					HardCap:             hardCap,
					PriorReviews:        judgeReviews,
					VerdictPath:         outcome.Paths.Judge,
					PreviousHandoffPath: prevHandoffPath,
					HandoffPath:         outcome.Paths.Handoff,
					Model:               p.JudgeModel,
					Effort:              p.JudgeEffort,
					StencilsDir:         e.stencilsDir,
				})
				// Only a REAL verdict is recorded — a fail-safe fallback
				// (judgeOK false) leaves the record's judge fields empty, so
				// an operator reading state.json can tell a genuine CONTINUE
				// apart from a judge infrastructure failure that never
				// actually answered (the Warn logged inside the call above
				// is the only trace of the failure). A judge call whose
				// verdict failed records no handoff either — recordHandoff
				// is only ever consulted inside this same judgeOK guard.
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
					Model:               p.JudgeModel,
					Effort:              p.JudgeEffort,
					StencilsDir:         e.stencilsDir,
				})
				// See the milestone-rung branch above: only a REAL verdict
				// is recorded, never the fail-safe fallback, and a failed
				// verdict records no handoff either.
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
			// round 1 with a blocking verdict runs no judge (there is no
			// prior round to compare it against yet), and neither does a
			// blocking round immediately after an APPROVED round (see
			// prevRoundApproved above).
		}
		// A VerdictApproved non-converged round (command mode only) runs no
		// judge at all and simply continues to the next round.

		st.Rounds = append(st.Rounds, record)
		if err := saveState(runDir, scratchDir, st); err != nil {
			return Result{}, err
		}
	}
}

// runPreRoundTargeting resolves and, when p.PreRoundTargeting is set, runs
// this round's pre-round targeting call, returning the seed path to thread
// into every attempt's AttemptInput.SeedPath for round, or "" when
// targeting is off, no valid handoff exists yet to target from (round 1, or
// every recorded handoff failed to read/parse — see latestValidHandoff),
// or the targeting call itself fails. The seed path is resolved ONCE here,
// at round's attempt-1 token via artifactPaths, and is never recomputed per
// attempt — a same-round retry reuses the identical path, exactly like the
// round-scoped priorReviews/priorFixerReports hydration. Callers must clear
// any leftover seed file at that path (moveStaleArtifacts) before calling
// this, so a re-run's targeting call never trips the pre-existing-output-
// file rejection.
func (e *Engine) runPreRoundTargeting(runDir string, round int, p Profile, rounds []roundRecord) string {
	if !p.PreRoundTargeting {
		return ""
	}
	handoffPath, _, ok := latestValidHandoff(e.name, rounds)
	if !ok {
		// Nothing to target from yet (round 1, or no handoff has ever
		// survived a valid parse) — this is not a failure, so it logs
		// nothing, mirroring the round-1-runs-no-judge posture in the main
		// loop below.
		return ""
	}
	seedPath := artifactPaths(runDir, round, 1).Seed
	if _, ok := runTargeting(e.stencilsDir, e.shuttle, e.name, round, handoffPath, seedPath, p.JudgeModel, p.JudgeEffort); !ok {
		return ""
	}
	return seedPath
}

// runRound drives round's RoundRunner attempts (up to two: a fresh attempt,
// then one deterministic retry after a died/timeout outcome or an
// asking-triage RETRY verdict), returning the round's outcome once the
// runner reaches done. priorReviews and priorFixerReports are the hydration
// accumulated from every already-completed round; seedPath is this round's
// pre-round-targeting seed (already resolved once by runPreRoundTargeting,
// or "" when targeting produced none) — both attempts of the same round
// number reuse the SAME hydration and the SAME seed path, since a retry
// produces no new completed round. A second consecutive non-done attempt is
// an infrastructure error, deliberately NOT modeled as OutcomeStuck — it
// means the machinery failed twice, not that the artifact will not converge.
func (e *Engine) runRound(runDir string, round int, p Profile, priorReviews, priorFixerReports []string, seedPath string) (roundOutcome, error) {
	// triagePath accumulates across the retry loop: it is set only when an
	// asking attempt actually spawns a triage call, and is threaded into
	// the eventual done-outcome's roundOutcome so state.json records that a
	// triage call ran, even though the retry that follows it produces the
	// round's final (done) attempt.
	var triagePath string
	for attempt := 1; attempt <= 2; attempt++ {
		// Attempt 1's stale artifacts (including any leftover seed file)
		// were already cleared by the caller, before pre-round targeting
		// ran — clearing them again here would move aside the seed file
		// targeting just wrote. A retry attempt's own stale artifacts from
		// an earlier interrupted resume still need clearing here.
		if attempt > 1 {
			if err := moveStaleArtifacts(e.name, runDir, round, attempt); err != nil {
				// Same rationale as the pre-round clear above: the wrapped error
				// names the file but not the round/attempt in progress.
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
			Model:             p.Model,
			Effort:            p.Effort,
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
				TriagePath:      triagePath,
				SessionID:       result.SessionID,
				Paths:           paths,
			}, nil
		}

		if result.Outcome == shuttleengine.OutcomeAsking {
			// A second consecutive asking outcome fails the same generic
			// "failed twice" way a died/timeout round does, WITHOUT a
			// second triage spawn: the round is already failing regardless
			// of this attempt's triage verdict, so there is nothing left
			// for triage to usefully classify.
			if attempt == 2 {
				return roundOutcome{}, e.errf("round %d failed twice (%s); session %s, kept run dir %s", round, result.Outcome, result.SessionID, result.RunDir)
			}
			// The agent stopped mid-round asking a question rather than
			// finishing; triage classifies whether a fresh retry can
			// plausibly proceed. Triage itself is fail-safe (never an
			// error) and defaults to RETRY on any of its own
			// infrastructure failures.
			triageVerdict, rationale := runTriage(e.stencilsDir, e.shuttle, e.name, round, result.LastAssistantMessage, paths.Triage, p.JudgeModel, p.JudgeEffort)
			triagePath = paths.Triage
			if triageVerdict == TriageGiveUp {
				return roundOutcome{}, e.errf("round %d agent gave up asking: %s (session %s, run dir %s)", round, rationale, result.SessionID, result.RunDir)
			}
			continue
		}

		// died / timeout: a cheap deterministic retry — these are nearly
		// always environmental, unlike asking's interpretable text.
		if attempt == 2 {
			return roundOutcome{}, e.errf("round %d failed twice (%s); session %s, kept run dir %s", round, result.Outcome, result.SessionID, result.RunDir)
		}
		// attempt 1 died/timed out: the retry itself is otherwise invisible
		// until it either succeeds silently or fails twice (already errored
		// with full context just above) — log it here so an operator can see
		// the retry happening, not only its eventual outcome.
		logger.Warn(e.name+": round attempt died or timed out, retrying", "round", round, "outcome", result.Outcome, "sessionID", result.SessionID)
	}
	// Unreachable: every path through the loop above returns by the end of
	// attempt 2.
	return roundOutcome{}, e.errf("round %d exhausted its bounded retries without a terminal outcome", round)
}

// collectPriorHydration builds the priorReviews and priorFixerReports lists
// a fresh round's attempt is seeded with, from every already completed
// round in rounds: each round's ReviewPath and FixerReportPath are always
// included, and a round's GatePath is included in priorReviews ONLY when
// that round's GatePassed is false — passing-gate output is never fed
// forward, since a clean command run has nothing for the next round to
// learn from.
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
// every completed round's ReviewPath in order, plus the current round's fresh
// review. Gate-command output files are deliberately NOT included here even
// though collectPriorHydration feeds them to the next round's attempt — the
// judge compares blocking findings across reviews, and a gate transcript is
// not a review (see the verdict-judge contract in doc.go).
func collectJudgeReviews(rounds []roundRecord, currentReviewPath string) []string {
	reviews := make([]string, 0, len(rounds)+1)
	for _, r := range rounds {
		reviews = append(reviews, r.ReviewPath)
	}
	return append(reviews, currentReviewPath)
}

// recordHandoffIfValid reads and ParseHandoff-validates the handoff file a
// successful judge call (judgeOK true) just produced at path, returning
// path back unchanged when it is well-formed. A missing or unparseable
// handoff logs a name-prefixed logger.Warn naming label (matching
// runJudgeCall's own label argument, e.g. "milestone judge", so the two
// Warns read consistently in an operator's log), round, and cause, and
// returns "" — never an error, never STUCK, and the verdict this handoff
// rode alongside is completely unaffected either way; the loop only ever
// calls this from inside the judgeOK guard, so a failed verdict never
// reaches here at all.
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

// isMilestoneRung reports whether round is one of caps' milestone rungs —
// every entry except the last, which is the hard cap rather than a
// judge-gated rung.
func isMilestoneRung(caps []int, round int) bool {
	for _, c := range caps[:len(caps)-1] {
		if c == round {
			return true
		}
	}
	return false
}

// resultFromState builds the block-level Result Engine.Run returns from st,
// mirroring every persisted roundRecord into a RoundSummary.
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
			TriagePath:      r.TriagePath,
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
