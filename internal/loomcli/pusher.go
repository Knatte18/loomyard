// pusher.go implements the hidden `pusher` loom verb, the process of a target's background pusher.
// A pair's pusher runs loom's pre-push check, which is why the verb lives here.

package loomcli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// pusherResolveWayForward ends the refusal of a pusher whose --worktree names no code worktree.
const pusherResolveWayForward = "; way forward: run lym loom pusher --worktree with the path of an existing code worktree; the next commit, dobby tick or resume starts the pusher again"

// pusherHubWayForward ends the refusal of a board or landing pusher whose hub has no readable prime.
const pusherHubWayForward = "; way forward: run lym loom pusher with --worktree naming a worktree of a hub whose prime stands; the next commit or dobby tick starts the pusher again"

// pusherCmd builds the hidden `pusher` subcommand.
// It resolves the worktree its --worktree flag names, never the cwd, since the pusher starts in the hub's Bolt directory.
func (c *loomCLI) pusherCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "pusher",
		Hidden:      true,
		Short:       "bring one worktree's records, the board or the landing target to origin (started by every commit, tick and resume)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `pusher is the background process that brings one target's commits to origin in order.
A commit of a run's records, a board write, a dobby tick and a resume start it, never an operator.
It takes the target's pusher lock and exits at once when another pusher holds it.
With --board or --landing the target is the hub's, and --worktree names any directory directly in the hub.
With --push-now it first marks the target to push without waiting out the interval.
A pair's pusher runs the pre-push check before each push; the others run none.
It retries an owed push once per interval in force, quietly, and stops on a refused push, a hand-over, a pause or a stop of the check.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()
			worktree, _ := cmd.Flags().GetString(runpusher.WorktreeFlag)
			slug, _ := cmd.Flags().GetString(runpusher.SlugFlag)
			board, _ := cmd.Flags().GetBool(fabriclayout.PusherBoardFlag)
			landing, _ := cmd.Flags().GetBool(fabriclayout.PusherLandingFlag)
			pushNow, _ := cmd.Flags().GetBool(fabriclayout.PusherPushNowFlag)

			var target runpusher.Target
			var prime *lymcwd.Location
			if board || landing {
				hubPrime, err := primeOfHubHolding(worktree)
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()+pusherHubWayForward))
					return nil
				}
				prime = hubPrime
				target = runpusher.LandingTarget(prime)
				if board {
					target = runpusher.BoardTarget(prime)
				}
			} else {
				location, err := lymcwd.ResolveWorktree(worktree)
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()+pusherResolveWayForward))
					return nil
				}
				target, err = runpusher.TargetOf(location, slug)
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
				prime, err = primeOfHub(location)
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
			}
			pushRun := func(prime, l *lymcwd.Location, slug string, push pairteardown.RunPush) error {
				_, err := pairteardown.PushRun(prime, l, slug, push)
				return err
			}
			deps := pusherDeps(prime, target, newPrePushCheck(target.Location, shedrun.SelfRunID), pushRun)
			if pushNow {
				// Set before the lock is tried, so the pusher holding it wakes on the mark.
				if err := markPushNow(deps, target); err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
			}
			if err := runpusher.Run(ctx, target, deps); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
			}
			return nil
		},
	}
	cmd.Flags().String(runpusher.WorktreeFlag, "", "the code worktree whose records the pusher pushes")
	cmd.Flags().String(runpusher.SlugFlag, "", "the run slug of a pair; the prime names none")
	cmd.Flags().Bool(fabriclayout.PusherBoardFlag, false, "push the board's branch")
	cmd.Flags().Bool(fabriclayout.PusherLandingFlag, false, "serve the landing target of the prime's code branch")
	cmd.Flags().Bool(fabriclayout.PusherPushNowFlag, false, "push without waiting out the interval")
	_ = cmd.MarkFlagRequired(runpusher.WorktreeFlag)
	return cmd
}

// primeOfHubHolding returns the prime of the hub whose directory holds worktree, which may be the board directory.
// It reads the prime from the board's own repository, whose main worktree is the prime's records.
func primeOfHubHolding(worktree string) (*lymcwd.Location, error) {
	abs, err := filepath.Abs(worktree)
	if err != nil {
		return nil, fmt.Errorf("locate the hub of %s: %w", worktree, err)
	}
	boardDir := fabriclayout.BoardDir(filepath.Dir(abs))
	entries, err := fabricengine.List(boardDir)
	if err != nil {
		return nil, fmt.Errorf("read the worktrees of the board at %s: %w", boardDir, err)
	}
	for _, entry := range entries {
		if !entry.Main {
			continue
		}
		prime, err := lymcwd.ResolveWithin(filepath.FromSlash(entry.Path))
		if err != nil {
			return nil, fmt.Errorf("locate the hub's prime from %s: %w", entry.Path, err)
		}
		return prime, nil
	}
	return nil, fmt.Errorf("the board at %s lists no main worktree", boardDir)
}

// markPushNow asks the pusher of t to push without waiting out its interval.
func markPushNow(deps runpusher.Deps, t runpusher.Target) error {
	return pushledger.Update(deps.HubPath, deps.Key, time.Now(), func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
		entry.PushNow = true
		if t.Kind == runpusher.KindRecords {
			entry.Worktree = t.Location.WorktreePath()
			entry.Slug = t.Slug
		}
		return entry, true, nil
	})
}

// pusherDeps fills the seams of the pusher of t that the hub's reads leave to loom.
// A records target pushes through pushRun, with the owed push's failure left unrecorded since the pusher retries it.
// A pair pushes its pair for its slug inside the pair, so a takeover leaves the pair for the parent's dobby.
// The prime pushes its own records and names no run, so a takeover of any batten run is handed over.
// Guard, for a pair only, is the pre-push check: the run-scope origin reconcile, with its stops.
// The board target commits and pushes the hub's board.
// The landing target pushes nothing but the landing tips of its parent step, and a beam pair runs both.
// Each push is one transport attempt, since the pusher retries on its own interval.
// The interval comes from the worktree's fabric config, and a config that does not load leaves the defaults.
func pusherDeps(prime *lymcwd.Location, t runpusher.Target, check prePushCheck, pushRun func(prime, l *lymcwd.Location, slug string, push pairteardown.RunPush) error) runpusher.Deps {
	deps := runpusher.NewDeps(t, pushTimingOf(t.Location))
	switch t.Kind {
	case runpusher.KindBoard:
		hubBoard := boardengine.New(boardengine.ApplySkipEnv(boardengine.Config{Path: fabriclayout.BoardDir(t.Location.HubPath)}))
		deps.Push = hubBoard.PusherSync
		return deps
	case runpusher.KindLanding:
		deps.Push = func() error { return nil }
		deps.Parent.PushTip = func(tip string) error {
			options := fabricengine.EnvSyncOptions()
			options.OwedUnrecorded = true
			return fabricengine.PushPrimeCodeTip(prime, tip, options)
		}
		deps.Parent.HandOver = func(run, owner string) error {
			return pairteardown.HandOverOwnedElsewhere(prime, run, owner, pairteardown.RunPush{})
		}
		return deps
	}
	push := pairteardown.RunPush{LockWait: fabriclayout.StatusPushLockWait, OwedUnrecorded: true, SingleAttempt: true}
	if t.Pair {
		push.TeardownWait = fabriclayout.StatusPushLockWait
		push.InsidePair = true
		deps.Guard = check.guard
	}
	deps.Push = func() error { return pushRun(prime, t.Location, t.Slug, push) }
	if deps.Parent.Fetch != nil {
		deps.Parent.PushTip = func(tip string) error {
			capped := push
			capped.CodeCeiling = tip
			return pushRun(prime, t.Location, t.Slug, capped)
		}
		deps.Parent.Refresh = func() error { return pairteardown.RefreshOwners(prime, t.Slug) }
		deps.Parent.HandOver = func(run, owner string) error {
			return pairteardown.HandOverOwnedElsewhere(prime, run, owner, pairteardown.RunPush{})
		}
	}
	return deps
}

// pushTimingOf reads the push timing of the worktree l, zero when its fabric config does not load.
// It logs each value the timing had to raise.
func pushTimingOf(l *lymcwd.Location) fabricengine.PushTiming {
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(l))
	if err != nil {
		logger.Warn("loom pusher: fabric config not loaded, pushing on the default timing", "worktree", l.WorktreePath(), "error", err)
		return fabricengine.PushTiming{}
	}
	timing, warnings := config.PushTiming()
	for _, warning := range warnings {
		logger.Warn("loom pusher: " + warning)
	}
	return timing
}
