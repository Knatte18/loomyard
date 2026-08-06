// result.go defines the block-level contract perchengine.Run reports to its caller: the three-way
// Outcome, the StuckReason recorded only alongside OutcomeStuck, and the per-round RoundSummary
// history that lets a caller (or an operator reading state.json) reconstruct exactly what happened
// each round without re-parsing every artifact file.

package perchengine

import "github.com/Knatte18/loomyard/internal/burlerengine"

// Outcome is the terminal classification of a perch block.
type Outcome string

// The three legal Outcome values.
// OutcomePaused is an operational exit — resumable, not judged — distinct from the judgment pair
// OutcomeApproved/OutcomeStuck.
const (
	OutcomeApproved Outcome = "APPROVED"
	OutcomeStuck    Outcome = "STUCK"
	OutcomePaused   Outcome = "PAUSED"
)

// StuckReason names why a block stopped with OutcomeStuck.
// It is set only when Outcome is OutcomeStuck;
// every other Outcome carries an empty StuckReason.
type StuckReason string

// The three legal StuckReason values.
const (
	// StuckHardCap fires when the final rung of RoundCaps is reached still BLOCKING — unconditional,
	// no judge call.
	StuckHardCap StuckReason = "hard-cap"
	// StuckMilestoneStop fires when the progress judge's milestone continuation gate returns STOP at a
	// non-final rung still BLOCKING.
	StuckMilestoneStop StuckReason = "milestone-stop"
	// StuckCircling fires when the progress judge's per-round circling check returns CIRCLING, any
	// round after the first BLOCKING one.
	StuckCircling StuckReason = "circling"
)

// RoundSummary records one round's outcome for Result.Rounds and state.json's per-round history.
type RoundSummary struct {
	// Round is the round number (1-based); Attempts is how many burler attempts it took to reach a done outcome.
	Round    int
	Attempts int
	// Verdict and BlockingCount are the fresh round's burler review result.
	Verdict       burlerengine.Verdict
	BlockingCount int
	// ReviewPath and FixerReportPath are always set; JudgePath, GatePath, and TriagePath are set only when the judge, gate, or triage actually ran.
	ReviewPath      string
	FixerReportPath string
	JudgePath       string
	GatePath        string
	// TriagePath is set when this round's burler attempt(s) included an asking-triage call; empty otherwise. A GIVE_UP triage verdict never reaches this record at all; it surfaces as an error from Engine.Run before any round record is appended.
	TriagePath string
	// JudgeVerdict is the raw progress-judge verdict string when the judge ran this round, empty otherwise.
	JudgeVerdict string
	// GatePassed is nil when the round's gate mode never runs a command (GateLLMVerdict), otherwise the command's pass/fail result.
	GatePassed *bool
}

// Result is the block-level outcome perchengine.Run returns: the terminal Outcome, the StuckReason
// (set only alongside OutcomeStuck), how many rounds actually ran, and the full per-round history.
// PAUSED is an operational exit — resumable, not judged — so a caller must branch on Outcome before
// reading StuckReason.
type Result struct {
	Outcome     Outcome
	StuckReason StuckReason
	RoundsRun   int
	Rounds      []RoundSummary
}
