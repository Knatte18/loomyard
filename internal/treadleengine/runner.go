// runner.go defines the RoundRunner seam Engine drives once per attempt,
// and the plain value types that cross it: AttemptInput (everything Engine assembles for one
// attempt) and AttemptResult (the shuttle-style outcome the runner reports back).
// The seam is deliberately attempt-level, not round-level: Engine (engine.go, run.go) owns the
// two-attempt retry policy, asking-triage, stale-artifact move-aside, round/attempt token naming,
// and prior-round hydration assembly, so a RoundRunner implementation only ever adapts "spawn one
// attempt, report its result" onto its own domain.

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

// AttemptInput contains everything Engine assembles for one RoundRunner.RunAttempt call: identity,
// paths, prior hydration, and per-round tuning.
// SeedPath is optional pre-round-targeting input, round-scoped not attempt-scoped.
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
	Model             string
	Effort            string
	Timeout           time.Duration
}

// AttemptResult is what one RoundRunner.RunAttempt call reports back: a shuttle-style Outcome and,
// when done, the verdict, blocking count, and artifact paths for Engine's round history.
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

// RoundRunner is the seam Engine drives once per attempt: RunAttempt runs one attempt of one round
// and reports its result or a hard error.
// Engine owns retry policy, triage, and stale-artifact move-aside;
// a RoundRunner implementation only adapts "spawn one attempt, report its result."
type RoundRunner interface {
	RunAttempt(AttemptInput) (AttemptResult, error)
}
