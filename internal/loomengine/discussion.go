// discussion.go implements DiscussionSpec, the discussion producer's Spec factory: a pure composer
// that resolves the discussion role's model, names the two _lyx/discussion/ output files, composes
// the interview prompt, and returns a shuttleengine.Spec ready for shuttle.Run.
// It does no spawning, polling, or filesystem writing itself — shedadapters.SingleLLMProducer drives
// the returned Spec through the shuttle seam, reached from internal/shedrecipe's DiscussionWrite
// registry entry.
//
// With discussion_producer twill the spec is the single writer's with the twill block rendered into the discussion template and fork subagents authorized.
// The block's lenses come from the twill-discussion fan in burler.yaml, and the writer spawns the forks once its record is written and passes the self-check, so they review the record their inherited context holds.
// The spec sets ForkSubagents, so the writer may spawn subagents of type fork, and the webster fork guard is installed whatever the Agent setting.
// While the shuttle config's claude_deny_agent_tool is true, the Agent hook admits fork subagents only and refuses every other subagent type; with it false no Agent hook is installed, so the writer and its forks may spawn any subagent type, as single may under the same setting.
// The hook admits a fork spawn from any session in the run, so a fork spawning a further fork is refused only by the fork boilerplate.
// Forks inherit every other standing deny of the writer's session.
// The writer's write scope and a fork's read-only and no-Agent-call rules are stencil-level, and the discussion gates bound what lands in the record.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// discussionRole is the agent-name role this module's Discussion-Write spawn carries.
const discussionRole = "discussion"

// DiscussionWriterRole returns the role of the session the parent-review gate prompts: the seats chair when discussion_producer is seats, and the single writer otherwise.
// Advisors are out, since they take no turn after the chair's record exists.
func DiscussionWriterRole(cfg Config) string {
	if cfg.DiscussionProducer == DiscussionProducerSeats {
		return seatengine.SeatRole(discussionRole, seatengine.RoleChair)
	}
	return discussionRole
}

// discussionSkills are the skills the Discussion-Write spawn loads, in order.
var discussionSkills = []string{"scribe:prose", "scribe:conversation"}

// DiscussionSpec builds the shuttleengine.Spec for one discussion producer run.
// specsDir is the directory the prompt points the writer at for the record format spec.
// parentName is told, never derived; an empty one renders the no-parent directive.
// dirs is the told config geometry the twill fan's burler.yaml is read through.
// Segment is the discussion segment, because the run is the Discussion-Write phase.
func DiscussionSpec(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir, parentName string, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error) {
	if slug == "" {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: slug must not be empty")
	}

	spec, err := modelspec.Parse(cfg.Discussion)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: discussion role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: discussion role model-spec: %w", err)
	}

	decisionRecordPath := DiscussionDecisionRecord(layout)
	supportLogPath := DiscussionSupportLog(layout)

	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleDesigner)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: pattern directive: %w", err)
	}

	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, !autonomous)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: parent directive: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	notePath := friction.NotePath(frictionDir, "Discussion-Write")
	frictionDirective, err := friction.Directive(notePath, stencilsDir, friction.RoleInterview)
	if err != nil {
		// Unlike the other errors in this function, a friction.Directive error is swallowed, never
		// returned: it is optional bookkeeping, not a binding constraint, so a transient stencil read
		// failure here must never fail the whole Discussion-Write spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "discussion-write", "stencil", "loom-template-discussion", "error", err)
		frictionDirective = ""
	}

	var twillDirective string
	twill := cfg.DiscussionProducer == DiscussionProducerTwill
	if twill {
		twillDirective, err = discussionTwillDirective(layout, dirs, stencilsDir)
		if err != nil {
			return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: %w", err)
		}
	}

	prompt, err := composePrompt(stencilsDir, specsDir, slug, decisionRecordPath, supportLogPath, patternDirective, frictionDirective, parentDirective, twillDirective, autonomous)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:        string(prompt),
		OutputFiles:   []string{decisionRecordPath, supportLogPath},
		Model:         resolved.Model,
		Effort:        resolved.Params["effort"],
		Version:       resolved.Params["version"],
		Interactive:   !autonomous,
		Role:          discussionRole,
		Segment:       segmentcolor.Discussion,
		Skills:        discussionSkills,
		Timeout:       time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
		ForkSubagents: twill,
	}, nil
}
