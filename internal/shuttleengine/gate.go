// gate.go declares the gate contract: the four exported types a caller uses to attach a mechanical
// validator to a gated run (Gate, GateSpec, GateResult, GateOutcome), plus the package-private
// defaults and helpers the run loop (run.go, wait.go) spends them through.
// It inherits the Shuttle Provider-Seam Invariant (doc.go): the gate loop asks "is a new event in"
// only through the existing Engine.ParseEvents seam, and knows nothing about any provider's own hook
// payloads.

package shuttleengine

import "fmt"

// defaultGateAttempts is the re-prompt budget a GateSpec with Attempts <= 0 falls back to.
const defaultGateAttempts = 3

// gateFindingsFileName is the file name a failed gate's findings are written to inside the run
// directory, overwritten by every subsequent failed attempt.
const gateFindingsFileName = "gate-findings.md"

// GateResult is one gate closure invocation's verdict.
type GateResult struct {
	// Passed reports whether the gate found no defect.
	Passed bool
	// Findings is the detailed what-is-wrong text, empty when Passed. It is written to a per-run
	// findings file and never sent inline — see the "findings always ride a file" decision.
	Findings string
}

// Gate is a mechanical validator a gated run consults at its single verdict site (finalize):
// evaluate the run's declared output artifacts and report GateResult.
// It takes no argument, because the two production validators (planparser.ParsePlan,
// discussionparser's own validator) take materially different path shapes, and a closure captures
// its own told paths rather than this seam threading them through.
type Gate func() (GateResult, error)

// GateSpec packs a gate closure with its re-prompt budget — the one value every downstream seam
// (RunGated, AttachGated, a producer's RunOpts.Gate) carries from the point both are known through
// to the run loop, per the "one GateSpec at every hop" decision.
// The zero value (a nil Gate) means "ungated", which is what every row but the four gated ones
// supplies.
type GateSpec struct {
	// Gate is the validator this run's gate loop consults, or nil for an ungated run.
	Gate Gate
	// Attempts is the re-prompt budget: how many times a failed gate re-prompts the agent before the
	// run gives up and returns Done with a failed GateOutcome. 0 means the package default
	// (defaultGateAttempts).
	Attempts int
}

// attempts returns s.Attempts, or defaultGateAttempts when s.Attempts <= 0.
func (s GateSpec) attempts() int {
	if s.Attempts <= 0 {
		return defaultGateAttempts
	}
	return s.Attempts
}

// GateOutcome is a gated run's 1:1 gate report, carried on Result.Gate.
type GateOutcome struct {
	// Passed reports whether the run's final gate evaluation found no defect.
	Passed bool
	// Attempts counts re-prompts actually SENT on this run, and nothing else: a gate that passed
	// first try reports 0; a Done reached with no live session reports however many re-prompts had
	// already been sent before the session was lost (0 when it was lost before the first one); a
	// deadline that expires after N sends reports N. See the "attempts counts re-prompts actually
	// sent" decision.
	Attempts int
	// FindingsPath is the absolute path of the findings file the last failed attempt wrote, or empty
	// when Passed. It is diagnostic text ONLY, never a path to dereference after Wait returns: the
	// findings file lives in the run directory that finalize deletes on the Done cleanup every
	// exhausted gate takes, so a producer that opens it is a defect.
	FindingsPath string
}

// gateRepromptText returns the single-line re-prompt text Wait sends the agent after a failed gate
// attempt, naming findingsPath and instructing the agent to read it, fix every finding, and end its
// turn. The returned text carries no newline, the one-line shape validateSendText (run.go) requires.
func gateRepromptText(findingsPath string) string {
	return fmt.Sprintf("Gate findings recorded at %s — read it, fix every finding, and end your turn.", findingsPath)
}
