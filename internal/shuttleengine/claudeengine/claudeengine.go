// claudeengine.go defines the Claude type and its compile-time assertion
// against shuttleengine.Engine. The type itself carries no state — every
// method it implements is a pure function of its arguments (see command.go,
// settings.go, events.go, startup.go) — which is what makes the whole
// adapter hermetically testable without psmux or a real claude process.

package claudeengine

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Claude implements shuttleengine.Engine for the Claude Code CLI. It carries
// no fields: every method is a pure function of its arguments, so a single
// zero-value Claude is safe to share across concurrent runs.
type Claude struct{}

// New returns a Claude engine ready to use.
func New() *Claude {
	return &Claude{}
}

// var _ shuttleengine.Engine = (*Claude)(nil) is the compile-time proof that
// Claude satisfies the provider seam; a missing or mis-signed method here
// fails the build immediately rather than surfacing later as a runtime
// type-assertion panic.
var _ shuttleengine.Engine = (*Claude)(nil)

// newSessionID mints a UUID v4 (crypto/rand, RFC-4122 version/variant bits
// set) as the session identity Prepare hands to claude via --session-id.
// This mirrors muxpoccli's newSessionID recipe; it is reimplemented here
// rather than imported because muxpoccli is a feature package shuttleengine
// must not depend on.
func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}

	// Set version to 4 (bits 12-15 of time_hi_and_version).
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant to RFC 4122 (bits 6-7 of clock_seq_hi_and_reserved).
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// Prepare writes this run's prompt.md and settings.json into runDir and
// returns the Launch command strings to start (or later resume) it. It is
// the one place claudeengine composes launch/resume commands from all three
// inputs: the session id it mints here, the POSIX form of the run's events
// path (hook commands run under git-bash, which cannot parse a Windows
// backslash path), and the claude binary/flags cfg and spec.Interactive
// select.
func (c *Claude) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	sessionID, err := newSessionID()
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("mint session id: %w", err)
	}

	promptPath := filepath.Join(runDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(spec.Prompt), 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write prompt: %w", err)
	}

	eventsPath := filepath.Join(runDir, "events.jsonl")
	eventsPathPosix, err := shuttleengine.PosixPath(eventsPath)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("convert events path to posix: %w", err)
	}

	settingsJSON, err := buildSettings(eventsPathPosix, spec.Interactive, cfg)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("build settings: %w", err)
	}
	settingsPath := filepath.Join(runDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsJSON, 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write settings: %w", err)
	}

	bin := claudeBinary(cfg)
	return shuttleengine.Launch{
		Cmd:       buildLaunchCmd(bin, promptPath, settingsPath, sessionID, spec.Model, spec.Interactive),
		ResumeCmd: buildResumeCmd(bin, settingsPath, sessionID),
		SessionID: sessionID,
	}, nil
}
