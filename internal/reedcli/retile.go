// retile.go implements the `retile` reed verb.
// It puts a hub's open terminal windows back on their home desktops and tiling zones through internal/kwinplace.
// The hub is the one holding the cwd.

package reedcli

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/kwinplace"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// retileVerbName is the name the retile verb is called by.
const retileVerbName = "retile"

// retileWindowMode returns the lookup, by worktree name, of the window placement mode in force in prime's hub.
// primeName is the prime worktree's name, and machineMode is the machine's own mode.
func retileWindowMode(prime *lymcwd.Location, primeName, machineMode string) func(string) (string, error) {
	return func(worktree string) (string, error) {
		// An empty slug names the prime to the settings chain.
		slug := worktree
		if worktree == primeName {
			slug = ""
		}
		return windowPlacementMode(prime, slug, machineMode)
	}
}

// retileHubWindows retiles the open windows of the hub holding location, under the desktop config of its worktree.
func retileHubWindows(ctx context.Context, location *lymcwd.Location) (kwinplace.RetileReport, error) {
	dirs := fabricengine.ConfigDirs(location)
	if dirs.Board == "" {
		return kwinplace.RetileReport{}, errors.New("the working directory belongs to no hub; way forward: run it from a worktree of the hub whose windows are to be retiled")
	}
	// Every window a placement recorded is a Konsole window, so the session alone is checked.
	if err := kwinplace.PreLaunch(true); err != nil {
		return kwinplace.RetileReport{}, fmt.Errorf("this session cannot move windows: %w; way forward: run it from a terminal of the KDE Plasma 6 Wayland session the windows are on", err)
	}
	// The load's refusals of a desktop key name the command that sets it.
	desktop, err := desktophook.LoadConfig(dirs)
	if err != nil {
		return kwinplace.RetileReport{}, fmt.Errorf("load the desktop config: %w", err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return kwinplace.RetileReport{}, fmt.Errorf("resolve the prime; way forward: repair the hub's worktrees, then retile again: %w", err)
	}
	prime, err := lymcwd.NamedWorktree(location.HubPath, primeName)
	if err != nil {
		return kwinplace.RetileReport{}, fmt.Errorf("resolve the prime's worktree; way forward: repair it, then retile again: %w", err)
	}
	runner, err := kwinplace.NewRunner()
	if err != nil {
		return kwinplace.RetileReport{}, err
	}
	return kwinplace.NewFlow(runner, nil).Retile(ctx, kwinplace.RetileRequest{
		BoltDir:       fabriclayout.BoltDir(location.HubPath),
		MinTileWidth:  desktop.Tiling.MinTileWidth,
		MinTileHeight: desktop.Tiling.MinTileHeight,
		Mode:          retileWindowMode(prime, primeName, desktop.WindowPlacement),
	})
}

// retileCmd builds the `retile` subcommand.
func (c *reedCLI) retileCmd() *cobra.Command {
	return &cobra.Command{
		Use:         retileVerbName,
		Short:       "put the hub's terminal windows back on their home desktops and tiling zones",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `retile puts every open terminal window of this hub that window placement
remembers back on its home desktop, tiled in its zone, and returns once
KWin has been told. It takes the lock window placement takes, so it waits
for a placement that is under way, and no window opens into the grid it
builds. Each home desktop's grid holds the hub's windows homed there in
their remembered order, then the desktop's other windows as they stand;
a window that belongs to no hub is never moved off its desktop. A desktop
whose grid already stands so is left untouched.

A window is left where it is while its placement still runs, when window
placement is off for its worktree, when it is minimized, when it has no
home desktop or its home is no longer a desktop, and when its home has no
zone left for it. A window moved by hand to another desktop takes that
desktop as its home once another window is placed, and retile then leaves
it there.

The envelope lists the windows "retiled", the windows "in_place" and the
windows "not_placed" with the reason for each. It runs only from a
terminal of the KDE Plasma 6 Wayland session the windows are on.

Example:
  lym reed retile`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			report, err := retileHubWindows(ctx, location)
			fields := map[string]any{
				"retiled":    report.Retiled,
				"in_place":   report.InPlace,
				"not_placed": report.NotPlaced,
			}
			if err != nil {
				// A retile cut short has moved windows, so its error envelope keeps the lists.
				if len(report.Retiled)+len(report.InPlace)+len(report.NotPlaced) == 0 {
					fields = nil
				}
				clihelp.SetExit(ctx, output.ErrFields(out, fmt.Sprintf("%s: %v", retileVerbName, err), fields))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, fields))
			return nil
		},
	}
}
