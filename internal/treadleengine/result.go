// result.go defines the block-level contract Engine.Run reports to its caller: the three-way
// Outcome, the StuckReason recorded only alongside OutcomeStuck, and the per-round RoundSummary
// history that lets a caller (or an operator reading state.json) reconstruct exactly what happened
// each round without re-parsing every artifact file.

package treadleengine

// Outcome is the terminal classification of a treadle block.
type Outcome string

// The three legal Outcome values.
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
	StuckHardCap       StuckReason = "hard-cap"
	StuckMilestoneStop StuckReason = "milestone-stop"
	StuckCircling      StuckReason = "circling"
)

// RoundSummary records one round's outcome for the result and state.json.
// Empty or nil fields mean the corresponding sub-step did not occur.
type RoundSummary struct {
	Round           int
	Attempts        int
	Verdict         Verdict
	BlockingCount   int
	ReviewPath      string
	FixerReportPath string
	JudgePath       string
	GatePath        string
	TriagePath      string
	JudgeVerdict    string
	GatePassed      *bool
}

// Result is the block-level outcome Engine.Run returns: the terminal Outcome, the StuckReason (set
// only alongside OutcomeStuck), how many rounds actually ran, and the full per-round history.
// PAUSED is an operational exit — resumable, not judged — so a caller must branch on Outcome before
// reading StuckReason.
type Result struct {
	Outcome     Outcome
	StuckReason StuckReason
	RoundsRun   int
	Rounds      []RoundSummary
}
