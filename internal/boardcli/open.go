// open.go is the work behind `lym board open`: it finds or starts the hub's board server and opens its page in a browser,
// and it holds the refusal both board viewer verbs share.

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

// refuseRunAgent refuses verb for a run agent's strand name; an orch's name and an empty one pass.
func refuseRunAgent(verb, strand string) error {
	if !boardengine.IsRunAgentSession(strand) {
		return nil
	}
	return fmt.Errorf("the run session %s cannot run lym board %s: the board viewer and its server are the operator's tools, "+
		"and a server an agent started would carry its session into the operator's page: way forward: ask your orch or the operator to run lym board open", strand, verb)
}

// openDeps is what runOpen reaches the machine through.
type openDeps struct {
	Paths boardserve.StatePaths
	// Prime resolves the prime's anchored directory, where a server is started.
	Prime   func() (string, error)
	Spawn   func(primeAnchor, logPath string) error
	Probe   func(boardserve.State) error
	Running func(pid int) bool
	// StartOpener opens a file in the default browser and reports the opener's exit on the channel.
	StartOpener func(path string) (<-chan error, error)
	// StartBound is how long a server has to answer, and Poll how often it is asked.
	StartBound time.Duration
	Poll       time.Duration
	// OpenerCap is how long the opener is waited on; zero waits until it exits.
	OpenerCap time.Duration
}

// openResult is what an open did.
type openResult struct {
	// Address is the page's address without the token.
	Address  string
	Launcher string
	// Started reports whether this open started a server.
	Started bool
}

// runOpen opens the page of the hub's board server, starting the server when none runs.
func runOpen(deps openDeps) (openResult, error) {
	live, err := boardserve.Live(deps.Paths)
	if err != nil {
		return openResult{}, err
	}
	if !live {
		primeAnchor, err := deps.Prime()
		if err != nil {
			return openResult{}, err
		}
		if err := deps.Spawn(primeAnchor, deps.Paths.Log); err != nil {
			return openResult{}, fmt.Errorf("start the board server: %w: way forward: check that lym runs from this shell and read the server log %s, then run lym board open again", err, deps.Paths.Log)
		}
	}

	// A state left by an earlier server may still be on disk, so a state counts only once its server answers.
	var server boardserve.State
	bound := time.After(deps.StartBound)
	for answered := false; !answered; {
		held, err := boardserve.Live(deps.Paths)
		if err != nil {
			return openResult{}, err
		}
		if held {
			read, found, err := boardserve.ReadState(deps.Paths)
			if err != nil {
				return openResult{}, err
			}
			if found && deps.Running(read.PID) && deps.Probe(read) == nil {
				server, answered = read, true
				continue
			}
		}
		select {
		case <-bound:
			return openResult{}, fmt.Errorf("the board server did not answer within %s; its log %s opens with its pid: way forward: end that process if it hangs, then run lym board open again", deps.StartBound, deps.Paths.Log)
		case <-time.After(deps.Poll):
		}
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
	}
	return openResult{Address: boardserve.PageAddress(server.Port), Launcher: launcher, Started: !live}, nil
}

// spawnServer starts `lym board serve` detached in primeAnchor, with its stderr appended to the log at logPath.
func spawnServer(primeAnchor, logPath string) error {
	exe, err := selfexec.LymBinary()
	if err != nil {
		return fmt.Errorf("resolve this binary: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create the server log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the server log: %w", err)
	}
	defer logFile.Close()
	cmd := exec.Command(exe, "board", "serve")
	cmd.Dir = primeAnchor
	cmd.Stderr = logFile
	proc.Detach(cmd)
	logger.Info("boardcli: spawning the detached board server", "exe", exe, "dir", primeAnchor, "log", logPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", exe, err)
	}
	proc.Reap(cmd)
	return nil
}
