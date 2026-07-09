// result.go defines the block-level contract perchengine.Run reports to its
// caller: the three-way Outcome, the StuckReason recorded only alongside
// OutcomeStuck, and the per-round RoundSummary history that lets a caller
// (or an operator reading state.json) reconstruct exactly what happened
// each round without re-parsing every artifact file.

package perchengine

import "github.com/Knatte18/loomyard/internal/burlerengine"

// Outcome is the terminal classification of a perch block.
type Outcome string

// The three legal Outcome values. OutcomePaused is an operational exit —
// resumable, not judged — distinct from the judgment pair
// OutcomeApproved/OutcomeStuck.
const (
	OutcomeApproved Outcome = "APPROVED"
	OutcomeStuck    Outcome = "STUCK"
	OutcomePaused   Outcome = "PAUSED"
)

// StuckReason names why a block stopped with OutcomeStuck. It is set only
// when Outcome is OutcomeStuck; every other Outcome carries an empty
// StuckReason.
type StuckReason string

// The three legal StuckReason values.
const (
	// StuckHardCap fires when the final rung of RoundCaps is reached still
	// BLOCKING — unconditional, no judge call.
	StuckHardCap StuckReason = "hard-cap"
	// StuckMilestoneStop fires when the progress judge's milestone
	// continuation gate returns STOP at a non-final rung still BLOCKING.
	StuckMilestoneStop StuckReason = "milestone-stop"
	// StuckCircling fires when the progress judge's per-round circling
	// check returns CIRCLING, any round after the first BLOCKING one.
	StuckCircling StuckReason = "circling"
)

// RoundSummary records one round's outcome for Result.Rounds and for
// state.json's per-round history. Every field describes something that may
// or may not have happened that round: an empty or nil field means the
// corresponding sub-step did not occur this round (e.g. JudgePath is empty
// on round 1, where the judge never runs; GatePassed is nil for a round
// whose gate mode is GateLLMVerdict, which never runs a command).
type RoundSummary struct {
	// Round is the round number (1-based); Attempts is how many burler
	// attempts it took to reach a done outcome (>1 means a died/timeout
	// retry occurred before this round completed).
	Round    int
	Attempts int
	// Verdict and BlockingCount are the fresh round's burler review
	// result — the review the round loop reasons about, independent of
	// gate mode.
	Verdict       burlerengine.Verdict
	BlockingCount int
	// ReviewPath and FixerReportPath are always set for a completed round;
	// JudgePath, GatePath, and TriagePath are set only when the judge, the
	// command gate, or asking-triage actually ran that round.
	ReviewPath      string
	FixerReportPath string
	JudgePath       string
	GatePath        string
	// TriagePath is set when this round's burler attempt(s) included an
	// asking-triage call (an attempt stopped mid-round asking a question);
	// empty otherwise. There is no TriageVerdict field alongside it — a
	// GIVE_UP triage verdict never reaches this record at all, since it
	// surfaces as a hard ERROR from Engine.Run before any round record is
	// appended (see doc.go's non-done-outcomes section), so a persisted
	// TriagePath always implies the triage verdict was RETRY.
	TriagePath string
	// JudgeVerdict is the raw progress-judge verdict string (one of the
	// circling-check or milestone-gate vocabularies) when the judge ran
	// this round, empty otherwise.
	JudgeVerdict string
	// GatePassed is nil when the round's gate mode never runs a command
	// (GateLLMVerdict), and set to the command's pass/fail result
	// otherwise (GateCommand/GateBoth).
	GatePassed *bool
}

// Result is the block-level outcome perchengine.Run returns: the terminal
// Outcome, the StuckReason (set only alongside OutcomeStuck), how many
// rounds actually ran, and the full per-round history. PAUSED is an
// operational exit — resumable, not judged — so a caller must branch on
// Outcome before reading StuckReason.
type Result struct {
	Outcome     Outcome
	StuckReason StuckReason
	RoundsRun   int
	Rounds      []RoundSummary
}
