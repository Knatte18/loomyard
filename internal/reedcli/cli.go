// cli.go builds the cobra command tree for the reed module and the RunCLI seam that runs it.
// The group's pre-run resolves the Engine once per invocation, and the verbs that need it read it from reedCLI.
// The watchdog, switch, follow, button and window verbs skip that resolution and resolve nothing from the cwd.

package reedcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/spf13/cobra"
)

// reedCLI carries the resolved *reedengine.Engine for the reed verbs.
// The zero value is invalid: eng is set only for the verbs that resolve a location.
type reedCLI struct {
	eng *reedengine.Engine

	// hubPath is the hub path of the resolved *lymcwd.Location.
	// It holds only the path: a stored Location would invite code to re-read geometry the engine was already told.
	hubPath string

	// anchorPath is the invoking worktree's anchor, Location.AnchorPath().
	// The detached remove child runs in it.
	// The strand verbs resolve their session, socket and reed.json from the cwd, which must equal the anchor.
	// Neither the hub nor, under a subpath anchor, the worktree root is that cwd.
	anchorPath string

	// suppressWatchdogSpawn, when true, makes the watchdog spawn a no-op.
	// Command sets it from testing.Testing().
	// Spawning the test binary would run the whole suite recursively.
	// An in-package test clears it to drive the real spawn path.
	suppressWatchdogSpawn bool

	// strands and spawnRemove are test seams for the strand verbs.
	// nil means the real engine and the real detached spawn.
	strands     strandOps
	spawnRemove func(guid string, recursive bool) error

	// endProcesses is the test seam for the pre-step `down` runs in a pair; nil means endPairProcesses.
	endProcesses func(ctx context.Context) ([]runproc.Ended, error)

	// openTerminal is the test seam for the window verb; nil means termwindow.Resolve then Launcher.OpenAttach.
	openTerminal func(dir, title, lymPath string) error

	// lymBinary is the test seam for the lookup of this lym binary that the window verb hands the launcher.
	// nil means selfexec.LymBinary, which refuses a test binary.
	lymBinary func() (string, error)

	// placeWindow is the test seam for the placement the window verb runs after it opens a window.
	// nil means placeTerminalWindow.
	placeWindow func(dir string) error

	// windowTitle is the test seam for the title the window verb forms when no --title is told.
	// nil means terminalWindowTitle.
	windowTitle func(dir string) string

	// watchdogTiming, when non-nil, replaces watchdogDefaultTiming() in watchdogCmd's loop.
	// It lets a test shorten the wait on the daemon; production never sets it.
	watchdogTiming *watchdogTiming

	// watchdogExeStat and respawnWatchdog are test seams for the replaced-binary retirement in watchdogCmd.
	// nil means a stat of this process's own binary and reedengine.SpawnWatchdog.
	watchdogExeStat func() (os.FileInfo, error)
	respawnWatchdog func(hubPath, tmuxPath, shellPath string, suppress bool)

	// watchdogListSessions is the test seam for the daemon's session listing, in its loop and in its exit re-check.
	// nil means reedengine.ListSessions.
	watchdogListSessions func(tmuxPath, socketKey string) ([]string, error)

	// watchdogOpenWatch is the test seam for the daemon's discover signal watch; nil means reedengine.OpenFileWatch.
	watchdogOpenWatch reedengine.FileWatchOpener

	// The downLock fields are test seams for the wait `down` makes for the watchdog lock.
	// Zero or nil means watchdogDownWait, watchdogDownPoll, probeWatchdogLock and time.Sleep.
	downLockWait  time.Duration
	downLockPoll  time.Duration
	downLockProbe func(lockPath string) (bool, error)
	downLockSleep func(time.Duration)
}

// strandOps is the subset of *reedengine.Engine that the strand verbs call, so tests can substitute a fake.
type strandOps interface {
	Down() (reedengine.DownResult, error)
	ResolveStrandGUID(name string) (string, error)
	MarkRetiring(guid string, retiring bool) error
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	Status() (reedengine.StatusResult, error)
}

// strandEngine returns the test seam when set, else the resolved engine.
func (c *reedCLI) strandEngine() strandOps {
	if c.strands != nil {
		return c.strands
	}
	return c.eng
}

// Command returns the cobra command tree for the reed module.
//
// The group's pre-run resolves the cwd, location, config and geometry into a *reedengine.Engine stored on c.
// The group command and the verbs the pre-run guard lists skip that resolution,
// so a bare listing or an unknown subcommand needs no git repository.
func Command() *cobra.Command {
	c := &reedCLI{suppressWatchdogSpawn: testing.Testing()}

	parent := &cobra.Command{
		Use:   "reed",
		Short: "manage the tmux strand overlay for this worktree",
		Long: `reed drives a per-hub tmux server that lays out one strand per pane for
this worktree's session: adding, removing, resuming, and attaching to
strands, plus rendering their layout on every mutation.

The tmux session is named after this worktree's directory, so that name must
carry none of ".", ":", "\" or any control character or invalid UTF-8 — tmux
silently rewrites all of these and would then create a session reed can never
address or tear down. Every reed verb refuses up front, naming the directory,
rather than booting substrate it cannot reach.`,
		// RunE is set so that bare "lym reed" lists subcommands and "lym reed bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// The group command and the watchdog, switch, follow, button and window verbs skip the resolution.
			// The watchdog daemon is told its hub path on its own flags, and it must never use c.eng.
			// Running the normal pre-run would make it refuse to start outside a worktree and hold a geometry it must not use.
			// switch, follow and button are told their inputs on flags and resolve no location from the cwd.
			// They load no config, so a reed.yaml that fails to load cannot abort a click.
			// window is told a worktree directory that may lie outside the hub's worktrees, so a location lookup would refuse it.
			if cmd.Name() == "reed" || cmd.Name() == "watchdog" || cmd.Name() == "switch" || cmd.Name() == "follow" || cmd.Name() == "button" || cmd.Name() == "window" {
				return nil
			}

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				// lymcwd.Resolve's error is already self-describing: it is the "not a git repository" sentinel.
				// Passing it through unchanged avoids repeating that text.
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// attach and up boot a session, so they refuse fabric's own checkouts, the Bolt checkout and a records sibling.
			// A session booted there sits on the hub's server under a name no reed listing of the hub's worktrees shows.
			if cmd.Name() == "attach" || cmd.Name() == "up" {
				if err := fabricengine.RequireDrivableWorktree(location); err != nil {
					output.Err(out, fmt.Sprintf("reed: lym reed %s boots a session only in a worktree of the hub: %v; way forward: run it from the hub's prime or a task worktree under %s", cmd.Name(), err, location.HubPath))
					clihelp.Abort(ctx, 1)
					return nil
				}
			}

			// The _lym/config/ root is anchored at location.AnchorPath(), not at the worktree root or a fabric sibling.
			// Reed config lives with the worktree the operator is standing in.
			cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reedGeom, err := hubgeom.ReedGeometry(location)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			c.eng = reedengine.New(cfg, reedGeom, shuttleengines.EnvScrub())
			c.hubPath = location.HubPath
			c.anchorPath = location.AnchorPath()
			return nil
		},
	}

	parent.AddCommand(c.upCmd(), c.downCmd(), c.addCmd(), c.removeCmd(), c.statusCmd(), c.resumeCmd(), c.attachCmd(), c.windowCmd(), c.watchdogCmd(), c.listCmd(), c.switchCmd(), c.followCmd(), c.buttonCmd())

	return parent
}

// RunCLI is the public seam for the reed module CLI.
//
// It runs the command tree on args in the process working directory.
// All output, including cobra's error text, goes to out.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the reed command tree on args with cwd as its working directory, writing all output to out.
// An empty cwd means the process working directory and runs through clihelp.Execute;
// any other value is seeded into the execution context through clihelp.ExecuteIn.
// The branch exists because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
