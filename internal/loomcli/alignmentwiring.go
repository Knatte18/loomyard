// alignmentwiring.go wires the alignment agent into loom's parent review: the Aligner the review gate drives, the settings and decision snapshot it works from, and the switch that turns it off.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// alignmentOff reports whether alignment is off for this run: skip_alignment is set, or the hub has no board dir for the agent to read.
func alignmentOff(location *lyxcwd.Location, cfg loomengine.Config) bool {
	if cfg.SkipAlignment {
		return true
	}
	info, err := os.Stat(fabricengine.BoardDir(location.HubPath))
	return err != nil || !info.IsDir()
}

// alignmentSettings resolves the alignment keys of cfg into the agent's settings.
func alignmentSettings(cfg loomengine.Config, registry modelspec.Registry) (alignment.Settings, error) {
	resolved, err := loomengine.ResolveAlignment(cfg, registry)
	if err != nil {
		return alignment.Settings{}, err
	}
	return alignment.Settings{ModelChoice: resolved.ModelChoice, Timeout: resolved.Timeout}, nil
}

// hubBoardTasks returns the production board read: every task of the hub's board, opened on each call.
func hubBoardTasks(location *lyxcwd.Location) func() ([]boardengine.Task, error) {
	return func() ([]boardengine.Task, error) {
		board, err := boardengine.OpenHub(location.HubPath)
		if err != nil {
			return nil, err
		}
		return board.ListTasksFull()
	}
}

// alignmentSnapshot returns the closure that reads the decision snapshot of a task: the decision record's bytes and the text of every note naming slug, sorted by id.
// A decision record that does not exist yet reads as empty.
func alignmentSnapshot(decisionRecord, slug string, notes func() ([]boardengine.Task, error)) func() (string, error) {
	return func() (string, error) {
		record, err := os.ReadFile(decisionRecord)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("alignment: read the decision record: %w", err)
		}
		tasks, err := notes()
		if err != nil {
			return "", fmt.Errorf("alignment: read the board notes: %w", err)
		}
		tasks = slices.Clone(tasks)
		slices.SortFunc(tasks, func(a, b boardengine.Task) int { return a.ID - b.ID })
		var texts []string
		for _, task := range tasks {
			text := strings.Join([]string{task.Title, task.Brief, task.Body}, "\n")
			if task.Kind == boardengine.KindNote && strings.Contains(text, slug) {
				texts = append(texts, text)
			}
		}
		return alignment.Snapshot(record, texts), nil
	}
}

// parentReviewAligner is the parent-review gate's Aligner over an alignment session.
type parentReviewAligner struct {
	location *lyxcwd.Location
	session  *alignment.Session
	settings alignment.Settings
	snapshot func() (string, error)
	prompt   alignment.PromptInput
	// parentBranch reads the branch the run lands on, from the pair's origin record.
	parentBranch func() (string, error)
}

// newParentReviewAligner returns the gate's Aligner for a run, or nil when alignment is off.
func newParentReviewAligner(location *lyxcwd.Location, runID string, cfg loomengine.Config, registry modelspec.Registry, stencilsDir string, shuttle alignment.Shuttle, notes func() ([]boardengine.Task, error)) (parentreview.Aligner, error) {
	if alignmentOff(location, cfg) {
		return nil, nil
	}
	settings, err := alignmentSettings(cfg, registry)
	if err != nil {
		return nil, err
	}
	pauseGeometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return nil, err
	}
	slug := seedSlug(location.WorktreeName)
	return &parentReviewAligner{
		location: location,
		session: alignment.NewSession(alignment.SessionConfig{
			Root:        shedrun.AlignmentDir(location, runID, string(alignment.PointParentReview)),
			StencilsDir: stencilsDir,
			Shuttle:     shuttle,
			Now:         time.Now,
			Pause:       pauseGeometry,
		}),
		settings:     settings,
		parentBranch: func() (string, error) { return readRecordedParentBranch(location) },
		snapshot:     alignmentSnapshot(loomengine.DiscussionDecisionRecord(location), slug, notes),
		prompt: alignment.PromptInput{
			Point:              alignment.PointParentReview,
			Slug:               slug,
			Worktree:           location.WorktreePath(),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			SupportLogPath:     loomengine.DiscussionSupportLog(location),
			PlanDir:            planparser.PlanDir(location.AnchorPath()),
			MeasureStencil:     alignment.MeasureLoom,
		},
	}, nil
}

// Spawn starts the agent for round, or returns the attempt that already holds the round's key.
func (p *parentReviewAligner) Spawn(round int) (parentreview.Alignment, error) {
	snapshot, err := p.snapshot()
	if err != nil {
		return parentreview.Alignment{}, err
	}
	parentBranch, err := p.parentBranch()
	if err != nil {
		return parentreview.Alignment{}, fmt.Errorf("alignment: read the parent branch: %w", err)
	}
	prompt := p.prompt
	prompt.ParentBranch = parentBranch
	prompt.BriefPath = filepath.Join(loomengine.LoomParentReviewDir(p.location), "round-"+strconv.Itoa(round), "brief.md")
	attempt, err := p.session.Begin(alignment.SpawnInput{
		Key:      "round-" + strconv.Itoa(round),
		Prompt:   prompt,
		Settings: p.settings,
		Snapshot: snapshot,
	})
	if err != nil {
		return parentreview.Alignment{}, err
	}
	return parentreview.Alignment{Attempt: attempt.Dir, Strand: attempt.StrandGUID, SpawnedAt: attempt.SpawnedAt}, nil
}

// Poll maps the session's outcome onto the gate's poll.
// An approve is an escalation when the decision snapshot differs from the one the attempt spawned with.
func (p *parentReviewAligner) Poll(a parentreview.Alignment) (parentreview.AlignmentPoll, error) {
	attempt, err := alignment.ReadAttempt(a.Attempt)
	if err != nil {
		return parentreview.AlignmentPoll{}, err
	}
	outcome, err := p.session.Poll(attempt)
	if err != nil || !outcome.Done {
		return parentreview.AlignmentPoll{}, err
	}
	report := attempt.ReportPath()
	if _, err := os.Stat(report); err != nil {
		report = ""
	}
	if outcome.Cause != "" {
		return parentreview.AlignmentPoll{Done: true, Report: report, Reason: outcome.Cause + ": " + outcome.Detail}, nil
	}
	verdict := outcome.Verdict
	switch verdict.Verdict {
	case alignment.VerdictApprove:
		current, err := p.snapshot()
		if err != nil {
			return parentreview.AlignmentPoll{}, err
		}
		if current != attempt.Snapshot {
			return parentreview.AlignmentPoll{Done: true, Verdict: alignment.VerdictEscalate, Report: report, Reason: alignment.DecisionRecordedReason}, nil
		}
	case alignment.VerdictEscalate:
		return parentreview.AlignmentPoll{Done: true, Verdict: verdict.Verdict, Report: report, Reason: verdict.Question}, nil
	}
	return parentreview.AlignmentPoll{Done: true, Verdict: verdict.Verdict, Report: report, Reason: verdict.Reason}, nil
}

// Close stops the agent of a if it is live and closes its attempt.
func (p *parentReviewAligner) Close(a parentreview.Alignment, reason string) error {
	attempt, err := alignment.ReadAttempt(a.Attempt)
	if err != nil {
		return err
	}
	return p.session.Close(attempt, reason)
}
