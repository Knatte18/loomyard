// entries_prreview.go implements the PRReview registry entry: it refuses every config key and builds the PR-Review row over a copy of the run's landing deps.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// prReviewKeyWayForward is the way forward of a PRReview row that carries a config key.
const prReviewKeyWayForward = "way forward: remove the key; a PRReview row takes no config"

// prReviewEntry is the Constructor for the "PRReview" registry row.
// The row takes no config key: whether it opens a pull request is decided in its own call, from the receiving branch's beam.yaml, the landing entry's field and the origin.
// It builds the row over a copy of env.Landing whose GateKey is the row name in lower case.
func prReviewEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, prReviewKeyWayForward)
	}
	deps, err := landingRowDeps("PRReview", row, cfg, env)
	if err != nil {
		return nil, err
	}
	return landingshed.NewPRReview(row.Name, deps), nil
}
