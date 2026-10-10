// remove.go implements the `remove` reed verb: deletes a strand by guid or by fixed name, requiring
// --recursive for a non-leaf so children are never silently orphaned, and reports every strand
// actually removed. `--name --detach` lets a strand remove itself: a detached child, started from
// outside the strand's own process tree, waits for the invoking process to exit and then removes.

package reedcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// pidAlive is the liveness check the parent-exit wait polls; a package variable so a test can
// substitute a fake without a real process.
var pidAlive = proc.IsAlive

// validateRemoveArgs enforces the verb's argument rules: exactly one of the positional guid and
// --name, and --detach only with --name.
func validateRemoveArgs(guidArgs []string, name string, detach bool) error {
	hasGUID := len(guidArgs) > 0
	hasName := name != ""
	switch {
	case hasGUID && hasName:
		return errors.New("give either a positional <guid> or --name, not both")
	case !hasGUID && !hasName:
		return errors.New("a strand is required: give a positional <guid> or --name")
	case detach && !hasName:
		return errors.New("--detach requires --name")
	}
	return nil
}

// waitParentExit blocks until the process waitPID is gone, polling with a capped attempt count.
// A zero waitPID means no wait was requested.
func waitParentExit(waitPID int) error {
	if waitPID == 0 {
		return nil
	}
	if !reedengine.WaitPIDGone(waitPID, pidAlive, reedengine.ParentExitMaxAttempts, reedengine.ParentExitPollInterval) {
		return fmt.Errorf("parent process %d still alive after %d polls; not removing", waitPID, reedengine.ParentExitMaxAttempts)
	}
	return nil
}

// spawnDetachedRemove starts a detached `lyx reed remove <guid>` in the invoking worktree, handing it
// this process's pid so it removes only once the pane's descendant (this process) is gone. It is a
// no-op under a test binary (suppress), where re-exec'ing os.Executable() would run the suite.
func (c *reedCLI) spawnDetachedRemove(guid string, recursive bool) error {
	if c.suppressWatchdogSpawn {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve this binary: %w", err)
	}
	args := []string{"reed", "remove", guid, "--wait-pid", strconv.Itoa(os.Getpid())}
	if recursive {
		args = append(args, "--recursive")
	}
	cmd := exec.Command(exe, args...)
	// The worktree's anchor, never the hub: the strand verbs resolve from a cwd that must equal the
	// anchor. A child that exits once the removal is done cannot pin the worktree's deletion the way
	// the watchdog could.
	cmd.Dir = c.anchorPath
	proc.Detach(cmd)

	logger.Info("reed: spawning detached remove", "exe", exe, "guid", guid, "dir", c.anchorPath, "wait_pid", os.Getpid())
	if err := cmd.Start(); err != nil { // no Wait: detached, so the spawn is the only lifecycle line
		logger.Warn("reed: detached remove spawn failed", "exe", exe, "guid", guid, "err", err)
		return fmt.Errorf("spawn detached remove: %w", err)
	}
	return nil
}

// markAndSpawnDetachedRemove marks guid retiring, then starts the detached remover.
// A failed spawn clears the mark again, so no strand is left marked with no remover behind it;
// a failed clear is logged and named in the returned error.
func (c *reedCLI) markAndSpawnDetachedRemove(guid string, recursive bool) error {
	eng := c.strandEngine()
	if err := eng.MarkRetiring(guid, true); err != nil {
		return fmt.Errorf("mark strand retiring: %w", err)
	}
	spawn := c.spawnRemove
	if spawn == nil {
		spawn = c.spawnDetachedRemove
	}
	spawnErr := spawn(guid, recursive)
	if spawnErr == nil {
		return nil
	}
	if clearErr := eng.MarkRetiring(guid, false); clearErr != nil {
		logger.Warn("reed: clearing the retiring mark after a failed detached remove spawn failed", "guid", guid, "err", clearErr)
		return fmt.Errorf("%w; clearing the retiring mark also failed: %v", spawnErr, clearErr)
	}
	return spawnErr
}

// removeCmd builds the `remove` subcommand: deletes the strand identified by <guid> or --name.
func (c *reedCLI) removeCmd() *cobra.Command {
	var (
		recursive bool
		name      string
		detach    bool
		waitPID   int
	)

	cmd := &cobra.Command{
		Use:         "remove [<guid>]",
		Short:       "remove a strand from the reed layout",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `remove deletes the strand identified by <guid> or by --name (exactly one
of the two); --name takes a role segment (driver) or a full name
(shortname:slug:driver). Removing a strand that has children requires --recursive,
which cascades the removal through the strand's whole descendant subtree;
without --recursive a non-leaf remove is rejected outright, so children are
never silently orphaned.

--name resolves the guid among this worktree's strands and refuses an unknown
or ambiguous name. --detach (with --name) lets a strand remove itself: it
resolves the guid, starts a detached "lyx reed remove <guid>" that waits for
this process to exit before removing, prints {"detached": true, "guid",
"name"} and returns at once, since reed kills the strand's pane before it
re-applies the layout. --detach first marks the strand retiring (shown by
"lyx reed status"), and clears the mark if the detached remover cannot be
started. The detached remover is a no-op for a guid already gone, answering
ok with an empty "removed" list; a positional <guid> typed by an operator
still errors on an unknown guid.

Example:
  lyx reed remove 3fae21ac9b1d4c0e --recursive
  lyx reed remove --name driver --detach`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := validateRemoveArgs(args, name, detach); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			guid := ""
			if len(args) == 1 {
				guid = args[0]
			} else {
				resolved, err := c.strandEngine().ResolveStrandGUID(name)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				guid = resolved
			}

			if detach {
				if err := c.markAndSpawnDetachedRemove(guid, recursive); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
					"detached": true,
					"guid":     guid,
					"name":     name,
				}))
				return nil
			}

			if err := waitParentExit(waitPID); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			removed, err := c.strandEngine().RemoveStrand(guid, recursive)
			if waitPID != 0 && errors.Is(err, reedengine.ErrUnknownStrand) {
				// The detached remover found its strand already gone, as when `lyx loom start` replaced it under a new guid first: nothing left to remove.
				logger.Info("reed: detached remove found its strand already gone", "guid", guid)
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
					"removed": []map[string]any{},
				}))
				return nil
			}
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			list := make([]map[string]any, len(removed.Strands))
			for i, s := range removed.Strands {
				list[i] = map[string]any{"guid": s.GUID, "name": s.Name}
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"removed": list,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&recursive, "recursive", false, "cascade removal through the strand's whole descendant subtree")
	cmd.Flags().StringVar(&name, "name", "", "remove the strand this role segment or full name addresses, instead of a positional guid")
	cmd.Flags().BoolVar(&detach, "detach", false, "with --name: resolve the guid, start a detached remove that waits for this process to exit, and return at once")
	cmd.Flags().IntVar(&waitPID, "wait-pid", 0, "internal: wait for this pid to exit before removing (set by --detach)")
	if err := cmd.Flags().MarkHidden("wait-pid"); err != nil {
		panic(err)
	}

	return cmd
}
