// command.go composes the opaque pane-shell command lines Prepare (settings.go) hands back as a
// Launch: the launch line that starts a fresh session and the resume line that reattaches an
// existing one.
// Both are single-line strings typed verbatim into a pane via tmux send-keys (see
// reedengine/spawn.go's launchStrandLocked) — no newline may appear in either, since send-keys
// submits a line at a time.
// Argument quoting and the call operator are pane-shell mechanics
// owned entirely by internal/shell (the Shell Mechanics Seam invariant);
// this file only ever calls into that seam and never emits raw pwsh/posix syntax of its own.

package claudeengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// maxLaunchPromptBytes is the largest launch argument Prepare accepts without failing.
// The Windows command-line limit is 32,767 UTF-16 characters;
// the argument is the fixed pointer to prompt.md, never the prompt itself, so only a pathological run-directory path reaches this bound.
const maxLaunchPromptBytes = 30000

// launchPointer returns the one argument a fresh session starts with: a pointer to the prompt file at promptPath.
func launchPointer(promptPath string) string {
	return "Read " + promptPath + " in full first; it is your complete, authoritative instructions."
}

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

// Legal Spec.PermissionMode values besides the empty default.
const (
	permissionModeBypass = "bypass"
	permissionModePrompt = "prompt"
)

// validatePermissionMode resolves a run's permission mode into whether its launch lines carry --dangerously-skip-permissions.
// Empty resolves to the run mode's default: an autonomous run skips, an interactive run prompts.
// "bypass" skips and "prompt" adds nothing.
// "prompt" on an autonomous run is an error, since a run that prompts with nobody to answer stalls;
// any other value is an error naming the three legal values.
func validatePermissionMode(mode string, interactive bool) (skipPermissions bool, err error) {
	switch mode {
	case "":
		return !interactive, nil
	case permissionModeBypass:
		return true, nil
	case permissionModePrompt:
		if !interactive {
			return false, fmt.Errorf("claudeengine: permission mode %q on an autonomous run would stall at its first permission dialog with no operator to answer; use it only on an interactive run", mode)
		}
		return false, nil
	}
	return false, fmt.Errorf("claudeengine: invalid permission mode %q; valid values are \"\" (the run mode's default), %q, %q (case-sensitive)", mode, permissionModeBypass, permissionModePrompt)
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

// buildLaunchCmd composes the pane-shell line that starts a claude session.
// A fresh session is named with --session-id;
// when resume is true the line takes over the existing session sessionID names with --resume instead,
// and everything else on the line is identical, so the run's own settings file routes the adopted session's hooks.
// It passes pointer as the first message when non-empty, and none otherwise (a launch that loads skills first sends the pointer afterwards),
// quotes all interpolated values, and appends --effort/--model only when non-empty.
// When notice is non-empty it rides the line as --append-system-prompt,
// so the session is told which tools are denied;
// an empty notice leaves the line unchanged.
// It names the session with --name, passed as a reference to LYX_STRAND_NAME rather than a value:
// shuttle builds this line before reed forms the strand's name,
// so the pane shell expands the variable from the export reed's launch script makes.
// It adds --dangerously-skip-permissions when skipPermissions is true, the mode validatePermissionMode resolved.
// When forkSubagents is true, it wraps the line via sh.WithEnv to enable fork subagent type.
func buildLaunchCmd(sh shell.Shell, bin, pointer, settingsPath, sessionID, model, effort, notice string, resume, skipPermissions, forkSubagents bool) string {
	sessionFlag := " --session-id "
	if resume {
		sessionFlag = " --resume "
	}
	cmd := sh.Invoke(bin)
	if pointer != "" {
		cmd += " " + sh.Quote(pointer)
	}
	cmd += sessionFlag + sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath) +
		" --name " + sh.EnvRef(agentname.StrandNameEnv)
	if model != "" {
		cmd += " --model " + sh.Quote(model)
	}
	if effort != "" {
		cmd += " --effort " + sh.Quote(effort)
	}
	if skipPermissions {
		cmd += " --dangerously-skip-permissions"
	}
	if notice != "" {
		cmd += " --append-system-prompt " + sh.Quote(notice)
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
// It carries the notice too, when non-empty: Claude Code re-renders the system prompt from the current flags after a compaction and where recording is not enabled,
// so a resume line without --append-system-prompt would lose the notice.
// It carries --name as a reference to LYX_STRAND_NAME, not a value, for the same reason buildLaunchCmd does,
// and because reed replays this line on every resume, a line without it would bring Claude back unnamed.
// When forkSubagents is true, the line is wrapped to keep the fork-subagent capability.
func buildResumeCmd(sh shell.Shell, bin, settingsPath, sessionID, model, effort, notice string, skipPermissions, forkSubagents bool) string {
	cmd := sh.Invoke(bin) + " --resume " + sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath) +
		" --name " + sh.EnvRef(agentname.StrandNameEnv)
	if model != "" {
		cmd += " --model " + sh.Quote(model)
	}
	if effort != "" {
		cmd += " --effort " + sh.Quote(effort)
	}
	if skipPermissions {
		cmd += " --dangerously-skip-permissions"
	}
	if notice != "" {
		cmd += " --append-system-prompt " + sh.Quote(notice)
	}
	if forkSubagents {
		cmd = sh.WithEnv(forkSubagentEnvKey, "1", cmd)
	}
	return cmd
}
