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

// burlerAdapter implements treadleengine.RoundRunner over the Burler seam,
// closing over the perch Profile's content fields (Target/Fasit/Rubric/
// FixScope/ToolUse/ClusterFan) that stay fixed for the whole block — only
// the per-attempt paths and hydration lists in AttemptInput vary call to
// call.
type burlerAdapter struct {
	burler  Burler
	profile Profile
}

// RunAttempt implements treadleengine.RoundRunner: build's this attempt's
// burlerengine.Profile from the adapter's fixed content fields plus in's
// per-attempt paths and hydration (buildRoundProfile), runs it through the
// Burler seam with in's tuning and RoundToken, and maps the resulting
// burlerengine.Result onto treadleengine.AttemptResult.
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

// buildRoundProfile composes the burlerengine.Profile for one round: p's
// content fields carried through 1:1, this round's output paths from
// reviewPath/fixerReportPath, and the accumulated prior-round hydration
// lists supplied by the caller (treadleengine, via AttemptInput).
// buildRoundProfile never invents priorReviews/priorFixerReports entries
// itself (e.g. appending a prior round's gate-output file) — that
// accumulation is treadleengine's responsibility; this function only maps
// already-decided inputs onto burlerengine's field names. Its post-extraction
// signature is pinned: the old roundArtifactPaths parameter is gone (that
// type stays unexported inside treadleengine) in favor of plain path
// strings sourced from AttemptInput.
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

// countBlockingFindings returns how many of findings carry
// burlerengine.SeverityBlocking, the count treadleengine.AttemptResult
// carries independent of the round's overall Verdict.
func countBlockingFindings(findings []burlerengine.Finding) int {
	count := 0
	for _, f := range findings {
		if f.Severity == burlerengine.SeverityBlocking {
			count++
		}
	}
	return count
}
