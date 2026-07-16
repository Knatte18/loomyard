// claudeengine.go defines the Claude type and its compile-time assertion
// against shuttleengine.Engine. The type itself carries no state — every
// method it implements is a pure function of its arguments (see command.go,
// settings.go, events.go, startup.go) — which is what makes the whole
// adapter hermetically testable without tmux or a real claude process.

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
// This mirrors the same recipe muxpoccli (now deleted) used; it is
// independently implemented here rather than shared via import because
// a CLI feature package is not something shuttleengine should depend on.
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
// select. spec.Effort is hard-error-validated (validateEffort) before any
// artifact is written; a valid value is threaded through to buildLaunchCmd
// unchanged. spec.Model and spec.Version are likewise resolved
// (resolveModelID) before any artifact is written; the resolved model id —
// not spec.Model — is what buildLaunchCmd receives.
func (c *Claude) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	// Reject an over-ceiling prompt before any artifact is written: past
	// maxLaunchPromptBytes the pane launch is guaranteed to fail against the
	// Windows command-line limit, and the only symptom would be an opaque
	// `died` a full startup window later. Failing here is immediate and
	// self-describing instead.
	if len(spec.Prompt) > maxLaunchPromptBytes {
		return shuttleengine.Launch{}, fmt.Errorf(
			"prompt is %d bytes, over the %d-byte launch limit: the pane launch expands the whole prompt into one command-line argument and Windows caps a process command line at 32,767 characters — move the long content into a file and make the prompt a short pointer to it",
			len(spec.Prompt), maxLaunchPromptBytes,
		)
	}

	// Reject an unrealizable effort before any artifact is written, for the
	// same reason as the prompt-size guard above: claude only
	// warns-and-ignores a bad --effort value rather than failing the launch,
	// so failing here is the only way to surface the mistake at all, and
	// failing before prompt.md/settings.json exist keeps a rejected Prepare
	// call from leaving a half-written run directory behind.
	if err := validateEffort(spec.Effort); err != nil {
		return shuttleengine.Launch{}, err
	}

	// Resolve the bare-word model + version pin into the final model id
	// before any artifact is written, for the same reason as the effort
	// guard above: a (model, version) pair the engine cannot realize must
	// fail here, not leave a half-written run directory behind.
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

	// The hook command embeds this path and runs under git-bash on Windows,
	// where a backslash path is silently misread (backslash is git-bash's
	// escape character) — so on Windows convert to the git-bash POSIX form.
	// On a POSIX host the hook runs in the native shell and the path is already
	// correct; pass it through unconverted (PosixPath only accepts drive-rooted
	// Windows paths and would reject an ordinary /tmp/... run dir).
	eventsPath := filepath.Join(runDir, "events.jsonl")
	eventsPathForHook := eventsPath
	if runtime.GOOS == "windows" {
		eventsPathForHook, err = shuttleengine.PosixPath(eventsPath)
		if err != nil {
			return shuttleengine.Launch{}, fmt.Errorf("convert events path to posix: %w", err)
		}
	}

	settingsJSON, err := buildSettings(eventsPathForHook, spec.Interactive, cfg)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("build settings: %w", err)
	}
	settingsPath := filepath.Join(runDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsJSON, 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write settings: %w", err)
	}

	bin := claudeBinary(cfg)
	// sh selects the pane-shell mechanics (quoting, call operator, prompt-file
	// read idiom) for the current host OS — pwsh on Windows, posix elsewhere —
	// so buildLaunchCmd/buildResumeCmd never hardcode either shell's syntax.
	sh := shell.ForGOOS()
	return shuttleengine.Launch{
		Cmd:       buildLaunchCmd(sh, bin, promptPath, settingsPath, sessionID, resolvedModel, spec.Effort, spec.Interactive, spec.ForkSubagents),
		ResumeCmd: buildResumeCmd(sh, bin, settingsPath, sessionID, spec.ForkSubagents),
		SessionID: sessionID,
	}, nil
}
