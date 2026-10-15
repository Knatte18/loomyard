// capabilities.go implements Claude's optional engine capabilities.
// They are requirement support, project instruction files and resume-context rendering.

package claudeengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

var (
	_ shuttleengine.RequirementSupporter  = (*Claude)(nil)
	_ shuttleengine.ForkAuditor           = (*Claude)(nil)
	_ shuttleengine.ResumeContextRenderer = (*Claude)(nil)
	_ shuttleengine.InstructionFileLister = (*Claude)(nil)
)

// Supports reports whether Claude Code meets req; it meets each requirement the seam names.
func (c *Claude) Supports(req shuttleengine.Requirement) bool {
	switch req {
	case shuttleengine.RequirementForks, shuttleengine.RequirementPeerMessaging, shuttleengine.RequirementBackgroundWake:
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
