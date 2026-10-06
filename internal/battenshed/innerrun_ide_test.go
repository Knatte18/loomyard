// innerrun_ide_test.go covers the inner-run watch's once-per-run IDE open: where it sits relative to the spawn, its once-marker, and its warning-only failure mode.

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
	_, spawnCalls, deps := newInnerRunDeps(spawnErr, nil, awaitingStatus(), &fakeClock{})
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}, true, nil
	}
	return spawnCalls, deps
}

func TestInnerRun_OpensIDEOnceAfterSpawnBeforeSecondRead(t *testing.T) {
	scratchDir := t.TempDir()
	var order []string
	_, _, deps := newInnerRunDeps(nil, nil, absentThenRunning(), &fakeClock{})
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
	deps.OpenIDE = func(ctx context.Context) error {
		opens++
		order = append(order, "open")
		return nil
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	shedfake.CallOK(t, producer)
	if opens != 1 {
		t.Errorf("OpenIDE calls = %d; want 1", opens)
	}
	if got, want := strings.Join(order, ","), "read,spawn,open,read"; got != want {
		t.Errorf("call order = %s; want %s", got, want)
	}
}

func TestInnerRun_LaterSpawnsInSameRunDoNotReopen(t *testing.T) {
	scratchDir := t.TempDir()
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, absentThenRunning(), &fakeClock{})
	opens := 0
	deps.OpenIDE = func(ctx context.Context) error { opens++; return nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("first Call() error = %v", err)
	}
	if err := os.Remove(SpawnConfirmedFile(scratchDir, "innerrun")); err != nil {
		t.Fatalf("remove spawn-confirmed marker: %v", err)
	}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("second Call() error = %v", err)
	}
	if *spawnCalls != 2 {
		t.Fatalf("Spawn calls = %d; want 2", *spawnCalls)
	}
	if opens != 1 {
		t.Errorf("OpenIDE calls = %d after a re-spawn; want 1", opens)
	}
}

// TestInnerRun_OpensIDEOncePerRunAcrossAMarker asserts the once-marker gates the open: an approved resume after an earlier open does not reopen, an approved resume with no marker opens once, and a stale marker left from an earlier run does not suppress the open for a fresh child.
//
//testtiming:keep pins that the once-marker gates the IDE open, which the covering resume tests never count
func TestInnerRun_OpensIDEOncePerRunAcrossAMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		marker     bool
		approved   bool
		wantSpawns int
		wantOpens  int
	}{
		{name: "ApprovedResumeAfterOpenDoesNotReopen", marker: true, approved: true, wantSpawns: 1, wantOpens: 0},
		{name: "ApprovedResumeWithoutMarkerOpensOnce", marker: false, approved: true, wantSpawns: 1, wantOpens: 1},
		{name: "StaleMarkerDoesNotSuppressFreshChildOpen", marker: true, approved: false, wantSpawns: 1, wantOpens: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			if tt.marker {
				if err := os.WriteFile(ideOpenedFile(scratchDir, "innerrun"), []byte("opened\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var spawnCalls *int
			var deps InnerRunDeps
			if tt.approved {
				spawnCalls, deps = approvedDeps(t, nil)
			} else {
				_, spawnCalls, deps = newInnerRunDeps(nil, nil, absentThenRunning(), &fakeClock{})
			}
			opens := 0
			deps.OpenIDE = func(ctx context.Context) error { opens++; return nil }
			producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

			shedfake.CallOK(t, producer)
			if *spawnCalls != tt.wantSpawns || opens != tt.wantOpens {
				t.Errorf("Spawn calls = %d, OpenIDE calls = %d; want %d and %d", *spawnCalls, opens, tt.wantSpawns, tt.wantOpens)
			}
		})
	}
}

func TestInnerRun_OpenErrorIsWarnedNotEscalated(t *testing.T) {
	run := func(openErr error) (shedengine.Outcome, shedengine.OutputPointer, string, string) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(os.Stderr) })
		scratchDir := t.TempDir()
		_, _, deps := newInnerRunDeps(nil, nil, absentThenRunning(), &fakeClock{})
		deps.OpenIDE = func(ctx context.Context) error { return openErr }
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
	outcome, ptr, logs, scratchDir := run(errors.New("code not found"))
	if outcome != wantOutcome || ptr.Reason != wantPtr.Reason || ptr.BudgetExempt != wantPtr.BudgetExempt {
		t.Errorf("Call() = %v %+v; want the success case's %v %+v", outcome, ptr, wantOutcome, wantPtr)
	}
	// The running arm's own stuck warning appears in both runs, so the failed open must add exactly one warning line to the success case's.
	successWarns, failWarns := warnLines(successLogs), warnLines(logs)
	var errWarns []string
	for _, line := range failWarns {
		if strings.Contains(line, "code not found") {
			errWarns = append(errWarns, line)
		}
	}
	if len(failWarns) != len(successWarns)+1 || len(errWarns) != 1 || !strings.Contains(errWarns[0], "myslug") {
		t.Errorf("warning lines = %q, success case's = %q; want exactly one more, naming the slug and the error", failWarns, successWarns)
	}
	if _, err := os.Stat(ideOpenedFile(scratchDir, "innerrun")); err != nil {
		t.Errorf("ide-opened marker: %v; want it written after a failed open", err)
	}
}

func TestInnerRun_FailedSpawnDoesNotOpen(t *testing.T) {
	spawnErr := errors.New("bootstrap failed")

	t.Run("read-before-spawn", func(t *testing.T) {
		_, _, deps := newInnerRunDeps(spawnErr, nil, absentThenRunning(), &fakeClock{})
		opens := 0
		deps.OpenIDE = func(ctx context.Context) error { opens++; return nil }
		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace)
		if _, _, err := producer.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want the spawn error")
		}
		if opens != 0 {
			t.Errorf("OpenIDE calls = %d; want 0", opens)
		}
	})

	t.Run("approved-resume", func(t *testing.T) {
		_, deps := approvedDeps(t, spawnErr)
		opens := 0
		deps.OpenIDE = func(ctx context.Context) error { opens++; return nil }
		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace)
		if _, _, err := producer.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want the spawn error")
		}
		if opens != 0 {
			t.Errorf("OpenIDE calls = %d; want 0", opens)
		}
	})
}
