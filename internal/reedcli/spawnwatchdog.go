// spawnwatchdog.go delegates this CLI's post-op watchdog spawn attempt to
// internal/reedengine.SpawnWatchdog, the owner of the daemon's detached spawn mechanism.

package reedcli

import "github.com/Knatte18/loomyard/internal/reedengine"

// ensureWatchdogSpawned reaches reedengine.SpawnWatchdog with this CLI's three inputs: c.hubPath,
// c.eng.TmuxPath(), and c.suppressWatchdogSpawn (this CLI's own testing.Testing()-derived guard
// against re-exec'ing os.Executable() from a test binary).
func (c *reedCLI) ensureWatchdogSpawned() {
	reedengine.SpawnWatchdog(c.hubPath, c.eng.TmuxPath(), c.suppressWatchdogSpawn)
}
