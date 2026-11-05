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

// defaultSubmitRedrawSettleMS is the redraw settle New uses, matching the shipped shuttle template.
const defaultSubmitRedrawSettleMS = 300

// Codex implements shuttleengine.Engine for the Codex CLI's interactive TUI.
// Prepare and CheckAfterStartup read codex.yaml through the Dirs fixed at construction, and every other method is a pure function of its arguments, its settle and the files it is pointed at,
// so a Codex is safe to share across concurrent runs.
type Codex struct {
	// dirs locate the layered codex.yaml, the anchor a launch starts in and the records checkout the session may write; the zero value makes Prepare refuse, so an engine built by New serves reading uses only.
	dirs configengine.Dirs
	// submitRedrawSettleMS is the wait after an Enter before the composer is read, so Codex has redrawn it.
	submitRedrawSettleMS int
	// warn logs what a launch goes on despite; tests replace it to read the warnings of one engine.
	warn func(msg string, args ...any)
}

var (
	_ shuttleengine.Engine         = (*Codex)(nil)
	_ shuttleengine.StartupChecker = (*Codex)(nil)
	_ shuttleengine.SessionOpener  = (*Codex)(nil)
)

// New returns a Codex engine with the default redraw settle and no Dirs.
func New() *Codex {
	return &Codex{submitRedrawSettleMS: defaultSubmitRedrawSettleMS, warn: logger.Warn}
}

// NewFromConfig returns a Codex engine whose redraw settle is cfg.SubmitRedrawSettleMS and which reads codex.yaml and its anchor from dirs.
func NewFromConfig(cfg shuttleengine.Config, dirs configengine.Dirs) *Codex {
	return &Codex{dirs: dirs, submitRedrawSettleMS: cfg.SubmitRedrawSettleMS, warn: logger.Warn}
}

// Legal Spec.PermissionMode values: empty and bypass, the mode every launch runs in.
const permissionModeBypass = "bypass"

// validateSpec refuses what a Codex run cannot realize, before anything is written: a resumed session, and whatever validateChoice refuses.
func validateSpec(spec shuttleengine.Spec, codex Config) error {
	if spec.ResumeSessionID != "" {
		return fmt.Errorf("codexengine: Codex sessions cannot be resumed by a new run; way forward: start a fresh session")
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

// Prepare readies a Codex launch and returns its lines.
// It validates the spec and loads the hub's gate config, whose refuse_raw list the shell guard installs.
// A gate config that fails to load refuses the launch.
// It reads the checkout the anchor sits in and refuses under Codex rules there, all before anything is written.
// It then readies the lym home, trusting the worktree root and a linked worktree's main checkout root, and writes prompt.md into runDir.
// The prompt file opens with the deny notice announcing each installed shell guard row and the standing passages heddle holds for engine codex and the spec's role, then the spec's prompt.
// The launch line carries a PreToolUse hook per installed row, and the writable roots readyHome returns.
// One root is the records checkout, so the session writes its report through the anchor's `.lym` and `_lym` links.
// Another is the checkout's git dirs, so the session can commit.
// The rest are codex.yaml's writable_roots that exist, such as a build cache the session's tools fill.
// A spec that names skills launches with the skill-load line as its first turn and returns the prompt pointer as Launch.PromptLine, for shuttle to send after the skills load.
// The launch line turns on the sandbox's network access, so the session reaches the creel daemon's socket, and appends the exit record to the run's events file once Codex exits.
// The launch records session starts, carries no creel hooks and names no session id, since Codex mints its own at the first turn.
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
	return shuttleengine.Launch{
		Cmd: buildLaunchCmd(sh, launchLine{
			bin:           codexBinary(codex),
			home:          home,
			writableRoots: writableRoots,
			model:         spec.Model,
			effort:        spec.Effort,
			hooks:         hookSettings(eventsPath, guards),
			firstTurn:     firstTurn,
			eventsPath:    eventsPath,
		}),
		ResumeCmd:            buildResumeCmd(sh),
		PromptLine:           promptLine,
		RecordsSessionStarts: true,
	}, nil
}

// OpenLaunch returns the launch of a named interactive Codex session with no prompt.
// It validates the model choice and the permission mode and readies the lym home as Prepare does, refusing under Codex rules before anything is written.
// Its line runs Codex with no hooks and no first turn, so the session carries no shell guard and waits at its composer, and the pane's shell goes on to Runner.Open's chain when Codex exits.
// The line turns on the sandbox's network access as a run's does, so an opened session reaches the creel daemon's socket too.
// It writes nothing into dir, carries no creel hooks and names no session id.
// Its resume line is empty: Codex mints the session id at the first turn, so a revive starts no Codex and the chain after it ends the strand.
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
	return shuttleengine.Launch{
		Cmd: codexCommand(shell.ForGOOS(), launchLine{
			bin:           codexBinary(codex),
			home:          home,
			writableRoots: writableRoots,
			model:         spec.Model,
			effort:        spec.Effort,
		}),
	}, nil
}

// readyHome readies the lym home codex.yaml names for a launch in the checkout the engine's anchor sits in.
// It refuses an anchor that is not absolute, a folder in no git checkout, Codex rules in the checkout and an empty or relative writable_roots value before anything is written, then trusts the checkout's roots in the home.
// It returns the home and the launch's writable roots, each symlink-resolved, in this order:
// the records checkout the engine is told, kept as told with a Warn when it cannot be resolved, or none outside a hub;
// the checkout's git dir and the common git dir it links to, since a linked worktree's git dirs lie under the main checkout, outside the workspace, and a commit writes its index lock, objects and refs there;
// then codex.yaml's writable_roots that exist.
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
		// Codex keys a linked path by its resolved target, as it does the trust roots.
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

// promptText returns the prompt file's text: the deny notice, each passage, then prompt, separated by blank lines, with an empty notice left out.
func promptText(notice string, passages []string, prompt string) string {
	var parts []string
	if notice != "" {
		parts = append(parts, notice)
	}
	return strings.Join(append(append(parts, passages...), prompt), "\n\n")
}

// PromptCacheLifetime returns zero: Codex states no warm lifetime for its prompt cache, so a caller keeps nothing warm.
func (c *Codex) PromptCacheLifetime(shuttleengine.Spec, shuttleengine.Config) (time.Duration, error) {
	return 0, nil
}

// KeepWarmCaps returns zero caps: with no warm lifetime to keep, a Codex session blocks to the end of a wait.
func (c *Codex) KeepWarmCaps(string) (time.Duration, time.Duration, error) {
	return 0, 0, nil
}
