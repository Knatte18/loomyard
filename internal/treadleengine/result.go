// result.go defines the block-level outcome that Engine.Run returns.
// It holds the three-way Outcome, the StuckReason set only with OutcomeStuck, and the per-round history.
// The history lets an operator reading state.json reconstruct each round without re-parsing the artifact files.

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

// RoundSummary records one round's outcome.
// It holds the attempts, verdict, blocking count, artifact paths, and judge and gate results.
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
	JudgeVerdict    string
	GatePassed      *bool
}

// Result is the outcome of one treadle block.
// It holds the terminal Outcome and the StuckReason, which is set only with OutcomeStuck.
// It also holds the number of rounds run and the per-round history.
// PAUSED is an operational exit, resumable and not judged.
// A caller must check Outcome before it reads StuckReason.
type Result struct {
	Outcome     Outcome
	StuckReason StuckReason
	RoundsRun   int
	Rounds      []RoundSummary
}
