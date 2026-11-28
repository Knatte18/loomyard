// editloop_test.go covers the notice Wait sends a parent when one agent edits one file again and again.
// It pins when the notice is sent, what it names, and the EditLoopNotifiedOffset it persists.

package shuttleengine

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWait_EditLoopNotice pins the edit-loop watch through Wait, under a config of three edits within one minute.
// The watch reads the run's edit records file, not its events file.
// The run clock moves dialogTick per sleep, and an edit with no stamp takes the run clock's time.
func TestWait_EditLoopNotice(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const window = time.Minute
	const strandName = "lym:slug:writer"
	loopOf := func(agentID string, edits int) editLoop {
		return editLoop{key: editLoopKey{agentID: agentID, filePath: "a.go"}, edits: edits}
	}
	three := "EDIT::a.go\nEDIT::a.go\nEDIT::a.go\n"
	threeLen := int64(len(three))
	appendEdit := func(env *dialogEnv) { env.appendEvents("EDIT::a.go") }
	stampBase := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	// stamped is an edit of a.go the hook stamped seconds after stampBase; every stamped edit has the same length.
	stamped := func(seconds int) string {
		return "@" + stampBase.Add(time.Duration(seconds)*time.Second).Format(time.RFC3339) + "\nEDIT::a.go\n"
	}
	stampedLen := int64(len(stamped(0)))
	appendStamped := func(seconds int) func(*dialogEnv) {
		return func(env *dialogEnv) { env.appendEvents(strings.TrimSuffix(stamped(seconds), "\n")) }
	}
	tests := []struct {
		name   string
		events string
		// markEvents makes the fake engine read "@<time>" lines as the stamp of the edit after them.
		markEvents bool
		// off sets edit_loop_edits to 0.
		off         bool
		interactive bool
		noNotifier  bool
		// covered tells the runner that a live hub entry covers the run's pause record until a step calls env.resume.
		covered bool
		// failFirstSave makes the run-state save fail until the first step, which runs after the first sleep.
		failFirstSave bool
		// editLoopOffset is the persisted EditLoopNotifiedOffset the run starts with.
		editLoopOffset int64
		// steps[k] runs after the (k+1)th sleep, at (k+1)*dialogTick on the run clock.
		steps map[int]func(env *dialogEnv)
		// wantLoops holds the loop each edit-loop notice names, in order.
		// wantAt holds each notice's elapsed time and wantPersisted the offset at its call.
		wantLoops     []editLoop
		wantAt        []time.Duration
		wantPersisted []int64
		// wantFinal is the EditLoopNotifiedOffset the record holds when Wait returns.
		wantFinal int64
	}{
		{
			name: "three edits of one file within the window notify once, with the offset persisted before the call", events: three,
			wantLoops: []editLoop{loopOf("", 3)}, wantAt: []time.Duration{0}, wantPersisted: []int64{threeLen}, wantFinal: threeLen,
		},
		{
			name: "a subagent's loop names the subagent", events: "EDIT:agent-1:a.go\nEDIT:agent-1:a.go\nEDIT:agent-1:a.go\n",
			wantLoops: []editLoop{loopOf("agent-1", 3)}, wantAt: []time.Duration{0},
			wantPersisted: []int64{int64(len("EDIT:agent-1:a.go\n") * 3)}, wantFinal: int64(len("EDIT:agent-1:a.go\n") * 3),
		},
		{name: "the session's and a subagent's edits of one file count apart", events: "EDIT::a.go\nEDIT:agent-1:a.go\nEDIT::a.go\n"},
		{name: "edits of two files count apart", events: "EDIT::a.go\nEDIT::b.go\nEDIT::a.go\n"},
		{
			name: "edits across ticks within the window notify at the edit that reaches the count", events: "EDIT::a.go\n",
			steps:     map[int]func(*dialogEnv){5: appendEdit, 10: appendEdit},
			wantLoops: []editLoop{loopOf("", 3)}, wantAt: []time.Duration{55 * time.Second}, wantPersisted: []int64{threeLen}, wantFinal: threeLen,
		},
		{
			name: "an edit older than the window before the latest edit drops out of the count", events: "EDIT::a.go\n",
			steps: map[int]func(*dialogEnv){5: appendEdit, 12: appendEdit},
		},
		{
			name: "three edits read in one batch but stamped further apart than the window do not notify", markEvents: true,
			events: stamped(0) + stamped(120) + stamped(240),
		},
		{
			name: "three edits stamped within the window notify though the run clock read them further apart", markEvents: true,
			events:    stamped(0),
			steps:     map[int]func(*dialogEnv){12: appendStamped(10), 13: appendStamped(20)},
			wantLoops: []editLoop{loopOf("", 3)}, wantAt: []time.Duration{70 * time.Second},
			wantPersisted: []int64{3 * stampedLen}, wantFinal: 3 * stampedLen,
		},
		{
			name: "six edits notify twice, the count restarting after each notice", events: three + three,
			wantLoops: []editLoop{loopOf("", 3), loopOf("", 3)}, wantAt: []time.Duration{0, 0},
			wantPersisted: []int64{2 * threeLen, 2 * threeLen}, wantFinal: 2 * threeLen,
		},
		{
			name: "edits read while a pause covers the run notify once it lifts", events: three, covered: true,
			steps:     map[int]func(*dialogEnv){3: func(env *dialogEnv) { env.resume() }},
			wantLoops: []editLoop{loopOf("", 3)}, wantAt: []time.Duration{20 * time.Second}, wantPersisted: []int64{threeLen}, wantFinal: threeLen,
		},
		{
			name: "a failed offset save is retried on the next tick", events: three, failFirstSave: true,
			wantLoops: []editLoop{loopOf("", 3)}, wantAt: []time.Duration{dialogTick}, wantPersisted: []int64{threeLen}, wantFinal: threeLen,
		},
		{name: "a reattach whose persisted offset is past every edit sends nothing", events: three, editLoopOffset: threeLen, wantFinal: threeLen},
		{name: "a zero edit count turns the watch off", events: three, off: true},
		{name: "an interactive run sends nothing", events: three, interactive: true},
		{name: "a runner with no notifier sends nothing and persists no offset", events: three, noNotifier: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := gateConfig
			cfg.EditLoopWindowS = int(window / time.Second)
			if !tt.off {
				cfg.EditLoopEdits = 3
			}
			reed := &dialogReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}, pane: dialogPaneBox}
			fx := newFixture(t, reed, &sessionFakeEngine{markEvents: tt.markEvents}, withConfig(cfg))
			fc := newFakeClock(time.Now())
			start := fc.Now()
			steps := make([]func(), 1+slices.Max(append([]int{0}, slices.Collect(maps.Keys(tt.steps))...)))
			for index := range steps {
				steps[index] = func() {}
			}
			clk := &jumpStepClock{multiStepClock: &multiStepClock{fakeClock: fc, steps: steps}, jump: dialogTick}
			timeout := 5 * time.Minute
			run := fx.newRun(Spec{OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}, Timeout: timeout, Interactive: tt.interactive},
				withRunState(RunState{StrandGUID: "strand-1", StrandName: strandName, SessionID: "session-1", EditLoopNotifiedOffset: tt.editLoopOffset}),
				withRunEvents(""),
				withRunClock(clk, fc.Now().Add(timeout)))
			recordsPath := filepath.Join(run.runDir, EditRecordsFileName)
			if err := os.WriteFile(recordsPath, []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed the edit records file: %v", err)
			}
			env := &dialogEnv{appendEvents: func(line string) { appendEventsLine(t, recordsPath, line) }, reed: reed}
			if tt.covered {
				geometry, resume := coverHub(t, fc.Now())
				fx.Runner.SetPause(geometry)
				env.resume = func() { resume(fc.Now()) }
			}
			for index, step := range tt.steps {
				steps[index] = func() { step(env) }
			}
			if tt.failFirstSave {
				// run.json as a directory makes the run-state save fail until the first step removes it.
				stateFile := filepath.Join(run.runDir, runStateFileName)
				if err := os.RemoveAll(stateFile); err != nil {
					t.Fatalf("remove run.json: %v", err)
				}
				if err := os.MkdirAll(stateFile, 0o755); err != nil {
					t.Fatalf("plant run.json dir: %v", err)
				}
				steps[0] = func() {
					if err := os.RemoveAll(stateFile); err != nil {
						t.Errorf("remove the run.json dir: %v", err)
					}
				}
			}

			var gotNotices []string
			var gotAt []time.Duration
			var gotPersisted []int64
			if !tt.noNotifier {
				fx.Runner.SetNotifier(func(line string) error {
					if !strings.Contains(line, " edited the file ") {
						return nil
					}
					state, _, err := loadRunState(run.runDir)
					if err != nil {
						t.Fatalf("loadRunState in notifier: %v", err)
					}
					gotNotices = append(gotNotices, line)
					gotAt = append(gotAt, fc.Now().Sub(start))
					gotPersisted = append(gotPersisted, state.EditLoopNotifiedOffset)
					return nil
				})
			}
			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeTimeout {
				t.Errorf("outcome = %q, want %q: an edit-loop notice stops no session and changes no outcome", result.Outcome, OutcomeTimeout)
			}
			if len(gotNotices) != len(tt.wantLoops) {
				t.Fatalf("edit-loop notices = %q, want %d", gotNotices, len(tt.wantLoops))
			}
			for index, line := range gotNotices {
				if !strings.HasPrefix(line, "Shuttle notice:") || strings.ContainsAny(line, "\n\r") {
					t.Errorf("edit-loop notice = %q, want one line opening Shuttle notice:", line)
				}
				loop := tt.wantLoops[index]
				wantParts := []string{
					strandName,
					noticeOpen + loop.key.filePath + noticeClose,
					fmt.Sprintf("%d times", loop.edits),
					"within " + window.String(),
				}
				if loop.key.agentID != "" {
					wantParts = append(wantParts, "subagent "+noticeOpen+loop.key.agentID+noticeClose)
				}
				for _, part := range wantParts {
					if !strings.Contains(line, part) {
						t.Errorf("edit-loop notice = %q, want it to hold %q", line, part)
					}
				}
				if loop.key.agentID == "" && strings.Contains(line, "subagent") {
					t.Errorf("edit-loop notice = %q, want no subagent for the session's own edits", line)
				}
			}
			if !slices.Equal(gotAt, tt.wantAt) {
				t.Errorf("edit-loop notices at %v, want %v", gotAt, tt.wantAt)
			}
			if !slices.Equal(gotPersisted, tt.wantPersisted) {
				t.Errorf("EditLoopNotifiedOffset at each notifier call = %v, want %v", gotPersisted, tt.wantPersisted)
			}
			final, _, err := loadRunState(run.runDir)
			if err != nil {
				t.Fatalf("loadRunState after Wait: %v", err)
			}
			if final.EditLoopNotifiedOffset != tt.wantFinal {
				t.Errorf("final EditLoopNotifiedOffset = %d, want %d", final.EditLoopNotifiedOffset, tt.wantFinal)
			}
		})
	}
}
