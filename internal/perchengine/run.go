// run.go implements Engine.Run, the deterministic round loop that drives
// one perch block from a fresh or resumed run dir to a terminal Result: it
// validates the profile, resolves the block's on-disk identity and resume
// point, then loops one round at a time through the burler round, the
// pluggable convergence gate, and the milestone-laddered stuck ladder,
// persisting state after every round so a crash or an operator pause can
// resume from exactly where the block left off.

package perchengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// runLockName is the exclusive-lease file name inside a block's run dir,
// held for the ENTIRE duration of one Engine.Run call — distinct from
// state.json.lock, which internal/state only holds for the instant of one
// read or write. Without this, two concurrent `lyx perch run` invocations
// against the same run dir would each classify resume/fresh from state.json
// and then both drive rounds into the same dir: colliding artifact paths,
// clobbered state.json appends, and two burler agents editing the worktree
// at once.
const runLockName = "run.lock"

// roundOutcome captures what a round's retry loop produced once burler
// finally reached a done outcome: everything the round-loop body needs to
// run the gate, evaluate convergence, run the stuck-ladder judge, and
// persist a roundRecord.
type roundOutcome struct {
	Attempts        int
	Verdict         burlerengine.Verdict
	Findings        []burlerengine.Finding
	ReviewPath      string
	FixerReportPath string
	TriagePath      string
	SessionID       string
	Paths           roundArtifactPaths
}

// Run drives one perch block's round loop for Profile p, reading and
// persisting state at runDir. It validates p against e.cfg, ensures runDir
// exists, derives the block's identity (ProfileHash) and resume point
// (loadOrInitState), then loops one round at a time: a pause check at the
// round boundary only, a burler round with its bounded non-done retry, the
// pluggable convergence gate, and — on a non-converged round — the
// milestone-laddered stuck ladder. Every returned error is
// "perch: "-prefixed; the returned Result mirrors the persisted state's
// rounds as RoundSummary values.
func (e *Engine) Run(p Profile, runDir string) (result Result, err error) {
	// A pause requested while the final round was still in flight can
	// observe a terminal, non-PAUSED outcome once that round settles on its
	// own (the pause flag is checked only at the NEXT round boundary, which
	// never arrives). The stale flag must not linger in the run dir (and get
	// weft-committed alongside a finished block) once the block is done
	// judging — clearing it centrally here, once, covers every terminal
	// return site without duplicating the call at each one.
	defer func() {
		if err == nil && result.Outcome != OutcomePaused {
			_ = clearPauseFlag(runDir)
		}
	}()

	// Identity is the profile AS SUPPLIED by the caller, hashed before
	// default resolution mutates it: a perch.yaml default change (judge
	// model, cap ladder) must never silently change — or invalidate the
	// resume of — a block whose profile file the operator never touched.
	hash, err := ProfileHash(p)
	if err != nil {
		return Result{}, err
	}

	if err := p.validate(e.cfg); err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("perch: create run dir %q: %w", runDir, err)
	}

	// Held for this entire call: a second concurrent `lyx perch run` (or a
	// loom re-entry) against the SAME run dir must fail fast rather than
	// silently interleave rounds with this one. Released by the OS on
	// process exit/crash even if this call never reaches Release, so a
	// killed process never bricks the run dir for a later resume.
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(runDir, runLockName))
	if err != nil {
		return Result{}, fmt.Errorf("perch: acquire run lock for %q: %w", runDir, err)
	}
	if !locked {
		return Result{}, fmt.Errorf("perch: block %q is already running (run.lock held); wait for it to finish or use a different --run-id", runDir)
	}
	defer runLock.Release()

	// A resumed block must never instantly re-pause on a flag left over
	// from the run that requested the pause it is now resuming from.
	if err := clearPauseFlag(runDir); err != nil {
		return Result{}, err
	}

	st, resume, err := loadOrInitState(runDir, hash, p.RoundCaps)
	if err != nil {
		return Result{}, err
	}

	// Seam defaulting happens here, once, at Run's entry: card 10's New
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
	// actually started with even if perch.yaml's default changed in between,
	// which the identity hash above deliberately does not cover.
	caps := st.RoundCaps
	if len(caps) == 0 {
		return Result{}, fmt.Errorf("perch: state.json in %q records no round-caps ladder; the state file is corrupt", runDir)
	}
	hardCap := caps[len(caps)-1]

	for round := resume.NextRound; ; round++ {
		// Pause is checked ONLY here, at the round boundary — never
		// mid-round — so a paused block always resumes at a clean round
		// start rather than an in-progress one.
		if pause() {
			if err := saveState(runDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomePaused, ""), nil
		}

		priorReviews, priorFixerReports := collectPriorHydration(st.Rounds)

		outcome, err := e.runRound(runDir, round, p, priorReviews, priorFixerReports)
		if err != nil {
			return Result{}, err
		}

		record := roundRecord{
			Round:           round,
			Attempts:        outcome.Attempts,
			ShuttleOutcome:  string(shuttleengine.OutcomeDone),
			Verdict:         string(outcome.Verdict),
			BlockingCount:   countBlockingFindings(outcome.Findings),
			ReviewPath:      outcome.ReviewPath,
			FixerReportPath: outcome.FixerReportPath,
			TriagePath:      outcome.TriagePath,
			SessionID:       outcome.SessionID,
		}

		// The gate command runs after this round's fix phase, in
		// llm-verdict-ignoring modes only; its cwd is always the worktree
		// root, not the run dir, since the command exercises the host
		// repo's own build/test surface.
		if p.Gate.Mode == GateCommand || p.Gate.Mode == GateBoth {
			output, exitZero, err := runCommand(p.Gate.Command, e.layout.WorktreeRoot, p.Gate.Timeout)
			if err != nil {
				// A could-not-start gate failure is a hard error, but the
				// burler round it follows COMPLETED — persist its record
				// first so a resume continues at the next round instead of
				// re-buying this one. The record carries no gate result (nil
				// GatePassed), which the loop reads as non-converged: the
				// safe direction.
				st.Rounds = append(st.Rounds, record)
				if saveErr := saveState(runDir, st); saveErr != nil {
					return Result{}, saveErr
				}
				return Result{}, fmt.Errorf("perch: round %d gate command: %w", round, err)
			}
			// Written on pass AND fail — the record is cheap — even though
			// only a failing gate file is ever fed forward as hydration.
			if err := writeGateOutput(outcome.Paths.Gate, p.Gate.Command, output, exitZero); err != nil {
				return Result{}, err
			}
			record.GatePath = outcome.Paths.Gate
			record.GatePassed = &exitZero
		}

		if converged(p.Gate.Mode, outcome.Verdict, record.GatePassed) {
			st.Rounds = append(st.Rounds, record)
			st.Outcome = string(OutcomeApproved)
			if err := saveState(runDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomeApproved, ""), nil
		}

		// The stuck ladder is reached only on a non-converged round. Every
		// trigger below is burler-verdict-based: a round with
		// VerdictApproved but a failing command (command/both gate modes)
		// skips every rung and simply loops, bounded by the hard cap below
		// and fed forward via the gate file.
		if round == hardCap {
			st.Rounds = append(st.Rounds, record)
			st.Outcome = string(OutcomeStuck)
			st.StuckReason = string(StuckHardCap)
			if err := saveState(runDir, st); err != nil {
				return Result{}, err
			}
			return resultFromState(st, OutcomeStuck, StuckHardCap), nil
		}

		if outcome.Verdict == burlerengine.VerdictBlocking {
			// The judge reasons over the full REVIEW history including this
			// round's own fresh review — and only reviews: unlike the burler
			// hydration in priorReviews, failed gate-command output files are
			// deliberately excluded, since the judge's material is blocking
			// findings recurring across review files (doc.go's verdict-judge
			// contract), and a gate transcript has no findings to compare.
			judgeReviews := collectJudgeReviews(st.Rounds, outcome.ReviewPath)

			// The circling check never runs on the round immediately after an
			// APPROVED round (reachable in command/both gate modes, where an
			// APPROVED round with a failing command does not converge): the
			// immediately-prior review has zero blocking findings, so fresh
			// findings here are new work surfacing, not evidence of circling —
			// and a false CIRCLING verdict is a permanent, wrong STUCK. The
			// milestone gate is deliberately NOT exempted: a rung asks about
			// the whole trajectory, not recurrence against the prior round.
			prevRoundApproved := len(st.Rounds) > 0 &&
				st.Rounds[len(st.Rounds)-1].Verdict == string(burlerengine.VerdictApproved)

			switch {
			case isMilestoneRung(caps, round):
				// The milestone gate REPLACES the circling check for this
				// round — a rung round issues exactly one judge call.
				jv, _, judgeOK := runMilestone(e.shuttle, judgeInputs{
					Round:        round,
					HardCap:      hardCap,
					PriorReviews: judgeReviews,
					VerdictPath:  outcome.Paths.Judge,
					Model:        p.JudgeModel,
					Effort:       p.JudgeEffort,
				})
				// Only a REAL verdict is recorded — a fail-safe fallback
				// (judgeOK false) leaves the record's judge fields empty, so
				// an operator reading state.json can tell a genuine CONTINUE
				// apart from a judge infrastructure failure that never
				// actually answered (the Warn logged inside the call above
				// is the only trace of the failure).
				if judgeOK {
					record.JudgePath = outcome.Paths.Judge
					record.JudgeVerdict = string(jv)
				}
				if jv == JudgeStop {
					st.Rounds = append(st.Rounds, record)
					st.Outcome = string(OutcomeStuck)
					st.StuckReason = string(StuckMilestoneStop)
					if err := saveState(runDir, st); err != nil {
						return Result{}, err
					}
					return resultFromState(st, OutcomeStuck, StuckMilestoneStop), nil
				}
				// JudgeContinue / JudgeUncertain: fall through and loop.
			case round >= 2 && !prevRoundApproved:
				jv, _, judgeOK := runCircling(e.shuttle, judgeInputs{
					Round:        round,
					PriorReviews: judgeReviews,
					VerdictPath:  outcome.Paths.Judge,
					Model:        p.JudgeModel,
					Effort:       p.JudgeEffort,
				})
				// See the milestone-rung branch above: only a REAL verdict
				// is recorded, never the fail-safe fallback.
				if judgeOK {
					record.JudgePath = outcome.Paths.Judge
					record.JudgeVerdict = string(jv)
				}
				if jv == JudgeCircling {
					st.Rounds = append(st.Rounds, record)
					st.Outcome = string(OutcomeStuck)
					st.StuckReason = string(StuckCircling)
					if err := saveState(runDir, st); err != nil {
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
		if err := saveState(runDir, st); err != nil {
			return Result{}, err
		}
	}
}

// runRound drives round's burler attempts (up to two: a fresh attempt, then
// one deterministic retry after a died/timeout outcome or an
// asking-triage RETRY verdict), returning the round's outcome once burler
// reaches done. priorReviews and priorFixerReports are the hydration
// accumulated from every already-completed round; both attempts of the same
// round number reuse the same hydration, since a retry produces no new
// completed round. A second consecutive non-done attempt is an
// infrastructure error, deliberately NOT modeled as OutcomeStuck — it means
// the machinery failed twice, not that the artifact will not converge.
func (e *Engine) runRound(runDir string, round int, p Profile, priorReviews, priorFixerReports []string) (roundOutcome, error) {
	// triagePath accumulates across the retry loop: it is set only when an
	// asking attempt actually spawns a triage call, and is threaded into
	// the eventual done-outcome's roundOutcome so state.json records that a
	// triage call ran, even though the retry that follows it produces the
	// round's final (done) attempt.
	var triagePath string
	for attempt := 1; attempt <= 2; attempt++ {
		// A round that started but never reached done on a prior resume
		// left partial artifacts behind; move them aside before this
		// attempt writes to the same paths (shuttle rejects pre-existing
		// output files).
		if err := moveStaleArtifacts(runDir, round, attempt); err != nil {
			return roundOutcome{}, err
		}
		paths := artifactPaths(runDir, round, attempt)

		roundProfile := buildRoundProfile(p, paths, priorReviews, priorFixerReports)
		result, err := e.burler.Run(roundProfile, burlerengine.RunOpts{
			Model:   p.Model,
			Effort:  p.Effort,
			Timeout: p.Timeout,
			Round:   roundToken(round, attempt),
		})
		if err != nil {
			return roundOutcome{}, fmt.Errorf("perch: round %d burler run: %w", round, err)
		}

		if result.Outcome == shuttleengine.OutcomeDone {
			return roundOutcome{
				Attempts:        attempt,
				Verdict:         result.Verdict,
				Findings:        result.Findings,
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
				return roundOutcome{}, fmt.Errorf("perch: round %d failed twice (%s); session %s, kept shuttle run dir %s", round, result.Outcome, result.SessionID, result.RunDir)
			}
			// The agent stopped mid-round asking a question rather than
			// finishing; triage classifies whether a fresh retry can
			// plausibly proceed. Triage itself is fail-safe (never an
			// error) and defaults to RETRY on any of its own
			// infrastructure failures.
			triageVerdict, rationale := runTriage(e.shuttle, round, result.LastAssistantMessage, paths.Triage, p.JudgeModel, p.JudgeEffort)
			triagePath = paths.Triage
			if triageVerdict == TriageGiveUp {
				return roundOutcome{}, fmt.Errorf("perch: round %d agent gave up asking: %s (session %s, run dir %s)", round, rationale, result.SessionID, result.RunDir)
			}
			continue
		}

		// died / timeout: a cheap deterministic retry — these are nearly
		// always environmental, unlike asking's interpretable text.
		if attempt == 2 {
			return roundOutcome{}, fmt.Errorf("perch: round %d failed twice (%s); session %s, kept shuttle run dir %s", round, result.Outcome, result.SessionID, result.RunDir)
		}
	}
	// Unreachable: every path through the loop above returns by the end of
	// attempt 2.
	return roundOutcome{}, fmt.Errorf("perch: round %d exhausted its bounded retries without a terminal outcome", round)
}

// collectPriorHydration builds the priorReviews and priorFixerReports lists
// a fresh round's burler profile is seeded with, from every already
// completed round in rounds: each round's ReviewPath and FixerReportPath
// are always included, and a round's GatePath is included in priorReviews
// ONLY when that round's GatePassed is false — passing-gate output is never
// fed forward, since a clean command run has nothing for the next round to
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
// though collectPriorHydration feeds them to the next BURLER round — the judge
// compares blocking findings across reviews, and a gate transcript is not a
// review (see the verdict-judge contract in doc.go).
func collectJudgeReviews(rounds []roundRecord, currentReviewPath string) []string {
	reviews := make([]string, 0, len(rounds)+1)
	for _, r := range rounds {
		reviews = append(reviews, r.ReviewPath)
	}
	return append(reviews, currentReviewPath)
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

// countBlockingFindings returns how many of findings carry
// burlerengine.SeverityBlocking, the count a roundRecord persists
// independent of the round's overall Verdict.
func countBlockingFindings(findings []burlerengine.Finding) int {
	count := 0
	for _, f := range findings {
		if f.Severity == burlerengine.SeverityBlocking {
			count++
		}
	}
	return count
}

// resultFromState builds the block-level Result Engine.Run returns from st,
// mirroring every persisted roundRecord into a RoundSummary.
func resultFromState(st runState, outcome Outcome, stuckReason StuckReason) Result {
	rounds := make([]RoundSummary, 0, len(st.Rounds))
	for _, r := range st.Rounds {
		rounds = append(rounds, RoundSummary{
			Round:           r.Round,
			Attempts:        r.Attempts,
			Verdict:         burlerengine.Verdict(r.Verdict),
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
