// watchdog.go samples the process tree of a running go test and sends one SIGQUIT to a test binary that keeps a CPU core busy for the configured window.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// enclosingGateEnv carries the pid of the `lyx gate test` that started go test, so a nested `lyx gate test` can tell the outer run's watchdog already sees its test binaries.
const enclosingGateEnv = "LYX_GATE_TEST_PID"

// defaultSamplePeriod is how often the watchdog samples the child tree.
const defaultSamplePeriod = 5 * time.Second

// busyCoreFraction is the share of one core a sample must use to count as busy.
const busyCoreFraction = 0.9

// testBinarySuffix ends the path of every binary go test builds for a package.
const testBinarySuffix = ".test"

// watchdog quits a test binary that keeps a CPU core busy for window.
type watchdog struct {
	window time.Duration
	period time.Duration
	// sample returns the process tree under root.
	sample func(root int) ([]proc.ProcessCPU, error)
	// signal sends SIGQUIT to pid.
	signal func(pid int) error
	now    func() time.Time
	after  func(time.Duration) <-chan time.Time
	// out receives the line a quit prints.
	out io.Writer
}

// newWatchdog returns the production watchdog: the real process sampler, SIGQUIT and clock, printing to out.
func newWatchdog(window, period time.Duration, out io.Writer) watchdog {
	return watchdog{
		window: window,
		period: period,
		sample: proc.TreeCPU,
		signal: sendQuit,
		now:    time.Now,
		after:  time.After,
		out:    out,
	}
}

// sendQuit sends SIGQUIT to pid, which makes a Go program print every goroutine's stack and exit.
func sendQuit(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGQUIT)
}

// busyTrack is what the watchdog remembers about one sampled test binary.
type busyTrack struct {
	at   time.Time
	cpu  time.Duration
	busy time.Duration
	quit bool
}

// run samples the tree under root once per period until ctx ends.
// A test binary whose CPU use stays at or above 90 percent of one core for window gets one SIGQUIT and the line gateslot.WatchdogLine renders.
// A platform that cannot sample ends run after one log line.
func (w watchdog) run(ctx context.Context, root int) {
	logger.Info("gate test: CPU watchdog started", "window", w.window, "period", w.period)
	tracks := map[int]*busyTrack{}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-w.after(w.period):
		}
		tree, err := w.sample(root)
		if errors.Is(err, errors.ErrUnsupported) {
			logger.Info("gate test: no CPU watchdog applies on this platform")
			return
		}
		if err != nil {
			logger.Warn("gate test: CPU watchdog could not sample the process tree", "cause", err)
			continue
		}
		w.account(tracks, tree)
	}
}

// account folds one sample of the tree into tracks and quits each test binary whose busy time reached the window.
func (w watchdog) account(tracks map[int]*busyTrack, tree []proc.ProcessCPU) {
	now := w.now()
	seen := map[int]bool{}
	for _, sampled := range tree {
		if !strings.HasSuffix(sampled.Path, testBinarySuffix) {
			continue
		}
		seen[sampled.PID] = true
		track, known := tracks[sampled.PID]
		if !known {
			tracks[sampled.PID] = &busyTrack{at: now, cpu: sampled.CPU}
			continue
		}
		elapsed := now.Sub(track.at)
		if float64(sampled.CPU-track.cpu) >= busyCoreFraction*float64(elapsed) {
			track.busy += elapsed
		} else {
			track.busy = 0
		}
		track.at, track.cpu = now, sampled.CPU
		if track.busy >= w.window && !track.quit {
			track.quit = true
			w.quit(sampled)
		}
	}
	for pid := range tracks {
		if !seen[pid] {
			delete(tracks, pid)
		}
	}
}

// quit sends SIGQUIT to the test binary and prints the watchdog line.
func (w watchdog) quit(binary proc.ProcessCPU) {
	if err := w.signal(binary.PID); err != nil {
		logger.Warn("gate test: CPU watchdog could not signal the test binary", "binary", binary.Path, "pid", binary.PID, "cause", err)
		return
	}
	logger.Info("gate test: CPU watchdog sent SIGQUIT", "binary", binary.Path, "pid", binary.PID, "window", w.window)
	fmt.Fprintln(w.out, gateslot.WatchdogLine(filepath.Base(binary.Path), binary.PID, w.window))
}

// watchdogApplies decides whether the watchdog runs for a go test run, and returns the reason it logs.
// It does not run with a window of 0, with -bench or -fuzz among flags, which keep a core busy by intent, or when enclosing names a live ancestor `lyx gate test` whose watchdog already sees this run's test binaries.
// An empty, unparsable or non-ancestor enclosing value is ignored.
func watchdogApplies(window time.Duration, flags []string, enclosing string, isAncestor func(pid int) bool) (bool, string) {
	if window <= 0 {
		return false, "busy_quit_sec is 0"
	}
	for _, flag := range flags {
		name, _, _ := strings.Cut(strings.TrimLeft(flag, "-"), "=")
		switch name {
		case "bench", "test.bench", "fuzz", "test.fuzz":
			return false, "the go test flags carry -bench or -fuzz"
		}
	}
	if pid, err := strconv.Atoi(enclosing); err == nil && isAncestor(pid) {
		return false, fmt.Sprintf("an enclosing lyx gate test (pid %d) already watches this run", pid)
	}
	return true, "busy_quit_sec is set and nothing exempts this run"
}
