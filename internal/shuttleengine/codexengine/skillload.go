// skillload.go implements Codex's one-turn skill load: one line asks the model to read each named skill's SKILL.md by absolute path and follow it,
// and the rollout the turn end names says which of those files a tool call of that turn named.
// A skill is a file the model reads, since Codex's own skill discovery outside a repository is untested and a typed `$` opens a popup that takes the next Enter.

package codexengine

import (
	"cmp"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.SkillLoader = (*Codex)(nil)

// defaultSkillLoadTimeout bounds one load turn of the whole skill list: a load turn is one short model turn.
const defaultSkillLoadTimeout = 2 * time.Minute

// The folder and file names of a skill in the Claude plugin cache's layout.
const (
	skillsDirName = "skills"
	skillFileName = "SKILL.md"
)

// DefaultSkillLoadTimeout returns the bound on one load turn of the whole skill list.
func (c *Codex) DefaultSkillLoadTimeout() time.Duration {
	return defaultSkillLoadTimeout
}

// SkillLoadMessage returns the one line that asks the model to read, in this one turn and in order, the SKILL.md of each of skills found under codex.yaml's skill roots, by absolute path, and follow it.
// A skill found under no root is left out of the line, and ClassifySkillLoad reports it unknown.
func (c *Codex) SkillLoadMessage(skills []string) string {
	return skillLoadOver(skills, c.skillRoots())
}

// skillLoadOver returns the load line over the SKILL.md files skills have under the already expanded roots.
func skillLoadOver(skills, roots []string) string {
	return skillLoadLine(resolvedSkillPaths(skills, roots))
}

// ClassifySkillLoad classifies the load of skills SkillLoadMessage asked from the rollout turnEnd's transcript_path names.
// The latest user message that is a load message starts the turn, and only the tool calls after it count;
// a load message is recognised by its fixed text, not by the paths it names, so a skill file that appeared after the launch does not hide it.
// A skill is unknown when no skill root holds it, loaded when a call names its skills/<name>/SKILL.md tail, and missing otherwise.
// The report is unverified when the rollout cannot be read or holds no load message.
func (c *Codex) ClassifySkillLoad(turnEnd shuttleengine.Event, skills []string) shuttleengine.SkillLoadReport {
	data, err := os.ReadFile(payloadField(turnEnd.Raw, payloadTranscriptPath))
	if err != nil {
		return shuttleengine.SkillLoadReport{}
	}
	roots := c.skillRoots()
	calls, found := loadTurnCalls(data)
	if !found {
		return shuttleengine.SkillLoadReport{}
	}
	report := shuttleengine.SkillLoadReport{Verified: true}
	for _, skill := range skills {
		tail := skillFileTail(skill)
		switch {
		case skillPath(skill, roots) == "":
			report.Unknown = append(report.Unknown, skill)
		case slices.ContainsFunc(calls, func(call string) bool { return strings.Contains(call, tail) }):
			report.Loaded = append(report.Loaded, skill)
		default:
			report.Missing = append(report.Missing, skill)
		}
	}
	return report
}

// The fixed texts of the load message: the whole line when no skill file was found, and the opening before the paths otherwise.
const (
	skillLoadNone    = "No skill file is to be read now. Do nothing else, and reply only `ok`."
	skillLoadOpening = "Read each of these skill files in full now, all in this one turn, in this order, and follow each as standing instructions for the rest of this session: "
)

// skillLoadLine returns the load message over the SKILL.md files at paths.
func skillLoadLine(paths []string) string {
	if len(paths) == 0 {
		return skillLoadNone
	}
	return skillLoadOpening +
		strings.Join(paths, ", ") +
		". Do nothing else, and reply only `ok` followed by the skill names in order."
}

// isSkillLoadMessage reports whether text is a load message, whatever skill files it names.
func isSkillLoadMessage(text string) bool {
	return text == skillLoadNone || strings.HasPrefix(text, skillLoadOpening)
}

// skillRoots returns codex.yaml's skill roots with a leading ~/ expanded.
// A root that resolves to no absolute path is left out with a Warn, and a codex.yaml that does not load leaves none, with a Warn.
func (c *Codex) skillRoots() []string {
	codex, err := LoadConfig(c.dirs)
	if err != nil {
		c.warn("codexengine: codex.yaml did not load, so no skill resolves", "error", err)
		return nil
	}
	return c.expandSkillRoots(codex)
}

// expandSkillRoots returns codex's skill roots with a leading ~/ expanded, leaving out with a Warn a root that resolves to no absolute path.
func (c *Codex) expandSkillRoots(codex Config) []string {
	var roots []string
	for _, root := range codex.SkillRoots {
		expanded, err := expandHomePath("skill_roots", root)
		if err != nil {
			c.warn("codexengine: skill root left out", "error", err)
			continue
		}
		roots = append(roots, expanded)
	}
	return roots
}

// resolvedSkillPaths returns the SKILL.md path of each of skills that roots hold, in order.
func resolvedSkillPaths(skills, roots []string) []string {
	var paths []string
	for _, skill := range skills {
		if found := skillPath(skill, roots); found != "" {
			paths = append(paths, found)
		}
	}
	return paths
}

// skillPath returns the SKILL.md path of skill, `<plugin>:<name>`, under the first of roots that holds it, in the Claude plugin cache's layout:
// `<root>/<marketplace>/<plugin>/<version>/skills/<name>/SKILL.md`, the newest version's.
// It returns "" for a skill that names no plugin or that no root holds.
func skillPath(skill string, roots []string) string {
	plugin, name, ok := strings.Cut(skill, ":")
	if !ok || plugin == "" || name == "" {
		return ""
	}
	for _, root := range roots {
		matches := skillFiles(root, plugin, name)
		if len(matches) == 0 {
			continue
		}
		return slices.MaxFunc(matches, func(a, b string) int {
			return compareVersions(skillVersion(a), skillVersion(b))
		})
	}
	return ""
}

// skillFiles returns the SKILL.md of plugin's skill name in every marketplace and version folder under root, in folder order.
// It lists folders rather than globbing, so a root holding a glob metacharacter resolves as written; an unreadable folder holds no skill.
func skillFiles(root, plugin, name string) []string {
	var files []string
	marketplaces, _ := os.ReadDir(root)
	for _, marketplace := range marketplaces {
		pluginDir := filepath.Join(root, marketplace.Name(), plugin)
		versions, _ := os.ReadDir(pluginDir)
		for _, version := range versions {
			file := filepath.Join(pluginDir, version.Name(), skillsDirName, name, skillFileName)
			if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
				files = append(files, file)
			}
		}
	}
	return files
}

// skillVersion returns the version folder's name of a SKILL.md path in the plugin cache's layout.
func skillVersion(skillFile string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(skillFile))))
}

// compareVersions orders two dotted versions field by field under one total order on fields:
// two numeric fields compare as numbers, a numeric field orders above a text one, and two text fields compare as text.
// A field with a numeric prefix and a suffix, such as a pre-release's 0-beta, is text, so a release orders above its pre-release.
// A version that is a prefix of the other orders first.
func compareVersions(a, b string) int {
	aFields, bFields := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aFields) && i < len(bFields); i++ {
		if order := compareVersionFields(aFields[i], bFields[i]); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(aFields), len(bFields))
}

// compareVersionFields orders two version fields: numbers by value, any number above any text, and text by its bytes.
func compareVersionFields(a, b string) int {
	aNumber, aErr := strconv.ParseUint(a, 10, 64)
	bNumber, bErr := strconv.ParseUint(b, 10, 64)
	switch {
	case aErr == nil && bErr == nil:
		return cmp.Compare(aNumber, bNumber)
	case aErr == nil:
		return 1
	case bErr == nil:
		return -1
	default:
		return strings.Compare(a, b)
	}
}

// skillFileTail returns the path tail a read of skill's SKILL.md names: skills/<name>/SKILL.md, <name> being what follows the plugin's colon.
func skillFileTail(skill string) string {
	_, name, _ := strings.Cut(skill, ":")
	return path.Join(skillsDirName, name, skillFileName)
}

// Rollout line and payload types the skill load check reads.
const (
	rolloutResponseItem = "response_item"
	payloadMessage      = "message"
	payloadUserMessage  = "user_message"
	roleUser            = "user"
)

// toolCallPayloads are the response item types that carry a tool call and its arguments.
var toolCallPayloads = []string{"function_call", "custom_tool_call", "local_shell_call"}

// loadTurnCalls returns the raw payload of every tool call after the latest load message in the rollout data, and whether such a message was found.
// A user message is a response item of role user, its text blocks joined, or a user_message event.
func loadTurnCalls(data []byte) ([]string, bool) {
	var calls []string
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		var item struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &item) != nil {
			continue
		}
		var payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Message string `json:"message"`
		}
		if json.Unmarshal(item.Payload, &payload) != nil {
			continue
		}
		switch {
		case item.Type == rolloutEventMessage && payload.Type == payloadUserMessage:
			if isSkillLoadMessage(strings.TrimSpace(payload.Message)) {
				calls, found = nil, true
			}
		case item.Type == rolloutResponseItem && payload.Type == payloadMessage && payload.Role == roleUser:
			if isSkillLoadMessage(strings.TrimSpace(messageText(item.Payload))) {
				calls, found = nil, true
			}
		case item.Type == rolloutResponseItem && found && slices.Contains(toolCallPayloads, payload.Type):
			calls = append(calls, string(item.Payload))
		}
	}
	return calls, found
}

// messageText returns the joined text blocks of a message payload, or "" when its content is no list of blocks.
func messageText(payload json.RawMessage) string {
	var message struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(payload, &message) != nil {
		return ""
	}
	var text strings.Builder
	for _, block := range message.Content {
		text.WriteString(block.Text)
	}
	return text.String()
}
