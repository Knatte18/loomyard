// placewindowchild.go implements the hidden `place-window` reed verb.
// The verb is the detached process that places one terminal window.
// It resolves the hub's facts from its flags.
// It places the window through internal/kwinplace and then runs the machine's window command.

package reedcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/kwinplace"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// handedWindow is the window the verb's flags tell it to place.
type handedWindow struct {
	hubPath  string
	worktree string
	title    string
	pid      int
	prime    bool
}

// placeWindowCmd builds the hidden `place-window` subcommand.
// It always exits zero: whatever keeps a window from being placed is logged, and the window stays open.
func (c *reedCLI) placeWindowCmd() *cobra.Command {
	var window handedWindow

	cmd := &cobra.Command{
		Use: fmt.Sprintf("%s --%s <path> --%s <name> --%s <title> --%s <n>",
			kwinplace.ChildVerbName, kwinplace.ChildHubFlag, kwinplace.ChildWorktreeFlag, kwinplace.ChildTitleFlag, kwinplace.ChildPIDFlag),
		Short:       "place a just-opened terminal window, then run the machine's window command",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			logger.SetDurableSinkDir(fabriclayout.HubLogsDir(window.hubPath))
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			placeHandedWindow(ctx, window)
			clihelp.SetExit(cmd.Context(), output.Ok(cmd.OutOrStdout(), map[string]any{"worktree": window.worktree}))
			return nil
		},
	}

	cmd.Flags().StringVar(&window.hubPath, kwinplace.ChildHubFlag, "", "hub holding the window's worktree")
	cmd.Flags().StringVar(&window.worktree, kwinplace.ChildWorktreeFlag, "", "name of the window's worktree")
	cmd.Flags().StringVar(&window.title, kwinplace.ChildTitleFlag, "", "title the window was opened with")
	cmd.Flags().IntVar(&window.pid, kwinplace.ChildPIDFlag, 0, "process id of the window")
	cmd.Flags().BoolVar(&window.prime, kwinplace.ChildPrimeFlag, false, "the window is the prime worktree's own")
	for _, name := range []string{kwinplace.ChildHubFlag, kwinplace.ChildWorktreeFlag, kwinplace.ChildTitleFlag, kwinplace.ChildPIDFlag} {
		// MarkFlagRequired fails only for a flag the command does not declare.
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}

	return cmd
}

// placeHandedWindow places the window and runs the window command of its worktree, logging whatever fails.
// A window whose placement cannot be resolved is still recorded and settled, so no other window waits on it.
func placeHandedWindow(ctx context.Context, window handedWindow) {
	location, err := lymcwd.NamedWorktree(window.hubPath, window.worktree)
	if err != nil {
		recordUnplacedWindow(window)
		logger.Warn("reed place-window: the worktree does not resolve; the window is left where it opened and no window command runs",
			"worktree", window.worktree, "hub", window.hubPath, "error", err)
		return
	}
	dirs := fabricengine.ConfigDirs(location)
	afterPlace := func(ctx context.Context) error {
		return desktophook.AfterPlacement(ctx, dirs, location.AnchorPath())
	}

	request, err := placementRequest(location, dirs, window)
	var runner *kwinplace.Runner
	if err == nil {
		runner, err = kwinplace.NewRunner()
	}
	if err != nil {
		recordUnplacedWindow(window)
		logger.Warn("reed place-window: the window is left where it opened", "worktree", window.worktree, "error", err)
		if ctx.Err() != nil {
			return
		}
		if err := afterPlace(ctx); err != nil {
			logger.Warn("reed place-window: the window command failed; the window is open", "worktree", window.worktree, "error", err)
		}
		return
	}

	if err := kwinplace.NewFlow(runner, afterPlace).Place(ctx, request); err != nil {
		logger.Warn("reed place-window: placement or window command failed; the window is open", "worktree", window.worktree, "error", err)
	}
}

// recordUnplacedWindow records the window in the hub's remembered places and settles its entry.
func recordUnplacedWindow(window handedWindow) {
	path := kwinplace.PlacesPath(fabriclayout.BoltDir(window.hubPath))
	now := time.Now()
	err := errors.Join(
		kwinplace.RecordOpen(path, window.worktree, window.pid, window.title, now),
		kwinplace.Settle(path, window.worktree, window.pid, "", now),
	)
	if err != nil {
		logger.Warn("reed place-window: window not recorded", "worktree", window.worktree, "error", err)
	}
}

// placementRequest resolves what placing the window of the worktree at location takes from its hub and config.
func placementRequest(location *lymcwd.Location, dirs configengine.Dirs, window handedWindow) (kwinplace.Request, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return kwinplace.Request{}, fmt.Errorf("resolve the prime; way forward: repair the hub's worktrees, then open the window again: %w", err)
	}
	prime, err := lymcwd.NamedWorktree(window.hubPath, primeName)
	if err != nil {
		return kwinplace.Request{}, fmt.Errorf("resolve the prime's worktree; way forward: repair it, then open the window again: %w", err)
	}

	// The prime's own window has no parent, and an empty slug names the prime to the settings chain.
	slug, parent := "", ""
	if !window.prime {
		slug = window.worktree
		parent, err = hubgeom.ParentOf(prime, window.worktree)
		if err != nil {
			return kwinplace.Request{}, fmt.Errorf("resolve the parent; way forward: repair what the error names, then open the window again: %w", err)
		}
		if parent == "" {
			parent = primeName
		}
	}

	perTaskKey := configengine.KeyName(dobbyengine.ConfigSpec().Name, dobbyengine.WindowPlacementKey)
	chain, err := hubgeom.TaskSettingChain(prime, slug)
	if err != nil {
		return kwinplace.Request{}, fmt.Errorf("resolve the chain %s is read over; way forward: repair what the error names, then open the window again: %w", perTaskKey, err)
	}
	perTask, err := dobbyengine.LookupWindowPlacement(chain)
	if err != nil {
		return kwinplace.Request{}, fmt.Errorf("read %s; way forward: run `lym config beam --set %s=<value>` in that worktree, or set it in the task's run_settings: %w",
			perTaskKey, dobbyengine.WindowPlacementKey, err)
	}
	// The load's refusals of a desktop key name the command that sets it.
	desktop, err := desktophook.LoadConfig(dirs)
	if err != nil {
		return kwinplace.Request{}, fmt.Errorf("load the desktop config: %w", err)
	}

	return kwinplace.Request{
		BoltDir:       fabriclayout.BoltDir(window.hubPath),
		Worktree:      window.worktree,
		Title:         window.title,
		PID:           window.pid,
		Prime:         window.prime,
		Parent:        parent,
		Mode:          kwinplace.Mode(perTask, desktop.WindowPlacement),
		MinTileWidth:  desktop.Tiling.MinTileWidth,
		MinTileHeight: desktop.Tiling.MinTileHeight,
		Worktrees:     func() ([]string, error) { return hubWorktreeNames(location.AnchorPath()) },
	}, nil
}

// hubWorktreeNames returns the name of every worktree of the repository holding dir.
func hubWorktreeNames(dir string) ([]string, error) {
	entries, err := fabricengine.List(dir)
	if err != nil {
		return nil, fmt.Errorf("list the hub's worktrees: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, filepath.Base(filepath.FromSlash(entry.Path)))
	}
	return names, nil
}
