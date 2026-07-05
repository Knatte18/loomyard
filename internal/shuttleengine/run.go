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
	"time"

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
// error returns — there is nothing yet a caller could resume.
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
// longer tracked in mux state, using the live-guid set read from mux.json
// (an absent or unreadable state file degrades to an empty set — the age
// guard inside sweepOrphans is what keeps a genuinely still-starting run
// safe either way, not this set being complete). A failure anywhere in this
// step — loading mux state or the sweep itself — is logged and never blocks
// Start: an orphaned run directory left behind by a failed sweep costs
// nothing but disk, while blocking a new run on housekeeping would.
func (r *Runner) sweepOrphansOpportunistic() {
	guids := map[string]bool{}
	if st, err := muxengine.LoadState(r.layout.DotLyxDir()); err != nil {
		log.Printf("shuttle: orphan sweep: load mux state (non-fatal, treating as no live strands): %v", err)
	} else if st != nil {
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
