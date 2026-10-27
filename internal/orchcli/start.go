// start.go implements the orch's bring-up, `BeamUp`: the idempotent bootstrap that starts the worktree's dobby, ensures the runs strand, leaves one live orchestrator strand and one watcher bound to it, then reports.
// It never attaches or switches a tmux client;
// `lym reed attach` is the only way into the session.

package orchcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// orchStrandName is the role the orchestrator strand is added and addressed by in the prime's reed session.
const orchStrandName = agentname.RoleOrch

// orchSkills are the skills shuttle loads into the orch session before its prompt pointer on every launch path.
var orchSkills = []string{"scribe:prose", "scribe:conversation", "ly:board"}

// Report "action" values.
const (
	actionAlreadyRunning = "already-running"
	actionSpawnedWatcher = "spawned-watcher"
	actionRelaunched     = "relaunched"
)

// stoppingWatcherHint is the report hint when the watcher is still stopping after the start's wait.
const stoppingWatcherHint = `the watcher is stopping; run "lym beam start" again once it has exited`

// BeamUpOptions tunes one BeamUp.
type BeamUpOptions struct {
	// Prepare runs the hub-scope origin reconcile, the failed-push read, the prime claim and the owner pauses before the beam steps.
	// A caller that already judged origin leaves it false.
	Prepare bool
	// Index renders the operator command index the orch's role file carries; nil runs `<os.Executable()> help index`.
	Index func() (string, error)
	// Here names this machine for the owner refusal; empty reads machinename.Name.
	Here string
}

// BeamUpReport is what one BeamUp did, and what it had done when it refused or failed.
type BeamUpReport struct {
	// Worktree is the name of the worktree brought up.
	Worktree string `json:"worktree"`
	// Dobby is the outcome of the worktree's dobby start, empty when none was attempted, and DobbyDetail says why it was refused or failed.
	Dobby       string `json:"dobby,omitempty"`
	DobbyDetail string `json:"dobby_detail,omitempty"`
	// Action is what the bring-up of `lym beam start` did to the strand: relaunched, spawned-watcher or already-running.
	Action string `json:"action,omitempty"`
	// Strand is the orchestrator strand's guid.
	Strand string `json:"strand,omitempty"`
	// PromptSource names where a launch took its prompt from; empty when no launch ran.
	PromptSource string `json:"prompt_source,omitempty"`
	// SessionID is the session a resume continued, empty for any other launch.
	SessionID string `json:"session_id,omitempty"`
	// AbandonedSession is the recorded session whose resume failed to start and which a fresh launch replaced.
	AbandonedSession string `json:"abandoned_session,omitempty"`
	// Hint names the way forward when the watcher was still stopping.
	Hint            string `json:"hint,omitempty"`
	WatcherStopping bool   `json:"watcher_stopping,omitempty"`
	// Reconcile, FailedPushes and OwnerPauses are what Prepare gathered.
	Reconcile    fabricengine.OriginReport `json:"reconcile"`
	FailedPushes []map[string]any          `json:"failed_pushes"`
	OwnerPauses  []map[string]string       `json:"owner_pauses"`
	// Mutations is what the reconcile changed, and Partial whether a failure left that change standing.
	Mutations []fabricengine.Mutation `json:"mutations"`
	Partial   bool                    `json:"partial"`
	// Warnings are the notes that did not stop the start.
	Warnings []string `json:"warnings,omitempty"`
}

// BeamUp brings up the worktree at location: its dobby, its runs strand and its orch with the watcher bound to it.
// A refusal or failure returns its error with the report gathered so far.
// It refuses a worktree that is neither the prime nor a beam.
func BeamUp(location *lymcwd.Location, opts BeamUpOptions) (BeamUpReport, error) {
	c := &orchCLI{}
	if err := wireCLI(c, location); err != nil {
		return BeamUpReport{}, err
	}
	return c.beamUp(opts)
}

// startedSession is what a started orchestrator run reports back.
type startedSession struct {
	// GUID is the run's strand guid.
	GUID string
	// SessionID is the provider session id the launch used: minted for a fresh launch, the resumed id for a resume; empty when the engine names none.
	SessionID string
	// Warning is the resume check's warning, empty when there is none.
	Warning string
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

// withdrawWayForward is the way forward a start appends to a first withdraw that failed.
// Creel's refusals for these causes carry no code, so one fixed text names them all.
const withdrawWayForward = "way forward: repair what the error names, such as a worktree whose reed state it names as unreadable or a daemon that would not start, whose log lines it quotes; " +
	"when creel refused the ORCHWATCH program claim from a non-orch strand's pane, run `lym beam start` from the orch's pane or from a terminal outside every strand pane; " +
	"when the error names a strand whose pane process is unknown, restart or remove that strand; then run `lym beam start` again"

// orchStrandFullName returns the full name creel addresses the orch strand by: the listed strand's name, else the fixed `<shortname>:orch`.
func orchStrandFullName(listed reedengine.StrandStatus, hasStrand bool, shortname string) string {
	if hasStrand {
		return listed.Name
	}
	return agentname.Name{Shortname: shortname, Role: orchStrandName}.String()
}

// orchReceiver returns the name of the orch strand creel is addressed by.
// A listed strand needs no shortname record; an unlisted one fails when the hub records none, since creel cannot serve such a hub either.
func (c *orchCLI) orchReceiver(listed reedengine.StrandStatus, hasStrand bool) (string, error) {
	if hasStrand {
		return orchStrandFullName(listed, true, ""), nil
	}
	shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(c.location.HubPath))
	if !found {
		return "", fmt.Errorf("orch: the hub of %s has no usable shortname record to name the orch strand; way forward: run `lym fabric shortname <shortname>`, then run `lym beam start` again", c.location.WorktreePath())
	}
	return orchStrandFullName(listed, false, shortname), nil
}

// withdrawFirst runs the round before a fresh launch: it withdraws the old cycle's queued steps on the orch strand's name and, when listed, releases the hold.
// A failed withdraw is the error, which makes the start launch nothing.
// A failed release is answered as text for State.FreshLaunchFailure and is no error.
// A withdrawn resume line clears the noticed mark of the orch session the state still records.
func (c *orchCLI) withdrawFirst(creel orchengine.Creel, name string, listed bool) (string, error) {
	withdrawn, err := creel.Withdraw(name)
	if err != nil {
		return "", fmt.Errorf("orch: creel could not withdraw the old cycle's steps for %s: %w; %s", name, err, withdrawWayForward)
	}
	c.clearResumeNoticed(withdrawn)
	if !listed {
		return "", nil
	}
	if err := creel.Release(name); err != nil {
		return fmt.Sprintf("release before the fresh launch: %v", err), nil
	}
	return "", nil
}

// withdrawSecond runs the round after a fresh launch's reset state is saved: a withdraw and a release on the orch strand's name, catching a step an old watcher queued in between.
// It answers the text of each failure, empty when both succeeded.
func (c *orchCLI) withdrawSecond(creel orchengine.Creel, name string) string {
	var failures []string
	withdrawn, err := creel.Withdraw(name)
	if err != nil {
		failures = append(failures, fmt.Sprintf("withdraw after the fresh launch: %v", err))
	} else {
		c.clearResumeNoticed(withdrawn)
	}
	if err := creel.Release(name); err != nil {
		failures = append(failures, fmt.Sprintf("release after the fresh launch: %v", err))
	}
	return strings.Join(failures, "; ")
}

// clearResumeNoticed hands a withdrawn resume line to orchengine.ClearResumeNoticed with the orch GUID the persisted state records.
// A failure is logged and no error: the worst outcome is a resume line that is not posted again.
func (c *orchCLI) clearResumeNoticed(withdrawn []orchengine.CreelWithdrawn) {
	if len(withdrawn) == 0 {
		return
	}
	st, err := orchengine.LoadState(c.paths)
	if err == nil {
		var geometry pauserecord.Geometry
		if geometry, err = hubgeom.PauseGeometry(c.location); err == nil {
			err = orchengine.ClearResumeNoticed(geometry.RecordPath, st.Strand, withdrawn, time.Now())
		}
	}
	if err != nil {
		logger.Warn("orch: clearing the resume-noticed mark after a withdraw failed", "cause", err)
	}
}

// joinFailures joins the non-empty failure texts.
func joinFailures(texts ...string) string {
	var kept []string
	for _, text := range texts {
		if text != "" {
			kept = append(kept, text)
		}
	}
	return strings.Join(kept, "; ")
}

// saveFreshLaunch saves next, the state reset onto a fresh launch, with the first round's failure, runs the second round and saves its failure on top.
func (c *orchCLI) saveFreshLaunch(next orchengine.State, firstFailure string, creel orchengine.Creel, name string) error {
	next.FreshLaunchFailure = firstFailure
	if err := orchengine.SaveState(c.paths, next); err != nil {
		return err
	}
	secondFailure := c.withdrawSecond(creel, name)
	if secondFailure == "" {
		return nil
	}
	next.FreshLaunchFailure = joinFailures(firstFailure, secondFailure)
	return orchengine.SaveState(c.paths, next)
}

// reconcileHub runs the hub-scope origin reconcile from the prime of the hub that location belongs to, whichever bring-up of `lym beam start` runs it, with the teardown, live-pair and parent-lock seams pairteardown builds and zero options.
func reconcileHub(location *lymcwd.Location) (fabricengine.OriginReport, error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	return fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{}, teardown.OriginSeams(pairteardown.HubSeamOptions(prime)), fabricengine.ReconcileOptions{})
}

// ensureDobbies starts the dobby of the worktree at location, the prime's or a beam's, when it is not live already, and returns that worktree's outcome.
// A dobby another machine owns, a standing anchor change and a held landing are refused by the engine and reported in the outcome.
func ensureDobbies(location *lymcwd.Location) (dobbyengine.StartEntry, error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return dobbyengine.StartEntry{}, err
	}
	report, err := dobbyengine.EnsureLive(hubgeom.DobbyStart(prime), []string{location.WorktreeName})
	if len(report.Entries) == 0 {
		return dobbyengine.StartEntry{}, err
	}
	return report.Entries[len(report.Entries)-1], err
}

// claimPrimeNotes claims the hub's prime for this machine and returns the warning lines that report a claim, a claim another machine won or a failure.
// It returns nothing when the board already records a prime owner or no claim is wired.
func (c *orchCLI) claimPrimeNotes() []string {
	if c.claimPrime == nil {
		return nil
	}
	claimed, lostTo, err := c.claimPrime()
	switch {
	case err != nil:
		return []string{fmt.Sprintf("the prime could not be claimed for this machine: %v", err)}
	case lostTo != "":
		return []string{fmt.Sprintf("the prime is owned by machine %s, which claimed it first", lostTo)}
	case claimed != "":
		return []string{fmt.Sprintf("the prime was claimed for machine %s, which had no prime owner recorded", claimed)}
	}
	return nil
}

// pauseOwnedRuns pauses the pair of each owned_elsewhere entry of report and returns the paused slugs with their owners, and a warning line per entry that could not be paused.
// An unwired pause names every such entry in a warning, since nothing holds it.
func (c *orchCLI) pauseOwnedRuns(report fabricengine.OriginReport) ([]map[string]string, []string) {
	paused := []map[string]string{}
	var warnings []string
	for _, entry := range report.Entries {
		if entry.State != fabricengine.EntryOwnedElsewhere {
			continue
		}
		if c.pauseOwned == nil {
			warnings = append(warnings, fmt.Sprintf("%s is owned by machine %s and was not paused; re-run `lym beam start`, which pauses it again", entry.Slug, entry.Owner))
			continue
		}
		if err := c.pauseOwned(c.location, entry.Slug); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s is owned by machine %s and could not be paused (%v); re-run `lym beam start`, which pauses it again", entry.Slug, entry.Owner, err))
			continue
		}
		paused = append(paused, map[string]string{"slug": entry.Slug, "owner": entry.Owner})
	}
	return paused, warnings
}

// reconcileNote renders the line a fresh launch's prompt gains: the report's summary and the outstanding failed pushes with their branches.
// It is empty when the reconcile moved, stopped and ended nothing and no push is outstanding.
func reconcileNote(report fabricengine.OriginReport, failedPushes []map[string]any) string {
	var parts []string
	if summary := report.Summary(); summary != "" {
		parts = append(parts, "origin reconcile at start: "+summary)
	}
	if len(failedPushes) > 0 {
		branches := make([]string, 0, len(failedPushes))
		for _, row := range failedPushes {
			branches = append(branches, fmt.Sprintf("%v", row["branch"]))
		}
		parts = append(parts, fmt.Sprintf("%d failed push(es) outstanding: %s", len(failedPushes), strings.Join(branches, ", ")))
	}
	return strings.Join(parts, "\n")
}

// prepare runs the steps Prepare asks for and records them on report: the origin reconcile, the failed-push read, the prime claim and the owner pauses.
// The reconcile never refuses the start: the orch is the session that resolves what it reports.
// It returns the note a fresh launch's prompt gains.
func (c *orchCLI) prepare(report *BeamUpReport) string {
	reconciled, reconcileErr := c.reconcile()
	report.Reconcile = reconciled
	report.Mutations = reconciled.Mutations.Entries()
	if report.Mutations == nil {
		report.Mutations = []fabricengine.Mutation{}
	}
	if reconcileErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("origin reconcile failed: %v", reconcileErr))
	}
	if reconciled.Unreachable {
		report.Warnings = append(report.Warnings, "origin was unreachable, so the hub's branches were neither judged against it nor moved")
	}
	failedPushes, failedPushesErr := fabricengine.FailedPushFields(c.location.HubPath)
	if failedPushesErr != nil {
		failedPushes = []map[string]any{}
		report.Warnings = append(report.Warnings, fmt.Sprintf("failed-push records could not be read: %v", failedPushesErr))
	}
	report.FailedPushes = failedPushes

	report.Warnings = append(report.Warnings, c.claimPrimeNotes()...)
	paused, pauseWarnings := c.pauseOwnedRuns(reconciled)
	report.OwnerPauses = paused
	report.Warnings = append(report.Warnings, pauseWarnings...)
	return reconcileNote(reconciled, failedPushes)
}

// beamUp is BeamUp over a wired receiver.
// It holds start.lock throughout, so a stop or a status restart never races it.
// A failure after the reconcile reports whatever the reconcile changed as partial.
func (c *orchCLI) beamUp(opts BeamUpOptions) (report BeamUpReport, err error) {
	report = BeamUpReport{
		Worktree:     c.location.WorktreeName,
		FailedPushes: []map[string]any{},
		OwnerPauses:  []map[string]string{},
		Mutations:    []fabricengine.Mutation{},
	}
	defer func() { report.Partial = err != nil && len(report.Mutations) > 0 }()
	if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
		return report, fmt.Errorf("orch: create %s: %w", c.paths.Dir, err)
	}
	startLock, err := lock.AcquireWriteLock(c.paths.StartLockPath)
	if err != nil {
		return report, err
	}
	defer func() { _ = startLock.Release() }()

	note := ""
	if opts.Prepare {
		note = c.prepare(&report)
	}

	if c.readBeamFacts != nil {
		facts, err := c.readBeamFacts()
		if err != nil {
			return report, err
		}
		here := facts.here
		if opts.Here != "" {
			here = opts.Here
		}
		if err := beamUpRefusal(facts.slug, here, facts.owner, facts.moveWayForward, facts.held, facts.status); err != nil {
			return report, err
		}
	}

	c.startDobbyStep(&report)

	if err := c.reedUp(); err != nil {
		return report, err
	}
	c.ensureRunsStrand()

	index := opts.Index
	if index == nil {
		index = func() (string, error) {
			executable, err := os.Executable()
			if err != nil {
				return "", fmt.Errorf("orch: locate the lym binary to render the command index: %w", err)
			}
			return c.indexRunner(executable)
		}
	}
	orchReport, err := c.startOrch(index, note)
	report.Action, report.Strand, report.PromptSource = orchReport.Action, orchReport.Strand, orchReport.PromptSource
	report.SessionID, report.AbandonedSession = orchReport.SessionID, orchReport.AbandonedSession
	report.Hint, report.WatcherStopping = orchReport.Hint, orchReport.WatcherStopping
	report.Warnings = append(report.Warnings, orchReport.Warnings...)
	return report, err
}

// startDobbyStep starts this worktree's dobby and records its outcome on report.
// A dobby that did not start only warns, naming `lym beam start` as the way to start it again.
func (c *orchCLI) startDobbyStep(report *BeamUpReport) {
	if c.startDobby == nil {
		return
	}
	entry, err := c.startDobby()
	report.Dobby, report.DobbyDetail = entry.Outcome, entry.Detail
	cause := entry.Detail
	switch {
	case err != nil:
		cause = err.Error()
	case entry.Outcome != dobbyengine.OutcomeRefused && entry.Outcome != dobbyengine.OutcomeFailed:
		return
	}
	report.Warnings = append(report.Warnings, fmt.Sprintf("the dobby was not started (%s); way forward: fix the cause it names, then run `lym beam start`", cause))
}

// startOrch leaves one live orchestrator strand and one watcher bound to it, and reports the orch's part of the bring-up.
// A live strand with a live watcher is left alone, a live strand with a dead watcher gets a watcher, and a missing or dead strand is relaunched under a withdraw round on each side.
// note joins the prompt of a fresh launch.
func (c *orchCLI) startOrch(index func() (string, error), note string) (BeamUpReport, error) {
	var report BeamUpReport
	strands, err := c.strands.Strands()
	if err != nil {
		return report, err
	}
	named := orchStrands(strands)
	if len(named) > 1 {
		guids := make([]string, len(named))
		for i, s := range named {
			guids[i] = s.GUID
		}
		return report, fmt.Errorf("orch: %d strands are named %q (%s); remove the extras with `lym reed remove` and re-run", len(named), orchStrandName, strings.Join(guids, ", "))
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
		return report, err
	}
	st, err := orchengine.LoadState(c.paths)
	if err != nil {
		return report, err
	}

	report.Strand = strand.GUID
	switch orchengine.DecideStart(strandLive, watcherLive) {
	case orchengine.StartRelaunch:
		receiver, err := c.orchReceiver(strand, hasStrand)
		if err != nil {
			return report, err
		}
		creel, err := c.newCreel()
		if err != nil {
			return report, fmt.Errorf("orch: reach creel: %w; %s", err, withdrawWayForward)
		}
		firstFailure, err := c.withdrawFirst(creel, receiver, hasStrand)
		if err != nil {
			return report, err
		}
		if hasStrand {
			if err := c.strands.RemoveStrand(strand.GUID); err != nil {
				return report, err
			}
		}
		operatorIndex, err := index()
		if err != nil {
			return report, err
		}
		if err := orchengine.RenderRoleFile(c.stencilsDir, c.paths.RolePath, operatorIndex, c.paths.HubRulesPath, c.paths.OwnRulesPath); err != nil {
			return report, err
		}
		started, source, abandoned, err := c.launchOrch(st, note)
		report.AbandonedSession = abandoned
		if err != nil {
			return report, err
		}
		report.Strand = started.GUID
		if started.Warning != "" {
			report.Warnings = append(report.Warnings, started.Warning)
		}
		next := orchengine.ResetForFreshLaunch(st, started.GUID, time.Now())
		next.SessionID = started.SessionID
		if err := c.saveFreshLaunch(next, firstFailure, creel, receiver); err != nil {
			return report, err
		}
		logger.Info("orch: launched orchestrator session", "strandGUID", started.GUID, "sessionID", started.SessionID, "promptSource", source, "abandonedSessionID", abandoned)
		if err := c.spawnWatcher(); err != nil {
			return report, err
		}
		report.Action, report.PromptSource = actionRelaunched, source
		if source == orchengine.SourceResume {
			report.SessionID = started.SessionID
		}
	case orchengine.StartSpawnWatcher:
		if err := c.adoptListedStrand(st, strand); err != nil {
			return report, err
		}
		logger.Info("orch: restarted a dead watcher", "strandGUID", strand.GUID, "previousExit", st.WatcherExit)
		if err := c.spawnWatcher(); err != nil {
			return report, err
		}
		report.Action = actionSpawnedWatcher
	default:
		if err := c.adoptListedStrand(st, strand); err != nil {
			return report, err
		}
		report.Action = actionAlreadyRunning
	}
	if watcherLive && st.WatcherStopping {
		report.WatcherStopping = true
		report.Hint = stoppingWatcherHint
	}
	return report, nil
}

// launchOrch starts the orch session: the session st records, resumed with the reload pointer, else a fresh launch.
// A resume that fails to start, its transcript gone or its process not coming up live, falls back to a fresh launch and names the session it gave up as abandoned.
// A fresh launch takes the last completed handoff, else the start stencil, and note joins its prompt.
func (c *orchCLI) launchOrch(st orchengine.State, note string) (started startedSession, source, abandoned string, err error) {
	if st.SessionID != "" {
		reload, err := orchengine.RenderReloadPrompt(c.stencilsDir, c.paths.RolePath)
		if err != nil {
			return startedSession{}, "", "", err
		}
		spec := c.orchSpec(reload, time.Now())
		spec.ResumeSessionID = st.SessionID
		started, err := c.starter.StartSession(spec)
		if err == nil {
			return started, orchengine.SourceResume, "", nil
		}
		logger.Warn("orch: the recorded session could not be resumed; launching fresh", "sessionID", st.SessionID, "cause", err)
		abandoned = st.SessionID
	}
	prompt, source, err := orchengine.ChooseStartPrompt(c.stencilsDir, c.paths.RolePath, st, fileExists)
	if err != nil {
		return startedSession{}, "", abandoned, err
	}
	if note != "" {
		prompt += "\n\n" + note
	}
	started, err = c.starter.StartSession(c.orchSpec(prompt, time.Now()))
	return started, source, abandoned, err
}

// adoptListedStrand binds the persisted state to the live listed orch strand, reaching creel only when the state names another run.
func (c *orchCLI) adoptListedStrand(st orchengine.State, strand reedengine.StrandStatus) error {
	if st.Strand == strand.GUID {
		return nil
	}
	creel, err := c.newCreel()
	if err != nil {
		return fmt.Errorf("orch: reach creel: %w; %s", err, withdrawWayForward)
	}
	return c.adoptStrand(st, strand.GUID, creel, strand.Name)
}

// adoptStrand resets the persisted state onto guid when it belongs to another run than the recorded one, with the two withdraw rounds around the reset on the orch strand's name.
// The reset clears the recorded session id, since it named the other run's session; the watcher's first reading of guid's run fills it.
func (c *orchCLI) adoptStrand(st orchengine.State, guid string, creel orchengine.Creel, name string) error {
	if st.Strand == guid {
		return nil
	}
	firstFailure, err := c.withdrawFirst(creel, name, true)
	if err != nil {
		return err
	}
	next := orchengine.ResetForFreshLaunch(st, guid, time.Now())
	next.SessionID = ""
	return c.saveFreshLaunch(next, firstFailure, creel, name)
}
