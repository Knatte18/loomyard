// loop_test.go drives step's --until-stop loop through stepCmd over a fake ChildRunner that writes step records, stdout envelopes and status files the way a child would,
// and a fake trace lister over files in the test's directory.
// No test in this file calls t.Parallel: the loop logs through the process-global logger.

package shedverbs

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// loopFixture is one loop run's fake world: a seeded status file, a steps directory and a Runner that plays the scripted children in order.
type loopFixture struct {
	t        *testing.T
	paths    testPaths
	stepsDir string
	spec     *Spec
	scripts  []childScript
	requests []ChildRequest
	// traces maps a child's trace id to the trace files the fake lister reports for it.
	traces map[string][]string
}

// childScript plays one child: it writes what the child would leave behind and returns what Wait reports.
type childScript func(fx *loopFixture, traceID string, req ChildRequest) error

// fakeChild is the Child a scripted run returns; the script has already run when it is built.
type fakeChild struct{ waitErr error }

func (c fakeChild) Wait() error             { return c.waitErr }
func (c fakeChild) Quit() error             { return nil }
func (c fakeChild) Kill() error             { return nil }
func (c fakeChild) Record() proc.TreeRecord { return proc.TreeRecord{PID: 4242} }

func newLoopFixture(t *testing.T, scripts ...childScript) *loopFixture {
	t.Helper()
	paths := newTestPaths(t)
	seedStatus(t, paths, "A")
	dir := t.TempDir()
	fx := &loopFixture{t: t, paths: paths, stepsDir: filepath.Join(dir, "steps"), scripts: scripts, traces: map[string][]string{}}
	fx.spec = &Spec{
		StatusPath:     paths.StatusPath,
		LockPath:       paths.LockPath,
		StatusLockPath: paths.StatusLockPath,
		RunID:          "run-1",
		StepsDir:       fx.stepsDir,
		ScratchDir:     filepath.Join(dir, "scratch"),
		FrictionDir:    filepath.Join(dir, "friction"),
		Hooks:          Hooks{InterruptPolicyFor: func(row string) string { return "policy-" + row }},
		Loop: LoopSpec{
			EnvelopePath: filepath.Join(dir, "loop-envelope.json"),
			LogPath:      filepath.Join(dir, "loop.log"),
			Executable:   "lyx-fake",
			StopFiles: func(traceID string) (string, string) {
				return filepath.Join(fx.stepsDir, traceID+".trace.log"), filepath.Join(fx.stepsDir, traceID+".stderr.log")
			},
			TraceFiles: func(traceID string) ([]string, error) { return fx.traces[traceID], nil },
			Runner:     fx.run,
		},
	}
	return fx
}

// run is the fixture's ChildRunner: it plays the next script under the trace id the loop put in the child's environment.
func (fx *loopFixture) run(_ context.Context, req ChildRequest) (Child, error) {
	fx.t.Helper()
	index := len(fx.requests)
	fx.requests = append(fx.requests, req)
	if index >= len(fx.scripts) {
		fx.t.Fatalf("the loop started child %d; the test scripted %d", index+1, len(fx.scripts))
	}
	return fakeChild{waitErr: fx.scripts[index](fx, traceIDOf(req), req)}, nil
}

// traceIDOf is the LYX_TRACE_ID the request's environment carries.
func traceIDOf(req ChildRequest) string {
	for _, entry := range req.Env {
		if id, found := strings.CutPrefix(entry, "LYX_TRACE_ID="); found {
			return id
		}
	}
	return ""
}

// execute runs `step --until-stop` over the fixture and decodes the printed envelope.
func (fx *loopFixture) execute() (map[string]any, int) {
	fx.t.Helper()
	return execEnvelope(fx.t, stepCmd(stepTexts(), fx.spec), []string{"--" + UntilStopFlag})
}

// childIDs lists the trace ids of the children the loop started, in order.
func (fx *loopFixture) childIDs() []string {
	var ids []string
	for _, req := range fx.requests {
		ids = append(ids, traceIDOf(req))
	}
	return ids
}

func historyOf(length int) []shedengine.HistoryEntry {
	history := make([]shedengine.HistoryEntry, length)
	for i := range history {
		history[i] = shedengine.HistoryEntry{Producer: "A", Outcome: shedengine.Done}
	}
	return history
}

// setStatus rewrites the status file the way the engine would after a step.
func (fx *loopFixture) setStatus(current string, st shedengine.State, history int) {
	fx.t.Helper()
	if err := state.WriteJSON(fx.paths.StatusPath, fx.paths.StatusLockPath, shedengine.Status{CurrentProducer: current, State: st, History: historyOf(history)}); err != nil {
		fx.t.Fatalf("write status: %v", err)
	}
}

func (fx *loopFixture) readStatus() shedengine.Status {
	fx.t.Helper()
	st, found, err := state.ReadJSONStrict[shedengine.Status](fx.paths.StatusPath, fx.paths.StatusLockPath)
	if err != nil || !found {
		fx.t.Fatalf("read status = %v, found %v; want the file", err, found)
	}
	return st
}

// writeRecord stores fields as the child's own record, <traceID>.json in the steps directory.
func (fx *loopFixture) writeRecord(traceID string, fields map[string]any) {
	fx.t.Helper()
	if err := os.MkdirAll(fx.stepsDir, 0o755); err != nil {
		fx.t.Fatalf("mkdir steps: %v", err)
	}
	data, err := json.Marshal(fields)
	if err != nil {
		fx.t.Fatalf("marshal record: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fx.stepsDir, traceID+".json"), data, 0o644); err != nil {
		fx.t.Fatalf("write record: %v", err)
	}
}

func (fx *loopFixture) writeSuccess(traceID string, res shedengine.StepResult) {
	fx.t.Helper()
	fields := StepEnvelope(res, "", fx.paths.StatusPath, "", StepLocations{TraceID: traceID, RunID: "run-1"}, nil)
	fields["ok"] = true
	fx.writeRecord(traceID, fields)
}

func (fx *loopFixture) writeError(traceID, kind, transient, msg string) {
	fx.t.Helper()
	fields := stepErrFields(kind, transient, "", StepLocations{TraceID: traceID, RunID: "run-1"})
	fields["ok"] = false
	fields["error"] = msg
	fx.writeRecord(traceID, fields)
}

// continuing is a child whose step succeeded and left the run running at next.
func continuing(next string, history int) childScript {
	return func(fx *loopFixture, traceID string, _ ChildRequest) error {
		fx.writeSuccess(traceID, shedengine.StepResult{Producer: "A", Outcome: shedengine.Done, Next: next, State: shedengine.StateRunning, History: historyOf(history)})
		fx.setStatus(next, shedengine.StateRunning, history)
		return nil
	}
}

// halting is a child whose step succeeded and left the run in st, with reason.
func halting(st shedengine.State, reason string, history int) childScript {
	return func(fx *loopFixture, traceID string, _ ChildRequest) error {
		fx.writeSuccess(traceID, shedengine.StepResult{Producer: "A", Outcome: shedengine.Done, Next: "B", State: st, Reason: reason, History: historyOf(history)})
		fx.setStatus("B", st, history)
		return nil
	}
}

// failing is a child whose step ended in an error of kind, leaving the status file running.
func failing(kind, transient, msg string) childScript {
	return func(fx *loopFixture, traceID string, _ ChildRequest) error {
		fx.writeError(traceID, kind, transient, msg)
		return nil
	}
}

// loopOf is the envelope's loop object.
func loopOf(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	loop, ok := env["loop"].(map[string]any)
	if !ok {
		t.Fatalf("envelope %v has no loop object", env)
	}
	return loop
}

// assertLoopKeys checks the loop object's closed key set.
func assertLoopKeys(t *testing.T, loop map[string]any, withRestep bool) {
	t.Helper()
	want := []string{"steps", "first_producer", "stop", "detail", "status_moved", "current_producer", "history_length", "state", "interrupt_policy", "trace_copy", "stderr_path"}
	if withRestep {
		want = append(want, "restep")
	}
	var got []string
	for key := range loop {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loop keys = %v; want the closed set %v", got, want)
	}
}

func TestLoop_RunsUntilStopAndPrintsOneEnvelope(t *testing.T) {
	stdoutRefusal := func(fx *loopFixture, _ string, req ChildRequest) error {
		var line bytes.Buffer
		output.ErrFields(&line, "arming refused", map[string]any{"kind": KindBootstrap, "trace_file": "/trace/child.log"})
		_, _ = req.Stdout.Write(line.Bytes())
		return errors.New("exit status 1")
	}
	tests := []struct {
		name     string
		scripts  []childScript
		wantStop LoopStop
		wantExit int
		// wantStep is the step part of the envelope when the child left no record to derive it from.
		wantStep map[string]any
	}{
		{name: "blocked", scripts: []childScript{continuing("B", 1), halting(shedengine.StateBlocked, shedengine.ReasonBounceBudgetExhausted, 2)}, wantStop: LoopStopHalted},
		{name: "awaiting", scripts: []childScript{halting(shedengine.StateAwaiting, "", 1)}, wantStop: LoopStopHalted},
		{name: "done", scripts: []childScript{continuing("B", 1), halting(shedengine.StateDone, "", 2)}, wantStop: LoopStopHalted},
		{name: "bare pause", scripts: []childScript{halting(shedengine.StatePaused, "", 1)}, wantStop: LoopStopHalted},
		{name: "stop condition", scripts: []childScript{halting(shedengine.StatePaused, "paused before B, as requested", 1)}, wantStop: LoopStopCondition},
		{name: "busy", scripts: []childScript{failing(KindBusy, "", "run lock held")}, wantStop: LoopStopBusy, wantExit: 1},
		{
			name:     "arming refusal on stdout",
			scripts:  []childScript{stdoutRefusal},
			wantStop: LoopStopError,
			wantExit: 1,
			wantStep: map[string]any{"kind": KindBootstrap, "transient": "", "run_id": "", "trace_file": "/trace/child.log", "envelope_path": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newLoopFixture(t, tt.scripts...)
			env, code := fx.execute()

			if code != tt.wantExit {
				t.Errorf("exit code = %d; want %d (%v)", code, tt.wantExit, env)
			}
			loop := loopOf(t, env)
			assertLoopKeys(t, loop, false)
			if loop["stop"] != string(tt.wantStop) || loop["steps"] != float64(len(tt.scripts)) {
				t.Errorf("loop stop/steps = %v/%v; want %s/%d", loop["stop"], loop["steps"], tt.wantStop, len(tt.scripts))
			}

			ids := fx.childIDs()
			wantStep := tt.wantStep
			if wantStep == nil {
				recordPath := filepath.Join(fx.stepsDir, ids[len(ids)-1]+".json")
				data, err := os.ReadFile(recordPath)
				if err != nil {
					t.Fatalf("read the stop step's record: %v", err)
				}
				var full map[string]any
				if err := json.Unmarshal(data, &full); err != nil {
					t.Fatalf("decode the stop step's record: %v", err)
				}
				if full["ok"] == true {
					wantStep = shortStepEnvelope(full, recordPath)
				} else {
					wantStep = shortStepErrFields(full, recordPath)
				}
			}
			stepKeys := map[string]any{}
			for key, value := range env {
				if key != "loop" && key != "ok" && key != "error" {
					stepKeys[key] = value
				}
			}
			if !reflect.DeepEqual(stepKeys, wantStep) {
				t.Errorf("step keys = %v; want the stop step's short envelope %v", stepKeys, wantStep)
			}

			written, err := os.ReadFile(fx.spec.Loop.EnvelopePath)
			if err != nil {
				t.Fatalf("read the loop envelope file: %v", err)
			}
			var fromFile map[string]any
			if err := json.Unmarshal(written, &fromFile); err != nil || !reflect.DeepEqual(fromFile, env) {
				t.Errorf("loop envelope file = %v, %v; want the printed envelope %v", fromFile, err, env)
			}

			if _, err := os.Stat(filepath.Join(fx.stepsDir, logger.TraceID()+inflightSuffix)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the invoking process kept an in-flight record (stat err %v); the loop branch must return before writing one", err)
			}
			seen := map[string]bool{}
			for _, req := range fx.requests {
				id := traceIDOf(req)
				if id == "" || id == logger.TraceID() || seen[id] {
					t.Errorf("child trace id %q is empty, repeated or the loop's own", id)
				}
				seen[id] = true
				if !reflect.DeepEqual(req.Argv, []string{"lyx-fake"}) {
					t.Errorf("child argv = %v; want the executable alone, the loop's own flag dropped", req.Argv)
				}
			}
		})
	}
}

func TestLoop_InterruptedChildFillsTheErrorForm(t *testing.T) {
	t.Run("child ends without an envelope", func(t *testing.T) {
		var hookArgs []string
		var traceFile string
		fx := newLoopFixture(t, func(fx *loopFixture, traceID string, _ ChildRequest) error {
			traceFile = filepath.Join(t.TempDir(), "trace-"+traceID+".log")
			if err := os.WriteFile(traceFile, []byte("time=t level=INFO msg=started\n"), 0o644); err != nil {
				t.Fatalf("write trace: %v", err)
			}
			fx.traces[traceID] = []string{traceFile}
			return errors.New("exit status 3")
		})
		fx.spec.Hooks.AfterInterrupt = func(_ context.Context, producer, cause, file string) string {
			hookArgs = []string{producer, cause, file}
			return "reflected"
		}

		env, code := fx.execute()

		id := fx.childIDs()[0]
		wantText := "shedverbs: step A interrupted (exited: exit status 3); way forward: read loop.trace_copy and loop.stderr_path, apply the interrupted rule under loop.interrupt_policy, then re-run lyx shed step run-1 --until-stop"
		recordPath := filepath.Join(fx.stepsDir, id+".json")
		if code != 1 || env["kind"] != KindInterrupted || env["error"] != wantText || env["transient"] != "" || env["run_id"] != "run-1" {
			t.Errorf("exit/kind/error/transient/run_id = %d/%v/%v/%v/%v; want 1/%s/%q/empty/run-1", code, env["kind"], env["error"], env["transient"], env["run_id"], KindInterrupted, wantText)
		}
		if env["trace_file"] != traceFile || env["friction"] != "reflected" || env["envelope_path"] != recordPath {
			t.Errorf("trace_file/friction/envelope_path = %v/%v/%v; want %s/reflected/%s", env["trace_file"], env["friction"], env["envelope_path"], traceFile, recordPath)
		}
		if !reflect.DeepEqual(hookArgs, []string{"A", "exited: exit status 3", traceFile}) {
			t.Errorf("AfterInterrupt args = %v; want the stop row, the cause and the first trace file", hookArgs)
		}

		data, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatalf("read the interrupted record: %v", err)
		}
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatalf("decode the interrupted record: %v", err)
		}
		wantRecord := map[string]any{
			"ok": false, "error": wantText, "kind": KindInterrupted, "transient": "", "friction": "reflected",
			"trace_file": traceFile, "friction_dir": fx.spec.FrictionDir, "scratch_dir": fx.spec.ScratchDir, "trace_id": id, "run_id": "run-1",
		}
		if !reflect.DeepEqual(record, wantRecord) {
			t.Errorf("interrupted record = %v; want the full error form %v", record, wantRecord)
		}

		st := fx.readStatus()
		if st.State != shedengine.StateFailed || st.Error != wantText {
			t.Errorf("status state/error = %s/%q; want failed with the way-forward text", st.State, st.Error)
		}
		loop := loopOf(t, env)
		_, wantStderr := fx.spec.Loop.StopFiles(id)
		if loop["stop"] != string(LoopStopInterrupted) || loop["state"] != "failed" || loop["interrupt_policy"] != "policy-A" || loop["stderr_path"] != wantStderr {
			t.Errorf("loop stop/state/interrupt_policy/stderr_path = %v/%v/%v/%v; want interrupted/failed/policy-A/%s", loop["stop"], loop["state"], loop["interrupt_policy"], loop["stderr_path"], wantStderr)
		}
	})

	t.Run("status file vanishes between steps", func(t *testing.T) {
		fx := newLoopFixture(t, func(fx *loopFixture, traceID string, req ChildRequest) error {
			if err := continuing("B", 1)(fx, traceID, req); err != nil {
				return err
			}
			return os.Remove(fx.paths.StatusPath)
		})

		env, code := fx.execute()

		if code != 1 || env["kind"] != KindInterrupted || !strings.Contains(fmt.Sprint(env["error"]), "step B interrupted (status-missing)") {
			t.Errorf("exit/kind/error = %d/%v/%v; want an interrupted stop with cause status-missing at B", code, env["kind"], env["error"])
		}
		if len(fx.requests) != 1 {
			t.Errorf("children started = %d; want the loop to stop without starting another", len(fx.requests))
		}
	})
}

func TestLoop_TransientRestepOnce(t *testing.T) {
	const class = string(shedengine.TransientGitTransport)
	tests := []struct {
		name       string
		scripts    []childScript
		wantStop   LoopStop
		wantRestep string
		// wantRestepRow is the producer the re-step names; empty means the first row, A.
		wantRestepRow string
		wantStatus    shedengine.State
		wantStatusEr  string
	}{
		{
			name:       "passing re-step",
			scripts:    []childScript{failing(KindProducer, class, "first failure"), continuing("B", 1), halting(shedengine.StateDone, "", 2)},
			wantStop:   LoopStopHalted,
			wantRestep: "first failure",
			wantStatus: shedengine.StateDone,
		},
		{
			name:         "failing re-step reports both failures",
			scripts:      []childScript{failing(KindProducer, class, "first failure"), failing(KindProducer, class, "second failure")},
			wantStop:     LoopStopError,
			wantRestep:   "first failure",
			wantStatus:   shedengine.StateFailed,
			wantStatusEr: "second failure",
		},
		{
			name: "re-step after an intervening success",
			scripts: []childScript{
				failing(KindProducer, class, "first failure"), continuing("B", 1),
				failing(KindProducer, class, "later failure"), halting(shedengine.StateDone, "", 2),
			},
			wantStop:      LoopStopHalted,
			wantRestep:    "later failure",
			wantRestepRow: "B",
			wantStatus:    shedengine.StateDone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newLoopFixture(t, tt.scripts...)
			env, _ := fx.execute()

			loop := loopOf(t, env)
			assertLoopKeys(t, loop, true)
			if loop["stop"] != string(tt.wantStop) || loop["steps"] != float64(len(tt.scripts)) {
				t.Errorf("loop stop/steps = %v/%v; want %s/%d", loop["stop"], loop["steps"], tt.wantStop, len(tt.scripts))
			}
			wantRow := cmp.Or(tt.wantRestepRow, "A")
			wantRestep := map[string]any{"producer": wantRow, "error": tt.wantRestep, "transient": class}
			if !reflect.DeepEqual(loop["restep"], wantRestep) {
				t.Errorf("loop.restep = %v; want %v", loop["restep"], wantRestep)
			}
			st := fx.readStatus()
			if st.State != tt.wantStatus {
				t.Errorf("status state = %s; want %s", st.State, tt.wantStatus)
			}
			if tt.wantStatusEr != "" && (st.Error != tt.wantStatusEr || st.Transient != class) {
				t.Errorf("status error/transient = %q/%q; want the second failure %q with class %q", st.Error, st.Transient, tt.wantStatusEr, class)
			}
			if tt.wantStop == LoopStopError && env["error"] != tt.wantStatusEr {
				t.Errorf("envelope error = %v; want the second failure %q", env["error"], tt.wantStatusEr)
			}
		})
	}
}

func TestLoop_FailureStopLeavesStatusFailedUnlessBusy(t *testing.T) {
	t.Run("an error stop writes failed", func(t *testing.T) {
		fx := newLoopFixture(t, failing(KindProducer, "", "boom"))

		env, _ := fx.execute()

		st := fx.readStatus()
		if st.State != shedengine.StateFailed || st.Error != "boom" {
			t.Errorf("status state/error = %s/%q; want failed/boom", st.State, st.Error)
		}
		if loop := loopOf(t, env); loop["stop"] != string(LoopStopError) || loop["state"] != "failed" {
			t.Errorf("loop stop/state = %v/%v; want error/failed", loop["stop"], loop["state"])
		}
	})

	t.Run("another holder of the run lock makes the stop busy and writes nothing", func(t *testing.T) {
		fx := newLoopFixture(t, failing(KindProducer, "", "boom"))
		held, locked, err := lock.TryAcquireWriteLock(fx.paths.LockPath)
		if err != nil || !locked {
			t.Fatalf("hold the run lock = %v, locked %v; want held", err, locked)
		}
		t.Cleanup(func() { _ = held.Release() })

		env, _ := fx.execute()

		if st := fx.readStatus(); st.State != shedengine.StateRunning {
			t.Errorf("status state = %s; want running, untouched", st.State)
		}
		if loop := loopOf(t, env); loop["stop"] != string(LoopStopBusy) || loop["state"] != "running" {
			t.Errorf("loop stop/state = %v/%v; want busy/running", loop["stop"], loop["state"])
		}
	})
}

func TestLoop_StopDetailAndTraceCopy(t *testing.T) {
	tests := []struct {
		name  string
		lines int
		width int
	}{
		{name: "more lines than the line budget", lines: 60, width: 20},
		{name: "more bytes than the byte budget", lines: 10, width: 2000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var olderFile, newerFile string
			stopScript := func(fx *loopFixture, traceID string, _ ChildRequest) error {
				dir := t.TempDir()
				var older strings.Builder
				for i := range tt.lines {
					fmt.Fprintf(&older, "time=t level=WARN msg=warn-%d %s\n", i, strings.Repeat("x", tt.width))
				}
				olderFile, newerFile = filepath.Join(dir, "older.log"), filepath.Join(dir, "newer.log")
				if err := os.WriteFile(olderFile, []byte(older.String()), 0o644); err != nil {
					t.Fatalf("write older trace: %v", err)
				}
				if err := os.WriteFile(newerFile, []byte("time=t level=ERROR msg=last-error\n"), 0o644); err != nil {
					t.Fatalf("write newer trace: %v", err)
				}
				fx.traces[traceID] = []string{olderFile, newerFile}
				fx.writeError(traceID, KindProducer, "", "boom")
				return nil
			}
			fx := newLoopFixture(t, continuing("B", 1), stopScript)

			var printed bytes.Buffer
			clihelp.Execute(stepCmd(stepTexts(), fx.spec), &printed, []string{"--" + UntilStopFlag})

			if lines := strings.Split(strings.TrimRight(printed.String(), "\n"), "\n"); len(lines) != 1 {
				t.Errorf("printed %d lines; want the one envelope and nothing for the successful step", len(lines))
			}
			var env map[string]any
			if err := json.Unmarshal(printed.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			loop := loopOf(t, env)
			detail, _ := loop["detail"].(string)
			lines := strings.Split(detail, "\n")
			if len(lines) > detailLineBudget || len(detail) > detailByteBudget {
				t.Errorf("detail has %d lines and %d bytes; want at most %d and %d", len(lines), len(detail), detailLineBudget, detailByteBudget)
			}
			if lines[0] != "error: boom" || lines[len(lines)-1] != "[detail truncated]" || !strings.Contains(detail, "msg=warn-0") {
				t.Errorf("detail = %q; want the error first, a WARN line and the truncation marker last", detail)
			}

			if err := os.Remove(olderFile); err != nil {
				t.Fatalf("remove the original trace: %v", err)
			}
			copyPath, _ := loop["trace_copy"].(string)
			copied, err := os.ReadFile(copyPath)
			if err != nil || !strings.Contains(string(copied), fmt.Sprintf("msg=warn-%d", tt.lines-1)) || !strings.Contains(string(copied), "last-error") {
				t.Errorf("trace copy %q = %v, %v; want both trace files in order, surviving the removed original", copyPath, string(copied), err)
			}
		})
	}
}

func TestLoop_UnarmedSpecRefused(t *testing.T) {
	fx := newLoopFixture(t)
	fx.spec.Loop = LoopSpec{}

	env, code := fx.execute()

	if code != 1 || env["kind"] != KindBootstrap || !strings.Contains(fmt.Sprint(env["error"]), "arms no loop; way forward: run the step without --until-stop") {
		t.Errorf("exit/kind/error = %d/%v/%v; want a bootstrap refusal naming its way forward", code, env["kind"], env["error"])
	}
	if _, present := env["loop"]; present {
		t.Errorf("envelope %v carries a loop object; an unarmed refusal ran no loop", env)
	}
}
