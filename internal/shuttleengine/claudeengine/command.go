// command.go composes the opaque pane-shell command lines Prepare (settings.go)
// hands back as a Launch: the launch line that starts a fresh session and
// the resume line that reattaches an existing one. Both are single-line
// strings typed verbatim into a pane via psmux send-keys (see
// muxengine/spawn.go's launchStrandLocked) — no newline may appear in
// either, since send-keys submits a line at a time. Argument quoting, the
// call operator, and the prompt-file read idiom are pane-shell mechanics
// owned entirely by internal/shell (the Shell Mechanics Seam invariant);
// this file only ever calls into that seam and never emits raw pwsh/posix
// syntax of its own.

package claudeengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shell"
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

// validEfforts is the exact-lowercase set of --effort values claude accepts,
// verified live against `claude --effort`: per-model support is NOT policed
// here (it is invisible from the CLI — a mismatched model/effort pair
// returns success with no signal, proven live), so this set is the entire
// vocabulary validateEffort ever checks against.
var validEfforts = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

// validateEffort reports an error unless effort is either empty (defers to
// claude's own default) or an exact-lowercase member of validEfforts — a
// case-sensitive check, so "High"/"HIGH" are rejected exactly like any other
// unrecognized value. claude itself only warns-and-ignores an unrecognized
// --effort value rather than failing the launch, so shuttle must hard-error
// here instead: silently dropping an operator's effort override would be a
// worse failure mode than refusing to start the run.
func validateEffort(effort string) error {
	if effort == "" {
		return nil
	}
	if validEfforts[effort] {
		return nil
	}
	return fmt.Errorf("claudeengine: invalid effort %q; valid values are low, medium, high, xhigh, max (case-sensitive, exact-lowercase)", effort)
}

// resolveModelID translates a bare-word model plus an optional version pin
// into the model id claudeengine actually launches, implementing the generic
// bare-word rule (discussion decision "claudeengine translation rule"):
// (1) an empty version defers entirely to the caller's model value, unchanged
// (including an empty model — claude's own default); (2) a non-empty version
// with no model has nothing to compose against, so it is a hard error; (3) a
// model already containing a dash is a full model id (e.g. the escape form),
// which already pins its own version — combining it with version is a
// contradiction and a hard error; (4) otherwise model and version compose
// into "claude-<model>-<version, dots as dashes>" (e.g. "sonnet" + "4.5" →
// "claude-sonnet-4-5", "fable" + "5" → "claude-fable-5"). Deliberately NO
// closed alias list: a brand-new provider alias composes correctly on an old
// binary with no recompile, since the rule is purely mechanical string
// composition. A nonsense composition is not caught here — it fails loudly
// downstream at the claude CLI launch itself (fail-loud is preserved; quoting
// in buildLaunchCmd already prevents the composed id from ever being an
// injection vector).
func resolveModelID(model, version string) (string, error) {
	if version == "" {
		return model, nil
	}
	if model == "" {
		return "", fmt.Errorf("claudeengine: version %q given with no model to compose against", version)
	}
	if strings.Contains(model, "-") {
		return "", fmt.Errorf("claudeengine: model %q already contains a dash and pins its own version; combining it with version %q is a contradiction", model, version)
	}
	return "claude-" + model + "-" + strings.ReplaceAll(version, ".", "-"), nil
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

// buildLaunchCmd composes the pane-shell line that starts a fresh claude
// session: the prompt is read from promptPath via sh.ReadFile rather than
// typed inline, so a large or quote-laden prompt never has to survive psmux
// send-keys or shell string escaping — though it still becomes one argument
// of the claude process's command line, which is why Prepare bounds it at
// maxLaunchPromptBytes. --model is appended only when model is
// non-empty (an empty value defers to claude's own default) and, like every
// other interpolated value on this line, quoted via sh.Quote: a raw
// interpolation would let a model value containing a space, quote, or shell
// metacharacter (`;`, `|`, `$`) corrupt the single line typed into the pane.
// The session id is quoted for the same reason: its locally-minted UUID
// shape ([0-9a-f-]) needs no escaping today, but quoting every interpolated
// argument uniformly is the invariant that stops a future change to how the
// id is sourced from silently reintroducing an injection.
// --effort is appended only when effort is non-empty (an empty value defers
// to claude's own default), quoted for the same injection-hardening reason
// as --model, and positioned immediately after --model — Prepare has
// already hard-errored on any effort value validateEffort cannot realize
// before this function is ever called, so buildLaunchCmd itself never
// re-validates.
// --dangerously-skip-permissions is appended only when interactive is
// false — the autonomous default (Shared Decision "Interactive bool encodes
// the discussion's Autonomous default true"). The result is exactly one
// line with no embedded newlines, since it is typed into a pane via a
// single send-keys call. sh supplies every bit of shell syntax (the call
// operator, quoting, and the prompt-file read idiom) so this function never
// hardcodes a pwsh- or posix-specific token.
func buildLaunchCmd(sh shell.Shell, bin, promptPath, settingsPath, sessionID, model, effort string, interactive bool) string {
	cmd := sh.Invoke(bin) + " " + sh.ReadFile(promptPath) +
		" --session-id " + sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath)
	if model != "" {
		cmd += " --model " + sh.Quote(model)
	}
	if effort != "" {
		cmd += " --effort " + sh.Quote(effort)
	}
	if !interactive {
		cmd += " --dangerously-skip-permissions"
	}
	return cmd
}

// buildResumeCmd composes the pane-shell line that reattaches an existing
// claude session by its session id. It always uses --resume <id>, never
// --continue: --continue resumes "the most recent conversation in this
// directory", which is ambiguous the moment two runs share a worktree
// concurrently, whereas --resume <id> names the exact session (discussion
// "Session identity"). sh supplies the call operator and quoting exactly as
// buildLaunchCmd does, so the two builders never diverge on shell syntax.
func buildResumeCmd(sh shell.Shell, bin, settingsPath, sessionID string) string {
	return sh.Invoke(bin) + " --resume " + sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath)
}
