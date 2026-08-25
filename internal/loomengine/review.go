// review.go implements ResolveReview, the review segment's config-to-settings resolver: a pure
// composer that parses and resolves the review role's model-spec and pairs it with the review
// round timeout.
// Unlike DiscussionSpec and PlanSpec, it returns no shuttleengine.Spec -- there is no prompt to
// compose here, because the review segment's prompts are the Bouncer's own stencils, composed
// inside internal/shedadapters at call time. The caller threads ReviewSettings' four values onto
// shedrecipe.Env instead.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// ReviewSettings is the review segment's run-wide model and timeout settings, resolved once from
// Config and threaded onto shedrecipe.Env for every review-segment row to fall back to.
type ReviewSettings struct {
	// Model is the resolved review role's provider-side model string.
	Model string
	// Effort is the resolved review role's "effort" parameter, empty when unset.
	Effort string
	// Version is the resolved review role's "version" parameter, empty when unset.
	Version string
	// Timeout is one review round's shuttle-run deadline, derived from cfg.ReviewTimeoutMin.
	Timeout time.Duration
}

// ResolveReview parses and resolves the review role's model-spec from cfg, and pairs it with the
// review round timeout, returning the ReviewSettings the caller threads onto shedrecipe.Env.
func ResolveReview(cfg Config, reg modelspec.Registry) (ReviewSettings, error) {
	spec, err := modelspec.Parse(cfg.Review)
	if err != nil {
		return ReviewSettings{}, fmt.Errorf("loom: ResolveReview: review role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return ReviewSettings{}, fmt.Errorf("loom: ResolveReview: review role model-spec: %w", err)
	}

	return ReviewSettings{
		Model:   resolved.Model,
		Effort:  resolved.Params["effort"],
		Version: resolved.Params["version"],
		Timeout: time.Duration(cfg.ReviewTimeoutMin) * time.Minute,
	}, nil
}
