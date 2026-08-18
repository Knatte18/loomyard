// run.go implements the run loop's provider-invariant core: Runner, the per-run Run handle, and
// Start — the sequence that prepares a run's artifacts, registers its strand with reed, and
// persists run.json so the CLI's interrupt/send verbs and a later diagnosis pass can find it again.
// Wait (wait.go) and Interrupt/Send round out the Run handle's public surface.

package shuttleengine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// Runner is the provider-invariant run loop: it drives one Engine implementation over the file
// contract through the ReedOps seam, so a caller (review, loom) constructs exactly one Runner per
// (reed, engine, anchorPath, worktreeRoot, cfg) combination and calls Start/Run for every agent
// spawn.
// Runner is told its anchor path and worktree root as plain strings and derives neither;
// populating both with usable absolute paths is the caller's obligation.
type Runner struct {
	reed         ReedOps
	engine       Engine
	anchorPath   string
	worktreeRoot string
	cfg          Config
	// toldErr is validateToldPaths' verdict on the pair this Runner was constructed with, computed
	// once and returned by every public entry point. It is held rather than returned from
	// NewRunner because a constructor that cannot fail is what every caller already writes.
	toldErr error
}

// NewRunner returns a Runner ready to start runs against reed and engine, scoped to anchorPath and
// worktreeRoot and cfg's tuning knobs.
// NewRunner is told anchorPath and worktreeRoot as plain strings and derives neither;
// populating both with usable absolute paths is the caller's obligation.
// The pair is validated here (see validateToldPaths) and an unusable one is reported by every
// public method rather than by this constructor, which stays total.
func NewRunner(reed ReedOps, engine Engine, anchorPath, worktreeRoot string, cfg Config) *Runner {
	return &Runner{
		reed:         reed,
		engine:       engine,
		anchorPath:   anchorPath,
		worktreeRoot: worktreeRoot,
		cfg:          cfg,
		toldErr:      validateToldPaths(anchorPath, worktreeRoot),
	}
}

// validateToldPaths reports an error unless the told pair is one this package can spend.
//
// It exists because the two fields are ADJACENT PARAMETERS OF THE SAME TYPE with no structural
// distinction between them, while their four consumers are semantically distinct: anchorPath sites
// the run-dir root (.lyx is the anchor-side sibling of _lyx), is where reed keeps the reed.json the
// orphan sweep reads, and is the pane's own process cwd that the fork audit derives the provider's
// transcript directory from; worktreeRoot is what a relative OutputFiles entry resolves against, as
// the run verb's own help promises. A caller that swaps them compiles cleanly and, in a
// subpath-anchored worktree, silently puts all three of the first three somewhere real but wrong.
// reed hardened the same seam from the other side (validateToldAnchorPath, server.go), on the same
// reasoning: an empty or relative value does not fail, it succeeds against the WRONG tree.
//
// The containment clause is the swap detector rather than a geometric preference: hub geometry
// always satisfies AnchorPath == WorktreeRoot/AnchorRel (hubgeom.ReedGeometry reads both off one
// resolved Location), so a swap is exactly the case that violates it. Equality is allowed, since a
// worktree anchored at its own root has AnchorRel ".".
func validateToldPaths(anchorPath, worktreeRoot string) error {
	if anchorPath == "" || worktreeRoot == "" {
		return fmt.Errorf("shuttle: NewRunner was told an empty path (anchorPath %q, worktreeRoot %q); both are required and neither is derived", anchorPath, worktreeRoot)
	}
	if !filepath.IsAbs(anchorPath) || !filepath.IsAbs(worktreeRoot) {
		return fmt.Errorf("shuttle: NewRunner was told a relative path (anchorPath %q, worktreeRoot %q): a relative value does not fail, it silently resolves the run directory, reed's state lookup, and the fork audit's transcript directory against whatever working directory the caller happens to have", anchorPath, worktreeRoot)
	}
	rel, err := filepath.Rel(worktreeRoot, anchorPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("shuttle: NewRunner was told an anchor path %q outside its worktree root %q: the anchor is always the worktree root or a subdirectory of it, so this pair is most likely swapped — anchorPath sites the run directory, reed's state lookup, and the fork audit's workdir, while worktreeRoot only resolves relative output files", anchorPath, worktreeRoot)
	}
	return nil
}

// Result is a completed run's terminal report: how it was classified, the identities a caller needs
// to act on it further (SessionID for a resume, StrandGUID for interrupt/send/diagnosis), the
// agent's last message (set only for OutcomeAsking), and the run directory (already removed for a
// cleaned-up OutcomeDone, still present otherwise).
type Result struct {
	Outcome              Outcome
	SessionID            string
	StrandGUID           string
	LastAssistantMessage string
	RunDir               string
	// ForkAudit is populated only when the Spec that started this run set
	// ForkSubagents, the run classified OutcomeDone, and the audit itself
	// succeeded; nil otherwise. A nil ForkAudit therefore always reads as "not
	// audited" and never as "audited, found nothing" — including on the one
	// path where Outcome is OutcomeDone and the returned error is non-nil,
	// which is a done run whose audit could not be read (see finalize).
	ForkAudit *ForkAudit
}

// Run is the handle to one in-progress or completed shuttle run, returned by Start.
// Wait blocks until the run reaches a terminal outcome;
// Interrupt and Send drive the live pane while Wait is blocked (or from another process, via the
// CLI verbs that resolve a Run from run.json).
type Run struct {
	runner *Runner
	spec   Spec
	runDir string
	state  RunState

	// offset is the byte offset already consumed from state.EventsPath.
	offset int64
	// deadline is the wall-clock time after which a run is classified OutcomeTimeout.
	deadline time.Time
	// clock is the time seam for tests.
	clock clock
}

// The run directory's fixed artifact file names. Every Engine.Prepare
// implementation writes to these same names (claudeengine does), and Start
// independently derives the same paths to populate RunState — the run-dir
// layout convention the CLI's interrupt/send verbs and diagnosis rely on.
const (
	promptFileName   = "prompt.md"
	settingsFileName = "settings.json"
	eventsFileName   = "events.jsonl"
)

// Start prepares one run described by spec and registers it with reed, returning a handle without
// blocking.
// On AddStrand failure the run directory is removed.
// On a run.json persistence failure after AddStrand, both the directory and strand are cleaned up
// to avoid leaking an untracked agent pane.
func (r *Runner) Start(spec Spec) (*Run, error) {
	if r.toldErr != nil {
		return nil, r.toldErr
	}
	if err := spec.validate(r.worktreeRoot, r.cfg); err != nil {
		return nil, err
	}

	r.sweepOrphansOpportunistic()

	root := runDirRoot(r.cfg, r.anchorPath)
	runID, runDir, err := createRunDir(root)
	if err != nil {
		return nil, fmt.Errorf("shuttle: start run: %w", err)
	}

	launch, err := r.engine.Prepare(runDir, spec, r.cfg)
	if err != nil {
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("shuttle: prepare run: %w", err)
	}

	strand, err := r.reed.AddStrand(reedengine.AddSpec{
		Role:      spec.Role,
		Round:     spec.Round,
		Parent:    spec.Parent,
		Cmd:       launch.Cmd,
		ResumeCmd: launch.ResumeCmd,
		SessionID: launch.SessionID,
		Display:   spec.Display,
	})
	if err != nil {
		// Nothing to resume: the strand never registered, so the run
		// directory this attempt created is cleaned up rather than left as
		// an unclaimable orphan.
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("shuttle: add strand: %w", err)
	}

	state := RunState{
		RunID:        runID,
		StrandGUID:   strand.GUID,
		SessionID:    launch.SessionID,
		Interactive:  spec.Interactive,
		OutputFiles:  spec.OutputFiles,
		PromptPath:   filepath.Join(runDir, promptFileName),
		SettingsPath: filepath.Join(runDir, settingsFileName),
		EventsPath:   filepath.Join(runDir, eventsFileName),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := saveRunState(runDir, state); err != nil {
		// The strand registered and its pane is already launching, but
		// without a persisted run.json nothing can find, wait on, or clean up
		// this run: findRunByStrand can't resolve its guid, no process ever
		// enters Wait, and sweepOrphans would only reach it much later. Tear
		// the strand and directory back down so the failure is honest rather
		// than leaking a live, untracked agent pane — the same cleanup the
		// AddStrand-failure path above performs.
		logger.Warn("shuttle: save run state failed", "runDir", runDir, "strandGUID", strand.GUID, "error", err)
		if _, rerr := r.reed.RemoveStrand(strand.GUID, false); rerr != nil {
			log.Printf("shuttle: start run: remove strand %s after save-state failure (non-fatal): %v", strand.GUID, rerr)
		}
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("shuttle: save run state: %w", err)
	}

	logger.Info("shuttle: run started", "runDir", runDir, "strandGUID", strand.GUID, "sessionID", launch.SessionID, "role", spec.Role, "round", spec.Round, "forkSubagents", spec.ForkSubagents)

	clk := clock(realClock{})
	return &Run{
		runner:   r,
		spec:     spec,
		runDir:   runDir,
		state:    state,
		clock:    clk,
		deadline: clk.Now().Add(spec.Timeout),
	}, nil
}

// StrandGUID returns the reed strand guid bound to this run.
// It is available as soon as Start returns — before Wait completes — so an in-process caller
// holding the handle can capture the run's pane, log its identity, or resolve it for diagnosis
// while the run is still in flight (the same guid Result carries once Wait finishes).
func (run *Run) StrandGUID() string {
	return run.state.StrandGUID
}

// Run starts spec and blocks until it reaches a terminal outcome — the Start+Wait convenience for a
// caller with no need to Interrupt/Send between the two.
func (r *Runner) Run(spec Spec) (Result, error) {
	run, err := r.Start(spec)
	if err != nil {
		return Result{}, err
	}
	return run.Wait()
}

// sweepOrphansOpportunistic removes run directories whose strand is no longer
// tracked in reed state. A LoadState error skips the sweep entirely, to avoid
// sweeping kept diagnosis dirs over an unrelated I/O problem. Failures never block Start.
func (r *Runner) sweepOrphansOpportunistic() {
	st, err := reedengine.LoadState(filepath.Join(r.anchorPath, lyxdirs.DotLyxDirName))
	if err != nil {
		log.Printf("shuttle: orphan sweep: load reed state failed, skipping this sweep (non-fatal, new run proceeds): %v", err)
		return
	}

	guids := map[string]bool{}
	if st != nil {
		for _, s := range st.Strands {
			guids[s.GUID] = true
		}
	}

	startupTimeout := time.Duration(r.cfg.StartupTimeoutS) * time.Second
	minAge := 2 * startupTimeout
	if _, err := sweepOrphans(runDirRoot(r.cfg, r.anchorPath), guids, minAge, time.Now()); err != nil {
		log.Printf("shuttle: orphan sweep failed (non-fatal, new run proceeds): %v", err)
	}
}

// Interrupt stops run's in-progress turn without killing its pane or session.
// Safe to call concurrently with a blocked Wait.
// Note: Wait may classify and return from the interrupted turn's Stop event before a subsequent
// Send is issued;
// the redirect is delivered but the same Wait call won't observe it.
func (run *Run) Interrupt() error {
	if err := requireReadyAgentPane(run.runner.reed, run.runner.engine, run.state.StrandGUID); err != nil {
		return err
	}
	return playInputs(run.runner.reed, run.state.StrandGUID, run.runner.engine.InterruptSequence())
}

// Send types text as run's next turn.
// Text must be a single, non-empty line.
// Verifies delivery by observing the text in the pane capture, replaying once if it never appears.
// Safe to call concurrently with a blocked Wait.
func (run *Run) Send(text string) error {
	if err := validateSendText(text); err != nil {
		return err
	}
	if err := requireReadyAgentPane(run.runner.reed, run.runner.engine, run.state.StrandGUID); err != nil {
		return err
	}
	return sendVerified(run.runner.reed, run.runner.engine, run.state.StrandGUID, text)
}

// validateSendText rejects multiline text, empty text, or whitespace-only text
// that cannot be delivered as a single agent turn.
func validateSendText(text string) error {
	if strings.ContainsAny(text, "\n\r") {
		return fmt.Errorf("shuttle: Send: text must be a single line; multiline updates ride the file contract (write a file, Send a one-line pointer to it)")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("shuttle: Send: text must not be empty or whitespace-only; there is nothing to deliver as the agent's next turn")
	}
	return nil
}

// Interrupt stops the in-progress turn of the run identified by guid, without needing an in-process
// Run handle.
// This is how the CLI's interrupt verb reaches a run started by a separate process.
func (r *Runner) Interrupt(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireReadyAgentPane(r.reed, r.engine, guid); err != nil {
		return err
	}
	return playInputs(r.reed, guid, r.engine.InterruptSequence())
}

// Send types text as the next turn of the run identified by guid, without needing an in-process Run
// handle.
// This is how the CLI's send verb reaches a run started by a separate process.
func (r *Runner) Send(guid, text string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	if err := validateSendText(text); err != nil {
		return err
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireReadyAgentPane(r.reed, r.engine, guid); err != nil {
		return err
	}
	return sendVerified(r.reed, r.engine, guid, text)
}

// Inject plays inputs into the live pane of the run identified by guid, without needing an
// in-process Run handle.
// Unlike Send/Interrupt, Inject deliberately skips the requireReadyAgentPane guard to deliver keys
// while the provider is busy.
// Empty inputs is rejected.
func (r *Runner) Inject(guid string, inputs []PaneInput) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	if len(inputs) == 0 {
		return fmt.Errorf("shuttle: Inject: inputs must not be empty — there is nothing to deliver")
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return err
	}
	return playInputs(r.reed, guid, inputs)
}

const (
	sendVerifyAttempts = 20
	sendVerifyInterval = 250 * time.Millisecond
	sendReplays        = 1
)

// inputSleep is the time seam for tests to control pacing.
var inputSleep = time.Sleep

const (
	agentPaneProbeAttempts = 3
	agentPaneProbeInterval = 250 * time.Millisecond
)

// requireReadyAgentPane fails unless guid's strand has a live pane and the
// current capture classifies as StartupReady. Distinguishes between a provider
// that failed at launch and one still booting, with known residual limitations
// where a shell prompt styled with the provider's ready marker may false-pass.
func requireReadyAgentPane(reed ReedOps, engine Engine, guid string) error {
	if err := requireLiveStrand(reed, guid); err != nil {
		return err
	}

	var lastCaptureErr error
	for attempt := 0; attempt < agentPaneProbeAttempts; attempt++ {
		if attempt > 0 {
			inputSleep(agentPaneProbeInterval)
		}
		capture, err := reed.CapturePane(guid)
		if err != nil {
			// A capture error may be transient noise (like sendVerified's
			// polls treat it); only after every attempt fails does it become
			// the reported reason.
			lastCaptureErr = err
			continue
		}
		lastCaptureErr = nil
		if engine.Startup(capture) == StartupReady {
			return nil
		}
	}
	if lastCaptureErr != nil {
		return fmt.Errorf("shuttle: capture strand %q's pane to confirm the provider TUI: %w", guid, lastCaptureErr)
	}
	return fmt.Errorf("shuttle: strand %q's pane shows no input-ready provider TUI — either the provider is still starting up (retry once it is ready), or its process exited (launch failure or crash) while the pane's shell stayed alive, in which case keys would be executed by the shell instead of reaching an agent", guid)
}

// requireLiveStrand fails unless guid's strand is tracked by reed and bound
// to a live pane. This guards against tmux send-keys exiting 0 on dead panes.
func requireLiveStrand(reed ReedOps, guid string) error {
	status, err := reed.Status()
	if err != nil {
		return fmt.Errorf("shuttle: check strand liveness: %w", err)
	}
	for _, s := range status.Strands {
		if s.GUID != guid {
			continue
		}
		if !s.Live {
			return fmt.Errorf("shuttle: strand %q has no live pane — its run already reached a terminal outcome or its pane died; keys would be silently dropped", guid)
		}
		return nil
	}
	// Naming only the completed-and-cleaned-up cause would be wrong more often than right: a
	// `lyx reed remove`, a `down`/`up` cycle, and a server rebirth all reach this same branch with
	// the run still very much unfinished (proven live). State what was observed, then the causes.
	return fmt.Errorf("shuttle: strand %q is not tracked by reed — either its run completed and was cleaned up, or reed's strand table was reset under it (a reed remove/down, or a lost or rebuilt reed.json); check \"lyx reed status\"", guid)
}

// playInputs plays inputs into guid's pane through reed in order, with
// SettleMS honored after each step to allow pauses between steps.
func playInputs(reed ReedOps, guid string, inputs []PaneInput) error {
	for _, in := range inputs {
		if in.Key != "" {
			if err := reed.SendKey(guid, in.Key); err != nil {
				return fmt.Errorf("shuttle: send key %q: %w", in.Key, err)
			}
		} else if err := reed.SendText(guid, in.Text, in.Submit); err != nil {
			return fmt.Errorf("shuttle: send text: %w", err)
		}
		if in.SettleMS > 0 {
			inputSleep(time.Duration(in.SettleMS) * time.Millisecond)
		}
	}
	return nil
}

// sendVerified plays engine.ComposeSend(text) and verifies delivery by
// polling for the sent text to appear MORE times than before the send.
func sendVerified(reed ReedOps, engine Engine, guid, text string) error {
	needle := normalizePaneText(text)
	if runes := []rune(needle); len(runes) > 48 {
		needle = string(runes[:48])
	}

	baseline := 0
	if capture, err := reed.CapturePane(guid); err == nil {
		baseline = strings.Count(normalizePaneText(capture), needle)
	}

	for try := 0; try <= sendReplays; try++ {
		if err := playInputs(reed, guid, engine.ComposeSend(text)); err != nil {
			return err
		}
		for attempt := 0; attempt < sendVerifyAttempts; attempt++ {
			capture, err := reed.CapturePane(guid)
			if err == nil && strings.Count(normalizePaneText(capture), needle) > baseline {
				return nil
			}
			inputSleep(sendVerifyInterval)
		}
	}
	return fmt.Errorf("shuttle: Send: sent text never appeared in the pane after %d attempt(s) — the provider TUI likely swallowed the input; the send was NOT delivered", 1+sendReplays)
}

// normalizePaneText lowercases s and strips whitespace for canonical matching.
func normalizePaneText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
