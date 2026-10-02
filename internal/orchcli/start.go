// start.go implements the `start` orch verb: the idempotent bootstrap that leaves one live orchestrator strand and one watcher bound to it, then hands the terminal over.
// Every fallible step reports on the envelope before the handover, which alone takes the CLI/Cobra Invariant's interactive-handoff exception.

package orchcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// orchStrandName is the role the orchestrator strand is added and addressed by in the prime's reed session.
const orchStrandName = agentname.RoleOrch

// Envelope "action" values.
const (
	actionAttachOnly     = "attach-only"
	actionSpawnedWatcher = "spawned-watcher"
	actionRelaunched     = "relaunched"
)

// sessionStarter starts the orchestrator's shuttle run and returns its strand guid.
type sessionStarter interface {
	StartSession(spec shuttleengine.Spec) (guid string, err error)
}

// runnerSessionStarter adapts *shuttleengine.Runner to sessionStarter.
type runnerSessionStarter struct {
	runner *shuttleengine.Runner
}

// StartSession delegates to Runner.Start, which returns only once the provider is past its startup gates.
func (s runnerSessionStarter) StartSession(spec shuttleengine.Spec) (string, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return "", err
	}
	return run.StrandGUID(), nil
}

// orchSpec builds the orchestrator run's spec: interactive, awaiting the operator, focused, with a never-written sentinel as its one output file so the run never finishes on one.
func (c *orchCLI) orchSpec(prompt string, now time.Time) shuttleengine.Spec {
	sentinel := filepath.Join(c.paths.Dir, "session-"+now.UTC().Format("20060102T150405Z")+".never")
	return shuttleengine.Spec{
		Prompt:        prompt,
		OutputFiles:   []string{sentinel},
		Model:         c.cfg.Model,
		Effort:        c.cfg.Effort,
		Interactive:   true,
		AwaitOperator: true,
		Role:          orchStrandName,
		NameOverride:  orchStrandName,
		Display:       render.Display{Focus: true},
	}
}

// orchStrands returns the strands agentname.Matches addresses as the orchestrator.
func orchStrands(strands []reedengine.StrandStatus) []reedengine.StrandStatus {
	var found []reedengine.StrandStatus
	for _, s := range strands {
		if agentname.Matches(s.Name, orchStrandName) {
			found = append(found, s)
		}
	}
	return found
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// startFields builds the success envelope.
func startFields(action, strand, promptSource string, attached bool, hint string) map[string]any {
	fields := map[string]any{
		"action":        action,
		"strand":        strand,
		"prompt_source": promptSource,
		"attached":      attached,
	}
	if hint != "" {
		fields["hint"] = hint
	}
	return fields
}

// startCmd builds the `start` subcommand.
func (c *orchCLI) startCmd() *cobra.Command {
	var noAttach bool
	var handoffFlag string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "launch or reattach the orchestrator session and its watcher",
		Long: `start is idempotent. With a live orchestrator strand and a live watcher it only
hands the terminal over. With a live strand and no watcher it spawns the watcher.
With a dead or absent strand it removes the corpse, launches a fresh session and
spawns a watcher for it. The fresh session resumes from --handoff when given, else
from the last completed handoff, else starts from the start stencil.
The terminal is attached to reed's session (or the tmux client switched onto it)
unless --no-attach is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			if handoffFlag != "" {
				abs, err := filepath.Abs(handoffFlag)
				if err != nil {
					return fail(fmt.Errorf("orch: resolve --handoff: %w", err))
				}
				handoffFlag = abs
			}

			if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
				return fail(fmt.Errorf("orch: create %s: %w", c.paths.Dir, err))
			}
			startLock, err := lock.AcquireWriteLock(c.paths.StartLockPath)
			if err != nil {
				return fail(err)
			}
			lockHeld := true
			releaseLock := func() {
				if lockHeld {
					lockHeld = false
					_ = startLock.Release()
				}
			}
			defer releaseLock()

			if err := c.reedUp(); err != nil {
				return fail(err)
			}
			strands, err := c.strands.Strands()
			if err != nil {
				return fail(err)
			}
			named := orchStrands(strands)
			if len(named) > 1 {
				guids := make([]string, len(named))
				for i, s := range named {
					guids[i] = s.GUID
				}
				return fail(fmt.Errorf("orch: %d strands are named %q (%s); remove the extras with `lyx reed remove` and re-run", len(named), orchStrandName, strings.Join(guids, ", ")))
			}
			var strand reedengine.StrandStatus
			hasStrand := len(named) == 1
			if hasStrand {
				strand = named[0]
			}
			strandLive := hasStrand && strand.Live

			watcherLive, err := orchengine.WatcherLive(c.paths)
			if err != nil {
				return fail(err)
			}
			st, err := orchengine.LoadState(c.paths)
			if err != nil {
				return fail(err)
			}

			action := orchengine.DecideStart(strandLive, watcherLive)
			if handoffFlag != "" && action != orchengine.StartRelaunch {
				return fail(fmt.Errorf("orch: --handoff applies to a fresh launch only, but the orchestrator strand %s is live -- run `lyx orch stop` first", strand.GUID))
			}

			envAction, promptSource, guid := "", "", strand.GUID
			switch action {
			case orchengine.StartRelaunch:
				if hasStrand {
					if err := c.strands.RemoveStrand(strand.GUID); err != nil {
						return fail(err)
					}
				}
				prompt, source, err := orchengine.ChooseStartPrompt(c.stencilsDir, handoffFlag, st, fileExists)
				if err != nil {
					return fail(err)
				}
				guid, err = c.starter.StartSession(c.orchSpec(prompt, time.Now()))
				if err != nil {
					return fail(err)
				}
				if err := orchengine.SaveState(c.paths, orchengine.ResetForFreshLaunch(st, guid)); err != nil {
					return fail(err)
				}
				logger.Info("orch: launched orchestrator session", "strandGUID", guid, "promptSource", source)
				if err := c.spawnWatcher(); err != nil {
					return fail(err)
				}
				envAction, promptSource = actionRelaunched, source
			case orchengine.StartSpawnWatcher:
				if err := c.adoptStrand(st, guid); err != nil {
					return fail(err)
				}
				if err := c.spawnWatcher(); err != nil {
					return fail(err)
				}
				envAction = actionSpawnedWatcher
			default:
				if err := c.adoptStrand(st, guid); err != nil {
					return fail(err)
				}
				envAction = actionAttachOnly
			}

			releaseLock()

			decision := handoverEnvelope
			if !noAttach {
				tmuxEnv := os.Getenv("TMUX")
				reedOwns := tmuxEnv != "" && c.reed.OwnsTmuxEnv(tmuxEnv)
				decision = decideHandover(false, tmuxEnv, reedOwns, c.currentTmuxSession(reedOwns), c.reed.SessionName())
			}
			switch decision {
			case handoverEnvelope:
				clihelp.SetExit(ctx, output.Ok(out, startFields(envAction, guid, promptSource, false, "")))
			case handoverHint:
				clihelp.SetExit(ctx, output.Ok(out, startFields(envAction, guid, promptSource, false, attachHint)))
			default:
				c.runHandover(ctx, decision)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noAttach, "no-attach", false, "for unattended callers: return once the session and watcher are up instead of handing the terminal to the session")
	cmd.Flags().StringVar(&handoffFlag, "handoff", "", "resume a fresh launch from this handoff file instead of the last completed one; refused while the strand is live")
	return cmd
}

// adoptStrand resets the persisted state onto guid when it belongs to another run than the recorded one.
func (c *orchCLI) adoptStrand(st orchengine.State, guid string) error {
	if st.Strand == guid {
		return nil
	}
	return orchengine.SaveState(c.paths, orchengine.ResetForFreshLaunch(st, guid))
}
