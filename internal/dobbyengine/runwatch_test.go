// runwatch_test.go pins the run watch over a fake level, child reading and post:
// which changes owe a line, what the line says, what the memory keeps and how owed lines are delivered.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// watchWritten is when batten's status of the watched run was written.
var watchWritten = time.Date(2026, 10, 25, 11, 0, 0, 0, time.UTC)

// watchFake is the seams of one run watch test, and the harness that ticks over them.
type watchFake struct {
	t *testing.T
	h *passHarness

	level    string
	levelErr error

	// child is the child reading, childMissing reports no status file, and childErr is a read error.
	// childReads counts the reads.
	child        ChildReading
	childMissing bool
	childErr     error
	childReads   int
	// childWritten is the modification time of the child status, which the next listed view carries.
	childWritten time.Time

	// noOrch answers that no orch strand is recorded, postErr fails the post, and postCalls counts the posts.
	noOrch    bool
	postErr   error
	postCalls int
	lines     []string
}

func newWatchFake(t *testing.T) *watchFake {
	t.Helper()
	w := &watchFake{t: t, h: newPassHarness(t), level: RunWatchStops, childWritten: watchWritten}
	w.h.d.deps.RunNotices.ReadChild = func(string) (ChildReading, bool, error) {
		w.childReads++
		return w.child, !w.childMissing, w.childErr
	}
	w.h.d.deps.RunWatch = RunWatchDeps{
		Level: func(string) (string, error) { return w.level, w.levelErr },
		Post: func(_ context.Context, _, line string) (bool, error) {
			w.postCalls++
			if w.postErr != nil || w.noOrch {
				return false, w.postErr
			}
			w.lines = append(w.lines, line)
			return true, nil
		},
	}
	w.writeChild("running", "Plan-Write")
	return w
}

// writeChild writes the child's status with state at row, one second after the last write.
func (w *watchFake) writeChild(state, row string, change ...func(*ChildReading)) {
	w.childWritten = w.childWritten.Add(time.Second)
	w.child = ChildReading{State: state, CurrentProducer: row, Modified: w.childWritten, PairDir: "/wt/task"}
	for _, apply := range change {
		apply(&w.child)
	}
}

// view is the listing of the running run task, carrying the child status's write time, with change applied.
func (w *watchFake) view(change ...func(*RunView)) RunView {
	run := runningRun("task")
	run.StatusModified, run.ChildStatusModified = watchWritten, w.childWritten
	for _, apply := range change {
		apply(&run)
	}
	return run
}

// tick lists the run with change applied and runs one tick.
func (w *watchFake) tick(change ...func(*RunView)) {
	w.t.Helper()
	w.h.fake.runs = []RunView{w.view(change...)}
	w.h.tick(false)
}

// tickUnlisted runs one tick that lists no run.
func (w *watchFake) tickUnlisted() {
	w.t.Helper()
	w.h.fake.runs = nil
	w.h.tick(false)
}

// landNotice makes the landing notice read on for the run.
func (w *watchFake) landNotice() {
	w.h.d.deps.LandNotice = func(string) (bool, error) { return true, nil }
}

func (w *watchFake) memory() (RunWatchState, bool) {
	w.t.Helper()
	watch, found := w.h.stateRecord().Watches["task"]
	return watch, found
}

func landedView(run *RunView) {
	run.State, run.Run, run.Producer, run.PastLand, run.Landed = runStateDone, runfsm.Landed, "Done", true, true
}

func landingView(run *RunView) {
	run.Run, run.Producer, run.PastLand = runfsm.Landing, "Land", true
}

func TestRunWatch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		head    = "[run watch] task: running; batten running at Run-Shed; child "
		landed  = "[run watch] task: landed; batten done at Done; child running at Plan-Write"
		blocked = head + "blocked at Plan-Write"
	)

	tests := []struct {
		name string
		act  func(t *testing.T, w *watchFake)
		// want is every line posted, in order, each without its closing since clause unless exact is set.
		want  []string
		exact bool
	}{
		{
			name: "AChildBlockedAtARowPostsItsLineOnce",
			act: func(t *testing.T, w *watchFake) {
				logged := w.h.recordLog()
				w.tick()
				w.writeChild("blocked", "Plan-Write", func(c *ChildReading) { c.Error = "needs a decision" })
				w.tick()
				w.tick()
				want := "run watch task: " + blocked + "; needs a decision; since 2026-10-25T11:00:02Z"
				if !slices.Contains(*logged, want) {
					t.Errorf("log lines = %q; want %q among them", *logged, want)
				}
			},
			want:  []string{blocked + "; needs a decision; since 2026-10-25T11:00:02Z"},
			exact: true,
		},
		{
			name: "AFoldedRepeatOfABlockAtTheSameRowPostsASecondLine",
			act: func(_ *testing.T, w *watchFake) {
				w.writeChild("blocked", "Plan-Write", func(c *ChildReading) { c.HistoryLen = 3 })
				w.tick()
				w.writeChild("blocked", "Plan-Write", func(c *ChildReading) { c.HistoryLen, c.LastRepeats = 3, 1 })
				w.tick()
			},
			want: []string{blocked, blocked},
		},
		{
			name: "AChildChangedWithNoStatusWriteIsNotRead",
			act: func(t *testing.T, w *watchFake) {
				w.tick()
				reads := w.childReads
				w.child.State = "blocked"
				w.tick()
				if w.childReads != reads {
					t.Errorf("ReadChild calls = %d; want %d, none without a status write", w.childReads, reads)
				}
				w.child.State = "running"
				w.tick(func(r *RunView) { r.StatusModified = watchWritten.Add(time.Minute) })
				if w.childReads != reads+1 {
					t.Errorf("ReadChild calls = %d; want %d after batten's status write", w.childReads, reads+1)
				}
			},
		},
		{
			name: "AChildBlockedAgainWithALongerHistoryPostsASecondLine",
			act: func(_ *testing.T, w *watchFake) {
				w.writeChild("blocked", "Plan-Write", func(c *ChildReading) { c.HistoryLen = 3 })
				w.tick()
				w.writeChild("blocked", "Plan-Write", func(c *ChildReading) { c.HistoryLen = 4 })
				w.tick()
			},
			want: []string{blocked, blocked},
		},
		{
			name: "BattenBlockedAgainAtTheSameRowPostsASecondLine",
			act: func(_ *testing.T, w *watchFake) {
				w.tick(func(r *RunView) { r.State, r.HistoryLen = "blocked", 1 })
				w.tick(func(r *RunView) { r.State, r.HistoryLen = "blocked", 2 })
				w.tick(func(r *RunView) { r.State, r.HistoryLen, r.LastRepeats = "blocked", 2, 1 })
			},
			want: []string{
				"[run watch] task: running; batten blocked at Run-Shed; child running at Plan-Write",
				"[run watch] task: running; batten blocked at Run-Shed; child running at Plan-Write",
				"[run watch] task: running; batten blocked at Run-Shed; child running at Plan-Write",
			},
		},
		{
			name: "ARewriteOfOnlyTheTimeAndReasonOwesNoLineUnderRows",
			act: func(_ *testing.T, w *watchFake) {
				w.level = RunWatchRows
				w.tick()
				w.writeChild("running", "Plan-Write", func(c *ChildReading) { c.Error = "a new reason" })
				w.tick()
			},
			want: []string{head + "running at Plan-Write"},
		},
		{
			name: "AChildWrittenAfterTheListingWaitsForTheNextTick",
			act: func(t *testing.T, w *watchFake) {
				w.tick()
				// The listing sees a write the watch has not read, and the child is written again after it.
				w.writeChild("running", "Plan-Write")
				listed := w.childWritten
				w.writeChild("blocked", "Plan-Write")
				w.tick(func(r *RunView) { r.ChildStatusModified = listed })
				if len(w.lines) != 0 {
					t.Errorf("lines over the older listing = %q; want none", w.lines)
				}
				w.tick(func(r *RunView) { r.Run = runfsm.Halted })
			},
			want: []string{"[run watch] task: halted; batten running at Run-Shed; child blocked at Plan-Write"},
		},
		{
			name: "AFirstReadingOverAnOlderListingKeepsNone",
			act: func(t *testing.T, w *watchFake) {
				listed := w.childWritten
				w.writeChild("blocked", "Plan-Write")
				w.tick(func(r *RunView) { r.Run, r.ChildStatusModified = runfsm.Halted, listed })
				if watch, _ := w.memory(); watch.Last != nil {
					t.Errorf("memory = %+v; want no reading", watch.Last)
				}
				w.tick(func(r *RunView) { r.Run = runfsm.Halted })
			},
			want: []string{"[run watch] task: halted; batten running at Run-Shed; child blocked at Plan-Write"},
		},
		{
			name: "AFailedChildReadKeepsTheLastChildAndReadsAgainNextTick",
			act: func(t *testing.T, w *watchFake) {
				w.tick()
				w.writeChild("blocked", "Plan-Write")
				w.childErr = errors.New("unreadable")
				w.tick()
				if len(w.lines) != 0 {
					t.Errorf("lines after the failed read = %q; want none", w.lines)
				}
				reads := w.childReads
				w.childErr = nil
				w.tick()
				if w.childReads != reads+1 {
					t.Errorf("ReadChild calls = %d; want %d after the recovery", w.childReads, reads+1)
				}
			},
			want: []string{blocked},
		},
		{
			name: "AReasonIsRelativeToThePairAndOnOneLine",
			act: func(_ *testing.T, w *watchFake) {
				w.childMissing = true
				w.tick(func(r *RunView) { r.State, r.Error = "blocked", "no seed in /wt/task/_lym/seed.yaml" })
				w.childMissing = false
				w.writeChild("blocked", "Plan-Write", func(c *ChildReading) { c.Error = "stuck on /wt/task/_lym/x.md\nagain" })
				w.tick()
			},
			want: []string{
				"[run watch] task: running; batten blocked at Run-Shed; child none; no seed in _lym/seed.yaml",
				blocked + "; stuck on _lym/x.md again",
			},
		},
		{
			name: "LearningThePairDirectoryAloneOwesNoLine",
			act: func(_ *testing.T, w *watchFake) {
				w.level, w.childMissing, w.child.PairDir = RunWatchRows, true, ""
				w.tick()
				w.writeChild("running", "Plan-Write")
				w.tick()
			},
			want: []string{"[run watch] task: running; batten running at Run-Shed; child none"},
		},
		{
			name: "AChildDoneAndACancellingRunAreStops",
			act: func(_ *testing.T, w *watchFake) {
				w.tick()
				w.writeChild("done", "Land")
				w.tick()
				w.tick(func(r *RunView) { r.Run = runfsm.Cancelling })
			},
			want: []string{head + "done at Land", "[run watch] task: cancelling; batten running at Run-Shed; child done at Land"},
		},
		{
			name: "BattenBlockedAtLandOverADoneChildPostsItsLine",
			act: func(_ *testing.T, w *watchFake) {
				refused := func(r *RunView) { r.State, r.Producer, r.Error = "blocked", "Land", "the land refused" }
				w.writeChild("done", "Land")
				w.tick(refused)
				w.writeChild("blocked", "Land", func(c *ChildReading) { c.Error = "the child's own reason" })
				w.tick(refused)
			},
			want: []string{
				"[run watch] task: running; batten blocked at Land; child done at Land; the land refused",
				"[run watch] task: running; batten blocked at Land; child blocked at Land; the child's own reason",
			},
		},
		{
			name: "ARunWithNoChildStatusPostsNothingWhileRunning",
			act: func(_ *testing.T, w *watchFake) {
				w.childMissing = true
				w.tick()
				w.tick()
			},
		},
		{
			name: "StopsTellsOnlyTheStops",
			act:  watchRowMoves,
			want: []string{head + "blocked at Webster", head + "done at Land"},
		},
		{
			name: "RowsTellsEveryChange",
			act: func(t *testing.T, w *watchFake) {
				w.level = RunWatchRows
				watchRowMoves(t, w)
			},
			want: []string{
				head + "running at Plan-Write",
				head + "running at Webster",
				head + "blocked at Webster",
				head + "running at Webster",
				head + "done at Land",
				"[run watch] task: running; batten running at Land; child done at Land",
			},
		},
		{
			name: "OffTellsNothing",
			act: func(t *testing.T, w *watchFake) {
				w.level = RunWatchOff
				watchRowMoves(t, w)
			},
		},
		{
			name: "AChildGoingFromBlockedToFailedAndAPauseLiftingOverItEachPost",
			act: func(_ *testing.T, w *watchFake) {
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.writeChild("failed", "Plan-Write")
				w.tick()
				w.tick(func(r *RunView) { r.State, r.Run = runStatePaused, runfsm.Paused })
				w.tick()
			},
			want: []string{
				blocked,
				head + "failed at Plan-Write",
				"[run watch] task: paused; batten paused at Run-Shed; child failed at Plan-Write",
				head + "failed at Plan-Write",
			},
		},
		{
			name: "ALevelThatNeverReadKeepsNoReadingAndTellsTheStandingStopOnceItReads",
			act: func(t *testing.T, w *watchFake) {
				w.levelErr = errors.New("unreadable")
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				if watch, _ := w.memory(); watch.Last != nil || len(w.lines) != 0 {
					t.Errorf("memory = %+v, lines = %q; want no reading and no line", watch.Last, w.lines)
				}
				w.levelErr = nil
				w.tick()
			},
			want: []string{blocked},
		},
		{
			name: "AFailedLevelReadKeepsTheLastLevel",
			act: func(_ *testing.T, w *watchFake) {
				w.level = RunWatchRows
				w.tick()
				w.level, w.levelErr = "", errors.New("unreadable")
				w.writeChild("running", "Webster")
				w.tick()
			},
			want: []string{head + "running at Plan-Write", head + "running at Webster"},
		},
		{
			name: "AStandingAnchorChangeHoldsTheLine",
			act: func(t *testing.T, w *watchFake) {
				record := filepath.Join(t.TempDir(), "move")
				if err := os.WriteFile(record, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				w.h.d.geom.MoveRecords = []string{record}
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				if len(w.lines) != 0 {
					t.Errorf("lines under the anchor change = %q; want none", w.lines)
				}
				if err := os.Remove(record); err != nil {
					t.Fatal(err)
				}
				w.tick()
			},
			want: []string{blocked},
		},
		{
			name: "AnUnlistedRunWithNoStatusAndNothingOwedIsForgotten",
			act: func(t *testing.T, w *watchFake) {
				w.tick()
				w.tickUnlisted()
				if _, found := w.memory(); found {
					t.Errorf("memory of the gone run is kept; want it dropped")
				}
			},
		},
		{
			name: "AnUnlistedRunThatStillHasItsStatusIsRemembered",
			act: func(t *testing.T, w *watchFake) {
				w.tick()
				w.h.fake.statuses["task"] = true
				w.tickUnlisted()
				if _, found := w.memory(); !found {
					t.Errorf("memory of the run with a status is dropped; want it kept")
				}
			},
		},
		{
			name: "AnUnlistedRunWhoseStatusCheckFailsIsRemembered",
			act: func(t *testing.T, w *watchFake) {
				w.tick()
				w.h.d.deps.StatusExists = func(string) (bool, error) { return false, errors.New("unreadable") }
				w.tickUnlisted()
				if _, found := w.memory(); !found {
					t.Errorf("memory of the run whose status check failed is dropped; want it kept")
				}
			},
		},
		{
			name: "AnUnlistedRunOwingALineIsRemembered",
			act: func(t *testing.T, w *watchFake) {
				w.noOrch = true
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.tickUnlisted()
				if _, found := w.memory(); !found {
					t.Errorf("memory of the run owing a line is dropped; want it kept")
				}
			},
		},
		{
			name: "ARestartedDobbyTellsNoReadingTwice",
			act: func(_ *testing.T, w *watchFake) {
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.h.restartDobby()
				w.tick()
			},
			want: []string{blocked},
		},
		{
			name: "ALandedRunWithLandNoticeOffPostsOnceAndAVanishedChildStatusPostsNothing",
			act: func(_ *testing.T, w *watchFake) {
				w.tick()
				w.tick(landedView)
				w.childMissing, w.childWritten = true, time.Time{}
				w.tick(landedView)
				w.tick(landedView)
			},
			want: []string{landed},
		},
		{
			name: "ALandedRunWhoseLandNoticeNeverReadPostsItsLine",
			act: func(_ *testing.T, w *watchFake) {
				w.h.d.deps.LandNotice = func(string) (bool, error) { return false, errors.New("unreadable") }
				w.tick()
				w.tick(landedView)
			},
			want: []string{landed},
		},
		{
			name: "ALandedRunWithLandNoticeOnPostsNoWatchLine",
			act: func(_ *testing.T, w *watchFake) {
				w.landNotice()
				w.tick()
				w.tick(landedView)
			},
		},
		{
			name: "ACancelledRunWithLandNoticeOnPostsItsLine",
			act: func(_ *testing.T, w *watchFake) {
				w.landNotice()
				w.tick()
				w.tick(func(r *RunView) { r.State, r.Run, r.PastLand = "cancelled", runfsm.Cancelled, true })
			},
			want: []string{"[run watch] task: cancelled; batten cancelled at Run-Shed; child running at Plan-Write"},
		},
		{
			name: "ARunUnlistedAheadOfLandAndNextListedLandedPostsItsLine",
			act: func(_ *testing.T, w *watchFake) {
				w.landNotice()
				w.h.fake.statuses["task"] = true
				w.tick()
				w.tickUnlisted()
				w.tick(landedView)
			},
			want: []string{landed},
		},
		{
			name: "ARunUnlistedPastLandKeepsItsLandNoticeReading",
			act: func(_ *testing.T, w *watchFake) {
				w.landNotice()
				w.h.fake.statuses["task"] = true
				w.tick()
				w.tick(landingView)
				w.tickUnlisted()
				w.tick(landedView)
			},
		},
		{
			name: "ALevelThatFirstReadsPastLandKeepsTheLandNoticeReadingCopiedAheadOfIt",
			act: func(_ *testing.T, w *watchFake) {
				w.landNotice()
				w.levelErr = errors.New("unreadable")
				w.tick()
				w.levelErr = nil
				w.tick(landedView)
			},
		},
		{
			name: "ALevelOffAheadOfLandIsKeptPastIt",
			act: func(_ *testing.T, w *watchFake) {
				w.level = RunWatchOff
				w.tick()
				w.level = RunWatchStops
				w.tick(landingView)
				w.h.restartDobby()
				w.tick(landedView)
			},
		},
		{
			name: "ALevelStopsAheadOfLandIsKeptPastIt",
			act: func(_ *testing.T, w *watchFake) {
				w.tick()
				w.level = RunWatchOff
				w.tick(landingView)
				w.tick(landedView)
				w.tick(landedView)
			},
			want: []string{landed},
		},
		{
			name: "OwedLinesWaitForAnOrchAndPostOldestFirst",
			act: func(t *testing.T, w *watchFake) {
				w.noOrch = true
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.writeChild("failed", "Plan-Write")
				w.tick()
				w.tick()
				w.tick()
				if watch, _ := w.memory(); len(watch.Owed) != 2 || watch.Owed[0].Attempts != 0 {
					t.Errorf("owed = %+v; want both lines owed with no attempt counted", watch.Owed)
				}
				w.noOrch = false
				w.tick()
			},
			want: []string{blocked, head + "failed at Plan-Write"},
		},
		{
			name: "AFailingPostWaitsAProbeIntervalAndIsDroppedAfterThreeFailures",
			act: func(t *testing.T, w *watchFake) {
				w.postErr = errors.New("creel down")
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.h.tickAfter(w.h.d.probe / 2)
				if w.postCalls != 1 {
					t.Errorf("posts within a probe interval = %d; want 1", w.postCalls)
				}
				for range 3 {
					w.h.tickAfter(w.h.d.probe)
				}
				w.postErr = nil
				w.h.tickAfter(w.h.d.probe)
				if w.postCalls != 3 {
					t.Errorf("posts = %d; want the line dropped after 3", w.postCalls)
				}
			},
		},
		{
			name: "ALineOwedAnHourAwakeIsDropped",
			act: func(_ *testing.T, w *watchFake) {
				w.noOrch = true
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.h.advance(time.Hour)
				w.noOrch = false
				w.tick()
			},
		},
		{
			name: "ALineOwedAcrossAnHourOfMachineSleepStillPosts",
			act: func(_ *testing.T, w *watchFake) {
				w.noOrch = true
				w.writeChild("blocked", "Plan-Write")
				w.tick()
				w.h.clock.suspend(time.Hour)
				w.noOrch = false
				w.tick()
			},
			want: []string{blocked},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWatchFake(t)

			tt.act(t, w)

			got := slices.Clone(w.lines)
			if !tt.exact {
				for i := range got {
					got[i], _, _ = strings.Cut(got[i], "; since ")
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("lines posted = %q; want %q", got, tt.want)
			}
		})
	}
}

// watchRowMoves moves the child through a row, a block, a resume and its end, then moves batten's row.
func watchRowMoves(_ *testing.T, w *watchFake) {
	w.tick()
	w.writeChild("running", "Webster")
	w.tick()
	w.writeChild("blocked", "Webster")
	w.tick()
	w.writeChild("running", "Webster")
	w.tick()
	w.writeChild("done", "Land")
	w.tick()
	w.tick(func(r *RunView) { r.Producer = "Land" })
}
