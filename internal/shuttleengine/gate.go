// gate.go declares the gate contract: the exported types a caller uses to attach mechanical validators to a gated run and read its report (Gate, GateEntry, GateSpec, GateResult, GateOutcome, GateEntryOutcome, GateEntryState), plus the package-private helpers the run loop (run.go, wait.go) spends them through.
// It inherits the Shuttle Provider-Seam Invariant (doc.go): the gate loop asks "is a new event in"
// only through the existing Engine.ParseEvents seam, and knows nothing about any provider's own hook
// payloads.

package shuttleengine

import "fmt"

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

// Gate is a mechanical validator a gated run consults at each arrival and at its single verdict site (finalize):
// evaluate the run's declared output artifacts and report GateResult.
// It takes no argument, because the two production validators (planparser.ParsePlan,
// discussionparser's own validator) take materially different path shapes, and a closure captures
// its own told paths rather than this seam threading them through.
type Gate func() (GateResult, error)

// GateEntry is one gate in a GateSpec: a validator closure with its own re-prompt budget.
type GateEntry struct {
	// Name labels the entry in GateOutcome.Entries and in the re-prompt log line.
	Name string
	// Gate is the validator this entry consults.
	Gate Gate
	// Attempts is the entry's re-prompt budget: how many consecutive failures re-prompt the agent before the entry gives up.
	// 0 means the entry is off: it is skipped at every arrival, whatever its PassOnCap.
	Attempts int
	// PassOnCap lets the run through, rather than failing it, once the entry has failed Attempts consecutive times:
	// from then on the entry's closure is not run again and the entry is reported let through.
	// The count lives in memory for one shuttle run, so after an attach a capped entry starts from zero and can fire again,
	// and a pass before the cap resets the count, so the entry can fire again later;
	// a request that must not repeat makes its own closure idempotent.
	PassOnCap bool
}

// GateSpec is the ordered list of gate entries a gated run consults at each arrival — the one value every downstream seam (RunGated, AttachGated, a producer's RunOpts.Gate) carries from the point it is known through to the run loop, per the "one GateSpec at every hop" decision.
// An empty or nil list means "ungated", which is what every row but the gated ones supplies.
type GateSpec []GateEntry

// GateEntryState is one entry's state at the final arrival a GateOutcome reports.
type GateEntryState string

const (
	// GateEntryPassed marks an entry whose closure ran and passed.
	GateEntryPassed GateEntryState = "passed"
	// GateEntryFailed marks the entry whose closure ran and failed, stopping the evaluation.
	GateEntryFailed GateEntryState = "failed"
	// GateEntryLetThrough marks a PassOnCap entry past its cap, whose closure was not run.
	GateEntryLetThrough GateEntryState = "let_through"
	// GateEntryOff marks an entry with Attempts 0, wherever it sits in the list.
	GateEntryOff GateEntryState = "off"
	// GateEntryNotReached marks a non-off entry after the entry that stopped the evaluation.
	GateEntryNotReached GateEntryState = "not_reached"
)

// GateEntryOutcome is one entry's line in GateOutcome.Entries.
type GateEntryOutcome struct {
	// Name is the entry's GateEntry.Name.
	Name string
	// Attempts counts the re-prompts sent for this entry.
	Attempts int
	// State is the entry's state at the final arrival.
	State GateEntryState
}

// GateOutcome is a gated run's 1:1 gate report, carried on Result.Gate.
type GateOutcome struct {
	// Passed reports whether every entry that is neither off nor PassOnCap passed at the run's final arrival.
	Passed bool
	// Entries reports each entry in list order.
	Entries []GateEntryOutcome
	// Attempts counts re-prompts actually SENT on this run over all entries, and nothing else:
	// a gate that passed first try reports 0;
	// a Done reached with no live session reports however many re-prompts had already been sent before the session was lost (0 when it was lost before the first one);
	// a deadline that expires after N sends reports N.
	// See the "attempts counts re-prompts actually sent" decision.
	Attempts int
	// FindingsPath is the absolute path of the findings file the failing entry wrote, or empty when Passed.
	// It is diagnostic text ONLY, never a path to dereference after Wait returns: the
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
