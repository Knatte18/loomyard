// run.go implements the run loop's provider-invariant core: Runner, the
// per-run Run handle, and Start — the sequence that prepares a run's
// artifacts, registers its strand with mux, and persists run.json so the
// CLI's interrupt/send verbs and a later diagnosis pass can find it again.
// Wait (wait.go) and Interrupt/Send round out the Run handle's public
// surface.

package shuttleengine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// Runner is the provider-invariant run loop: it drives one Engine
// implementation over the file contract through the MuxOps seam, so a
// caller (review, loom) constructs exactly one Runner per (mux, engine,
// layout, cfg) combination and calls Start/Run for every agent spawn.
type Runner struct {
	mux    MuxOps
	engine Engine
	layout *hubgeometry.Layout
	cfg    Config
}

// NewRunner returns a Runner ready to start runs against mux and engine,
// scoped to layout's worktree and cfg's tuning knobs.
func NewRunner(mux MuxOps, engine Engine, layout *hubgeometry.Layout, cfg Config) *Runner {
	return &Runner{mux: mux, engine: engine, layout: layout, cfg: cfg}
}

// Result is a completed run's terminal report: how it was classified, the
// identities a caller needs to act on it further (SessionID for a resume,
// StrandGUID for interrupt/send/diagnosis), the agent's last message (set
// only for OutcomeAsking), and the run directory (already removed for a
// cleaned-up OutcomeDone, still present otherwise).
type Result struct {
	Outcome              Outcome
	SessionID            string
	StrandGUID           string
	LastAssistantMessage string
	RunDir               string
}

// Run is the handle to one in-progress or completed shuttle run, returned by
// Start. Wait blocks until the run reaches a terminal outcome; Interrupt and
// Send drive the live pane while Wait is blocked (or from another process,
// via the CLI verbs that resolve a Run from run.json).
type Run struct {
	runner *Runner
	spec   Spec
	runDir string
	state  RunState

	// offset is the byte offset already consumed from state.EventsPath —
	// Wait's poll loop only re-reads and re-parses bytes appended since the
	// last tick.
	offset int64
	// deadline is the wall-clock time after which an in-progress run is
	// classified OutcomeTimeout (spec.Timeout after Start).
	deadline time.Time
	// clock is Wait's poll-loop time seam (wait.go): realClock{} in
	// production, overridden with a fake by same-package tests so a whole
	// poll sequence replays instantly.
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

// Start prepares one run described by spec and registers it with mux,
// returning a handle a caller waits on (or drives via Interrupt/Send)
// without blocking. Sequence: validate spec; opportunistically sweep
// orphaned run directories left behind by strands mux no longer tracks
// (never blocking on a sweep failure); mint a fresh run directory; ask the
// engine to prepare its provider-specific artifacts; register the strand
// with mux using the engine's Launch commands; and persist run.json. On an
// AddStrand failure the just-created run directory is removed before the
// error returns — there is nothing yet a caller could resume. A failure
// persisting run.json AFTER AddStrand succeeded removes both the run
// directory and the just-registered strand: without run.json nothing can
// resolve the strand's guid back to this run (findRunByStrand scans
// run.json files), so leaving the strand behind would launch a live,
// untracked agent pane no caller can ever wait on, interrupt, or clean up.
func (r *Runner) Start(spec Spec) (*Run, error) {
	if err := spec.validate(r.layout.WorktreeRoot, r.cfg); err != nil {
		return nil, err
	}

	r.sweepOrphansOpportunistic()

	root := runDirRoot(r.cfg, r.layout)
	runID, runDir, err := createRunDir(root)
	if err != nil {
		return nil, fmt.Errorf("shuttle: start run: %w", err)
	}

	launch, err := r.engine.Prepare(runDir, spec, r.cfg)
	if err != nil {
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("shuttle: prepare run: %w", err)
	}

	strand, err := r.mux.AddStrand(muxengine.AddSpec{
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
		if _, rerr := r.mux.RemoveStrand(strand.GUID, false); rerr != nil {
			log.Printf("shuttle: start run: remove strand %s after save-state failure (non-fatal): %v", strand.GUID, rerr)
		}
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("shuttle: save run state: %w", err)
	}

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

// StrandGUID returns the mux strand guid bound to this run. It is available
// as soon as Start returns — before Wait completes — so an in-process caller
// holding the handle can capture the run's pane, log its identity, or resolve
// it for diagnosis while the run is still in flight (the same guid Result
// carries once Wait finishes).
func (run *Run) StrandGUID() string {
	return run.state.StrandGUID
}

// Run starts spec and blocks until it reaches a terminal outcome — the
// Start+Wait convenience for a caller with no need to Interrupt/Send between
// the two.
func (r *Runner) Run(spec Spec) (Result, error) {
	run, err := r.Start(spec)
	if err != nil {
		return Result{}, err
	}
	return run.Wait()
}

// sweepOrphansOpportunistic removes run directories whose strand is no
// longer tracked in mux state, using the live-guid set read from mux.json.
// A genuinely absent state file (st == nil, no error — the post-`mux down`
// case) degrades to an empty live set: every dir old enough is a real
// orphan there. A LoadState ERROR (corrupt or unreadable mux.json) is
// different and skips the sweep entirely for this Start, rather than also
// degrading to an empty set: treating a read failure as "no live strands"
// would sweep every run dir past the age guard — including the kept
// asking/died/timeout dirs of strands mux still actually tracks — destroying
// diagnosis material over an unrelated I/O problem. Either way, a failure
// here never blocks Start: an orphaned run directory left behind costs
// nothing but disk, while blocking a new run on housekeeping would.
func (r *Runner) sweepOrphansOpportunistic() {
	st, err := muxengine.LoadState(r.layout.DotLyxDir())
	if err != nil {
		log.Printf("shuttle: orphan sweep: load mux state failed, skipping this sweep (non-fatal, new run proceeds): %v", err)
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
	if _, err := sweepOrphans(runDirRoot(r.cfg, r.layout), guids, minAge, time.Now()); err != nil {
		log.Printf("shuttle: orphan sweep failed (non-fatal, new run proceeds): %v", err)
	}
}

// Interrupt stops run's in-progress turn without killing its pane or
// session: after confirming the strand still has a live pane showing the
// provider's input-ready TUI (see requireReadyAgentPane), it plays the
// engine's InterruptSequence (e.g. a single
// Escape key press) through the mux seam. The pane stays warm and idle afterward —
// the caller typically follows with Send to give the agent updated
// instructions and let it continue, or lets the operator attach directly.
// Safe to call concurrently with a blocked Wait: mux's op lock serializes
// the underlying send-keys calls, and Interrupt mutates no Run-local state.
//
// Calibration (verified live): a provider's Stop hook fires on ANY turn end,
// including one ended by Interrupt itself, not only a self-completed or
// asking turn. A blocked Wait can therefore classify and return (typically
// OutcomeAsking, since the file contract is usually still unmet) from the
// INTERRUPTED turn's own Stop event before a caller's subsequent Send ever
// starts its redirect turn — Wait has no notion of "an interrupt is coming,
// don't classify yet." Interrupt+Send still reliably DELIVER the redirect
// to the still-live pane (the agent process keeps running independently of
// whatever Wait already returned), but a caller must not assume the SAME
// Wait call will observe the redirect's own eventual outcome — this is the
// documented v1 limitation that there is no re-wait path once Wait returns.
func (run *Run) Interrupt() error {
	if err := requireReadyAgentPane(run.runner.mux, run.runner.engine, run.state.StrandGUID); err != nil {
		return err
	}
	return playInputs(run.runner.mux, run.state.StrandGUID, run.runner.engine.InterruptSequence())
}

// Send types text as run's next turn: text must be a single, non-empty line
// — the file contract carries multiline updates (write a file and Send a
// one-line pointer to it, e.g. "read <file> — updated instructions — and
// continue"), and an empty or whitespace-only send has nothing to deliver
// (see validateSendText).
// It plays the engine's ComposeSend choreography (typically clearing a
// leaked auto-suggest before typing text and submitting it) through the mux
// seam and then VERIFIES delivery by observing the text in the pane capture,
// replaying once if it never appears (see sendVerified — the provider TUI
// can silently swallow the whole input). A nil return therefore means the
// text was observed on screen, not merely that keys were emitted. Safe to
// call concurrently with a blocked Wait, for the same reason as Interrupt.
func (run *Run) Send(text string) error {
	if err := validateSendText(text); err != nil {
		return err
	}
	if err := requireReadyAgentPane(run.runner.mux, run.runner.engine, run.state.StrandGUID); err != nil {
		return err
	}
	return sendVerified(run.runner.mux, run.runner.engine, run.state.StrandGUID, text)
}

// validateSendText rejects text that cannot be delivered as a single agent
// turn, the shared guard both (*Run).Send and (*Runner).Send run before
// touching the pane. Multiline text is rejected because the file contract —
// not the input line — carries multiline updates (write a file and Send a
// one-line pointer to it). Empty or whitespace-only text is rejected because
// there is nothing to deliver: it would still play the Escape+submit
// choreography (a stray empty turn) yet make sendVerified's delivery check
// vacuous — the normalized needle would be "", which every pane capture
// trivially "contains", so a nil return would falsely claim a verified send.
func validateSendText(text string) error {
	if strings.ContainsAny(text, "\n\r") {
		return fmt.Errorf("shuttle: Send: text must be a single line; multiline updates ride the file contract (write a file, Send a one-line pointer to it)")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("shuttle: Send: text must not be empty or whitespace-only; there is nothing to deliver as the agent's next turn")
	}
	return nil
}

// Interrupt stops the in-progress turn of the run whose strand is identified
// by guid, without needing an in-process Run handle — this is how the CLI's
// interrupt verb reaches a run started by a separate process. It resolves
// guid via FindRun to confirm it actually names a shuttle run, confirms the
// strand still has a live pane showing the provider's input-ready TUI
// (requireReadyAgentPane), then plays the engine's
// InterruptSequence through the mux seam via the same playInputs helper
// (*Run).Interrupt uses. FindRun's underlying error is wrapped (%w) into the
// "not a shuttle strand" message rather than discarded, so an operator
// debugging a genuine I/O error against the run-dir root (as opposed to a
// simple wrong guid) is not told the guid is the problem.
func (r *Runner) Interrupt(guid string) error {
	if _, _, err := FindRun(r.cfg, r.layout, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireReadyAgentPane(r.mux, r.engine, guid); err != nil {
		return err
	}
	return playInputs(r.mux, guid, r.engine.InterruptSequence())
}

// Send types text as the next turn of the run whose strand is identified by
// guid, without needing an in-process Run handle — this is how the CLI's
// send verb reaches a run started by a separate process. It enforces the
// same single-line, non-empty rule as (*Run).Send (validateSendText),
// resolves guid via FindRun to
// confirm it actually names a shuttle run, confirms the strand still has a
// live pane showing the provider's input-ready TUI (requireReadyAgentPane),
// then plays and delivery-verifies the
// engine's ComposeSend choreography via the same sendVerified helper
// (*Run).Send uses — a nil return means the text was observed in the pane.
// FindRun's underlying error is wrapped (%w) into the "not a shuttle
// strand" message rather than discarded, for the same reason as
// (*Runner).Interrupt.
func (r *Runner) Send(guid, text string) error {
	if err := validateSendText(text); err != nil {
		return err
	}
	if _, _, err := FindRun(r.cfg, r.layout, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireReadyAgentPane(r.mux, r.engine, guid); err != nil {
		return err
	}
	return sendVerified(r.mux, r.engine, guid, text)
}

// Send delivery-verification tuning: after playing the ComposeSend
// choreography, the send path polls the pane capture for the sent text —
// up to sendVerifyAttempts polls, sendVerifyInterval apart — and replays
// the whole choreography up to sendReplays more times before reporting an
// honest delivery failure. The polling exists because the swallow it
// guards against (see PaneInput.SettleMS) produces NO error anywhere:
// without observing the text in the pane, "ok" would be a guess.
const (
	sendVerifyAttempts = 20
	sendVerifyInterval = 250 * time.Millisecond
	sendReplays        = 1
)

// inputSleep is the real-time pause seam for pane-input pacing
// (PaneInput.SettleMS) and send-delivery polling. A package-level variable
// rather than a direct time.Sleep call so same-package tests can replace it
// and drive the retry/verify loops instantly.
var inputSleep = time.Sleep

// Agent-pane probe tuning: requireReadyAgentPane classifies up to
// agentPaneProbeAttempts pane captures, agentPaneProbeInterval apart, before
// refusing — one capture could land mid-redraw and transiently classify a
// perfectly healthy provider TUI as still booting.
const (
	agentPaneProbeAttempts = 3
	agentPaneProbeInterval = 250 * time.Millisecond
)

// requireReadyAgentPane fails unless guid's strand has a live pane
// (requireLiveStrand) whose current capture the engine classifies as
// StartupReady — the provider's input-ready TUI actually on screen. Every
// Interrupt/Send entry point runs this guard before playing keys, because
// pane liveness alone only proves the pane's SHELL is alive: a provider
// that failed at launch (or was killed while its shell survived) leaves a
// live pane where played keys land at the shell prompt — proven live, a
// send against a kept "died" run reported ok while its text was executed as
// a pwsh command in the diagnosis pane. The same refusal also fires for a
// provider that is merely STILL BOOTING (an interrupt/send issued seconds
// after Start — the probe window is much shorter than a normal provider
// startup), which is equally correct to refuse but a different diagnosis;
// the error text names both readings. Known residual limitations,
// inherited from the same startup-marker heuristic (see claudeengine's
// Startup): a shell prompt styled with the provider's ready marker, or a
// dead provider whose final TUI frame is still rendered in the pane, can
// still false-pass — the guard narrows the hole to the states a capture can
// distinguish, it cannot close it.
func requireReadyAgentPane(mux MuxOps, engine Engine, guid string) error {
	if err := requireLiveStrand(mux, guid); err != nil {
		return err
	}

	var lastCaptureErr error
	for attempt := 0; attempt < agentPaneProbeAttempts; attempt++ {
		if attempt > 0 {
			inputSleep(agentPaneProbeInterval)
		}
		capture, err := mux.CapturePane(guid)
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
	// A non-Ready capture cannot distinguish a provider that never came up
	// (or crashed behind a surviving shell) from one that is simply still
	// booting — the probe window is far shorter than a normal provider
	// startup, so an interrupt/send issued seconds after Start lands here on
	// a perfectly healthy run. The message must own both readings rather
	// than misdiagnose a booting pane as a dead agent.
	return fmt.Errorf("shuttle: strand %q's pane shows no input-ready provider TUI — either the provider is still starting up (retry once it is ready), or its process exited (launch failure or crash) while the pane's shell stayed alive, in which case keys would be executed by the shell instead of reaching an agent", guid)
}

// requireLiveStrand fails unless guid's strand is currently tracked by mux
// AND bound to a live pane. It is the first half of requireReadyAgentPane's
// guard (and separately keeps the cheap failure modes cheap): psmux's
// send-keys against a dead or missing pane exits 0 while delivering nothing
// (proven live: interrupt/send against a run that had classified "died"
// both reported success as silent no-ops) — without the guard, the exact
// verbs the kept died/timeout state exists to support would lie to the
// operator.
func requireLiveStrand(mux MuxOps, guid string) error {
	status, err := mux.Status()
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
	return fmt.Errorf("shuttle: strand %q is not tracked by mux — its run has completed and been cleaned up", guid)
}

// playInputs plays inputs into guid's pane through mux, in order: a Key
// step sends a named key (SendKey), a Text step types literal text and,
// when Submit is set, follows it with Enter (SendText's submit flag) — the
// shared choreography both Interrupt and Send drive, and the same one
// batch 5's CLI interrupt/send verbs reuse through the engine so every
// caller plays a PaneInput sequence identically. A step's SettleMS is
// honored after the step lands, so an engine can force a pause between an
// Escape and the text that follows it (see PaneInput.SettleMS for why).
func playInputs(mux MuxOps, guid string, inputs []PaneInput) error {
	for _, in := range inputs {
		if in.Key != "" {
			if err := mux.SendKey(guid, in.Key); err != nil {
				return fmt.Errorf("shuttle: send key %q: %w", in.Key, err)
			}
		} else if err := mux.SendText(guid, in.Text, in.Submit); err != nil {
			return fmt.Errorf("shuttle: send text: %w", err)
		}
		if in.SettleMS > 0 {
			inputSleep(time.Duration(in.SettleMS) * time.Millisecond)
		}
	}
	return nil
}

// sendVerified plays engine.ComposeSend(text) into guid's pane and then
// CONFIRMS delivery by polling the pane capture until the sent text appears
// MORE times than it did before the send, replaying the choreography up to
// sendReplays more times before failing. The confirmation is not optional
// politeness: the provider TUI can swallow the entire Escape+text chunk
// with no error anywhere (observed live — `lyx shuttle send` reported ok
// while nothing reached the agent), so "the text newly appeared on screen"
// is the only honest definition of a delivered send. It is the occurrence
// COUNT that must rise, not mere presence, because the text can already be
// on screen before the send — an operator retrying the same instruction
// after an uncertain first attempt (the natural reaction to exactly the
// swallow this check guards against), or text quoting the agent's own
// visible output — and a presence check would then verify a swallowed send
// vacuously. Matching is whitespace-stripped and lowercased
// (normalizePaneText) because pane captures can drop spaces entirely and
// wrap long lines, and only a bounded prefix of the text is required so a
// line-wrapped tail cannot defeat the match.
func sendVerified(mux MuxOps, engine Engine, guid, text string) error {
	needle := normalizePaneText(text)
	if runes := []rune(needle); len(runes) > 48 {
		needle = string(runes[:48])
	}

	// Snapshot how often the needle is already on screen; delivery is a NEW
	// occurrence on top of this. A failed baseline capture degrades to 0 —
	// the presence-only semantics this check strengthens, never weaker.
	baseline := 0
	if capture, err := mux.CapturePane(guid); err == nil {
		baseline = strings.Count(normalizePaneText(capture), needle)
	}

	for try := 0; try <= sendReplays; try++ {
		if err := playInputs(mux, guid, engine.ComposeSend(text)); err != nil {
			return err
		}
		for attempt := 0; attempt < sendVerifyAttempts; attempt++ {
			capture, err := mux.CapturePane(guid)
			if err == nil && strings.Count(normalizePaneText(capture), needle) > baseline {
				return nil
			}
			// A capture error here is transient noise, not fatal: the next
			// poll (or the replay) either observes the text or the loop
			// reports the delivery failure honestly at the end.
			inputSleep(sendVerifyInterval)
		}
	}
	return fmt.Errorf("shuttle: Send: sent text never appeared in the pane after %d attempt(s) — the provider TUI likely swallowed the input; the send was NOT delivered", 1+sendReplays)
}

// normalizePaneText lowercases s and strips every whitespace rune — the
// canonical form sendVerified matches pane captures against, since a pane
// capture can drop spaces entirely (an observed TUI rendering quirk) and
// wraps long lines with newlines.
func normalizePaneText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
