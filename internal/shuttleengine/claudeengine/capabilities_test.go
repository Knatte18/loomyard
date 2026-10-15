// capabilities_test.go verifies Claude's optional capability answers: instruction files, requirement support and resume-context rendering.

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
