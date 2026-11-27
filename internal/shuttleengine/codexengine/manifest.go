// manifest.go builds the launch manifest of a Codex run.
// Its items come from the launch line Prepare composed and from the instruction file heddle renders for Codex.

package codexengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// sandboxModeSettingKey keys the manifest's settings item for the sandbox mode a launch runs.
const sandboxModeSettingKey = "sandbox_mode"

// guardCandidate is one PreToolUse deny a launch could carry: its manifest key and its hook command.
type guardCandidate struct {
	key     string
	command string
}

// claudeFileCandidate is one CLAUDE.md on the path from the checkout root to the working directory.
type claudeFileCandidate struct {
	// key is the file's slash-separated path from the checkout root.
	key  string
	path string
	// unread marks a file Codex does not read itself, which reaches the session only through the pointer to it.
	unread bool
}

// manifestInputs carries each candidate item of a launch's manifest with the exact text or command its delivery writes,
// and the launch line as composed.
type manifestInputs struct {
	role         string
	promptEngine string
	handWritten  bool

	// sh is the shell launchCmd is quoted for.
	sh        shell.Shell
	launchCmd string

	passages         []shuttleengine.Passage
	instructionFiles []shuttleengine.InstructionItem
	claudeFiles      []claudeFileCandidate
	// pointers is the passage naming the CLAUDE.md files Codex does not read itself.
	pointers string

	// fileEntries are the heddle entries of the file Codex loads, and agentsOverride that file's content.
	fileEntries    []shuttleengine.Passage
	agentsOverride string

	// skills are the preloaded skills, skillRoots the roots they resolve under and skillLoad the load message.
	skills     []string
	skillRoots []string
	skillLoad  string

	skillListing   string
	listedSkills   []string
	pluginContexts []shuttleengine.InstructionItem
	omittedHooks   []shuttleengine.OmittedHook

	guards          []guardCandidate
	batchingCommand string

	denyNotice      string
	subagentPassage string

	sandboxMode           string
	developerInstructions string
}

// guardCandidates returns every PreToolUse deny a launch could install for the told refuse list.
func guardCandidates(refuseRaw []string) []guardCandidate {
	candidates := []guardCandidate{{key: shuttleengine.QuestionDenyKey, command: questionDenyCommand()}}
	for _, row := range shellguard.Rows(foregroundPhrase, refuseRaw) {
		candidates = append(candidates, guardCandidate{key: row.Name, command: guardCommand(row)})
	}
	return candidates
}

// claudeFileCandidates returns each CLAUDE.md in the directories from workDir up to root, which holds workDir.
func claudeFileCandidates(root, workDir string) []claudeFileCandidate {
	var files []claudeFileCandidate
	for dir := workDir; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, instructionFallbackFile)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if relative, err := filepath.Rel(root, path); err == nil {
				files = append(files, claudeFileCandidate{key: filepath.ToSlash(relative), path: path, unread: claudeFileIsUnread(path, workDir)})
			}
		}
		if dir == root || filepath.Dir(dir) == dir {
			return files
		}
	}
}

// codexManifest returns the manifest of a launch.
// A candidate is listed only when the launch line holds what its delivery writes.
// A heddle file entry is listed only when the rendered file holds its text.
// A delivery that did not reach the launch therefore leaves no item.
func codexManifest(in manifestInputs) shuttleengine.Manifest {
	var items []shuttleengine.ManifestItem
	add := func(channel shuttleengine.ManifestChannel, key, engine string) {
		items = append(items, shuttleengine.ManifestItem{Channel: channel, Key: key, Engine: engine, Delivery: shuttleengine.ManifestDelivered})
	}
	// quotedBody strips the outer quotes the shell puts around a word, leaving the escaped form the word holds inside it.
	quotedBody := func(text string) string {
		quoted := in.sh.Quote(text)
		return quoted[1 : len(quoted)-1]
	}
	// inSetting reports whether a TOML string setting on the line holds text, which may be a part of the string.
	inSetting := func(text string) bool {
		if text == "" {
			return false
		}
		encoded := tomlString(text)
		return strings.Contains(in.launchCmd, quotedBody(encoded[1:len(encoded)-1]))
	}
	// A hook setting holds its command as one whole TOML string.
	hooked := func(command string) bool { return strings.Contains(in.launchCmd, quotedBody(tomlString(command))) }

	switch {
	case in.handWritten:
		add(shuttleengine.ManifestChannelPromptEngine, shuttleengine.HandWrittenPromptKey, "")
	case in.promptEngine != "":
		add(shuttleengine.ManifestChannelPromptEngine, shuttleengine.PromptEngineKey, "")
	}
	for _, passage := range in.passages {
		if inSetting(passage.Text) {
			add(shuttleengine.ManifestChannelStandingPassage, passage.Slug, passage.Engine)
		}
	}
	for _, file := range in.instructionFiles {
		if inSetting(file.Text) {
			add(shuttleengine.ManifestChannelInstructionSource, file.Name, "")
		}
	}
	// Codex reads a CLAUDE.md of its own accord only under the fallback setting.
	readsClaudeFiles := strings.Contains(in.launchCmd, in.sh.Quote("project_doc_fallback_filenames="+tomlArray([]string{instructionFallbackFile})))
	pointed := inSetting(in.pointers)
	for _, file := range in.claudeFiles {
		named := pointed && slices.Contains(strings.Split(in.pointers, "\n"), "- "+file.path)
		if file.unread && named || !file.unread && readsClaudeFiles {
			add(shuttleengine.ManifestChannelInstructionSource, file.key, "")
		}
	}
	if strings.HasPrefix(in.agentsOverride, heddleengine.Marker) {
		for _, entry := range in.fileEntries {
			if entry.Text != "" && strings.Contains(in.agentsOverride, entry.Text) {
				add(shuttleengine.ManifestChannelInstructionSource, shuttleengine.HeddleFileKeyPrefix+entry.Slug, entry.Engine)
			}
		}
	}
	if in.skillLoad != "" && strings.Contains(in.launchCmd, in.sh.Quote(in.skillLoad)) {
		for _, skill := range in.skills {
			if file := skillPath(skill, in.skillRoots); file != "" && strings.Contains(in.skillLoad, file) {
				add(shuttleengine.ManifestChannelSkillLoad, skill, "")
			}
		}
	}
	if inSetting(in.skillListing) {
		for _, name := range in.listedSkills {
			add(shuttleengine.ManifestChannelSkillListing, name, "")
		}
	}
	for _, context := range in.pluginContexts {
		if inSetting(context.Text) {
			add(shuttleengine.ManifestChannelPluginContext, context.Plugin+"/"+context.Hook, "")
		}
	}
	for _, hook := range in.omittedHooks {
		if hook.Plugin != "" {
			items = append(items, shuttleengine.ManifestItem{Channel: shuttleengine.ManifestChannelPluginContext, Key: hook.Plugin + "/" + hook.Hook, Delivery: shuttleengine.ManifestOmitted, Reason: hook.Reason})
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
	if inSetting(in.denyNotice) {
		add(shuttleengine.ManifestChannelNotice, shuttleengine.DenyNoticeKey, "")
	}
	if inSetting(shuttleengine.CreelTrustSentence) {
		add(shuttleengine.ManifestChannelNotice, shuttleengine.CreelTrustKey, "")
	}
	if inSetting(in.subagentPassage) {
		add(shuttleengine.ManifestChannelNotice, shuttleengine.SubagentPassageKey, "")
	}
	size := 0
	if in.developerInstructions != "" {
		size = len(developerInstructionsSetting(in.developerInstructions))
	}
	for _, setting := range []struct{ key, detail string }{
		{sandboxModeSettingKey, in.sandboxMode},
		{developerInstructionsKey, fmt.Sprintf("%d bytes", size)},
	} {
		items = append(items, shuttleengine.ManifestItem{
			Channel:  shuttleengine.ManifestChannelSettings,
			Key:      setting.key,
			Engine:   engineName,
			Delivery: shuttleengine.ManifestDelivered,
			Detail:   setting.detail,
		})
	}
	return shuttleengine.Manifest{Engine: engineName, Role: in.role, PromptEngine: in.promptEngine, Items: items}
}
