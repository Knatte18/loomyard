// open.go holds what `board open` and `board serve` share: the run-agent refusal, the open sequence and the server spawn.

package boardcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// The bounds of the board server and of opening its page.
const (
	serverLockWait  = 5 * time.Second
	serverIdleBound = 30 * time.Minute
	serverPollBound = 50 * time.Second
	openStartBound  = 15 * time.Second
	openPoll        = time.Second
)

// needsHubConfig is the refusal of a viewer verb run with no resolved worktree; it takes the verb.
const needsHubConfig = "lym board %s needs the hub's board config, which only a run from a worktree of the hub resolves: way forward: run it from a worktree of the hub, without --board-path"

// refuseRunAgent refuses verb for the session of a run's agent; an orch's strand name and an empty one pass.
func refuseRunAgent(verb, strand string) error {
	if !boardengine.IsRunAgentSession(strand) {
		return nil
	}
	return fmt.Errorf("the run session %s cannot run lym board %s: the board viewer and its server are the operator's tools, "+
		"and a server an agent started would carry its session into the operator's page: way forward: ask your orch or the operator to run lym board open", strand, verb)
}

// openDeps is what opening the board page reaches outside itself.
type openDeps struct {
	Paths boardserve.StatePaths
	// Prime resolves the prime's anchored directory, where a server runs.
	Prime func() (string, error)
	// Spawn starts a detached server in dir, logging to logPath.
	Spawn func(dir, logPath string) error
	// Probe asks the server of a state for its board.
	Probe func(boardserve.State) error
	// Running reports whether a process runs.
	Running func(pid int) bool
	// StartOpener opens path in a browser, and delivers the opener's exit.
	StartOpener func(path string) (<-chan error, error)
	// StartBound is the longest wait for a server to answer, and Poll the time between tries.
	StartBound time.Duration
	Poll       time.Duration
	// OpenerCap is the longest wait for the opener to exit; zero waits until it does.
	OpenerCap time.Duration
}

// openResult is what opening the board page did.
type openResult struct {
	// Address is the page's address, with no token.
	Address string
	// Launcher is the file the opener was given.
	Launcher string
	// Started reports whether a server was started.
	Started bool
}

// runOpen makes sure the board server runs, writes its launcher and opens it.
func runOpen(deps openDeps) (openResult, error) {
	live, err := boardserve.Live(deps.Paths)
	if err != nil {
		return openResult{}, err
	}
	if !live {
		anchor, err := deps.Prime()
		if err != nil {
			return openResult{}, err
		}
		if err := deps.Spawn(anchor, deps.Paths.Log); err != nil {
			return openResult{}, fmt.Errorf("start the board server: %w: way forward: check that lym runs from this shell and read the server log %s, then run lym board open again", err, deps.Paths.Log)
		}
	}

	// A state outlives its server, so it counts only while the lock is held and its own process answers its token.
	var server boardserve.State
	deadline := time.Now().Add(deps.StartBound)
	for {
		held, err := boardserve.Live(deps.Paths)
		if err != nil {
			return openResult{}, err
		}
		if held {
			recorded, found, err := boardserve.ReadState(deps.Paths)
			if err != nil {
				return openResult{}, err
			}
			if found && deps.Running(recorded.PID) && deps.Probe(recorded) == nil {
				server = recorded
				break
			}
		}
		if !time.Now().Before(deadline) {
			return openResult{}, fmt.Errorf("the board server did not answer within %s; its log %s opens with its pid: way forward: end that process if it hangs, then run lym board open again", deps.StartBound, deps.Paths.Log)
		}
		time.Sleep(deps.Poll)
	}

	launcher, err := boardserve.WriteLauncher(deps.Paths, server)
	if err != nil {
		return openResult{}, err
	}
	openerFailed := func(err error) error {
		return fmt.Errorf("the opener %s failed: %w: way forward: open %s in a browser by hand", openerName, err, launcher)
	}
	exited, err := deps.StartOpener(launcher)
	if err != nil {
		return openResult{}, openerFailed(err)
	}
	var capped <-chan time.Time
	if deps.OpenerCap > 0 {
		capped = time.After(deps.OpenerCap)
	}
	select {
	case err := <-exited:
		if err != nil {
			return openResult{}, openerFailed(err)
		}
	case <-capped:
		// The opener is left running: some stay up as long as the browser they started.
	}
	return openResult{Address: boardserve.PageAddress(server.Port), Launcher: launcher, Started: !live}, nil
}

// spawnServer starts a detached `lym board serve` in dir, its errors appended to the file at logPath.
func spawnServer(dir, logPath string) error {
	binary, err := selfexec.LymBinary()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	// The child holds its own descriptor once started.
	defer func() { _ = log.Close() }()

	cmd := exec.Command(binary, "board", "serve")
	cmd.Dir = dir
	cmd.Stderr = log
	proc.Detach(cmd)
	logger.Info("boardcli: starting the board server", "dir", dir, "log", logPath)
	if err := cmd.Start(); err != nil {
		return err
	}
	proc.Reap(cmd)
	return nil
}
