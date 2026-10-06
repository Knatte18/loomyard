// skillload.go implements Claude's one-turn skill load: one typed message asks the model to load a list of skills through the Skill tool,
// and the transcript a Stop payload names says which loads happened.
// The check reads the transcript backward through readBackward and degrades to an unverified report on every failure and never errors,
// since the transcript format is a Claude Code internal.
// All message text and transcript shape knowledge stays in this file, per the Shuttle Provider-Seam Invariant.
package claudeengine

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// SkillLoadMessage returns the single line, with no leading slash, that asks the model to load each of skills through the Skill tool in this one turn.
// The line is engine choreography like the clear and compact sequences, not a stencil.
func (c *Claude) SkillLoadMessage(skills []string) string {
	return "Load these skills now, all in this one turn, with one Skill tool call each, in this order: " +
		strings.Join(skills, ", ") +
		". Do nothing else, and reply only `ok` followed by the skill names in order."
}

// ClassifySkillLoad classifies the load of skills asked by SkillLoadMessage from the transcript turnEnd's transcript_path names.
// The latest main-chain user entry equal to the load message starts the turn,
// and only main-chain entries after it count.
// A skill is loaded when its Skill call is answered by a successful result, unknown when that result is an error or the latest skill listing does not name it, and missing otherwise.
// The report is unverified for a missing transcript_path, an unreadable file, a transcript with no matching load message, or one with no skill listing.
func (c *Claude) ClassifySkillLoad(turnEnd shuttleengine.Event, skills []string) shuttleengine.SkillLoadReport {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return shuttleengine.SkillLoadReport{}
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return shuttleengine.SkillLoadReport{}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return shuttleengine.SkillLoadReport{}
	}
	var report shuttleengine.SkillLoadReport
	readBackward(f, info.Size(), initialReadChunk, func(data []byte) bool {
		var found bool
		report, found = classifyWindow(data, c.SkillLoadMessage(skills), skills)
		return found
	})
	return report
}

// loadEntry is the part of a transcript line the skill load check looks at.
type loadEntry struct {
	Type          string          `json:"type"`
	IsSidechain   bool            `json:"isSidechain"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	Attachment    struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	} `json:"attachment"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// loadBlock is one content block of a transcript message.
type loadBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
	Input     struct {
		Skill string `json:"skill"`
	} `json:"input"`
}

// blocks returns the content blocks of e's message.
// A plain string content is one text block.
func (e loadEntry) blocks() []loadBlock {
	var text string
	if json.Unmarshal(e.Message.Content, &text) == nil {
		return []loadBlock{{Type: "text", Text: text}}
	}
	var blocks []loadBlock
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return nil
	}
	return blocks
}

// text returns the concatenated text blocks of e's message.
func (e loadEntry) text() string {
	var text strings.Builder
	for _, block := range e.blocks() {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

// resultSucceeded reports whether e's toolUseResult carries success: true.
func (e loadEntry) resultSucceeded() bool {
	var result struct {
		Success bool `json:"success"`
	}
	return json.Unmarshal(e.ToolUseResult, &result) == nil && result.Success
}

// classifyWindow classifies the load of skills in the complete lines of data,
// which end at the transcript end.
// It reports found false when the window holds no load message or no skill listing yet,
// so the caller widens the window.
func classifyWindow(data []byte, message string, skills []string) (shuttleengine.SkillLoadReport, bool) {
	var entries []loadEntry
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var e loadEntry
		if json.Unmarshal(bytes.TrimSpace(line), &e) != nil || e.IsSidechain {
			continue
		}
		entries = append(entries, e)
	}

	messageAt := -1
	for i, e := range entries {
		if e.Type == "user" && strings.TrimSpace(e.text()) == message {
			messageAt = i
		}
	}
	var listing map[string]bool
	for _, e := range entries {
		if e.Type == "attachment" && e.Attachment.Type == "skill_listing" {
			listing = listedSkills(e.Attachment.Content)
		}
	}
	if messageAt < 0 || listing == nil {
		return shuttleengine.SkillLoadReport{}, false
	}

	// calls maps a Skill call's tool_use id to its skill and, once answered, whether the result succeeded.
	type call struct {
		skill     string
		answered  bool
		succeeded bool
	}
	calls := map[string]*call{}
	for _, e := range entries[messageAt+1:] {
		for _, block := range e.blocks() {
			switch {
			case e.Type == "assistant" && block.Type == "tool_use" && block.Name == "Skill":
				calls[block.ID] = &call{skill: block.Input.Skill}
			case e.Type == "user" && block.Type == "tool_result":
				if pending, ok := calls[block.ToolUseID]; ok {
					pending.answered = true
					pending.succeeded = !block.IsError && e.resultSucceeded()
				}
			}
		}
	}

	report := shuttleengine.SkillLoadReport{Verified: true}
	for _, skill := range skills {
		var loaded, errored bool
		for _, c := range calls {
			if c.skill != skill || !c.answered {
				continue
			}
			if c.succeeded {
				loaded = true
			} else {
				errored = true
			}
		}
		switch {
		case loaded:
			report.Loaded = append(report.Loaded, skill)
		case errored || !listing[skill]:
			report.Unknown = append(report.Unknown, skill)
		default:
			report.Missing = append(report.Missing, skill)
		}
	}
	return report, true
}

// listedSkills returns the skill names of a skill_listing attachment, one `- <name>: <description>` line each.
func listedSkills(content string) map[string]bool {
	names := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- ")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, ": ")
		names[strings.TrimSpace(name)] = true
	}
	return names
}
