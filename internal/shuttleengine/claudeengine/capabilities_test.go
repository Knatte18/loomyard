//go:build tier0

// capabilities_test.go verifies Claude's optional capability answers: instruction files, requirement support, resume-context rendering and creel's hook delivery.

package claudeengine

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestClaude_Capabilities pins the instruction file names and their order, the requirements Claude meets and the resume context bytes.
func TestClaude_Capabilities(t *testing.T) {
	t.Parallel()

	c := New()

	if got, want := c.InstructionFiles(), []string{"CLAUDE.md", "CLAUDE.local.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("InstructionFiles() = %v; want %v", got, want)
	}

	supports := []struct {
		req  shuttleengine.Requirement
		want bool
	}{
		{shuttleengine.RequirementForks, true},
		{shuttleengine.RequirementPeerMessaging, true},
		{shuttleengine.RequirementBackgroundWake, true},
		{shuttleengine.Requirement("other"), false},
	}
	for _, tt := range supports {
		if got := c.Supports(tt.req); got != tt.want {
			t.Errorf("Supports(%q) = %t; want %t", tt.req, got, tt.want)
		}
	}

	const text = "resume me"
	want, err := SessionStartContext(text)
	if err != nil {
		t.Fatalf("SessionStartContext: %v", err)
	}
	got, err := c.ResumeContext(text)
	if err != nil {
		t.Fatalf("ResumeContext: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("ResumeContext(%q) = %q; want %q", text, got, want)
	}
}

// TestClaude_HookInput pins what the hook-input parse reads from Claude Code's payload, and the wake output's text and exit code.
func TestClaude_HookInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    shuttleengine.HookInput
		wantErr bool
	}{
		{"MainThreadPostToolUse", `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`, shuttleengine.HookInput{Event: shuttleengine.HookEventToolBoundary}, false},
		{"StopWithTheFlagSet", `{"hook_event_name":"Stop","stop_hook_active":true}`, shuttleengine.HookInput{Event: shuttleengine.HookEventStop, StopHookActive: true}, false},
		{"StopWithTheFlagClear", `{"hook_event_name":"Stop","stop_hook_active":false}`, shuttleengine.HookInput{Event: shuttleengine.HookEventStop}, false},
		{"SubagentPostToolUse", `{"hook_event_name":"PostToolUse","agent_id":"agent-1","agent_type":"general"}`, shuttleengine.HookInput{Event: shuttleengine.HookEventToolBoundary, Subagent: true}, false},
		{"AgentTypeAloneIsTheMainThread", `{"hook_event_name":"PostToolUse","agent_type":"reviewer"}`, shuttleengine.HookInput{Event: shuttleengine.HookEventToolBoundary}, false},
		{"UnrelatedEventHasNoEvent", `{"hook_event_name":"SessionStart"}`, shuttleengine.HookInput{}, false},
		{"NotJSON", `PostToolUse`, shuttleengine.HookInput{}, true},
		{"JSONThatIsNoObject", `null`, shuttleengine.HookInput{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New().HookInput([]byte(tt.payload))
			if (err != nil) != tt.wantErr {
				t.Fatalf("HookInput(%s) error = %v; want error %v", tt.payload, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("HookInput(%s) = %+v; want %+v", tt.payload, got, tt.want)
			}
		})
	}

	stderr, code := New().RewakeOutput("mail waits")
	if string(stderr) != "mail waits\n" || code != 2 {
		t.Errorf("RewakeOutput = (%q, %d); want the text and a newline with exit code 2", stderr, code)
	}
}
