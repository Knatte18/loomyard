// claudeengine.go defines the Claude type and its compile-time assertion against
// shuttleengine.Engine.
// The type itself carries no state — every method it implements is a pure function of its arguments
// (see command.go, settings.go, events.go, startup.go) — which is what makes the whole adapter
// hermetically testable without tmux or a real claude process.

package claudeengine

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Claude implements shuttleengine.Engine for the Claude Code CLI.
// All methods are pure functions, so a zero-value Claude is safe to share across concurrent runs.
type Claude struct{}

// New returns a Claude engine ready to use.
func New() *Claude {
	return &Claude{}
}

// Compile-time proof that Claude satisfies the provider seam.
var _ shuttleengine.Engine = (*Claude)(nil)

// newSessionID mints a UUID v4 (crypto/rand, RFC-4122 bits set) as the session identity.
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

// Prepare writes prompt.md and settings.json into runDir and returns the Launch command strings.
// It validates spec.Effort and spec.Model before writing any artifacts.
func (c *Claude) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	// Reject oversized prompts before any artifact is written (failing now is immediate and self-describing).
	if len(spec.Prompt) > maxLaunchPromptBytes {
		return shuttleengine.Launch{}, fmt.Errorf(
			"prompt is %d bytes, over the %d-byte launch limit: the pane launch expands the whole prompt into one command-line argument and Windows caps a process command line at 32,767 characters — move the long content into a file and make the prompt a short pointer to it",
			len(spec.Prompt), maxLaunchPromptBytes,
		)
	}

	// Reject unrealizable effort before any artifact is written (claude ignores bad efforts at launch).
	if err := validateEffort(spec.Effort); err != nil {
		return shuttleengine.Launch{}, err
	}

	// Resolve the bare-word model + version into the final model id before any artifact is written.
	resolvedModel, err := resolveModelID(spec.Model, spec.Version)
	if err != nil {
		return shuttleengine.Launch{}, err
	}

	sessionID, err := newSessionID()
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("mint session id: %w", err)
	}

	promptPath := filepath.Join(runDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(spec.Prompt), 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write prompt: %w", err)
	}

	// On Windows, convert the events path to git-bash POSIX form (backslash is git-bash's escape character).
	// On POSIX, pass it through unconverted.
	eventsPath := filepath.Join(runDir, "events.jsonl")
	eventsPathForHook := eventsPath
	if runtime.GOOS == "windows" {
		eventsPathForHook, err = shuttleengine.PosixPath(eventsPath)
		if err != nil {
			return shuttleengine.Launch{}, fmt.Errorf("convert events path to posix: %w", err)
		}
	}

	settingsJSON, err := buildSettings(eventsPathForHook, spec.Interactive, cfg, spec.ForkSubagents)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("build settings: %w", err)
	}
	settingsPath := filepath.Join(runDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsJSON, 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write settings: %w", err)
	}

	bin := claudeBinary(cfg)
	// sh selects pane-shell mechanics per OS (pwsh on Windows, posix elsewhere).
	sh := shell.ForGOOS()
	return shuttleengine.Launch{
		Cmd:       buildLaunchCmd(sh, bin, promptPath, settingsPath, sessionID, resolvedModel, spec.Effort, spec.Interactive, spec.ForkSubagents),
		ResumeCmd: buildResumeCmd(sh, bin, settingsPath, sessionID, resolvedModel, spec.Effort, spec.Interactive, spec.ForkSubagents),
		SessionID: sessionID,
	}, nil
}
