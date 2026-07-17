// discussion.go implements DiscussionSpec, the discussion producer's Spec
// factory: a pure composer that resolves the discussion role's model,
// names the two _lyx/discussion/ output files, composes the interview
// prompt, and returns a shuttleengine.Spec ready for shuttle.Run. It does
// no spawning, polling, or filesystem writing itself — the future loom
// phase machine drives the returned Spec through shuttle.Run and reacts to
// its outcome.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// DiscussionSpec builds the shuttleengine.Spec for one discussion producer
// run against layout, using cfg's discussion role model-spec and timeout
// knob, reg to resolve that model-spec, slug for the interview prompt's
// board-read step, and autonomous to select interactive vs auto-mode
// instructions (autonomous sets Interactive to false).
//
// The Resolved→Spec field mapping mirrors builderengine/roles.go's
// documented spawn-site mapping: Spec.Model = resolved.Model, Spec.Effort =
// resolved.Params["effort"], Spec.Version = resolved.Params["version"].
//
// DiscussionSpec does not stat or create the output files: shuttleengine's
// Spec.validate rejects a Spec naming a pre-existing output file, and
// creating the _lyx/discussion/ directory is the discussion agent's own
// write concern (see discussion-template.md's Step 5).
func DiscussionSpec(layout *hubgeometry.Layout, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error) {
	if slug == "" {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: slug must not be empty")
	}

	// Resolve the discussion role's model-spec now, before composing the
	// prompt or naming output paths, so an unknown alias or malformed spec
	// fails loud before anything else about this run is constructed.
	spec, err := modelspec.Parse(cfg.Discussion)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: discussion role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: DiscussionSpec: discussion role model-spec: %w", err)
	}

	decisionRecordPath := layout.DiscussionDecisionRecord()
	supportLogPath := layout.DiscussionSupportLog()

	prompt, err := composePrompt(slug, decisionRecordPath, supportLogPath, autonomous)
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
		Role:        "discussion",
		Timeout:     time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
	}, nil
}
