// main.go holds the lym process entry point, the run function it and the tests share, and the cobra root builder.

// Package main is the cobra root for the lym CLI.
// It assembles each module's Command() into a single root and mounts the shortcuts of internal/shortcutcli on it.
// It installs --json help and delegates execution to cobra via clihelp.RunRoot.
package main

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/adoptcli"
	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlercli"
	"github.com/Knatte18/loomyard/internal/claudecli"
	"github.com/Knatte18/loomyard/internal/cleanupcli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/driftcli"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hardencli"
	"github.com/Knatte18/loomyard/internal/heddlecli"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/helpcli"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/idecli"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/machinecli"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/quarrycli"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reportcli"
	"github.com/Knatte18/loomyard/internal/retrocli"
	"github.com/Knatte18/loomyard/internal/selfreportcli"
	"github.com/Knatte18/loomyard/internal/shedcli"
	"github.com/Knatte18/loomyard/internal/shortcutcli"
	"github.com/Knatte18/loomyard/internal/shuttlecli"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/slackcli"
	"github.com/Knatte18/loomyard/internal/stencilcli"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/weavercli"
	"github.com/Knatte18/loomyard/internal/webstercli"
)

func main() {
	gitexec.SetKillReporter(logGitKill)
	// Callers of the production binary expect stdout and stderr split.
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	// The process run directory exists only if this process created one;
	// a removal error is left to the next start's sweep.
	_ = lymtemp.Remove()
	os.Exit(code)
}

// logGitKill logs the outcome of killing a timed-out remote git command.
// A non-nil groupErr means git was killed alone,
// so a descendant such as git-remote-https may outlive it.
func logGitKill(args []string, pid int, groupErr error) {
	if groupErr != nil {
		logger.Warn("gitexec: killed only git after its deadline; a descendant may outlive it", "args", args, "pid", pid, "cause", groupErr)
		return
	}
	logger.Info("gitexec: killed git's process group after its deadline", "args", args, "pid", pid)
}

// run executes args against a fresh root, writing to stdout and stderr, and returns the exit code.
// A JSON error envelope goes to stdout.
// A test passes one buffer as both writers to capture all output.
func run(args []string, stdout, stderr io.Writer) int {
	root := newRoot()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	code := clihelp.RunRoot(root, stdout)
	notifyExitAndSweep(code)
	return code
}

// pauseSlugOverride returns a pointer to the pause slug the executing command was told by its hidden flag, empty included.
// It returns nil when the command has no such flag or the flag was not set,
// so the gate takes its slug from the working directory.
func pauseSlugOverride(cmd *cobra.Command) *string {
	flag := cmd.Flags().Lookup(fabriclayout.PauseSlugFlag)
	if flag == nil || !flag.Changed {
		return nil
	}
	slug := flag.Value.String()
	return &slug
}

// detachedChild reads the flags a detached push, board-sync or board-pusher child was spawned with.
// An undeclared or unset flag reads false.
// The board pusher defers under a pause and is held under the hub scope, as a deferrable board sync is.
func detachedChild(cmd *cobra.Command) tendercli.DetachedChild {
	changed := func(name string) bool {
		flag := cmd.Flags().Lookup(name)
		return flag != nil && flag.Changed
	}
	hubScope := changed(boardengine.DeferrableSyncFlag) || changed(fabriclayout.PusherBoardFlag)
	return tendercli.DetachedChild{
		Deferrable:   changed(fabriclayout.PauseSlugFlag) || hubScope,
		ParentLocked: changed(fabriclayout.ParentLockedFlag),
		HubScope:     hubScope,
	}
}

// installPauseGate installs the pause gate that holds this process's network git and GitHub calls.
// The gate holds them only while a pause covers the process.
// It holds only a run's own process; an operator's verb passes.
func installPauseGate(cmd *cobra.Command) {
	tendercli.InstallNetworkGate(cmd.Context(), pauseSlugOverride(cmd), detachedChild(cmd), runCommand(cmd))
}

// runCommand reports whether the executing command is marked as a run's own process, which a pause holds.
func runCommand(cmd *cobra.Command) bool {
	return cmd.Annotations[clihelp.RunProcessAnnotation] == clihelp.AnnotationEnabled
}

// newRoot builds the lym cobra root with all module subcommands, --json flag, and JSON help.
func newRoot() *cobra.Command {
	var verbosity int

	root := &cobra.Command{
		Use:   "lym",
		Short: "drive the loomyard task board, worktrees, agents and runs",
		Long: `lym is the CLI for the Loomyard task tracker.

It assembles every module's cobra command tree under a single root so that
all modules are discoverable via "lym --help" and every subcommand carries
its own --help and --json help output; --json prints the help as JSON and
never runs the command.

On a terminal, an operator verb prints a short readable summary of its JSON
envelope: what it did, each error and warning, and each way forward, with long
lists collapsed to a count. --envelope prints the full envelope instead; any
output that is no terminal, as an agent's or a script's, gets the envelope.

Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler, webster, weaver, stencil, loom, quarry, batten, shed, orch, cleanup, adopt, beam, report, retro, tender, hibernate, drift, gate, creel, slack, heddle, machine, claude, harden, help.
Shortcuts, declared in internal/shortcutcli's embedded file, are listed under their own heading in lym --help.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Modules' PersistentPreRunE hooks run after root's via EnableTraverseRunHooks.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// With no -v the level LYM_LOG_LEVEL set at init stands,
			// so a spawned daemon keeps its spawner's Debug trace.
			if verbosity > 0 {
				logger.SetVerbosity(verbosity)
			}
			if !testing.Testing() {
				logger.MintOrAdoptAndExport()
				logger.Arm()
				installPauseGate(cmd)
				// Fabric takes the run records' paths, the run locks and the status codec only through these seams.
				fabriccli.InstallRequiredSeams()
				// A push that the origin refuses reaches the prime orch as creel mail from FABRIC, whichever process pushed.
				orchcli.InstallPushNotifier(cmd.Context())
				// A reconcile that rewrites or flags a reinterpreted hub config value reaches the prime orch as creel mail from RECONCILE.
				orchcli.InstallReconcileNotifier(cmd.Context())
				// A pair's teardown drops the unread creel mail about its run, so no orch reads it as live.
				orchcli.InstallRunMailDropper()
				// A records replay merges the board by entry through boardengine's Bolt replayer, in the detached push child and in a pull-first Bolt write.
				fabricengine.RegisterBoltReplayer(boardengine.FabricReplay{})
				// Every board render shows each beam's and the root's autonomy and pull-request setting as the runs read them.
				boardengine.InstallBeamModeReader(hubgeom.BeamModes)
				// A worktree that fabric makes or moves reaches heddle through the worktree seam, to render its files.
				fabricengine.InstallWorktreeSeam(heddleengine.Seam{})
				// The hub workspace file leads a pair's folder name with its project's handle, read from the board.
				fabricengine.InstallPairFolderNamer(boardengine.WorkspaceFolderNames)
				// Each workspace file carries one beam start task, which the alt+shift+0 key runs.
				fabricengine.InstallWorkspaceContent(ideengine.WorkspaceContent)
				// A records replay merges heddle.json by entry through heddle's Bolt replayer.
				// That replayer also claims HEDDLE.md and renders it afresh after each replay.
				fabricengine.RegisterBoltReplayer(heddleengine.Replayer{})
				// Every Claude session launched in a hub appends the passages heddle stores for its engine and role, an orch session's orch-prompt ones included.
				shuttleengine.InstallPassageReader(readHeddleStanding)
			}
			// Seeding must never block a command from running, regardless of its outcome.
			seedStencils(cmd)
			clihelp.UseReadableOutput(cmd, clihelp.WriterIsTerminal)
			return nil
		},
	}

	clihelp.InstallJSONHelp(root)
	clihelp.InstallReadableOutput(root)

	root.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "increase log verbosity (-v info, -vv debug)")

	cobra.EnableTraverseRunHooks = true

	root.AddCommand(
		boardcli.Command(),
		configcli.Command(),
		idecli.Command(),
		reedcli.Command(),
		fabriccli.Command(),
		selfreportcli.Command(),
		shuttlecli.Command(),
		burlercli.Command(),
		stencilcli.Command(),
		webstercli.Command(),
		weavercli.Command(),
		loomcli.Command(),
		battencli.Command(),
		shedcli.Command(),
		orchcli.Command(),
		cleanupcli.Command(),
		adoptcli.Command(),
		beamcli.Command(),
		reportcli.Command(),
		retrocli.Command(),
		tendercli.Command(),
		hibernatecli.Command(),
		driftcli.Command(),
		quarrycli.Command(),
		gatecli.Command(),
		creelcli.Command(),
		slackcli.Command(),
		heddlecli.Command(),
		machinecli.Command(),
		claudecli.Command(),
		hardencli.Command(),
	)

	// SetHelpCommand replaces cobra's built-in help command;
	// InitDefaultHelpCmd adds it now, because cobra would add it only at execution;
	// the tree walks read root.Commands() before that.
	root.SetHelpCommand(helpcli.Command())
	root.InitDefaultHelpCmd()

	shortcutcli.Mount(root, newRoot)

	return root
}
