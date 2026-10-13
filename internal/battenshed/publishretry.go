// publishretry.go implements the automatic retry of a child's first Publish verify failure in a batten run:
// the halted arm sends the child to the failure record's target row and resumes it, once, instead of waiting for the operator.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// publishRetryCap is how long after the retry started its goto and resume are retried before the retry gives up.
	publishRetryCap = 30 * time.Minute
	// publishRetryFileSuffix is the fixed suffix of the marker holding the retry's state, joined onto the producer's own name.
	publishRetryFileSuffix = "-publish-retry"
)

// The states the publish-retry marker records.
const (
	// publishRetryStarted is written before the goto.
	publishRetryStarted = "started"
	// publishRetryGotoDone is written after a goto that succeeded.
	publishRetryGotoDone = "goto-done"
	// publishRetryResumed is written after a resume that landed; it spends the run's one retry.
	publishRetryResumed = "resumed"
	// publishRetryCapped is written when the retry gave up; it spends the run's one retry.
	publishRetryCapped = "capped"
)

// publishRetryAction is what the halted arm does about the retry in this Call.
type publishRetryAction int

const (
	// publishRetryNone does nothing: no retry is to start or unfinished.
	publishRetryNone publishRetryAction = iota
	// publishRetryGoto sends the child to the target, starting the retry when the marker is absent.
	publishRetryGoto
	// publishRetryResume resumes the child paused at the target.
	publishRetryResume
)

// publishRetry is the decision about the automatic Publish retry for one Call's halted child.
type publishRetry struct {
	action publishRetryAction
	// target is the row the child is sent to.
	target string
	// started is when the retry started, from the marker.
	started time.Time
	// fresh is true while no marker exists yet, so the step writes `started` first.
	fresh bool
	// spent is true when the run's one retry has been used or given up on.
	spent bool
	// refusal is the last refusal the retry gave up on, cut to one line, empty when it gave up on none.
	refusal string
}

// publishRetryMarker is the decoded publish-retry marker.
type publishRetryMarker struct {
	state   string
	started time.Time
	target  string
}

// publishRetryFile returns the path of the publish-retry marker.
func publishRetryFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+publishRetryFileSuffix)
}

// readPublishRetryMarker decodes the marker, reporting found == false when it is absent.
// A marker that does not decode is an error.
func (p *innerRunProducer) readPublishRetryMarker() (publishRetryMarker, bool, error) {
	return readPublishRetryMarkerFile(publishRetryFile(p.scratchDir, p.name))
}

// PublishRetrySpent reports whether batten's publish-retry marker for producer exists in scratchDir in any state:
// a started, goto-done, resumed or capped retry each means batten took the run's one retry, so no one else retries it.
// An absent marker is false, and a marker that does not decode is an error.
// The read is only as current as batten's marker write, which batten makes before its goto.
func PublishRetrySpent(scratchDir, producer string) (bool, error) {
	_, found, err := readPublishRetryMarkerFile(publishRetryFile(scratchDir, producer))
	return found, err
}

// readPublishRetryMarkerFile decodes the marker at path, reporting found == false when it is absent.
// A marker that does not decode is an error.
func readPublishRetryMarkerFile(path string) (publishRetryMarker, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return publishRetryMarker{}, false, nil
	}
	if err != nil {
		return publishRetryMarker{}, false, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 3 {
		return publishRetryMarker{}, false, fmt.Errorf("malformed publish-retry marker %q", strings.TrimSpace(string(raw)))
	}
	started, err := time.Parse(time.RFC3339, fields[1])
	if err != nil {
		return publishRetryMarker{}, false, fmt.Errorf("malformed publish-retry marker time: %w", err)
	}
	return publishRetryMarker{state: fields[0], started: started, target: fields[2]}, true, nil
}

// writePublishRetryMarker records the retry's state, the time it started and its target.
func (p *innerRunProducer) writePublishRetryMarker(state string, started time.Time, target string) error {
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return fmt.Errorf("battenshed: %s: create scratch directory for publish-retry marker: %w", p.name, err)
	}
	content := fmt.Sprintf("%s %s %s\n", state, started.UTC().Format(time.RFC3339), target)
	if err := os.WriteFile(publishRetryFile(p.scratchDir, p.name), []byte(content), 0o644); err != nil {
		return fmt.Errorf("battenshed: %s: write publish-retry marker: %w", p.name, err)
	}
	return nil
}

// RecordMatchesHalt reports whether record belongs to the verify attempt that halted the child.
// It needs a target and a write time, the child's HEAD as the record's head, and a halting entry that is the child's last, a Stuck of the record's row, which is one of rows.
// A record naming no row, or another row than the halting entry's, never matches.
// The write time must be strictly later than the entry before the halting one, when there is one, and at or before the halting entry, all compared as whole-second UTC.
// A record from an earlier halt at the same HEAD fails the lower bound.
func RecordMatchesHalt(record ChildPublishFailure, head string, status shedengine.Status, rows []string) bool {
	if record.Target == "" || record.WrittenAt == "" || record.Head != head {
		return false
	}
	n := len(status.History)
	if n == 0 || status.History[n-1].Outcome != shedengine.Stuck {
		return false
	}
	if row := status.History[n-1].Producer; row != record.Row || !slices.Contains(rows, row) {
		return false
	}
	wrote, ok := wholeSecondUTC(record.WrittenAt)
	if !ok {
		return false
	}
	halted, ok := wholeSecondUTC(status.History[n-1].At)
	if !ok || wrote.After(halted) {
		return false
	}
	if n >= 2 {
		before, ok := wholeSecondUTC(status.History[n-2].At)
		if !ok || !wrote.After(before) {
			return false
		}
	}
	return true
}

// wholeSecondUTC parses an RFC 3339 time and truncates it to the second in UTC.
func wholeSecondUTC(text string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC().Truncate(time.Second), true
}

// publishRetryDecision decides, from the marker and the halted child's status, what the automatic Publish retry does in this Call.
// A seam or marker read error is warned about and leaves the halt to the usual handling.
func (p *innerRunProducer) publishRetryDecision(ctx context.Context, status shedengine.Status) publishRetry {
	if !p.retryOn {
		return publishRetry{}
	}
	marker, found, err := p.readPublishRetryMarker()
	if err != nil {
		logger.Warn("battenshed: publish-retry marker unreadable; leaving the halt to the operator", "producer", p.name, "slug", p.slug, "error", err)
		return publishRetry{}
	}
	blockedAtVerifyRow := status.State == shedengine.StateBlocked && slices.Contains(p.deps.VerifyRows, status.CurrentProducer)
	if !found {
		if !blockedAtVerifyRow {
			return publishRetry{}
		}
		goSpent, err := p.deps.GoVerifyRetrySpent()
		if err != nil {
			logger.Warn("battenshed: Go verify-retry records unreadable; leaving the halt to the usual handling", "producer", p.name, "slug", p.slug, "error", err)
			return publishRetry{}
		}
		if goSpent {
			return publishRetry{spent: true}
		}
		record, recorded, err := p.deps.PublishFailure()
		if err != nil {
			logger.Warn("battenshed: Publish failure record unreadable; leaving the halt to the operator", "producer", p.name, "slug", p.slug, "error", err)
			return publishRetry{}
		}
		if !recorded {
			return publishRetry{}
		}
		head, err := p.deps.ChildHead()
		if err != nil {
			logger.Warn("battenshed: child HEAD unreadable; leaving the halt to the operator", "producer", p.name, "slug", p.slug, "error", err)
			return publishRetry{}
		}
		if !RecordMatchesHalt(record, head, status, p.deps.VerifyRows) {
			return publishRetry{}
		}
		return publishRetry{action: publishRetryGoto, target: record.Target, fresh: true}
	}

	pausedAtTarget := status.State == shedengine.StatePaused && status.CurrentProducer == marker.target
	switch {
	case marker.state == publishRetryStarted && blockedAtVerifyRow:
		return publishRetry{action: publishRetryGoto, target: marker.target, started: marker.started}
	case (marker.state == publishRetryStarted || marker.state == publishRetryGotoDone) && pausedAtTarget:
		return publishRetry{action: publishRetryResume, target: marker.target, started: marker.started}
	}
	return publishRetry{spent: true}
}

// publishRetryStep carries out the retry the Call's decision calls for, and returns nil to go on with the usual halted-arm work.
// The goto and the resume are each retried at most once per notice probe while refused as busy or not parked, and the retrying stops publishRetryCap after the marker's start time.
func (p *innerRunProducer) publishRetryStep(ctx context.Context, w *childWait) *waitEnd {
	r := w.retry
	if r.action == publishRetryNone {
		return nil
	}
	now := p.deps.Now()
	if r.fresh {
		if err := p.writePublishRetryMarker(publishRetryStarted, now, r.target); err != nil {
			return hardEnd(err)
		}
		r.started, r.fresh = now, false
		w.retry = r
	}
	if now.Sub(r.started) >= publishRetryCap {
		return p.capPublishRetry(w, r)
	}
	if now.Before(w.publishRetryAt) {
		p.report(w, w.publishRetryReason)
		return nil
	}
	if r.action == publishRetryGoto {
		return p.publishRetryGoto(ctx, w, r)
	}
	return p.publishRetryResume(ctx, w, r)
}

// publishRetryGoto sends the child to the retry's target.
func (p *innerRunProducer) publishRetryGoto(ctx context.Context, w *childWait, r publishRetry) *waitEnd {
	logger.Info("battenshed: sending the child to the failed Publish's target", "producer", p.name, "slug", p.slug, "target", r.target)
	err := p.deps.Goto(ctx, r.target)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		w.publishRefusal = oneLine(err.Error(), noticeErrorMax)
		if !errors.Is(err, ErrChildRunBusy) {
			return p.capPublishRetry(w, r)
		}
		w.publishRetryAt = p.deps.Now().Add(p.noticeProbe)
		w.publishRetryReason = fmt.Sprintf("the automatic Publish verify retry waits to send the child to %s: its run lock is held; retrying (%s)", r.target, w.publishRefusal)
		p.report(w, w.publishRetryReason)
		return nil
	}
	if err := p.writePublishRetryMarker(publishRetryGotoDone, r.started, r.target); err != nil {
		return hardEnd(err)
	}
	return p.exemptEnd(fmt.Sprintf("automatic Publish verify retry: sent the child to %s", r.target))
}

// publishRetryResume resumes the child paused at the retry's target through its bootstrap.
func (p *innerRunProducer) publishRetryResume(ctx context.Context, w *childWait, r publishRetry) *waitEnd {
	logger.Info("battenshed: resuming the child at the failed Publish's target", "producer", p.name, "slug", p.slug, "target", r.target)
	err := p.deps.Spawn(ctx)
	logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		busy := errors.Is(err, ErrChildDriverBusy)
		if !busy && !errors.Is(err, ErrChildNotParked) {
			return hardEnd(fmt.Errorf("battenshed: %s: resume the child for the automatic Publish verify retry (resuming this run retries it): %w", p.name, err))
		}
		w.publishRefusal = oneLine(err.Error(), noticeErrorMax)
		w.publishRetryAt = p.deps.Now().Add(p.noticeProbe)
		w.publishRetryReason = fmt.Sprintf("the automatic Publish verify retry waits to resume the child at %s; retrying (%s)", r.target, w.publishRefusal)
		if busy {
			p.noticeDriverBusy(ctx, w, err)
		}
		p.report(w, w.publishRetryReason)
		return nil
	}
	p.driverBusyNoticed = false
	recordSpawnConfirmed(p.name, p.slug, p.scratchDir, SpawnConfirmedFile(p.scratchDir, p.name))
	p.openTerminalOnce(ctx)
	if err := p.writePublishRetryMarker(publishRetryResumed, r.started, r.target); err != nil {
		return hardEnd(err)
	}
	return p.exemptEnd(fmt.Sprintf("automatic Publish verify retry: resumed the child at %s", r.target))
}

// capPublishRetry gives the retry up: the marker becomes capped, spending the run's one retry, and the usual handling takes over, so the notice for the child's actual state goes with the last refusal.
func (p *innerRunProducer) capPublishRetry(w *childWait, r publishRetry) *waitEnd {
	logger.Warn("battenshed: the automatic Publish verify retry gave up", "producer", p.name, "slug", p.slug, "target", r.target, "refusal", w.publishRefusal)
	if err := p.writePublishRetryMarker(publishRetryCapped, r.started, r.target); err != nil {
		return hardEnd(err)
	}
	w.retry = publishRetry{spent: true, refusal: w.publishRefusal}
	return nil
}

// publishRetryNoticeText returns the words the state-changed notice of a halted child carries about the automatic retry, empty when it has none.
// A retry that gave up on a refusal names it, and a blocked-at-Publish child whose one retry is spent says the retry already ran.
func (p *innerRunProducer) publishRetryNoticeText(w *childWait) string {
	if w.retry.refusal != "" {
		return "automatic Publish verify retry gave up: " + w.retry.refusal
	}
	if w.retry.spent && w.status.State == shedengine.StateBlocked && slices.Contains(p.deps.VerifyRows, w.status.CurrentProducer) {
		return "automatic Publish verify retry already ran"
	}
	return ""
}
