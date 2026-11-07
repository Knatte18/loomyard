// spawnwatchdog.go delegates the post-op watchdog spawn attempt of the reed CLI to `reedengine.SpawnWatchdog`.
// That function owns the daemon's detached spawn mechanism.

package reedcli

import "github.com/Knatte18/loomyard/internal/reedengine"

// ensureWatchdogSpawned starts the watchdog daemon for this CLI's hub through `reedengine.SpawnWatchdog`.
// Under a test binary the spawn is skipped, since os.Executable() there is the test binary itself.
// Re-executing it would run the whole test suite again.
func (c *reedCLI) ensureWatchdogSpawned() {
	reedengine.SpawnWatchdog(c.hubPath, c.eng.TmuxPath(), c.eng.ShellPath(), c.suppressWatchdogSpawn)
}
