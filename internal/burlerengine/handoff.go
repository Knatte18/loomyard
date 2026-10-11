// handoff.go decides a round from its seat result: reviewReady turns the reviewer's done outcome into the ready marker that releases the fixer, roundAcceptance runs it as the reviewer seat's accept hook, and concludeRound and decideRound turn the seats engine's answer into the round's one Result.

package burlerengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrHalfNotStopped reports that a seat of the round could not be stopped, so it may still be writing the round's files.
// It wraps the seats engine's seatengine.ErrSeatNotStopped, whose message ends with the way forward.
// The round never archives over or retries past it.
var ErrHalfNotStopped = errors.New("burler: a half of the round could not be stopped")

// reviewHandoff is what reviewReady took from a finished review: the bytes it parsed, the parse, and the cluster audit's warnings.
type reviewHandoff struct {
	content  []byte
	verdict  Verdict
	findings []Finding
	warnings []string
}

// reviewDigest returns the hex SHA-256 of a review file's bytes, the identity the fix half checks the review against.
func reviewDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// acceptReview accepts the review a done reviewer left at p.ReviewPath.
// For a cluster round it audits the reviewer's forks, then it parses the review file strictly with ParseCurrentReview.
// It touches nothing on disk.
func acceptReview(p *Profile, reviewer shuttleengine.Result) (reviewHandoff, error) {
	var handoff reviewHandoff
	if p.ClusterFan != "" {
		warnings, err := auditClusterRound(reviewer.ForkAudit, len(p.clusterLenses))
		if err != nil {
			return reviewHandoff{}, err
		}
		handoff.warnings = warnings
	}

	content, err := os.ReadFile(p.ReviewPath)
	if err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: read review file %q: %w", p.ReviewPath, err)
	}
	verdict, findings, err := ParseCurrentReview(content)
	if err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: round reached done but its review file is invalid: %w", err)
	}
	handoff.content, handoff.verdict, handoff.findings = content, verdict, findings
	return handoff, nil
}

// reviewReady is the one step from the reviewer's done outcome to the ready marker.
// It accepts the review through acceptReview, and only then writes the marker, creating its parent directory.
// Any failure before the write leaves no marker, so the fixer is never released over a review that was not accepted.
// released is set immediately before the marker write begins, so it is true whenever the marker can be seen on disk.
func reviewReady(p *Profile, reviewer shuttleengine.Result, released *atomic.Bool) (reviewHandoff, error) {
	handoff, err := acceptReview(p, reviewer)
	if err != nil {
		return reviewHandoff{}, err
	}

	if err := os.MkdirAll(filepath.Dir(p.ReadyMarkerPath), 0o755); err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: create the ready marker directory for %q: %w", p.ReadyMarkerPath, err)
	}
	released.Store(true)
	if err := os.WriteFile(p.ReadyMarkerPath, []byte("ready\n"), 0o644); err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: write ready marker %q: %w", p.ReadyMarkerPath, err)
	}
	return handoff, nil
}

// roundAcceptance is the reviewer seat's acceptance of one round: the released flag, set as the marker write begins, and the review it accepted.
type roundAcceptance struct {
	profile  *Profile
	released atomic.Bool

	mu      sync.Mutex
	handoff reviewHandoff
}

// accept is the reviewer seat's accept hook: it runs reviewReady on the reviewer's done result and keeps what it took.
// Any failure writes no marker and is returned as the seat's accept error.
func (a *roundAcceptance) accept(reviewer shuttleengine.Result) error {
	handoff, err := reviewReady(a.profile, reviewer, &a.released)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.handoff = handoff
	a.mu.Unlock()
	return nil
}

// accepted returns the review the acceptance took, the zero handoff when it accepted none.
func (a *roundAcceptance) accepted() reviewHandoff {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.handoff
}

// halfOf names a half by the identity of its terminal result, falling back to strandGUID and runDir when the result carries none.
func halfOf(result shuttleengine.Result, strandGUID, runDir string) Half {
	half := Half{
		SessionID:            result.SessionID,
		StrandGUID:           result.StrandGUID,
		LastAssistantMessage: result.LastAssistantMessage,
		RunDir:               result.RunDir,
		StartedAt:            result.StartedAt,
		EndedAt:              result.EndedAt,
		Usage:                result.Usage,
	}
	if half.StrandGUID == "" {
		half.StrandGUID = strandGUID
	}
	if half.RunDir == "" {
		half.RunDir = runDir
	}
	return half
}

// roundResult returns the part of the round's Result every ending shares: the two paths, both halves' identities and the lenses the reviewer ran.
func roundResult(p *Profile, seats seatengine.Result) Result {
	result := Result{
		ReviewPath:      p.ReviewPath,
		FixerReportPath: p.FixerReportPath,
		Fix:             halfOf(seats.Chair, "", ""),
	}
	if len(seats.Advisors) > 0 {
		reviewer := seats.Advisors[0]
		result.Review = halfOf(reviewer.Result, reviewer.StrandGUID, reviewer.RunDir)
	}
	for _, lens := range p.clusterLenses {
		result.Lenses = append(result.Lenses, lens.Name)
	}
	return result
}

// concludeRound turns the seats engine's answer, a seat result and the error it ran with, into the round's one Result.
// A seat that could not be stopped is returned wrapped in ErrHalfNotStopped.
// A chair that never started, recognised by the result holding no advisor, is a not-started round when its provider never came up and a wrapped start error otherwise.
// A chair whose wait errored is that error wrapped, unless the reviewer ended the step, in which case the reviewer's rules decide as for any other ending.
func (e *Engine) concludeRound(p *Profile, opts RunOpts, seats seatengine.Result, acceptance *roundAcceptance, runErr error) (Result, error) {
	switch {
	case runErr == nil:
	case errors.Is(runErr, seatengine.ErrSeatNotStopped):
		return roundResult(p, seats), fmt.Errorf("%w: %w", ErrHalfNotStopped, runErr)
	case len(seats.Advisors) == 0:
		if errors.Is(runErr, shuttleengine.ErrNotStarted) {
			return notStartedResult(p, Half{}, Half{StartError: runErr.Error()}), nil
		}
		return Result{}, fmt.Errorf("burler: shuttle run: %w", runErr)
	case !seats.Advisors[0].EndedStep:
		return roundResult(p, seats), fmt.Errorf("burler: shuttle run: %w", runErr)
	}
	return e.decideRound(p, opts, seats, acceptance)
}

// decideRound decides the round's one Result from the seat result: the reviewer's from its advisor-1 entry and the fixer's from the chair's result.
// It is the one place the round's failure rules live, in this order:
// a reviewer that failed to start is a not-started round, the fixer already stopped by the seats engine;
// a reviewer that ended the step gives its wait error, its accept error (the cluster audit or the strict parse) with its outcome and fork audit, its non-done outcome, or an error naming a failed review gate;
// a fixer that died or timed out gives the fixer's outcome;
// a fixer that is done while the review was never released gives an error naming the skipped handoff, the released flag being the recorded fact and never the order the ends arrived in;
// otherwise the fixer's run decides: its Gate passes through, a failing gate leaves Verdict and Findings empty,
// and a done fixer is accepted only when the review file still holds the bytes the acceptance parsed.
// The seats engine has stopped every seat before it returns, so no half is left live and no marker write outlives the attempt.
func (e *Engine) decideRound(p *Profile, opts RunOpts, seats seatengine.Result, acceptance *roundAcceptance) (Result, error) {
	reviewer := seats.Advisors[0]
	chair := seats.Chair

	logger.Info("burler: round seats joined", "round", opts.Round, "reviewOutcome", reviewer.Result.Outcome, "fixOutcome", chair.Outcome)

	if reviewer.StartError != "" {
		return notStartedResult(p, Half{StartError: reviewer.StartError}, Half{}), nil
	}

	result := roundResult(p, seats)
	if reviewer.EndedStep {
		switch {
		case reviewer.WaitErr != nil:
			return result, fmt.Errorf("burler: shuttle run: %w", reviewer.WaitErr)
		case reviewer.AcceptErr != nil:
			result.Outcome = reviewer.Result.Outcome
			result.ForkAudit = reviewer.Result.ForkAudit
			return result, reviewer.AcceptErr
		case reviewer.Result.Outcome != shuttleengine.OutcomeDone:
			result.Outcome = reviewer.Result.Outcome
			return result, nil
		}
		result.Outcome = reviewer.Result.Outcome
		reason := ""
		if gate := reviewer.Result.Gate; gate != nil {
			reason = gate.Reason
		}
		return result, fmt.Errorf("burler: the reviewer reached done but its review gate failed, so the round has no verdict: %s", reason)
	}

	switch {
	case chair.Outcome != shuttleengine.OutcomeDone:
		result.Outcome = chair.Outcome
		return result, nil
	case !acceptance.released.Load():
		result.Outcome = chair.Outcome
		return result, fmt.Errorf("burler: the fixer reached done before the review was handed off: the ready marker %q was never written by the engine, so the fixer skipped its wait", p.ReadyMarkerPath)
	}

	accepted := acceptance.accepted()
	result.Outcome = chair.Outcome
	result.Gate = chair.Gate
	result.ForkAudit = reviewer.Result.ForkAudit
	result.ClusterWarnings = accepted.warnings
	if result.Gate != nil && !result.Gate.Passed {
		// A failed fixer gate discredits the fixer's work, so the review that work answered is not reported as a verdict.
		return result, nil
	}

	now, err := os.ReadFile(p.ReviewPath)
	if err != nil {
		return result, fmt.Errorf("burler: read review file %q after the fixer finished: %w", p.ReviewPath, err)
	}
	if !bytes.Equal(now, accepted.content) {
		return result, fmt.Errorf("burler: review file %q changed after it was handed off to the fixer; only the reviewer writes it", p.ReviewPath)
	}
	result.Verdict = accepted.verdict
	result.Findings = accepted.findings
	return result, nil
}
