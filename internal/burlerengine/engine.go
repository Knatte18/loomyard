// engine.go implements the round driver: Engine.Run validates a Profile, composes the round's seat values and instruction files, runs the round as one seat table on the seats engine (the fixer as chair, the reviewer as required advisor-1), and decides the round's one Result from the seat result (handoff.go).
// This is the library's one external entry point — a caller invokes it once per round.
// Today that caller is internal/shedadapters.BurlerProducer, which wraps the call as a Shed row.

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// burlerRolePrefix is the role prefix of the round's two seats; the seats engine forms their strand roles from it.
const burlerRolePrefix = "burler"

// burlerSkills are the skills both seats of the round load, in order.
var burlerSkills = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}

// Engine drives burler rounds through a seat engine, resolving Profile paths against geom.WorktreeRoot
// and Profile.ClusterFan against cfg's lens/fan library.
type Engine struct {
	seats       *seatengine.Engine
	geom        Geometry
	cfg         Config
	stencilsDir string
	frictionDir string
}

// New returns an Engine ready to run rounds, resolving relative Profile paths
// against geom.WorktreeRoot and any Profile.ClusterFan against cfg (the burler.yaml lens/fan
// library, loaded via LoadConfig).
// It builds its own seat engine over shuttle, told geom's paths and names and stencilsDir.
// geom is the told geometry the caller supplies (hubgeom.BurlerGeometry in hub mode).
// stencilsDir is the absolute stencils directory (see fabricengine.StencilsDir) the round's seat stencils and instruction stencils are read from at call time via stencilstore.Read.
// frictionDir is told rather than derived: burlerengine must not import loomengine, and
// burlerengine.Geometry is internal/hubgeom's/internal/standalonegeom's to construct under the
// Told-Geometry Invariant, so an explicit constructor parameter is the remaining told seam. An empty
// value means Tier 2 is off for this engine.
func New(shuttle seatengine.Shuttle, geom Geometry, cfg Config, stencilsDir, frictionDir string) *Engine {
	seats := seatengine.New(shuttle, seatengine.Geometry{
		WorktreeRoot: geom.WorktreeRoot,
		AnchorPath:   geom.AnchorPath,
		StencilsDir:  stencilsDir,
		ParentName:   geom.ParentName,
		Shortname:    geom.Shortname,
		Slug:         geom.Slug,
	})
	return &Engine{seats: seats, geom: geom, cfg: cfg, stencilsDir: stencilsDir, frictionDir: frictionDir}
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
	// StartedAt and EndedAt are the half's shuttle run times, zero for a half that produced no terminal result.
	StartedAt time.Time
	EndedAt   time.Time
	// Usage is the half's session token reading, forks included, unknown unless the half's run was done on an engine that reads usage.
	Usage shuttleengine.SessionUsage
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
	// Lenses names the lenses the reviewer ran, in fan order after exclusion; nil for a solo round.
	Lenses []string
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

// roundTable builds the round's seat table for p under opts: the fixer as the chair and the reviewer as the required advisor-1, whose end is judged by accept.
// The chair reads the review file, writes the fixer-report, never forks, and is gated by opts.Gate, every entry wrapped by repairReportBeforeGate;
// the reviewer writes the review file alone, is gated by the review-parse entry alone, and may fork exactly when p names a cluster fan.
// prompts carries the marker values each seat's stencil is filled from.
func (p *Profile) roundTable(opts RunOpts, prompts roundPrompts, accept func(shuttleengine.Result) error) seatengine.Table {
	// A fresh copy, so the caller's slice is never mutated;
	// off entries are wrapped too, since they never run and the wrap is harmless there.
	fixGate := make(shuttleengine.GateSpec, 0, len(opts.Gate))
	for _, entry := range opts.Gate {
		entry.Gate = repairReportBeforeGate(entry.Gate, p.FixerReportPath)
		fixGate = append(fixGate, entry)
	}
	return seatengine.Table{
		RolePrefix: burlerRolePrefix,
		Segment:    segmentcolor.Review,
		Gate:       fixGate,
		Round:      opts.Round,
		Timeout:    opts.Timeout,
		Seats: []seatengine.Seat{
			{
				Name:    seatengine.RoleChair,
				Stencil: "burler-template-fix-orchestrator",
				Model:   opts.Fix.Model,
				Effort:  opts.Fix.Effort,
				Version: opts.Fix.Version,
				Inputs:  []string{p.ReviewPath},
				Outputs: []string{p.FixerReportPath},
				Skills:  burlerSkills,
				Values:  prompts.FixerValues,
			},
			{
				Name:          seatengine.AdvisorName(1),
				Stencil:       "burler-template-review-orchestrator",
				Model:         opts.Review.Model,
				Effort:        opts.Review.Effort,
				Version:       opts.Review.Version,
				Outputs:       []string{p.ReviewPath},
				Skills:        burlerSkills,
				Values:        prompts.ReviewerValues,
				Gate:          shuttleengine.GateSpec{ReviewGateEntry(p.ReviewPath)},
				ForkSubagents: p.ClusterFan != "",
				Required:      true,
				Accept:        accept,
			},
		},
	}
}

// Run drives one burler round for p, tuned by opts.
// Sequence: validate p against the engine's worktree root;
// resolve opts.NoteID against the engine's friction directory and swallow a friction.Directive error
// as a Warn, exactly as if Tier 2 were off (see composePrompt's frictionDirective parameter);
// compose both seats' marker values and the four instruction files;
// materialize the four rendered instruction files to a fresh per-round directory under .lyx (via
// lyxdirs.DotLyxDirName) so the seat stencils can name their absolute paths;
// remove any stale ready marker before anything starts, since no caller carries that duty and the fixer starts first;
// run the round's seat table on the seats engine, the fixer as chair and the reviewer as required advisor-1;
// and decide the round from the seat result (see decideRound).
//
// A seat that never starts because its provider never came up is that half's OutcomeDied with NotStarted set and a nil error.
// The seats engine returns only the error there, so the half carries its StartError text and no identity.
// A fixer that fails to start starts no reviewer; a reviewer that fails to start has the fixer stopped by the seats engine.
// Any other chair start error is a pre-strand failure and is returned wrapped.
// A seat that cannot be stopped is returned wrapped in ErrHalfNotStopped.
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

	prompts, err := composePrompt(e.stencilsDir, &p, directive, frictionDirective, roundFilesIn(roundDir))
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

	acceptance := &roundAcceptance{profile: &p}
	seats, err := e.seats.Run(p.roundTable(opts, prompts, acceptance.accept))
	return e.concludeRound(&p, opts, seats, acceptance, err)
}

// notStartedResult is the round's result when a half's provider never came up: OutcomeDied with NotStarted set.
// review and fix are the two halves as they stand, the one that never started carrying its StartError.
func notStartedResult(p *Profile, review, fix Half) Result {
	return Result{
		Outcome:         shuttleengine.OutcomeDied,
		ReviewPath:      p.ReviewPath,
		FixerReportPath: p.FixerReportPath,
		Review:          review,
		Fix:             fix,
		NotStarted:      true,
	}
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
// It is composed in roundTable, rather than in the closure the caller built, because only the round's profile knows its own fixer-report path.
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
