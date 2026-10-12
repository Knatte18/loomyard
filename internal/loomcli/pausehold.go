// pausehold.go refuses the operator's restart verbs of a loom run while a pause covers its slug.

package loomcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// refuseCovered returns the refusal naming "lyx resume" and the retry when a live pause entry covers this worktree's slug, and nil otherwise.
// A geometry or record that cannot be read refuses nothing and logs one Warn, so a corrupt file never blocks a restart.
func (c *loomCLI) refuseCovered(retry string) error {
	geometry, err := hubgeom.PauseGeometry(c.location)
	if err != nil {
		logger.Warn("loomcli: pause geometry unreadable, starting despite a possible pause", "cause", err)
		return nil
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, time.Now())
	if err != nil {
		logger.Warn("loomcli: pause record unreadable, starting despite a possible pause", "cause", err)
		return nil
	}
	if !coverage.Covered {
		return nil
	}
	return pauserecord.CoveredError(coverage.Entry, geometry.Slug, retry)
}
