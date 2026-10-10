// innerrun_terminal_test.go covers the inner-run watch's once-per-run terminal window open: where it sits relative to the spawn, its once-marker, and its warning-only failure mode.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

func absentThenRunning() []statusResult {
	return []statusResult{
		{found: false},
		{status: shedengine.Status{State: shedengine.StateRunning}, found: true},
	}
}

func approvedDeps(t *testing.T, spawnErr error) (*int, InnerRunDeps) {
	t.Helper()
	_, spawnCalls, deps := newInnerRunDeps(t, spawnErr, nil, awaitingStatus(), &fakeClock{})
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}, true, nil
	}
	return spawnCalls, deps
}

// TestInnerRun_OpensTerminalOnceAfterSpawn asserts the open sits between the spawn and the read after it, and that a re-spawn later in the same run does not reopen.
func TestInnerRun_OpensTerminalOnceAfterSpawn(t *testing.T) {
	t.Parallel()

	scratchDir := t.TempDir()
	var order []string
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, absentThenRunning(), &fakeClock{})
	spawn, read := deps.Spawn, deps.ReadStatus
	deps.Spawn = func(ctx context.Context) error {
		order = append(order, "spawn")
		return spawn(ctx)
	}
	deps.ReadStatus = func(a, b string) (shedengine.Status, bool, error) {
		order = append(order, "read")
		return read(a, b)
	}
	opens := 0
	deps.OpenTerminal = func(ctx context.Context) error {
		opens++
		order = append(order, "open")
		return nil
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	shedfake.CallOK(t, producer)
	if got, want := strings.Join(order, ","), "read,spawn,open,read"; got != want {
		t.Errorf("call order = %s; want %s", got, want)
	}

	if err := os.Remove(SpawnConfirmedFile(scratchDir, "innerrun")); err != nil {
		t.Fatalf("remove spawn-confirmed marker: %v", err)
	}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("second Call() error = %v", err)
	}
	if *spawnCalls != 2 || opens != 1 {
		t.Errorf("Spawn calls = %d, OpenTerminal calls = %d after a re-spawn; want 2 and 1", *spawnCalls, opens)
	}
}

// TestInnerRun_TerminalOpenGatedPerRun asserts the once-marker gates the open across resumes, a stale marker never suppresses a fresh child's open, and a failed spawn never opens.
func TestInnerRun_TerminalOpenGatedPerRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		marker    bool
		approved  bool
		spawnErr  error
		wantOpens int
	}{
		{name: "ApprovedResumeAfterOpenDoesNotReopen", marker: true, approved: true, wantOpens: 0},
		{name: "ApprovedResumeWithoutMarkerOpensOnce", marker: false, approved: true, wantOpens: 1},
		{name: "StaleMarkerDoesNotSuppressFreshChildOpen", marker: true, approved: false, wantOpens: 1},
		{name: "FailedSpawnDoesNotOpen", approved: false, spawnErr: errors.New("bootstrap failed"), wantOpens: 0},
		{name: "FailedApprovedResumeDoesNotOpen", approved: true, spawnErr: errors.New("bootstrap failed"), wantOpens: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			if tt.marker {
				if err := os.WriteFile(terminalOpenedFile(scratchDir, "innerrun"), []byte("opened\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var spawnCalls *int
			var deps InnerRunDeps
			if tt.approved {
				spawnCalls, deps = approvedDeps(t, tt.spawnErr)
			} else {
				_, spawnCalls, deps = newInnerRunDeps(t, tt.spawnErr, nil, absentThenRunning(), &fakeClock{})
			}
			opens := 0
			deps.OpenTerminal = func(ctx context.Context) error { opens++; return nil }
			producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

			_, _, err := producer.Call(context.Background())
			if (err != nil) != (tt.spawnErr != nil) {
				t.Fatalf("Call() error = %v; want an error only when the spawn fails", err)
			}
			if *spawnCalls != 1 || opens != tt.wantOpens {
				t.Errorf("Spawn calls = %d, OpenTerminal calls = %d; want 1 and %d", *spawnCalls, opens, tt.wantOpens)
			}
		})
	}
}

// TestInnerRun_TerminalOpenErrorIsWarnedNotEscalated runs serially: it redirects the process-wide logger output.
func TestInnerRun_TerminalOpenErrorIsWarnedNotEscalated(t *testing.T) {
	run := func(openErr error) (shedengine.Outcome, shedengine.OutputPointer, string, string) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(os.Stderr) })
		scratchDir := t.TempDir()
		_, _, deps := newInnerRunDeps(t, nil, nil, absentThenRunning(), &fakeClock{})
		deps.OpenTerminal = func(ctx context.Context) error { return openErr }
		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
		outcome, ptr := shedfake.CallOK(t, producer)
		return outcome, ptr, buf.String(), scratchDir
	}

	warnLines := func(logs string) []string {
		var lines []string
		for _, line := range strings.Split(logs, "\n") {
			if strings.Contains(line, "level=WARN") {
				lines = append(lines, line)
			}
		}
		return lines
	}

	wantOutcome, wantPtr, successLogs, _ := run(nil)
	outcome, ptr, logs, scratchDir := run(errors.New("konsole not found"))
	if outcome != wantOutcome || ptr.Reason != wantPtr.Reason || ptr.BudgetExempt != wantPtr.BudgetExempt {
		t.Errorf("Call() = %v %+v; want the success case's %v %+v", outcome, ptr, wantOutcome, wantPtr)
	}
	// The running arm's own stuck warning appears in both runs, so the failed open must add exactly one warning line to the success case's.
	successWarns, failWarns := warnLines(successLogs), warnLines(logs)
	var errWarns []string
	for _, line := range failWarns {
		if strings.Contains(line, "konsole not found") {
			errWarns = append(errWarns, line)
		}
	}
	if len(failWarns) != len(successWarns)+1 || len(errWarns) != 1 || !strings.Contains(errWarns[0], "myslug") {
		t.Errorf("warning lines = %q, success case's = %q; want exactly one more, naming the slug and the error", failWarns, successWarns)
	}
	if _, err := os.Stat(terminalOpenedFile(scratchDir, "innerrun")); err != nil {
		t.Errorf("terminal-opened marker: %v; want it written after a failed open", err)
	}
}
