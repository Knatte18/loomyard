// window.go implements the `window` reed verb: it opens one desktop terminal window attached to a told worktree's reed session through internal/termwindow.
// It needs no location, config or engine, so the root pre-run skips it.

package reedcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/termwindow"
	"github.com/spf13/cobra"
)

// openTerminalWindow opens a terminal window titled title whose shell runs `lyxPath reed attach` in dir, through the launcher this machine resolves.
func openTerminalWindow(dir, title, lyxPath string) error {
	launcher, err := termwindow.Resolve()
	if err != nil {
		return err
	}
	return launcher.OpenAttach(dir, title, lyxPath)
}

// windowCmd builds the `window` subcommand.
// It opens a window on any directory the caller names; the window runs only `lyx reed attach` there, which attaches to that worktree's own reed session and nothing else.
func (c *reedCLI) windowCmd() *cobra.Command {
	var title string

	cmd := &cobra.Command{
		Use:         "window <worktree-dir>",
		Short:       "open a desktop terminal window attached to a worktree's reed session",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `window opens one desktop terminal window whose shell runs
"lyx reed attach" in the told worktree directory, and returns once the
launcher process has started, so the window outlives this command. The
title defaults to the directory's base name. A directory that does not
exist is refused before any launcher runs, and a machine with no terminal
launcher reports why on the envelope.

Example:
  lyx reed window /path/to/hub/my-task --title my-task`,
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
				windowTitle = filepath.Base(dir)
			}
			lyxPath, err := os.Executable()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("window: resolve the running lyx: %v", err)))
				return nil
			}

			open := c.openTerminal
			if open == nil {
				open = openTerminalWindow
			}
			if err := open(dir, windowTitle, lyxPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("window: %v", err)))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"dir": dir, "title": windowTitle}))
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "window title (default: the directory's base name)")

	return cmd
}
