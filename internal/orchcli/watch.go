// watch.go implements the hidden `watch` orch verb:
// the detached daemon that `start` launches.
// It polls the orchestrator session and runs the handoff cycle until the strand is gone.

package orchcli

import (
	"errors"
	"fmt"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/spf13/cobra"
)

// realClock is the wall clock.
type realClock struct{}

// Now returns the current time.
func (realClock) Now() time.Time { return time.Now() }

// orchAgentName returns the agent name of the orch this receiver's worktree hosts:
// `<shortname>:orch` in the prime and `<shortname>:<slug>:orch` in a beam.
// A hub with no recorded shortname yields the empty name, which the watcher reads as the prime's orch.
func (c *orchCLI) orchAgentName() (string, error) {
	shortname, found := fabricengine.ReadShortname(fabriclayout.BoltDir(c.location.HubPath))
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

// beamOwnerElsewhere reads the owner of the worktree location on the hub's local board:
// the prime's owner for the prime, and for a beam its effective owner, resolved through `hubgeom.ParentOf`.
// elsewhere is true when that owner names a machine other than this one;
// a worktree with no owner recorded is owned by no other machine.
func beamOwnerElsewhere(location *lymcwd.Location) (owner string, elsewhere bool, err error) {
	here, err := machinename.ReadIdentity()
	if err != nil {
		return "", false, fmt.Errorf("orch: read this machine's name: %w", err)
	}
	owners, err := boardengine.ReadOwners(fabriclayout.BoardDir(location.HubPath))
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
	return owner, boardengine.OwnedElsewhere(owner, owners.Registry, here), nil
}

// UnderOwnHold returns the accessor of the hub of prime.
// The accessor reports whether a Bolt hold of this machine covers a worktree.
// For the worktree named slug, empty for the prime, it reads the hub's local Bolt and this machine's identity.
// The worktree's subject is `root` for the prime, else the beam subject `Owners.AdoptSubject` names.
// It moves up to the subject above while the beam records no owner of its own and the hold does not list it.
// It reports true only when a hold whose machine reads as this machine lists that subject.
// A beam that keeps its own owner outside the hold's subjects is not covered.
// A Bolt or identity that does not read counts as no hold, logged once per episode.
// The accessor is not safe for concurrent use.
func UnderOwnHold(prime *lymcwd.Location) func(slug string) bool {
	var failures logger.Episode
	return func(slug string) bool {
		covered, err := ownHoldCovers(prime, slug)
		if err != nil {
			failures.Fail("orch: the Bolt hold or the identity it is judged against does not read; they count as no hold", "hub", prime.HubPath, "error", err)
			return false
		}
		failures.Recover("orch: the Bolt hold and the identity read again", "hub", prime.HubPath)
		return covered
	}
}

// ownHoldCovers reads the local Bolt and this machine's identity.
// It judges whether a hold of this machine lists the subject of slug.
func ownHoldCovers(prime *lymcwd.Location, slug string) (bool, error) {
	here, err := machinename.ReadIdentity()
	if err != nil {
		return false, fmt.Errorf("read this machine's name: %w", err)
	}
	owners, err := boardengine.ReadOwners(fabriclayout.BoardDir(prime.HubPath))
	if err != nil {
		return false, fmt.Errorf("read the board's owners: %w", err)
	}
	var ownHolds []boardengine.Hold
	for _, hold := range owners.Holds {
		if owners.Registry.ReadsAsHere(hold.Machine, here) {
			ownHolds = append(ownHolds, hold)
		}
	}
	if len(ownHolds) == 0 {
		return false, nil
	}
	listed := func(subject string) bool {
		return slices.ContainsFunc(ownHolds, func(hold boardengine.Hold) bool {
			return slices.ContainsFunc(hold.Subjects, func(held boardengine.HoldSubject) bool { return held.Subject == subject })
		})
	}
	parentOf := func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) }
	subject := boardengine.RootSubject
	if slug != "" {
		if subject, err = owners.AdoptSubject(slug, parentOf); err != nil {
			return false, fmt.Errorf("resolve the adopt subject of %q: %w", slug, err)
		}
	}
	for subject != boardengine.RootSubject && !listed(subject) && owners.Entries[subject].Owner == "" {
		parent, err := parentOf(subject)
		if err != nil {
			return false, fmt.Errorf("resolve the parent of %q: %w", subject, err)
		}
		if parent == "" {
			subject = boardengine.RootSubject
			break
		}
		if subject, err = owners.AdoptSubject(parent, parentOf); err != nil {
			return false, fmt.Errorf("resolve the adopt subject of %q: %w", parent, err)
		}
	}
	return listed(subject), nil
}

// underAdoptWatch returns the Bolt-hold accessor of the receiver's hub and the slug it asks about:
// the beam's slug, empty for the prime.
func (c *orchCLI) underAdoptWatch() (func(slug string) bool, string, error) {
	prime, err := resolvePrime(c.location)
	if err != nil {
		return nil, "", err
	}
	slug := c.location.WorktreeName
	if slug == prime.WorktreeName {
		slug = ""
	}
	return UnderOwnHold(prime), slug, nil
}

// reapInheritedChildren waits on the children a watcher re-exec'd onto a new binary keeps from its old image.
// Nothing else waits on them.
func reapInheritedChildren() {
	if pids, err := proc.ReapInheritedChildren(); err != nil {
		logger.Warn("orch: the watcher's inherited children are not reaped", "error", err)
	} else if len(pids) > 0 {
		logger.Info("orch: the watcher reaps the children it inherited", "pids", pids)
	}
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
			c.reapInherited()

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
			watcher := orchengine.NewWatcher(runnerSession{runner: c.runner, strands: c.strands}, creel, c.cfg, c.paths, c.stencilsDir, c.orchSkills, realClock{})
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
			// The accessor resolves the hub's prime only when a foreign owner first makes the watcher ask.
			// Once built it is kept, so its read failures log once per episode.
			var underHold func(slug string) bool
			var holdSlug string
			var buildFailures logger.Episode
			watcher.WatchOwner(orchengine.OwnerWatch{
				Read: func() (string, bool, error) { return beamOwnerElsewhere(c.location) },
				End:  c.endSubject,
				Stop: c.strands.RemoveStrand,
				UnderAdopt: func() bool {
					if underHold == nil {
						built, slug, err := c.underAdoptWatch()
						if err != nil {
							buildFailures.Fail("orch: the Bolt-hold accessor could not be built; no hold covers the beam", "error", err)
							return false
						}
						buildFailures.Recover("orch: the Bolt-hold accessor is built")
						underHold, holdSlug = built, slug
					}
					return underHold(holdSlug)
				},
			})
			relaunch := func(fresh bool) func(orchengine.State) (orchengine.State, error) {
				return func(st orchengine.State) (orchengine.State, error) {
					if _, err := c.relaunchOrch(st, fresh); err != nil {
						return st, err
					}
					return orchengine.LoadState(c.paths)
				}
			}
			watcher.WatchRestart(orchengine.OrchRestart{Resume: relaunch(false), Fresh: relaunch(true)})
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
