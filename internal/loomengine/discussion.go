// discussion.go implements DiscussionSpec, the discussion producer's Spec factory: a pure composer
// that resolves the discussion role's model, names the two _lyx/discussion/ output files, composes
// the interview prompt, and returns a shuttleengine.Spec ready for shuttle.Run.
// It does no spawning, polling, or filesystem writing itself — shedadapters.SingleLLMProducer drives
// the returned Spec through the shuttle seam, reached from internal/shedrecipe's DiscussionWrite
// registry entry.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// discussionRole is the agent-name role this module's Discussion-Write spawn carries.
const discussionRole = "discussion"

// discussionSkills are the skills the Discussion-Write spawn loads, in order.
var discussionSkills = []string{"scribe:prose", "scribe:conversation"}

// DiscussionSpec builds the shuttleengine.Spec for one discussion producer run.
// parentName is told, never derived; an empty one renders the no-parent directive.
// Segment is the discussion segment, because the run is the Discussion-Write phase.
func DiscussionSpec(layout *lyxcwd.Location, stencilsDir, parentName string, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error) {
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

	prompt, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, patternDirective, frictionDirective, parentDirective, autonomous)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{decisionRecordPath, supportLogPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: !autonomous,
		Role:        discussionRole,
		Segment:     segmentcolor.Discussion,
		Skills:      discussionSkills,
		Timeout:     time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
	}, nil
}
