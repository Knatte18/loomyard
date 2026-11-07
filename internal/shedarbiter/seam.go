// seam.go builds the sheddriver.ArbiterSeam a recipe's Go driver takes, over the arbiter call.

package shedarbiter

import (
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
)

// Seam builds the arbiter seam a recipe's driver takes, over deps, for the run runID.
// EscalationPending reports whether escalationEligible, the recipe's own reading, admits an awaiting stop at a producer.
// Judge runs the arbiter call on the stop envelope it is given;
// it maps the settled envelope onto the outcome the driver reads, with role loomengine.ArbiterRole.
// AttemptEnvelope reads the stop envelope path an attempt directory recorded.
func Seam(deps Deps, runID string, escalationEligible func(producer string) bool) *sheddriver.ArbiterSeam {
	return &sheddriver.ArbiterSeam{
		EscalationPending: func(producer string) (bool, error) {
			return escalationEligible(producer), nil
		},
		Judge: func(envelopePath, reportPath, caseBriefPath string) (sheddriver.ArbiterOutcome, error) {
			call, err := NewCall(deps, Input{RunID: runID, EnvelopePath: envelopePath, ReportPath: reportPath, CaseBriefPath: caseBriefPath})
			if err != nil {
				return sheddriver.ArbiterOutcome{}, err
			}
			envelope, err := call.Settle()
			if err != nil {
				return sheddriver.ArbiterOutcome{}, err
			}
			return sheddriver.ArbiterOutcome{
				Resume:      envelope.Verdict == loomengine.ArbiterVerdictResume,
				Reason:      envelope.Reason,
				Question:    envelope.Question,
				Goto:        envelope.Goto,
				Cause:       envelope.Cause,
				Attempt:     envelope.Attempt,
				AttemptDir:  envelope.AttemptDir,
				VerdictPath: envelope.VerdictPath,
				Verdict:     envelope.Verdict,
				Role:        loomengine.ArbiterRole,
			}, nil
		},
		AttemptEnvelope: func(attemptDir string) (string, error) {
			attempt, err := loomengine.ReadArbiterAttempt(attemptDir)
			return attempt.EnvelopePath, err
		},
	}
}
