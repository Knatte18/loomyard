// open.go runs the two verbs of the board viewer.
// It holds how `open` starts or reaches a server and opens its page.
// It also holds the production values the server serves with.

package boardcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// The waits of the server `serve` runs.
const (
	// serverLockWait outlasts the instant liveness read of an `open`, so that read never turns a starting server away.
	serverLockWait  = 5 * time.Second
	serverIdleBound = 30 * time.Minute
	// serverPollBound stays under a minute, so an open page asks at least that often.
	serverPollBound = 50 * time.Second
)

// The waits of `open` for a server to answer.
const (
	openStartBound = 15 * time.Second
	openPoll       = time.Second
)

// refuseRunAgent refuses verb for the strand name of a run agent's session.
func refuseRunAgent(verb, strand string) error {
	if !boardengine.IsRunAgentSession(strand) {
		return nil
	}
	return fmt.Errorf("the run session %s cannot run lym board %s: the board viewer and its server are the operator's tools, "+
		"and a server an agent started would carry its session into the operator's page: way forward: ask your orch or the operator to run lym board open", strand, verb)
}

// refuseWithoutHubConfig is verb's refusal under --board-path, which resolves no worktree and so no board config.
func refuseWithoutHubConfig(verb string) error {
	return fmt.Errorf("lym board %s needs the hub's board config, which only a run from a worktree of the hub resolves: way forward: run it from a worktree of the hub, without --board-path", verb)
}

// openDeps are what runOpen reaches a server and an opener through.
type openDeps struct {
	Paths boardserve.StatePaths
	// Prime resolves the prime's anchored directory, the one a server is started in.
	Prime func() (string, error)
	// Spawn starts a server in primeAnchor that appends its stdout and stderr to logPath.
	Spawn   func(primeAnchor, logPath string) error
	Probe   func(boardserve.State) error
	Running func(pid int) bool
	// StartOpener opens path and delivers the opener's exit on the returned channel.
	StartOpener func(path string) (<-chan error, error)
	// StartBound is how long a server gets to answer, asked every Poll.
	StartBound time.Duration
	Poll       time.Duration
	// OpenerCap is the longest wait on the opener, zero for no cap.
	OpenerCap time.Duration
}

// openResult is what a successful open reports.
type openResult struct {
	// Address is the page's address, without the token.
	Address  string
	Launcher string
	// Started reports whether this open started the server.
	Started bool
}

// runOpen makes sure the hub's board server answers and opens its page through a launcher file.
// It starts a server when none holds the lock.
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

	server, err := awaitServer(deps)
	if err != nil {
		return openResult{}, err
	}
	launcher, err := boardserve.WriteLauncher(deps.Paths, server.Port, server.Token)
	if err != nil {
		return openResult{}, err
	}
	if err := openLauncher(deps, launcher); err != nil {
		return openResult{}, fmt.Errorf("the opener %s failed: %w: way forward: open %s in a browser by hand", openerName, err, launcher)
	}
	return openResult{Address: boardserve.PageAddress(server.Port), Launcher: launcher, Started: !live}, nil
}

// awaitServer waits, up to the start bound, for the state of the server that holds the lock.
// That state must name a running process which answers its token.
func awaitServer(deps openDeps) (boardserve.State, error) {
	deadline := time.Now().Add(deps.StartBound)
	for {
		live, err := boardserve.Live(deps.Paths)
		if err != nil {
			return boardserve.State{}, err
		}
		if live {
			server, found, err := boardserve.ReadState(deps.Paths)
			if err != nil {
				return boardserve.State{}, err
			}
			if found && deps.Running(server.PID) && deps.Probe(server) == nil {
				return server, nil
			}
		}
		if !time.Now().Before(deadline) {
			return boardserve.State{}, fmt.Errorf("the board server did not answer within %s; its log %s opens with its pid: way forward: end that process if it hangs, then run lym board open again", deps.StartBound, deps.Paths.Log)
		}
		time.Sleep(deps.Poll)
	}
}

// openerCapFor is the longest an open run by strand waits on its opener, over the config layers of dirs.
// It is zero, for no cap, with no strand name: only an agent's session has a wait to keep short.
func openerCapFor(dirs configengine.Dirs, strand string) time.Duration {
	if strand == "" {
		return 0
	}
	answer := shuttleengines.KeepWarm(dirs, strand)
	if answer.Fallback != nil {
		logger.Debug("boardcli: the keep-warm cap is the fallback", "strand", strand, "cause", answer.Fallback)
	}
	return answer.Cap
}

// openLauncher starts the opener on launcher and waits for its exit, no longer than a non-zero cap.
// An opener still running at the cap is left to finish.
func openLauncher(deps openDeps, launcher string) error {
	exited, err := deps.StartOpener(launcher)
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

// spawnServer starts `lym board serve` detached in primeAnchor,
// with its stdout and stderr appended to the file at logPath.
func spawnServer(primeAnchor, logPath string) error {
	exe, err := selfexec.LymBinary()
	if err != nil {
		return err
	}
	return spawnServerBinary(exe, primeAnchor, logPath)
}

// spawnServerBinary starts the board server of the binary exe detached in primeAnchor,
// with its stdout and stderr appended to the file at logPath.
// A server reports a failed start on stdout, so the log needs both streams to hold the cause.
func spawnServerBinary(exe, primeAnchor, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()

	cmd := exec.Command(exe, "board", "serve")
	cmd.Dir = primeAnchor
	cmd.Stdout = log
	cmd.Stderr = log
	proc.Detach(cmd)
	logger.Info("boardcli: spawning the board server", "exe", exe, "dir", primeAnchor, "log", logPath)
	if err := cmd.Start(); err != nil {
		logger.Warn("boardcli: board server spawn failed", "exe", exe, "dir", primeAnchor, "cause", err)
		return err
	}
	proc.Reap(cmd)
	return nil
}

// watchDirectory opens the file events of the entries names of dir, with the function that closes the watch.
func watchDirectory(dir string, names ...string) (<-chan fswatch.Event, func() error, error) {
	watcher, err := fswatch.Watch(dir, names...)
	if err != nil {
		return nil, nil, err
	}
	return watcher.Events(), watcher.Close, nil
}
