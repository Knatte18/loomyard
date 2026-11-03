// codexengine.go defines the Codex type, its constructors and Prepare, which readies the lym Codex home and writes the run's prompt file before returning the launch.

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

// validateSpec refuses what a Codex launch cannot realize, before anything is written: a version pin, a permission mode other than bypass, a resumed session, and an effort codex.yaml does not list.
func validateSpec(spec shuttleengine.Spec, codex Config) error {
	if spec.Version != "" {
		return fmt.Errorf("codexengine: version %q given, but Codex takes a model slug and no version pin; way forward: name the slug as the model-spec's model", spec.Version)
	}
	if spec.PermissionMode != "" && spec.PermissionMode != permissionModeBypass {
		return fmt.Errorf("codexengine: permission mode %q is not realizable; every Codex launch runs -a never, the bypass mode; way forward: leave it empty or set %q", spec.PermissionMode, permissionModeBypass)
	}
	if spec.ResumeSessionID != "" {
		return fmt.Errorf("codexengine: Codex sessions cannot be resumed by a new run; way forward: start a fresh session")
	}
	return validateEffort(spec.Effort, codex)
}

// Prepare readies a Codex launch and returns its lines.
// It validates the spec and loads the hub's gate config, whose refuse_raw list the shell guard installs.
// A gate config that fails to load refuses the launch.
// It reads the checkout the anchor sits in and refuses under committed Codex rules, all before anything is written.
// It then readies the lym home, trusting the worktree root and a linked worktree's main checkout root, and writes prompt.md into runDir.
// The prompt file opens with the deny notice announcing each installed shell guard row and the standing passages heddle holds for engine codex and the spec's role, then the spec's prompt.
// The launch line carries a PreToolUse hook per installed row, and its writable root is the records checkout the engine is told through its Dirs, symlink-resolved and kept as told with a Warn when it cannot be resolved, so the session writes its report through the anchor's `.lym` and `_lym` links; outside a hub it is empty and the line adds none.
// A spec that names skills launches with the skill-load line as its first turn and returns the prompt pointer as Launch.PromptLine, for shuttle to send after the skills load.
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
	homes, err := resolveHomes(codex)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	anchor := c.dirs.Anchor
	if anchor == "" || !filepath.IsAbs(anchor) {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: the engine was told no absolute anchor (%q), so it cannot locate the checkout a launch starts in", anchor)
	}
	found, err := readCheckout(anchor)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	if err := c.checkCodexFolders(found, anchor); err != nil {
		return shuttleengine.Launch{}, err
	}
	if err := c.prepareHome(homes, found.trustRoots()); err != nil {
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

	var writableRoots []string
	if c.dirs.Records != "" {
		// Codex keys a linked path by its resolved target, as it does the trust roots.
		records, err := filepath.EvalSymlinks(c.dirs.Records)
		if err != nil {
			c.warn("codexengine: records checkout not resolvable, using the told path as the writable root", "path", c.dirs.Records, "error", err)
			records = c.dirs.Records
		}
		writableRoots = []string{records}
	}
	sh := shell.ForGOOS()
	return shuttleengine.Launch{
		Cmd: buildLaunchCmd(sh, launchLine{
			bin:           codexBinary(codex),
			home:          homes.lym,
			writableRoots: writableRoots,
			model:         spec.Model,
			effort:        spec.Effort,
			hooks:         hookSettings(filepath.Join(absRunDir, "events.jsonl"), guards),
			firstTurn:     firstTurn,
		}),
		ResumeCmd:            buildResumeCmd(sh),
		PromptLine:           promptLine,
		RecordsSessionStarts: true,
	}, nil
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
