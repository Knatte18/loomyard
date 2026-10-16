// review.go implements the review half of a round: Engine.Review runs one fresh reviewer session as the chair of a one-seat table and accepts the review it leaves, and ProbeReview and ResumeReview attach to a reviewer a previous session left live.
// The half neither waits on, messages nor knows of the fix half; it hands back the accepted review's digest, which the fix half checks the review against.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// reviewRolePrefix is the role prefix of the review half's one seat; the seats engine forms its strand role from it.
const reviewRolePrefix = "burler-review"

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
	// ForkAudit is a 1:1 passthrough of the reviewer's shuttleengine.Result.ForkAudit, set only for a cluster round whose reviewer reached done.
	ForkAudit *shuttleengine.ForkAudit
	// Lenses names the lenses the reviewer ran, in fan order after exclusion; nil for a solo round.
	Lenses []string
	// ClusterWarnings carries the non-fatal findings of the cluster audit; empty for a non-cluster round.
	ClusterWarnings []string
	// NotStarted is true when the reviewer's provider never came up, so a producer can tell that from an agent that died mid-run.
	NotStarted bool
	// Gate is a 1:1 passthrough of the reviewer's shuttleengine.Result.Gate, the review-parse entry's report; nil when the reviewer never ran gated.
	Gate *shuttleengine.GateOutcome
}

// ErrReviewChanged reports that the review file no longer holds the bytes the review half accepted, so the fix half refuses to act on it.
var ErrReviewChanged = errors.New("burler: the review file changed after it was accepted")

// reviewTable builds the review half's one-seat table for p under opts: the reviewer as the chair, gated by the review-parse entry alone.
// The reviewer writes the review file alone and may fork exactly when p names a cluster fan.
// values carries the marker values the seat's stencil is filled from, and skills the skills the seat loads.
func (p *Profile) reviewTable(opts RunOpts, values map[string]string, skills []string) seatengine.Table {
	return seatengine.Table{
		RolePrefix: p.rolePrefix(reviewRolePrefix),
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
				Skills:        skills,
				Values:        values,
				ForkSubagents: p.ClusterFan != "",
			},
		},
	}
}

// Review runs the review half for p, tuned by opts: one fresh reviewer session that writes the review file.
// It validates p, resolves the PATTERN directive, composes the reviewer's instruction files into a fresh directory under the anchor's ephemeral burler directory and runs the review table.
// A reviewer that never starts because its provider never came up is OutcomeDied with NotStarted set and a nil error.
// A reviewer that died or timed out is its outcome with a nil error.
// A reviewer that is done is accepted through the cluster audit and the strict parse, and any failure of either is returned with the outcome and fork audit set.
// A reviewer that could not be stopped is returned wrapped in ErrHalfNotStopped.
func (e *Engine) Review(p Profile, opts RunOpts) (ReviewResult, error) {
	if err := p.validateReview(e.geom.WorktreeRoot, e.cfg); err != nil {
		return ReviewResult{}, err
	}

	logger.Info("burler: review starting", "round", opts.Round, "clusterFan", p.ClusterFan, "forkCount", len(p.clusterLenses), "reviewPath", p.ReviewPath)

	declared, err := discussionparser.DeclaredPatternChanges(e.geom.PatternRecordPath)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("burler: %w re-step the review row with %q", err, "lyx loom start")
	}
	directive, err := pattern.Directive(e.geom.RepoRoot, e.stencilsDir, pattern.RoleReviewFix, declared)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("burler: %w", err)
	}
	dir, err := e.instructionDir("review")
	if err != nil {
		return ReviewResult{}, err
	}
	prompt, err := composeReviewPrompt(e.stencilsDir, &p, directive, reviewFilesIn(dir))
	if err != nil {
		return ReviewResult{}, err
	}
	if err := writeInstructionFiles(prompt.Files); err != nil {
		return ReviewResult{}, err
	}

	seats, err := e.seats.Run(p.reviewTable(opts, prompt.Values, e.seatSkills(&p)))
	return e.concludeReview(&p, opts, seats, err)
}

// instructionDir creates and returns a fresh instruction directory named from prefix under the anchor's ephemeral burler directory.
func (e *Engine) instructionDir(prefix string) (string, error) {
	burlerDir := filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName, "burler")
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

// probeDirOf returns the directory a probe of a half names its instruction files under; a probe composes no prompt, so the directory is never created.
func (e *Engine) probeDirOf() string {
	return filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName, "burler", "round-probe")
}

// ProbeReview reports the state of the review half p describes under opts, without waiting on it:
// live when the seats engine finds a live reviewer run, even one whose review file is already on disk, as a reviewer repairing its review in the parse gate is;
// done when it has no live run and the review file is present; and gone otherwise.
// A probe error, and an invalid p, are returned as is, a reviewer that could not be stopped wrapped in ErrHalfNotStopped.
func (e *Engine) ProbeReview(p Profile, opts RunOpts) (LiveHalf, error) {
	if err := p.validateReview(e.geom.WorktreeRoot, e.cfg); err != nil {
		return LiveHalf{}, err
	}
	table := p.reviewTable(opts, reviewSeatValues(&p, reviewFilesIn(e.probeDirOf())), e.seatSkills(&p))
	return e.probeHalf(table, p.ReviewPath)
}

// ResumeReview completes the review half whose reviewer is live, attaching to it instead of starting one, and concludes it as a fresh review is.
// A LiveHalf that is not live is an error, and so is one with no probed table, which only a LiveHalf not taken from ProbeReview lacks; nothing is stopped or started.
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

	table := p.reviewTable(opts, reviewSeatValues(&p, reviewFilesIn(e.probeDirOf())), e.seatSkills(&p))
	seats, err := e.seats.Resume(table, *live.table)
	return e.concludeReview(&p, opts, seats, err)
}

// reviewResultOf returns the part of the review half's result every ending shares: the review path, the reviewer's identity and the lenses it ran.
func reviewResultOf(p *Profile, reviewer shuttleengine.Result) ReviewResult {
	result := ReviewResult{
		Outcome:    reviewer.Outcome,
		ReviewPath: p.ReviewPath,
		Review:     halfOf(reviewer, "", ""),
		Gate:       reviewer.Gate,
	}
	for _, lens := range p.clusterLenses {
		result.Lenses = append(result.Lenses, lens.Name)
	}
	return result
}

// concludeReview turns the seats engine's answer, the reviewer's result and the error it ran with, into the review half's one result.
// A reviewer that could not be stopped is returned wrapped in ErrHalfNotStopped.
// One that never started is a not-started result when its provider never came up and a wrapped start error otherwise.
// A reviewer whose wait errored is that error wrapped.
// A reviewer that died or timed out is its outcome with a nil error.
// A done reviewer whose gate failed is an error naming the gate, and one that passed is accepted through acceptReview, whose error is returned with the outcome and fork audit set.
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
	if reviewer.Outcome != shuttleengine.OutcomeDone {
		return result, nil
	}
	if gate := reviewer.Gate; gate != nil && !gate.Passed {
		return result, fmt.Errorf("burler: the reviewer reached done but its review gate failed, so the round has no verdict: %s", gate.Reason)
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
