// innerrun_pause.go makes InnerRun's wait read the hub's pause record: the per-check coverage reading, the arm hold and the spawn hold.

package battenshed

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// coverage reads deps.Pause into w.pause and returns it.
// An error reads as not covered and is warned about once per distinct message, since a corrupt record must never freeze a run.
func (p *innerRunProducer) coverage(w *childWait) pauserecord.Coverage {
	coverage, err := p.deps.Pause()
	if err != nil {
		if msg := err.Error(); msg != w.coverageWarned {
			w.coverageWarned = msg
			logger.Warn("battenshed: could not read the pause record; treating the run as not paused", "producer", p.name, "slug", p.slug, "error", err)
		}
		coverage = pauserecord.Coverage{}
	} else {
		w.coverageWarned = ""
	}
	w.pause = coverage
	return coverage
}

// pauseHoldReason is the wait reason of a run a pause covers.
func pauseHoldReason(entry pauserecord.Entry) string {
	return fmt.Sprintf("held by the pause of %s since %s", entry.Scope, entry.PausedAt.Format(time.RFC3339))
}

// runArm runs the arm's step, or reports the pause hold and runs nothing while w.pause is covered.
// So no resume spawn, stale-base resume, Publish retry or driver revive happens under a pause.
func (p *innerRunProducer) runArm(ctx context.Context, w *childWait, step armStep) *waitEnd {
	if w.pause.Covered {
		p.reportState(w, pauseHoldReason(w.pause.Entry))
		return nil
	}
	return step(ctx, w)
}

// holdSpawnWhileCovered waits, re-reading coverage every poll interval, while the pause record covers the slug, so a covered child is never started.
// Batten's own pause flag and a cancelled context still end the hold.
func (p *innerRunProducer) holdSpawnWhileCovered(ctx context.Context) *waitEnd {
	w := &childWait{}
	for p.coverage(w).Covered {
		p.reportState(w, pauseHoldReason(w.pause.Entry))
		p.deps.Sleep(ctx, p.pollInterval)
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		if p.pauseRequested(w) {
			return p.exemptEnd("pause requested at " + p.deps.Now().Format(time.RFC3339))
		}
	}
	return nil
}
