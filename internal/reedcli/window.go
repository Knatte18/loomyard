// window.go implements the `window` reed verb.
// It opens one desktop terminal window attached to a told worktree's reed session through internal/termwindow.
// It then runs the machine's window command through internal/desktophook.
// It runs that command with the config of the worktree holding the told directory.
// It needs no acting location or engine, so the root pre-run skips it.
// With no told title it titles the window from the told directory's worktree and the hub's board.

package reedcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/termwindow"
	"github.com/spf13/cobra"
)

// openTerminalWindow opens a terminal window titled title in dir, through the launcher this machine resolves.
// Its shell runs `lymPath reed attach`.
func openTerminalWindow(dir, title, lymPath string) error {
	launcher, err := termwindow.Resolve()
	if err != nil {
		return err
	}
	return launcher.OpenAttach(dir, title, lymPath)
}

// placeTerminalWindow runs the machine's window command for a just-opened window on dir.
// The command runs under the config stack of the worktree holding dir.
// A dir in no worktree has no hub config to name a command, so nothing runs.
func placeTerminalWindow(dir string) error {
	l, err := lymcwd.ResolveWorktree(dir)
	if err != nil {
		logger.Debug("reed window: the window's directory is in no worktree, running no window command", "dir", dir, "cause", err)
		return nil
	}
	return desktophook.AfterWindow(fabricengine.ConfigDirs(l), dir)
}

// terminalWindowTitle returns the title of a terminal window on dir:
// the console title of the reed session of the worktree holding dir, or dir's base name when dir is in no worktree.
// A prime it cannot resolve is logged at Warn, and the worktree is then titled as a task.
func terminalWindowTitle(dir string) string {
	l, err := lymcwd.ResolveWorktree(dir)
	if err != nil {
		logger.Debug("reed window: the window's directory is in no worktree, titling it with its base name", "dir", dir, "cause", err)
		return filepath.Base(dir)
	}
	primeName, err := fabricengine.PrimeName(l)
	if err != nil {
		logger.Warn("reed window: could not resolve the prime, titling the window as a task's", "dir", dir, "cause", err)
	}
	return boardengine.ConsoleWindowTitle(l.HubPath, primeName, l.WorktreeName)
}

// windowCmd builds the `window` subcommand.
// It opens a window on any directory the caller names;
// the window runs only `lym reed attach` there, which attaches to that worktree's own reed session and nothing else.
func (c *reedCLI) windowCmd() *cobra.Command {
	var title string

	cmd := &cobra.Command{
		Use:         "window <worktree-dir>",
		Short:       "open a desktop terminal window attached to a worktree's reed session",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `window opens one desktop terminal window whose shell runs
"lym reed attach" in the told worktree directory, and returns once the
launcher process has started, so the window outlives this command. When
the machine names a window command in desktop.yaml, it runs it next, after
the configured wait, and a failure of that command only warns. The
title defaults to the console title of the worktree holding the
directory, read from the hub's board: "<T#> [<hub shortname>:<P#>] <slug>"
for a task, and "ORCH [<hub shortname>:<P#>] <slug>" for a beam, whose
sessions hold an orch. The prime's names the root project, as
"ORCH [<hub shortname>:P0] root". A part that does not exist is left out. A directory in no worktree
takes its base name. A directory that does not exist is refused before
any launcher runs, and a machine with no terminal launcher reports why
on the envelope.

Example:
  lym reed window /path/to/hub/my-task`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			dir := args[0]
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("window: %s is not an existing directory", dir)))
				return nil
			}
			windowTitle := title
			if windowTitle == "" {
				formTitle := c.windowTitle
				if formTitle == nil {
					formTitle = terminalWindowTitle
				}
				windowTitle = formTitle(dir)
			}
			resolveLym := c.lymBinary
			if resolveLym == nil {
				resolveLym = selfexec.LymBinary
			}
			lymPath, err := resolveLym()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("window: resolve the running lym: %v", err)))
				return nil
			}

			open := c.openTerminal
			if open == nil {
				open = openTerminalWindow
			}
			if err := open(dir, windowTitle, lymPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("window: %v", err)))
				return nil
			}
			place := c.placeWindow
			if place == nil {
				place = placeTerminalWindow
			}
			if err := place(dir); err != nil {
				logger.Warn("reed window: the window command failed; the window is open", "dir", dir, "cause", err)
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"dir": dir, "title": windowTitle}))
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "window title (default: the worktree's console title, or the directory's base name outside a worktree)")

	return cmd
}
