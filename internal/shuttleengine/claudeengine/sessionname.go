// sessionname.go gives Claude the three reedengine.SessionNamer methods the per-hub watchdog uses to repair a session's own name.
// All Claude shape knowledge stays here, per the Shuttle Provider-Seam Invariant.
//
// The session name is read from the transcript's latest `custom-title` line, which Claude Code writes on `--name` and on `/rename`.
// `custom-title` is a Claude Code internal, not a documented interface.
// If its shape changes, every read finds no entry, so the watchdog retypes `/rename` on each idle tick and logs each one.

package claudeengine

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

var _ reedengine.SessionNamer = (*Claude)(nil)

// customTitleType is the transcript line type that records a session's name.
const customTitleType = "custom-title"

// SessionNameDrift reports whether the session's latest recorded name differs from want.
// A transcript with no name line at all counts as drift, since Claude never got the name or a compact dropped it.
// A missing or unreadable transcript reports no drift, so a session whose transcript is not yet written is never retyped.
// A transcript that does not exist yet logs at Debug.
// Any other read failure logs at Warn once per session id for as long as c lives, and at Debug after that.
func (c *Claude) SessionNameDrift(sessionID, workdir, want string) bool {
	projectDir, err := claudeProjectDirFor(workdir)
	if err != nil {
		c.logNameReadFailure(sessionID, "session_id", sessionID, "err", err)
		return false
	}
	path := filepath.Join(projectDir, sessionID+".jsonl")

	latest, found := "", false
	err = forEachTranscriptLine(path, func(line transcriptLine) {
		if line.Type == customTitleType {
			latest, found = line.CustomTitle, true
		}
	})
	if errors.Is(err, fs.ErrNotExist) {
		logger.Debug("claudeengine: could not read the session name", "path", path, "err", err)
		return false
	}
	if err != nil {
		c.logNameReadFailure(sessionID, "path", path, "err", err)
		return false
	}
	return !found || latest != want
}

// logNameReadFailure logs a session-name read failure at Warn the first time sessionID fails and at Debug after that.
func (c *Claude) logNameReadFailure(sessionID string, fields ...any) {
	const message = "claudeengine: could not read the session name"
	if c.nameReads.first(sessionID) {
		logger.Warn(message, fields...)
		return
	}
	logger.Debug(message, fields...)
}

// nameReadWarnings is the set of session ids whose name-read failure has already been warned about.
// The zero value is ready and safe for concurrent use.
type nameReadWarnings struct {
	mu   sync.Mutex
	seen map[string]bool
}

// first records sessionID and reports whether this is the first time it was seen.
// A nil receiver reports true every time, so a Claude built without a constructor warns on each failure.
func (s *nameReadWarnings) first(sessionID string) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[sessionID] {
		return false
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	s.seen[sessionID] = true
	return true
}

// SessionIdle delegates to IdleSession, the same probe the orch watcher gates /clear on.
func (c *Claude) SessionIdle(capture string) bool {
	return c.IdleSession(capture)
}

// RenameText returns the text typed, before Enter, to rename the session to want.
func (c *Claude) RenameText(want string) string {
	return "/rename " + want
}
