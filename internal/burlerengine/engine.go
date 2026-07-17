// engine.go implements the round driver: Engine.Run validates a Profile,
// composes its prompt, drives one shuttle run over the Shuttle seam, and
// maps the shuttle's outcome (plus, on done, the parsed review file) into a
// Result. This is the library's one external entry point — perch (unbuilt)
// will call it once per round.

package burlerengine

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Shuttle is the seam Engine drives one round through: the subset of
// shuttleengine's API a round needs, satisfied as-is by
// *shuttleengine.Runner in production and by a fake in unit tests. Keeping
// this interface package-local (rather than importing shuttleengine's own
// MuxOps-style seam) is what lets burlerengine stay engine-agnostic and
// testable without wiring mux or an LLM provider.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// var _ Shuttle = (*shuttleengine.Runner)(nil) is the compile-time proof
// that *shuttleengine.Runner satisfies Shuttle as-is, so production wiring
// (burlercli) never needs an adapter type.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// Engine drives burler rounds through a Shuttle, resolving Profile paths
// against layout's worktree root and Profile.ClusterFan against cfg's
// lens/fan library.
type Engine struct {
	shuttle Shuttle
	layout  *hubgeometry.Layout
	cfg     Config
}

// New returns an Engine ready to run rounds against shuttle, resolving
// relative Profile paths against layout.WorktreeRoot and any Profile.ClusterFan
// against cfg (the burler.yaml lens/fan library, loaded via LoadConfig).
func New(shuttle Shuttle, layout *hubgeometry.Layout, cfg Config) *Engine {
	return &Engine{shuttle: shuttle, layout: layout, cfg: cfg}
}

// Result is one round's outcome: how the shuttle run classified (Outcome),
// the parsed verdict and findings (set only when Outcome is
// shuttleengine.OutcomeDone and the review file parses cleanly), the
// resolved output paths, and the identities/last-message/run-dir a caller
// needs to act on a non-done outcome further.
type Result struct {
	Outcome              shuttleengine.Outcome
	Verdict              Verdict
	Findings             []Finding
	ReviewPath           string
	FixerReportPath      string
	SessionID            string
	StrandGUID           string
	LastAssistantMessage string
	// RunDir is a 1:1 passthrough of shuttleengine.Result.RunDir: the kept
	// shuttle run directory a caller surfaces when a round dies or times
	// out, so it can point an operator (or perch's own error message) at
	// the run's SessionID/StrandGUID and artifacts for inspection.
	RunDir string
	// ForkAudit is a 1:1 passthrough of shuttleengine.Result.ForkAudit, set
	// only for a cluster round (Profile.ClusterFan != "") whose run reached
	// shuttleengine.OutcomeDone. nil for a non-cluster round or a
	// non-done outcome.
	ForkAudit *shuttleengine.ForkAudit
	// ClusterWarnings carries the non-fatal audit findings auditClusterRound
	// returns for a cluster round (e.g. a fork that never returned a
	// report) — sloppiness no mechanism prevents in advance, surfaced here
	// rather than failing the round. Empty for a non-cluster round.
	ClusterWarnings []string
}

// Run drives one burler round for p, tuned by opts. Sequence: validate p
// against the engine's worktree root; compose its prompt; build the
// shuttle Spec (Interactive/Parent/Display/KeepPane stay zero-valued —
// rounds are autonomous by default, per the run-tuning-off-profile
// decision); run it through the Shuttle seam; populate Result from the
// shuttle Result; for a cluster round (p.ClusterFan != "") that reached
// done, copy the shuttle's ForkAudit onto Result and enforce the cluster
// audit policy (auditClusterRound) before reading the review file at all;
// and, only when the run reached shuttleengine.OutcomeDone, read and
// strictly parse the review file into Verdict/Findings.
//
// Run returns a nil error for every non-done outcome (asking/died/timeout
// are normal loop events a caller branches on via Result.Outcome, with an
// empty Verdict) and reserves errors for hard failures: an invalid profile,
// a shuttle start/run failure, a cluster audit policy violation, and —
// deliberately fail-loud — a verdict parse failure on a done run, since a
// defaulted verdict could silently terminate a caller's round loop on a
// malformed round.
func (e *Engine) Run(p Profile, opts RunOpts) (Result, error) {
	if err := p.validate(e.layout.WorktreeRoot, e.cfg); err != nil {
		return Result{}, err
	}

	prompt, err := composePrompt(&p)
	if err != nil {
		return Result{}, err
	}

	spec := shuttleengine.Spec{
		Prompt:      prompt,
		OutputFiles: []string{p.ReviewPath, p.FixerReportPath},
		Model:       opts.Model,
		Effort:      opts.Effort,
		Timeout:     opts.Timeout,
		Role:        "burler",
		Round:       opts.Round,
		// ForkSubagents authorizes cluster-round fork spawns only when a
		// fan actually resolved — a non-cluster round never touches this
		// authorization.
		ForkSubagents: p.ClusterFan != "",
	}

	shuttleResult, err := e.shuttle.Run(spec)
	if err != nil {
		return Result{}, fmt.Errorf("burler: shuttle run: %w", err)
	}

	result := Result{
		Outcome:              shuttleResult.Outcome,
		ReviewPath:           p.ReviewPath,
		FixerReportPath:      p.FixerReportPath,
		SessionID:            shuttleResult.SessionID,
		StrandGUID:           shuttleResult.StrandGUID,
		LastAssistantMessage: shuttleResult.LastAssistantMessage,
		RunDir:               shuttleResult.RunDir,
	}

	if result.Outcome != shuttleengine.OutcomeDone {
		// asking/died/timeout are normal loop events, not errors — the
		// caller branches on Outcome (and, for asking, LastAssistantMessage
		// above). Verdict stays empty: there is no review file to trust yet.
		return result, nil
	}

	if p.ClusterFan != "" {
		// Copy the audit onto the Result before checking it, so a caller
		// inspecting a policy failure below still gets the raw ForkAudit
		// for diagnosis — the same "populated-so-far Result on a hard
		// error" shape the verdict-parse failure path below uses.
		result.ForkAudit = shuttleResult.ForkAudit
		warnings, err := auditClusterRound(shuttleResult.ForkAudit, len(p.clusterLenses))
		if err != nil {
			return result, err
		}
		result.ClusterWarnings = warnings
	}

	content, err := os.ReadFile(p.ReviewPath)
	if err != nil {
		return result, fmt.Errorf("burler: read review file %q: %w", p.ReviewPath, err)
	}

	verdict, findings, err := ParseReview(content)
	if err != nil {
		return result, fmt.Errorf("burler: round reached done but its review file is invalid: %w", err)
	}

	result.Verdict = verdict
	result.Findings = findings
	return result, nil
}
