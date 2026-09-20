// engine.go implements the round driver: Engine.Run validates a Profile, composes its prompt,
// drives one shuttle run over the Shuttle seam, and maps the shuttle's outcome (plus, on done, the
// parsed review file) into a Result.
// This is the library's one external entry point — a caller invokes it once per round. Today that
// caller is internal/shedadapters.BurlerProducer, which wraps the call as a Shed row.

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Shuttle is the seam Engine drives one round through.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
	// RunGated is Run, gated: the run's declared output artifacts are additionally validated by
	// gate.Gate (if non-nil) before the round's report is trusted. Added beside Run rather than a
	// widening of it, per the "added forms, never widened signatures" decision.
	RunGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, error)
}

var _ Shuttle = (*shuttleengine.Runner)(nil)

// Engine drives burler rounds through a Shuttle, resolving Profile paths against geom.WorktreeRoot
// and Profile.ClusterFan against cfg's lens/fan library.
type Engine struct {
	shuttle     Shuttle
	geom        Geometry
	cfg         Config
	stencilsDir string
	frictionDir string
}

// New returns an Engine ready to run rounds against shuttle, resolving relative Profile paths
// against geom.WorktreeRoot and any Profile.ClusterFan against cfg (the burler.yaml lens/fan
// library, loaded via LoadConfig).
// geom is the told geometry the caller supplies (hubgeom.BurlerGeometry in hub mode).
// stencilsDir is the absolute stencils directory (see fabricengine.StencilsDir) composePrompt reads
// burler's four round prompts from at call time via stencilstore.Read.
// frictionDir is told rather than derived: burlerengine must not import loomengine, and
// burlerengine.Geometry is internal/hubgeom's/internal/standalonegeom's to construct under the
// Told-Geometry Invariant, so an explicit constructor parameter is the remaining told seam. An empty
// value means Tier 2 is off for this engine.
func New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir, frictionDir string) *Engine {
	return &Engine{shuttle: shuttle, geom: geom, cfg: cfg, stencilsDir: stencilsDir, frictionDir: frictionDir}
}

// Result is one round's outcome: how the shuttle run classified (Outcome), the parsed verdict and
// findings (set only when Outcome is shuttleengine.OutcomeDone and the review file parses cleanly),
// the resolved output paths, and the identities/last-message/run-dir a caller needs to act on a
// non-done outcome further.
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
	// out, so it can point an operator (or the caller's own error message) at
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
	// Gate is a 1:1 passthrough of shuttleengine.Result.Gate, exactly as RunDir and ForkAudit
	// already are. nil means the round ran ungated.
	Gate *shuttleengine.GateOutcome
}

// Run drives one burler round for p, tuned by opts.
// Sequence: validate p against the engine's worktree root;
// resolve opts.NoteID against the engine's friction directory and swallow a friction.Directive error
// as a Warn, exactly as if Tier 2 were off (see composePrompt's frictionDirective parameter);
// compose its prompt;
// materialize the three rendered instruction files to a fresh per-round directory under .lyx (via
// lyxdirs.DotLyxDirName) so the orchestrator prompt can name their absolute paths;
// build the shuttle Spec (Interactive/Parent/Display/ KeepPane stay zero-valued — rounds are
// autonomous by default, per the run-tuning-off-profile decision) with Prompt set to the thin
// orchestrator only;
// run it through the Shuttle seam via RunGated, wrapping a non-nil opts.Gate.Gate in
// repairReportBeforeGate so a failing gate's findings also instruct the agent to rewrite this
// round's own review and fixer-report files;
// populate Result (including its 1:1 Gate passthrough) from the shuttle Result;
// when the run reached done with a non-nil, failing Result.Gate, return immediately with Verdict and
// Findings left empty — the round's review file was written before the gate ran, so a gate that
// failed leaves it describing a fix over an artifact state that has since been proven invalid, and a
// caller parsing it would be trusting a report the gate itself just discredited;
// for a cluster round (p.ClusterFan != "") that reached done with a passing (or absent) gate, copy
// the shuttle's ForkAudit onto Result and enforce the cluster audit policy (auditClusterRound)
// before reading the review file at all;
// and, only then, read and strictly parse the review file into Verdict/Findings.
//
// Run returns a nil error for every non-done outcome (asking/died/timeout are normal loop events a
// caller branches on via Result.Outcome, with an empty Verdict) and reserves errors for hard
// failures: an invalid profile, a shuttle start/run failure, a cluster audit policy violation, and
// — deliberately fail-loud — a verdict parse failure on a done run, since a defaulted verdict could
// silently terminate a caller's round loop on a malformed round.
func (e *Engine) Run(p Profile, opts RunOpts) (Result, error) {
	if err := p.validate(e.geom.WorktreeRoot, e.cfg); err != nil {
		return Result{}, err
	}

	logger.Info("burler: round starting", "round", opts.Round, "clusterFan", p.ClusterFan, "forkCount", len(p.clusterLenses), "reviewPath", p.ReviewPath)

	directive, err := pattern.Directive(e.geom.AnchorPath, e.stencilsDir, pattern.RoleReviewFix)
	if err != nil {
		return Result{}, fmt.Errorf("burler: %w", err)
	}

	// Unlike the pattern.Directive call immediately above, a friction.Directive error is swallowed,
	// never returned: it is optional bookkeeping, not a binding constraint, so a transient stencil
	// read failure here must never kill the whole round.
	notePath := friction.NotePath(e.frictionDir, opts.NoteID)
	frictionDirective, err := friction.Directive(notePath, e.stencilsDir, friction.RoleReviewFix)
	if err != nil {
		logger.Warn("burler: friction directive failed, continuing without one", "role", "review-fix", "stencil", "burler-step-1-explore", "error", err)
		frictionDirective = ""
	}

	// AnchorPath-anchored so this per-round instruction dir is a directory
	// sibling of the durable, fabric-synced _lyx tree, not a second
	// WorktreePath-rooted .lyx.
	burlerDir := filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName, "burler")
	if err := os.MkdirAll(burlerDir, 0o755); err != nil {
		logger.Warn("burler: create instruction dir failed", "burlerDir", burlerDir, "round", opts.Round, "error", err)
		return Result{}, fmt.Errorf("burler: materialize instruction files: %w", err)
	}
	roundDir, err := os.MkdirTemp(burlerDir, "round-")
	if err != nil {
		logger.Warn("burler: create round temp dir failed", "burlerDir", burlerDir, "round", opts.Round, "error", err)
		return Result{}, fmt.Errorf("burler: materialize instruction files: %w", err)
	}

	inst1Path := filepath.Join(roundDir, "instruction-1-explore.md")
	inst2Path := filepath.Join(roundDir, "instruction-2-review.md")
	inst3Path := filepath.Join(roundDir, "instruction-3-fix.md")

	prompt, files, err := composePrompt(e.stencilsDir, &p, directive, frictionDirective, inst1Path, inst2Path, inst3Path)
	if err != nil {
		return Result{}, err
	}

	for _, f := range files {
		if err := os.WriteFile(f.Path, []byte(f.Content), 0o644); err != nil {
			return Result{}, fmt.Errorf("burler: materialize instruction files: %w", err)
		}
	}

	spec := shuttleengine.Spec{
		Prompt:        prompt,
		OutputFiles:   []string{p.ReviewPath, p.FixerReportPath},
		Model:         opts.Model,
		Effort:        opts.Effort,
		Timeout:       opts.Timeout,
		Role:          "burler",
		Round:         opts.Round,
		ForkSubagents: p.ClusterFan != "",
	}

	gateSpec := opts.Gate
	if gateSpec.Gate != nil {
		gateSpec.Gate = repairReportBeforeGate(opts.Gate.Gate, p.ReviewPath, p.FixerReportPath)
	}

	shuttleResult, err := e.shuttle.RunGated(spec, gateSpec)
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
		Gate:                 shuttleResult.Gate,
	}

	if result.Outcome != shuttleengine.OutcomeDone {
		// asking/died/timeout are normal loop events, not errors — the
		// caller branches on Outcome (and, for asking, LastAssistantMessage
		// above). Verdict stays empty: there is no review file to trust yet.
		return result, nil
	}

	if result.Gate != nil && !result.Gate.Passed {
		// A failed gate is not an error and not a synthesised Outcome — the round genuinely
		// classified OutcomeDone and the gate is a separate fact about it. A burler round writes
		// both its review file and its fixer report BEFORE its gate runs, so a gate that fails,
		// re-prompts, and then passes would otherwise leave this function parsing a verdict written
		// against the pre-repair artifact — a report claiming a fix over a state that has since
		// changed. Verdict/Findings are left empty and the review file is never read.
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

// repairReportBeforeGate wraps told, a round's own gate closure, in a per-round closure that
// additionally instructs the agent to rewrite reviewPath and fixerReportPath when the gate fails.
//
// It exists because a burler round writes both of those files BEFORE its gate runs, so a gate that
// fails, re-prompts, and then passes would otherwise leave Engine.Run parsing a verdict written
// against the pre-repair artifact — a report claiming a fix over a state that has since changed,
// which is exactly what the segment's judge then consumes.
//
// It calls told exactly once; on a non-nil error or a passing result it returns that verbatim; on a
// failing result it appends to GateResult.Findings a blank line and an instruction naming reviewPath
// and fixerReportPath, requiring both to be rewritten to reflect the repair the agent is about to
// make. The instruction rides the findings FILE and never the Send line, which must stay a single
// line — see the "findings always ride a file" decision.
//
// It is composed here, in Engine.Run, rather than in the closure the caller built, because only
// Engine.Run knows the round's own two paths.
func repairReportBeforeGate(told shuttleengine.Gate, reviewPath, fixerReportPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		result, err := told()
		if err != nil || result.Passed {
			return result, err
		}
		result.Findings += fmt.Sprintf(
			"\n\nBoth this round's own review file (%s) and its own fixer report (%s) were written before this gate ran. Rewrite both to reflect the repair you are about to make.",
			reviewPath, fixerReportPath,
		)
		return result, nil
	}
}
