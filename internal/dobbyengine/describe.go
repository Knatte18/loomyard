// describe.go implements dobby's description rewrite for a beam whose pull request is recorded.
// After a member landing it spawns the describer agent over the beam.
// Once the agent's draft passes as a change description, it records the draft in place of the beam's description.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	// BeamDescribeRole is the agent-name role of the describer agent's strand.
	BeamDescribeRole = "beam-describe"
	// beamDescribeStencilName is the registered name of the describer agent's opening stencil.
	beamDescribeStencilName = "beam-template-describe"
	// noBeamChanges stands in the prompt for the beam's changes when its line holds no commit its parent lacks.
	noBeamChanges = "- none: the beam holds no commit its parent branch lacks"
	// describeStandsWayForward ends each line that reports a rewrite that recorded nothing.
	describeStandsWayForward = "; the recorded description stands, and the next member landing rewrites it again"
)

// BeamDescribeFacts carries what the describer agent's prompt names about the beam.
type BeamDescribeFacts struct {
	// DescriptionPath is the beam's recorded change description, which the agent reads.
	DescriptionPath string
	// Branch is the beam's branch.
	Branch string
	// ParentBranch is the branch the beam lands on.
	ParentBranch string
	// Changes lists the commits on the beam's line that its parent branch lacks, oldest first, one markdown list item each.
	// A member pull request's merge is listed by its number.
	Changes string
}

// BeamDescribeDeps holds the seams of a beam's description rewrite, which the hub wiring sets for a beam's dobby.
// A nil Record leaves the rewrite off.
type BeamDescribeDeps struct {
	// Shuttle starts and stops the describer agent.
	Shuttle AgentShuttle
	// Model is the model the agent runs on, resolved by the hub wiring.
	Model shuttleengine.ModelChoice
	// Timeout is how long the agent may run.
	Timeout time.Duration
	// Facts reads what the agent's prompt names about the beam.
	Facts func() (BeamDescribeFacts, error)
	// Findings lists what keeps the draft at path from passing as a change description, none when it passes.
	Findings func(path string) ([]string, error)
	// Record writes the draft at path over the beam's change description, commits it and updates the recorded pull request.
	// recorded reports whether the description was recorded;
	// an error with recorded set means a step after the record failed, the pull request update among them.
	// An error with recorded unset leaves the recorded description standing.
	Record func(ctx context.Context, path string) (recorded bool, err error)
}

// BeamDescribePromptInput carries what the describer stencil's markers are filled from.
type BeamDescribePromptInput struct {
	// Slug is the beam's slug.
	Slug string
	// DraftPath is the one file the agent writes.
	DraftPath string
	// Parent is the name of the nearest live orch, empty when there is none.
	Parent string
	// Facts are the beam's recorded description, branches and changes.
	Facts BeamDescribeFacts
}

// BeamDescribePrompt composes the describer agent's launch prompt for the engine named engine.
// It reads the describer stencil from stencilsDir at call time and fills its markers from in.
// The parent directive names in.Parent, and the edit directive is the one of engine.
// Empty changes are named as a beam with no commit its parent lacks.
// A stencil read or fill error is returned wrapped with the stencil's name.
func BeamDescribePrompt(stencilsDir, engine string, in BeamDescribePromptInput) (string, error) {
	directive, err := parentdirective.Directive(stencilsDir, in.Parent, false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir, engine)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, beamDescribeStencilName)
	if err != nil {
		return "", fmt.Errorf("dobby: describer prompt: read stencil %q: %w", beamDescribeStencilName, err)
	}
	changes := in.Facts.Changes
	if strings.TrimSpace(changes) == "" {
		changes = noBeamChanges
	}
	filled, err := stencil.Fill(template, map[string]string{
		"slug":                     in.Slug,
		"description_path":         in.Facts.DescriptionPath,
		"draft_path":               in.DraftPath,
		"beam_branch":              in.Facts.Branch,
		"parent_branch":            in.Facts.ParentBranch,
		"changes":                  changes,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("dobby: describer prompt: fill stencil %q: %w", beamDescribeStencilName, err)
	}
	return string(filled), nil
}

// BeamDescribeSpec composes the shuttle spec the describer agent launches from.
// PromptEngine is engine, the engine the prompt was rendered for, which must be the one model resolves to.
// OutputFiles holds draftPath alone.
// ConfigRole is the describer role, whose skills come from agents.yaml (role `describer`).
// Segment is the describe segment, since the agent writes a pull request's description.
func BeamDescribeSpec(prompt, draftPath, engine string, model shuttleengine.ModelChoice, timeout time.Duration) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		ConfigRole:   agentconfig.RoleDescriber,
		OutputFiles:  []string{draftPath},
		ModelChoice:  model,
		Timeout:      timeout,
		Role:         BeamDescribeRole,
		Segment:      segmentcolor.Describe,
	}
}

// describes reports whether this dobby rewrites its beam's description: a beam's dobby whose rewrite seams are set.
func (d *daemon) describes() bool {
	return d.geom.Slug != "" && d.deps.BeamDescribe.Record != nil && d.deps.PullRequestRecorded != nil
}

// describeStep spawns the describer agent over the member landings not yet described, unless one is in flight.
// It also settles the agent in flight, which keeps the loop out of its idle backoff.
// It spawns only while Deps.PullRequestRecorded reads a pull request;
// landings counted while none is recorded are dropped, since no pull request carries the text.
// An unreadable landing record keeps the landings until it reads, and is logged once per episode.
// A spawn that fails drops the landings and tells the nearest live orch why, so the next member landing tries again.
func (d *daemon) describeStep(ctx context.Context, now time.Time) error {
	if !d.describes() {
		return nil
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	if current.Describe != nil {
		waiting, err := d.settleDescribe(ctx, now, *current.Describe)
		d.busy = d.busy || waiting
		return err
	}
	if len(current.LandedSinceDescribe) == 0 {
		return nil
	}
	recorded, err := d.deps.PullRequestRecorded()
	if err != nil {
		d.describePullRequest.Fail("dobbyengine: the beam's landing record is unreadable; the description rewrite waits", "worktree", d.geom.WorktreeName, "error", err)
		return nil
	}
	d.describePullRequest.Recover("dobbyengine: the beam's landing record reads again; the description rewrite goes on", "worktree", d.geom.WorktreeName)
	if !recorded {
		return d.endDescribe(current.LandedSinceDescribe)
	}
	if err := d.spawnDescribe(now, current.LandedSinceDescribe); err != nil {
		return d.endFailedSpawn(ctx, now, current.LandedSinceDescribe, err)
	}
	return nil
}

// endFailedSpawn tells the nearest live orch why a spawn failed with cause, then drops landings.
// The line goes out first, so a post that fails keeps the landings and the next tick tries the spawn again.
// It returns an error only when the line cannot be posted or the landings cannot be dropped.
func (d *daemon) endFailedSpawn(ctx context.Context, now time.Time, landings []string, cause error) error {
	line := fmt.Sprintf("%s: the description rewrite after %s did not start: %v%s", d.geom.Slug, strings.Join(landings, ", "), cause, describeStandsWayForward)
	if err := d.postLine(ctx, line); err != nil {
		return errors.Join(cause, err)
	}
	if err := d.endDescribe(landings); err != nil {
		return errors.Join(cause, err)
	}
	d.logLine(now, "description rewrite did not start: "+oneLine(cause.Error()))
	return nil
}

// postLine sends line to the nearest live orch through the Notice seam and returns the post's failure.
// A nil Notice posts nothing.
func (d *daemon) postLine(ctx context.Context, line string) error {
	if d.deps.Notice == nil {
		return nil
	}
	if err := d.deps.Notice(ctx, line); err != nil {
		return fmt.Errorf("post the description rewrite's line: %w", err)
	}
	return nil
}

// spawnDescribe starts the describer agent over landed and records it in the state record.
// A record that cannot be written ends the agent again, so no unrecorded agent is left running.
func (d *daemon) spawnDescribe(now time.Time, landed []string) error {
	describe := d.deps.BeamDescribe
	facts, err := describe.Facts()
	if err != nil {
		return fmt.Errorf("read the beam's facts for the describer agent: %w", err)
	}
	orch, err := d.deps.NearestOrch()
	if err != nil {
		return fmt.Errorf("read the nearest live orch: %w", err)
	}
	if err := os.MkdirAll(d.geom.DescribeDir, 0o755); err != nil {
		return err
	}
	draftPath := filepath.Join(d.geom.DescribeDir, now.UTC().Format("20060102T150405Z")+".md")
	engine, err := describe.Shuttle.EngineName(describe.Model)
	if err != nil {
		return fmt.Errorf("resolve the describer agent's engine: %w", err)
	}
	prompt, err := BeamDescribePrompt(d.deps.StencilsDir, engine, BeamDescribePromptInput{Slug: d.geom.Slug, DraftPath: draftPath, Parent: orch, Facts: facts})
	if err != nil {
		return err
	}
	guid, err := describe.Shuttle.Start(BeamDescribeSpec(prompt, draftPath, engine, describe.Model, describe.Timeout))
	if err != nil {
		return fmt.Errorf("start the describer agent: %w", err)
	}
	err = UpdateState(d.geom.StatePath, func(s *State) error {
		s.Describe = &DescribeState{StrandGUID: guid, DraftPath: draftPath, Deadline: now.Add(describe.Timeout), Landings: slices.Clone(landed)}
		return nil
	})
	if err != nil {
		d.stopDescribeStrand(guid)
		return err
	}
	logger.Info("dobbyengine: the describer agent started", "worktree", d.geom.WorktreeName, "landings", len(landed), "draft", draftPath)
	d.logLine(now, fmt.Sprintf("description rewrite started after %s: %s", strings.Join(landed, ", "), draftPath))
	return nil
}

// settleDescribe ends the agent in flight once its draft passes or its deadline has passed.
// It reports whether the agent still runs.
// A passing draft is recorded through BeamDescribeDeps.Record.
// A record that fails, and a deadline passed with no passing draft, post one line to the nearest live orch naming why.
// A record that committed the description but not the pull request update names `lym beam pr update` as the way forward.
// Either way the agent's landings leave the landings to describe, so the next member landing rewrites the description.
// A draft that cannot be read is an error while the deadline stands, and counts as no passing draft once it has passed.
func (d *daemon) settleDescribe(ctx context.Context, now time.Time, describe DescribeState) (waiting bool, err error) {
	if describe.OwedLine != "" {
		return d.concludeDescribe(ctx, describe, describe.OwedLine)
	}
	landings := strings.Join(describe.Landings, ", ")
	findings, written, readErr := d.draftFindings(describe.DraftPath)
	if readErr == nil && written && len(findings) == 0 {
		d.stopDescribeStrand(describe.StrandGUID)
		recorded, recordErr := d.deps.BeamDescribe.Record(ctx, describe.DraftPath)
		line := ""
		switch {
		case recordErr == nil:
			d.logLine(now, "description rewritten after "+landings)
		case recorded:
			d.logLine(now, "description recorded, its pull request not updated: "+oneLine(recordErr.Error()))
			line = fmt.Sprintf("%s: the description rewrite after %s: %v; way forward: run `lym beam pr update %s`", d.geom.Slug, landings, recordErr, d.geom.Slug)
		default:
			d.logLine(now, "description rewrite not recorded: "+oneLine(recordErr.Error()))
			line = fmt.Sprintf("%s: the description rewrite after %s was not recorded: %v%s", d.geom.Slug, landings, recordErr, describeStandsWayForward)
		}
		return d.concludeDescribe(ctx, describe, line)
	}
	if now.Before(describe.Deadline) {
		if readErr != nil {
			return true, fmt.Errorf("check the describer agent's draft: %w", readErr)
		}
		return true, nil
	}
	d.stopDescribeStrand(describe.StrandGUID)
	cause := "no draft written"
	switch {
	case readErr != nil:
		cause = "draft unreadable: " + readErr.Error()
	case written:
		cause = "draft findings: " + strings.Join(findings, "; ")
	}
	d.logLine(now, "description rewrite timed out: "+cause)
	return d.concludeDescribe(ctx, describe, fmt.Sprintf("%s: the describer agent gave no valid description after %s by its deadline (%s)%s", d.geom.Slug, landings, cause, describeStandsWayForward))
}

// concludeDescribe posts line to the nearest live orch when it is not empty, then ends the agent and drops its landings.
// The line is stored in the agent's record first and the agent is ended only once the post went out,
// so a post that fails is an error and the next tick posts the stored line without recording again.
func (d *daemon) concludeDescribe(ctx context.Context, describe DescribeState, line string) (waiting bool, err error) {
	if line != "" && describe.OwedLine == "" {
		err := UpdateState(d.geom.StatePath, func(s *State) error {
			if s.Describe != nil {
				s.Describe.OwedLine = line
			}
			return nil
		})
		if err != nil {
			return true, err
		}
	}
	if line != "" {
		if err := d.postLine(ctx, line); err != nil {
			return true, err
		}
	}
	return false, d.endDescribe(describe.Landings)
}

// draftFindings reports whether the draft at path is written and, when it is, what keeps it from passing.
func (d *daemon) draftFindings(path string) (findings []string, written bool, err error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	findings, err = d.deps.BeamDescribe.Findings(path)
	return findings, true, err
}

// endDescribe clears the agent in flight and drops landings from the landings to describe.
// A landing counted after the agent started stays, so it gets a rewrite of its own.
func (d *daemon) endDescribe(landings []string) error {
	return UpdateState(d.geom.StatePath, func(s *State) error {
		s.Describe = nil
		s.LandedSinceDescribe = slices.DeleteFunc(s.LandedSinceDescribe, func(slug string) bool { return slices.Contains(landings, slug) })
		return nil
	})
}

// stopDescribeStrand ends the describer agent's strand;
// a failure is logged, since the agent ends on its own at its timeout.
func (d *daemon) stopDescribeStrand(guid string) {
	if err := d.deps.BeamDescribe.Shuttle.StopStrand(guid); err != nil {
		logger.Warn("dobbyengine: the describer agent's strand did not stop", "worktree", d.geom.WorktreeName, "strand", guid, "error", err)
	}
}
