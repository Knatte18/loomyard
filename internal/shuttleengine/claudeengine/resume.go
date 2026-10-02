package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Compile-time assertion that Claude offers the optional resume check.
var _ shuttleengine.SessionResumer = (*Claude)(nil)

// registryEntry is the audited subset of one Claude session-registry file.
type registryEntry struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
}

// CheckResume implements shuttleengine.SessionResumer.
// It derives the project and registry directories from the same home resolution the fork audit uses, then delegates to checkResumable.
func (c *Claude) CheckResume(sessionID, workdir string) (string, error) {
	projectDir, err := claudeProjectDirFor(workdir)
	if err != nil {
		return "", err
	}
	home, err := claudeHomeDir()
	if err != nil {
		return "", fmt.Errorf("claudeengine: resolve home dir: %w", err)
	}
	registryDir := filepath.Join(home, ".claude", "sessions")
	return checkResumable(sessionID, projectDir, registryDir, proc.IsAlive)
}

// checkResumable refuses a resume unless sessionID is well-formed, has a transcript under projectDir, and is not held by a live process in registryDir.
// Only a positively identified live holder refuses;
// an unreadable registry returns a warning and the check proceeds.
// It kills, signals and edits nothing.
func checkResumable(sessionID, projectDir, registryDir string, alive func(pid int) bool) (warning string, err error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", fmt.Errorf("%w; Claude's /status shows the session id", err)
	}

	transcript := filepath.Join(projectDir, sessionID+".jsonl")
	if _, err := os.Stat(transcript); err != nil {
		return "", fmt.Errorf("claudeengine: no transcript for session %s in %s; only a session run from this directory can be resumed: %w", sessionID, projectDir, err)
	}

	entries, err := os.ReadDir(registryDir)
	if err != nil {
		return fmt.Sprintf("claudeengine: could not read session registry %s (%v); a live holder of session %s could not be ruled out", registryDir, err, sessionID), nil
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(registryDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			warning = fmt.Sprintf("claudeengine: could not read session registry entry %s (%v); a live holder of session %s could not be ruled out", path, err, sessionID)
			continue
		}
		var entry registryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			warning = fmt.Sprintf("claudeengine: could not decode session registry entry %s (%v); a live holder of session %s could not be ruled out", path, err, sessionID)
			continue
		}
		if entry.SessionID == sessionID && alive(entry.PID) {
			return "", fmt.Errorf("claudeengine: session %s is held by live process %d (%s); exit that session first, then resume", sessionID, entry.PID, path)
		}
	}
	return warning, nil
}
