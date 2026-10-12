// lock.go implements the `lock` verb group of batten: `lyx batten lock list` shows the hub's parent locks, and `lyx batten lock release` frees one whose owner is alive but wedged.

package battencli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/parentlock"
)

// openLockStore resolves the store the lock verbs act on.
// A test replaces it to skip the hub the real resolution needs, which makes such a test serial.
var openLockStore = lockStore

// lockCommand returns the `lock` verb group.
func lockCommand() *cobra.Command {
	operator := map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator}
	group := &cobra.Command{
		Use:   "lock",
		Short: "see and free the parent locks of the hub's runs",
		Long: `lock shows the parent locks the hub's runs hold on parent branches, and frees one
whose owner is alive but wedged. Both verbs run from the hub's prime worktree only.
A release frees the record and never stops a run: the released run takes the lock
again at its next lock-taking row and reroutes if the parent moved meanwhile.

Example:
  lyx batten lock list
  lyx batten lock release main
  lyx batten lock release --all`,
		RunE: clihelp.GroupRunE,
	}
	list := &cobra.Command{
		Use:         "list",
		Short:       "list every parent lock with its owner, age and queued waiters",
		Annotations: operator,
		Args:        cobra.NoArgs,
		RunE:        runLockList,
	}
	release := &cobra.Command{
		Use:         "release [<parent-branch>]",
		Short:       "force-release the parent lock of a branch, or every lock with --all",
		Annotations: operator,
		Args:        cobra.MaximumNArgs(1),
		RunE:        runLockRelease,
	}
	release.Flags().Bool("all", false, "release every parent lock instead of one branch's")
	group.AddCommand(list, release)
	return group
}

// inLockGroup reports whether cmd is the lock group or one of its verbs.
func inLockGroup(cmd *cobra.Command) bool {
	return cmd.Name() == "lock" || (cmd.Parent() != nil && cmd.Parent().Name() == "lock")
}

// lockStore resolves the cwd, refuses outside the hub's prime worktree and returns the store at the hub's parent lock directory.
func lockStore(ctx context.Context) (*parentlock.Store, error) {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return nil, err
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	if err := RefuseUnlessPrime(location); err != nil {
		return nil, err
	}
	return &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(location.HubPath))}, nil
}

// runLockList prints one entry per parent lock record.
// A record shows its branch, owner slug, pair, run id and pid, whether the owner is alive, its age from the time it was acquired and its queued waiters oldest first; a record that cannot be read shows its branch and the error.
func runLockList(cmd *cobra.Command, args []string) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	out := cmd.OutOrStdout()
	store, err := openLockStore(cmd.Context())
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
		return nil
	}
	entries, err := store.List()
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
		return nil
	}
	now := time.Now()
	locks := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.Err != "" {
			locks = append(locks, map[string]any{"branch": entry.Record.Branch, "error": entry.Err})
			continue
		}
		owner := entry.Record.Owner
		waiters := make([]map[string]any, 0, len(entry.Waiters))
		for _, waiter := range entry.Waiters {
			waiters = append(waiters, map[string]any{
				"slug":      waiter.Owner.Slug,
				"pair":      waiter.Owner.Pair,
				"run_id":    waiter.Owner.RunID,
				"pid":       waiter.Owner.PID,
				"queued_at": waiter.QueuedAt,
				"age":       now.Sub(waiter.QueuedAt).Round(time.Second).String(),
			})
		}
		locks = append(locks, map[string]any{
			"branch":      entry.Record.Branch,
			"slug":        owner.Slug,
			"pair":        owner.Pair,
			"run_id":      owner.RunID,
			"pid":         owner.PID,
			"alive":       entry.Alive,
			"acquired_at": entry.Record.AcquiredAt,
			"age":         now.Sub(entry.Record.AcquiredAt).Round(time.Second).String(),
			"waiters":     waiters,
		})
	}
	clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"locks": locks}))
	return nil
}

// runLockRelease force-releases the record of the one branch args names, or every record under --all.
// A branch with no record, a branch beside --all, and neither form are refused naming the way forward.
func runLockRelease(cmd *cobra.Command, args []string) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	out := cmd.OutOrStdout()
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}
	if all == (len(args) == 1) {
		clihelp.SetExit(cmd.Context(), output.Err(out, `lyx batten lock release takes either one parent branch or --all; way forward: run "lyx batten lock release <parent-branch>" or "lyx batten lock release --all"`))
		return nil
	}
	store, err := openLockStore(cmd.Context())
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
		return nil
	}
	released := []string{}
	if all {
		released, err = store.ReleaseAll()
		if err != nil {
			clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
			return nil
		}
		if released == nil {
			released = []string{}
		}
	} else {
		branch := args[0]
		freed, err := store.ForceRelease(branch)
		if err != nil {
			clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
			return nil
		}
		if !freed {
			clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf(`no parent lock on %q; way forward: run "lyx batten lock list" to see the held locks`, branch)))
			return nil
		}
		released = append(released, branch)
	}
	clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"released": released}))
	return nil
}
