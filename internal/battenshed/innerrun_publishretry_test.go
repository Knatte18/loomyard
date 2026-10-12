// innerrun_publishretry_test.go covers the halted arm's one automatic retry of a child's first Publish verify failure, over fake seams and a fake clock:
// the goto and resume, the records it declines, the refusals it retries, its cap, and its restarts from the marker.

package battenshed

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const (
	retryPublishRow = "Publish"
	retryMergeInRow = "Merge-In"
	retryTarget     = "Webster"
	retryHead       = "abc123"
	// retryEarlier, retryHalting are the times of the history entries before the halt and of the halting entry.
	retryEarlier = "2026-01-01T11:00:00Z"
	retryHalting = "2026-01-01T11:30:00Z"
	// retryWritten is a record write time inside the halting attempt.
	retryWritten = "2026-01-01T11:20:00Z"
)

// retryHarness holds the fake child, the fake seams of the retry and what they recorded.
type retryHarness struct {
	t          *testing.T
	scratchDir string
	clock      *fakeClock
	producer   shedengine.ShedProducer

	status    shedengine.Status
	record    *ChildPublishFailure
	recordErr error
	head      string
	headErr   error
	// gotoErrs and spawnErrs are consumed one per call; a call past the end succeeds, except that an exhausted list ending in an error repeats it.
	gotoErrs    []error
	spawnErrs   []error
	gotoTargets []string
	spawns      int
	notices     []string
}

func nextErr(errs *[]error) error {
	if len(*errs) == 0 {
		return nil
	}
	err := (*errs)[0]
	if len(*errs) > 1 {
		*errs = (*errs)[1:]
	}
	return err
}

// newRetryHarness builds a producer over a child blocked at Publish with a matching record at retryHead.
func newRetryHarness(t *testing.T) *retryHarness {
	t.Helper()
	h := &retryHarness{
		t:          t,
		scratchDir: t.TempDir(),
		clock:      &fakeClock{},
		head:       retryHead,
		record:     &ChildPublishFailure{Row: retryPublishRow, Head: retryHead, Target: retryTarget, WrittenAt: retryWritten},
	}
	h.blockedAtPublish()
	_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{found: true}}, h.clock)
	deps.NoticeProbe = 30 * time.Second
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return h.status, true, nil }
	deps.VerifyRows = []string{retryMergeInRow, retryPublishRow}
	deps.PublishFailure = func() (ChildPublishFailure, bool, error) {
		if h.record == nil {
			return ChildPublishFailure{}, false, h.recordErr
		}
		return *h.record, true, h.recordErr
	}
	deps.ChildHead = func() (string, error) { return h.head, h.headErr }
	deps.Goto = func(_ context.Context, target string) error {
		h.gotoTargets = append(h.gotoTargets, target)
		return nextErr(&h.gotoErrs)
	}
	deps.Spawn = func(context.Context) error {
		h.spawns++
		return nextErr(&h.spawnErrs)
	}
	deps.Notify = func(_ context.Context, line string) (bool, error) {
		h.notices = append(h.notices, line)
		return true, nil
	}
	deps.PairDir = func() (string, error) { return "/pair", nil }
	h.producer = NewInnerRun("innerrun", "myslug", deps, time.Second, h.scratchDir, testGrace)
	return h
}

func (h *retryHarness) blockedAtPublish() {
	h.blockedAt(retryPublishRow)
}

// blockedAt halts the child at row, a verify row, with a Stuck entry at retryHalting.
func (h *retryHarness) blockedAt(row string) {
	h.status = shedengine.Status{
		State:           shedengine.StateBlocked,
		Error:           "verify failed",
		CurrentProducer: row,
		History: []shedengine.HistoryEntry{
			{Producer: "Plan-Write", Outcome: shedengine.Done, At: retryEarlier},
			{Producer: row, Outcome: shedengine.Stuck, At: retryHalting},
		},
	}
}

func (h *retryHarness) pausedAtTarget() {
	h.status = shedengine.Status{State: shedengine.StatePaused, CurrentProducer: retryTarget, History: h.status.History}
}

// call runs one Call and returns the Stuck path, requiring an exempt Stuck.
func (h *retryHarness) call() string {
	h.t.Helper()
	outcome, ptr, err := h.producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		h.t.Fatalf("Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	return ptr.Path
}

func (h *retryHarness) marker() string {
	raw, err := os.ReadFile(publishRetryFile(h.scratchDir, "innerrun"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (h *retryHarness) writeMarker(state string, started time.Time) {
	h.t.Helper()
	if err := os.WriteFile(publishRetryFile(h.scratchDir, "innerrun"), []byte(state+" "+started.UTC().Format(time.RFC3339)+" "+retryTarget+"\n"), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// wantMarkerState requires the marker to hold state as its first field.
func (h *retryHarness) wantMarkerState(state string) {
	h.t.Helper()
	if got, _, _ := strings.Cut(h.marker(), " "); got != state {
		h.t.Errorf("publish-retry marker = %q; want state %q", h.marker(), state)
	}
}

// wantGotos requires exactly n goto calls, all to the retry's target.
func (h *retryHarness) wantGotos(n int) {
	h.t.Helper()
	if len(h.gotoTargets) != n {
		h.t.Fatalf("goto calls = %v; want %d", h.gotoTargets, n)
	}
	for _, target := range h.gotoTargets {
		if target != retryTarget {
			h.t.Errorf("goto target = %q; want %q", target, retryTarget)
		}
	}
}

// wantOneNotice requires exactly one notice, containing every substring in want.
func (h *retryHarness) wantOneNotice(want ...string) {
	h.t.Helper()
	if len(h.notices) != 1 {
		h.t.Fatalf("notices = %q; want exactly one", h.notices)
	}
	for _, text := range want {
		if !strings.Contains(h.notices[0], text) {
			h.t.Errorf("notice = %q; want it to contain %q", h.notices[0], text)
		}
	}
}

// TestInnerRun_PublishRetrySendsTheChildToItsTargetAndResumesItOnce asserts a matching record sends the child to its target and resumes it, with no notice for the halt or the paused state,
// and that a second Publish halt in the run gets the blocked notice saying the retry already ran, with no second goto.
func TestInnerRun_PublishRetrySendsTheChildToItsTargetAndResumesItOnce(t *testing.T) {
	t.Parallel()
	h := newRetryHarness(t)

	if path := h.call(); !strings.Contains(path, "automatic Publish verify retry: sent the child to "+retryTarget) {
		t.Errorf("goto Call path = %q; want it to name the automatic retry and the target", path)
	}
	h.wantGotos(1)
	h.wantMarkerState(publishRetryGotoDone)

	h.pausedAtTarget()
	h.clock.advance(10 * time.Minute)
	if path := h.call(); !strings.Contains(path, "automatic Publish verify retry: resumed the child at "+retryTarget) {
		t.Errorf("resume Call path = %q; want it to name the automatic retry and the target", path)
	}
	if h.spawns != 1 {
		t.Errorf("Spawn calls = %d; want 1", h.spawns)
	}
	h.wantMarkerState(publishRetryResumed)
	if len(h.notices) != 0 {
		t.Fatalf("notices after the goto and the resume = %q; want none", h.notices)
	}

	h.blockedAtPublish()
	h.status.History = append(h.status.History, shedengine.HistoryEntry{Producer: retryPublishRow, Outcome: shedengine.Stuck, At: "2026-01-01T11:50:00Z"})
	h.call()
	h.wantGotos(1)
	h.wantOneNotice("blocked at "+retryPublishRow, "automatic Publish verify retry already ran")
}

// TestInnerRun_PublishRetryDeclinesRecordsThatDoNotMatchTheHalt asserts a halt gets no goto and the blocked notice when its record does not belong to it.
// A record is declined when it is missing, at another HEAD, from an earlier halt, written in the same second as the entry before the halting attempt, lacking its target, write time or row, naming another row than the halt, or unreadable.
// It also asserts a record written in the same second as the halting entry is retried, and so is a plan verify record at Merge-In.
// It stays serial: logcapture.Capture replaces the process-global logger output.
func TestInnerRun_PublishRetryDeclinesRecordsThatDoNotMatchTheHalt(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(h *retryHarness)
		wantGoto   bool
		wantWarned string
	}{
		{name: "NoRecord", configure: func(h *retryHarness) { h.record = nil }},
		{name: "AnotherHead", configure: func(h *retryHarness) { h.record.Head = "other" }},
		{name: "FromAnEarlierHalt", configure: func(h *retryHarness) { h.record.WrittenAt = "2026-01-01T10:00:00Z" }},
		{name: "SameSecondAsTheEntryBefore", configure: func(h *retryHarness) { h.record.WrittenAt = retryEarlier }},
		{name: "NoTarget", configure: func(h *retryHarness) { h.record.Target = "" }},
		{name: "NoWriteTime", configure: func(h *retryHarness) { h.record.WrittenAt = "" }},
		{name: "NoRow", configure: func(h *retryHarness) { h.record.Row = "" }},
		{name: "RowDiffersFromTheHalt", configure: func(h *retryHarness) { h.record.Row = retryMergeInRow }},
		{name: "RecordReadError", configure: func(h *retryHarness) { h.recordErr = errors.New("record unreadable") }, wantWarned: "record unreadable"},
		{name: "HeadReadError", configure: func(h *retryHarness) { h.headErr = errors.New("head unreadable") }, wantWarned: "head unreadable"},
		{name: "SameSecondAsTheHaltingEntryIsRetried", configure: func(h *retryHarness) { h.record.WrittenAt = retryHalting }, wantGoto: true},
		{name: "PlanVerifyRecordAtMergeInIsRetried", configure: func(h *retryHarness) {
			h.record.Row = retryMergeInRow
			h.blockedAt(retryMergeInRow)
		}, wantGoto: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			h := newRetryHarness(t)
			tt.configure(h)

			h.call()

			if tt.wantGoto {
				h.wantGotos(1)
				if len(h.notices) != 0 {
					t.Errorf("notices = %q; want none for a retried halt", h.notices)
				}
				return
			}
			h.wantGotos(0)
			if h.marker() != "" {
				t.Errorf("publish-retry marker = %q; want none", h.marker())
			}
			h.wantOneNotice("blocked at " + retryPublishRow)
			if strings.Contains(h.notices[0], "already ran") {
				t.Errorf("notice = %q; want no claim that a retry ran", h.notices[0])
			}
			if tt.wantWarned != "" && !strings.Contains(logs.String(), tt.wantWarned) {
				t.Errorf("log = %q; want a Warn naming %q", logs.String(), tt.wantWarned)
			}
		})
	}
}

// TestInnerRun_PublishRetryRetriesRefusalsUntilTheyLandOrTheCapIsReached asserts a goto refused as busy, a resume refused as not parked and a resume refused with a busy driver pane are retried until they land.
// The busy pane also sends the driver-busy notice once, and past the cap the notice for the child's actual state carries the last refusal.
// It also asserts a restart with an old marker does not reset the cap, and a later Publish halt after the cap gets the blocked notice and no goto.
func TestInnerRun_PublishRetryRetriesRefusalsUntilTheyLandOrTheCapIsReached(t *testing.T) {
	t.Parallel()
	t.Run("RefusalsAreRetriedUntilTheyLand", func(t *testing.T) {
		t.Parallel()
		h := newRetryHarness(t)
		h.gotoErrs = []error{ErrChildRunBusy, ErrChildRunBusy, nil}
		h.clock.pauseAtSleep = 100
		h.clock.onSleep = func(int) { h.clock.advance(time.Minute) }

		h.call()
		h.wantGotos(3)
		h.wantMarkerState(publishRetryGotoDone)

		h.pausedAtTarget()
		h.spawnErrs = []error{ErrChildNotParked, ErrChildDriverBusy, nil}
		h.clock.sleepCalls = 0
		h.call()
		if h.spawns != 3 {
			t.Errorf("Spawn calls = %d; want 3 (not parked, busy pane, landed)", h.spawns)
		}
		h.wantMarkerState(publishRetryResumed)
		h.wantOneNotice(noticeDriverBusyCondition)
	})

	t.Run("PastTheCapTheNoticeCarriesTheLastRefusal", func(t *testing.T) {
		t.Parallel()
		h := newRetryHarness(t)
		h.gotoErrs = []error{ErrChildRunBusy}
		h.clock.pauseAtSleep = 20
		h.clock.onSleep = func(int) { h.clock.advance(5 * time.Minute) }

		h.call()
		h.wantMarkerState(publishRetryCapped)
		h.wantOneNotice("blocked at "+retryPublishRow, "automatic Publish verify retry gave up", ErrChildRunBusy.Error())
	})

	t.Run("ARestartDoesNotResetTheCap", func(t *testing.T) {
		t.Parallel()
		h := newRetryHarness(t)
		h.writeMarker(publishRetryStarted, h.clock.Now().Add(-publishRetryCap-time.Minute))
		h.gotoErrs = []error{ErrChildRunBusy}
		h.clock.pauseAtSleep = 3

		h.call()
		h.wantGotos(0)
		h.wantMarkerState(publishRetryCapped)
	})

	t.Run("ALaterPublishHaltAfterTheCapGetsTheBlockedNoticeAndNoGoto", func(t *testing.T) {
		t.Parallel()
		h := newRetryHarness(t)
		h.writeMarker(publishRetryCapped, h.clock.Now().Add(-time.Hour))

		h.call()
		h.wantGotos(0)
		h.wantOneNotice("blocked at "+retryPublishRow, "automatic Publish verify retry already ran")
	})
}

// TestInnerRun_PublishRetryDoesNotRetryAGotoRefusedForAnotherReason asserts a goto refused for an unknown target is tried once, caps the retry and sends the blocked notice with the refusal.
func TestInnerRun_PublishRetryDoesNotRetryAGotoRefusedForAnotherReason(t *testing.T) {
	t.Parallel()
	h := newRetryHarness(t)
	h.gotoErrs = []error{errors.New(`goto target "Webster" names no producer in the list`)}
	h.clock.pauseAtSleep = 3

	h.call()
	h.wantGotos(1)
	h.wantMarkerState(publishRetryCapped)
	h.wantOneNotice("blocked at "+retryPublishRow, "automatic Publish verify retry gave up", "names no producer in the list")
}

// TestInnerRun_PublishRetryContinuesFromTheMarkerAfterARestart asserts a batten restarted after the goto and before the resume resumes the child, and one restarted after `started` and before the goto repeats the goto.
func TestInnerRun_PublishRetryContinuesFromTheMarkerAfterARestart(t *testing.T) {
	t.Parallel()
	t.Run("AfterTheGotoItResumes", func(t *testing.T) {
		t.Parallel()
		h := newRetryHarness(t)
		h.writeMarker(publishRetryGotoDone, h.clock.Now())
		h.pausedAtTarget()

		h.call()
		if h.spawns != 1 {
			t.Errorf("Spawn calls = %d; want 1", h.spawns)
		}
		h.wantGotos(0)
		h.wantMarkerState(publishRetryResumed)
	})

	t.Run("BeforeTheGotoItRepeatsTheGoto", func(t *testing.T) {
		t.Parallel()
		h := newRetryHarness(t)
		h.writeMarker(publishRetryStarted, h.clock.Now())
		h.record = nil

		h.call()
		h.wantGotos(1)
		h.wantMarkerState(publishRetryGotoDone)
	})
}
