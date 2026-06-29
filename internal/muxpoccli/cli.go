// cli.go provides the cobra Command() entry point for the muxpoc module and
// the RunCLI seam that wires it into the legacy io.Writer-based call contract.
//
// Command() builds a parent "muxpoc" cobra command with persistent tuning flags
// and a PersistentPreRunE that resolves the worktree root via paths.Resolve.
// Each subcommand's RunE closes over the resolved cfg variable that PreRunE populates.

// Package muxpoc is a shipped proof-of-concept psmux orchestrator that proves
// the risky parts — daemon and pane recovery — of the planned mux module.
// It is distinct from and not a replacement for internal/mux, which is unbuilt.
//
// Subcommands:
//   - up       Cold-start or cold-recover the muxpoc session
//   - review   Add a reviewer pane to the active session
//   - attach   Pop the session into a maximized terminal
//   - status   Show session and pane status
//   - down     Stop the session and delete state
//   - daemon   Foreground poller that recovers a crashed session (crash-loop-guarded)
package muxpoccli

import (
	"fmt"
	"io"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/spf13/cobra"
)

// Config holds paths and dimensions for muxpoc operations.
type Config struct {
	PsmuxPath    string
	PwshPath     string
	ClaudePath   string
	LaunchTpl    string
	ResumeTpl    string
	Width        int
	Height       int
	Interval     time.Duration
	WorktreeRoot string
}

// Command builds the cobra command tree for the muxpoc module.
//
// The parent command carries persistent tuning flags (--psmux, --pwsh, --claude,
// --launch, --resume, --width, --height, --interval) so every subcommand inherits
// them. A PersistentPreRunE resolves the worktree root via paths.Resolve and
// populates the closure-local cfg variable; on failure it writes an error response
// and signals abort so that subcommand RunE bodies do not execute against an
// uninitialised environment. Running "lyx muxpoc" with no arguments lists
// subcommands (exit 0) without invoking the PreRunE.
func Command() *cobra.Command {
	// cfg is populated by PersistentPreRunE and closed over by every RunE.
	// It is not valid until after PersistentPreRunE has run.
	var cfg Config

	cmd := &cobra.Command{
		Use:   "muxpoc",
		Short: "proof-of-concept psmux mux",
		Long: `muxpoc is a shipped proof-of-concept psmux orchestrator that proves
the risky parts — daemon and pane recovery — of the planned mux module.`,
		// RunE is set so that bare "lyx muxpoc" lists subcommands and "lyx muxpoc bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
	}

	// Register persistent tuning flags with the same defaults and usage strings
	// as the legacy stdlib flag.FlagSet. These flags are inherited by all subcommands.
	cmd.PersistentFlags().String("psmux", `C:\Code\tools\bin\psmux.exe`, "path to psmux executable")
	cmd.PersistentFlags().String("pwsh", `C:\Code\tools\powershell7\pwsh.exe`, "path to powershell executable")
	cmd.PersistentFlags().String("claude", "", "path to claude executable (empty: find on PATH)")
	cmd.PersistentFlags().String("launch", "%CLAUDE% --session-id %SID% %TASK%", "template for new claude launch")
	cmd.PersistentFlags().String("resume", "%CLAUDE% --resume %SID%", "template for claude resume")
	cmd.PersistentFlags().Int("width", 220, "psmux window width")
	cmd.PersistentFlags().Int("height", 50, "psmux window height")
	cmd.PersistentFlags().Duration("interval", 2*time.Second, "poll interval for session checks")

	// PersistentPreRunE resolves the worktree root and builds cfg before any subcommand
	// runs. On failure, it writes an error JSON response and signals abort so that the
	// leaf RunE is a no-op.
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		// Guard: when the muxpoc group command itself is invoked (bare listing or
		// unknown-subcommand error path via GroupRunE), skip cwd/layout resolution
		// so that neither path requires a git repository to be present.
		if c.Name() == "muxpoc" {
			return nil
		}

		cwd, err := paths.Getwd()
		if err != nil {
			output.Err(c.OutOrStdout(), fmt.Sprintf("failed to get current working directory: %v", err))
			clihelp.Abort(c.Context(), 1)
			return nil
		}

		layout, err := paths.Resolve(cwd)
		if err != nil {
			output.Err(c.OutOrStdout(), fmt.Sprintf("not a git repository: %v", err))
			clihelp.Abort(c.Context(), 1)
			return nil
		}

		// Read all persistent flag values into cfg so subcommand RunE bodies can use them.
		psmuxPath, _ := c.Flags().GetString("psmux")
		pwshPath, _ := c.Flags().GetString("pwsh")
		claudePath, _ := c.Flags().GetString("claude")
		launchTpl, _ := c.Flags().GetString("launch")
		resumeTpl, _ := c.Flags().GetString("resume")
		width, _ := c.Flags().GetInt("width")
		height, _ := c.Flags().GetInt("height")
		interval, _ := c.Flags().GetDuration("interval")

		cfg = Config{
			PsmuxPath:    psmuxPath,
			PwshPath:     pwshPath,
			ClaudePath:   claudePath,
			LaunchTpl:    launchTpl,
			ResumeTpl:    resumeTpl,
			Width:        width,
			Height:       height,
			Interval:     interval,
			WorktreeRoot: layout.WorktreeRoot,
		}
		return nil
	}

	// up: cold-start or cold-recover the muxpoc session.
	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "cold-start or cold-recover the muxpoc session",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return cmdUp(out, cfg) }),
	})

	// review: add a reviewer pane to the active session.
	cmd.AddCommand(&cobra.Command{
		Use:   "review",
		Short: "add a reviewer pane to the active session",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return cmdReview(out, cfg) }),
	})

	// attach: pop the session into a maximized terminal.
	cmd.AddCommand(&cobra.Command{
		Use:   "attach",
		Short: "pop the session into a maximized terminal",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return cmdAttach(out, cfg) }),
	})

	// status: show session and pane status.
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "show session and pane status",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return cmdStatus(out, cfg) }),
	})

	// down: stop the session and delete state.
	cmd.AddCommand(&cobra.Command{
		Use:   "down",
		Short: "stop the session and delete state",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return cmdDown(out, cfg) }),
	})

	// daemon: foreground poller; recovers a crashed session (crash-loop-guarded).
	cmd.AddCommand(&cobra.Command{
		Use:   "daemon",
		Short: "foreground poller; recovers a crashed session (crash-loop-guarded)",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return cmdDaemon(out, cfg) }),
	})

	return cmd
}

// RunCLI is the public seam for the muxpoc module.
//
// It delegates to clihelp.Execute(Command(), out, args) so in-process tests can
// capture all output via a single io.Writer. Returns the exit code (0 on success,
// 1 on cobra-level error such as unknown command or bad flag).
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
