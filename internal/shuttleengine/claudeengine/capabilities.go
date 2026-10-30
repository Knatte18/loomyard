// capabilities.go implements Claude's optional engine capabilities.
// They are requirement support, project instruction files, resume-context rendering and creel's hook delivery: the hook-input parse, the hook output and the wake output.

package claudeengine

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var (
	_ shuttleengine.RequirementSupporter        = (*Claude)(nil)
	_ shuttleengine.ForkAuditor                 = (*Claude)(nil)
	_ shuttleengine.ResumeContextRenderer       = (*Claude)(nil)
	_ shuttleengine.ToolBoundaryContextRenderer = (*Claude)(nil)
	_ shuttleengine.InstructionFileLister       = (*Claude)(nil)
)

// Supports reports whether Claude Code meets req; it meets each requirement the seam names.
func (c *Claude) Supports(req shuttleengine.Requirement) bool {
	switch req {
	case shuttleengine.RequirementForks, shuttleengine.RequirementBackgroundWake:
		return true
	}
	return false
}

// InstructionFiles returns the file names Claude Code loads as project instructions.
func (c *Claude) InstructionFiles() []string {
	return []string{"CLAUDE.md", "CLAUDE.local.md"}
}

// ResumeContext renders text as Claude Code's session-start hook output.
func (c *Claude) ResumeContext(text string) ([]byte, error) {
	return SessionStartContext(text)
}

// HookInput parses the JSON object Claude Code passes a hook on standard input.
// PostToolUse maps to the tool-boundary event and Stop to the stop event; any other event leaves Event empty.
// A non-empty agent_id marks a subagent or fork; agent_type does not, since the --agent flag sets it for the main thread too.
func (c *Claude) HookInput(payload []byte) (shuttleengine.HookInput, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte("{")) {
		return shuttleengine.HookInput{}, fmt.Errorf("parse hook input: the payload is not a JSON object")
	}
	var parsed struct {
		HookEventName  string `json:"hook_event_name"`
		AgentID        string `json:"agent_id"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return shuttleengine.HookInput{}, fmt.Errorf("parse hook input: %w", err)
	}
	input := shuttleengine.HookInput{Subagent: parsed.AgentID != "", StopHookActive: parsed.StopHookActive}
	switch parsed.HookEventName {
	case hookEventPostToolUse:
		input.Event = shuttleengine.HookEventToolBoundary
	case hookEventStop:
		input.Event = shuttleengine.HookEventStop
	}
	return input, nil
}

// HookOutput renders the JSON a PostToolUse or Stop hook prints to hand mail to the model as additional context, never as a block.
// After a tool call the operator's system message is echo and a newline before mail, or mail alone with no echo;
// on a Stop it is echo alone, absent with no echo.
// With echo and no mail the output is the system message alone, and every empty key is omitted.
func (c *Claude) HookOutput(event shuttleengine.HookEvent, mail, echo string) ([]byte, error) {
	type specificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	output := struct {
		SystemMessage      string          `json:"systemMessage,omitempty"`
		HookSpecificOutput *specificOutput `json:"hookSpecificOutput,omitempty"`
	}{SystemMessage: echo}
	if mail != "" {
		eventName := hookEventPostToolUse
		if event == shuttleengine.HookEventStop {
			eventName = hookEventStop
		} else if echo == "" {
			output.SystemMessage = mail
		} else {
			output.SystemMessage = echo + "\n" + mail
		}
		output.HookSpecificOutput = &specificOutput{HookEventName: eventName, AdditionalContext: mail}
	}
	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal hook output: %w", err)
	}
	return data, nil
}

// RewakeOutput returns text plus a newline and exit code 2, which an asyncRewake hook writes to wake an idle session.
func (c *Claude) RewakeOutput(text string) (stderr []byte, exitCode int) {
	return []byte(text + "\n"), 2
}
