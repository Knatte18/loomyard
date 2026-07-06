// command.go composes the opaque pwsh command lines Prepare (settings.go)
// hands back as a Launch: the launch line that starts a fresh session and
// the resume line that reattaches an existing one. Both are single-line
// strings typed verbatim into a pane via psmux send-keys (see
// muxengine/spawn.go's launchStrandLocked) — no newline may appear in
// either, since send-keys submits a line at a time.

package claudeengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// pwshSingleQuote wraps s in pwsh single quotes, doubling any embedded
// single quote (pwsh's own escape for a literal `'` inside a single-quoted
// string) so a path containing a quote or a space still round-trips as one
// argument.
func pwshSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// claudeBinary resolves which claude executable a launch/resume command
// invokes: cfg.Claude when the operator has configured an explicit path,
// otherwise the bare literal "claude" resolved off PATH.
func claudeBinary(cfg shuttleengine.Config) string {
	if cfg.Claude != "" {
		return cfg.Claude
	}
	return "claude"
}

// buildLaunchCmd composes the pwsh line that starts a fresh claude session:
// the prompt is read from promptPath via Get-Content rather than typed
// inline, so an arbitrarily large or quote-laden prompt never has to survive
// pwsh command-line escaping. --model is appended only when model is
// non-empty (an empty value defers to claude's own default) and, like every
// other argument on this line, single-quoted: a raw interpolation would let
// a model value containing a space, quote, or pwsh metacharacter (`;`, `|`,
// `$`) corrupt the single line typed into the pane.
// --dangerously-skip-permissions is appended only when interactive is
// false — the autonomous default (Shared Decision "Interactive bool encodes
// the discussion's Autonomous default true"). The result is exactly one
// line with no embedded newlines, since it is typed into a pane via a
// single send-keys call.
func buildLaunchCmd(bin, promptPath, settingsPath, sessionID, model string, interactive bool) string {
	cmd := fmt.Sprintf(
		"& %s (Get-Content -Raw %s) --session-id %s --settings %s",
		pwshSingleQuote(bin), pwshSingleQuote(promptPath), sessionID, pwshSingleQuote(settingsPath),
	)
	if model != "" {
		cmd += " --model " + pwshSingleQuote(model)
	}
	if !interactive {
		cmd += " --dangerously-skip-permissions"
	}
	return cmd
}

// buildResumeCmd composes the pwsh line that reattaches an existing claude
// session by its session id. It always uses --resume <id>, never
// --continue: --continue resumes "the most recent conversation in this
// directory", which is ambiguous the moment two runs share a worktree
// concurrently, whereas --resume <id> names the exact session (discussion
// "Session identity").
func buildResumeCmd(bin, settingsPath, sessionID string) string {
	return fmt.Sprintf(
		"& %s --resume %s --settings %s",
		pwshSingleQuote(bin), sessionID, pwshSingleQuote(settingsPath),
	)
}
