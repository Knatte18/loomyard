// sessionname.go gives Claude the three reedengine.SessionNamer methods the per-hub watchdog uses to repair a session's own name.
// All Claude shape knowledge stays here, per the Shuttle Provider-Seam Invariant.
//
// The session name is read from the transcript's latest `custom-title` line, which Claude Code writes on `--name` and on `/rename`.
// `custom-title` is a Claude Code internal, not a documented interface.
// If its shape changes, every read finds no entry, so the watchdog retypes `/rename` on each idle tick and logs each one.

package claudeengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

var _ reedengine.SessionNamer = (*Claude)(nil)

// customTitleType is the transcript line type that records a session's name.
const customTitleType = "custom-title"

// SessionNameDrift reports whether the session's latest recorded name differs from want.
// A transcript with no name line at all counts as drift, since Claude never got the name or a compact dropped it.
// A missing or unreadable transcript logs why and reports no drift, so a session whose transcript is not yet written is never retyped.
func (c *Claude) SessionNameDrift(sessionID, workdir, want string) bool {
	projectDir, err := claudeProjectDirFor(workdir)
	if err != nil {
		logger.Warn("claudeengine: could not read the session name", "session_id", sessionID, "err", err)
		return false
	}
	path := filepath.Join(projectDir, sessionID+".jsonl")

	latest, found := "", false
	err = forEachTranscriptLine(path, func(line transcriptLine) {
		if line.Type == customTitleType {
			latest, found = line.CustomTitle, true
		}
	})
	if err != nil {
		logger.Warn("claudeengine: could not read the session name", "path", path, "err", err)
		return false
	}
	return !found || latest != want
}

// SessionIdle delegates to IdleSession, the same probe the orch watcher gates /clear on.
func (c *Claude) SessionIdle(capture string) bool {
	return c.IdleSession(capture)
}

// RenameText returns the text typed, before Enter, to rename the session to want.
func (c *Claude) RenameText(want string) string {
	return "/rename " + want
}
