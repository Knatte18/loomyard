// teardownchild.go builds `lym fabric remote-teardown`, the hidden verb a removal, a task-branch discard or a cleanup sweep starts as a detached child to run its remote steps.

package fabriccli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/spf13/cobra"
)

// warnWhenEnded makes a signal that ends the teardown child of slug in the hub at hubPath log and mail a notice naming the way forward, since the signal can arrive before the job's last remote step.
// The returned function stops the watch once the child's body has returned.
func warnWhenEnded(hubPath, slug string) (stop func()) {
	ended := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(ended, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-ended:
			fabricengine.NoticeRemoteTeardownEnded(hubPath, slug, "the child was ended by the signal "+sig.String())
			os.Exit(1)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(ended)
		close(done)
	}
}

// teardownChildCmd builds the hidden teardown child verb.
// It writes no envelope: its stdout carries the one line that tells the parent whether it holds the teardown lock, and the parent stops reading after it.
func teardownChildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         fabricengine.TeardownChildVerb,
		Short:       "hold a pair's teardown lock and run the remote steps of its teardown",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `remote-teardown takes the named slug's teardown lock exclusive, answers held or busy on one line,
reads the remote teardown job its parent writes to stdin and runs it, then releases the lock.
lym fabric remove, a task-branch discard and lym fabric cleanup --apply start it from the hub's Bolt checkout;
it is not a verb to run by hand.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			slug, _ := cmd.Flags().GetString(fabricengine.TeardownChildSlugFlag)
			try, _ := cmd.Flags().GetBool(fabricengine.TeardownChildTryFlag)
			wait, _ := cmd.Flags().GetDuration(fabricengine.TeardownChildWaitFlag)
			cwd, err := lymcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), 1)
				return nil
			}
			// The child starts in the hub's Bolt checkout, so the hub is the parent of the worktree root found there.
			hubLocation, err := lymcwd.ResolveWorktree(cwd)
			if err != nil {
				clihelp.SetExit(cmd.Context(), 1)
				return nil
			}
			logger.SetDurableSinkDir(fabricengine.HubLogsDir(hubLocation.HubPath))
			defer warnWhenEnded(hubLocation.HubPath, slug)()
			if err := fabricengine.RunTeardownChild(hubLocation.HubPath, slug, try, wait, os.Stdin, os.Stdout); err != nil {
				logger.Warn("fabriccli: teardown child failed", "slug", slug, "error", err)
				clihelp.SetExit(cmd.Context(), 1)
			}
			return nil
		},
	}
	cmd.Flags().String(fabricengine.TeardownChildSlugFlag, "", "the slug whose teardown lock the child holds")
	cmd.Flags().Bool(fabricengine.TeardownChildTryFlag, false, "take the lock by one attempt instead of waiting")
	cmd.Flags().Duration(fabricengine.TeardownChildWaitFlag, fabricengine.StatusPushLockWait, "how long to wait for the lock")
	return cmd
}
