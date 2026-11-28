// viewer.go holds the work behind the board page's two verbs: serving the page, and opening it in a browser.

package boardcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// The bounds of a board server's life and of the wait for one to answer.
const (
	serverLockWait  = 5 * time.Second
	serverIdleBound = 30 * time.Minute
	// serverPollBound stays under a minute, so an open page asks well within the idle bound.
	serverPollBound = 50 * time.Second
	openStartBound  = 15 * time.Second
	openPoll        = time.Second
)

// refuseRunAgent refuses verb for the strand name of a run's agent.
func refuseRunAgent(verb, strand string) error {
	if !boardengine.IsRunAgentSession(strand) {
		return nil
	}
	return fmt.Errorf("the run session %s cannot run lym board %s: the board viewer and its server are the operator's tools, "+
		"and a server an agent started would carry its session into the operator's page: "+
		"way forward: ask your orch or the operator to run lym board open", strand, verb)
}

// checkViewerVerb refuses verb for a run's agent, and without the worktree location it needs.
// location is nil under --board-path.
func checkViewerVerb(verb string, location *lymcwd.Location) error {
	if err := refuseRunAgent(verb, strandNameFromEnv()); err != nil {
		return err
	}
	if location == nil {
		return fmt.Errorf("lym board %s needs the hub's board config, which only a run from a worktree of the hub resolves: "+
			"way forward: run it from a worktree of the hub, without --board-path", verb)
	}
	return nil
}

// serveBoardPage serves the board of config as the hub's one board server, until that server's exit.
func serveBoardPage(out io.Writer, location *lymcwd.Location, config boardengine.Config) int {
	// The page's writes are the operator's, whoever started the server.
	config.StrandName = ""
	hub := location.HubPath
	err := boardserve.Run(context.Background(), boardserve.Options{
		Paths:     boardserve.Paths(fabricengine.HubScratchDir(hub)),
		Board:     boardengine.New(config),
		BoardPath: config.Path,
		LockWait:  serverLockWait,
		IdleBound: serverIdleBound,
		PollBound: serverPollBound,
		Log:       os.Stderr,
		Register: func() error {
			return runproc.Register(fabricengine.RunProcDir(hub), runproc.HubOwner, runproc.SelfRecord(runproc.RoleBoardServer))
		},
		Replaced: selfexec.New().Replaced,
		Watch:    fswatch.Watch,
		Now:      time.Now,
		After:    time.After,
	})
	if errors.Is(err, boardserve.ErrRunning) {
		return output.Ok(out, map[string]any{"running": true})
	}
	if err != nil {
		return outputError(out, err.Error())
	}
	return outputSuccess(out)
}

// openBoardPage opens the page of the hub of location in the default browser, starting its server when none runs.
func openBoardPage(out io.Writer, location *lymcwd.Location) int {
	deps := openDeps{
		Paths: boardserve.Paths(fabricengine.HubScratchDir(location.HubPath)),
		Prime: func() (string, error) {
			prime, err := fabricengine.PrimeName(location)
			if err != nil {
				return "", err
			}
			at, err := lymcwd.NamedWorktree(location.HubPath, prime)
			if err != nil {
				return "", err
			}
			return at.AnchorPath(), nil
		},
		Spawn:       spawnServer,
		Probe:       boardserve.Probe,
		Running:     proc.IsRunning,
		StartOpener: startOpener,
		StartBound:  openStartBound,
		Poll:        openPoll,
	}
	// An agent's blocking wait is capped; the operator's lasts as long as the opener does.
	if strand := strandNameFromEnv(); strand != "" {
		caps := shuttleengines.KeepWarm(fabricengine.ConfigDirs(location), strand)
		if caps.Fallback != nil {
			logger.Debug("boardcli: the keep-warm cap is the fallback", "cause", caps.Fallback)
		}
		deps.OpenerCap = caps.Cap
	}
	result, err := runOpen(deps)
	if err != nil {
		return outputError(out, err.Error())
	}
	return output.Ok(out, map[string]any{"address": result.Address, "launcher": result.Launcher, "started": result.Started})
}

// openDeps is what opening the page runs on.
type openDeps struct {
	Paths boardserve.StatePaths
	// Prime returns the anchored directory of the hub's prime worktree, where a server is started.
	Prime func() (string, error)
	// Spawn starts a detached server in dir, with its errors appended to logPath.
	Spawn func(dir, logPath string) error
	Probe func(boardserve.State) error
	// Running reports whether the process pid runs.
	Running func(pid int) bool
	// StartOpener opens the file at path, and returns the channel its exit is sent on.
	StartOpener func(path string) (<-chan error, error)
	// StartBound bounds the wait for a server to answer, which is tried every Poll.
	StartBound time.Duration
	Poll       time.Duration
	// OpenerCap bounds the wait on the opener; zero waits until it exits.
	OpenerCap time.Duration
}

// openResult is what opening the page did.
type openResult struct {
	// Address is the page's address, without the token.
	Address  string
	Launcher string
	// Started reports whether no server ran, so one was started.
	Started bool
}

// runOpen makes sure the hub's board server answers, starting one when none runs, and opens the page on it.
func runOpen(deps openDeps) (openResult, error) {
	live, err := boardserve.Live(deps.Paths)
	if err != nil {
		return openResult{}, err
	}
	if !live {
		prime, err := deps.Prime()
		if err != nil {
			return openResult{}, err
		}
		if err := deps.Spawn(prime, deps.Paths.Log); err != nil {
			return openResult{}, fmt.Errorf("start the board server: %w: way forward: check that lym runs from this shell and read the server log %s, then run lym board open again", err, deps.Paths.Log)
		}
	}
	server, err := awaitServer(deps)
	if err != nil {
		return openResult{}, err
	}
	launcher, err := boardserve.WriteLauncher(deps.Paths, server.Port, server.Token)
	if err != nil {
		return openResult{}, err
	}
	if err := runOpener(deps, launcher); err != nil {
		return openResult{}, fmt.Errorf("the opener %s failed: %w: way forward: open %s in a browser by hand", openerName, err, launcher)
	}
	return openResult{Address: boardserve.PageAddress(server.Port), Launcher: launcher, Started: !live}, nil
}

// awaitServer returns the state of the server that holds the lock and answers, waiting up to the start bound.
// A state left by an earlier server never answers, so it is read again on every try.
func awaitServer(deps openDeps) (boardserve.State, error) {
	deadline := time.Now().Add(deps.StartBound)
	for {
		live, err := boardserve.Live(deps.Paths)
		if err != nil {
			return boardserve.State{}, err
		}
		if live {
			// A state that does not read yet is one more try.
			current, found, err := boardserve.ReadState(deps.Paths)
			if err == nil && found && deps.Running(current.PID) && deps.Probe(current) == nil {
				return current, nil
			}
		}
		if !time.Now().Before(deadline) {
			return boardserve.State{}, fmt.Errorf("the board server did not answer within %s; its log %s opens with its pid: way forward: end that process if it hangs, then run lym board open again", deps.StartBound, deps.Paths.Log)
		}
		time.Sleep(deps.Poll)
	}
}

// runOpener opens the file at path and waits for the opener, up to the cap.
// An opener still running at the cap is left running and counts as success.
func runOpener(deps openDeps, path string) error {
	exited, err := deps.StartOpener(path)
	if err != nil {
		return err
	}
	var capped <-chan time.Time
	if deps.OpenerCap > 0 {
		capped = time.After(deps.OpenerCap)
	}
	select {
	case err := <-exited:
		return err
	case <-capped:
		return nil
	}
}
