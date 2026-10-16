// start.go implements the `start` orch verb: the idempotent bootstrap that ensures the runs strand, leaves one live orchestrator strand and one watcher bound to it, then reports on the envelope.
// It never attaches or switches a tmux client;
// `lyx reed attach` is the only way into the session.

package orchcli

import (
	"errors"
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
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// orchStrandName is the role the orchestrator strand is added and addressed by in the prime's reed session.
const orchStrandName = agentname.RoleOrch

// orchSkills are the skills shuttle loads into the orch session before its prompt pointer on every launch path.
var orchSkills = []string{"scribe:prose", "scribe:conversation", "ly:board"}

// Envelope "action" values.
const (
	actionAlreadyRunning = "already-running"
	actionSpawnedWatcher = "spawned-watcher"
	actionRelaunched     = "relaunched"
)

// stoppingWatcherHint is the envelope hint when the watcher is still stopping after start's wait.
const stoppingWatcherHint = `the watcher is stopping; run "lyx orch start" again once it has exited`

// startedSession is what a started orchestrator run reports back.
type startedSession struct {
	// GUID is the run's strand guid.
	GUID string
	// SessionID is the provider session id the launch used: minted for a fresh launch, the resumed id for a resume; empty when the engine names none.
	SessionID string
	// Warning is the resume check's warning, empty when there is none.
	Warning string
}

// previousSessionHint is the envelope hint naming a recorded orch session that a fresh launch replaced and that `--adopt` can still resume.
func previousSessionHint(id string) string {
	return fmt.Sprintf(`the previous orch session %s is still resumable: run "lyx orch stop" then "lyx orch start --adopt %s"`, id, id)
}

// withContinueWaysForward appends the ways forward from a refused or failed `--continue` to err.
func withContinueWaysForward(err error) error {
	return fmt.Errorf("%w; run `lyx orch start` for a fresh launch, or `lyx orch start --adopt <id>` with the id `/status` shows in the session", err)
}

// sessionStarter starts the orchestrator's shuttle run and returns its strand guid, session id and the resume check's warning.
type sessionStarter interface {
	StartSession(spec shuttleengine.Spec) (startedSession, error)
}

// runnerSessionStarter adapts *shuttleengine.Runner to sessionStarter.
type runnerSessionStarter struct {
	runner *shuttleengine.Runner
}

// StartSession delegates to Runner.Start, which returns only once the provider is past its startup gates.
func (s runnerSessionStarter) StartSession(spec shuttleengine.Spec) (startedSession, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return startedSession{}, err
	}
	return startedSession{GUID: run.StrandGUID(), SessionID: run.SessionID(), Warning: run.ResumeWarning()}, nil
}

// orchSpec builds the orchestrator run's spec: interactive, awaiting the operator, focused, with a never-written sentinel as its one output file so the run never finishes on one.
// It carries the configured permission mode verbatim and allows the Agent tool with forks, so the hosted hub can spawn typed subagents and forks.
func (c *orchCLI) orchSpec(prompt string, now time.Time) shuttleengine.Spec {
	sentinel := filepath.Join(c.paths.Dir, "session-"+now.UTC().Format("20060102T150405Z")+".never")
	return shuttleengine.Spec{
		Prompt:         prompt,
		OutputFiles:    []string{sentinel},
		ModelChoice:    shuttleengine.ModelChoice{Model: c.cfg.Model, Effort: c.cfg.Effort},
		Interactive:    true,
		PermissionMode: c.cfg.PermissionMode,
		AllowAgentTool: true,
		ForkSubagents:  true,
		Role:           orchStrandName,
		NameOverride:   orchStrandName,
		Segment:        segmentcolor.Coordinator,
		ColorByCaller:  true,
		Display:        render.Display{Anchor: render.AnchorOwnWindow, Focus: true},

		Skills:           orchSkills,
		SkillLoadTimeout: c.cfg.HandoffTimeout(),

		ContextAfterCompaction: resumeContextCommand(c.location.AnchorPath()),
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
func startFields(action, strand, promptSource, hint, warning string) map[string]any {
	fields := map[string]any{
		"action":        action,
		"strand":        strand,
		"prompt_source": promptSource,
	}
	if hint != "" {
		fields["hint"] = hint
	}
	if warning != "" {
		fields["warning"] = warning
	}
	return fields
}

// startCmd builds the `start` subcommand.
func (c *orchCLI) startCmd() *cobra.Command {
	var noAttach bool
	var handoffFlag, adoptFlag string
	var continueFlag bool

	cmd := &cobra.Command{
		Use:         "start",
		Short:       "launch the orchestrator session and its watcher",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `start first ensures the runs strand, the overview of the batten runs in
flight, in the session's overview window; a failure there only warns. start is
idempotent. With a live orchestrator strand and a live watcher it changes
nothing and reports them. With a live strand and no watcher it spawns the watcher.
With a dead or absent strand it removes the corpse, launches a fresh session and
spawns a watcher for it. The fresh session resumes from --handoff when given, else
from the last completed handoff, else starts from the start stencil.
With --adopt <session-id> the fresh launch instead resumes that existing Claude
session, recorded under the prime's own directory, as the orchestrator strand;
--adopt and --handoff are exclusive, and --adopt is refused while the strand is live.
With --continue the fresh launch resumes the session orch recorded for its strand at
launch and after each /clear, the reboot recovery after "lyx orch stop" or a dead
strand; it is refused alongside --adopt or --handoff, while the strand is live, or
with no session recorded. A fresh launch that replaces a recorded session names it in
the envelope hint.
A watcher that has received SIGINT or SIGTERM and not yet exited is waited for, up to
three watcher poll intervals, so a kill followed by start spawns a fresh watcher; past
that bound start reports the watcher live with "watcher_stopping": true.
start never attaches or switches a tmux client; run "lyx reed attach" to watch the
session.`,
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

			if adoptFlag != "" && handoffFlag != "" {
				return fail(fmt.Errorf("orch: --adopt and --handoff are exclusive; drop one"))
			}
			if continueFlag && adoptFlag != "" {
				return fail(fmt.Errorf("orch: --continue and --adopt are exclusive; drop --adopt"))
			}
			if continueFlag && handoffFlag != "" {
				return fail(fmt.Errorf("orch: --continue and --handoff are exclusive; drop --handoff"))
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
			c.ensureRunsStrand()
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

			pollInterval := c.cfg.PollInterval()
			watcherLive, err := orchengine.WaitWatcherGone(c.paths, 3*pollInterval, pollInterval, c.sleep)
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

			if adoptFlag != "" && action != orchengine.StartRelaunch {
				return fail(fmt.Errorf("orch: --adopt applies to a fresh launch only, but the orchestrator strand %s is live -- run `lyx orch stop` first", strand.GUID))
			}

			if continueFlag {
				if action != orchengine.StartRelaunch {
					return fail(fmt.Errorf("orch: --continue applies to a fresh launch only, but the orchestrator strand %s is live -- run `lyx orch stop` first", strand.GUID))
				}
				if st.SessionID == "" {
					return fail(withContinueWaysForward(errors.New("orch: --continue found no recorded orch session id")))
				}
			}
			resumeSessionID := adoptFlag
			if continueFlag {
				resumeSessionID = st.SessionID
			}

			envAction, promptSource, guid, warning, hint := "", "", strand.GUID, "", ""
			switch action {
			case orchengine.StartRelaunch:
				if hasStrand {
					if err := c.strands.RemoveStrand(strand.GUID); err != nil {
						return fail(err)
					}
				}
				// start runs as a fresh process of the binary the pane names, so its own tree is the index source.
				if err := orchengine.RenderRoleFile(c.stencilsDir, c.paths.RolePath, clihelp.RenderIndex(cmd.Root(), clihelp.AudienceOperator), c.paths.RepoRulesPath); err != nil {
					return fail(err)
				}
				var prompt, source string
				if resumeSessionID != "" {
					prompt, err = orchengine.RenderAdoptPrompt(c.stencilsDir, c.paths.RolePath)
					source = orchengine.SourceAdopt
				} else {
					prompt, source, err = orchengine.ChooseStartPrompt(c.stencilsDir, c.paths.RolePath, handoffFlag, st, fileExists)
				}
				if err != nil {
					return fail(err)
				}
				spec := c.orchSpec(prompt, time.Now())
				spec.ResumeSessionID = resumeSessionID
				started, err := c.starter.StartSession(spec)
				if err != nil {
					if continueFlag {
						err = withContinueWaysForward(err)
					}
					return fail(err)
				}
				guid, warning = started.GUID, started.Warning
				next := orchengine.ResetForFreshLaunch(st, guid, time.Now())
				next.SessionID = started.SessionID
				if err := orchengine.SaveState(c.paths, next); err != nil {
					return fail(err)
				}
				if resumeSessionID != "" {
					logger.Info("orch: launched orchestrator session", "strandGUID", guid, "sessionID", started.SessionID, "adoptedSessionID", resumeSessionID, "promptSource", source)
				} else {
					logger.Info("orch: launched orchestrator session", "strandGUID", guid, "sessionID", started.SessionID, "promptSource", source)
				}
				if st.SessionID != "" && st.SessionID != started.SessionID {
					hint = previousSessionHint(st.SessionID)
				}
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
				envAction = actionAlreadyRunning
			}

			releaseLock()

			fields := startFields(envAction, guid, promptSource, hint, warning)
			if continueFlag {
				fields["session_id"] = resumeSessionID
			}
			if watcherLive && st.WatcherStopping {
				fields["watcher_stopping"] = true
				fields["hint"] = stoppingWatcherHint
				if hint != "" {
					fields["hint"] = stoppingWatcherHint + "; " + hint
				}
			}
			clihelp.SetExit(ctx, output.Ok(out, fields))
			return nil
		},
	}
	cmd.Flags().BoolVar(&noAttach, "no-attach", false, "accepted and ignored; kept for existing callers")
	_ = cmd.Flags().MarkHidden("no-attach")
	cmd.Flags().StringVar(&handoffFlag, "handoff", "", "resume a fresh launch from this handoff file instead of the last completed one; refused while the strand is live")
	cmd.Flags().StringVar(&adoptFlag, "adopt", "", "resume this existing Claude session (the id `/status` shows in it) as the orchestrator strand; exclusive with --handoff, and refused while the strand is live")
	cmd.Flags().BoolVar(&continueFlag, "continue", false, "resume the orch session recorded at the last launch or cycle as the orchestrator strand; exclusive with --adopt and --handoff, refused while the strand is live, and with no session recorded")
	return cmd
}

// adoptStrand resets the persisted state onto guid when it belongs to another run than the recorded one.
// The reset clears the recorded session id, since it named the other run's session; the watcher's first reading of guid's run fills it.
func (c *orchCLI) adoptStrand(st orchengine.State, guid string) error {
	if st.Strand == guid {
		return nil
	}
	next := orchengine.ResetForFreshLaunch(st, guid, time.Now())
	next.SessionID = ""
	return orchengine.SaveState(c.paths, next)
}
