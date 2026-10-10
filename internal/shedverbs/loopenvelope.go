// loopenvelope.go builds what the loop reports.
// That is the `loop` object beside the stop step's short envelope, the detail of a stop, the record of an interrupted step and the report of an arming refusal of the --until-stop invocation itself.

package shedverbs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// detailLineBudget caps the lines of a stop's detail.
	detailLineBudget = 40
	// detailByteBudget caps the bytes of a stop's detail.
	detailByteBudget = 8192
	// detailTailLines is how many of a trace's last lines the detail carries beside its WARN and ERROR lines.
	detailTailLines = 10
)

// loopRestep records the immediate re-step the loop made after a transient failure.
type loopRestep struct {
	Producer  string
	Error     string
	Transient string
}

// loopObject is the `loop` object of the loop's envelope; its key set is closed at the keys fields lists.
type loopObject struct {
	Steps           int
	FirstProducer   string
	Stop            LoopStop
	Detail          string
	StatusMoved     bool
	Restep          *loopRestep
	CurrentProducer string
	HistoryLength   int
	State           string
	InterruptPolicy string
	TraceCopy       string
	StderrPath      string
}

// fields renders the object as its envelope keys; restep is omitted when no re-step ran.
func (o loopObject) fields() map[string]any {
	fields := map[string]any{
		"steps":            o.Steps,
		"first_producer":   o.FirstProducer,
		"stop":             string(o.Stop),
		"detail":           o.Detail,
		"status_moved":     o.StatusMoved,
		"current_producer": o.CurrentProducer,
		"history_length":   o.HistoryLength,
		"state":            o.State,
		"interrupt_policy": o.InterruptPolicy,
		"trace_copy":       o.TraceCopy,
		"stderr_path":      o.StderrPath,
	}
	if o.Restep != nil {
		fields["restep"] = map[string]any{"producer": o.Restep.Producer, "error": o.Restep.Error, "transient": o.Restep.Transient}
	}
	return fields
}

// loopEnvelope is the stop step's short envelope fields plus the loop object under `loop`.
func loopEnvelope(step map[string]any, loop loopObject) map[string]any {
	envelope := make(map[string]any, len(step)+1)
	for key, value := range step {
		envelope[key] = value
	}
	envelope["loop"] = loop.fields()
	return envelope
}

// interruptedError returns the message of an interrupted stop and the way-forward clause that follows it.
// The envelope, the status file's error and the refusal row all name that way forward.
func interruptedError(runID, producer, cause string) (string, string) {
	message := fmt.Sprintf("shedverbs: step %s interrupted (%s)", producer, cause)
	wayForward := fmt.Sprintf("read loop.trace_copy and loop.stderr_path, apply the interrupted rule under loop.interrupt_policy, then re-run lyx shed step %s --until-stop", runID)
	return message, wayForward
}

// interruptedRecord writes the error-form full envelope of a step that ended without one, as <traceID>.json in the steps directory.
// It returns the envelope with the path written, which is empty when no record could be kept.
func interruptedRecord(spec *Spec, traceID, errText, traceFile, friction string) (map[string]any, string) {
	locations := StepLocations{TraceFile: traceFile, FrictionDir: spec.FrictionDir, ScratchDir: spec.ScratchDir, TraceID: traceID, RunID: spec.RunID}
	fields := stepErrFields(KindInterrupted, "", friction, locations)
	var buf bytes.Buffer
	output.ErrFields(&buf, errText, fields)
	if spec.StepsDir != "" {
		if err := os.MkdirAll(spec.StepsDir, 0o755); err != nil {
			logger.Warn("shed: loop could not create the steps directory", "dir", spec.StepsDir, "error", err.Error())
		}
	}
	path, _ := newStepRecorder(spec.StepsDir, traceID, buildvcs.Running()).write(buf.Bytes())
	return fields, path
}

// stdoutErrEnvelope decodes the last non-empty line of a child's stdout as an error envelope and fills the short error form's keys it lacks with empty values.
// It reports false when that line is not an error envelope.
func stdoutErrEnvelope(stdout []byte) (map[string]any, bool) {
	lines := strings.Split(strings.TrimRight(string(stdout), "\r\n \t"), "\n")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
		return nil, false
	}
	if succeeded, present := envelope["ok"].(bool); !present || succeeded {
		return nil, false
	}
	for _, key := range []string{"kind", "transient", "run_id", "trace_file"} {
		if _, present := envelope[key]; !present {
			envelope[key] = ""
		}
	}
	return envelope, true
}

// stopDetail composes the detail of a stop: the step's error or reason, the transient class, then the WARN and ERROR lines of the told trace files and their last lines.
// The detail is capped at detailLineBudget lines and detailByteBudget bytes.
func stopDetail(stepErr, reason, transient string, traceFiles []string) string {
	var lines []string
	if stepErr != "" {
		lines = append(lines, "error: "+stepErr)
	}
	if reason != "" {
		lines = append(lines, "reason: "+reason)
	}
	if transient != "" {
		lines = append(lines, "transient: "+transient)
	}
	seen := map[string]bool{}
	for _, file := range traceFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		traceLines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
		for _, line := range traceLines {
			if !seen[line] && (strings.Contains(line, "level=WARN") || strings.Contains(line, "level=ERROR")) {
				seen[line] = true
				lines = append(lines, line)
			}
		}
		tailStart := max(len(traceLines)-detailTailLines, 0)
		for _, line := range traceLines[tailStart:] {
			if !seen[line] {
				seen[line] = true
				lines = append(lines, line)
			}
		}
	}
	return capDetail(lines)
}

// capDetail joins lines, and when they pass the line or byte budget keeps the leading ones and ends the detail with a marker inside the budgets.
func capDetail(lines []string) string {
	const marker = "[detail truncated]"
	joined := strings.Join(lines, "\n")
	if len(lines) <= detailLineBudget && len(joined) <= detailByteBudget {
		return joined
	}
	kept := make([]string, 0, detailLineBudget)
	size := len(marker)
	for _, line := range lines {
		if len(kept) >= detailLineBudget-1 || size+len(line)+1 > detailByteBudget {
			break
		}
		kept = append(kept, line)
		size += len(line) + 1
	}
	return strings.Join(append(kept, marker), "\n")
}

// ArmStop carries what ReportLoopArmError needs to stop a run whose --until-stop invocation failed to arm:
// the run's id and the paths of its status file and of the three locks that say who owns it.
type ArmStop struct {
	RunID          string
	StatusPath     string
	RunLockPath    string
	StatusLockPath string
	LoopLockPath   string
}

// ReportLoopArmError prints the refusal err raised while a --until-stop step was being armed, and returns the exit code.
// A KindlessRefusal prints a bare error line.
// Any other error prints the bootstrap envelope of ReportArmError plus a loop object that stopped on error with no step run.
// It also writes the status file failed when the file exists and no live loop holds the loop lock.
// A held loop lock, or another holder of the run lock, writes nothing and makes the stop busy.
func ReportLoopArmError(out io.Writer, stop ArmStop, err error) int {
	var kindless KindlessRefusal
	if errors.As(err, &kindless) {
		return output.Err(out, err.Error())
	}
	message := bootstrapMessage(err)
	logger.Warn("shed: step arming refused", "error", message)

	obj := loopObject{Stop: LoopStopError, Detail: message}
	if _, present := readStatusAt(stop.StatusPath, stop.StatusLockPath); present {
		if loopLockFree(stop.LoopLockPath) {
			failed, writeErr := shedengine.WriteFailedStop(shedengine.FailedStopRequest{
				StatusPath:     stop.StatusPath,
				LockPath:       stop.RunLockPath,
				StatusLockPath: stop.StatusLockPath,
				Error:          message,
			})
			if writeErr != nil {
				logger.Warn("shed: arming refusal could not write the failed stop", "error", writeErr.Error())
			} else if failed.Busy {
				obj.Stop = LoopStopBusy
			}
		} else {
			obj.Stop = LoopStopBusy
		}
		after, _ := readStatusAt(stop.StatusPath, stop.StatusLockPath)
		obj.CurrentProducer = after.CurrentProducer
		obj.HistoryLength = len(after.History)
		obj.State = string(after.State)
	}
	step := map[string]any{"kind": KindBootstrap, "trace_file": logger.TraceFile()}
	return output.ErrFields(out, message, loopEnvelope(step, obj))
}

// loopLockFree reports whether no loop holds the loop lock at path.
// A lock that cannot be probed counts as held, so nothing is written over a run that may be live.
func loopLockFree(path string) bool {
	held, free, err := lock.TryAcquireWriteLock(path)
	if err != nil || !free {
		return false
	}
	_ = held.Release()
	return true
}
