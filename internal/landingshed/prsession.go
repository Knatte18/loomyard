// prsession.go owns the PR-Review row's one interactive agent session.
// It starts or resumes the session and stops it around the reads that judge the pull request.
// It also builds the spec the session starts under.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
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

// prSessions is the seam the PR-Review row starts and stops its interactive session through.
type prSessions interface {
	shuttleengine.EngineNamer
	// StartSession starts one unawaited session and returns once its provider is past startup.
	StartSession(spec shuttleengine.Spec) (shuttleengine.OpenResult, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

var _ prSessions = (*shuttleengine.Runner)(nil)

// sessionStartHalt is a session start that failed for a reason that is not transient, and the row halts on it.
type sessionStartHalt struct {
	cause error
}

func (h *sessionStartHalt) Error() string {
	return fmt.Sprintf("the PR-Review session could not be started: %v", h.cause)
}

func (h *sessionStartHalt) Unwrap() error { return h.cause }

// haltReason is the reason the row reports as stuck for the failed start.
func (h *sessionStartHalt) haltReason() string {
	return fmt.Sprintf("%s; way forward: run lym loom resume", h.Error())
}

// ensureSession makes sure the run has a live PR-Review session and returns rec with the session's identity.
// A recorded strand that is live is reused;
// a recorded session id whose strand is not live is resumed;
// with none recorded, a new session starts.
// A transient start failure is returned as an error, and any other is returned as a start halt.
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

// RenderPRReviewPrompt renders the PR-Review session's opening prompt for the pull request at prURL.
// The edit directive in it is the copy for the named engine, so the prompt suits the engine the session runs on.
// The prompt carries paths only, never file content, except the drift verdict, which no file holds.
func RenderPRReviewPrompt(deps Deps, prURL, engine string) (string, error) {
	template, err := stencilstore.Read(deps.StencilsDir, prReviewStencilName)
	if err != nil {
		return "", err
	}
	directive, err := parentdirective.Directive(deps.StencilsDir, deps.ParentName, true)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(deps.StencilsDir, engine)
	if err != nil {
		return "", err
	}
	verdict, err := deps.driftVerdict()
	if err != nil {
		return "", err
	}
	if verdict == "" {
		verdict = noDriftVerdict
	}
	prompt, err := stencil.Fill(template, map[string]string{
		"drift_verdict":            verdict,
		"slug":                     deps.Slug,
		"pr_url":                   prURL,
		"task_worktree":            deps.WorktreeRoot,
		"task_branch":              deps.TaskBranch,
		"parent_branch":            deps.ParentBranch,
		"description_path":         deps.DescriptionPath,
		"run_material":             deps.runMaterial(),
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("fill the PR-Review prompt: %w", err)
	}
	return string(prompt), nil
}

// sessionSpec builds the spec the PR-Review session starts under for the pull request rec records.
// The prompt is rendered for the engine the session's model choice resolves to.
func (p *PRReview) sessionSpec(rec pullrequest.Record) (shuttleengine.Spec, error) {
	parsed, err := modelspec.Parse(p.deps.Config.PRReviewer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: pr_reviewer model-spec: %w", p.name, err)
	}
	resolved, err := p.deps.Registry.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: pr_reviewer model-spec: %w", p.name, err)
	}
	choice := shuttleengine.NewModelChoice(resolved)
	engine := p.deps.PullRequest.Sessions.EngineName(choice)

	prompt, err := RenderPRReviewPrompt(p.deps, rec.URL, engine)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
	}

	return shuttleengine.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		ModelChoice:  choice,
		Interactive:  true,
		Unawaited:    true,
		DenyPublish:  true,
		Role:         prReviewerRole,
		Segment:      segmentcolor.Landing,
		ConfigRole:   agentconfig.RolePRReviewer,
		Skills:       p.deps.LanguageSkills,
	}, nil
}

// runMaterial lists, one path per line, the decision record and plan directory the run has on disk.
// The stencil needs a non-empty value, so a run with none of them lists that instead.
func (d Deps) runMaterial() string {
	var lines []string
	for _, path := range []string{d.DecisionRecordPath, d.PlanDir} {
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
	if !found || rec.ReviewEnded {
		return nil
	}
	_, err = p.ensureSession(ctx, rec)
	return err
}

// endSession stops the session when the call ends with a verdict or a failure of its own.
// It leaves the session running when the context was cancelled,
// so the re-call after an interrupt finds it live.
// A failed stop is logged, since the verdict stands whatever it says.
func (p *PRReview) endSession(ctx context.Context, err error) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	if _, stopErr := p.stopRecordedSession(); stopErr != nil {
		logger.Warn("landingshed: could not stop the PR-Review session at the end of the row", "row", p.name, "cause", stopErr)
	}
}
