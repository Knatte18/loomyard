// run.go runs one `lyx gate test`: it resolves hub membership, takes or inherits a gate slot, runs go test inside it and turns a terminating signal into a clean exit.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// siteLabelPrefix starts every holder and wait record this verb writes, so `lyx loom status` names the command that holds or waits for a slot.
const siteLabelPrefix = "lyx gate test"

// runTest runs request and returns the verb's exit code: go test's own, SlotBusyExit for a hub whose slots stay held past its wait bound, 128 plus the signal number after a terminating signal, and 1 for a refusal.
func runTest(ctx context.Context, out io.Writer, request testRequest) int {
	if len(request.packages) == 0 {
		return refusePackages(out)
	}
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate test: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	dir, err := absoluteDir(cwd, request.dir)
	if err != nil {
		return output.Err(out, err.Error())
	}

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()

	location, inHub, err := resolveHub(dir)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate test: %v; way forward: re-run the same command", err))
	}

	// The tests run without the strand name, so a test driving `lyx fabric` never meets a role guard that reads it.
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, agentname.StrandNameEnv+"=") })
	var parallel int
	if inHub {
		pool := hubgeom.GateSlots(location)
		limits, err := pool.Limits()
		if errors.Is(err, gateslot.ErrConfigAbsent) {
			return output.Err(out, fmt.Sprintf("gate test: the hub has no gate limits: %v; way forward: run \"lyx fabric reconcile\", then re-run the same command", err))
		}
		if err != nil {
			return output.Err(out, fmt.Sprintf("gate test: cannot read the hub's gate limits: %v; way forward: fix the file with \"lyx config gate\" from the prime, then re-run the same command", err))
		}
		parallel = limits.GoParallel
		if inherited := os.Getenv(gateslot.InheritEnv); inherited != "" && pool.Inherited(inherited) {
			logger.Info("gate test: running inside the slot an enclosing gate run holds", "slot", inherited, "dir", dir)
		} else {
			lease, code := acquireSlot(runCtx, out, pool, limits, location, request)
			if lease == nil {
				return exitAfterSignal(caught, code)
			}
			defer releaseSlot(lease, dir)
			env = lease.Env(env)
		}
	} else {
		template, err := gateslot.TemplateConfig()
		if err != nil {
			return output.Err(out, fmt.Sprintf("gate test: cannot read the gate template: %v", err))
		}
		parallel = template.GoParallel
		logger.Info("gate test: no hub bound applies; running unslotted under the template's -p cap", "dir", dir, "parallel", parallel)
	}

	cmd := exec.CommandContext(runCtx, request.goBinary, goTestArgs(dir, parallel, request.tags, request.packages, request.flags)...)
	cmd.Env = env
	cmd.Stdout = out
	cmd.Stderr = out
	logger.Info("gate test: spawning go test", "binary", request.goBinary, "args", strings.Join(cmd.Args[1:], " "))
	runErr := runChild(cmd)
	code := exitCodeOf(runErr)
	logger.Info("gate test: go test ended", "exit", code, "error", runErr)
	if cmd.Process == nil && caught.Load() == 0 {
		return output.Err(out, fmt.Sprintf("gate test: could not start %s: %v; way forward: put go on PATH, then re-run the same command", request.goBinary, runErr))
	}
	return exitAfterSignal(caught, code)
}

// resolveHub returns the location of the hub worktree that contains dir, and whether dir is inside a hub at all.
// A dir outside every git worktree, or in a worktree whose hub carries no board, is outside every hub.
func resolveHub(dir string) (*lyxcwd.Location, bool, error) {
	location, err := lyxcwd.ResolveWorktree(dir)
	if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve the worktree of %s: %w", dir, err)
	}
	if !preflight.BoardLyxPresent(location) {
		return nil, false, nil
	}
	return location, true, nil
}

// acquireSlot waits for a slot of pool, keeping a wait record in the worktree's wait directory until the acquire returns, for at most limits.CLIWait.
// It returns the held lease, or a nil lease and the exit code to return: SlotBusyExit with the holders named once the wait bound passes, and 1 for any other failure, a cancelled ctx included.
func acquireSlot(ctx context.Context, out io.Writer, pool *gateslot.Pool, limits gateslot.Limits, location *lyxcwd.Location, request testRequest) (*gateslot.Lease, int) {
	site := siteLabelPrefix + " " + strings.Join(request.packages, " ")
	waitPath, err := gateslot.WriteWait(gateslot.WaitDir(location.AnchorPath()), gateslot.Wait{Site: site, PID: os.Getpid(), Started: time.Now()})
	if err != nil {
		logger.Warn("gate test: could not write the gate wait record", "cause", err)
	} else {
		defer os.Remove(waitPath)
	}

	waitCtx, cancel := context.WithTimeout(ctx, limits.CLIWait)
	defer cancel()
	lease, err := pool.Acquire(waitCtx, gateslot.Holder{Worktree: location.WorktreePath(), Site: site})
	switch {
	case err == nil:
		return lease, 0
	case ctx.Err() != nil:
		return nil, 1
	case errors.Is(err, context.DeadlineExceeded):
		return nil, refuseSlotBusy(out, pool, limits)
	default:
		return nil, output.Err(out, fmt.Sprintf("gate test: cannot acquire a gate slot: %v; way forward: re-run the same command", err))
	}
}

// refuseSlotBusy writes the slot-busy refusal naming each holder's worktree and site, and returns SlotBusyExit.
func refuseSlotBusy(out io.Writer, pool *gateslot.Pool, limits gateslot.Limits) int {
	holders, err := pool.Holders()
	if err != nil {
		logger.Warn("gate test: could not read the gate holders", "cause", err)
	}
	named := make([]map[string]string, 0, len(holders))
	described := make([]string, 0, len(holders))
	for _, holder := range holders {
		named = append(named, map[string]string{"worktree": holder.Worktree, "site": holder.Site})
		described = append(described, fmt.Sprintf("%s (%s)", holder.Worktree, holder.Site))
	}
	message := fmt.Sprintf("gate test: no gate slot freed within %s; held by %s; way forward: re-run the same command", limits.CLIWait, strings.Join(described, ", "))
	output.ErrFields(out, message, map[string]any{"holders": named})
	return SlotBusyExit
}

func releaseSlot(lease *gateslot.Lease, dir string) {
	if err := lease.Release(); err != nil {
		logger.Warn("gate test: could not release the gate slot", "dir", dir, "cause", err)
	}
}

// catchTerminatingSignals returns a context cancelled by the first terminating signal, the signal's number once one was caught (zero before), and the function that stops the catching.
func catchTerminatingSignals(ctx context.Context) (context.Context, *atomic.Int32, func()) {
	runCtx, cancel := context.WithCancel(ctx)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, terminatingSignals...)
	var caught atomic.Int32
	go func() {
		select {
		case received := <-signals:
			if number, ok := received.(syscall.Signal); ok {
				caught.Store(int32(number))
			}
			cancel()
		case <-runCtx.Done():
		}
	}()
	return runCtx, &caught, func() {
		signal.Stop(signals)
		cancel()
	}
}

// exitAfterSignal returns 128 plus the caught signal's number when a terminating signal ended the run, and code otherwise.
func exitAfterSignal(caught *atomic.Int32, code int) int {
	if number := caught.Load(); number != 0 {
		return 128 + int(number)
	}
	return code
}

// exitCodeOf returns the exit code of a finished child: its own, 1 for a child that did not exit normally, and 0 for none.
func exitCodeOf(runErr error) int {
	if runErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() >= 0 {
		return exitErr.ExitCode()
	}
	return 1
}
