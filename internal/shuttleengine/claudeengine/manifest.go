// manifest.go builds the launch manifest of a Claude run.
// Its items come from the artifacts Prepare wrote and from the engine's resolution of what Claude Code loads itself.

package claudeengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// guardCandidate is one PreToolUse deny a launch could carry: its manifest key and its hook command.
type guardCandidate struct {
	key     string
	command string
}

// manifestInputs carries each candidate item of a launch's manifest with the exact text or command its delivery writes,
// and the artifacts as written.
type manifestInputs struct {
	role         string
	promptEngine string
	handWritten  bool

	// settings and appendPrompt are the written files' bytes, and launchCmd the launch line.
	settings     []byte
	appendPrompt string
	launchCmd    string

	passages        []shuttleengine.Passage
	denyNotice      string
	subagentPassage string

	// skills are the preloaded skills, and skillLoadArgument the quoted load message the launch line passes for them.
	skills            []string
	skillLoadArgument string

	guards          []guardCandidate
	batchingCommand string

	// fileEntries are the heddle entries of the file Claude Code loads, and localInstructions that file's content.
	fileEntries       []shuttleengine.Passage
	localInstructions string

	// native holds the items Claude Code loads itself.
	native []shuttleengine.ManifestItem
}

// guardCandidates returns every PreToolUse deny a launch could install for the told refuse list.
func guardCandidates(refuseRaw []string) []guardCandidate {
	var candidates []guardCandidate
	for _, deny := range claudeDenies() {
		key := shuttleengine.WebsterForkDenyKey
		if deny.matcher == askUserQuestionTool {
			key = shuttleengine.QuestionDenyKey
		}
		candidates = append(candidates, guardCandidate{key: key, command: deny.command})
	}
	for _, row := range shellguard.Rows(bashForegroundPhrase, refuseRaw) {
		candidates = append(candidates, guardCandidate{key: row.Name, command: renderShellGuard(row).command})
	}
	return candidates
}

// claudeManifest returns the manifest of a launch.
// A candidate is listed only when the artifact its delivery writes holds its text or command,
// so a delivery that did not reach the launch leaves no item.
func claudeManifest(in manifestInputs) shuttleengine.Manifest {
	var items []shuttleengine.ManifestItem
	add := func(channel shuttleengine.ManifestChannel, key, engine string) {
		items = append(items, shuttleengine.ManifestItem{Channel: channel, Key: key, Engine: engine, Delivery: shuttleengine.ManifestDelivered})
	}
	appended := func(text string) bool { return text != "" && strings.Contains(in.appendPrompt, text) }
	// A settings document holds a hook command as the JSON string it marshals to.
	hooked := func(command string) bool {
		// A string always marshals.
		encoded, _ := json.Marshal(command)
		return bytes.Contains(in.settings, encoded)
	}

	switch {
	case in.handWritten:
		add(shuttleengine.ManifestChannelPromptEngine, shuttleengine.HandWrittenPromptKey, "")
	case in.promptEngine != "":
		add(shuttleengine.ManifestChannelPromptEngine, shuttleengine.PromptEngineKey, "")
	}
	for _, passage := range in.passages {
		if appended(passage.Text) {
			add(shuttleengine.ManifestChannelStandingPassage, passage.Slug, passage.Engine)
		}
	}
	if in.skillLoadArgument != "" && strings.Contains(in.launchCmd, in.skillLoadArgument) {
		for _, skill := range in.skills {
			add(shuttleengine.ManifestChannelSkillLoad, skill, "")
		}
	}
	for _, guard := range in.guards {
		if hooked(guard.command) {
			add(shuttleengine.ManifestChannelGuard, guard.key, "")
		}
	}
	if hooked(in.batchingCommand) {
		add(shuttleengine.ManifestChannelBatchingNudge, shuttleengine.BatchingNudgeKey, "")
	}
	if appended(in.denyNotice) {
		add(shuttleengine.ManifestChannelNotice, shuttleengine.DenyNoticeKey, "")
	}
	if appended(shuttleengine.CreelTrustSentence) {
		add(shuttleengine.ManifestChannelNotice, shuttleengine.CreelTrustKey, "")
	}
	if appended(in.subagentPassage) {
		add(shuttleengine.ManifestChannelNotice, shuttleengine.SubagentPassageKey, "")
	}
	if strings.HasPrefix(in.localInstructions, heddleengine.Marker) {
		for _, entry := range in.fileEntries {
			if entry.Text != "" && strings.Contains(in.localInstructions, entry.Text) {
				items = append(items, shuttleengine.ManifestItem{Channel: shuttleengine.ManifestChannelInstructionSource, Key: shuttleengine.HeddleFileKeyPrefix + entry.Slug, Engine: entry.Engine, Delivery: shuttleengine.ManifestNative})
			}
		}
	}
	items = append(items, in.native...)
	items = append(items, shuttleengine.ManifestItem{
		Channel:  shuttleengine.ManifestChannelSettings,
		Key:      settingsFileName,
		Engine:   engineName,
		Delivery: shuttleengine.ManifestDelivered,
		Detail:   fmt.Sprintf("%d bytes", len(in.settings)),
	})
	return shuttleengine.Manifest{Engine: engineName, Role: in.role, PromptEngine: in.promptEngine, Items: items}
}

// writeLaunchManifest writes the launch manifest into runDir.
// It first completes in with the written settings, the appended system prompt and what Claude Code loads itself.
func (c *Claude) writeLaunchManifest(runDir string, spec shuttleengine.Spec, settingsPath, appendPromptPath string, in manifestInputs) error {
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("claudeengine: read back %s for the launch manifest: %w", settingsPath, err)
	}
	appendPrompt, err := os.ReadFile(appendPromptPath)
	if err != nil {
		return fmt.Errorf("claudeengine: read back %s for the launch manifest: %w", appendPromptPath, err)
	}
	workDir, err := filepath.EvalSymlinks(c.dirs.Anchor)
	if err != nil {
		return fmt.Errorf("claudeengine: resolve the working directory %s: %w", c.dirs.Anchor, err)
	}
	// An anchor with no repository above it has no root.
	repoRoot := ""
	found, _, err := dotgit.FindRoot(c.dirs.Anchor)
	switch {
	case err == nil:
		repoRoot, err = filepath.EvalSymlinks(found)
		if err != nil {
			return fmt.Errorf("claudeengine: resolve the worktree root %s: %w", found, err)
		}
	case !errors.Is(err, dotgit.ErrNotRepository):
		return fmt.Errorf("claudeengine: find the worktree root of %s: %w", c.dirs.Anchor, err)
	}
	// An absent or unreadable file lists no heddle entry.
	local, _ := os.ReadFile(filepath.Join(workDir, heddleengine.ClaudeLocalFile))

	in.role, in.promptEngine, in.handWritten = spec.Role, spec.PromptEngine, spec.HandWrittenPrompt
	in.settings, in.appendPrompt, in.localInstructions = settings, string(appendPrompt), string(local)
	in.skills = spec.Skills
	in.native = c.nativeManifestItems(filepath.Base(runDir), spec, workDir, repoRoot)

	if err := shuttleengine.WriteManifest(runDir, claudeManifest(in)); err != nil {
		return fmt.Errorf("claudeengine: write the launch manifest %s: %w", filepath.Join(runDir, shuttleengine.ManifestFileName), err)
	}
	return nil
}

// nativeManifestItems returns what Claude Code loads itself for a session in workDir:
// the instruction files, the listed skills and the session-start hooks of the scope's plugins, none of which it runs.
// An empty repoRoot stands for no repository, and then only the working directory's instruction file is listed.
// A user setup that cannot be resolved is logged and lists the worktree's own files alone, so it never blocks a launch.
// Each part the resolution left out with a warning is logged too.
// An empty instruction file in the config directory or above the repository is not listed,
// since it gives the session nothing.
func (c *Claude) nativeManifestItems(sessionID string, spec shuttleengine.Spec, workDir, repoRoot string) []shuttleengine.ManifestItem {
	var items []shuttleengine.ManifestItem
	add := func(channel shuttleengine.ManifestChannel, key string) {
		items = append(items, shuttleengine.ManifestItem{Channel: channel, Key: key, Delivery: shuttleengine.ManifestNative})
	}
	root := repoRoot
	if root == "" {
		root = workDir
	}

	source := &UserSetupSource{dirs: c.dirs, claudeDir: c.claudeDir}
	setup, err := source.UserSetup(shuttleengine.UserSetupRequest{WorkDir: workDir, RepoRoot: root, Skills: spec.Skills, Role: spec.Role, SessionID: sessionID})
	if err != nil {
		logger.Warn("claudeengine: the launch manifest lists no user instruction file, skill or plugin context; way forward: fix the file the error names", "error", err)
	}
	for _, warning := range setup.Warnings {
		logger.Warn("claudeengine: the launch manifest leaves out part of the user setup; way forward: follow the way forward the warning names", "warning", warning)
	}
	if repoRoot != "" {
		for _, file := range setup.InstructionFiles {
			if file.Text != "" {
				add(shuttleengine.ManifestChannelInstructionSource, file.Name)
			}
		}
	}
	// workDir lies in root, so the walk up from it ends there.
	var inRepo []string
	for dir := workDir; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, instructionFileName)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if relative, err := filepath.Rel(root, path); err == nil {
				inRepo = append(inRepo, filepath.ToSlash(relative))
			}
		}
		if dir == root || filepath.Dir(dir) == dir {
			break
		}
	}
	for _, key := range inRepo {
		add(shuttleengine.ManifestChannelInstructionSource, key)
	}
	for _, name := range setup.ListedSkills {
		add(shuttleengine.ManifestChannelSkillListing, name)
	}
	// With no runner every hook is reported as omitted; Claude Code runs each itself.
	for _, hook := range setup.Omitted {
		if hook.Hook != "" {
			add(shuttleengine.ManifestChannelPluginContext, hook.Plugin+"/"+hook.Hook)
		}
	}
	return items
}
