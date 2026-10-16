// claudeengine.go defines the Claude type and its compile-time assertion against
// shuttleengine.Engine.
// The type carries its two settles, the reading clock and the told Dirs, fixed at construction.
// Prepare, OpenLaunch and PromptCacheLifetime read claude.yaml through those Dirs,
// and every other method it implements is a pure function of its arguments, those settles and that clock (see command.go, settings.go, events.go, startup.go), which keeps the adapter hermetically testable without tmux or a real claude process.

package claudeengine

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// defaultSubmitSettleMS is the submit settle New uses, matching the shipped shuttle template.
const defaultSubmitSettleMS = 300

// defaultSubmitRedrawSettleMS is the redraw settle New uses, matching the shipped shuttle template.
const defaultSubmitRedrawSettleMS = 300

// Claude implements shuttleengine.Engine for the Claude Code CLI.
// Prepare, OpenLaunch and PromptCacheLifetime read claude.yaml through the Dirs fixed at construction;
// every other method is a pure function of its arguments, its settle fields and its clock,
// so a Claude is safe to share across concurrent runs.
// The one piece of shared mutable state is nameReads, which remembers the session ids whose name-read failure was already warned about.
type Claude struct {
	// dirs locate the layered claude.yaml; the zero value makes the config-reading methods refuse, so an engine built by New serves reading uses only.
	dirs configengine.Dirs
	// now is the clock the activity reading takes the time of a fork's pending tool call from.
	now func() time.Time
	// submitSettleMS is the pause between typed text and the Enter that submits it.
	// Claude Code reads a fast character burst as a paste and swallows an Enter that arrives inside it,
	// so the settle moves the Enter out of the paste window.
	submitSettleMS int
	// submitRedrawSettleMS is the wait after an Enter before the input box is read, so Claude Code has redrawn it.
	submitRedrawSettleMS int
	// nameReads is a pointer so a copy of the struct shares one set; nil warns on every failure.
	nameReads *nameReadWarnings
}

// New returns a Claude engine with the default submit settle.
func New() *Claude {
	return &Claude{now: time.Now, submitSettleMS: defaultSubmitSettleMS, submitRedrawSettleMS: defaultSubmitRedrawSettleMS, nameReads: &nameReadWarnings{}}
}

// NewFromConfig returns a Claude engine whose submit settle is cfg.SubmitSettleMS and whose redraw settle is cfg.SubmitRedrawSettleMS.
// The engine reads claude.yaml from the layers dirs locate.
func NewFromConfig(cfg shuttleengine.Config, dirs configengine.Dirs) *Claude {
	return &Claude{dirs: dirs, now: time.Now, submitSettleMS: cfg.SubmitSettleMS, submitRedrawSettleMS: cfg.SubmitRedrawSettleMS, nameReads: &nameReadWarnings{}}
}

// loadConfigOrRefuse loads claude.yaml from dirs.
// A load error keeps configengine's text naming the file and key and its invalid mark, and gains the way forward.
func loadConfigOrRefuse(dirs configengine.Dirs) (Config, error) {
	claude, err := LoadConfig(dirs)
	if err != nil {
		return Config{}, fmt.Errorf("claudeengine: %w; way forward: correct or remove the key in the file the error names, with lyx config claude --layer <default|worktree|local> (or --set for a value)", err)
	}
	return claude, nil
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
// A spec that names skills carries the skill-load message on the launch line instead and returns the pointer as Launch.PromptLine, for shuttle to send after the skills load.
// It loads claude.yaml, then validates spec.Effort, spec.Model, spec.PermissionMode, spec.ResumeSessionID and the prompt-cache TTL config before writing any artifacts.
func (c *Claude) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, err
	}

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
	promptCacheTTL, err := resolvePromptCacheTTL(spec.Role, claude)
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
	skillLoadMessage := ""
	if len(spec.Skills) > 0 {
		skillLoadMessage = c.SkillLoadMessage(spec.Skills)
		if len(skillLoadMessage) > maxLaunchPromptBytes {
			return shuttleengine.Launch{}, fmt.Errorf(
				"skill-load message is %d bytes, over the %d-byte launch limit: the pane launch carries it as one command-line argument and Windows caps a process command line at 32,767 characters — name fewer or shorter skills than %v",
				len(skillLoadMessage), maxLaunchPromptBytes, spec.Skills,
			)
		}
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

	settingsJSON, err := buildSettings(eventsPathForHook, spec.Interactive, cfg, claude, spec.ForkSubagents, spec.AllowAgentTool, spec.ContextAfterCompaction)
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
	if err := os.WriteFile(envFilePath, []byte(envFileContentFor(runtime.GOOS)), 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write env file: %w", err)
	}

	bin := claudeBinary(claude)
	notice := buildDenyNotice(spec.Interactive, cfg, claude, spec.ForkSubagents, spec.AllowAgentTool)
	// sh selects pane-shell mechanics per OS (pwsh on Windows, posix elsewhere).
	sh := shell.ForGOOS()
	// A spec that names skills launches with the load message as its first turn, and the pointer goes out after the skills as Launch.PromptLine.
	launchArg, promptLine := pointer, ""
	if len(spec.Skills) > 0 {
		launchArg, promptLine = skillLoadMessage, pointer
	}
	return shuttleengine.Launch{
		Cmd:        buildLaunchCmd(sh, bin, launchArg, settingsPath, sessionID, resolvedModel, spec.Effort, notice, envFilePath, promptCacheTTL, resume, skipPermissions, spec.ForkSubagents),
		ResumeCmd:  buildResumeCmd(sh, bin, settingsPath, sessionID, resolvedModel, spec.Effort, notice, envFilePath, promptCacheTTL, skipPermissions, spec.ForkSubagents),
		SessionID:  sessionID,
		PromptLine: promptLine,

		RecordsSessionStarts: true,
	}, nil
}
