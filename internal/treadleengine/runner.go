// runner.go defines the RoundRunner seam that Engine drives once per attempt, and the value types that cross it.
// AttemptInput carries everything Engine assembles for one attempt.
// AttemptResult is the shuttle-style outcome the runner reports back.
// Engine owns the two-attempt retry policy and stale-artifact move-aside.
// It also owns round and attempt token naming and prior-round hydration assembly.

package treadleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Verdict is treadle's round-level judgment vocabulary: approved or blocking.
type Verdict string

// The two legal Verdict values.
const (
	VerdictApproved Verdict = "APPROVED"
	VerdictBlocking Verdict = "BLOCKING"
)

// AttemptInput is the input to one attempt of a round:
// its identity, artifact paths, prior-round hydration, and per-round tuning.
// SeedPath is optional input from pre-round targeting; it is set once per round, not per attempt.
type AttemptInput struct {
	RunDir            string
	Round             int
	Attempt           int
	RoundToken        string
	ReviewPath        string
	FixerReportPath   string
	SeedPath          string
	PriorReviews      []string
	PriorFixerReports []string
	Choice            shuttleengine.ModelChoice
	Timeout           time.Duration
}

// AttemptResult is the outcome of one attempt.
// It carries a shuttle-style Outcome.
// When it is `OutcomeDone`, the verdict, blocking count, and artifact paths are set for the round history.
type AttemptResult struct {
	Outcome              shuttleengine.Outcome
	Verdict              Verdict
	BlockingCount        int
	ReviewPath           string
	FixerReportPath      string
	SessionID            string
	LastAssistantMessage string
	RunDir               string
}

// RoundRunner runs one attempt of one round and reports its result or a hard error.
// The caller owns retry policy and stale-artifact move-aside;
// an implementation only adapts "spawn one attempt, report its result" onto its own domain.
type RoundRunner interface {
	RunAttempt(AttemptInput) (AttemptResult, error)
}
