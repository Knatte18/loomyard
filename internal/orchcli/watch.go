// watch.go implements the hidden `watch` orch verb: the detached daemon `start` launches, which polls the orchestrator session and runs the handoff cycle until the strand is gone.

package orchcli

import (
	"errors"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/spf13/cobra"
)

// realClock is the wall clock.
type realClock struct{}

// Now returns the current time.
func (realClock) Now() time.Time { return time.Now() }

// orchAgentName returns the agent name of the orch this receiver's worktree hosts: `<shortname>:orch` in the prime and `<shortname>:<slug>:orch` in a beam.
// A hub with no recorded shortname yields the empty name, which the watcher reads as the prime's orch.
func (c *orchCLI) orchAgentName() (string, error) {
	shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(c.location.HubPath))
	if !found {
		return "", nil
	}
	primeName, err := fabricengine.PrimeName(c.location)
	if err != nil {
		return "", err
	}
	slug := c.location.WorktreeName
	if slug == primeName {
		slug = ""
	}
	return agentname.Format(shortname, slug, agentname.RoleOrch)
}

// beamOwnerElsewhere reads the owner of the worktree location on the hub's local board, the prime's owner for the prime and a beam's effective owner through hubgeom.ParentOf for a beam.
// elsewhere is true when that owner names a machine other than this one; a worktree with no owner recorded is owned by no other machine.
func beamOwnerElsewhere(location *lymcwd.Location) (owner string, elsewhere bool, err error) {
	here, err := machinename.Name()
	if err != nil {
		return "", false, fmt.Errorf("orch: read this machine's name: %w", err)
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return "", false, fmt.Errorf("orch: read the board's owners: %w", err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return "", false, err
	}
	if location.WorktreeName == primeName {
		owner = owners.Prime
	} else {
		prime, err := resolvePrime(location)
		if err != nil {
			return "", false, err
		}
		owner, err = owners.Effective(location.WorktreeName, func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) })
		if err != nil {
			return "", false, fmt.Errorf("orch: resolve the owner of %q: %w", location.WorktreeName, err)
		}
	}
	return owner, boardengine.OwnedElsewhere(owner, here), nil
}

// watchCmd builds the `watch` subcommand, hidden because start launches it and an operator never needs to.
func (c *orchCLI) watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "watch",
		Short:       "run the orchestrator watcher daemon (launched by start)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `watch polls the orchestrator session and runs the handoff cycle until the strand
is gone or it is signalled. start launches it detached with its output in watch.log.
A second watcher finding the first alive reports already_running and exits.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			st, err := orchengine.LoadState(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st.Strand == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "orch: no orchestrator strand is recorded; nothing to watch -- run `lym beam start` first"))
				return nil
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			creel, err := c.newCreel()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			watcher := orchengine.NewWatcher(runnerSession{runner: c.runner, strands: c.strands}, creel, c.cfg, c.paths, c.stencilsDir, orchSkills, c.watcherIndexSource(), realClock{})
			pauseGeometry, err := hubgeom.PauseGeometry(c.location)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			watcher.WatchPause(orchengine.PauseWatch{RecordPath: pauseGeometry.RecordPath, Alive: parentlock.ProcessAlive})
			orchName, err := c.orchAgentName()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			watcher.SetOrchName(orchName)
			watcher.WatchOwner(orchengine.OwnerWatch{
				Read: func() (string, bool, error) { return beamOwnerElsewhere(c.location) },
				Stop: c.strands.RemoveStrand,
			})
			watcher.WatchBinary(selfexec.New().Check)
			runErr := watcher.Run(ctx, time.Sleep)

			if errors.Is(runErr, orchengine.ErrWatcherRunning) {
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"already_running": true}))
				return nil
			}
			exit := ""
			if final, loadErr := orchengine.LoadState(c.paths); loadErr == nil {
				exit = final.WatcherExit
			}
			if runErr != nil {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, runErr.Error(), map[string]any{"watcher_exit": exit}))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"already_running": false,
				"watcher_exit":    exit,
			}))
			return nil
		},
	}
}
