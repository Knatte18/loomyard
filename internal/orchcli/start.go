// start.go implements the orch's bring-up, `BeamUp`, which is idempotent.
// It starts the worktree's dobby and runs strand, and leaves a live orchestrator strand with one watcher bound to it.
// It never attaches or switches a tmux client;
// `lym reed attach` is the only way into the session.

package orchcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
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
	"github.com/Knatte18/loomyard/internal/slackengine"
)

// orchStrandName is the role the orchestrator strand is added and addressed by in the prime's reed session.
const orchStrandName = agentname.RoleOrch

// dobbyStartProgram is the program name a notice about a dobby start signs with.
const dobbyStartProgram = "DOBBY"

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
	// Prepare runs, before the beam steps, the hub-scope origin reconcile, the failed-push read, and the prime claim.
	// It also ends the processes of each run another machine owns; a caller that already judged origin leaves it false.
	Prepare bool
	// NoPrimeClaim leaves the prime claim out of Prepare, for a caller that must claim no prime.
	NoPrimeClaim bool
	// Here names this machine for the owner refusal; empty reads machinename.Name.
	Here string
	// SkipOrch brings the beam up without its orch:
	// every step runs but the orch's start, and the report's orch fields stay empty.
	// Only `lym adopt` sets it, for a request carrying `--no-orch`; `lym beam start` starts the orch later.
	SkipOrch bool
	// ArchiveOwned runs before the refusal of a worktree another machine owns, and its error joins that refusal;
	// a nil ArchiveOwned runs nothing.
	// `lym beam start` sets it to archive this machine's unpushed tips of the worktree for the machine that owns it.
	ArchiveOwned func() error
}

// BeamUpReport is what one BeamUp did, and what it had done when it refused or failed.
type BeamUpReport struct {
	// Worktree is the name of the worktree brought up.
	Worktree string `json:"worktree"`
	// Dobby is the outcome of the dobby start, empty when none was attempted; DobbyDetail says why it was refused or failed.
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
	// AbandonedSession is the recorded session this machine holds no transcript of, which a fresh launch replaced.
	AbandonedSession string `json:"abandoned_session,omitempty"`
	// Hint names the way forward when the watcher was still stopping.
	Hint            string `json:"hint,omitempty"`
	WatcherStopping bool   `json:"watcher_stopping,omitempty"`
	// Reconcile, FailedPushes and OwnerPauses are what Prepare gathered.
	// OwnerPauses names each owned run whose processes it ended.
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

// ErrBeamUpUnderTest reports that a test binary refused to bring a worktree up.
// Bringing one up would start a real dobby, tmux session and agent.
var ErrBeamUpUnderTest = errors.New("orch: no worktree brought up under test")

// BeamUpWorktree brings up the worktree named worktree in the hub of prime.
// When worktree is the prime's name, the prime itself is brought up.
// It brings up the worktree's dobby, its runs strand and its orch with the watcher bound to it, without a reconcile.
// It is the one way a verb that needs a live dobby starts it, so a dobby never comes up without its orch.
// A refusal or failure of the bring-up is returned.
// So is a dobby that was refused or failed to start, carrying its detail.
// It returns ErrBeamUpUnderTest, starting nothing, in a test binary.
func BeamUpWorktree(prime *lymcwd.Location, worktree string) error {
	if testing.Testing() {
		return ErrBeamUpUnderTest
	}
	location, err := bringUpLocation(prime, worktree, lymcwd.ResolveWorktree)
	if err != nil {
		return err
	}
	report, err := BeamUp(location, BeamUpOptions{})
	if err != nil {
		return err
	}
	return dobbyStartError(worktree, report)
}

// bringUpLocation returns the location to bring up for the worktree named worktree.
// It is prime when worktree is the prime's name;
// otherwise it is the location resolve gives for the worktree's directory in the prime's hub.
func bringUpLocation(prime *lymcwd.Location, worktree string, resolve func(worktreeRoot string) (*lymcwd.Location, error)) (*lymcwd.Location, error) {
	if worktree == prime.WorktreeName {
		return prime, nil
	}
	resolved, err := resolve(fabricengine.WorktreePath(prime, worktree))
	if err != nil {
		return nil, fmt.Errorf("orch: resolve the worktree of %s: %w", worktree, err)
	}
	return resolved, nil
}

// dobbyStartError returns nil when the dobby was started or found live.
// Otherwise it returns an error naming worktree and the detail of report's dobby outcome.
// A start that never reported an outcome carries the report's warnings as its detail.
func dobbyStartError(worktree string, report BeamUpReport) error {
	switch report.Dobby {
	case dobbyengine.OutcomeStarted, dobbyengine.OutcomeLive:
		return nil
	}
	detail := report.DobbyDetail
	if detail == "" {
		detail = strings.Join(report.Warnings, "; ")
	}
	return fmt.Errorf("orch: the dobby of %s was not started: %s", worktree, detail)
}

// startedSession is what a started orchestrator run reports back.
type startedSession struct {
	// GUID is the run's strand guid.
	GUID string
	// SessionID is the session the launch used: minted when fresh, else the resumed one; empty when the engine names none.
	SessionID string
	// Warning is the resume check's warning, empty when there is none.
	Warning string
	// HolderPID is the live provider process holding the started session, zero when none was found.
	HolderPID int
	// HolderErr is why the holder could not be read, nil when the read succeeded or found none.
	HolderErr error
	// Carried counts the operator instructions the launch carried over from an unfinished predecessor.
	Carried int
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
// It then reads the live provider process holding the started session.
func (s runnerSessionStarter) StartSession(spec shuttleengine.Spec) (startedSession, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return startedSession{}, err
	}
	started := startedSession{GUID: run.StrandGUID(), SessionID: run.SessionID(), Warning: run.ResumeWarning(), Carried: len(run.CarriedInstructions())}
	if pid, found, err := run.SessionHolder(); found {
		started.HolderPID = pid
	} else {
		started.HolderErr = err
	}
	return started, nil
}

// orchSpec builds the orchestrator run's spec as an interactive, focused run that awaits the operator.
// It names the Claude engine and its prompt engine explicitly, so a Codex default engine never takes the orch role.
// Its one output file is a sentinel that is never written, so the run never finishes on its own.
// It carries the configured permission mode verbatim and authorizes forks,
// so the hosted hub can spawn forks beside typed subagents.
// Its skills come from the orch role in agents.yaml, which shuttle loads before the prompt pointer on every launch path.
// Its turn hook is the Slack hook.
// It switches the presence back on the operator's typed line.
// While away, it forwards each turn's final reply to the orch's channel.
// A non-empty remoteControlName starts the session with remote control on under that name.
func (c *orchCLI) orchSpec(prompt string, now time.Time, remoteControlName string) shuttleengine.Spec {
	sentinel := filepath.Join(c.paths.Dir, "session-"+now.UTC().Format("20060102T150405Z")+".never")
	return shuttleengine.Spec{
		Prompt:         prompt,
		OutputFiles:    []string{sentinel},
		ModelChoice:    c.orchModelChoice(),
		PromptEngine:   orchEngine,
		Interactive:    true,
		PermissionMode: c.cfg.PermissionMode,
		ForkSubagents:  true,
		Role:           orchStrandName,
		NameOverride:   orchStrandName,
		Segment:        segmentcolor.Coordinator,
		ColorByCaller:  true,
		Display:        render.Display{Anchor: render.AnchorOwnWindow, Focus: true},

		ConfigRole:       agentconfig.RoleOrch,
		SkillLoadTimeout: c.cfg.HandoffTimeout(),

		ContextAfterCompaction: resumeContextCommand(c.location.AnchorPath()),
		TurnHookCommand:        slackengine.HookCommand,
		RemoteControlName:      remoteControlName,
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

// orchStrandFullName returns the full name creel addresses the orch strand by.
// It is the listed strand's name, or else `<shortname>:orch` built from shortname.
func orchStrandFullName(listed reedengine.StrandStatus, hasStrand bool, shortname string) string {
	if hasStrand {
		return listed.Name
	}
	return agentname.Name{Shortname: shortname, Role: orchStrandName}.String()
}

// orchReceiver returns the name of the orch strand creel is addressed by.
// A listed strand needs no shortname record.
// An unlisted one fails when the hub records none, because creel cannot serve such a hub either.
func (c *orchCLI) orchReceiver(listed reedengine.StrandStatus, hasStrand bool) (string, error) {
	if hasStrand {
		return orchStrandFullName(listed, true, ""), nil
	}
	shortname, found := fabricengine.ReadShortname(fabricengine.BoltDir(c.location.HubPath))
	if !found {
		return "", fmt.Errorf("orch: the hub of %s has no usable shortname record to name the orch strand; way forward: run `lym fabric shortname <shortname>`, then run `lym beam start` again", c.location.WorktreePath())
	}
	return orchStrandFullName(listed, false, shortname), nil
}

// withdrawFirst runs the round before a fresh launch.
// It withdraws the old cycle's queued steps on the orch strand's name and, when listed, releases the hold.
// A failed withdraw is returned as the error, so the start launches nothing.
// A failed release is returned as text for State.FreshLaunchFailure, not as an error.
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

// withdrawSecond runs the round after a fresh launch's reset state is saved.
// It withdraws and releases the orch strand's name, catching a step an old watcher queued in between.
// It returns the failures as one text, empty when both steps succeeded.
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

// clearResumeNoticed clears the noticed mark of withdrawn resume lines, for the persisted state's strand.
// A failure is logged, not returned: the worst outcome is a resume line that is not posted again.
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

// saveFreshLaunch saves next, the state reset onto a fresh launch, with the first round's failure.
// It then runs the second round and saves its failure on top.
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

// reconcileHub runs the hub-scope origin reconcile from the prime of the hub that location belongs to.
// Any bring-up of `lym beam start` runs it.
// It passes the seams pairteardown builds for the hub, and zero ReconcileScope and ReconcileOptions.
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

// ensureDobbies starts the dobby of the worktree at location when it is not live already.
// location may be the prime's worktree or a beam's.
// It returns that worktree's outcome.
// The engine refuses a dobby another machine owns, a standing anchor change and a held landing.
// It reports each refusal in the outcome.
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

// claimPrimeNotes claims the hub's prime for this machine.
// It returns the warning lines that report the outcome: a claim, a loss to another machine, or a failure.
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

// pauseOwnedRuns ends the processes of each owned_elsewhere entry of report, its dobby too when the entry is a beam.
// It returns the slugs it ended with their owners, and a warning line per entry whose processes could not be ended.
// An unwired end names every such entry in a warning, since nothing ended it.
// A failed read of whether an entry is a beam ends nothing for that entry,
// so a beam's dobby is never left running by a guess.
func (c *orchCLI) pauseOwnedRuns(report fabricengine.OriginReport) ([]map[string]string, []string) {
	ended := []map[string]string{}
	var warnings []string
	for _, entry := range report.Entries {
		if entry.State != fabricengine.EntryOwnedElsewhere {
			continue
		}
		if c.endOwned == nil {
			warnings = append(warnings, fmt.Sprintf("%s is owned by machine %s and was not ended; re-run `lym beam start`, which ends it again", entry.Slug, entry.Owner))
			continue
		}
		endDobby := false
		if c.isBeam != nil {
			beam, err := c.isBeam(entry.Slug)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s is owned by machine %s and could not be ended (%v); re-run `lym beam start`, which ends it again", entry.Slug, entry.Owner, err))
				continue
			}
			endDobby = beam
		}
		if err := c.endOwned(c.location, entry.Slug, endDobby); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s is owned by machine %s and could not be ended (%v); re-run `lym beam start`, which ends it again", entry.Slug, entry.Owner, err))
			continue
		}
		ended = append(ended, map[string]string{"slug": entry.Slug, "owner": entry.Owner})
	}
	return ended, warnings
}

// reconcileNote renders the note a fresh launch's prompt gains.
// It holds the report's summary and the outstanding failed pushes with their branches.
// It is empty when the reconcile moved, stopped and ended nothing, and no push is outstanding.
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

// prepare records on report the origin reconcile, the failed-push read, the prime claim and the ends of owned runs.
// The prime is claimed only when claimPrime is set.
// The reconcile never refuses the start: the orch is the session that resolves what it reports.
// It returns the note a fresh launch's prompt gains.
func (c *orchCLI) prepare(report *BeamUpReport, claimPrime bool) string {
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

	if claimPrime {
		report.Warnings = append(report.Warnings, c.claimPrimeNotes()...)
	}
	ended, endWarnings := c.pauseOwnedRuns(reconciled)
	report.OwnerPauses = ended
	report.Warnings = append(report.Warnings, endWarnings...)
	return reconcileNote(reconciled, failedPushes)
}

// beamUp brings the beam up over a wired receiver.
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
		note = c.prepare(&report, !opts.NoPrimeClaim)
	}

	ownLandingOf := ""
	if c.readBeamFacts != nil {
		facts, err := c.readBeamFacts()
		if err != nil {
			return report, err
		}
		here := facts.here
		if opts.Here != "" {
			here = opts.Here
		}
		if err := beamUpRefusal(facts.slug, here, facts.owner, facts.elsewhere, facts.moveWayForward, facts.held, facts.ownLanding, facts.status); err != nil {
			return report, archiveBeforeOwnerRefusal(err, facts.owner, facts.elsewhere, opts.ArchiveOwned)
		}
		if facts.held && facts.ownLanding {
			ownLandingOf = facts.slug
		}
	}

	if ownLandingOf != "" {
		report.Warnings = append(report.Warnings, fmt.Sprintf("the dobby was not started: the beam %s is held by its own landing run, and its dobby stays down while the landing runs; way forward: follow the landing with `lym batten status %s`", ownLandingOf, ownLandingOf))
	} else {
		c.startDobbyStep(&report)
	}
	c.startRelayStep(&report)

	if err := c.reedUp(); err != nil {
		return report, err
	}
	if warning := c.ensureRunsStrand(); warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}

	if opts.SkipOrch {
		return report, nil
	}

	orchReport, err := c.startOrch(note)
	report.Action, report.Strand, report.PromptSource = orchReport.Action, orchReport.Strand, orchReport.PromptSource
	report.SessionID, report.AbandonedSession = orchReport.SessionID, orchReport.AbandonedSession
	report.Hint, report.WatcherStopping = orchReport.Hint, orchReport.WatcherStopping
	report.Warnings = append(report.Warnings, orchReport.Warnings...)
	return report, err
}

// archiveBeforeOwnerRefusal runs archive before refusal stands when elsewhere is set.
// elsewhere says the registry reads owner as a machine other than this one.
// A failed archive joins the refusal with its retry; any other refusal, or a nil archive, returns refusal as it is.
func archiveBeforeOwnerRefusal(refusal error, owner string, elsewhere bool, archive func() error) error {
	if archive == nil || !elsewhere {
		return refusal
	}
	if err := archive(); err != nil {
		return fmt.Errorf("%w; archiving this machine's unpushed tips for the new owner also failed: %v; a re-run of `lym beam start` retries it", refusal, err)
	}
	return refusal
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
	if entry.Outcome == dobbyengine.OutcomeRefused {
		postMachineRefusal(c.location, report, entry.Detail)
	}
}

// postMachineRefusal posts the refusal of a dobby start whose machine identity does not resolve to the prime's orch.
// detail is the refused entry's detail.
// Only the refusal `boardengine.IdentityRefusal` reads is posted, once per bring-up.
// A post that fails adds a warning to report, and the bring-up goes on.
func postMachineRefusal(location *lymcwd.Location, report *BeamUpReport, detail string) {
	refusal := boardengine.IdentityRefusal()
	if refusal == nil || detail != refusal.Error() {
		return
	}
	if _, err := NotifyPrime(location, dobbyStartProgram, refusal.Error()); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("the machine refusal was not posted to the prime's orch (%v); way forward: read it in this warning and in `lym beam status`", err))
	}
}

// startRelayStep starts the hub's Slack relay when it is down, and records on report only a failure.
// A relay that was started, was live or has no Slack setup to start from adds nothing;
// one that did not start only warns, naming `lym slack start`, since the relay is no part of the orch.
func (c *orchCLI) startRelayStep(report *BeamUpReport) {
	if c.startRelay == nil {
		return
	}
	if _, err := c.startRelay(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("the Slack relay was not started (%v); way forward: run `lym slack start`", err))
	}
}

// relayStarter returns the relay start of the worktree at location, or nil for a beam.
// The start ensures the hub's relay with the worktree as its prime.
// A beam's bring-up never starts the hub's relay, so it gets no start.
func relayStarter(location *lymcwd.Location, beam bool) func() (slackengine.StartOutcome, error) {
	if beam {
		return nil
	}
	return func() (slackengine.StartOutcome, error) {
		geometry, err := hubgeom.SlackGeometry(location.HubPath)
		if err != nil {
			return "", err
		}
		return slackengine.EnsureRelay(geometry, location.WorktreeName)
	}
}

// startOrch leaves one live orchestrator strand and one watcher bound to it.
// It reports the orch's part of the bring-up.
// A live strand with a live watcher is left alone, a live strand with a dead watcher gets a watcher,
// and a missing or dead strand is relaunched under a withdraw round on each side.
// note joins the prompt of a fresh launch.
func (c *orchCLI) startOrch(note string) (BeamUpReport, error) {
	var report BeamUpReport
	strands, err := c.strands.Strands()
	if err != nil {
		return report, err
	}
	strand, hasStrand, err := soleOrchStrand(strands)
	if err != nil {
		return report, err
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
		_, source, err := c.relaunchStrand(st, strand, hasStrand, note, false, &report)
		if err != nil {
			return report, err
		}
		if err := c.spawnWatcher(); err != nil {
			return report, err
		}
		report.Action, report.PromptSource = actionRelaunched, source
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

// soleOrchStrand returns the orch strand among strands and whether one is listed.
// It refuses a listing that names more than one.
func soleOrchStrand(strands []reedengine.StrandStatus) (strand reedengine.StrandStatus, listed bool, err error) {
	named := orchStrands(strands)
	if len(named) > 1 {
		guids := make([]string, len(named))
		for i, s := range named {
			guids[i] = s.GUID
		}
		return strand, false, fmt.Errorf("orch: %d strands are named %q (%s); remove the extras with `lym reed remove` and re-run", len(named), orchStrandName, strings.Join(guids, ", "))
	}
	if len(named) == 1 {
		return named[0], true, nil
	}
	return strand, false, nil
}

// relaunchOrch relaunches the orch session over the orch strand reed lists, live or not.
// It returns the new strand's GUID.
// It runs directly, deciding nothing about whether the orch is already running and waiting for no watcher to go.
// The live strand and the watcher calling it would read as a running orch.
// The session st records is resumed; with fresh set the recorded session id is cleared and the launch starts fresh.
// It spawns no watcher.
func (c *orchCLI) relaunchOrch(st orchengine.State, fresh bool) (string, error) {
	strands, err := c.strands.Strands()
	if err != nil {
		return "", err
	}
	strand, hasStrand, err := soleOrchStrand(strands)
	if err != nil {
		return "", err
	}
	var report BeamUpReport
	guid, _, err := c.relaunchStrand(st, strand, hasStrand, "", fresh, &report)
	for _, warning := range report.Warnings {
		logger.Warn("orch: the restart's launch reported a warning", "strandGUID", guid, "warning", warning)
	}
	return guid, err
}

// relaunchStrand relaunches the orch session under a withdraw round on each side.
// It removes the strand listed and launches through launchOrch.
// It then registers the new process as the worktree's orch and saves st reset onto the new strand.
// It records the launch's strand, abandoned session and warnings in report, and the session id when the launch resumed.
// It returns the new strand's GUID and the prompt source of the launch.
// With fresh set the recorded session id is cleared first, so the launch starts fresh.
func (c *orchCLI) relaunchStrand(st orchengine.State, strand reedengine.StrandStatus, hasStrand bool, note string, fresh bool, report *BeamUpReport) (guid, source string, err error) {
	if fresh {
		st.SessionID = ""
	}
	receiver, err := c.orchReceiver(strand, hasStrand)
	if err != nil {
		return "", "", err
	}
	creel, err := c.newCreel()
	if err != nil {
		return "", "", fmt.Errorf("orch: reach creel: %w; %s", err, withdrawWayForward)
	}
	firstFailure, err := c.withdrawFirst(creel, receiver, hasStrand)
	if err != nil {
		return "", "", err
	}
	if hasStrand {
		if err := c.strands.RemoveStrand(strand.GUID); err != nil {
			return "", "", err
		}
	}
	if err := orchengine.RenderRoleFile(c.paths.RolePath); err != nil {
		return "", "", err
	}
	remoteControlName := c.launchRemoteControlName(strand, hasStrand)
	started, source, abandoned, err := c.launchOrch(st, note, remoteControlName)
	report.AbandonedSession = abandoned
	if err != nil {
		return "", "", err
	}
	if remoteControlName != "" {
		c.markLaunchedRemoteControl(started.GUID)
	}
	report.Strand = started.GUID
	if started.Warning != "" {
		report.Warnings = append(report.Warnings, started.Warning)
	}
	if abandoned != "" {
		report.Warnings = append(report.Warnings, freshLaunchNote(abandoned, started.Carried))
	}
	if warning := c.registerOrchProcess(started); warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}
	next := orchengine.ResetForFreshLaunch(st, started.GUID, time.Now())
	next.SessionID = started.SessionID
	if err := c.saveFreshLaunch(next, firstFailure, creel, receiver); err != nil {
		return "", "", err
	}
	logger.Info("orch: launched orchestrator session", "strandGUID", started.GUID, "sessionID", started.SessionID, "promptSource", source, "abandonedSessionID", abandoned)
	if source == orchengine.SourceResume {
		report.SessionID = started.SessionID
	}
	return started.GUID, source, nil
}

// launchOrch starts the orch session: the session st records, resumed with the reload pointer, else a fresh launch.
// A resume refused because this machine holds no transcript of the session launches fresh,
// and the session it gave up is named as abandoned; every other failed resume fails the start.
// A fresh launch takes the last completed handoff, else the start stencil, and note joins its prompt.
// Either launch starts with remote control on under remoteControlName when it is non-empty.
func (c *orchCLI) launchOrch(st orchengine.State, note, remoteControlName string) (started startedSession, source, abandoned string, err error) {
	if st.SessionID != "" {
		started, err := c.resumeOrch(st.SessionID, remoteControlName)
		if err == nil {
			return started, orchengine.SourceResume, "", nil
		}
		if !errors.Is(err, shuttleengine.ErrNoTranscript) {
			return startedSession{}, "", "", err
		}
		logger.Warn("orch: this machine holds no transcript of the recorded session; launching fresh", "sessionID", st.SessionID, "cause", err)
		abandoned = st.SessionID
	}
	prompt, source, err := orchengine.ChooseStartPrompt(c.stencilsDir, c.paths.RolePath, st, fileExists)
	if err != nil {
		return startedSession{}, "", abandoned, err
	}
	prompt = orchengine.WithLaunchFacts(c.paths, prompt)
	if note != "" {
		prompt += "\n\n" + note
	}
	started, err = c.starter.StartSession(c.orchSpec(prompt, time.Now(), remoteControlName))
	return started, source, abandoned, err
}

// resumeOrch resumes the orch session sessionID with the reload pointer.
// Remote control is on under remoteControlName when it is non-empty.
// A resume refused for a live holder ends that holder when it is this worktree's registered orch.
// It then resumes once more.
// Any other holder refuses, naming its pid.
func (c *orchCLI) resumeOrch(sessionID, remoteControlName string) (startedSession, error) {
	reload, err := orchengine.RenderReloadPrompt(c.stencilsDir, c.paths.RolePath)
	if err != nil {
		return startedSession{}, err
	}
	spec := c.orchSpec(orchengine.WithLaunchFacts(c.paths, reload), time.Now(), remoteControlName)
	spec.ResumeSessionID = sessionID
	started, err := c.starter.StartSession(spec)
	var held *shuttleengine.SessionHeldError
	if !errors.As(err, &held) {
		return started, resumeFailure(sessionID, err)
	}
	if err := c.endOwnHolder(sessionID, held.PID); err != nil {
		return startedSession{}, err
	}
	spec.TranscriptCopied = held.TranscriptCopied
	started, err = c.starter.StartSession(spec)
	return started, resumeFailure(sessionID, err)
}

// endOwnHolder ends pid, the live process holding the orch session sessionID, when it is this worktree's registered orch.
// Any other holder refuses with the way forward, so the start never launches fresh over a live session.
func (c *orchCLI) endOwnHolder(sessionID string, pid int) error {
	registered := false
	if c.orchRegistered != nil {
		var err error
		if registered, err = c.orchRegistered(pid); err != nil {
			return fmt.Errorf("orch: read whether process %d, which holds the orch session %s, is this worktree's orch: %w; way forward: repair what the error names, then run `lym beam start` again", pid, sessionID, err)
		}
	}
	if !registered {
		return fmt.Errorf("orch: the orch session %s is held by live process %d, which lym did not register as this worktree's orch, so the start launches nothing; way forward: exit the session of process %d, then run `lym beam start`", sessionID, pid, pid)
	}
	ended, err := c.endOrch()
	if err != nil {
		return fmt.Errorf("orch: end process %d, this worktree's earlier orch holding session %s: %w; way forward: exit that session, then run `lym beam start`", pid, sessionID, err)
	}
	logger.Info("orch: ended this worktree's earlier orch process to resume its session", "pid", pid, "sessionID", sessionID, "ended", len(ended))
	return nil
}

// resumeFailure wraps the failed resume of sessionID with the way forward, and is nil for a resume that started.
func resumeFailure(sessionID string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("orch: the recorded orch session %s could not be resumed: %w; way forward: fix the cause it names, such as a session that still holds it, then run `lym beam start` again", sessionID, err)
}

// freshLaunchNote says that the orch launched fresh because this machine holds no transcript of the session abandoned.
// It counts the operator instructions the launch carried over from the session it replaced.
func freshLaunchNote(abandoned string, carried int) string {
	return fmt.Sprintf("this machine holds no transcript of the orch session %s, so the orch launched fresh, carrying over %d operator instruction(s) from its earlier session", abandoned, carried)
}

// registerOrchProcess registers the provider process of the started orch session as this worktree's orch,
// so a later start that finds it holding the session ends it and resumes.
// It returns a warning when the process could not be found or registered:
// the launch stands, and a later start refuses while that process holds the session.
// A receiver with no registration wired registers nothing.
func (c *orchCLI) registerOrchProcess(started startedSession) string {
	if c.registerOrch == nil {
		return ""
	}
	if started.HolderPID == 0 {
		cause := "the provider names no live process holding it"
		if started.HolderErr != nil {
			cause = started.HolderErr.Error()
		}
		return fmt.Sprintf("the orch's provider process is not registered as this worktree's orch (%s); a later start refuses while it holds the session", cause)
	}
	if err := c.registerOrch(started.HolderPID); err != nil {
		return fmt.Sprintf("the orch's provider process %d is not registered as this worktree's orch (%v); a later start refuses while it holds the session", started.HolderPID, err)
	}
	return ""
}

// adoptListedStrand binds the persisted state to the live listed orch strand.
// It reaches creel only when the state names another run.
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

// adoptStrand resets the persisted state onto guid when it belongs to another run than the recorded one.
// The two withdraw rounds around the reset run on the orch strand's name.
// The reset clears the recorded session id, since it named the other run's session;
// the watcher's first reading of guid's run fills it.
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
