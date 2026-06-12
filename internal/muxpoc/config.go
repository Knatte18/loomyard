// config.go — muxpoc configuration: the binary paths and the per-pane launch command.
//
// Defaults are the paths verified during exploration on the dev machine. They are
// overridable by flags so the PoC is not hard-wired to one user. The launch command is a
// template so tests can substitute a cheap placeholder (e.g. `pwsh -c echo`) instead of a
// real, token-spending claude.
package muxpoc

import (
	"fmt"
	"strings"
)

// Config holds the external binaries and launch behavior for a muxpoc run.
type Config struct {
	Psmux  string // psmux binary
	Pwsh   string // PowerShell 7 binary (explicit path — bare `pwsh` is a broken alias under ConPTY)
	Claude string // claude binary (explicit path)
	// LaunchTemplate builds the in-pane shell command for a fresh session given a session id.
	// %CLAUDE% and %SID% are substituted. Default starts an interactive claude.
	LaunchTemplate string
	// ResumeTemplate builds the in-pane shell command to resume an existing session id.
	ResumeTemplate string
}

// DefaultConfig returns the verified-on-dev-machine defaults.
func DefaultConfig() Config {
	return Config{
		Psmux:          `C:\Code\tools\bin\psmux.exe`,
		Pwsh:           `C:\Code\tools\powershell7\pwsh.exe`,
		Claude:         `C:\Users\hanf\.local\bin\claude.exe`,
		LaunchTemplate: `& '%CLAUDE%' --session-id %SID%`,
		ResumeTemplate: `& '%CLAUDE%' --resume %SID%`,
	}
}

// launchCmd renders the fresh-session launch command for a pane.
func (c Config) launchCmd(sessionID string) string {
	return subst(c.LaunchTemplate, c.Claude, sessionID)
}

// resumeCmd renders the resume command for a pane.
func (c Config) resumeCmd(sessionID string) string {
	return subst(c.ResumeTemplate, c.Claude, sessionID)
}

func subst(tmpl, claude, sid string) string {
	tmpl = strings.ReplaceAll(tmpl, "%CLAUDE%", claude)
	tmpl = strings.ReplaceAll(tmpl, "%SID%", sid)
	return tmpl
}

// String gives a human-readable one-liner for diagnostics.
func (c Config) String() string {
	return fmt.Sprintf("psmux=%s pwsh=%s claude=%s", c.Psmux, c.Pwsh, c.Claude)
}
