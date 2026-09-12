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
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// DiscussionSpec builds the shuttleengine.Spec for one discussion producer run.
func DiscussionSpec(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error) {
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

	prompt, err := composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath, frictionDirective, autonomous)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: %w", err)
	}

	// Interactive and AwaitOperator are both set from the same !autonomous
	// expression yet remain two fields: Interactive means "an operator is
	// present" and governs launch flags and the AskUserQuestion recording
	// hook, while AwaitOperator means "wait for the operator rather than
	// reporting back" and governs the wait loop. loom's interactive
	// Discussion-Write deliberately wants both, dropping the real-time
	// asking signal Interactive alone would add, since the whole point here
	// is to wait rather than report back.
	return shuttleengine.Spec{
		Prompt:        string(prompt),
		OutputFiles:   []string{decisionRecordPath, supportLogPath},
		Model:         resolved.Model,
		Effort:        resolved.Params["effort"],
		Version:       resolved.Params["version"],
		Interactive:   !autonomous,
		AwaitOperator: !autonomous,
		Role:          "discussion",
		Timeout:       time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
	}, nil
}
