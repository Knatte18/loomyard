// claudeengine.go defines the Claude type and its compile-time assertion against
// shuttleengine.Engine.
// The type carries only its two settles, fixed at construction,
// and every method it implements is a pure function of its arguments and those settles (see command.go, settings.go, events.go, startup.go), which is what makes the whole adapter hermetically testable without tmux or a real claude process.

package claudeengine

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// defaultSubmitSettleMS is the submit settle New uses, matching the shipped shuttle template.
const defaultSubmitSettleMS = 300

// defaultSubmitRedrawSettleMS is the redraw settle New uses, matching the shipped shuttle template.
const defaultSubmitRedrawSettleMS = 300

// Claude implements shuttleengine.Engine for the Claude Code CLI.
// Its methods are pure functions of their arguments and its settle fields, fixed at construction,
// so a Claude is safe to share across concurrent runs.
type Claude struct {
	// submitSettleMS is the pause between typed text and the Enter that submits it.
	// Claude Code reads a fast character burst as a paste and swallows an Enter that arrives inside it,
	// so the settle moves the Enter out of the paste window.
	submitSettleMS int
	// submitRedrawSettleMS is the wait after an Enter before the input box is read, so Claude Code has redrawn it.
	submitRedrawSettleMS int
}

// New returns a Claude engine with the default submit settle.
func New() *Claude {
	return &Claude{submitSettleMS: defaultSubmitSettleMS, submitRedrawSettleMS: defaultSubmitRedrawSettleMS}
}

// NewFromConfig returns a Claude engine whose submit settle is cfg.SubmitSettleMS and whose redraw settle is cfg.SubmitRedrawSettleMS.
func NewFromConfig(cfg shuttleengine.Config) *Claude {
	return &Claude{submitSettleMS: cfg.SubmitSettleMS, submitRedrawSettleMS: cfg.SubmitRedrawSettleMS}
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

// sessionIDShape is the lowercase hyphenated UUID shape newSessionID mints.
var sessionIDShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validateSessionID reports an error unless id is the lowercase hyphenated UUID shape newSessionID mints.
// The shape also keeps shell metacharacters out of the id quoted onto the pane line.
func validateSessionID(id string) error {
	if !sessionIDShape.MatchString(id) {
		return fmt.Errorf("claudeengine: invalid resume session id %q; expected a lowercase hyphenated UUID (8-4-4-4-12 hex digits)", id)
	}
	return nil
}

// Prepare writes prompt.md, settings.json and the Bash env file bash-env.sh into runDir and returns the Launch command strings.
// The launch line carries a pointer to prompt.md rather than the prompt, so a prompt of any size launches.
// A spec that names skills leaves the pointer off the launch line and returns it as Launch.PromptLine, for the skills to load first.
// It validates spec.Effort, spec.Model, spec.PermissionMode, spec.ResumeSessionID and the prompt-cache TTL config before writing any artifacts.
func (c *Claude) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	// Reject unrealizable effort before any artifact is written (claude ignores bad efforts at launch).
	if err := validateEffort(spec.Effort); err != nil {
		return shuttleengine.Launch{}, err
	}

	// Reject an unrealizable permission mode before any artifact is written.
	skipPermissions, err := validatePermissionMode(spec.PermissionMode, spec.Interactive)
	if err != nil {
		return shuttleengine.Launch{}, err
	}

	// Resolve the bare-word model + version into the final model id before any artifact is written.
	resolvedModel, err := resolveModelID(spec.Model, spec.Version)
	if err != nil {
		return shuttleengine.Launch{}, err
	}

	// Resolve the role's prompt-cache TTL once, so both lines carry the same value, before any artifact is written.
	promptCacheTTL, err := resolvePromptCacheTTL(spec.Role, cfg)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	logger.Info("claudeengine: resolved the prompt-cache TTL", "role", spec.Role, "ttl", promptCacheTTL)

	// An adopted run takes over an existing session; reject a malformed id before any artifact is written.
	resume := spec.ResumeSessionID != ""
	sessionID := spec.ResumeSessionID
	if resume {
		if err := validateSessionID(sessionID); err != nil {
			return shuttleengine.Launch{}, err
		}
	} else {
		sessionID, err = newSessionID()
		if err != nil {
			return shuttleengine.Launch{}, fmt.Errorf("mint session id: %w", err)
		}
	}

	promptPath, err := filepath.Abs(filepath.Join(runDir, "prompt.md"))
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("resolve prompt path: %w", err)
	}
	// The launch argument is a pointer to prompt.md, so the prompt's own size never reaches the command line.
	pointer := launchPointer(promptPath)
	if len(pointer) > maxLaunchPromptBytes {
		return shuttleengine.Launch{}, fmt.Errorf(
			"launch pointer is %d bytes, over the %d-byte launch limit: the pane launch carries it as one command-line argument and Windows caps a process command line at 32,767 characters — shorten the run-directory path %q",
			len(pointer), maxLaunchPromptBytes, promptPath,
		)
	}
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

	settingsJSON, err := buildSettings(eventsPathForHook, spec.Interactive, cfg, spec.ForkSubagents, spec.AllowAgentTool, spec.ContextAfterCompaction)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("build settings: %w", err)
	}
	settingsPath := filepath.Join(runDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsJSON, 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write settings: %w", err)
	}

	// The env file path stays the native absolute path on every OS: Claude Code reads the file itself and runs its content in the Bash tool's shell.
	envFilePath, err := filepath.Abs(filepath.Join(runDir, envFileName))
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("resolve env file path: %w", err)
	}
	if err := os.WriteFile(envFilePath, []byte(envFileContent), 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write env file: %w", err)
	}

	bin := claudeBinary(cfg)
	notice := buildDenyNotice(spec.Interactive, cfg, spec.ForkSubagents, spec.AllowAgentTool)
	// sh selects pane-shell mechanics per OS (pwsh on Windows, posix elsewhere).
	sh := shell.ForGOOS()
	// A spec that names skills starts on an empty input box, and the pointer goes out after the skills as Launch.PromptLine.
	launchArg, promptLine := pointer, ""
	if len(spec.Skills) > 0 {
		launchArg, promptLine = "", pointer
	}
	return shuttleengine.Launch{
		Cmd:        buildLaunchCmd(sh, bin, launchArg, settingsPath, sessionID, resolvedModel, spec.Effort, notice, envFilePath, promptCacheTTL, resume, skipPermissions, spec.ForkSubagents),
		ResumeCmd:  buildResumeCmd(sh, bin, settingsPath, sessionID, resolvedModel, spec.Effort, notice, envFilePath, promptCacheTTL, skipPermissions, spec.ForkSubagents),
		SessionID:  sessionID,
		PromptLine: promptLine,
	}, nil
}
