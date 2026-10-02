// list.go implements the `list` reed verb: the hub-wide name directory, one row per strand across every task worktree and the prime.
// Each row shows the strand's pane title and whether it has drifted from the name.
// It is read-only — each worktree's engine only reads its own state and its tmux panes.

package reedcli

import (
	"sort"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// listCmd builds the `list` subcommand.
func (c *reedCLI) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list every strand in the hub with its full name, pane and title drift",
		Long: `list enumerates the hub's worktrees and reports every strand tracked in
each: full name, guid, worktree, pane id, the pane's current title, whether
the strand is live, and whether its title has drifted from its name. It is
read-only; the watchdog repairs drift, this verb only shows it.

A worktree that cannot be read is skipped with a warning in the log, so one
broken worktree never hides the rest.

Example:
  lyx reed list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			entries, err := fabricengine.List(c.anchorPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			var rows []reedengine.DirectoryRow
			for _, entry := range entries {
				if entry.Prunable {
					continue
				}
				rows = append(rows, worktreeDirectory(entry.Path)...)
			}
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

			strands := make([]map[string]any, len(rows))
			for i, r := range rows {
				strands[i] = map[string]any{
					"name":     r.Name,
					"guid":     r.GUID,
					"worktree": r.Worktree,
					"pane_id":  r.PaneID,
					"title":    r.Title,
					"live":     r.Live,
					"drift":    r.Drift,
				}
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"strands": strands}))
			return nil
		},
	}
}

// worktreeDirectory reads one worktree's name directory, answering nothing for a worktree that cannot be resolved or read.
func worktreeDirectory(path string) []reedengine.DirectoryRow {
	location, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		logger.Warn("reed list: skipping a worktree that does not resolve", "path", path, "err", err)
		return nil
	}
	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
	if err != nil {
		logger.Warn("reed list: skipping a worktree whose config does not load", "path", path, "err", err)
		return nil
	}
	geom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		logger.Warn("reed list: skipping a worktree whose geometry does not resolve", "path", path, "err", err)
		return nil
	}
	rows, err := reedengine.New(cfg, geom).Directory()
	if err != nil {
		logger.Warn("reed list: skipping a worktree whose strands cannot be read", "path", path, "err", err)
		return nil
	}
	return rows
}
