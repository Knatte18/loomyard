// review.go implements the review half of a round.
// Engine.Review runs one fresh reviewer session as the chair of a one-seat table and accepts the review it leaves.
// ProbeReview and ResumeReview attach to a reviewer a previous session left live.
// The half neither waits on, messages nor knows of the fix half;
// it hands back the accepted review's digest, which the fix half checks the review against.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ReviewRolePrefix is the role prefix of the review half's one seat; the seats engine forms its strand role from it.
const ReviewRolePrefix = "burler-review"

// ReviewChairRole returns the strand role the review half's chair runs under for a profile whose RolePrefix is rolePrefix.
// It is ReviewRolePrefix alone for an empty rolePrefix, and rolePrefix and ReviewRolePrefix joined by a hyphen otherwise.
func ReviewChairRole(rolePrefix string) string {
	return ledRole(rolePrefix, ReviewRolePrefix)
}

// burlerDirName is the directory under the anchor's ephemeral .lym for burler's files.
// It holds burler's instruction files and an assistant's paths.
const burlerDirName = "burler"

// ReviewResult is the review half's outcome.
// Verdict, Findings and Digest are set only when the reviewer's review was accepted.
type ReviewResult struct {
	Outcome  shuttleengine.Outcome
	Verdict  Verdict
	Findings []Finding
	// Digest is the hex SHA-256 of the accepted review's bytes, which the fix half is told to check the review against.
	Digest     string
	ReviewPath string
	Review     Half
	// ForkAudit is the audit of the reviewer's forks, set only for a cluster round whose reviewer reached done.
	ForkAudit *shuttleengine.ForkAudit
	// Lenses names the lenses the reviewer ran, in fan order after exclusion; nil for a solo round.
	Lenses []string
	// ClusterWarnings carries the non-fatal findings of the cluster audit; empty for a non-cluster round.
	ClusterWarnings []string
	// NotStarted is true when the reviewer's provider never came up,
	// so a producer can tell that from an agent that died mid-run.
	NotStarted bool
	// Gate is the report of the review-parse gate entry; nil when the reviewer never ran gated.
	Gate *shuttleengine.GateOutcome
	// SessionStates records how the chair ended, set on every ending even when the audit errs, so a failure can say so.
	SessionStates []shuttleengine.SessionState
	// Assists holds what became of each assistant seat of an assisted round; nil for a round with no assistant.
	// No verdict or finding reads it: the verdict and findings are the chair's review alone.
	Assists []AssistOutcome
}

// ErrReviewChanged reports that the review file no longer holds the bytes the review half accepted, so the fix half refuses to act on it.
var ErrReviewChanged = errors.New("burler: the review file changed after it was accepted")

// reviewTable builds the review half's one-seat table for p under opts: the reviewer as the chair, gated by the review-parse entry alone.
// The reviewer writes the review file alone and may fork exactly when p names a cluster fan.
// values carries the marker values the seat's stencil is filled from, and skills names the skills the seat loads.
func (p *Profile) reviewTable(opts RunOpts, values map[string]string, skills []string) seatengine.Table {
	return seatengine.Table{
		RolePrefix: ReviewChairRole(p.RolePrefix),
		Segment:    p.segment(),
		Gate:       shuttleengine.GateSpec{ReviewGateEntry(p.ReviewPath)},
		Round:      opts.Round,
		Timeout:    opts.Timeout,
		Seats: []seatengine.Seat{
			{
				Stencil:       "burler-template-reviewer",
				ModelChoice:   opts.Review,
				ModelKey:      opts.ReviewKey,
				Outputs:       []string{p.ReviewPath},
				ConfigRole:    agentconfig.RoleBurler,
				Skills:        skills,
				Values:        values,
				ForkSubagents: p.ClusterFan != "",
			},
		},
	}
}

// Review runs the review half for p, tuned by opts: one fresh reviewer session that writes the review file.
// It validates p and resolves the PATTERN directive.
// It composes the reviewer's instruction files into a fresh directory under the anchor's ephemeral burler directory.
// Then it runs the review table.
// When opts names an assistant, the table also holds the assistant seat.
// The assistant's own instruction files are composed for its engine into a directory of their own;
// an assistant whose files cannot be composed does not start, and the round goes on without it.
// A reviewer that never starts because its provider never came up is OutcomeDied with NotStarted set and a nil error.
// A reviewer that died or timed out is its outcome with a nil error.
// A reviewer that is done is accepted through the cluster audit and the strict parse.
// A failure of either is returned with the outcome and fork audit set.
// A reviewer that could not be stopped is returned wrapped in ErrHalfNotStopped.
func (e *Engine) Review(p Profile, opts RunOpts) (ReviewResult, error) {
	if err := p.validateReview(e.geom.WorktreeRoot, e.cfg); err != nil {
		return ReviewResult{}, err
	}

	logger.Info("burler: review starting", "round", opts.Round, "clusterFan", p.ClusterFan, "forkCount", len(p.clusterLenses), "reviewPath", p.ReviewPath)

	declared, err := discussionparser.DeclaredPatternChanges(e.geom.PatternRecordPath)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("burler: %w re-step the review row with %q", err, "lym loom start")
	}
	directive, err := pattern.Directive(e.geom.RepoRoot, e.stencilsDir, pattern.RoleReviewFix, declared)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("burler: %w", err)
	}
	dir, err := e.instructionDir("review")
	if err != nil {
		return ReviewResult{}, err
	}
	chairEngine := e.seatEngine(opts.Review)
	assistRules, err := assistRulesBlock(e.stencilsDir, chairEngine, opts)
	if err != nil {
		return ReviewResult{}, err
	}
	parentRules, err := parentRulesBlock(e.stencilsDir, chairEngine, opts)
	if err != nil {
		return ReviewResult{}, err
	}
	prompt, err := composeReviewPrompt(e.stencilsDir, e.geom.SpecsDir, chairEngine, &p, directive, assistRules, parentRules, reviewFilesIn(dir))
	if err != nil {
		return ReviewResult{}, err
	}
	if err := writeInstructionFiles(prompt.Files); err != nil {
		return ReviewResult{}, err
	}

	var assist *seatengine.Seat
	if opts.assisted() {
		files, skipReason := e.composeAssist(&p, opts, directive)
		assist = e.assistSeat(&p, opts, files, e.seatSkills(&p), skipReason)
	}
	table := e.withParentCoverage(withAssistant(p.reviewTable(opts, prompt.Values, e.seatSkills(&p)), assist), &p, opts)
	seats, err := e.seats.Run(table)
	return e.concludeReview(&p, opts, seats, err)
}

// probeTable returns the review table a probe or resume of p's review builds under opts.
// The assistant seat is included when opts names one, and the parent coverage entry when opts names a parent-review ask.
// No prompt is composed.
func (e *Engine) probeTable(p *Profile, opts RunOpts) seatengine.Table {
	files := reviewFilesIn(e.probeDirOf())
	assist := e.assistSeat(p, opts, files, e.seatSkills(p), "")
	return e.withParentCoverage(withAssistant(p.reviewTable(opts, reviewSeatValues(p, files), e.seatSkills(p)), assist), p, opts)
}

// instructionDir creates a fresh instruction directory named from prefix under the anchor's ephemeral burler directory.
// It returns its path.
func (e *Engine) instructionDir(prefix string) (string, error) {
	burlerDir := filepath.Join(e.geom.AnchorPath, lymdirs.DotLymDirName, burlerDirName)
	if err := os.MkdirAll(burlerDir, 0o755); err != nil {
		logger.Warn("burler: create instruction dir failed", "burlerDir", burlerDir, "error", err)
		return "", fmt.Errorf("burler: materialize instruction files: %w", err)
	}
	dir, err := os.MkdirTemp(burlerDir, prefix+"-")
	if err != nil {
		logger.Warn("burler: create half temp dir failed", "burlerDir", burlerDir, "prefix", prefix, "error", err)
		return "", fmt.Errorf("burler: materialize instruction files: %w", err)
	}
	return dir, nil
}

// writeInstructionFiles writes each composed instruction file to its path.
func writeInstructionFiles(files []instructionFile) error {
	for _, f := range files {
		if err := os.WriteFile(f.Path, []byte(f.Content), 0o644); err != nil {
			return fmt.Errorf("burler: materialize instruction files: %w", err)
		}
	}
	return nil
}

// probeDirOf returns the directory a probe of a half names its instruction files under.
// A probe composes no prompt, so the directory is never created.
func (e *Engine) probeDirOf() string {
	return filepath.Join(e.geom.AnchorPath, lymdirs.DotLymDirName, burlerDirName, "round-probe")
}

// ProbeReview reports the state of the review half p describes under opts, without waiting on it:
// live when the seats engine finds a live reviewer run, even one whose review file is already on disk;
// done when it has no live run and the review file is present; and gone otherwise.
// This includes a reviewer repairing its review in the parse gate.
// The probed table holds the assistant seat opts names,
// so a live assistant is found by its outputs and stopped with the chair when the chair is not live.
// A probe error and an invalid p are returned as is;
// a reviewer that could not be stopped is wrapped in ErrHalfNotStopped.
func (e *Engine) ProbeReview(p Profile, opts RunOpts) (LiveHalf, error) {
	if err := p.validateReview(e.geom.WorktreeRoot, e.cfg); err != nil {
		return LiveHalf{}, err
	}
	return e.probeHalf(e.probeTable(&p, opts), p.ReviewPath)
}

// ResumeReview completes the review half whose reviewer is live.
// It attaches to that reviewer instead of starting one, and then concludes the half as a fresh review does.
// A LiveHalf that is not live is an error, and so is one with no probed table.
// Only a LiveHalf not taken from ProbeReview lacks a probed table.
// Nothing is stopped or started.
func (e *Engine) ResumeReview(p Profile, opts RunOpts, live LiveHalf) (ReviewResult, error) {
	if live.State != HalfLive {
		return ReviewResult{}, fmt.Errorf("burler: resume needs the review half live (state %d)", live.State)
	}
	if live.table == nil {
		return ReviewResult{}, fmt.Errorf("burler: resume needs the table ProbeReview found; way forward: take the LiveHalf from ProbeReview, or run the row again so it starts the review fresh")
	}
	if err := p.validateReview(e.geom.WorktreeRoot, e.cfg); err != nil {
		return ReviewResult{}, err
	}

	logger.Info("burler: resuming a live review", "round", opts.Round, "reviewStrand", live.Handle.StrandGUID())

	seats, err := e.seats.Resume(e.probeTable(&p, opts), *live.table)
	return e.concludeReview(&p, opts, seats, err)
}

// reviewResultOf returns the part of the review half's result that every ending shares.
// It holds the review path, the reviewer's identity and the lenses it ran.
func reviewResultOf(p *Profile, reviewer shuttleengine.Result) ReviewResult {
	result := ReviewResult{
		Outcome:       reviewer.Outcome,
		ReviewPath:    p.ReviewPath,
		Review:        halfOf(reviewer, "", ""),
		Gate:          reviewer.Gate,
		SessionStates: reviewer.SessionStates,
	}
	for _, lens := range p.clusterLenses {
		result.Lenses = append(result.Lenses, lens.Name)
	}
	return result
}

// concludeReview turns the seats engine's answer into the review half's one result.
// It reads the reviewer's result and the error the run returned.
// A reviewer that could not be stopped is returned wrapped in ErrHalfNotStopped.
// A reviewer that never started is a not-started result with a nil error.
// Any other run error is wrapped as a shuttle run error.
// A reviewer that ends without reaching done is its outcome with a nil error.
// A done reviewer whose gate failed returns an error naming the gate and its reason,
// or the failing entry's findings when the failure carries no reason.
// One whose gate passed is accepted through acceptReview, and its error is returned with the result.
// Once the chair and every seat have stopped, the reports of assistants that end done are copied beside the review file.
// Assists says what became of each one.
func (e *Engine) concludeReview(p *Profile, opts RunOpts, seats seatengine.Result, runErr error) (ReviewResult, error) {
	reviewer := seats.Chair
	switch {
	case runErr == nil:
	case errors.Is(runErr, seatengine.ErrSeatNotStopped):
		return reviewResultOf(p, reviewer), fmt.Errorf("%w: %w", ErrHalfNotStopped, runErr)
	case errors.Is(runErr, shuttleengine.ErrNotStarted):
		result := reviewResultOf(p, shuttleengine.Result{Outcome: shuttleengine.OutcomeDied})
		result.Review = Half{StartError: runErr.Error()}
		result.NotStarted = true
		return result, nil
	default:
		return ReviewResult{}, fmt.Errorf("burler: shuttle run: %w", runErr)
	}

	logger.Info("burler: review joined", "round", opts.Round, "reviewOutcome", reviewer.Outcome)

	result := reviewResultOf(p, reviewer)
	result.Assists = keepAssistReports(p.ReviewPath, seats.Advisors, e.assistLensNames(opts.AssistFan))
	if reviewer.Outcome != shuttleengine.OutcomeDone {
		return result, nil
	}
	if gate := reviewer.Gate; gate != nil && !gate.Passed {
		reason := gate.Reason
		if reason == "" {
			reason = gate.Findings
		}
		return result, fmt.Errorf("burler: the reviewer reached done but its review gate failed, so the round has no verdict: %s", reason)
	}

	result.ForkAudit = reviewer.ForkAudit
	accepted, err := acceptReview(p, reviewer)
	if err != nil {
		return result, err
	}
	result.Verdict = accepted.verdict
	result.Findings = accepted.findings
	result.Digest = reviewDigest(accepted.content)
	result.ClusterWarnings = accepted.warnings
	return result, nil
}
