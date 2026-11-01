// detachrun.go implements the hidden `lym gate run-detached` verb, the runner of a gate call that outlasts its caller, and the run directory it keeps its state in.

package gatecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

// detachDirName is the subpath of the gate directory that holds the detached runs, one directory per gate key and run.
const detachDirName = "detach"

// The files of one detached run's directory.
const (
	// requestFile holds the detachRequest the runner runs.
	requestFile = "request.json"
	// leaseFile holds the deadline the runner stops at unless a call renews it.
	leaseFile = "lease.json"
	// processFile holds the pid and start reading of the runner that claimed the run.
	processFile = "process.json"
	// phaseFile holds the runPhase the run is in.
	phaseFile = "phase.json"
	// lineFile holds the proc.TreeRecord of the card line a card-verify run is running.
	lineFile = "line.json"
	// stdoutFile and stderrFile receive the call's output.
	stdoutFile = "stdout"
	stderrFile = "stderr"
	// exitFile holds the detachExit, the runner's last write.
	exitFile = "exit.json"
)

// The states a run's phase file names.
const (
	phaseStarting = "starting"
	phaseSlots    = "slots"
	phaseRunning  = "running"
)

// The verbs a request may name.
const (
	// verbTest runs `lym gate test` over the request's words.
	verbTest = "test"
	// verbCardVerify runs the `**Verify:**` line of the one card the request's words name.
	verbCardVerify = "card-verify"
)

// processReadAttempts and processReadRetryPeriod bound how often and how long apart a reader re-reads a process file that does not decode yet.
const (
	processReadAttempts    = 5
	processReadRetryPeriod = 20 * time.Millisecond //lym:one-shot bounded re-read of at most processReadAttempts
)

// leaseReadPeriod is how often the runner reads its lease; it is the one-second floor of the wake rule.
const leaseReadPeriod = time.Second

// abandonedLine returns the line appended to the stderr of a run of verb when its lease passed before the call ended.
func abandonedLine(verb string) string {
	return gateVerbName(verb) + ": the run was abandoned, since no call renewed its lease; way forward: run the same command again\n"
}

// detachRun is one detached run's directory.
type detachRun struct {
	// dir is the run's directory, `<gate dir>/detach/<key>/<run id>`.
	dir string
}

// detachRequest is the call a runner runs: the verb, the words after it and the directory it ran in.
type detachRequest struct {
	// Verb is `test` or `card-verify`.
	Verb string `json:"verb"`
	// Args are the call's words after the verb.
	Args []string `json:"args"`
	// Cwd is the absolute directory the call resolves against.
	Cwd string `json:"cwd"`
}

// runPhase says where a run stands.
type runPhase struct {
	// State is `starting`, `slots` or `running`.
	State string `json:"state"`
	// Tier names the tier a `slots` or `running` state is for, empty for a run that has not reached one.
	Tier string `json:"tier,omitempty"`
	// Line names the card line a `running` state is for, empty for a tier run.
	Line string `json:"line,omitempty"`
}

// detachExit is a run's result: the call's exit code, and whether the run was cut short.
type detachExit struct {
	// Code is the call's exit code.
	Code int `json:"code"`
	// Abandoned is set when a passed lease or a terminating signal ended the call.
	Abandoned bool `json:"abandoned,omitempty"`
}

// leaseRecord is the content of a run's lease file.
type leaseRecord struct {
	Deadline time.Time `json:"deadline"`
}

// processRecord is the content of a run's process file.
type processRecord struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
}

// path returns the path of the run's file name.
func (d detachRun) path(name string) string {
	return filepath.Join(d.dir, name)
}

// writeLease makes deadline the lease's deadline unless a later one is stored.
// The lease is thus the latest deadline any call wrote, though calls of different binaries rewrite it.
func (d detachRun) writeLease(deadline time.Time) error {
	path := d.path(leaseFile)
	return state.UpdateJSON(path, path+".lock", func(current leaseRecord, found bool) (leaseRecord, error) {
		if found && current.Deadline.After(deadline) {
			return current, nil
		}
		return leaseRecord{Deadline: deadline}, nil
	})
}

// expireLease moves the lease's deadline to a second before now, the one write that moves it earlier.
func (d detachRun) expireLease(now time.Time) error {
	path := d.path(leaseFile)
	return state.UpdateJSON(path, path+".lock", func(leaseRecord, bool) (leaseRecord, error) {
		return leaseRecord{Deadline: now.Add(-time.Second)}, nil
	})
}

// leaseHolds reports whether a lease exists and lies after now.
// A lease that cannot be read holds nothing.
func (d detachRun) leaseHolds(now time.Time) bool {
	path := d.path(leaseFile)
	lease, found, err := state.ReadJSON[leaseRecord](path, path+".lock")
	return err == nil && found && lease.Deadline.After(now)
}

// leaseLapsed reports whether a lease was read and no longer holds at now, or none exists.
// A lease that cannot be read has not lapsed.
func (d detachRun) leaseLapsed(now time.Time) bool {
	path := d.path(leaseFile)
	lease, found, err := state.ReadJSON[leaseRecord](path, path+".lock")
	if err != nil {
		return false
	}
	return !found || !lease.Deadline.After(now)
}

// claim records pid and its start reading as the run's runner, failing when a runner already did.
// The create is exclusive, so of two claims the second fails with an error matching os.ErrExist.
// The record is written after the create, so a reader that finds the file undecodable reads it again.
func (d detachRun) claim(pid int, start string) error {
	data, err := json.Marshal(processRecord{PID: pid, Start: start})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(d.path(processFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}

// readProcessRecord reads the run's process file and reports false for an absent file.
// A file that exists but does not decode is a claim caught between its create and its write, so it is read again a few times before it counts as absent.
func (d detachRun) readProcessRecord() (processRecord, bool) {
	for attempt := 0; ; attempt++ {
		record, found := readRunFile[processRecord](d.path(processFile))
		if found || attempt == processReadAttempts {
			return record, found
		}
		if _, err := os.Stat(d.path(processFile)); err != nil {
			return record, false
		}
		time.Sleep(processReadRetryPeriod)
	}
}

// writePhase records where the run stands, logging a failure since the phase only feeds a status display.
func (d detachRun) writePhase(phase runPhase) {
	data, err := json.Marshal(phase)
	if err == nil {
		err = fsx.AtomicWriteBytes(d.path(phaseFile), data)
	}
	if err != nil {
		logger.Warn("gate run-detached: could not write the phase", "dir", d.dir, "cause", err)
	}
}

// writeExit records the run's result; it is the runner's last write.
func (d detachRun) writeExit(exit detachExit) error {
	data, err := json.Marshal(exit)
	if err != nil {
		return err
	}
	return fsx.AtomicWriteBytes(d.path(exitFile), data)
}

// startReading reads a start for pid that tells a reused pid apart from the process that held it.
// It is proc.StartTime's reading off Windows, and proc.CreationTime in Unix nanoseconds as a decimal string on Windows, where StartTime is unreadable.
func startReading(goos string, pid int, startTime func(int) (string, bool), creationTime func(int) (time.Time, bool)) (string, bool) {
	if goos != "windows" {
		return startTime(pid)
	}
	created, ok := creationTime(pid)
	if !ok {
		return "", false
	}
	return strconv.FormatInt(created.UnixNano(), 10), true
}

// phaseKey keys the phase hook in a context.
type phaseKey struct{}

// withPhase returns ctx carrying hook, which a call's tier runs report their phase to.
func withPhase(ctx context.Context, hook func(runPhase)) context.Context {
	return context.WithValue(ctx, phaseKey{}, hook)
}

// phaseFrom returns the hook ctx carries, nil outside a runner.
func phaseFrom(ctx context.Context) func(runPhase) {
	hook, _ := ctx.Value(phaseKey{}).(func(runPhase))
	return hook
}

// runnerCmd builds `lym gate run-detached <run-dir>`, the hidden verb that runs a front's request and keeps its state in the run directory.
// gateTree builds a fresh gate command tree for the call to run in.
func runnerCmd(gateTree func() *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:         "run-detached <run-dir>",
		Short:       "run a gate call a front started, until its lease passes",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `run-detached runs the request a lym gate call wrote into its run directory, in this process, and stops at the run's lease.
It is started by lym gate test and lym gate card-verify and refuses a run directory that holds no live lease, so a hand call adds at most one run, bounded by the lease it found.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runDetached(cmd.Context(), cmd.OutOrStdout(), args[0], gateTree))
			return nil
		},
	}
}

// detachRefusal writes the refusal of the run directory dir, which reason describes, naming the gate verbs that start a run of their own.
func detachRefusal(out io.Writer, dir, reason string) int {
	return output.Err(out, fmt.Sprintf("gate run-detached: %s %s; way forward: run lym gate test or lym gate card-verify, which start and join their own runner", dir, reason))
}

// runDetached runs the request in runDir and returns 0 once the run's exit file is written.
// It refuses, before it runs anything or writes an exit file, a directory with no live lease, a request that is missing or malformed, a verb it does not run and a run another runner claimed.
// While the call runs it reads the lease once a second and cancels the call when the lease has passed; a terminating signal cancels it too.
func runDetached(ctx context.Context, out io.Writer, runDir string, gateTree func() *cobra.Command) int {
	run := detachRun{dir: runDir}
	if !run.leaseHolds(time.Now()) {
		return detachRefusal(out, runDir, "holds no live lease of a gate call")
	}
	request, err := run.readRequest()
	if err != nil {
		return detachRefusal(out, runDir, fmt.Sprintf("holds a request that cannot be read: %v", err))
	}
	if request.Verb != verbTest && request.Verb != verbCardVerify {
		return detachRefusal(out, runDir, fmt.Sprintf("holds a request naming the verb %q, which no runner runs", request.Verb))
	}
	pid := os.Getpid()
	start, _ := startReading(runtime.GOOS, pid, proc.StartTime, proc.CreationTime)
	if err := run.claim(pid, start); err != nil {
		if errors.Is(err, os.ErrExist) {
			return detachRefusal(out, runDir, "is already claimed by another runner")
		}
		return detachRefusal(out, runDir, fmt.Sprintf("cannot be claimed: %v", err))
	}

	run.writePhase(runPhase{State: phaseStarting})
	exit := run.runRequest(ctx, request, gateTree)
	if err := run.writeExit(exit); err != nil {
		logger.Warn("gate run-detached: could not write the exit file", "dir", runDir, "cause", err)
		return 1
	}
	return 0
}

// readRequest reads the run's request and checks the fields a runner depends on.
func (d detachRun) readRequest() (detachRequest, error) {
	data, err := os.ReadFile(d.path(requestFile))
	if err != nil {
		return detachRequest{}, err
	}
	var request detachRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return detachRequest{}, err
	}
	if !filepath.IsAbs(request.Cwd) {
		return detachRequest{}, fmt.Errorf("cwd %q is not an absolute directory", request.Cwd)
	}
	if request.Verb == verbCardVerify && len(request.Args) == 0 {
		return detachRequest{}, errors.New("a card-verify request names no card")
	}
	return request, nil
}

// runRequest runs request, a `test` request through a fresh gate tree and a `card-verify` request through runCard, with its output in the run's files, and returns how it ended.
// The call's context is cancelled by a terminating signal, which ends it as {128 + signal, abandoned}, and by a passed lease, which ends it as {1, abandoned} with abandonedLine appended to stderr.
func (d detachRun) runRequest(ctx context.Context, request detachRequest, gateTree func() *cobra.Command) detachExit {
	stdout, stdoutErr := os.Create(d.path(stdoutFile))
	stderr, stderrErr := os.Create(d.path(stderrFile))
	if err := errors.Join(stdoutErr, stderrErr); err != nil {
		logger.Warn("gate run-detached: could not open the output files", "dir", d.dir, "cause", err)
		return detachExit{Code: 1}
	}
	defer stdout.Close()
	defer stderr.Close()

	signalCtx, caught, stopSignals := catchTerminatingSignals(ctx)
	defer stopSignals()
	callCtx, cancelCall := context.WithCancel(signalCtx)
	defer cancelCall()
	var leasePassed atomic.Bool
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		d.cancelOnPassedLease(callCtx, cancelCall, &leasePassed)
	}()

	runCtx := withPhase(lymcwd.WithCwd(callCtx, request.Cwd), d.writePhase)
	var code int
	if request.Verb == verbCardVerify {
		code = d.runCard(runCtx, request, stdout, stderr)
	} else {
		tree := gateTree()
		tree.SetOut(stdout)
		tree.SetErr(stderr)
		tree.SetArgs(append([]string{verbTest, "--wait"}, request.Args...))
		code = clihelp.RunRootCtx(runCtx, tree, stdout)
	}

	// A lease that passes after the call returned abandons nothing, so only a cancel seen before cancelCall counts.
	cancelledDuringCall := callCtx.Err() != nil
	cancelCall()
	<-watchDone
	switch {
	case caught.Load() != 0:
		return detachExit{Code: exitAfterSignal(caught, code), Abandoned: true}
	case cancelledDuringCall && leasePassed.Load():
		if _, err := io.WriteString(stderr, abandonedLine(request.Verb)); err != nil {
			logger.Warn("gate run-detached: could not write the abandoned line", "dir", d.dir, "cause", err)
		}
		return detachExit{Code: 1, Abandoned: true}
	}
	return detachExit{Code: code}
}

// cancelOnPassedLease reads the lease once per leaseReadPeriod and calls cancel, after setting passed, once the lease has lapsed.
// A lease that cannot be read is read again at the next period.
// It returns when ctx ends.
func (d detachRun) cancelOnPassedLease(ctx context.Context, cancel context.CancelFunc, passed *atomic.Bool) {
	ticker := time.NewTicker(leaseReadPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if d.leaseLapsed(time.Now()) {
				passed.Store(true)
				cancel()
				return
			}
		}
	}
}
