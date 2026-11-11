// testmap.go implements the prime dobby's test-map refresh step, which starts `lym gate testmap` every few landings.

package dobbyengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// testMapStep starts a test-map refresh once test_map_every landings were counted since the last one, 0 meaning never.
// The refresh plans every map at the prime's HEAD and measures at most test_map_batch of the packages it queues,
// and the count starts again from zero.
// While a refresh is live, one an earlier dobby started included, it starts none and keeps the count,
// so the refresh starts on the first tick after the live one ended.
// A start that fails keeps the count too, and the next tick tries again.
// While test_map_every is 0 it zeroes the count.
// Enabling the key later therefore starts no refresh on landings counted while it was off.
func (d *daemon) testMapStep(now time.Time) error {
	if d.deps.StartTestMap == nil || d.deps.TestMapRunning == nil {
		return nil
	}
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("load beam.yaml: %w", err)
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	landings := current.LandingsSinceTestMap
	if cfg.TestMapEvery == 0 {
		if landings == 0 {
			return nil
		}
		return UpdateState(d.geom.StatePath, func(s *State) error {
			s.LandingsSinceTestMap = 0
			return nil
		})
	}
	if landings < cfg.TestMapEvery {
		return nil
	}
	running, err := d.deps.TestMapRunning()
	if err != nil {
		return fmt.Errorf("read whether a test-map refresh runs: %w", err)
	}
	if running {
		return nil
	}
	if err := d.deps.StartTestMap(cfg.TestMapBatch); err != nil {
		return fmt.Errorf("start the test-map refresh: %w", err)
	}
	logger.Info("dobbyengine: the test-map refresh started", "worktree", d.geom.WorktreeName, "landings", landings, "max", cfg.TestMapBatch)
	d.logLine(now, fmt.Sprintf("test-map refresh started after %d landings, at most %d packages", landings, cfg.TestMapBatch))
	return UpdateState(d.geom.StatePath, func(s *State) error {
		s.LandingsSinceTestMap = 0
		return nil
	})
}
