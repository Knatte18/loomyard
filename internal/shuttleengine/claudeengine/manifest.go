// manifest.go builds the launch manifest Claude's Prepare writes: one item per thing the session receives.
// Each candidate item names the text or hook command its delivery puts into an artifact of the launch,
// and the manifest keeps the candidate only when the artifact as written holds it.

package claudeengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The keys of the manifest items that are no skill, passage, file or shell-guard row.
const (
	promptEngineKey      = "prompt-engine"
	handWrittenPromptKey = "hand-written"
	questionDenyKey      = "question-deny"
	batchingNudgeItemKey = "batching-nudge"
	denyNoticeKey        = "deny-notice"
	creelTrustKey        = "creel-trust"
	subagentPassageKey   = "subagent-passage"
	settingsKey          = "settings"
	heddleFileKeyPrefix  = "heddle-file/"
)

// artifact names the place a candidate item's delivery is read back from.
type artifact int

const (
	// inNothing marks an item the provider loads itself or that no artifact carries; the manifest keeps it as listed.
	inNothing artifact = iota
	// inAppendedPrompt is the appended system prompt file.
	inAppendedPrompt
	// inLaunchLine is the launch command line.
	inLaunchLine
	// inMarkedLocalInstructions is the anchor's local instruction file, read only when heddle's marker opens it.
	inMarkedLocalInstructions
	// inPreToolUseHooks is the commands of the settings document's PreToolUse hooks.
	inPreToolUseHooks
	// inPostToolUseHooks is the commands of the settings document's PostToolUse hooks.
	inPostToolUseHooks
)

// manifestCandidate is one item the launch may deliver.
// proof is the exact text or hook command the artifact holds when the delivery reached the launch.
type manifestCandidate struct {
	item  shuttleengine.ManifestItem
	in    artifact
	proof string
}

// manifestInputs are the artifacts of one launch as written, and the items its deliveries would give.
type manifestInputs struct {
	role         string
	promptEngine string
	// settings is the bytes of the settings document.
	settings []byte
	// appendedPrompt is the text of the appended system prompt file.
	appendedPrompt string
	// launchLine is the launch command the engine returned.
	launchLine string
	// localInstructions is the text of the anchor's local instruction file, empty when it is absent.
	localInstructions string
	candidates        []manifestCandidate
}

// claudeManifest returns the manifest of a launch: each candidate its artifact holds, then the settings item.
func claudeManifest(in manifestInputs) shuttleengine.Manifest {
	preToolUse := hookCommands(in.settings, "PreToolUse")
	postToolUse := hookCommands(in.settings, "PostToolUse")
	var items []shuttleengine.ManifestItem
	for _, candidate := range in.candidates {
		var held bool
		switch candidate.in {
		case inNothing:
			held = true
		case inAppendedPrompt:
			held = strings.Contains(in.appendedPrompt, candidate.proof)
		case inLaunchLine:
			held = strings.Contains(in.launchLine, candidate.proof)
		case inMarkedLocalInstructions:
			held = strings.HasPrefix(in.localInstructions, heddleengine.Marker) && strings.Contains(in.localInstructions, candidate.proof)
		case inPreToolUseHooks:
			held = slices.Contains(preToolUse, candidate.proof)
		case inPostToolUseHooks:
			held = slices.Contains(postToolUse, candidate.proof)
		}
		if held {
			items = append(items, candidate.item)
		}
	}
	items = append(items, shuttleengine.ManifestItem{
		Channel:  shuttleengine.ChannelSettings,
		Key:      settingsKey,
		Engine:   engineName,
		Delivery: shuttleengine.DeliveryDelivered,
		Detail:   fmt.Sprintf("%d bytes", len(in.settings)),
	})
	return shuttleengine.Manifest{Engine: engineName, Role: in.role, PromptEngine: in.promptEngine, Items: items}
}

// hookCommands returns the commands of the hooks the settings document holds for event.
// The document was marshalled by buildSettings, so it always decodes.
func hookCommands(settings []byte, event string) []string {
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	_ = json.Unmarshal(settings, &doc)
	var commands []string
	for _, entry := range doc.Hooks[event] {
		for _, hook := range entry.Hooks {
			commands = append(commands, hook.Command)
		}
	}
	return commands
}

// launchArtifacts is what Prepare composed and wrote for a launch, which the manifest reads back.
type launchArtifacts struct {
	spec      shuttleengine.Spec
	cfg       shuttleengine.Config
	sessionID string
	// settingsPath and appendPromptPath are the files Prepare wrote.
	settingsPath     string
	appendPromptPath string
	// launchLine is the launch command Prepare returns.
	launchLine string
	// eventsPathPosix is the events path the settings' hook commands name.
	eventsPathPosix string
	// notice and subagentPassage are the appended prompt's deny notice and subagent passage, empty when it has none.
	notice          string
	subagentPassage string
	standing        shuttleengine.Standing
}

// writeLaunchManifest writes the launch's manifest into runDir.
// It reads the artifacts back from the run directory and the launch line,
// so a delivery dropped from Prepare drops its item.
func (c *Claude) writeLaunchManifest(runDir string, launched launchArtifacts) error {
	in, err := c.manifestInputsFor(launched)
	if err != nil {
		return err
	}
	if err := shuttleengine.WriteManifest(runDir, claudeManifest(in)); err != nil {
		return fmt.Errorf("write launch manifest in %s: %w; way forward: make the run directory writable", runDir, err)
	}
	return nil
}

// manifestInputsFor reads the launch's artifacts and lists every item their deliveries may give.
func (c *Claude) manifestInputsFor(launched launchArtifacts) (manifestInputs, error) {
	spec := launched.spec
	settings, err := os.ReadFile(launched.settingsPath)
	if err != nil {
		return manifestInputs{}, fmt.Errorf("read back the settings %s: %w", launched.settingsPath, err)
	}
	appended, err := os.ReadFile(launched.appendPromptPath)
	if err != nil {
		return manifestInputs{}, fmt.Errorf("read back the appended system prompt %s: %w", launched.appendPromptPath, err)
	}
	workDir, err := filepath.EvalSymlinks(c.dirs.Anchor)
	if err != nil {
		return manifestInputs{}, fmt.Errorf("claudeengine: resolve the working directory %s: %w", c.dirs.Anchor, err)
	}
	root, err := repositoryRoot(workDir)
	if err != nil {
		return manifestInputs{}, err
	}
	local, err := os.ReadFile(filepath.Join(workDir, heddleengine.ClaudeLocalInstructionsFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return manifestInputs{}, fmt.Errorf("read back %s: %w", filepath.Join(workDir, heddleengine.ClaudeLocalInstructionsFile), err)
	}

	in := manifestInputs{
		role:              spec.Role,
		promptEngine:      spec.PromptEngine,
		settings:          settings,
		appendedPrompt:    string(appended),
		launchLine:        launched.launchLine,
		localInstructions: string(local),
	}
	in.candidates = append(in.candidates, promptEngineCandidate(spec))
	in.candidates = append(in.candidates, standingCandidates(launched.standing)...)
	in.candidates = append(in.candidates, noticeCandidates(launched)...)
	in.candidates = append(in.candidates, guardCandidates(launched.cfg.RefuseRaw)...)
	in.candidates = append(in.candidates, manifestCandidate{
		item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelBatchingNudge, Key: batchingNudgeItemKey, Delivery: shuttleengine.DeliveryDelivered},
		in:    inPostToolUseHooks,
		proof: batchingNudgeHookCommand(shQuote(path.Join(path.Dir(launched.eventsPathPosix), batchingNudgeStateFileName))),
	})
	for _, skill := range spec.Skills {
		in.candidates = append(in.candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelSkillLoad, Key: skill, Delivery: shuttleengine.DeliveryDelivered},
			in:    inLaunchLine,
			proof: strings.Join(spec.Skills, ", "),
		})
	}
	in.candidates = append(in.candidates, c.resolvedCandidates(launched, workDir, root)...)
	return in, nil
}

// repositoryRoot returns the root of the repository workDir lies in, with symlinks resolved.
// It returns "" for a working directory with no repository above it.
func repositoryRoot(workDir string) (string, error) {
	root, _, err := dotgit.FindRoot(workDir)
	if errors.Is(err, dotgit.ErrNotRepository) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("claudeengine: find the repository root of %s: %w", workDir, err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("claudeengine: resolve the repository root %s: %w", root, err)
	}
	return resolved, nil
}

// promptEngineCandidate returns the item naming how the spec's prompt was fitted to this engine.
func promptEngineCandidate(spec shuttleengine.Spec) manifestCandidate {
	key := promptEngineKey
	if spec.HandWrittenPrompt {
		key = handWrittenPromptKey
	}
	return manifestCandidate{item: shuttleengine.ManifestItem{Channel: shuttleengine.ChannelPromptEngine, Key: key, Delivery: shuttleengine.DeliveryDelivered}}
}

// standingCandidates returns one item per heddle passage appended to the system prompt.
// It also returns one item per heddle file entry rendered into the local instruction file.
func standingCandidates(standing shuttleengine.Standing) []manifestCandidate {
	var candidates []manifestCandidate
	for _, passage := range standing.Passages {
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelStandingPassage, Key: passage.Slug, Engine: passage.Engine, Delivery: shuttleengine.DeliveryDelivered},
			in:    inAppendedPrompt,
			proof: passage.Text,
		})
	}
	for _, entry := range standing.FileEntries {
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelInstructionSource, Key: heddleFileKeyPrefix + entry.Slug, Engine: entry.Engine, Delivery: shuttleengine.DeliveryDelivered},
			in:    inMarkedLocalInstructions,
			proof: entry.Text,
		})
	}
	return candidates
}

// noticeCandidates returns the fixed sentences the appended system prompt carries.
func noticeCandidates(launched launchArtifacts) []manifestCandidate {
	notices := []struct{ key, text string }{
		{denyNoticeKey, launched.notice},
		{creelTrustKey, shuttleengine.CreelTrustSentence},
		{subagentPassageKey, launched.subagentPassage},
	}
	var candidates []manifestCandidate
	for _, notice := range notices {
		if notice.text == "" {
			continue
		}
		candidates = append(candidates, manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelNotice, Key: notice.key, Delivery: shuttleengine.DeliveryDelivered},
			in:    inAppendedPrompt,
			proof: notice.text,
		})
	}
	return candidates
}

// guardCandidates returns one item per PreToolUse deny lym may install, keyed by its engine-neutral name.
// The settings document holds a command only for a deny the run installs.
func guardCandidates(refuseRaw []string) []manifestCandidate {
	guard := func(key, command string) manifestCandidate {
		return manifestCandidate{
			item:  shuttleengine.ManifestItem{Channel: shuttleengine.ChannelGuard, Key: key, Delivery: shuttleengine.DeliveryDelivered},
			in:    inPreToolUseHooks,
			proof: command,
		}
	}
	claudeKeys := map[string]string{steerWebsterForkDeny: shuttleengine.WebsterForkDenyKey, steerAskUserQuestionDeny: questionDenyKey}
	var candidates []manifestCandidate
	for _, deny := range claudeDenies() {
		candidates = append(candidates, guard(claudeKeys[deny.steer], deny.command))
	}
	for _, row := range shellguard.Rows(bashForegroundPhrase, refuseRaw) {
		candidates = append(candidates, guard(row.Name, renderShellGuard(row).command))
	}
	return candidates
}

// resolvedCandidates returns the items the provider loads itself.
// They are listed from the engine's own resolution of the session's setup:
// the instruction files, the listed skills and the session-start hooks of the plugins the scope turns on.
// The resolution runs no hook.
// A resolution that fails is logged and lists none of them,
// so the manifest then holds no instruction file above the working directory, listed skill or plugin context.
func (c *Claude) resolvedCandidates(launched launchArtifacts, workDir, root string) []manifestCandidate {
	native := func(channel shuttleengine.ManifestChannel, key string) manifestCandidate {
		return manifestCandidate{item: shuttleengine.ManifestItem{Channel: channel, Key: key, Delivery: shuttleengine.DeliveryNative}}
	}
	resolutionRoot := root
	if resolutionRoot == "" {
		resolutionRoot = workDir
	}
	source := &UserSetupSource{dirs: c.dirs, claudeDir: c.claudeDir}
	setup, err := source.UserSetup(shuttleengine.UserSetupRequest{
		WorkDir:   workDir,
		RepoRoot:  resolutionRoot,
		Skills:    launched.spec.Skills,
		Role:      launched.spec.Role,
		SessionID: launched.sessionID,
	})
	if err != nil {
		logger.Warn("claudeengine: the user setup cannot be resolved, so the launch manifest lists no instruction file above the working directory, listed skill or plugin context, and a Codex launch refuses on the same source error; way forward: fix the error and launch again", "error", err)
	}

	var candidates []manifestCandidate
	for _, file := range setup.InstructionFiles {
		candidates = append(candidates, native(shuttleengine.ChannelInstructionSource, file.Name))
	}
	for _, key := range workDirInstructionKeys(root, workDir) {
		candidates = append(candidates, native(shuttleengine.ChannelInstructionSource, key))
	}
	for _, skill := range setup.ListedSkills {
		candidates = append(candidates, native(shuttleengine.ChannelSkillListing, skill))
	}
	for _, hook := range setup.Omitted {
		if hook.Hook == "" || hook.Reason != hookNotRunReason {
			candidates = append(candidates, manifestCandidate{item: shuttleengine.ManifestItem{
				Channel: shuttleengine.ChannelPluginContext, Key: hook.Name, Delivery: shuttleengine.DeliveryOmitted, Reason: hook.Reason,
			}})
			continue
		}
		candidates = append(candidates, native(shuttleengine.ChannelPluginContext, hook.Plugin+"/"+hook.Hook))
	}
	return candidates
}

// workDirInstructionKeys returns the key of each CLAUDE.md on the path from the repository root to workDir that exists.
// A key is the file's slash-separated path relative to root, outermost first.
// With no repository above workDir, root is empty and only workDir's own file is listed, as `CLAUDE.md`.
func workDirInstructionKeys(root, workDir string) []string {
	exists := func(dir string) bool {
		info, err := os.Stat(filepath.Join(dir, instructionFileName))
		return err == nil && info.Mode().IsRegular()
	}
	if root == "" {
		if exists(workDir) {
			return []string{instructionFileName}
		}
		return nil
	}
	var keys []string
	for dir := workDir; ; dir = filepath.Dir(dir) {
		if exists(dir) {
			relative, err := filepath.Rel(root, filepath.Join(dir, instructionFileName))
			if err == nil {
				keys = append(keys, filepath.ToSlash(relative))
			}
		}
		if dir == root || dir == filepath.Dir(dir) {
			break
		}
	}
	slices.Reverse(keys)
	return keys
}
