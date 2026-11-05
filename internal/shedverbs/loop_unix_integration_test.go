//go:build !windows

// loop_unix_integration_test.go holds the loop test that kills the child tree through its process group, which Windows replaces with a named job.

package shedverbs

import (
	"bytes"
	"context"
	"encoding/json"
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
	wantExit := "; loop exit: " + strings.TrimSuffix(loopLogMark(record.LoopID, "received signal terminated and ends without an envelope"), "\n")
	if !strings.Contains(detail, "; step A was in flight under trace "+record.TraceID) || !strings.Contains(detail, wantExit) || !strings.Contains(detail, "; its step tree was still running and was killed") {
		t.Errorf("second waiter loop.detail = %q; want the in-flight step with its trace, the loop's signal mark and the killed step tree", detail)
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

func TestLoopIntegration_SpawnMarksTheLoopsExit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	spec := helperSpec(t.TempDir(), "/bin/sh")

	// The output ends without a newline, so the exit mark must open its own line to stay a mark.
	exited, err := spawnLoop(spec, "loop-x", []string{"-c", "printf crashed >&2; exit 3"})
	if err != nil {
		t.Fatalf("spawnLoop = %v; want the process started", err)
	}
	<-exited

	got := loopExitEvidence(spec.Loop.LogPath, "loop-x")
	want := regexp.MustCompile(`^; loop exit: lym shed: loop loop-x process [0-9]+ exited: exit status 3; loop log: crashed$`)
	if !want.MatchString(got) {
		t.Errorf("loopExitEvidence = %q; want it to match %s", got, want)
	}
}
