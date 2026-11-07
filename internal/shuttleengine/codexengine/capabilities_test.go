// capabilities_test.go tests the parts of Codex's creel hook delivery that run no process.
// They are the hook-input parse and the hook output that hands mail over.

package codexengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHookInput pins the parse of a Codex hook payload.
// PostToolUse is the tool boundary, Stop the turn end with its stop_hook_active, and another event is none.
// A payload that is no JSON object is refused.
func TestHookInput(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    shuttleengine.HookInput
		wantErr bool
	}{
		{name: "PostToolUse", payload: `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`, want: shuttleengine.HookInput{Event: shuttleengine.HookEventToolBoundary}},
		{name: "StopContinued", payload: `{"hook_event_name":"Stop","stop_hook_active":true}`, want: shuttleengine.HookInput{Event: shuttleengine.HookEventStop, StopHookActive: true}},
		{name: "OtherEvent", payload: `{"hook_event_name":"SessionStart"}`, want: shuttleengine.HookInput{}},
		{name: "NotAnObject", payload: `[1]`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New().HookInput([]byte(tt.payload))
			if tt.wantErr {
				if err == nil {
					t.Errorf("HookInput(%s) error = nil; want a refusal", tt.payload)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("HookInput(%s) = %+v, %v; want %+v", tt.payload, got, err, tt.want)
			}
		})
	}
}

// TestHookOutput pins that mail goes out as a block whose reason is the mail on either event.
// The echo is dropped; no mail is the empty hook output.
// It also pins that no hook exit wakes an idle Codex session.
func TestHookOutput(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	mail := `[creel] #1 from "ly:orch": go` + "\n" + `[creel] #2 from "ly:orch": stop`
	block := `{"decision":"block","reason":"[creel] #1 from \"ly:orch\": go\n[creel] #2 from \"ly:orch\": stop"}`
	tests := []struct {
		name  string
		event shuttleengine.HookEvent
		mail  string
		echo  string
		want  string
	}{
		{"ToolBoundaryMail", shuttleengine.HookEventToolBoundary, mail, "", block},
		{"StopMailWithEcho", shuttleengine.HookEventStop, mail, "[creel] Delivered while idle:\n[creel] #0", block},
		{"EchoAlone", shuttleengine.HookEventToolBoundary, "", "[creel] Delivered while idle:\n[creel] #0", "{}"},
		{"Nothing", shuttleengine.HookEventStop, "", "", "{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New().HookOutput(tt.event, tt.mail, tt.echo)
			if err != nil || string(got) != tt.want {
				t.Errorf("HookOutput(%q, %q, %q) = %s, %v; want %s", tt.event, tt.mail, tt.echo, got, err, tt.want)
			}
		})
	}
	if stderr, code := New().RewakeOutput(mail); stderr != nil || code != 0 {
		t.Errorf("RewakeOutput() = %q, %d; want no text and exit 0", stderr, code)
	}
}
