// gate.go installs the process-wide network gate for a lyx process that runs in a hub.

package pausecli

import (
	"context"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
)

// InstallNetworkGate installs the pause gate that holds this process's network git and GitHub calls while a pause covers it.
// The hub is the one the context's working directory resolves to; outside a hub nothing is installed.
// The gate is told the process's strand name, pid and start time, so its in-flight markers tell a live process from a dead one, and reads whether a live parent lock record names the slug it is asked about.
func InstallNetworkGate(ctx context.Context) {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return
	}
	pid := os.Getpid()
	startTime, _ := proc.StartTime(pid)
	locks := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(location.HubPath))}
	pauserecord.InstallGate(pauserecord.Gate{
		Geometry:  geometry,
		Strand:    os.Getenv(agentname.StrandNameEnv),
		PID:       pid,
		StartTime: startTime,
		HoldsParentLock: func(slug string) bool {
			entries, err := locks.List()
			if err != nil {
				logger.Warn("pausecli: the parent locks cannot be listed, so the gate reads none held", "error", err)
				return false
			}
			for _, entry := range entries {
				if entry.Err == "" && entry.Alive && entry.Record.Owner.Slug == slug {
					return true
				}
			}
			return false
		},
		Notify: func(line string) {
			fmt.Fprintln(os.Stderr, line)
			logger.Info("pausecli: a network call is held by a pause", "line", line)
		},
	})
}
