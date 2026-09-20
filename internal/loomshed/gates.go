// gates.go implements this package's two gate closures: NewDiscussionGate and NewPlanGate.
// Both are the gate half of the Gate Self-Check Parity Invariant -- each calls the identical package
// function its CLI self-check verb does (discussionparser.Validate and planglyph.ValidateFormat,
// respectively), so the operator's own self-check verb and the automated gate can never disagree
// about what "valid" means.

package loomshed

import (
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// NewDiscussionGate returns the Discussion-Write row's gate closure: a shuttleengine.Gate that
// calls discussionparser.Validate(decisionRecordPath, supportLogPath) once and maps the result onto
// the gate contract.
//
// Both paths are told rather than derived, because loomengine's own accessors for them take a
// *lyxcwd.Location this package may not import.
//
// A non-nil error from Validate is returned verbatim alongside the zero shuttleengine.GateResult: a
// gate that could not read the artifact has found no defect to re-prompt over, it is an
// infrastructure fault, and it must never burn an attempt or reach the LLM. A non-empty findings
// slice produces GateResult{Passed: false, Findings: formatDiscussionFindings(findings)} and a nil
// error, preceded by a logger.Warn carrying the gate name, the decision record path, and the same
// formatted findings -- that warn line is the only durable record of why an artifact was refused,
// because the findings file itself lives in the ephemeral run directory finalize deletes on the Done
// cleanup. An empty slice produces GateResult{Passed: true} and a nil error.
func NewDiscussionGate(decisionRecordPath, supportLogPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := discussionparser.Validate(decisionRecordPath, supportLogPath)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}

		formatted := formatDiscussionFindings(findings)
		logger.Warn("loomshed: discussion gate failed validation", "gate", "Discussion-Gate", "decisionRecord", decisionRecordPath, "findings", formatted)
		return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
	}
}
