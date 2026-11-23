// codexengine.go defines the Codex type, its constructors, Prepare and OpenLaunch.
// Prepare readies the lym Codex home and assembles the developer instructions.
// It writes the run's prompt file before returning the launch.
// OpenLaunch readies the same home for a session opened with no prompt.

package codexengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// LymBinary resolves the absolute path of the lym binary that Prepare names in the python guard hook.
// It defaults to `selfexec.LymBinary`, which refuses a test binary.
// A test that runs Prepare points it at a stand-in path.
var LymBinary = selfexec.LymBinary

// engineName is the name the engine registers under.
// The standing passages are keyed by it, and a spec's prompt engine must name it.
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
	// userSetup resolves the user's Claude setup for a launch; nil adds none.
	userSetup shuttleengine.UserSetupSource
	// runHook runs a plugin hook for the user setup; nil lists each hook as not run.
	runHook shuttleengine.HookRunner
	// rolloutWait bounds the batching nudge's wait for the call it judges to reach the rollout.
	rolloutWait batchingRolloutWait
	// now stamps the edit hook's records; tests replace it to pin the stamp.
	now func() time.Time
}

// Seams are the collaborators a Codex engine takes from its wiring, so a test can fake each of them.
type Seams struct {
	// UserSetup resolves the user's Claude setup; nil adds no user setup to a launch.
	UserSetup shuttleengine.UserSetupSource
	// RunHook runs one plugin hook for UserSetup; nil lists each hook as not run.
	RunHook shuttleengine.HookRunner
	// SandboxProbe runs executable bin under the Codex home in dir and reports why Codex's sandbox cannot start;
	// nil runs Codex's own probe.
	SandboxProbe func(bin, home, dir string) error
}

var (
	_ shuttleengine.Engine         = (*Codex)(nil)
	_ shuttleengine.StartupChecker = (*Codex)(nil)
	_ shuttleengine.SessionOpener  = (*Codex)(nil)
)

// NewFromConfig returns a Codex engine whose redraw settle is cfg.SubmitRedrawSettleMS.
// It reads codex.yaml and its anchor from dirs, and takes its collaborators from seams.
func NewFromConfig(cfg shuttleengine.Config, dirs configengine.Dirs, seams Seams) *Codex {
	probe := seams.SandboxProbe
	if probe == nil {
		probe = runSandboxProbe
	}
	return &Codex{
		dirs:                 dirs,
		submitRedrawSettleMS: cfg.SubmitRedrawSettleMS,
		warn:                 logger.Warn,
		sandboxProbe:         probe,
		userSetup:            seams.UserSetup,
		runHook:              seams.RunHook,
		rolloutWait:          defaultBatchingRolloutWait(),
		now:                  time.Now,
	}
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
// It first refuses a spec whose prompt was not rendered for Codex, unless the prompt is marked hand-written.
// Before anything is written, it validates the spec and reads the hub gate config's refuse_raw list.
// A gate key the launch does not read never refuses it.
// A gate config that does not merge or holds an invalid refuse_raw refuses the launch.
// So does a checkout that Codex rules refuse.
// It readies the lym home, settles the sandbox and writes prompt.md into runDir.
// Under codex.yaml's workspace-write, a sandbox that cannot start on the machine refuses the launch, naming the cause.
// The prompt file holds the spec's prompt alone.
// The launch line carries the standing items as the developer instructions:
// the deny notice with the creel trust sentence.
// Then comes the subagent passage when agents.yaml gives the role an effort.
// The standing passages follow.
// After the sandbox settles, the user's Claude setup follows them:
// the instruction files, the skill listing, then the plugin contexts.
// A source error refuses the launch before the prompt file is written.
// The instruction-file pointers sit between the instruction files and the skill listing.
// They name each CLAUDE.md in the worktree that Codex does not read itself.
// The launch line raises Codex's cap on the project instructions it reads to the size of the files it reads there.
// The launch line also carries the effort its subagents take and the cap on their concurrent threads.
// Items whose setting reaches the argument cap refuse the launch before anything is written.
// The launch line carries a PreToolUse hook per installed shell guard.
// An autonomous spec's question-tool deny comes before them, and the deny notice announces both.
// With codex.yaml's batching nudge on, Prepare seeds the nudge's state file in runDir.
// The line then carries the nudge's PostToolUse hook.
// While cfg's EditLoopEdits is above 0, an autonomous spec's line carries the edit hook.
// The hook writes to the edit records file in runDir.
// It runs the settled sandbox mode, and under workspace-write it turns on the sandbox's network access,
// so the session reaches the creel daemon's socket.
// Once Codex exits, the launch appends the exit record to the run's events file.
// A board directory adds creel's hook delivery and marks the launch CreelHooks.
// A spec with skills starts with the skill load as its first turn.
// Last, it writes the launch manifest into runDir, read back from the launch line it built.
// It returns the prompt pointer as Launch.PromptLine, for shuttle to send once the skills load.
// The launch records session starts and names no session id, since Codex mints its own at the first turn.
func (c *Codex) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	if err := shuttleengine.CheckPromptEngine(spec, engineName); err != nil {
		return shuttleengine.Launch{}, err
	}
	codex, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	if err := validateSpec(spec, codex); err != nil {
		return shuttleengine.Launch{}, err
	}
	// The refuse list lives in the hub's gate config; an absent file means the hub refuses nothing.
	refuseRaw, err := gateslot.LoadRefuseRaw(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: %w", err)
	}
	// The python guard hook runs this binary by path, so a lym of another version on PATH never answers it.
	lymPath, err := LymBinary()
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: resolve the lym binary the python guard hook runs: %w", err)
	}
	guardIn := guardInputs{denyPython: cfg.DenyPython, denyPublish: spec.DenyPublish, allowRawTmux: spec.AllowRawTmux}
	tools := toolHooks{
		guards:       installedGuards(refuseRaw, guardIn),
		lymPath:      lymPath,
		questionDeny: !spec.Interactive,
	}
	// The told Board dir is the hub's Bolt checkout, empty outside a hub.
	standing := shuttleengine.StandingPassages(c.dirs.Board, engineName, spec.Role)
	notice := denyNotice(tools.questionDeny, tools.guards, guardIn)
	items := []instructionItem{creelHeadItem(notice)}
	subagentEffort := c.subagentEffort(cfg.Agents, spec.ConfigRole, codex)
	passage := subagentPassage(subagentEffort, spec.Skills)
	if passage != "" {
		items = append(items, instructionItem{
			name:       "the subagent passage",
			text:       passage,
			wayForward: "shorten the role's skills in agents.yaml with lym config agents, since the passage lists them",
		})
	}
	items = append(items, passageItems(standing.Passages)...)
	home, writableRoots, found, err := c.readyHome(codex)
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
	workDir, err := filepath.EvalSymlinks(c.dirs.Anchor)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: resolve the working directory %s: %w", c.dirs.Anchor, err)
	}
	docMaxBytes, err := projectDocMaxBytes(found, workDir)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	// LoadConfig refused a value that does not parse as a positive duration.
	hookTimeout, _ := time.ParseDuration(codex.PluginHookTimeout)
	setup, err := c.resolveUserSetup(shuttleengine.UserSetupRequest{
		WorkDir:     workDir,
		RepoRoot:    found.root,
		Skills:      spec.Skills,
		Role:        spec.Role,
		SessionID:   filepath.Base(absRunDir),
		HookTimeout: hookTimeout,
		RunHook:     c.runHook,
	})
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	items = append(items, instructionItemsOf(setup.InstructionFiles)...)
	unread := c.unreadClaudeFiles(found, workDir)
	items = append(items, instructionItem{
		name:       instructionPointersItemName,
		text:       instructionPointers(unread),
		wayForward: instructionPointersWayForward,
	})
	items = append(items, instructionItem{name: skillListingItemName, text: setup.SkillListing, wayForward: skillListingWayForward})
	items = append(items, instructionItemsOf(setup.PluginContexts)...)
	instructions, err := developerInstructions(items)
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: %w", err)
	}
	promptPath := filepath.Join(absRunDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(spec.Prompt), 0o644); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("codexengine: write prompt: %w", err)
	}
	if codex.BatchingNudge.Enabled {
		tools.batchingStatePath = filepath.Join(absRunDir, batchingNudgeStateFileName)
		if err := writeBatchingNudgeState(tools.batchingStatePath, codex.BatchingNudge); err != nil {
			return shuttleengine.Launch{}, fmt.Errorf("codexengine: write the batching nudge state: %w", err)
		}
	}
	if cfg.EditLoopEdits > 0 && !spec.Interactive {
		tools.editRecordsPath = filepath.Join(absRunDir, shuttleengine.EditRecordsFileName)
	}
	firstTurn, promptLine := launchPointer(promptPath), ""
	// The roots come from the codex.yaml this launch already loaded, so one load serves the whole launch.
	skillRoots := c.expandSkillRoots(codex)
	if len(spec.Skills) > 0 && !spec.SkipSkillLoadTurn {
		firstTurn, promptLine = skillLoadOver(spec.Skills, skillRoots), firstTurn
	}

	sh := shell.ForGOOS()
	eventsPath := filepath.Join(absRunDir, "events.jsonl")
	var creel creelDelivery
	if c.dirs.Board != "" {
		creel = creelDelivery{flagDir: shuttleengine.CreelFlagDir(c.dirs.Board), home: home, bin: codexBinary(codex)}
	}
	launchCmd := buildLaunchCmd(sh, launchLine{
		bin:                   codexBinary(codex),
		home:                  home,
		sandboxMode:           sandboxMode,
		writableRoots:         writableRoots,
		model:                 spec.Model,
		effort:                spec.Effort,
		developerInstructions: instructions,
		subagentEffort:        subagentEffort,
		maxSubagentThreads:    codex.MaxSubagentThreads,
		projectDocMaxBytes:    docMaxBytes,
		hooks:                 hookSettings(eventsPath, tools, creel),
		firstTurn:             firstTurn,
		eventsPath:            eventsPath,
	})
	err = writeLaunchManifest(launchFacts{
		spec:            spec,
		codex:           codex,
		refuseRaw:       refuseRaw,
		lymPath:         lymPath,
		setup:           setup,
		standing:        standing,
		denyNotice:      notice,
		subagentPassage: passage,
		instructions:    instructions,
		unread:          unread,
		found:           found,
		workDir:         workDir,
		skillRoots:      skillRoots,
		runDir:          absRunDir,
		sandboxMode:     sandboxMode,
		shell:           sh,
		launchLine:      launchCmd,
	})
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	return shuttleengine.Launch{
		Cmd:                  launchCmd,
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
// It carries no creel hooks, since the session has no developer instructions to hold the creel trust sentence.
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
	home, writableRoots, _, err := c.readyHome(codex)
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
// It returns the home, the launch's writable roots and the checkout it read.
// The roots are each symlink-resolved, in this order:
// the records checkout the engine is told, or none outside a hub;
// the checkout's git dir and the common git dir it links to;
// then codex.yaml's writable_roots that exist.
// A records checkout that cannot be resolved is kept as told, with a warning logged.
// The git dirs are listed because a linked worktree's git dirs lie under the main checkout, outside the workspace,
// and a commit writes its index lock, objects and refs there.
func (c *Codex) readyHome(codex Config) (home string, writableRoots []string, found checkout, err error) {
	homes, err := resolveHomes(codex)
	if err != nil {
		return "", nil, checkout{}, err
	}
	anchor := c.dirs.Anchor
	if anchor == "" || !filepath.IsAbs(anchor) {
		return "", nil, checkout{}, fmt.Errorf("codexengine: the engine was told no absolute anchor (%q), so it cannot locate the checkout a launch starts in", anchor)
	}
	found, err = readCheckout(anchor)
	if err != nil {
		return "", nil, checkout{}, err
	}
	if err := c.checkCodexFolders(found, anchor); err != nil {
		return "", nil, checkout{}, err
	}
	configured, err := configuredWritableRoots(codex)
	if err != nil {
		return "", nil, checkout{}, err
	}
	if err := c.prepareHome(homes, found.trustRoots()); err != nil {
		return "", nil, checkout{}, err
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
	return homes.lym, append(writableRoots, configured...), found, nil
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
