// codexengine.go defines the Codex type, its constructors, Prepare and OpenLaunch.
// Prepare readies the lym Codex home and writes the run's prompt file before returning the launch.
// OpenLaunch readies the same home for a session opened with no prompt.

package codexengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// engineName is the name the engine registers under, which the standing passages are keyed by.
const engineName = "codex"

// defaultSubmitRedrawSettleMS is the redraw settle of an engine built without config.
// It matches submit_redraw_settle_ms in internal/shuttleengine/template.yaml.
const defaultSubmitRedrawSettleMS = 300

// Codex implements shuttleengine.Engine for the Codex CLI's interactive TUI.
// Methods that read codex.yaml do so through the dirs fixed at construction.
// Every other method is a pure function of its arguments, the redraw settle and the files it is pointed at,
// so a Codex is safe to share across concurrent runs.
type Codex struct {
	// dirs locate the layered codex.yaml, the anchor a launch starts in and the records checkout the session may write.
	// The zero value refuses a launch, so an engine built without dirs serves reading uses only.
	dirs configengine.Dirs
	// submitRedrawSettleMS is the wait after an Enter before the composer is read, so Codex has redrawn it.
	submitRedrawSettleMS int
	// warn logs what a launch goes on despite; tests replace it to read the warnings of one engine.
	warn func(msg string, args ...any)
	// sandboxProbe runs executable bin under the Codex home in dir and reports why Codex's sandbox cannot start.
	// It returns nil when the sandbox starts; tests replace it so that no test runs Codex.
	sandboxProbe func(bin, home, dir string) error
}

var (
	_ shuttleengine.Engine         = (*Codex)(nil)
	_ shuttleengine.StartupChecker = (*Codex)(nil)
	_ shuttleengine.SessionOpener  = (*Codex)(nil)
)

// New returns a Codex engine with the default redraw settle and no dirs, so it serves reading uses only.
func New() *Codex {
	return &Codex{submitRedrawSettleMS: defaultSubmitRedrawSettleMS, warn: logger.Warn, sandboxProbe: runSandboxProbe}
}

// NewFromConfig returns a Codex engine whose redraw settle is cfg.SubmitRedrawSettleMS.
// It reads codex.yaml and its anchor from dirs.
func NewFromConfig(cfg shuttleengine.Config, dirs configengine.Dirs) *Codex {
	return &Codex{dirs: dirs, submitRedrawSettleMS: cfg.SubmitRedrawSettleMS, warn: logger.Warn, sandboxProbe: runSandboxProbe}
}

// permissionModeBypass is the permission mode every Codex launch runs in.
// An empty permission mode is accepted too.
const permissionModeBypass = "bypass"

// validateSpec refuses what a Codex run cannot realize, before anything is written:
// a resumed session, an advisor, and a model choice or permission mode a Codex launch cannot realize.
func validateSpec(spec shuttleengine.Spec, codex Config) error {
	if spec.ResumeSessionID != "" {
		return fmt.Errorf("codexengine: Codex sessions cannot be resumed by a new run; way forward: start a fresh session")
	}
	if spec.Advisor != "" {
		return fmt.Errorf("codexengine: advisor %q given, but Codex has no advisor tool; way forward: drop the advisor, or run on an engine that has one, such as claude", spec.Advisor)
	}
	return validateChoice(spec.ModelChoice, spec.PermissionMode, codex)
}

// validateChoice refuses a model choice and permission mode a Codex launch cannot realize, before anything is written:
// a version pin, a permission mode other than bypass, and an effort codex.yaml does not list.
func validateChoice(choice shuttleengine.ModelChoice, permissionMode string, codex Config) error {
	if choice.Version != "" {
		return fmt.Errorf("codexengine: version %q given, but Codex takes a model slug and no version pin; way forward: name the slug as the model-spec's model", choice.Version)
	}
	if permissionMode != "" && permissionMode != permissionModeBypass {
		return fmt.Errorf("codexengine: permission mode %q is not realizable; every Codex launch runs -a never, the bypass mode; way forward: leave it empty or set %q", permissionMode, permissionModeBypass)
	}
	return validateEffort(choice.Effort, codex)
}

// Prepare readies a Codex launch for the run in runDir and returns it.
// Before anything is written, it validates the spec and loads the hub's gate config.
// A gate config that fails to load refuses the launch, and so does a checkout that Codex rules refuse.
// It readies the lym home, settles the sandbox and writes prompt.md into runDir.
// Under codex.yaml's workspace-write, a sandbox that cannot start on the machine refuses the launch, naming the cause.
// The prompt holds the deny notice, the creel trust sentence, the standing passages and the spec's prompt.
// The launch line carries a PreToolUse hook per installed shell guard.
// It runs the settled sandbox mode, and under workspace-write it turns on the sandbox's network access,
// so the session reaches the creel daemon's socket.
// Once Codex exits, the launch appends the exit record to the run's events file.
// A board directory adds creel's hook delivery and marks the launch CreelHooks.
// A spec with skills starts with the skill load as its first turn.
// It returns the prompt pointer as Launch.PromptLine, for shuttle to send once the skills load.
// The launch records session starts and names no session id, since Codex mints its own at the first turn.
func (c *Codex) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	codex, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	if err := validateSpec(spec, codex); err != nil {
		return shuttleengine.Launch{}, err
	}
	// The refuse list lives in the hub's gate config; an absent file means the hub refuses nothing.
	gate, err := gateslot.LoadConfig(c.dirs)
	if err != nil && !errors.Is(err, gateslot.ErrConfigAbsent) {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: %w", err)
	}
	guards := installedGuards(gate.RefuseRaw, guardInputs{denyPython: cfg.DenyPython, denyPublish: spec.DenyPublish})
	home, writableRoots, err := c.readyHome(codex)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	sandboxMode, err := c.launchSandboxMode(codex, home)
	if err != nil {
		return shuttleengine.Launch{}, err
	}

	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: resolve run directory %s: %w", runDir, err)
	}
	promptPath := filepath.Join(absRunDir, "prompt.md")
	// The told Board dir is the hub's Bolt checkout, empty outside a hub.
	passages := shuttleengine.StandingPassages(c.dirs.Board, engineName, spec.Role)
	if err := os.WriteFile(promptPath, []byte(promptText(denyNotice(guards), passages, spec.Prompt)), 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: write prompt: %w", err)
	}
	firstTurn, promptLine := launchPointer(promptPath), ""
	if len(spec.Skills) > 0 {
		// The roots come from the codex.yaml this launch already loaded, so one load serves the whole launch.
		firstTurn, promptLine = skillLoadOver(spec.Skills, c.expandSkillRoots(codex)), firstTurn
	}

	sh := shell.ForGOOS()
	eventsPath := filepath.Join(absRunDir, "events.jsonl")
	var creel creelDelivery
	if c.dirs.Board != "" {
		creel = creelDelivery{flagDir: shuttleengine.CreelFlagDir(c.dirs.Board), home: home, bin: codexBinary(codex)}
	}
	return shuttleengine.Launch{
		Cmd: buildLaunchCmd(sh, launchLine{
			bin:           codexBinary(codex),
			home:          home,
			sandboxMode:   sandboxMode,
			writableRoots: writableRoots,
			model:         spec.Model,
			effort:        spec.Effort,
			hooks:         hookSettings(eventsPath, guards, creel),
			firstTurn:     firstTurn,
			eventsPath:    eventsPath,
		}),
		ResumeCmd:            buildResumeCmd(sh),
		PromptLine:           promptLine,
		RecordsSessionStarts: true,
		CreelHooks:           creel.flagDir != "",
	}, nil
}

// OpenLaunch returns the launch of a named interactive Codex session with no prompt.
// It validates the model choice and the permission mode, readies the lym home and settles the sandbox as Prepare does.
// It refuses under Codex rules before anything is written.
// Its line runs Codex with no hooks and no first turn, so the session carries no shell guard and waits at its composer.
// When Codex exits, the pane's shell writes the exit marker, asks reed to remove the strand and exits.
// The line runs the settled sandbox mode, and under workspace-write it turns on the sandbox's network access,
// so an opened session reaches the creel daemon's socket too.
// It writes nothing into the directory it is given and names no session id.
// It carries no creel hooks, since the session has no prompt file to hold the creel trust sentence.
// Codex reads a hook's block as a failed tool call without that sentence, so the strand gets its mail by the pane path.
// Its resume line is empty: Codex mints the session id at the first turn,
// so a revive starts no Codex and the chain after it ends the strand.
func (c *Codex) OpenLaunch(_ string, spec shuttleengine.OpenSpec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
	codex, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	if err := validateChoice(spec.ModelChoice, spec.PermissionMode, codex); err != nil {
		return shuttleengine.Launch{}, err
	}
	home, writableRoots, err := c.readyHome(codex)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	sandboxMode, err := c.launchSandboxMode(codex, home)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	return shuttleengine.Launch{
		Cmd: codexCommand(shell.ForGOOS(), launchLine{
			bin:           codexBinary(codex),
			home:          home,
			sandboxMode:   sandboxMode,
			writableRoots: writableRoots,
			model:         spec.Model,
			effort:        spec.Effort,
		}),
	}, nil
}

// readyHome readies the lym home that codex.yaml names for a launch in the checkout the engine's anchor sits in.
// Before anything is written, it refuses an anchor that is not absolute or sits in no git checkout.
// It also refuses a checkout that Codex rules refuse, and an empty or relative writable_roots value.
// Then it trusts the checkout's roots in the home.
// It returns the home and the launch's writable roots, each symlink-resolved, in this order:
// the records checkout the engine is told, or none outside a hub;
// the checkout's git dir and the common git dir it links to;
// then codex.yaml's writable_roots that exist.
// A records checkout that cannot be resolved is kept as told, with a warning logged.
// The git dirs are listed because a linked worktree's git dirs lie under the main checkout, outside the workspace,
// and a commit writes its index lock, objects and refs there.
func (c *Codex) readyHome(codex Config) (home string, writableRoots []string, err error) {
	homes, err := resolveHomes(codex)
	if err != nil {
		return "", nil, err
	}
	anchor := c.dirs.Anchor
	if anchor == "" || !filepath.IsAbs(anchor) {
		return "", nil, fmt.Errorf("codexengine: the engine was told no absolute anchor (%q), so it cannot locate the checkout a launch starts in", anchor)
	}
	found, err := readCheckout(anchor)
	if err != nil {
		return "", nil, err
	}
	if err := c.checkCodexFolders(found, anchor); err != nil {
		return "", nil, err
	}
	configured, err := configuredWritableRoots(codex)
	if err != nil {
		return "", nil, err
	}
	if err := c.prepareHome(homes, found.trustRoots()); err != nil {
		return "", nil, err
	}
	if c.dirs.Records != "" {
		// Codex keys a linked path by its resolved target, so the writable root must be the resolved path.
		records, err := filepath.EvalSymlinks(c.dirs.Records)
		if err != nil {
			c.warn("codexengine: records checkout not resolvable, using the told path as the writable root", "path", c.dirs.Records, "error", err)
			records = c.dirs.Records
		}
		writableRoots = []string{records}
	}
	writableRoots = append(writableRoots, found.gitDirs()...)
	return homes.lym, append(writableRoots, configured...), nil
}

// promptText returns the prompt file's text as paragraphs separated by blank lines.
// The first paragraph is `shuttleengine.CreelTrustSentence`, with a non-empty deny notice before it on the same line.
// Each passage follows as its own paragraph, and the prompt comes last.
func promptText(notice string, passages []string, prompt string) string {
	head := shuttleengine.CreelTrustSentence
	if notice != "" {
		head = notice + " " + head
	}
	return strings.Join(append(append([]string{head}, passages...), prompt), "\n\n")
}

// PromptCacheLifetime returns zero: Codex has no warm lifetime for its prompt cache, so a caller keeps nothing warm.
func (c *Codex) PromptCacheLifetime(shuttleengine.Spec, shuttleengine.Config) (time.Duration, error) {
	return 0, nil
}

// KeepWarmCaps returns zero for both durations.
// With no warm lifetime to keep, a Codex session blocks to the end of a wait.
func (c *Codex) KeepWarmCaps(string) (time.Duration, time.Duration, error) {
	return 0, 0, nil
}
