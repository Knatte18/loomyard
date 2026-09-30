// triage.go classifies a red integration verify as flaky, pre-existing, or a regression.
// It runs the verify command through the verifyRunner seam and touches no state, report, or
// summary file, so the integration stage can call it with no state-mutation lease held.

package websterengine

import (
	"errors"
	"fmt"
	"os"
)

// baselineUnavailableWarning states why every head failure was classified a regression.
const baselineUnavailableWarning = "triage: baseline comparison unavailable (no repository handle or no baseline SHA); every failure is treated as a regression"

// triageOutcome is triageIntegrationFailure's result: the failures to record, the triage
// classification, and the warnings the stage surfaces.
type triageOutcome struct {
	Failures []IntegrationFailure
	Triage   IntegrationTriage
	Warnings []string
}

// triageIntegrationFailure classifies the integration fork's red verify run.
// firstLogPath is the first run's captured log; a missing or unreadable log leaves the first
// run's failure set unknown rather than failing the triage.
// It reruns verifyCmd once at the unchanged head, and on a red rerun compares each failing
// identity against baselineSHA.
// A non-member of the baseline failure set is a regression: triage never excuses an unproven failure.
// A spawn error from run and a checkout error are returned as errors.
func triageIntegrationFailure(run verifyRunner, repo FabricBisector, baselineSHA, verifyCmd, worktree, scratchDir, firstLogPath string) (triageOutcome, error) {
	var first []IntegrationFailure
	firstKnown := false
	if log, err := os.ReadFile(firstLogPath); err == nil {
		first = parseVerifyFailures(string(log), false)
		firstKnown = true
	}

	rerun, err := run(verifyCmd, worktree, rerunLogPath(scratchDir))
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

	if repo == nil || baselineSHA == "" {
		out.Triage.Verdict = TriageVerdictRegression
		out.Triage.Regressions = failureIDs(head)
		out.Warnings = []string{baselineUnavailableWarning}
		return out, nil
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
		if f.Kind != FailureKindOpaque && baselineIDs[f.ID] {
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

// runAtBaseline checks out sha detached, runs verifyCmd through run, and restores the original
// branch afterwards even when the run errored.
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

// failureIDs returns the ids of fs in order.
func failureIDs(fs []IntegrationFailure) []string {
	ids := make([]string, 0, len(fs))
	for _, f := range fs {
		ids = append(ids, f.ID)
	}
	return ids
}
