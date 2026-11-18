// restart_test.go covers the watcher's restart of the orch's session.
// The restarts are the account switch on a usage limit and the fresh start after a refused resume.

package orchengine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// restartStrand is the strand the told restarts of a restartEnv move the state onto.
const restartStrand = "s2"

// restartEnv is a watchEnv whose watcher is told restarts that record their calls and move the state onto restartStrand.
type restartEnv struct {
	*watchEnv
	// calls lists the restarts the watcher ran, "resume" or "fresh".
	calls []string
	// failWith, when set, is what every restart returns instead of moving the state.
	failWith error
}

func newRestartEnv(t *testing.T) *restartEnv {
	t.Helper()
	e := &restartEnv{watchEnv: newWatchEnv(t)}
	restart := func(name string) func(State) (State, error) {
		return func(st State) (State, error) {
			e.calls = append(e.calls, name)
			if e.failWith != nil {
				return st, e.failWith
			}
			next := ResetForFreshLaunch(st, restartStrand, e.clock.now)
			if err := SaveState(e.paths, next); err != nil {
				return st, err
			}
			return next, nil
		}
	}
	e.w.WatchRestart(OrchRestart{Resume: restart("resume"), Fresh: restart("fresh")})
	return e
}

// stopOnLimit scripts the orch's next turn end as a usage limit answered with switched, and appends it.
func (e *restartEnv) stopOnLimit(switched bool) {
	e.s.usage["limited"] = 2000
	e.s.limits = map[string]shuttleengine.UsageLimit{"limited": {Limited: true}}
	e.s.switches = map[string]bool{"limited": switched}
	e.s.events = append(e.s.events, stop("limited"))
}

// TestWatcher_UsageLimitSwitchRestartsTheOrch is not parallel: its subtests capture the process-global logger output.
func TestWatcher_UsageLimitSwitchRestartsTheOrch(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name      string
		switched  bool
		switchErr error
		resumeErr error
		// wantEnded is whether the tick ends the watcher.
		wantEnded bool
		wantCalls []string
		// wantLimitRecorded is whether the state records the usage limit and its wait.
		wantLimitRecorded bool
		wantLog           string
	}{
		{name: "ASwitchResumesTheOrchOnceAndRecordsNoLimit", switched: true, wantCalls: []string{"resume"}},
		{name: "NoSwitchWaitsTheLimitOut", wantLimitRecorded: true},
		{
			name: "AFailedSwitchWaitsTheLimitOutAndWarns", switchErr: errors.New("account file unreadable"),
			wantLimitRecorded: true, wantLog: "account file unreadable",
		},
		{
			name: "AFailedResumeEndsTheWatcherNamingTheWayForward", switched: true, resumeErr: errBoom,
			wantEnded: true, wantCalls: []string{"resume"}, wantLog: "lym beam start",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			e := newRestartEnv(t)
			e.failWith = tt.resumeErr
			e.tick()
			e.stopOnLimit(tt.switched)
			e.s.switchErr = tt.switchErr

			done, err := e.w.Tick()
			if err != nil || done != tt.wantEnded {
				t.Fatalf("Tick = %v, %v; want done %v and no error", done, err, tt.wantEnded)
			}
			if !slices.Equal(e.calls, tt.wantCalls) {
				t.Errorf("restarts = %v; want %v", e.calls, tt.wantCalls)
			}
			st := e.state()
			if recorded := !st.UsageLimitAt.IsZero(); recorded != tt.wantLimitRecorded {
				t.Errorf("usage limit recorded = %v; want %v, state %+v", recorded, tt.wantLimitRecorded, st)
			}
			if tt.wantEnded && !strings.HasPrefix(st.WatcherExit, "restart failed") {
				t.Errorf("WatcherExit = %q; want the failed restart recorded", st.WatcherExit)
			}
			if tt.wantLog != "" && !strings.Contains(logs.String(), tt.wantLog) {
				t.Errorf("logs = %q; want one naming %q", logs.String(), tt.wantLog)
			}
		})
	}

	t.Run("AfterASwitchTheNextTickReadsTheNewStrandAtTheSavedCursor", func(t *testing.T) {
		e := newRestartEnv(t)
		e.tick()
		e.stopOnLimit(true)
		e.tick()
		if st := e.state(); st.Strand != restartStrand {
			t.Fatalf("state strand = %q; want the restart's %q", st.Strand, restartStrand)
		}

		// The new strand's events file starts where the saved state's cursor says: at its beginning.
		e.s.events = []shuttleengine.Event{stop("answer")}
		e.tick()
		if got := e.s.readGUIDs[len(e.s.readGUIDs)-1]; got != restartStrand {
			t.Errorf("last ReadEvents guid = %q; want %q", got, restartStrand)
		}
		if !slices.Equal(e.s.switchAsks, []string{"limited"}) || !slices.Equal(e.calls, []string{"resume"}) {
			t.Errorf("switch asks = %v, restarts = %v; want the one switch and its one resume", e.s.switchAsks, e.calls)
		}
	})
}

//testtiming:keep pins the fresh restart and the refused-resume-only ask, which the usage-limit test never drives directly
func TestWatcher_RefusedResumeStartsTheOrchFresh(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		refused bool
		// refusedErr is what the answer to the refused-resume question fails with.
		refusedErr error
		// freshErr is what the fresh restart fails with.
		freshErr  error
		wantEnded bool
		wantCalls []string
	}{
		{name: "ARefusedResumeStartsFreshOnce", refused: true, wantCalls: []string{"fresh"}},
		{name: "AnyOtherApiErrorStartsNothing"},
		{name: "AFailedAnswerStartsNothing", refused: true, refusedErr: errBoom},
		{name: "AFailedFreshRestartEndsTheWatcher", refused: true, freshErr: errBoom, wantEnded: true, wantCalls: []string{"fresh"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newRestartEnv(t)
			e.failWith = tt.freshErr
			e.tick()
			e.s.refusals = map[string]bool{"apierror": tt.refused}
			e.s.refusedErr = tt.refusedErr
			e.s.events = append(e.s.events, stop("apierror"))
			done, err := e.w.Tick()
			if err != nil || done != tt.wantEnded {
				t.Fatalf("Tick = %v, %v; want done %v and no error", done, err, tt.wantEnded)
			}
			if !slices.Equal(e.calls, tt.wantCalls) {
				t.Errorf("restarts = %v; want %v", e.calls, tt.wantCalls)
			}
			if st := e.state(); tt.wantEnded && !strings.HasPrefix(st.WatcherExit, "restart failed") {
				t.Errorf("WatcherExit = %q; want the failed restart recorded", st.WatcherExit)
			}
			if !slices.Equal(e.s.refusedAsks, []string{"apierror"}) || len(e.s.switchAsks) != 0 {
				t.Errorf("refused asks = %v, switch asks = %v; want the turn end asked as a refused resume alone", e.s.refusedAsks, e.s.switchAsks)
			}
		})
	}
}
