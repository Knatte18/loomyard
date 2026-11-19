// serveopen.go holds the operator verbs `lym board serve` and `lym board open`:
// the verbs' wiring, the open sequence that starts the server and the browser, and the server's detached spawn.

package boardcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// What `lym board open` waits and probes with in production.
const (
	// openStartBound is how long open waits for a server to answer before it refuses.
	openStartBound = 15 * time.Second
	// openPollInterval is how often open re-reads the server's state while it waits.
	openPollInterval = time.Second
	// openProbeTimeout bounds one probe of a server.
	openProbeTimeout = 2 * time.Second
)

// refuseRunAgent refuses verb for a run agent's strand name, since the viewer and its server are the operator's tools.
// An orch's strand name and an empty one pass.
func refuseRunAgent(verb, strandName string) error {
	if !boardengine.IsRunAgentSession(strandName) {
		return nil
	}
	return fmt.Errorf("the run session %s cannot run lym board %s: the board viewer and its server are the operator's tools, "+
		"and a server an agent started would carry its session into the operator's page: "+
		"way forward: ask your orch or the operator to run lym board open", strandName, verb)
}

// refuseBoardVerb refuses serve and open for a run agent, then for a run that resolved no hub worktree.
// The PersistentPreRunE leaves location nil under --board-path, and both verbs need the hub's own board config.
func refuseBoardVerb(verb string, location *lymcwd.Location) error {
	if err := refuseRunAgent(verb, strandNameFromEnv()); err != nil {
		return err
	}
	if location == nil {
		return fmt.Errorf("lym board %s needs the hub's board config, which only a run from a worktree of the hub resolves: "+
			"way forward: run it from a worktree of the hub, without --board-path", verb)
	}
	return nil
}

// serveCommand builds `lym board serve`, the detached board server's body.
// config and location are read when the verb runs, after the PersistentPreRunE resolved them.
func serveCommand(config func() boardengine.Config, location func() *lymcwd.Location) *cobra.Command {
	return &cobra.Command{
		Use:         "serve",
		Short:       "run the hub's board server until it is idle or its binary is replaced",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `Serve the hub's board on a loopback port: the viewer page, the board's live changes and the page's writes.
"lym board open" starts it, so run it by hand only to see its behavior.
At most one server runs per hub: a second serve prints {"ok":true,"running":true} and exits 0.
The server exits, finishing the requests in flight, when its binary is replaced or no request comes for a long time.
Takes no argument, and is refused from a run's agent session.`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			resolved := location()
			if err := refuseBoardVerb("serve", resolved); err != nil {
				return outputError(out, err.Error())
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			err := boardserve.Run(ctx, serveOptions(config(), resolved.HubPath))
			if errors.Is(err, boardserve.ErrRunning) {
				return output.Ok(out, map[string]any{"running": true})
			}
			if err != nil {
				return outputError(out, err.Error())
			}
			return outputSuccess(out)
		}),
	}
}

// serveOptions returns what boardserve.Run runs with for the hub at hub, over boardConfig.
// Every page write is the operator's, so the server's board carries no strand name.
// The server registers under the hub's own owner with the board server role.
func serveOptions(boardConfig boardengine.Config, hub string) boardserve.Options {
	boardConfig.StrandName = ""
	return boardserve.Options{
		Paths:     boardserve.Paths(fabricengine.HubScratchDir(hub)),
		Board:     boardengine.New(boardConfig),
		BoardPath: boardConfig.Path,
		LockWait:  boardserve.DefaultLockWait,
		IdleBound: boardserve.DefaultIdleBound,
		PollBound: boardserve.DefaultPollBound,
		Log:       os.Stderr,
		Register: func() error {
			return runproc.Register(fabricengine.RunProcDir(hub), runproc.HubOwner, runproc.SelfRecord(runproc.RoleBoardServer))
		},
		Replaced: selfexec.New().Replaced,
		Watch:    boardserve.WatchDirectory,
		Now:      time.Now,
		After:    time.After,
	}
}

// openCommand builds `lym board open`.
func openCommand(location func() *lymcwd.Location) *cobra.Command {
	return &cobra.Command{
		Use:         "open",
		Short:       "open the board page in the default browser, starting the hub's board server when none runs",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Make sure the hub's board server runs, then open the board page in the default browser.
The server is started from the prime's anchored directory when none runs, and a second open reuses the one running.
The browser opens a launcher page in the server's state directory, which carries the page's access token;
the printed address does not.

Output:
  {"ok":true,"address":"http://127.0.0.1:<port>/","launcher":"<path>","started":true}

"started" is false when a server already ran. Takes no argument, and is refused from a run's agent session.`,
		Args: cobra.NoArgs,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			resolved := location()
			if err := refuseBoardVerb("open", resolved); err != nil {
				return outputError(out, err.Error())
			}
			deps := productionOpenDeps(resolved)
			result, err := runOpen(deps)
			if err != nil {
				return outputError(out, err.Error())
			}
			return output.Ok(out, map[string]any{"address": result.Address, "launcher": result.Launcher, "started": result.Started})
		}),
	}
}

// productionOpenDeps returns the deps open runs with from the hub worktree resolved.
// A named agent session, such as an orch's, waits on the opener only as long as its keep-warm cap allows.
func productionOpenDeps(resolved *lymcwd.Location) openDeps {
	deps := openDeps{
		Paths: boardserve.Paths(fabricengine.HubScratchDir(resolved.HubPath)),
		Prime: func() (string, error) {
			prime, err := fabricengine.PrimeName(resolved)
			if err != nil {
				return "", err
			}
			named, err := lymcwd.NamedWorktree(resolved.HubPath, prime)
			if err != nil {
				return "", fmt.Errorf("resolve the prime's anchored directory: %w", err)
			}
			return named.AnchorPath(), nil
		},
		Spawn:       spawnServer,
		Probe:       boardserve.Probe,
		Running:     proc.IsRunning,
		StartOpener: startOpener,
		StartBound:  openStartBound,
		Poll:        openPollInterval,
	}
	if strand := strandNameFromEnv(); strand != "" {
		deps.OpenerCap = shuttleengines.KeepWarm(fabricengine.ConfigDirs(resolved), strand).Cap
	}
	return deps
}

// openDeps holds what runOpen reads and starts, so a test can drive it without a server or a browser.
type openDeps struct {
	// Paths is where the board server keeps its files.
	Paths boardserve.StatePaths
	// Prime resolves the prime's anchored directory, whichever worktree open ran from.
	Prime func() (string, error)
	// Spawn starts the detached server in dir, appending its stderr to the log at logPath.
	Spawn func(dir, logPath string) error
	// Probe asks the server a state describes for its board within the timeout.
	Probe func(st boardserve.State, timeout time.Duration) error
	// Running reports whether the process pid runs.
	Running func(pid int) bool
	// StartOpener starts the browser opener on path and returns the channel its exit arrives on.
	StartOpener func(path string) (<-chan error, error)
	// StartBound is how long open waits for a server to answer.
	StartBound time.Duration
	// Poll is how often open re-reads the server's state while it waits.
	Poll time.Duration
	// OpenerCap bounds the wait on the opener; zero waits until the opener exits.
	OpenerCap time.Duration
}

// openResult is what a successful open reports.
type openResult struct {
	// Address is the server's page address, which carries no token.
	Address string
	// Launcher is the launcher page the browser was opened on.
	Launcher string
	// Started reports whether this open started the server.
	Started bool
}

// runOpen makes sure a board server answers, writes its launcher page and starts the browser opener on it.
// A held server lock reads as a live or starting server, which is waited on.
// A free lock gets a new server spawned from the prime's anchored directory.
func runOpen(deps openDeps) (openResult, error) {
	live, err := boardserve.Live(deps.Paths)
	if err != nil {
		return openResult{}, err
	}
	// A server that is draining after an exit still answers on its old state, so a new one is told by its token.
	previousToken := ""
	if !live {
		anchor, err := deps.Prime()
		if err != nil {
			return openResult{}, err
		}
		if previous, found, err := boardserve.ReadState(deps.Paths); err == nil && found {
			previousToken = previous.Token
		}
		if err := deps.Spawn(anchor, deps.Paths.Log); err != nil {
			return openResult{}, fmt.Errorf("start the board server: %w: way forward: check that lym runs from this shell "+
				"and read the server log %s, then run lym board open again", err, deps.Paths.Log)
		}
	}

	st, err := deps.awaitServer(previousToken, !live)
	if err != nil {
		return openResult{}, err
	}
	launcher, err := boardserve.WriteLauncher(deps.Paths, st)
	if err != nil {
		return openResult{}, err
	}
	exited, err := deps.StartOpener(launcher)
	if err != nil {
		return openResult{}, openerFailure(launcher, err)
	}
	result := openResult{Address: boardserve.PageAddress(st.Port), Launcher: launcher, Started: !live}
	if deps.OpenerCap == 0 {
		if err := <-exited; err != nil {
			return openResult{}, openerFailure(launcher, err)
		}
		return result, nil
	}
	select {
	case err := <-exited:
		if err != nil {
			return openResult{}, openerFailure(launcher, err)
		}
	case <-time.After(deps.OpenerCap):
		// The opener stays running, as a browser opener that blocks until the browser closes does.
	}
	return result, nil
}

// openerFailure is the refusal for an opener that did not start or exited with an error.
func openerFailure(launcher string, err error) error {
	return fmt.Errorf("the opener %s failed: %w: way forward: open %s in a browser by hand", openerName, err, launcher)
}

// awaitServer waits up to StartBound for a server state that answers a probe, and returns it.
// A state with previousToken is the earlier server's and is skipped.
// With needLock it also waits for the server lock to be held, which a newly spawned server takes first.
func (deps openDeps) awaitServer(previousToken string, needLock bool) (boardserve.State, error) {
	deadline := time.Now().Add(deps.StartBound)
	for {
		if st, answering := deps.answeringState(previousToken, needLock); answering {
			return st, nil
		}
		if !time.Now().Before(deadline) {
			return boardserve.State{}, fmt.Errorf("the board server did not answer within %s; its log %s opens with its pid: "+
				"way forward: end that process if it hangs, then run lym board open again", deps.StartBound, deps.Paths.Log)
		}
		time.Sleep(deps.Poll)
	}
}

// answeringState reads the server state and reports whether its process runs and it answers its token.
func (deps openDeps) answeringState(previousToken string, needLock bool) (boardserve.State, bool) {
	st, found, err := boardserve.ReadState(deps.Paths)
	if err != nil || !found || (previousToken != "" && st.Token == previousToken) {
		return boardserve.State{}, false
	}
	if needLock {
		if live, err := boardserve.Live(deps.Paths); err != nil || !live {
			return boardserve.State{}, false
		}
	}
	if !deps.Running(st.PID) || deps.Probe(st, openProbeTimeout) != nil {
		return boardserve.State{}, false
	}
	return st, true
}

// spawnServer starts `<this binary> board serve` detached, with dir as its working directory.
// The server's stderr is appended to the log at logPath, and the child is reaped in the background.
func spawnServer(dir, logPath string) error {
	exe, err := selfexec.LymBinary()
	if err != nil {
		return fmt.Errorf("resolve this binary: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create the server log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the server log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "board", "serve")
	cmd.Dir = dir
	cmd.Stderr = logFile
	proc.Detach(cmd)
	logger.Info("board: spawning the detached board server", "exe", exe, "dir", dir, "log", logPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", exe, err)
	}
	proc.Reap(cmd)
	return nil
}

// waitOnOpener starts the opener command detached and returns the channel its exit arrives on.
// One goroutine waits on the process, so the opener is reaped and a failed exit reaches the caller.
func waitOnOpener(cmd *exec.Cmd) (<-chan error, error) {
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
	}()
	return exited, nil
}
