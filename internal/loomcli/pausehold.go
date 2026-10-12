// pausehold.go refuses the operator's restart verbs of a loom run while a pause covers its slug.

package loomcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// pausedExtras returns the status keys a live pause entry covering this worktree's slug adds: paused_since (the entry's time, RFC 3339) and paused_by (its scope).
// An uncovered run, a receiver without a location and an unreadable record add nothing; the last logs one Warn, so a status read never fails on the record.
func (c *loomCLI) pausedExtras() map[string]any {
	if c.location == nil {
		return nil
	}
	geometry, err := hubgeom.PauseGeometry(c.location)
	if err != nil {
		logger.Warn("loomcli: pause geometry unreadable, status omits paused_since", "cause", err)
		return nil
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, time.Now())
	if err != nil {
		logger.Warn("loomcli: pause record unreadable, status omits paused_since", "cause", err)
		return nil
	}
	if !coverage.Covered {
		return nil
	}
	return map[string]any{
		"paused_since": coverage.Entry.PausedAt.Format(time.RFC3339),
		"paused_by":    coverage.Entry.Scope,
	}
}

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
