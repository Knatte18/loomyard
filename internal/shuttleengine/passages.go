// passages.go holds the process-wide seam through which standing instruction passages reach an engine's launch.

package shuttleengine

import (
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
)

// PassageReader returns the standing passages a hub's Bolt checkout holds for a session of engine whose strand role is role, in the order they are appended.
type PassageReader func(boltDir, engine, role string) ([]string, error)

var installedPassageReader atomic.Pointer[PassageReader]

// InstallPassageReader stores r as the process's passage reader and returns a function that reinstates the previous one.
func InstallPassageReader(r PassageReader) (restore func()) {
	previous := installedPassageReader.Swap(&r)
	return func() { installedPassageReader.Store(previous) }
}

// StandingPassages returns the passages the installed reader finds for a session of engine and strand role role in the hub whose Bolt checkout is boltDir.
// It returns none for an empty boltDir (a session outside a hub) or with no reader installed.
// A reader error also returns none and is logged at Warn, so an unreadable store never blocks a launch.
func StandingPassages(boltDir, engine, role string) []string {
	if boltDir == "" {
		return nil
	}
	reader := installedPassageReader.Load()
	if reader == nil {
		return nil
	}
	passages, err := (*reader)(boltDir, engine, role)
	if err != nil {
		logger.Warn("shuttleengine: the standing passages are unreadable, so the launch carries lym's own sentences only; run `lym heddle list` to inspect the store", "engine", engine, "error", err)
		return nil
	}
	return passages
}
