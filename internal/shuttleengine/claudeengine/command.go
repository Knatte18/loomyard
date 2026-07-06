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

// maxLaunchPromptBytes is the largest prompt (in UTF-8 bytes) Prepare
// accepts. The launch line reads the prompt file via `(Get-Content -Raw …)`,
// so the pane's pwsh expands the ENTIRE prompt into one argument of the
// claude process's command line — and Windows caps a CreateProcess command
// line at 32,767 UTF-16 characters. A prompt over that ceiling makes the
// launch itself fail inside the pane ("The command line is too long", proven
// live with a 40 KB prompt), which the run loop can only see as an opaque
// `died` after the full startup window. UTF-8 byte count is a safe upper
// bound on UTF-16 length (every code point's UTF-16 unit count ≤ its UTF-8
// byte count), and the ~2.7 KB left under the ceiling covers the binary
// path, session id, settings path, flags, quoting, and psmux's own claude
// wrapper function on the same line.
const maxLaunchPromptBytes = 30000

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
// inline, so a large or quote-laden prompt never has to survive psmux
// send-keys or pwsh string escaping — though it still becomes one argument
// of the claude process's command line, which is why Prepare bounds it at
// maxLaunchPromptBytes. --model is appended only when model is
// non-empty (an empty value defers to claude's own default) and, like every
// other argument on this line, single-quoted: a raw interpolation would let
// a model value containing a space, quote, or pwsh metacharacter (`;`, `|`,
// `$`) corrupt the single line typed into the pane. The session id is
// single-quoted for the same reason: its locally-minted UUID shape
// ([0-9a-f-]) needs no escaping today, but quoting every interpolated
// argument uniformly is the invariant that stops a future change to how the
// id is sourced from silently reintroducing an injection.
// --dangerously-skip-permissions is appended only when interactive is
// false — the autonomous default (Shared Decision "Interactive bool encodes
// the discussion's Autonomous default true"). The result is exactly one
// line with no embedded newlines, since it is typed into a pane via a
// single send-keys call.
func buildLaunchCmd(bin, promptPath, settingsPath, sessionID, model string, interactive bool) string {
	cmd := fmt.Sprintf(
		"& %s (Get-Content -Raw %s) --session-id %s --settings %s",
		pwshSingleQuote(bin), pwshSingleQuote(promptPath), pwshSingleQuote(sessionID), pwshSingleQuote(settingsPath),
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
		pwshSingleQuote(bin), pwshSingleQuote(sessionID), pwshSingleQuote(settingsPath),
	)
}
