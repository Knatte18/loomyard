// engine.go implements the round driver: Engine.Run validates a Profile, composes both halves' prompts, starts the reviewer and the fixer over the Shuttle seam, and joins them through the ready marker (handoff.go) into one Result.
// This is the library's one external entry point — a caller invokes it once per round.
// Today that caller is internal/shedadapters.BurlerProducer, which wraps the call as a Shed row.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The agent-name roles of a round's two halves.
const (
	burlerReviewRole = "burler-review"
	burlerFixRole    = "burler-fix"
)

// burlerSkills are the skills both halves of the round load, in order.
var burlerSkills = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}

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
// burler's round prompts from at call time via stencilstore.Read.
// frictionDir is told rather than derived: burlerengine must not import loomengine, and
// burlerengine.Geometry is internal/hubgeom's/internal/standalonegeom's to construct under the
// Told-Geometry Invariant, so an explicit constructor parameter is the remaining told seam. An empty
// value means Tier 2 is off for this engine.
func New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir, frictionDir string) *Engine {
	return &Engine{shuttle: shuttle, geom: geom, cfg: cfg, stencilsDir: stencilsDir, frictionDir: frictionDir}
}

// Half is one half of a round: the identities, last message and kept run directory a caller needs to act on a non-done outcome further.
type Half struct {
	SessionID            string
	StrandGUID           string
	LastAssistantMessage string
	// RunDir is the kept shuttle run directory a caller surfaces when the half dies or times out,
	// so it can point an operator at the run's artifacts for inspection.
	RunDir string
	// StartError is the text of the start error of a half that never started, which names the run dir, the strand and whether the strand was removed.
	// It is empty for a half that started.
	StartError string
}

// Result is one round's outcome.
// It carries how the deciding half's shuttle run classified (Outcome), the parsed verdict and findings (set only when the reviewer's review was accepted and the fixer finished cleanly), the resolved output paths, and each half's identity.
type Result struct {
	Outcome         shuttleengine.Outcome
	Verdict         Verdict
	Findings        []Finding
	ReviewPath      string
	FixerReportPath string
	// Review and Fix are the reviewer's and the fixer's halves.
	Review Half
	Fix    Half
	// ForkAudit is a 1:1 passthrough of the reviewer's shuttleengine.Result.ForkAudit, set
	// only for a cluster round (Profile.ClusterFan != "") whose reviewer reached
	// shuttleengine.OutcomeDone. nil for a non-cluster round or a
	// non-done reviewer.
	ForkAudit *shuttleengine.ForkAudit
	// ClusterWarnings carries the non-fatal audit findings auditClusterRound
	// returns for a cluster round (e.g. a fork that never returned a
	// report) — sloppiness no mechanism prevents in advance, surfaced here
	// rather than failing the round. Empty for a non-cluster round.
	ClusterWarnings []string
	// Gate is a 1:1 passthrough of the fixer's shuttleengine.Result.Gate.
	// nil means the fixer ran ungated.
	Gate *shuttleengine.GateOutcome
	// NotStarted is true when a half's provider never came up, so a producer can tell that from an agent that died mid-run.
	NotStarted bool
}

// halfSpec is one half's shuttle spec and gate.
type halfSpec struct {
	spec shuttleengine.Spec
	gate shuttleengine.GateSpec
}

// roundHalves builds both halves' specs and gates for p under opts, the prompts being the already-composed orchestrators.
// The reviewer declares the review file alone and is gated by the review-parse entry alone;
// the fixer declares the fixer-report alone and is gated by opts.Gate, every entry wrapped by repairReportBeforeGate.
func (p *Profile) roundHalves(opts RunOpts, reviewPrompt, fixPrompt string) (review, fix halfSpec) {
	review = halfSpec{
		spec: shuttleengine.Spec{
			Prompt:        reviewPrompt,
			OutputFiles:   []string{p.ReviewPath},
			Model:         opts.Review.Model,
			Effort:        opts.Review.Effort,
			Version:       opts.Review.Version,
			Timeout:       opts.Timeout,
			Role:          burlerReviewRole,
			Skills:        burlerSkills,
			Round:         opts.Round,
			ForkSubagents: p.ClusterFan != "",
		},
		gate: shuttleengine.GateSpec{ReviewGateEntry(p.ReviewPath)},
	}

	// A fresh copy, so the caller's slice is never mutated;
	// off entries are wrapped too, since they never run and the wrap is harmless there.
	fixGate := make(shuttleengine.GateSpec, 0, len(opts.Gate))
	for _, entry := range opts.Gate {
		entry.Gate = repairReportBeforeGate(entry.Gate, p.FixerReportPath)
		fixGate = append(fixGate, entry)
	}
	fix = halfSpec{
		spec: shuttleengine.Spec{
			Prompt:      fixPrompt,
			OutputFiles: []string{p.FixerReportPath},
			Model:       opts.Fix.Model,
			Effort:      opts.Fix.Effort,
			Version:     opts.Fix.Version,
			Timeout:     opts.Timeout,
			Role:        burlerFixRole,
			Skills:      burlerSkills,
			Round:       opts.Round,
		},
		gate: fixGate,
	}
	return review, fix
}

// Run drives one burler round for p, tuned by opts.
// Sequence: validate p against the engine's worktree root;
// resolve opts.NoteID against the engine's friction directory and swallow a friction.Directive error
// as a Warn, exactly as if Tier 2 were off (see composePrompt's frictionDirective parameter);
// compose both halves' prompts;
// materialize the four rendered instruction files to a fresh per-round directory under .lyx (via
// lyxdirs.DotLyxDirName) so the orchestrator prompts can name their absolute paths;
// remove any stale ready marker, since no caller carries that duty;
// start the reviewer, then the fixer, through the Shuttle seam, each as an autonomous run (Interactive/Parent/Display/KeepPane stay zero-valued);
// and join them (see join).
//
// A half that never starts because its provider never came up is that half's OutcomeDied with NotStarted set and a nil error.
// StartGated returns only the error there, so the half carries its StartError text and no identity.
// A reviewer that fails to start leaves the fixer never started; a fixer that fails to start stops the already-started reviewer first.
// Any other start error is a pre-strand failure and is returned wrapped, with the already-started reviewer stopped first.
//
// Run returns a nil error for every non-done outcome (died/timeout are normal loop events a
// caller branches on via Result.Outcome, with an empty Verdict) and reserves errors for hard
// failures: an invalid profile, a shuttle start failure, a cluster audit policy violation, a half that cannot be stopped, a fixer that skipped the handoff or changed the review, and
// — deliberately fail-loud — a verdict parse failure on a done review, since a defaulted verdict could
// silently terminate a caller's round loop on a malformed round.
func (e *Engine) Run(p Profile, opts RunOpts) (Result, error) {
	if err := p.validate(e.geom.WorktreeRoot, e.cfg); err != nil {
		return Result{}, err
	}

	logger.Info("burler: round starting", "round", opts.Round, "clusterFan", p.ClusterFan, "forkCount", len(p.clusterLenses), "reviewPath", p.ReviewPath)

	directive, err := pattern.Directive(e.geom.RepoRoot, e.stencilsDir, pattern.RoleReviewFix)
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

	prompts, err := composePrompt(e.stencilsDir, e.geom.ParentName, &p, directive, frictionDirective, roundFilePaths{
		ReviewerExplore: filepath.Join(roundDir, "instruction-1-explore-reviewer.md"),
		FixerExplore:    filepath.Join(roundDir, "instruction-1-explore-fixer.md"),
		Review:          filepath.Join(roundDir, "instruction-2-review.md"),
		Fix:             filepath.Join(roundDir, "instruction-3-fix.md"),
	})
	if err != nil {
		return Result{}, err
	}

	for _, f := range prompts.Files {
		if err := os.WriteFile(f.Path, []byte(f.Content), 0o644); err != nil {
			return Result{}, fmt.Errorf("burler: materialize instruction files: %w", err)
		}
	}

	if err := os.Remove(p.ReadyMarkerPath); err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("burler: remove the stale ready marker %q: %w", p.ReadyMarkerPath, err)
	}

	reviewSpec, fixSpec := p.roundHalves(opts, prompts.Reviewer, prompts.Fixer)

	reviewHandle, err := e.shuttle.StartGated(reviewSpec.spec, reviewSpec.gate)
	if err != nil {
		if errors.Is(err, shuttleengine.ErrNotStarted) {
			return notStartedResult(&p, err, nil), nil
		}
		return Result{}, fmt.Errorf("burler: shuttle run: %w", err)
	}

	fixHandle, err := e.shuttle.StartGated(fixSpec.spec, fixSpec.gate)
	if err != nil {
		if stopErr := e.stopHalf(reviewHandle); stopErr != nil {
			return Result{}, stopErr
		}
		if errors.Is(err, shuttleengine.ErrNotStarted) {
			return notStartedResult(&p, err, reviewHandle), nil
		}
		return Result{}, fmt.Errorf("burler: shuttle run: %w", err)
	}

	return e.join(&p, opts, reviewHandle, fixHandle)
}

// notStartedResult is the round's result when a half's provider never came up: OutcomeDied with NotStarted set.
// With a non-nil reviewHandle the fixer is the half that never started and the reviewer's started identity is kept; otherwise the reviewer never started.
func notStartedResult(p *Profile, startErr error, reviewHandle Handle) Result {
	result := Result{
		Outcome:         shuttleengine.OutcomeDied,
		ReviewPath:      p.ReviewPath,
		FixerReportPath: p.FixerReportPath,
		NotStarted:      true,
	}
	if reviewHandle == nil {
		result.Review.StartError = startErr.Error()
		return result
	}
	result.Review = Half{StrandGUID: reviewHandle.StrandGUID(), RunDir: reviewHandle.RunDir()}
	result.Fix.StartError = startErr.Error()
	return result
}

// repairReportBeforeGate wraps told, a round's own gate closure, in a per-round closure that additionally instructs the fixer to rewrite fixerReportPath when the gate fails.
//
// It exists because the fixer writes its fixer-report BEFORE its gate runs.
// A gate that fails, re-prompts, and then passes would otherwise leave the round trusting a report written against the pre-repair artifact.
// Such a report claims a fix over a state that has since changed, which is exactly what the segment's judge then consumes.
//
// It calls told exactly once.
// On a non-nil error or a passing result it returns that verbatim.
// On a failing result it appends to GateResult.Findings a blank line and an instruction naming fixerReportPath, requiring it to be rewritten to reflect the repair the agent is about to make.
// The instruction rides the findings FILE and never the Send line, which must stay a single line — see the "findings always ride a file" decision.
//
// It is composed in roundHalves, rather than in the closure the caller built, because only the round's profile knows its own fixer-report path.
func repairReportBeforeGate(told shuttleengine.Gate, fixerReportPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		result, err := told()
		if err != nil || result.Passed {
			return result, err
		}
		result.Findings += fmt.Sprintf(
			"\n\nThis round's own fixer report (%s) was written before this gate ran. Rewrite it to reflect the repair you are about to make.",
			fixerReportPath,
		)
		return result, nil
	}
}
