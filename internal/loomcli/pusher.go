// pusher.go implements the hidden `pusher` loom verb, the process of a worktree's background pusher.
// A pair's pusher runs loom's pre-push check, which is why the verb lives here.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// pusherResolveWayForward ends the refusal of a pusher whose --worktree names no code worktree.
const pusherResolveWayForward = "; way forward: run lym loom pusher --worktree with the path of an existing code worktree; the next commit, dobby tick or resume starts the pusher again"

// pusherCmd builds the hidden `pusher` subcommand.
// It resolves the worktree its --worktree flag names, never the cwd, since the pusher starts in the hub's Bolt directory.
func (c *loomCLI) pusherCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "pusher",
		Hidden:      true,
		Short:       "push one worktree's run commits to origin in order (started by every commit, tick and resume)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `pusher is the background process that brings one worktree's run commits to origin in order.
A commit of a run's records, a dobby tick and a resume start it, never an operator.
It takes the worktree's pusher lock and exits at once when another pusher holds it.
A pair's pusher runs the pre-push check before each push; the prime's runs none.
It retries a push that is owed without end, and stops on a refused push, a hand-over, a pause or a stop of the check.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()
			worktree, _ := cmd.Flags().GetString(runpusher.WorktreeFlag)
			slug, _ := cmd.Flags().GetString(runpusher.SlugFlag)

			location, err := lymcwd.ResolveWorktree(worktree)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()+pusherResolveWayForward))
				return nil
			}
			target, err := runpusher.TargetOf(location, slug)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			prime, err := primeOfHub(location)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			pushRun := func(prime, l *lymcwd.Location, slug string, push pairteardown.RunPush) error {
				_, err := pairteardown.PushRun(prime, l, slug, push)
				return err
			}
			deps := pusherDeps(prime, target, newPrePushCheck(location, shedrun.SelfRunID), pushRun)
			if err := runpusher.Run(ctx, target, deps); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
			}
			return nil
		},
	}
	cmd.Flags().String(runpusher.WorktreeFlag, "", "the code worktree whose records the pusher pushes")
	cmd.Flags().String(runpusher.SlugFlag, "", "the run slug of a pair; the prime names none")
	_ = cmd.MarkFlagRequired(runpusher.WorktreeFlag)
	return cmd
}

// pusherDeps fills the seams of the pusher of t that the hub's reads leave to loom.
// Push goes through pushRun, with the owed push's failure left unrecorded since the pusher retries it itself.
// A pair pushes its pair for its slug inside the pair, so a takeover leaves the pair for the parent's dobby.
// The prime pushes its own records and names no run, so a takeover of any batten run is handed over.
// Guard, for a pair only, is the pre-push check: the run-scope origin reconcile, with its stops.
// Notify posts one line to the nearest live orch.
func pusherDeps(prime *lymcwd.Location, t runpusher.Target, check prePushCheck, pushRun func(prime, l *lymcwd.Location, slug string, push pairteardown.RunPush) error) runpusher.Deps {
	deps := runpusher.NewDeps(t)
	push := pairteardown.RunPush{LockWait: fabricengine.StatusPushLockWait, OwedUnrecorded: true}
	if t.Pair {
		push.TeardownWait = fabricengine.StatusPushLockWait
		push.InsidePair = true
		deps.Guard = check.guard
	}
	deps.Push = func() error { return pushRun(prime, t.Location, t.Slug, push) }
	deps.Notify = func(line string) error {
		_, err := orchcli.NotifyParent(t.Location, loomProgram, fmt.Sprintf("loom %s: %s", t.Location.WorktreeName, line))
		return err
	}
	return deps
}
