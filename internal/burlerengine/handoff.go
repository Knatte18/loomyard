// handoff.go joins a round's two halves: reviewReady turns the reviewer's done outcome into the ready marker that releases the fixer, and join waits on both halves and decides the round's one Result.

package burlerengine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrHalfNotStopped reports that a half of the round could not be stopped, so it may still be writing the round's files.
// The round never archives over or retries past it.
var ErrHalfNotStopped = errors.New("burler: a half of the round could not be stopped")

// reviewHandoff is what reviewReady took from a finished review: the bytes it parsed, the parse, and the cluster audit's warnings.
type reviewHandoff struct {
	content  []byte
	verdict  Verdict
	findings []Finding
	warnings []string
}

// reviewReady is the one step from the reviewer's done outcome to the ready marker.
// For a cluster round it audits the reviewer's forks, then it parses the review file strictly, and only then writes the marker, creating its parent directory.
// Any failure before the write leaves no marker, so the fixer is never released over a review that was not accepted.
// released is set immediately before the marker write begins, so it is true whenever the marker can be seen on disk.
func reviewReady(p *Profile, reviewer shuttleengine.Result, released *atomic.Bool) (reviewHandoff, error) {
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
	verdict, findings, err := ParseReview(content)
	if err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: round reached done but its review file is invalid: %w", err)
	}
	handoff.content, handoff.verdict, handoff.findings = content, verdict, findings

	if err := os.MkdirAll(filepath.Dir(p.ReadyMarkerPath), 0o755); err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: create the ready marker directory for %q: %w", p.ReadyMarkerPath, err)
	}
	released.Store(true)
	if err := os.WriteFile(p.ReadyMarkerPath, []byte("ready\n"), 0o644); err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: write ready marker %q: %w", p.ReadyMarkerPath, err)
	}
	return handoff, nil
}

// reviewerEnd is the reviewer's goroutine report: its terminal result and, when it reached done, the handoff.
type reviewerEnd struct {
	result     shuttleengine.Result
	waitErr    error
	handoff    reviewHandoff
	handoffErr error
}

// accepted reports whether the reviewer reached done and its review was handed to the fixer.
func (end reviewerEnd) accepted() bool {
	return end.waitErr == nil && end.result.Outcome == shuttleengine.OutcomeDone && end.handoffErr == nil
}

// fixerEnd is the fixer's goroutine report.
type fixerEnd struct {
	result shuttleengine.Result
	err    error
}

// finished reports whether the fixer reached done.
func (end fixerEnd) finished() bool {
	return end.err == nil && end.result.Outcome == shuttleengine.OutcomeDone
}

// halfOf names a half by the identity of its terminal result, falling back to the handle's own strand and run directory when the result carries none.
func halfOf(handle Handle, result shuttleengine.Result) Half {
	half := Half{
		SessionID:            result.SessionID,
		StrandGUID:           result.StrandGUID,
		LastAssistantMessage: result.LastAssistantMessage,
		RunDir:               result.RunDir,
	}
	if half.StrandGUID == "" {
		half.StrandGUID = handle.StrandGUID()
	}
	if half.RunDir == "" {
		half.RunDir = handle.RunDir()
	}
	return half
}

// stopHalf stops handle's strand through the engine's remover.
// A failed stop returns an error wrapping ErrHalfNotStopped whose message ends with the way forward.
func (e *Engine) stopHalf(handle Handle) error {
	guid := handle.StrandGUID()
	if err := e.remover.RemoveStrandIfLive(guid); err != nil {
		return fmt.Errorf("%w: strand %s: %v; way forward: run \"lyx reed remove %s\", then re-step the row", ErrHalfNotStopped, guid, err, guid)
	}
	return nil
}

// join waits on both started halves and decides the round.
// The reviewer's goroutine runs reviewReady once its run is done;
// the fixer's goroutine just waits.
// The first half to end decides which rules apply:
// a reviewer that did not finish with an accepted review stops the fixer and gives the reviewer's outcome or error;
// a fixer that died or timed out stops the reviewer and gives the fixer's outcome;
// a fixer that is done before the marker was released stops the reviewer and returns an error naming the skipped handoff.
// Whether the marker was released is the reviewer goroutine's recorded fact, set as the marker write begins, never the order the two results arrive in.
// Otherwise the round completes with the fixer's run:
// a failing fixer gate gives the fixer's Gate with Verdict and Findings empty,
// and a done fixer is accepted only when the review file still holds the bytes reviewReady parsed.
// A half that timed out is stopped as well, since shuttle keeps a timed-out run's strand live.
// join returns only after both goroutines have returned, so no half is left live and no marker write outlives the attempt;
// the one exception is a half it failed to stop, which it reports as ErrHalfNotStopped without waiting for.
func (e *Engine) join(p *Profile, opts RunOpts, review, fix Handle) (Result, error) {
	var markerReleased atomic.Bool
	reviewCh := make(chan reviewerEnd, 1)
	fixCh := make(chan fixerEnd, 1)

	go func() {
		var end reviewerEnd
		end.result, end.waitErr = review.Wait()
		if end.waitErr == nil && end.result.Outcome == shuttleengine.OutcomeDone {
			end.handoff, end.handoffErr = reviewReady(p, end.result, &markerReleased)
		}
		reviewCh <- end
	}()
	go func() {
		var end fixerEnd
		end.result, end.err = fix.Wait()
		fixCh <- end
	}()

	var reviewEnd reviewerEnd
	var fixEnd fixerEnd
	var stopErr error
	// fixerDecided is set when the fixer ended first without finishing, so the stopped reviewer's consequential outcome is not the round's.
	var fixerDecided, skippedHandoff bool

	// A half whose stop failed is still live, so its goroutine is not waited for: it would block until the run's own deadline.
	select {
	case reviewEnd = <-reviewCh:
		if !reviewEnd.accepted() {
			stopErr = e.stopHalf(fix)
		}
		if stopErr == nil {
			fixEnd = <-fixCh
		}
	case fixEnd = <-fixCh:
		switch {
		case !fixEnd.finished():
			fixerDecided = true
			stopErr = e.stopHalf(review)
		case !markerReleased.Load():
			skippedHandoff = true
			stopErr = e.stopHalf(review)
		}
		if stopErr == nil {
			reviewEnd = <-reviewCh
		}
	}

	// Shuttle keeps a timed-out run's strand, so a half that timed out is stopped too, or a retry would run beside it.
	for _, half := range []struct {
		handle Handle
		result shuttleengine.Result
	}{{review, reviewEnd.result}, {fix, fixEnd.result}} {
		if stopErr == nil && half.result.Outcome == shuttleengine.OutcomeTimeout {
			stopErr = e.stopHalf(half.handle)
		}
	}

	logger.Info("burler: round halves joined","round", opts.Round, "reviewOutcome", reviewEnd.result.Outcome, "fixOutcome", fixEnd.result.Outcome)

	result := Result{
		ReviewPath:      p.ReviewPath,
		FixerReportPath: p.FixerReportPath,
		Review:          halfOf(review, reviewEnd.result),
		Fix:             halfOf(fix, fixEnd.result),
	}
	if stopErr != nil {
		return result, stopErr
	}

	switch {
	case fixerDecided && fixEnd.err != nil:
		return result, fmt.Errorf("burler: shuttle run: %w", fixEnd.err)
	case fixerDecided:
		result.Outcome = fixEnd.result.Outcome
		return result, nil
	case skippedHandoff:
		result.Outcome = fixEnd.result.Outcome
		return result, fmt.Errorf("burler: the fixer reached done before the review was handed off: the ready marker %q was never written by the engine, so the fixer skipped its wait", p.ReadyMarkerPath)
	case reviewEnd.waitErr != nil:
		return result, fmt.Errorf("burler: shuttle run: %w", reviewEnd.waitErr)
	case reviewEnd.result.Outcome != shuttleengine.OutcomeDone:
		result.Outcome = reviewEnd.result.Outcome
		return result, nil
	case reviewEnd.handoffErr != nil:
		result.Outcome = reviewEnd.result.Outcome
		result.ForkAudit = reviewEnd.result.ForkAudit
		return result, reviewEnd.handoffErr
	case fixEnd.err != nil:
		return result, fmt.Errorf("burler: shuttle run: %w", fixEnd.err)
	case fixEnd.result.Outcome != shuttleengine.OutcomeDone:
		result.Outcome = fixEnd.result.Outcome
		return result, nil
	}

	result.Outcome = fixEnd.result.Outcome
	result.Gate = fixEnd.result.Gate
	result.ForkAudit = reviewEnd.result.ForkAudit
	result.ClusterWarnings = reviewEnd.handoff.warnings
	if result.Gate != nil && !result.Gate.Passed {
		// A failed fixer gate discredits the fixer's work, so the review that work answered is not reported as a verdict.
		return result, nil
	}

	now, err := os.ReadFile(p.ReviewPath)
	if err != nil {
		return result, fmt.Errorf("burler: read review file %q after the fixer finished: %w", p.ReviewPath, err)
	}
	if !bytes.Equal(now, reviewEnd.handoff.content) {
		return result, fmt.Errorf("burler: review file %q changed after it was handed off to the fixer; only the reviewer writes it", p.ReviewPath)
	}
	result.Verdict = reviewEnd.handoff.verdict
	result.Findings = reviewEnd.handoff.findings
	return result, nil
}
