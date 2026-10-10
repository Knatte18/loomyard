// loop.go implements step's --until-stop mode: the loop that runs one child step after another and returns at the first stop.
// It declares the loop's arming (LoopSpec), the child seam, the closed stop vocabulary and runLoop.
// The envelope the loop prints, and the arming refusal of the --until-stop invocation itself, live in loopenvelope.go.

package shedverbs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// UntilStopFlag is the name of step's flag that runs steps in a loop until a stop.
const UntilStopFlag = "until-stop"

// LoopSpec is the told arming of step's --until-stop mode.
// Every path is told by the arming module; the loop reads and copies only the paths it is handed.
// A zero EnvelopePath means the recipe arms no loop.
type LoopSpec struct {
	// LockPath is the loop's liveness lock.
	LockPath string
	// PIDPath is the loop's pid record.
	PIDPath string
	// LogPath is the file a child's stdout is appended to.
	LogPath string
	// EnvelopePath is the file the loop's final envelope is written to.
	EnvelopePath string
	// JobName is the Windows job-object name of the loop.
	JobName string
	// StopFiles names the two files a step keeps for the driver: the copy of the step's trace and the step's captured stderr.
	StopFiles func(traceID string) (traceCopy, stderr string)
	// TraceFiles lists the trace files of the child that ran under traceID, oldest first.
	TraceFiles func(traceID string) ([]string, error)
	// Runner starts a child step; nil selects no production value yet and is refused by the arming module.
	Runner ChildRunner
	// Executable is the program a child runs.
	Executable string
	// Now is the loop's clock; nil selects time.Now.
	Now func() time.Time
	// IdleTimeout is the window after which a child showing no activity is killed; zero disarms the watchdog.
	IdleTimeout time.Duration
	// Activity reports the newest agent activity of the run, and false when it knows of none.
	Activity func() (time.Time, bool, error)
}

// ChildRequest is everything a ChildRunner needs to start one child step.
type ChildRequest struct {
	// Argv is the command line, the executable first.
	Argv []string
	// Env is the child's whole environment.
	Env []string
	// Dir is the child's working directory; empty means the loop's own.
	Dir string
	// Stdout receives the child's standard output.
	Stdout io.Writer
	// StderrPath is the file the child's standard error is captured into.
	StderrPath string
}

// Child is a running child step.
type Child interface {
	// Wait waits for the child to exit and reports how it ended.
	Wait() error
	// Quit asks the child alone to dump its goroutines.
	Quit() error
	// Kill kills the child with its descendants.
	Kill() error
	// Record identifies the child's process tree.
	Record() proc.TreeRecord
}

// ChildRunner starts one child step.
type ChildRunner func(ctx context.Context, req ChildRequest) (Child, error)

// LoopStop is why the loop stopped.
type LoopStop string

const (
	// LoopStopHalted means the run halted: blocked, awaiting, done or paused by request.
	LoopStopHalted LoopStop = "halted"
	// LoopStopCondition means a pause_before or pause_after condition fired.
	LoopStopCondition LoopStop = "stop-condition"
	// LoopStopError means a step ended in an error.
	LoopStopError LoopStop = "error"
	// LoopStopInterrupted means a child ended without an envelope.
	LoopStopInterrupted LoopStop = "interrupted"
	// LoopStopBusy means another holder owns the run, so the loop wrote and decided nothing.
	LoopStopBusy LoopStop = "busy"
)

// childResult is what one child iteration left behind.
// It holds the step's full envelope and the path it was read from, or the cause the child is reported interrupted for when it left none.
type childResult struct {
	traceID string
	full    map[string]any
	path    string
	cause   string
}

// ok reports whether the iteration produced a success envelope.
func (r childResult) ok() bool {
	succeeded, _ := r.full["ok"].(bool)
	return r.full != nil && succeeded
}

// runLoop runs child steps until one stops the run, prints the loop's one envelope on out and returns the stop step's exit code.
// argv is the child's command line after the executable, the loop's own without the flag that selects the loop.
// A transient error gets one immediate re-step; an error or interrupted stop on a run still reading running writes it failed.
func runLoop(ctx context.Context, spec *Spec, argv []string, out io.Writer) int {
	loop := spec.Loop
	first, _ := readStatusFile(spec)
	obj := loopObject{FirstProducer: first.CurrentProducer}

	var (
		res        childResult
		before     = first
		stop       LoopStop
		restepped  bool
		lastKnown  = first.CurrentProducer
		stopReason string
	)
	for {
		traceID := logger.NewTraceID()
		current, present := readStatusFile(spec)
		if present {
			before = current
			lastKnown = current.CurrentProducer
			res = runChild(ctx, spec, argv, traceID)
			obj.Steps++
		} else {
			res = childResult{traceID: traceID, cause: "status-missing"}
		}

		if res.ok() {
			restepped = false
			if next, _ := res.full["next"].(string); next != "" {
				lastKnown = next
			}
			if keepGoing, _ := res.full["continue"].(bool); keepGoing {
				continue
			}
			stopReason, _ = res.full["reason"].(string)
			stop = LoopStopHalted
			if stateName, _ := res.full["state"].(string); stateName == string(shedengine.StatePaused) && isConditionReason(stopReason) {
				stop = LoopStopCondition
			}
			break
		}
		if res.full != nil {
			kind, _ := res.full["kind"].(string)
			transient, _ := res.full["transient"].(string)
			errText, _ := res.full["error"].(string)
			if kind == KindBusy {
				stop = LoopStopBusy
				break
			}
			if transient != "" && !restepped {
				restepped = true
				obj.Restep = &loopRestep{Producer: before.CurrentProducer, Error: errText, Transient: transient}
				logger.Warn("shed: loop re-steps a transient failure", "producer", before.CurrentProducer, "transient", transient, "error", errText)
				continue
			}
			stop = LoopStopError
			break
		}
		stop = LoopStopInterrupted
		break
	}

	var (
		errText   string
		transient string
	)
	switch stop {
	case LoopStopInterrupted:
		message, wayForward := interruptedError(spec.RunID, lastKnown, res.cause)
		errText = message + "; way forward: " + wayForward
		traceFile := ""
		if files := traceFilesOf(loop, res.traceID); len(files) > 0 {
			traceFile = files[0]
		}
		friction := ""
		if spec.Hooks.AfterInterrupt != nil {
			friction = spec.Hooks.AfterInterrupt(ctx, lastKnown, res.cause, traceFile)
		}
		res.full, res.path = interruptedRecord(spec, res.traceID, errText, traceFile, friction)
	case LoopStopError, LoopStopBusy:
		errText, _ = res.full["error"].(string)
		transient, _ = res.full["transient"].(string)
	}

	if stop == LoopStopError || stop == LoopStopInterrupted {
		failed, err := shedengine.WriteFailedStop(shedengine.FailedStopRequest{
			StatusPath:     spec.StatusPath,
			LockPath:       spec.LockPath,
			StatusLockPath: spec.StatusLockPath,
			Error:          errText,
			Transient:      shedengine.TransientClass(transient),
		})
		if err != nil {
			logger.Warn("shed: loop could not write the failed stop", "error", err.Error())
		} else if failed.Busy {
			stop = LoopStopBusy
		}
	}
	logger.Info("shed: loop stop", "stop", string(stop), "steps", obj.Steps, "trace_id", res.traceID, "error", errText)

	after, _ := readStatusFile(spec)
	obj.Stop = stop
	obj.CurrentProducer = after.CurrentProducer
	obj.HistoryLength = len(after.History)
	obj.State = string(after.State)
	obj.StatusMoved = before.CurrentProducer != after.CurrentProducer || len(before.History) != len(after.History)
	if spec.Hooks.InterruptPolicyFor != nil {
		obj.InterruptPolicy = spec.Hooks.InterruptPolicyFor(after.CurrentProducer)
	}

	files := traceFilesOf(loop, res.traceID)
	traceCopy, stderrPath := "", ""
	if loop.StopFiles != nil {
		traceCopy, stderrPath = loop.StopFiles(res.traceID)
	}
	obj.StderrPath = stderrPath
	if copyTraceFiles(files, traceCopy) {
		obj.TraceCopy = traceCopy
	}

	detailFiles := files
	if obj.State == string(shedengine.StateDone) || stop == LoopStopCondition || obj.State == string(shedengine.StatePaused) {
		detailFiles = nil
	}
	obj.Detail = stopDetail(errText, stopReason, transient, detailFiles)

	var buf bytes.Buffer
	var code int
	if res.ok() {
		code = output.Ok(&buf, loopEnvelope(shortStepEnvelope(res.full, res.path), obj))
	} else {
		code = output.ErrFields(&buf, errText, loopEnvelope(shortStepErrFields(res.full, res.path), obj))
	}
	if err := os.MkdirAll(filepath.Dir(loop.EnvelopePath), 0o755); err == nil {
		err = os.WriteFile(loop.EnvelopePath, buf.Bytes(), 0o644)
		if err != nil {
			logger.Warn("shed: loop could not write its envelope", "path", loop.EnvelopePath, "error", err.Error())
		}
	}
	_, _ = out.Write(buf.Bytes())
	return code
}

// runChild runs one child step under a fresh trace id and reads back what it left.
// The child's own record, <StepsDir>/<traceID>.json, is its full envelope;
// a child that refused before its step body ran left an error envelope on stdout instead;
// a child that left neither is reported interrupted for the cause its exit gives.
func runChild(ctx context.Context, spec *Spec, argv []string, traceID string) childResult {
	loop := spec.Loop
	var stdout bytes.Buffer
	stderrPath := ""
	if loop.StopFiles != nil {
		_, stderrPath = loop.StopFiles(traceID)
	}
	child, err := loop.Runner(ctx, ChildRequest{
		Argv:       append([]string{loop.Executable}, argv...),
		Env:        childEnvironment(traceID),
		Stdout:     &stdout,
		StderrPath: stderrPath,
	})
	if err != nil {
		logger.Warn("shed: loop child did not start", "trace_id", traceID, "error", err.Error())
		return childResult{traceID: traceID, cause: "start-failed: " + err.Error()}
	}
	logger.Info("shed: loop child started", "pid", child.Record().PID, "trace_id", traceID)
	waitErr := child.Wait()
	logger.Info("shed: loop child exited", "pid", child.Record().PID, "trace_id", traceID, "error", errorText(waitErr))
	appendLoopLog(loop.LogPath, stdout.Bytes())

	if spec.StepsDir != "" {
		path := filepath.Join(spec.StepsDir, traceID+envelopeSuffix)
		if data, err := os.ReadFile(path); err == nil {
			var full map[string]any
			if json.Unmarshal(data, &full) == nil {
				return childResult{traceID: traceID, full: full, path: path}
			}
		}
	}
	if full, ok := stdoutErrEnvelope(stdout.Bytes()); ok {
		return childResult{traceID: traceID, full: full}
	}
	cause := "exited"
	if waitErr != nil {
		cause += ": " + waitErr.Error()
	}
	return childResult{traceID: traceID, cause: cause}
}

// childEnvironment is the process environment with LYX_TRACE_ID set to traceID, so the child keeps its own trace group.
func childEnvironment(traceID string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "LYX_TRACE_ID=") {
			env = append(env, entry)
		}
	}
	return append(env, "LYX_TRACE_ID="+traceID)
}

// readStatusFile reads the status file, reporting false when it is missing or unreadable.
func readStatusFile(spec *Spec) (shedengine.Status, bool) {
	return readStatusAt(spec.StatusPath, spec.StatusLockPath)
}

// readStatusAt reads the status file at statusPath, reporting false when it is missing or unreadable.
func readStatusAt(statusPath, statusLockPath string) (shedengine.Status, bool) {
	st, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil || !found {
		return shedengine.Status{}, false
	}
	return st, true
}

// traceFilesOf lists the trace files of the child under traceID, or none when the arming module told no lister or it failed.
func traceFilesOf(loop LoopSpec, traceID string) []string {
	if loop.TraceFiles == nil {
		return nil
	}
	files, err := loop.TraceFiles(traceID)
	if err != nil {
		logger.Warn("shed: loop could not list a child's trace files", "trace_id", traceID, "error", err.Error())
		return nil
	}
	return files
}

// copyTraceFiles concatenates files, in order, into dest, and reports whether a copy was written.
func copyTraceFiles(files []string, dest string) bool {
	if dest == "" || len(files) == 0 {
		return false
	}
	var joined bytes.Buffer
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			logger.Warn("shed: loop could not read a trace file to copy", "path", file, "error", err.Error())
			continue
		}
		joined.Write(data)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		logger.Warn("shed: loop could not create the trace copy directory", "path", dest, "error", err.Error())
		return false
	}
	if err := os.WriteFile(dest, joined.Bytes(), 0o644); err != nil {
		logger.Warn("shed: loop could not write the trace copy", "path", dest, "error", err.Error())
		return false
	}
	return true
}

// appendLoopLog appends data to the loop log at path; an empty path or empty data keeps nothing.
func appendLoopLog(path string, data []byte) {
	if path == "" || len(data) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("shed: loop could not create the loop log directory", "path", path, "error", err.Error())
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Warn("shed: loop could not open the loop log", "path", path, "error", err.Error())
		return
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		logger.Warn("shed: loop could not write the loop log", "path", path, "error", err.Error())
	}
}

// isConditionReason reports whether reason is the one a fired pause_before or pause_after condition gives.
func isConditionReason(reason string) bool {
	return strings.HasPrefix(reason, "paused before ") || strings.HasPrefix(reason, "paused after ")
}

// errorText is err's text, or the empty string for a nil error.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
