// adapter.go implements burlerAdapter, the treadleengine.RoundRunner that
// adapts burler round-attempts onto treadleengine's attempt-level seam:
// RunAttempt maps a treadleengine.AttemptInput onto burlerengine.Profile/
// RunOpts (buildRoundProfile), runs the round through the Burler seam
// (engine.go), and maps burlerengine.Result back onto
// treadleengine.AttemptResult, including BlockingCount via
// countBlockingFindings over Result.Findings — the round-runner-seam shared
// decision's concrete perch-side realization.

package perchengine

import (
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// burlerAdapter implements treadleengine.RoundRunner, closing over Profile's content fields.
type burlerAdapter struct {
	burler  Burler
	profile Profile
}

// RunAttempt implements treadleengine.RoundRunner.
func (a *burlerAdapter) RunAttempt(in treadleengine.AttemptInput) (treadleengine.AttemptResult, error) {
	roundProfile := buildRoundProfile(a.profile, in.ReviewPath, in.FixerReportPath, in.PriorReviews, in.PriorFixerReports)

	result, err := a.burler.Run(roundProfile, burlerengine.RunOpts{
		Model:   in.Model,
		Effort:  in.Effort,
		Timeout: in.Timeout,
		Round:   in.RoundToken,
	})
	if err != nil {
		return treadleengine.AttemptResult{}, err
	}

	return treadleengine.AttemptResult{
		Outcome:              result.Outcome,
		Verdict:              treadleengine.Verdict(result.Verdict),
		BlockingCount:        countBlockingFindings(result.Findings),
		ReviewPath:           result.ReviewPath,
		FixerReportPath:      result.FixerReportPath,
		SessionID:            result.SessionID,
		LastAssistantMessage: result.LastAssistantMessage,
		RunDir:               result.RunDir,
	}, nil
}

// buildRoundProfile composes the burlerengine.Profile for one round, mapping Profile's content fields and the caller-supplied paths.
func buildRoundProfile(p Profile, reviewPath, fixerReportPath string, priorReviews, priorFixerReports []string) burlerengine.Profile {
	return burlerengine.Profile{
		Target:            p.Target,
		Fasit:             p.Fasit,
		Rubric:            p.Rubric,
		FixScope:          p.FixScope,
		ToolUse:           p.ToolUse,
		ClusterFan:        p.ClusterFan,
		ReviewPath:        reviewPath,
		FixerReportPath:   fixerReportPath,
		PriorReviews:      priorReviews,
		PriorFixerReports: priorFixerReports,
	}
}

// countBlockingFindings returns the count of findings with SeverityBlocking.
func countBlockingFindings(findings []burlerengine.Finding) int {
	count := 0
	for _, f := range findings {
		if f.Severity == burlerengine.SeverityBlocking {
			count++
		}
	}
	return count
}
