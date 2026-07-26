// runner.go defines the RoundRunner seam Engine drives once per attempt, and
// the plain value types that cross it: AttemptInput (everything Engine
// assembles for one attempt) and AttemptResult (the shuttle-style outcome
// the runner reports back). The seam is deliberately attempt-level, not
// round-level: Engine (engine.go, run.go) owns the two-attempt retry policy,
// asking-triage, stale-artifact move-aside, round/attempt token naming, and
// prior-round hydration assembly, so a RoundRunner implementation only ever
// adapts "spawn one attempt, report its result" onto its own domain.

package treadleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Verdict is treadle's own round-level judgment vocabulary — a generic
// approved/blocking classification a RoundRunner reports per attempt.
// Deliberately not burlerengine.Verdict: treadleengine never imports
// burlerengine (see the Treadle Runner-Seam Invariant in CONSTRAINTS.md), so
// a round-runner adapter maps its own domain's verdict onto this type
// itself.
type Verdict string

// The two legal Verdict values.
const (
	VerdictApproved Verdict = "APPROVED"
	VerdictBlocking Verdict = "BLOCKING"
)

// AttemptInput is everything Engine assembles for one RoundRunner.RunAttempt
// call: the run dir and round/attempt identity, RoundToken (the
// round/attempt artifact token, e.g. "3" or "3b" for a retry — a runner
// threads it through to its own domain's per-round naming, mirroring
// burlerengine.RunOpts.Round today), this attempt's own review/fixer-report
// output paths, the hydration lists accumulated from every already-completed
// round, and per-round tuning. SeedPath is optional pre-round-targeting
// input: empty when Profile.PreRoundTargeting is off, or when it is on but
// that round's targeting call produced no seed (no valid handoff yet, or a
// fail-safe targeting failure) — a RoundRunner MAY ignore it entirely (perch's
// burler adapter does, per the burler-hydration-unchanged shared decision).
// SeedPath is round-scoped, not attempt-scoped: both attempts of the same
// round (a fresh attempt and its retry) see the identical path.
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

// AttemptResult is what one RoundRunner.RunAttempt call reports back: a
// shuttle-style Outcome (done/asking/died/timeout — Engine branches on it
// exactly like it branches on a raw shuttleengine.Outcome today), and, once
// Outcome is shuttleengine.OutcomeDone, the round's Verdict and
// BlockingCount plus the artifact paths and identities Engine folds into its
// round history. ReviewPath/FixerReportPath echo AttemptInput's paths back
// rather than inventing new ones; SessionID, LastAssistantMessage, and RunDir
// feed Engine's retry/triage/error-diagnosis machinery exactly like
// burlerengine.Result's equivalent fields do today.
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

// RoundRunner is the seam Engine drives once per attempt: RunAttempt runs
// ONE attempt of one round — a single review/fix pass over in — and reports
// its AttemptResult or a hard error. Engine owns every piece of generic
// machinery around this call: the two-attempt retry policy, asking-triage,
// stale-artifact move-aside before a re-run, round/attempt token naming
// (roundToken), and the prior-round hydration lists threaded into each
// AttemptInput (collectPriorHydration). A RoundRunner implementation
// therefore only ever needs to adapt "spawn one attempt, report its result"
// onto its own domain (burlerengine today, via perchengine's adapter) —
// never re-implement retry or triage itself.
type RoundRunner interface {
	RunAttempt(AttemptInput) (AttemptResult, error)
}
