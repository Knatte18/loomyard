// command.go composes the opaque pane-shell command lines Prepare (settings.go) hands back as a
// Launch: the launch line that starts a fresh session and the resume line that reattaches an
// existing one.
// Both are single-line strings typed verbatim into a pane via tmux send-keys (see
// reedengine/spawn.go's launchStrandLocked) — no newline may appear in either, since send-keys
// submits a line at a time.
// Argument quoting, the call operator, and the prompt-file read idiom are pane-shell mechanics
// owned entirely by internal/shell (the Shell Mechanics Seam invariant);
// this file only ever calls into that seam and never emits raw pwsh/posix syntax of its own.

package claudeengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// maxLaunchPromptBytes is the largest prompt Prepare accepts without failing.
// The Windows command-line limit is 32,767 UTF-16 characters; the launch line
// expands the entire prompt into one argument, so this bounds it safely.
const maxLaunchPromptBytes = 30000

// validEfforts is the set of lowercase --effort values claude accepts.
var validEfforts = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

// validateEffort reports an error unless effort is empty or an exact-lowercase member of validEfforts.
// claude ignores invalid efforts rather than failing, so shuttle must reject them here to prevent silent drops.
func validateEffort(effort string) error {
	if effort == "" {
		return nil
	}
	if validEfforts[effort] {
		return nil
	}
	return fmt.Errorf("claudeengine: invalid effort %q; valid values are low, medium, high, xhigh, max (case-sensitive, exact-lowercase)", effort)
}

// resolveModelID translates a bare-word model plus an optional version into the final model id.
// Empty version defers to the caller's model; a version with no model or a dashed model with version is an error.
// Otherwise, model and version compose into "claude-<model>-<version, dots as dashes>" (e.g. "sonnet" + "4.5" → "claude-sonnet-4-5").
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

// claudeBinary returns cfg.Claude if set, otherwise "claude".
func claudeBinary(cfg shuttleengine.Config) string {
	if cfg.Claude != "" {
		return cfg.Claude
	}
	return "claude"
}

// forkSubagentEnvKey is the staged-rollout flag (Claude Code v2.1.117+) enabling built-in fork subagents.
// It must ride the pane command because the reed server env is scrubbed of CLAUDE_CODE_* at boot.
const forkSubagentEnvKey = "CLAUDE_CODE_FORK_SUBAGENT"

// buildLaunchCmd composes the pane-shell line that starts a fresh claude session.
// It reads the prompt via sh.ReadFile, quotes all interpolated values, and appends --effort/--model only when non-empty.
// When forkSubagents is true, it wraps the line via sh.WithEnv to enable fork subagent type.
func buildLaunchCmd(sh shell.Shell, bin, promptPath, settingsPath, sessionID, model, effort string, interactive, forkSubagents bool) string {
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
	if forkSubagents {
		cmd = sh.WithEnv(forkSubagentEnvKey, "1", cmd)
	}
	return cmd
}

// buildResumeCmd composes the pane-shell line that reattaches an existing claude session by id.
// It always uses --resume, never --continue, to avoid ambiguity under concurrent runs.
//
// It carries the SAME model, effort, and permission mode as buildLaunchCmd, because reed replays
// this string verbatim when it rebuilds a session (lifecycle.go's resume path) and the resumed
// process is a fresh claude that inherits none of them from the launch.
// Dropping them silently downgraded a resumed run: an autonomous run came back permission-gated and
// stalled at its first tool dialog with no operator present, which shuttle can only classify as a
// timeout, and it came back on the provider default model rather than the one the caller pinned.
// When forkSubagents is true, the line is wrapped to keep the fork-subagent capability.
func buildResumeCmd(sh shell.Shell, bin, settingsPath, sessionID, model, effort string, interactive, forkSubagents bool) string {
	cmd := sh.Invoke(bin) + " --resume " + sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath)
	if model != "" {
		cmd += " --model " + sh.Quote(model)
	}
	if effort != "" {
		cmd += " --effort " + sh.Quote(effort)
	}
	if !interactive {
		cmd += " --dangerously-skip-permissions"
	}
	if forkSubagents {
		cmd = sh.WithEnv(forkSubagentEnvKey, "1", cmd)
	}
	return cmd
}
