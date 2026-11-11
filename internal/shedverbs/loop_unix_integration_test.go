//go:build !windows

// loop_unix_integration_test.go holds the loop test that kills the child tree through its process group, which Windows replaces with a named job.

package shedverbs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLoopIntegration_ChildTreeDiesWithLoop(t *testing.T) {
	tiergate.Require(t, 1)
	dir := t.TempDir()
	spec := helperSpec(dir, "")
	status := shedengine.Status{CurrentProducer: "A", State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}}
	if err := state.WriteJSON(spec.StatusPath, spec.StatusLockPath, status); err != nil {
		t.Fatalf("write the status file: %v", err)
	}

	waiter, _ := startHelperWaiter(t, dir)
	t.Cleanup(func() { _ = waiter.Process.Kill() })
	var record LoopPIDRecord
	var grandchild int
	waitUntil(t, 30*time.Second, "the loop's step child to start a grandchild", func() bool {
		found := false
		var err error
		record, found, err = ReadLoopPIDRecord(spec.Loop.PIDPath)
		data, readErr := os.ReadFile(filepath.Join(dir, "grandchild.pid"))
		if err != nil || !found || record.Child.PID == 0 || readErr != nil {
			return false
		}
		grandchild, err = strconv.Atoi(string(data))
		return err == nil
	})
	t.Cleanup(func() {
		_, _ = proc.KillTree(record.Child)
		_ = proc.KillPID(record.Loop.PID)
		_ = proc.KillPID(grandchild)
	})
	childTraceID, err := os.ReadFile(filepath.Join(dir, "trace.id"))
	if err != nil || record.TraceID == "" || record.TraceID != string(childTraceID) {
		t.Errorf("pid record trace id = %q; want the trace id the child ran under, %q (read error %v)", record.TraceID, childTraceID, err)
	}

	if err := waiter.Process.Kill(); err != nil {
		t.Fatalf("kill the waiter: %v", err)
	}
	_ = waiter.Wait()
	if loopLockFree(spec.Loop.LockPath) || processEnded(record.Loop.PID) || processEnded(record.Child.PID) || processEnded(grandchild) {
		t.Fatalf("after the waiter's death the loop, its child or the grandchild ended; want all three to outlive the waiter")
	}

	if err := syscall.Kill(record.Loop.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("terminate the loop: %v", err)
	}
	waitUntil(t, 30*time.Second, "the step child to die with the loop", func() bool { return processEnded(record.Child.PID) })
	// The waiter that spawned the keeper is dead, so only the keeper can record the loop's exit status.
	exitMark := strings.TrimSuffix(loopLogMark(record.LoopID, fmt.Sprintf("process %d exited: signal: terminated", record.Loop.PID)), "\n")
	waitUntil(t, 30*time.Second, "the keeper to mark the loop's exit status", func() bool {
		log, err := os.ReadFile(spec.Loop.LogPath)
		return err == nil && strings.Contains(string(log), exitMark)
	})

	second, out := startHelperWaiter(t, dir)
	if err := second.Wait(); err == nil {
		t.Errorf("the second waiter exited 0; want the loop-exited stop's exit 1")
	}
	var env map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &env); err != nil {
		t.Fatalf("decode the second waiter's envelope from %q: %v", out.String(), err)
	}
	if env["kind"] != KindInterrupted || !strings.Contains(env["error"].(string), "(loop-exited)") {
		t.Errorf("second waiter kind/error = %v/%v; want the loop-exited stop", env["kind"], env["error"])
	}
	loopObj, _ := env["loop"].(map[string]any)
	detail, _ := loopObj["detail"].(string)
	wantExit := "; loop exit: " + strings.TrimSuffix(loopLogMark(record.LoopID, "received signal terminated and ends without an envelope"), "\n") + " | " + exitMark
	if !strings.Contains(detail, "; step A was in flight under trace "+record.TraceID) || !strings.Contains(detail, wantExit) || !strings.Contains(detail, "; its step tree was still running and was killed") {
		t.Errorf("second waiter loop.detail = %q; want the in-flight step with its trace, the loop's signal and exit marks and the killed step tree", detail)
	}
	waitUntil(t, 30*time.Second, "the dead loop's step tree to be killed", func() bool { return processEnded(grandchild) })
	if st, found, err := state.ReadJSON[shedengine.Status](spec.StatusPath, spec.StatusLockPath); err != nil || !found || st.State != shedengine.StateFailed {
		t.Errorf("status = %+v, %v, %v; want failed", st, found, err)
	}
}

// groupChild is a Child whose record names a real process group.
type groupChild struct{ record proc.TreeRecord }

func (c groupChild) Wait() error             { return nil }
func (c groupChild) Quit() error             { return nil }
func (c groupChild) Kill() error             { return nil }
func (c groupChild) Record() proc.TreeRecord { return c.record }

func TestLoopIntegration_PanicKillsTheInFlightStepTree(t *testing.T) {
	tiergate.Require(t, 1)
	sleeper := exec.Command("sleep", "60")
	sleeper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sleeper.Start(); err != nil {
		t.Fatalf("start the stand-in step child: %v", err)
	}
	reaped := make(chan struct{})
	go func() {
		_ = sleeper.Wait()
		close(reaped)
	}()
	t.Cleanup(func() {
		_ = sleeper.Process.Kill()
		<-reaped
	})
	pid := sleeper.Process.Pid
	startTime, _ := proc.StartTime(pid)
	fx := newLoopFixture(t)
	fx.spec.Loop.Runner = func(context.Context, ChildRequest) (Child, error) {
		return groupChild{record: proc.TreeRecord{PID: pid, PGID: pid, StartTime: startTime}}, nil
	}
	// The loop reads its clock right after it records the child, so this panics with the child in flight.
	fx.spec.Loop.Now = func() time.Time { panic("boom") }

	env, _ := fx.execute()

	if env["kind"] != KindInterrupted {
		t.Errorf("envelope kind = %v; want the interrupted loop-panicked stop", env["kind"])
	}
	waitUntil(t, 30*time.Second, "the panicked loop's in-flight step tree to be killed", func() bool { return processEnded(pid) })
}

func TestLoopIntegration_SpawnMarksKeeperExit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	spec := helperSpec(t.TempDir(), "/bin/sh")

	// The output ends without a newline, so the exit mark must open its own line to stay a mark.
	exited, err := spawnLoop(spec, "loop-x", []string{"-c", "printf crashed >&2; exit 3"})
	if err != nil {
		t.Fatalf("spawnLoop = %v; want the process started", err)
	}
	<-exited

	want := regexp.MustCompile(`^; loop exit: lym shed: loop loop-x keeper process [0-9]+ exited: exit status 3; loop log: crashed$`)
	if got := loopExitEvidence(spec.Loop.LogPath, "loop-x"); !want.MatchString(got) {
		t.Errorf("loopExitEvidence = %q; want it to match %s", got, want)
	}
}

// openKeeperLog writes the starting mark a spawn would write and opens the loop log a keeper appends to.
func openKeeperLog(t *testing.T, spec *Spec) *os.File {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(spec.Loop.LogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Loop.LogPath, []byte(loopLogMark("loop-x", loopStartingMark)), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(spec.Loop.LogPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return log
}

func TestLoopIntegration_KeeperMarksTheLoopProcessExit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name       string
		executable string
		// script is the shell script the loop process runs.
		script     string
		wantCode   int
		wantStarts int
		want       *regexp.Regexp
	}{
		{
			// The output ends without a newline, so the exit mark must open its own line to stay a mark.
			name:       "ExitStatus",
			executable: "/bin/sh",
			script:     "printf crashed >&2; exit 3",
			wantCode:   1,
			wantStarts: 1,
			want:       regexp.MustCompile(`^; loop exit: lym shed: loop loop-x process [0-9]+ exited: exit status 3; loop log: crashed$`),
		},
		{
			name:       "CleanExit",
			executable: "/bin/sh",
			script:     "exit 0",
			wantCode:   0,
			wantStarts: 1,
			want:       regexp.MustCompile(`^; loop exit: lym shed: loop loop-x process [0-9]+ exited: exit status 0$`),
		},
		{
			name:       "KilledBySignal",
			executable: "/bin/sh",
			script:     "kill -9 $$",
			wantCode:   1,
			wantStarts: 1,
			want:       regexp.MustCompile(`^; loop exit: lym shed: loop loop-x process [0-9]+ exited: signal: killed$`),
		},
		{
			name:       "StartFailure",
			executable: "/nonexistent/loop",
			wantCode:   1,
			wantStarts: 0,
			want:       regexp.MustCompile(`^; loop exit: lym shed: loop loop-x could not start: fork/exec /nonexistent/loop: no such file or directory$`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := helperSpec(t.TempDir(), tt.executable)
			log := openKeeperLog(t, spec)
			spec.Loop.Signals = func() (<-chan os.Signal, func()) { return nil, func() {} }
			starts := 0
			spec.Loop.OnStart = func() { starts++ }

			code := keepLoop(spec, "loop-x", []string{"-c", tt.script}, log)

			if code != tt.wantCode || starts != tt.wantStarts {
				t.Errorf("keepLoop = %d with OnStart run %d times; want %d with %d", code, starts, tt.wantCode, tt.wantStarts)
			}
			if got := loopExitEvidence(spec.Loop.LogPath, "loop-x"); !tt.want.MatchString(got) {
				t.Errorf("loopExitEvidence = %q; want it to match %s", got, tt.want)
			}
		})
	}
}

func TestLoopIntegration_KeeperKeepsWaitingThroughTerminatingSignals(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	spec := helperSpec(t.TempDir(), "/bin/sh")
	log := openKeeperLog(t, spec)
	signals := make(chan os.Signal)
	spec.Loop.Signals = func() (<-chan os.Signal, func()) { return signals, func() {} }
	gate := t.TempDir()
	started, release := filepath.Join(gate, "started"), filepath.Join(gate, "release")
	script := fmt.Sprintf("touch %s; while [ ! -e %s ]; do sleep 0.05; done", started, release)
	returned := make(chan int, 1)
	go func() { returned <- keepLoop(spec, "loop-x", []string{"-c", script}, log) }()
	waitUntil(t, 30*time.Second, "the loop process to start", func() bool {
		_, err := os.Stat(started)
		return err == nil
	})

	// The send completes only when the keeper's handler takes the signal.
	select {
	case signals <- syscall.SIGTERM:
	case <-time.After(30 * time.Second):
		t.Fatalf("the keeper did not take a terminating signal; want its handler running")
	}
	select {
	case code := <-returned:
		t.Fatalf("keepLoop returned %d after a terminating signal while the loop process still ran; want it to keep waiting", code)
	default:
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if code := <-returned; code != 0 {
		t.Errorf("keepLoop = %d; want 0 once the loop process ended cleanly", code)
	}
	want := regexp.MustCompile(`^; loop exit: lym shed: loop loop-x process [0-9]+ exited: exit status 0$`)
	if got := loopExitEvidence(spec.Loop.LogPath, "loop-x"); !want.MatchString(got) {
		t.Errorf("loopExitEvidence = %q; want it to match %s", got, want)
	}
}
