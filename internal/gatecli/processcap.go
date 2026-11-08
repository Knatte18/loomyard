// processcap.go caps the processes a tier command's run may hold at once,
// so a runaway spawn loop fails the gate instead of freezing the machine.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// runProcessCap is the most processes carrying a tier run's marker that may live at once.
// A tier's test binaries, compilers, git, tmux servers and panes together stay far below it.
// A test binary that re-runs its own suite from a tmux job passes it within minutes.
const runProcessCap = 1024

// processCap kills every process of a tier run once their count passes limit.
type processCap struct {
	limit  int
	period time.Duration
	// marker is the environment entry every process of the run carries.
	marker string
	// list returns the pids whose environment holds marker.
	list func(marker string) ([]int, error)
	// kill sends SIGKILL to pid.
	kill  func(pid int) error
	after func(time.Duration) <-chan time.Time
}

// newProcessCap returns the production cap over the processes carrying marker.
// It scans /proc, kills with SIGKILL and waits on the real clock.
func newProcessCap(marker string, period time.Duration) processCap {
	return processCap{
		limit:  runProcessCap,
		period: period,
		marker: marker,
		list:   proc.PIDsWithEnv,
		kill:   func(pid int) error { return proc.SignalProcess(pid, true) },
		after:  time.After,
	}
}

// run lists the run's processes once per period until ctx ends, and returns processCapLine once the cap tripped.
// It returns "" when the cap never tripped.
// The first list longer than limit trips the cap and kills every listed process.
// Each later list is killed whole too, so a loop that spawns faster than one kill is still ended.
// A platform that cannot list processes ends run after one log line.
func (c processCap) run(ctx context.Context) string {
	line := ""
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return line
		case <-c.after(c.period):
		}
		pids, err := c.list(c.marker)
		if errors.Is(err, errors.ErrUnsupported) {
			logger.Info("gate test: no process cap applies on this platform")
			return line
		}
		if err != nil {
			logger.Debug("gate test: the process cap could not list the run's processes", "cause", err)
			continue
		}
		if line == "" && len(pids) <= c.limit {
			continue
		}
		if line == "" {
			line = processCapLine(len(pids), c.limit)
			logger.Warn("gate test: the run passed the process cap; killing its processes", "processes", len(pids), "cap", c.limit)
		}
		for _, pid := range pids {
			if err := c.kill(pid); err != nil {
				logger.Debug("gate test: the process cap could not kill a process", "pid", pid, "cause", err)
			}
		}
	}
	return line
}

// processCapLine is the line a tier run that passed the process cap prints and logs.
func processCapLine(processes, limit int) string {
	return fmt.Sprintf("gate test: the run held %d processes at once, over the cap of %d, so every process it started was killed and the run fails; "+
		"a test likely hands this test binary to tmux or a child as if it were lym, and the binary re-runs its own suite; "+
		"way forward: name the binary in that command through selfexec.LymBinary, then re-run the same command", processes, limit)
}

// runMarker returns the entry of env that marks every process of a tier run, empty when env sets none.
func runMarker(env []string) string {
	marker := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, enclosingGateEnv+"=") {
			marker = entry
		}
	}
	return marker
}
