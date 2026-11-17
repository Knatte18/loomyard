// passages.go holds the process-wide seam through which standing instruction passages reach an engine's launch.

package shuttleengine

import (
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Passage is one heddle entry's text as it reaches a session, keyed by the entry's slug.
// Engine is empty when the entry reaches the session through a shared or engine-neutral target.
// Otherwise it names the engine, because only a target aimed at that engine reaches the entry.
type Passage struct {
	Slug   string
	Text   string
	Engine string
}

// Standing is what heddle's layers hold for one session.
// Passages are the entries appended to its system prompt, in the order they are appended.
// FileEntries are the entries rendered into the instruction file its engine reads at the worktree's anchor.
// The zero Standing holds none.
type Standing struct {
	Passages    []Passage
	FileEntries []Passage
}

// PassageReader returns what heddle's layers hold for a session of an engine in a strand role.
type PassageReader func(boltDir, engine, role string) (Standing, error)

var installedPassageReader atomic.Pointer[PassageReader]

// InstallPassageReader stores r as the process's passage reader and returns a function that reinstates the previous one.
func InstallPassageReader(r PassageReader) (restore func()) {
	previous := installedPassageReader.Swap(&r)
	return func() { installedPassageReader.Store(previous) }
}

// StandingPassages returns what the installed reader finds for a session of engine in a strand role.
// The hub is the one whose Bolt checkout is boltDir.
// It returns the zero Standing for an empty boltDir (a session outside a hub) or with no reader installed.
// A reader error also returns the zero Standing and is logged at Warn, so an unreadable store never blocks a launch.
func StandingPassages(boltDir, engine, role string) Standing {
	if boltDir == "" {
		return Standing{}
	}
	reader := installedPassageReader.Load()
	if reader == nil {
		return Standing{}
	}
	standing, err := (*reader)(boltDir, engine, role)
	if err != nil {
		logger.Warn("shuttleengine: the standing passages are unreadable, so the launch carries lym's own sentences only; run `lym heddle list` to inspect the store", "engine", engine, "error", err)
		return Standing{}
	}
	return standing
}
