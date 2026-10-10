// loopwait.go implements the --until-stop invocation: the waiter that starts or follows the detached loop and prints its envelope, and the stop it reports for a loop that is gone.
// The loop itself, which the waiter spawns, is runLoop in loop.go.

package shedverbs

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// loopLockGrace is how long a starting loop waits for the loop lock before it gives up.
	// A liveness probe holds the lock for an instant, so the wait must outlast one.
	loopLockGrace = 5 * time.Second
	// waitPollFloor is the waiter's first wait, and the wait it returns to when something changed.
	waitPollFloor = time.Second
	// waitPollCeiling is the longest wait the waiter backs off to while nothing changes.
	waitPollCeiling = 30 * time.Second
)

// loopFollow is the loop a waiter follows.
type loopFollow struct {
	// id is the followed loop's id, empty until a loop is adopted.
	id string
	// spawned is closed when the process the waiter itself spawned exits; nil for a loop the waiter found already running.
	spawned <-chan struct{}
}

// waitEnd is how a wait on a loop ended.
type waitEnd int

const (
	// waitDelivered means an envelope carrying the followed loop's id is in hand.
	waitDelivered waitEnd = iota
	// waitDead means the loop left a pid record and is gone without an envelope, or the teardown's mark stands.
	waitDead
	// waitExitedEarly means the loop is gone and never recorded itself.
	waitExitedEarly
	// waitRedecide means the lock read free before any loop was adopted, so the invocation decides again.
	waitRedecide
)

// loopWaitResult is what a wait on a loop found.
type loopWaitResult struct {
	end waitEnd
	// file is the envelope of a delivered end.
	file loopEnvelopeFile
	// retired is true when file came from the delivered record, which another invocation already retired.
	retired bool
	// record is the pid record of a dead end.
	record LoopPIDRecord
}

// runUntilStop is the --until-stop invocation: it prints the one envelope of the run's loop, starting the detached loop first when none runs.
// argv is the command line a loop's child steps run, which the detached loop receives with the flags that select it.
// It first reads the loop lock, the pid file and the envelope file:
// a free lock beside the teardown's mark, or beside a pid file and no envelope, reports a dead loop;
// a held lock follows the loop the pid file names, or waits for a pid file;
// a free lock beside an envelope delivers it while it is current and retires it undelivered otherwise;
// anything else spawns a loop and follows it.
func runUntilStop(ctx context.Context, spec *Spec, argv []string, out io.Writer) int {
	loop := spec.Loop
	rerun := fmt.Sprintf("lyx shed step %s --until-stop", spec.RunID)
	if err := os.MkdirAll(filepath.Dir(loop.EnvelopePath), 0o755); err != nil {
		return output.Err(out, fmt.Sprintf("shedverbs: create the loop directory: %v; way forward: re-run %s", err, rerun))
	}
	for {
		lockFree := loopLockFree(loop.LockPath)
		record, hasRecord, err := ReadLoopPIDRecord(loop.PIDPath)
		if err != nil {
			return output.Err(out, fmt.Sprintf("%v; way forward: delete %s once no loop of this run is alive, then re-run %s", err, loop.PIDPath, rerun))
		}
		file, hasEnvelope := readLoopEnvelopeFile(loop.EnvelopePath)

		var follow loopFollow
		switch {
		case lockFree && hasRecord && record.LoopID == TeardownLoopID:
			return deadLoopStop(ctx, spec, record, out)
		case !lockFree && hasRecord:
			follow.id = record.LoopID
		case !lockFree:
		case hasRecord && !hasEnvelope:
			return deadLoopStop(ctx, spec, record, out)
		case hasEnvelope:
			status, present := readStatusFile(spec)
			if present && envelopeCurrent(file.Envelope, status) {
				return deliverEnvelope(spec, out, file)
			}
			logger.Info("shed: retiring a stale loop envelope", "loop_id", file.LoopID)
			retireEnvelope(spec, file)
			continue
		default:
			loopID := logger.NewTraceID()
			spawn := loop.Spawn
			if spawn == nil {
				spawn = func(spawnArgv []string) (<-chan struct{}, error) { return spawnLoop(spec, spawnArgv) }
			}
			spawned, err := spawn(append(slices.Clone(argv), "--"+UntilStopFlag, "--"+LoopDetachedFlag+"="+loopID))
			if err != nil {
				return output.Err(out, fmt.Sprintf("%v; way forward: re-run %s", err, rerun))
			}
			follow = loopFollow{id: loopID, spawned: spawned}
		}

		result, err := waitLoopEnvelope(ctx, spec, &follow)
		if err != nil {
			return output.Err(out, fmt.Sprintf("shedverbs: stopped waiting for the loop of run %s: %v; way forward: the loop keeps running, re-run %s to wait for it again", spec.RunID, err, rerun))
		}
		switch result.end {
		case waitDelivered:
			if result.retired {
				return printEnvelope(out, result.file)
			}
			return deliverEnvelope(spec, out, result.file)
		case waitDead:
			return deadLoopStop(ctx, spec, result.record, out)
		case waitExitedEarly:
			return loopExitedEarlyStop(ctx, spec, follow.id, out)
		}
	}
}

// spawnLoop starts the detached loop: spec.Loop.Executable with argv, its output appended to the loop log, and configured to outlive the waiter.
// A Windows start refused for breakaway is retried without it, so such a loop lives only as long as the shell's job.
// The channel it returns is closed when the loop process exits, which the spawn reaps.
func spawnLoop(spec *Spec, argv []string) (<-chan struct{}, error) {
	loop := spec.Loop
	if err := os.MkdirAll(filepath.Dir(loop.LogPath), 0o755); err != nil {
		return nil, fmt.Errorf("shedverbs: create the loop log directory: %w", err)
	}
	logFile, err := os.OpenFile(loop.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("shedverbs: open the loop log: %w", err)
	}
	defer logFile.Close()

	start := func(detach func(*exec.Cmd)) (*exec.Cmd, error) {
		cmd := exec.Command(loop.Executable, argv...)
		cmd.Stdout, cmd.Stderr = logFile, logFile
		detach(cmd)
		return cmd, cmd.Start()
	}
	cmd, err := start(proc.DetachBreakaway)
	if err != nil && proc.IsBreakawayRefused(err) {
		logger.Warn("shed: the loop spawn was refused breakaway, so it lives only as long as the shell's job", "error", err.Error())
		cmd, err = start(proc.Detach)
	}
	if err != nil {
		return nil, fmt.Errorf("shedverbs: start the loop %q: %w", loop.Executable, err)
	}
	pid := cmd.Process.Pid
	logger.Info("shed: loop spawned", "pid", pid, "executable", loop.Executable, "log", loop.LogPath)
	exited := make(chan struct{})
	go func() {
		waitErr := cmd.Wait()
		logger.Info("shed: spawned loop exited", "pid", pid, "error", errorText(waitErr))
		close(exited)
	}()
	return exited, nil
}

// loopSnapshot is what one look at the loop's files and lock found.
type loopSnapshot struct {
	lockFree     bool
	record       LoopPIDRecord
	hasRecord    bool
	envelope     loopEnvelopeFile
	hasEnvelope  bool
	delivered    loopEnvelopeFile
	hasDelivered bool
	spawnExited  bool
}

// takeSnapshot looks at the loop lock and the pid file first, then the envelope file and the followed loop's delivered record, and the waiter's own spawn last.
// That order is what makes a loop that finished between two looks show up as a delivered envelope and never as a gone one:
// a loop writes its envelope before it removes its pid file and releases its lock.
func takeSnapshot(loop LoopSpec, follow loopFollow) loopSnapshot {
	snap := loopSnapshot{lockFree: loopLockFree(loop.LockPath)}
	record, found, err := ReadLoopPIDRecord(loop.PIDPath)
	if err != nil {
		logger.Warn("shed: the waiter could not read the loop pid file", "error", err.Error())
	} else {
		snap.record, snap.hasRecord = record, found
	}
	snap.envelope, snap.hasEnvelope = readLoopEnvelopeFile(loop.EnvelopePath)
	if validLoopID(follow.id) {
		snap.delivered, snap.hasDelivered = readLoopEnvelopeFile(loop.Delivered(follow.id))
	}
	if follow.spawned != nil {
		select {
		case <-follow.spawned:
			snap.spawnExited = true
		default:
		}
	}
	return snap
}

// key renders the snapshot so two looks can be compared for a change.
func (s loopSnapshot) key(followedID string) string {
	return fmt.Sprint(followedID, s.lockFree, s.record, s.hasRecord, s.envelope.LoopID, s.hasEnvelope, s.delivered.LoopID, s.hasDelivered, s.spawnExited)
}

// waitLoopEnvelope waits on the followed loop until it can say how the wait ended.
// It looks again on a timer that starts at waitPollFloor, doubles up to waitPollCeiling while nothing changes and returns to the floor on a change;
// a file event in the steps directory, or the exit of the waiter's own spawn, only cuts the current wait short, so a loop that dies without touching a file is still seen at the next tick.
// It returns the context's error when the wait is cancelled.
func waitLoopEnvelope(ctx context.Context, spec *Spec, follow *loopFollow) (loopWaitResult, error) {
	loop := spec.Loop
	events, stopWatching := watchLoopFiles(loop)
	defer stopWatching()

	delay := waitPollFloor
	previous := ""
	for {
		snap := takeSnapshot(loop, *follow)
		if result, done := judgeSnapshot(follow, snap); done {
			return result, nil
		}
		key := snap.key(follow.id)
		if key == previous {
			delay = min(delay*2, waitPollCeiling)
		} else {
			delay = waitPollFloor
		}
		previous = key

		var spawnExit <-chan struct{}
		if !snap.spawnExited {
			spawnExit = follow.spawned
		}
		if err := pauseWait(ctx, loop, delay, events, spawnExit); err != nil {
			return loopWaitResult{}, err
		}
	}
}

// judgeSnapshot decides from one look whether the wait is over, and adopts another loop's id into follow where the look shows the followed loop was replaced.
// A loop's envelope, or its delivered record, ends the wait first; a free lock beside the teardown's mark ends every wait;
// otherwise a held lock keeps waiting, and a free lock ends it unless the waiter's own spawn is still starting.
func judgeSnapshot(follow *loopFollow, snap loopSnapshot) (loopWaitResult, bool) {
	if follow.id != "" {
		if snap.hasEnvelope && snap.envelope.LoopID == follow.id {
			return loopWaitResult{end: waitDelivered, file: snap.envelope}, true
		}
		if snap.hasDelivered {
			return loopWaitResult{end: waitDelivered, file: snap.delivered, retired: true}, true
		}
	}
	if snap.lockFree && snap.hasRecord && snap.record.LoopID == TeardownLoopID {
		return loopWaitResult{end: waitDead, record: snap.record}, true
	}

	if follow.id == "" {
		switch {
		case snap.lockFree:
			return loopWaitResult{end: waitRedecide}, true
		case !snap.hasRecord:
			return loopWaitResult{}, false
		}
		follow.id = snap.record.LoopID
		return judgeSnapshot(follow, snap)
	}

	if !snap.lockFree {
		if snap.spawnExited && snap.hasRecord && snap.record.LoopID != follow.id {
			follow.id = snap.record.LoopID
			return judgeSnapshot(follow, snap)
		}
		return loopWaitResult{}, false
	}
	if follow.spawned != nil && !snap.spawnExited {
		return loopWaitResult{}, false
	}
	switch {
	case snap.hasRecord && snap.record.LoopID == follow.id:
		return loopWaitResult{end: waitDead, record: snap.record}, true
	case snap.hasRecord:
		follow.id = snap.record.LoopID
		return judgeSnapshot(follow, snap)
	case snap.hasEnvelope:
		follow.id = snap.envelope.LoopID
		return judgeSnapshot(follow, snap)
	}
	return loopWaitResult{end: waitExitedEarly}, true
}

// pauseWait waits delay, or until a file event or the exit of the waiter's own spawn, or until ctx ends.
// spec.Loop.Sleep, when set, replaces the whole wait.
func pauseWait(ctx context.Context, loop LoopSpec, delay time.Duration, events, spawnExit <-chan struct{}) error {
	if loop.Sleep != nil {
		return loop.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	case <-events:
	case <-spawnExit:
	}
	return nil
}

// watchLoopFiles watches the steps directory for the loop's envelope and pid files.
// It returns a nil channel and a no-op stop where no watch opens, and the waiter then polls on its timer alone.
func watchLoopFiles(loop LoopSpec) (<-chan struct{}, func()) {
	watch := loop.Watch
	if watch == nil {
		watch = watchDirectory
	}
	events, stop, err := watch(filepath.Dir(loop.EnvelopePath), filepath.Base(loop.EnvelopePath), filepath.Base(loop.PIDPath))
	if err != nil || stop == nil {
		logger.Debug("shed: the waiter could not watch the loop directory and polls instead", "error", errorText(err))
		return nil, func() {}
	}
	return events, stop
}

// watchDirectory is the production Watch: it forwards the events of an fswatch watcher as bare wake-ups.
func watchDirectory(dir string, names ...string) (<-chan struct{}, func(), error) {
	watcher, err := fswatch.Watch(dir, names...)
	if err != nil {
		return nil, nil, err
	}
	wake := make(chan struct{}, 1)
	go func() {
		for range watcher.Events() {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	}()
	return wake, func() { _ = watcher.Close() }, nil
}

// envelopeCurrent reports whether a loop's envelope still describes the run: its loop.current_producer, loop.history_length and loop.state match the status file.
func envelopeCurrent(envelope []byte, st shedengine.Status) bool {
	var decoded struct {
		Loop struct {
			CurrentProducer string `json:"current_producer"`
			HistoryLength   int    `json:"history_length"`
			State           string `json:"state"`
		} `json:"loop"`
	}
	if json.Unmarshal(envelope, &decoded) != nil {
		return false
	}
	return decoded.Loop.CurrentProducer == st.CurrentProducer &&
		decoded.Loop.HistoryLength == len(st.History) &&
		decoded.Loop.State == string(st.State)
}

// printEnvelope prints the envelope of file and returns its exit code.
func printEnvelope(out io.Writer, file loopEnvelopeFile) int {
	_, _ = out.Write(append(slices.Clone([]byte(file.Envelope)), '\n'))
	var decoded struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(file.Envelope, &decoded) == nil && decoded.OK {
		return 0
	}
	return 1
}

// deliverEnvelope prints file's envelope and retires it.
func deliverEnvelope(spec *Spec, out io.Writer, file loopEnvelopeFile) int {
	code := printEnvelope(out, file)
	retireEnvelope(spec, file)
	return code
}

// retireEnvelope renames the loop envelope file onto the delivered record of file's loop, so the next invocation starts a fresh loop and a later loop's delivery lands in its own record.
// It leaves the file alone when it has since been replaced by another loop's envelope.
// With the loop lock free it also removes a pid file left beside the envelope by the same loop, since the envelope proves that loop finished; the teardown's mark stays.
func retireEnvelope(spec *Spec, file loopEnvelopeFile) {
	loop := spec.Loop
	if !validLoopID(file.LoopID) {
		logger.Warn("shed: a loop envelope names an unusable loop id and is left in place", "loop_id", file.LoopID)
		return
	}
	if current, found := readLoopEnvelopeFile(loop.EnvelopePath); found && current.LoopID == file.LoopID {
		if err := os.Rename(loop.EnvelopePath, loop.Delivered(file.LoopID)); err != nil && !os.IsNotExist(err) {
			logger.Warn("shed: could not retire a loop envelope", "loop_id", file.LoopID, "error", err.Error())
		}
	}
	record, found, err := ReadLoopPIDRecord(loop.PIDPath)
	if err != nil || !found || record.LoopID != file.LoopID || !loopLockFree(loop.LockPath) {
		return
	}
	if err := os.Remove(loop.PIDPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("shed: could not retire a loop pid file", "loop_id", file.LoopID, "error", err.Error())
	}
}

// deadLoopStop reports a loop that is gone without an envelope, or the teardown's mark, as an interrupted stop with cause loop-exited.
// It first kills what is left of the loop's step tree, then writes the run failed while the status file reads running.
// For a dead loop it then writes the stop as the loop's envelope under its recorded id, delivers it and retires the pid file.
// The teardown's mark writes no envelope and stays, so every invocation until the pair is removed stops the same way and spawns nothing.
// Another holder of the run lock makes the stop busy: nothing is written and the pid file stays for the next invocation to retry.
func deadLoopStop(ctx context.Context, spec *Spec, rec LoopPIDRecord, out io.Writer) int {
	teardown := rec.LoopID == TeardownLoopID
	detail := fmt.Sprintf("loop %s ended without an envelope; its output is in %s", rec.LoopID, spec.Loop.LogPath)
	if teardown {
		detail = "the pair's session end stopped the loop; no loop runs again until the pair is removed"
	} else if killStepTree(rec) {
		detail += "; its step tree was killed"
	}

	envelope, busy := loopExitStop(ctx, spec, rec, detail)
	if busy || teardown {
		_, _ = out.Write(envelope)
		return 1
	}
	file := loopEnvelopeFile{LoopID: rec.LoopID, Envelope: envelope}
	if err := writeLoopEnvelopeFile(spec.Loop.EnvelopePath, rec.LoopID, envelope); err != nil {
		logger.Warn("shed: could not write the dead loop's envelope", "loop_id", rec.LoopID, "error", err.Error())
		_, _ = out.Write(envelope)
		return 1
	}
	return deliverEnvelope(spec, out, file)
}

// loopExitedEarlyStop reports a loop the waiter spawned that exited before it recorded itself, as an interrupted stop with cause loop-exited.
// It writes the run failed while the status file reads running, writes no loop envelope and leaves no pid file to retire.
func loopExitedEarlyStop(ctx context.Context, spec *Spec, loopID string, out io.Writer) int {
	detail := fmt.Sprintf("loop %s exited before it recorded itself; its output is in %s", loopID, spec.Loop.LogPath)
	envelope, _ := loopExitStop(ctx, spec, LoopPIDRecord{LoopID: loopID}, detail)
	_, _ = out.Write(envelope)
	return 1
}

// loopExitStop writes failed over a run a gone loop left reading running and builds the interrupted envelope of that stop.
// A status file in any other state is left as it is and the stop is reported all the same.
// It reports busy, with the busy stop as its envelope, when another holder has the run lock; nothing is written then.
func loopExitStop(ctx context.Context, spec *Spec, rec LoopPIDRecord, detail string) (envelope []byte, busy bool) {
	before, _ := readStatusFile(spec)
	producer := cmp.Or(before.CurrentProducer, rec.CurrentProducer)
	message, wayForward := interruptedError(spec.RunID, producer, "loop-exited")
	failed, err := shedengine.WriteFailedStop(shedengine.FailedStopRequest{
		StatusPath:     spec.StatusPath,
		LockPath:       spec.LockPath,
		StatusLockPath: spec.StatusLockPath,
		Error:          message,
		WayForward:     wayForward,
	})
	if err != nil {
		logger.Warn("shed: could not write the failed stop of a gone loop", "loop_id", rec.LoopID, "error", err.Error())
	}
	after, _ := readStatusFile(spec)
	obj := loopObject{
		FirstProducer:   rec.CurrentProducer,
		Stop:            LoopStopInterrupted,
		Detail:          detail,
		CurrentProducer: after.CurrentProducer,
		HistoryLength:   len(after.History),
		State:           string(after.State),
	}
	obj.StatusMoved = rec.hasChild() && (rec.CurrentProducer != after.CurrentProducer || rec.HistoryLength != len(after.History))
	if spec.Hooks.InterruptPolicyFor != nil {
		obj.InterruptPolicy = spec.Hooks.InterruptPolicyFor(after.CurrentProducer)
	}

	var buf bytes.Buffer
	if failed.Busy {
		text := fmt.Sprintf("shedverbs: the run lock is held, so the dead loop of run %s is not yet stopped; way forward: re-run lyx shed step %s --until-stop once the holder ends", spec.RunID, spec.RunID)
		obj.Stop, obj.Detail = LoopStopBusy, text
		step := map[string]any{"kind": KindBusy, "transient": "", "run_id": spec.RunID, "trace_file": logger.TraceFile(), "envelope_path": ""}
		output.ErrFields(&buf, text, loopEnvelope(step, obj))
		return buf.Bytes(), true
	}

	friction := ""
	if rec.LoopID != TeardownLoopID && spec.Hooks.AfterInterrupt != nil {
		friction = spec.Hooks.AfterInterrupt(ctx, producer, "loop-exited", spec.Loop.LogPath)
	}
	locations := StepLocations{TraceFile: logger.TraceFile(), FrictionDir: spec.FrictionDir, ScratchDir: spec.ScratchDir, TraceID: logger.TraceID(), RunID: spec.RunID}
	step := shortStepErrFields(stepErrFields(KindInterrupted, "", friction, locations), "")
	output.ErrFields(&buf, message+"; way forward: "+wayForward, loopEnvelope(step, obj))
	return buf.Bytes(), false
}
