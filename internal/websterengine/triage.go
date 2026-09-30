// triage.go classifies a red integration verify as flaky, pre-existing, or a regression.
// It runs the verify command through the verifyRunner seam and touches no state, report, or summary file,
// so the integration stage can call it with no state-mutation lease held.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
)

// baselineUnavailableWarning states why every head failure was classified a regression.
const baselineUnavailableWarning = "triage: baseline comparison unavailable (no repository handle or no batch start commit); every failure is treated as a regression"

// verifyChainMarker is echoed before the verify chain's last step on the triage rerun:
// its presence in the output proves every earlier step passed, so the chain's red exit came from its last step and no step was skipped.
const verifyChainMarker = "lyx-webster-triage-last-step-reached"

// verifyChainUnsafeChars are the sh and cmd characters that can mask a step's exit status, run past a failing step, or hide an "&&" from a plain split:
// separators, pipes, single ampersands (background, redirection, cmd's unconditional chain), subshells and substitutions, negation, comments, quotes, and escapes.
const verifyChainUnsafeChars = ";|&()`'\"\\!#^\n\r"

// triageOutcome is triageIntegrationFailure's result: the failures to record, the triage classification, and the warnings the stage surfaces.
type triageOutcome struct {
	Failures []IntegrationFailure
	Triage   IntegrationTriage
	Warnings []string
}

// triageIntegrationFailure classifies the integration fork's red verify run.
// firstLogPath is the first run's captured log;
// a missing or unreadable log leaves the first run's failure set unknown rather than failing the triage.
// It reruns verifyCmd once at the unchanged head,
// and on a red rerun it compares each failing test identity against the earliest of startSHAs, the batches' recorded start commits.
// A failure is excused as pre-existing only when all of these hold, and is a regression otherwise, so triage never excuses an unproven failure:
// it is a test identity (a package or opaque identity is never excused),
// the verify command is a plain "&&" chain whose rerun reached its last step (so no step failed unseen or went unrun),
// the start commits lie on one line of history,
// and the same identity fails at the earliest of them.
// A spawn error from run and a checkout error are returned as errors.
func triageIntegrationFailure(run verifyRunner, repo FabricBisector, startSHAs []string, verifyCmd, worktree, scratchDir, firstLogPath string) (triageOutcome, error) {
	var first []IntegrationFailure
	firstKnown := false
	if log, err := os.ReadFile(firstLogPath); err == nil {
		first = parseVerifyFailures(string(log), false)
		firstKnown = true
	}

	rerunCmd, chainReason := instrumentVerifyChain(verifyCmd)
	if chainReason != "" {
		rerunCmd = verifyCmd
	}
	rerun, err := run(rerunCmd, worktree, rerunLogPath(scratchDir))
	if err != nil {
		return triageOutcome{}, err
	}

	if rerun.Passed {
		if !firstKnown {
			first = []IntegrationFailure{{ID: opaqueFailureID, Kind: FailureKindOpaque}}
		}
		return triageOutcome{
			Failures: first,
			Triage: IntegrationTriage{
				Verdict: TriageVerdictFlaky,
				Rerun:   TriageRerunPassed,
				Flaky:   failureIDs(first),
			},
		}, nil
	}

	head := parseVerifyFailures(rerun.Output, false)
	headIDs := map[string]bool{}
	for _, f := range head {
		headIDs[f.ID] = true
	}

	failures := append([]IntegrationFailure(nil), head...)
	var flaky []string
	for _, f := range first {
		if headIDs[f.ID] {
			continue
		}
		flaky = append(flaky, f.ID)
		failures = append(failures, f)
	}

	out := triageOutcome{
		Failures: failures,
		Triage: IntegrationTriage{
			Rerun: TriageRerunFailed,
			Flaky: flaky,
		},
	}

	if repo == nil || len(startSHAs) == 0 {
		return classifyAllAsRegressions(out, head, baselineUnavailableWarning), nil
	}
	if chainReason != "" {
		return classifyAllAsRegressions(out, head, "triage: "+chainReason+"; every failure is treated as a regression"), nil
	}
	if rerunCmd != verifyCmd && !lastStepReached(rerun.Output) {
		return classifyAllAsRegressions(out, head, "triage: the verify chain stopped before its last step, so the steps after the failing one never ran; every failure is treated as a regression"), nil
	}
	baselineSHA, err := earliestCommit(repo, startSHAs)
	if err != nil {
		return classifyAllAsRegressions(out, head, "triage: no single earliest batch start commit ("+err.Error()+"); every failure is treated as a regression"), nil
	}

	baseline, err := runAtBaseline(run, repo, baselineSHA, verifyCmd, worktree, baselineLogPath(scratchDir))
	if err != nil {
		return triageOutcome{}, err
	}
	out.Triage.BaselineSHA = baselineSHA

	baselineIDs := map[string]bool{}
	for _, f := range parseVerifyFailures(baseline.Output, baseline.Passed) {
		baselineIDs[f.ID] = true
	}
	for _, f := range head {
		if f.Kind == FailureKindTest && baselineIDs[f.ID] {
			out.Triage.PreExisting = append(out.Triage.PreExisting, f.ID)
		} else {
			out.Triage.Regressions = append(out.Triage.Regressions, f.ID)
		}
	}
	if len(out.Triage.Regressions) > 0 {
		out.Triage.Verdict = TriageVerdictRegression
	} else {
		out.Triage.Verdict = TriageVerdictPreExisting
	}
	return out, nil
}

// classifyAllAsRegressions returns out with every head identity a regression and warning recorded,
// the fail-closed answer whenever no failure can be proven pre-existing.
func classifyAllAsRegressions(out triageOutcome, head []IntegrationFailure, warning string) triageOutcome {
	out.Triage.Verdict = TriageVerdictRegression
	out.Triage.Regressions = failureIDs(head)
	out.Warnings = []string{warning}
	return out
}

// instrumentVerifyChain returns verifyCmd with verifyChainMarker echoed before its last "&&" step, leaving a single-step command unchanged.
// It returns instead a non-empty reason when a red exit of verifyCmd cannot be attributed to the go test failures its last step prints:
// a step carries a verifyChainUnsafeChars character, a step is empty, or a step runs with -failfast, which leaves the tests after a failing one unrun.
func instrumentVerifyChain(verifyCmd string) (instrumented, reason string) {
	steps := strings.Split(verifyCmd, "&&")
	for _, step := range steps {
		if strings.TrimSpace(step) == "" {
			return "", "the verify command has an empty && step"
		}
		if strings.ContainsAny(step, verifyChainUnsafeChars) {
			return "", "the verify command is not a plain && chain, so a failing step can be masked or run past"
		}
		for _, arg := range strings.Fields(step) {
			if strings.HasPrefix(arg, "-") && strings.Contains(arg, "failfast") {
				return "", "the verify command runs with -failfast, so a failing test leaves the tests after it unrun"
			}
		}
	}
	if len(steps) == 1 {
		return verifyCmd, ""
	}
	last := len(steps) - 1
	return strings.Join(steps[:last], "&&") + "&& echo " + verifyChainMarker + " &&" + steps[last], ""
}

// lastStepReached reports whether output carries verifyChainMarker on a line of its own.
func lastStepReached(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == verifyChainMarker {
			return true
		}
	}
	return false
}

// earliestCommit returns the member of shas that is an ancestor of every other member, the earliest in history.
// It returns an error when two members lie on diverging lines of history or an ancestry probe fails,
// since no member can then be proven to predate every batch's work.
func earliestCommit(repo FabricBisector, shas []string) (string, error) {
	earliest := shas[0]
	for _, sha := range shas[1:] {
		if sha == earliest {
			continue
		}
		before, err := repo.IsAncestor(earliest, sha)
		if err != nil {
			return "", err
		}
		if before {
			continue
		}
		after, err := repo.IsAncestor(sha, earliest)
		if err != nil {
			return "", err
		}
		if !after {
			return "", fmt.Errorf("%s and %s lie on diverging lines of history", earliest, sha)
		}
		earliest = sha
	}
	return earliest, nil
}

// runAtBaseline checks out sha detached, runs verifyCmd through run, and restores the original branch afterwards even when the run errored.
// A failed restore is joined onto any run error, never dropped.
func runAtBaseline(run verifyRunner, repo FabricBisector, sha, verifyCmd, worktree, logPath string) (result verifyRun, err error) {
	branch, err := repo.CurrentBranch()
	if err != nil {
		return verifyRun{}, fmt.Errorf("webster: triage: capture current branch: %w", err)
	}
	defer func() {
		if restoreErr := repo.RestoreBranch(branch); restoreErr != nil {
			restoreErr = fmt.Errorf("webster: triage: restore branch %s (the worktree is left on a detached HEAD): %w", branch, restoreErr)
			if err == nil {
				err = restoreErr
				return
			}
			err = errors.Join(err, restoreErr)
		}
	}()

	if err := repo.CheckoutDetached(sha); err != nil {
		return verifyRun{}, fmt.Errorf("webster: triage: checkout baseline %s: %w", sha, err)
	}
	return run(verifyCmd, worktree, logPath)
}

// triageWarnings returns one warning per non-empty non-regression category, flaky then pre-existing, each naming its identities.
// It returns nil when t has neither.
func triageWarnings(t IntegrationTriage) []string {
	var warnings []string
	if len(t.Flaky) > 0 {
		warnings = append(warnings, "integration verify: flaky failures passed or vanished on rerun: "+strings.Join(t.Flaky, ", "))
	}
	if len(t.PreExisting) > 0 {
		warnings = append(warnings, "integration verify: pre-existing failures also fail at baseline "+t.BaselineSHA+": "+strings.Join(t.PreExisting, ", "))
	}
	return warnings
}

// triageStuckReason builds the RunResult.StuckReason for a run demoted by a regression.
// It names every regressing identity and the localized card, or states the card was not localized when offendingCard is "unknown".
func triageStuckReason(regressions []IntegrationFailure, offendingCard string) string {
	reason := "integration verify regressed: " + strings.Join(failureIDs(regressions), ", ")
	if offendingCard == "unknown" {
		return reason + " (offending card not localized)"
	}
	return reason + " (offending card: " + offendingCard + ")"
}

// writeTriageFrictionNote records the flaky and pre-existing identities as a friction note,
// since a pre-existing failure is environment trouble the hub collects from friction notes.
// It is a no-op when frictionDir is empty or both non-regression lists are empty.
func writeTriageFrictionNote(frictionDir string, t IntegrationTriage) error {
	if frictionDir == "" || (len(t.Flaky) == 0 && len(t.PreExisting) == 0) {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-verify-triage")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("Integration-suite triage: non-regression verify failures\n\n")
	if len(t.Flaky) > 0 {
		b.WriteString("These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n")
		for _, id := range t.Flaky {
			b.WriteString("- " + id + "\n")
		}
		b.WriteString("\n")
	}
	if len(t.PreExisting) > 0 {
		b.WriteString("These identities also fail at the plan's starting commit (pre-existing):\n\n")
		for _, id := range t.PreExisting {
			b.WriteString("- " + id + "\n")
		}
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: triage: write friction note %s: %w", path, err)
	}
	return nil
}

// failureIDs returns the ids of fs in order.
func failureIDs(fs []IntegrationFailure) []string {
	ids := make([]string, 0, len(fs))
	for _, f := range fs {
		ids = append(ids, f.ID)
	}
	return ids
}
