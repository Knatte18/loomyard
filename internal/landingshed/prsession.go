// prsession.go keeps the PR-Review row's one interactive agent session: it starts or resumes the session, stops it around the reads that judge the pull request, and builds the spec it starts under.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// prReviewerRole is the agent-name role the PR-Review session's spawn carries.
const prReviewerRole = "pr-reviewer"

// prReviewStencilName is the registered name of the PR-Review session's opening prompt.
const prReviewStencilName = "landing-template-pr-review"

// prReviewerSkills are the skills the PR-Review session loads before its prompt.
var prReviewerSkills = []string{"scribe:prose", "scribe:conversation"}

// prSessions is the seam the PR-Review row starts and stops its interactive session through.
type prSessions interface {
	// StartSession starts one unawaited session and returns once its provider is past startup.
	StartSession(spec shuttleengine.Spec) (shuttleengine.OpenResult, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

var _ prSessions = (*shuttleengine.Runner)(nil)

// sessionStartHalt is a session start that failed for a reason that is not transient, which Call turns into a Stuck verdict with Halt.
type sessionStartHalt struct {
	cause error
}

func (h *sessionStartHalt) Error() string {
	return fmt.Sprintf("the PR-Review session could not be started: %v", h.cause)
}

func (h *sessionStartHalt) Unwrap() error { return h.cause }

// haltReason is the Stuck reason of the failed start.
func (h *sessionStartHalt) haltReason() string {
	return fmt.Sprintf("%s; way forward: run lym loom resume", h.Error())
}

// ensureSession makes sure the run has a live PR-Review session and returns rec with the session's identity.
// A recorded strand that is live is reused; a recorded session id whose strand is not live is resumed; with none recorded a new session starts.
// A transient start failure is an error, and any other is a *sessionStartHalt.
// A failed registration is logged, since the registration only lets a teardown end the pane process.
func (p *PRReview) ensureSession(ctx context.Context, rec pullrequest.Record) (pullrequest.Record, error) {
	pr := p.deps.PullRequest
	if rec.StrandGUID != "" {
		live, err := pr.SessionLive(rec.StrandGUID)
		if err != nil {
			return rec, fmt.Errorf("landingshed: %s: read whether the PR-Review session is live: %w", p.name, err)
		}
		if live {
			return rec, nil
		}
	}

	spec, err := p.sessionSpec(rec)
	if err != nil {
		return rec, err
	}
	spec.ResumeSessionID = rec.SessionID
	result, err := pr.Sessions.StartSession(spec)
	if err != nil {
		if terr := transientFailure(ctx, p.name, "start the PR-Review session", err); terr != nil {
			return rec, terr
		}
		return rec, &sessionStartHalt{cause: err}
	}
	if err := pr.RegisterSession(result.StrandGUID); err != nil {
		logger.Warn("landingshed: could not register the PR-Review session's process, so a teardown will not end it", "row", p.name, "strand", result.StrandGUID, "cause", err)
	}
	rec.StrandGUID, rec.StrandName, rec.SessionID, rec.Engine = result.StrandGUID, result.StrandName, result.SessionID, result.Engine
	err = p.updateRecord(func(r *pullrequest.Record) {
		r.StrandGUID, r.StrandName, r.SessionID, r.Engine = rec.StrandGUID, rec.StrandName, rec.SessionID, rec.Engine
	})
	if err != nil {
		// A session the record does not name could never be found again, and the re-entry would start a second one.
		if stopErr := pr.Sessions.StopStrand(result.StrandGUID); stopErr != nil {
			logger.Warn("landingshed: could not stop the PR-Review session whose record write failed", "row", p.name, "strand", result.StrandGUID, "cause", stopErr)
		}
	}
	return rec, err
}

// sessionSpec builds the spec the PR-Review session starts under for the pull request rec records.
// The prompt carries paths only, never file content.
func (p *PRReview) sessionSpec(rec pullrequest.Record) (shuttleengine.Spec, error) {
	parsed, err := modelspec.Parse(p.deps.Config.PRReviewer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: pr_reviewer model-spec: %w", p.name, err)
	}
	resolved, err := p.deps.Registry.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: pr_reviewer model-spec: %w", p.name, err)
	}

	template, err := stencilstore.Read(p.deps.StencilsDir, prReviewStencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
	}
	directive, err := parentdirective.Directive(p.deps.StencilsDir, p.deps.ParentName, true)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
	}
	editDirective, err := editdirective.Directive(p.deps.StencilsDir)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
	}
	prompt, err := stencil.Fill(template, map[string]string{
		"slug":                     p.deps.Slug,
		"pr_url":                   rec.URL,
		"task_worktree":            p.deps.WorktreeRoot,
		"task_branch":              p.deps.TaskBranch,
		"parent_branch":            p.deps.ParentBranch,
		"description_path":         p.deps.DescriptionPath,
		"run_material":             p.runMaterial(),
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: fill the PR-Review prompt: %w", p.name, err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		ModelChoice: shuttleengine.NewModelChoice(resolved),
		Interactive: true,
		Unawaited:   true,
		DenyPublish: true,
		Role:        prReviewerRole,
		Segment:     segmentcolor.Landing,
		Skills:      languageconfig.AppendSkills(prReviewerSkills, p.deps.LanguageSkills),
	}, nil
}

// runMaterial lists, one path per line, the decision record, plan directory and board entry snapshot the run has on disk.
// The stencil needs a non-empty value, so a run with none of them lists that instead.
func (p *PRReview) runMaterial() string {
	var lines []string
	for _, path := range []string{p.deps.DecisionRecordPath, p.deps.PlanDir, p.deps.PullRequest.BoardEntryPath} {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			lines = append(lines, "- "+path)
		}
	}
	if len(lines) == 0 {
		return "- none"
	}
	return strings.Join(lines, "\n")
}

// stopLiveSession stops the session rec records when its strand is live, and reports whether it stopped one.
func (p *PRReview) stopLiveSession(rec pullrequest.Record) (bool, error) {
	if rec.StrandGUID == "" {
		return false, nil
	}
	pr := p.deps.PullRequest
	live, err := pr.SessionLive(rec.StrandGUID)
	if err != nil {
		return false, fmt.Errorf("landingshed: %s: read whether the PR-Review session is live: %w", p.name, err)
	}
	if !live {
		return false, nil
	}
	if err := pr.Sessions.StopStrand(rec.StrandGUID); err != nil {
		return false, fmt.Errorf("landingshed: %s: stop the PR-Review session: %w", p.name, err)
	}
	return true, nil
}

// stopRecordedSession stops the session the run's record names, and reports whether it stopped one.
// A run with no record has no session.
func (p *PRReview) stopRecordedSession() (bool, error) {
	pr := p.deps.PullRequest
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return false, fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)
	}
	if !found {
		return false, nil
	}
	return p.stopLiveSession(rec)
}

// resumeSession brings the session back after a stop that left the review going.
// A review the operator already ended has no session to bring back.
func (p *PRReview) resumeSession(ctx context.Context) error {
	pr := p.deps.PullRequest
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)
	}
	if !found || rec.ClosedForContinue {
		return nil
	}
	_, err = p.ensureSession(ctx, rec)
	return err
}

// endSession stops the session when the call ends with a verdict or a failure of its own, and leaves it running when the context was cancelled, so the re-call after an interrupt finds it live.
// A failed stop is logged, since the verdict stands whatever it says.
func (p *PRReview) endSession(ctx context.Context, err error) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	if _, stopErr := p.stopRecordedSession(); stopErr != nil {
		logger.Warn("landingshed: could not stop the PR-Review session at the end of the row", "row", p.name, "cause", stopErr)
	}
}
