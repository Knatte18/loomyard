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
	"slices"
	"strconv"
	"strings"
	"time"

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

// validatePermissionMode resolves a run's or an opened session's permission mode into whether its launch lines carry --dangerously-skip-permissions.
// An opened session is always interactive.
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

// claudeBinary returns claude.Binary if set, otherwise "claude".
func claudeBinary(claude Config) string {
	if claude.Binary != "" {
		return claude.Binary
	}
	return "claude"
}

// forkSubagentEnvKey is the staged-rollout flag (Claude Code v2.1.117+) enabling built-in fork subagents.
// It must ride the pane command because the reed server env is scrubbed of CLAUDE_CODE_* at boot.
const forkSubagentEnvKey = "CLAUDE_CODE_FORK_SUBAGENT"

// promptCacheTTLEnvKey is the variable Claude Code reads to pick the TTL of the prompt cache it writes.
// It rides the pane line for the same reason forkSubagentEnvKey does,
// and it outranks every Claude Code control for the TTL but FORCE_PROMPT_CACHING_5M.
const promptCacheTTLEnvKey = "CLAUDE_CODE_PROMPT_CACHE_TTL"

// bashMaxTimeoutEnvKey is the variable Claude Code reads as the ceiling on the Bash tool's timeout parameter.
// It rides the pane line for the same reason forkSubagentEnvKey does.
// BASH_DEFAULT_TIMEOUT_MS is left at Claude Code's default, so only a call that sets an explicit timeout runs past the default.
const bashMaxTimeoutEnvKey = "BASH_MAX_TIMEOUT_MS"

// bashMaxTimeout is the Bash tool timeout ceiling every lyx-spawned session runs under.
const bashMaxTimeout = time.Hour

// bashMaxTimeoutMS is bashMaxTimeout as the decimal millisecond string bashMaxTimeoutEnvKey takes.
var bashMaxTimeoutMS = strconv.FormatInt(bashMaxTimeout.Milliseconds(), 10)

// validPromptCacheTTLs is the set of exact, case-sensitive prompt-cache TTL values Claude Code accepts.
var validPromptCacheTTLs = map[string]bool{
	"5m": true,
	"1h": true,
}

// validatePromptCacheTTL reports an error unless value is a member of validPromptCacheTTLs.
// key names the claude.yaml entry the value came from, so the error points at what to fix.
func validatePromptCacheTTL(key, value string) error {
	if validPromptCacheTTLs[value] {
		return nil
	}
	return fmt.Errorf("claudeengine: invalid %s %q; valid values are 5m, 1h (case-sensitive); fix it in claude.yaml with lyx config claude", key, value)
}

// resolvePromptCacheTTL returns the prompt-cache TTL for a strand's role.
// It first validates claude.PromptCacheTTL and every value in claude.PromptCacheTTLRoles, in sorted key order so the reported entry is deterministic.
// One bad entry therefore refuses every role until claude.yaml is fixed.
// It then returns the map's entry for role when one exists, and the default otherwise;
// an empty role takes the default.
func resolvePromptCacheTTL(role string, claude Config) (string, error) {
	if err := validatePromptCacheTTL("prompt_cache_ttl", claude.PromptCacheTTL); err != nil {
		return "", err
	}
	roles := make([]string, 0, len(claude.PromptCacheTTLRoles))
	for mappedRole := range claude.PromptCacheTTLRoles {
		roles = append(roles, mappedRole)
	}
	slices.Sort(roles)
	for _, mappedRole := range roles {
		if err := validatePromptCacheTTL(fmt.Sprintf("prompt_cache_ttl_roles[%s]", mappedRole), claude.PromptCacheTTLRoles[mappedRole]); err != nil {
			return "", err
		}
	}
	if role == "" {
		return claude.PromptCacheTTL, nil
	}
	if ttl, ok := claude.PromptCacheTTLRoles[role]; ok {
		return ttl, nil
	}
	return claude.PromptCacheTTL, nil
}

// PromptCacheLifetime returns the prompt-cache TTL Prepare puts on the pane line for spec's role, parsed as a duration.
// It loads claude.yaml first; a load or resolution error is returned with its claude.yaml advice.
func (c *Claude) PromptCacheLifetime(spec shuttleengine.Spec, cfg shuttleengine.Config) (time.Duration, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return 0, err
	}
	ttl, err := resolvePromptCacheTTL(spec.Role, claude)
	if err != nil {
		return 0, err
	}
	return time.ParseDuration(ttl)
}

// envFileKey is the variable Claude Code reads to find a file whose content it runs in the Bash tool's shell before each command.
// It rides the pane line for the same reason forkSubagentEnvKey does.
const envFileKey = "CLAUDE_ENV_FILE"

// envFileName is the file Prepare writes beside settings.json for envFileKey to name.
const envFileName = "bash-env.sh"

// envFileContentPosix is the env file's content on a host whose Bash tool reads native paths.
// Its first statement makes /dev/null the default stdin of the Bash tool's shell, so an interpreter left reading the tool's open stdin ends at once instead of hanging the session.
// Its second prepends the parent directory of the spawning binary's path, which reed's pane prelude exports in LYX_BIN, to PATH,
// so the tool's shell resolves `lyx` to that binary after the operator's rc has reordered PATH.
// It adds nothing when LYX_BIN is unset or empty.
// The file runs those two statements and nothing else, and every expansion is double-quoted so a path with spaces holds.
// It changes only the default stdin and PATH of the tool's own shell: a heredoc, pipe or redirect attached to a command still supplies that command's stdin,
// a child `bash` the agent starts does not re-run the file, and it grants or denies nothing, so the agents' permission rules are untouched.
const envFileContentPosix = "exec </dev/null\n" +
	"if [ -n \"${" + agentname.LyxBinEnv + ":-}\" ]; then PATH=\"${" + agentname.LyxBinEnv + "%/*}:$PATH\"; fi\n"

// envFileContentWindows is the env file's content on a Windows host, where LYX_BIN holds a native path the Bash tool's shell must convert first.
// It behaves as the POSIX content, but converts LYX_BIN with `cygpath -u` into the scratch variable __lyx_bin_dir before taking the parent directory, and unsets the variable afterwards.
// It changes nothing about PATH when `cygpath` is absent or LYX_BIN is unset or empty.
const envFileContentWindows = "exec </dev/null\n" +
	"if [ -n \"${" + agentname.LyxBinEnv + ":-}\" ] && command -v cygpath >/dev/null 2>&1; then " +
	"__lyx_bin_dir=\"$(cygpath -u \"$" + agentname.LyxBinEnv + "\")\"; " +
	"PATH=\"${__lyx_bin_dir%/*}:$PATH\"; unset __lyx_bin_dir; fi\n"

// envFileContentFor returns the env file's content for the host goos names.
func envFileContentFor(goos string) string {
	if goos == "windows" {
		return envFileContentWindows
	}
	return envFileContentPosix
}

// buildLaunchCmd composes the pane-shell line that starts a claude session.
// A fresh session is named with --session-id;
// when resume is true the line takes over the existing session sessionID names with --resume instead,
// and everything else on the line is identical, so the run's own settings file routes the adopted session's hooks.
// It passes pointer as the first message when non-empty, and none otherwise (a launch that loads skills passes the skill-load message there and sends the prompt pointer afterwards),
// quotes all interpolated values, and appends --effort/--model only when non-empty.
// When notice is non-empty it rides the line as --append-system-prompt,
// so the session is told which tools are denied;
// an empty notice leaves the line unchanged.
// It names the session with --name, passed as a reference to LYX_STRAND_NAME rather than a value:
// shuttle builds this line before reed forms the strand's name,
// so the pane shell expands the variable from the export reed's launch script makes.
// It adds --dangerously-skip-permissions when skipPermissions is true, the mode validatePermissionMode resolved.
// When forkSubagents is true, it wraps the line via sh.WithEnv to enable fork subagent type.
// It assigns promptCacheTTL, the non-empty value Prepare validated, to promptCacheTTLEnvKey.
// It assigns bashMaxTimeoutMS to bashMaxTimeoutEnvKey, so the line carries the Bash timeout ceiling.
// It then wraps the finished line in the envFileKey assignment naming envFilePath, so that assignment leads the line.
func buildLaunchCmd(sh shell.Shell, bin, pointer, settingsPath, sessionID, model, effort, notice, envFilePath, promptCacheTTL string, resume, skipPermissions, forkSubagents bool) string {
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
	cmd = sh.WithEnv(promptCacheTTLEnvKey, promptCacheTTL, cmd)
	cmd = sh.WithEnv(bashMaxTimeoutEnvKey, bashMaxTimeoutMS, cmd)
	return sh.WithEnv(envFileKey, envFilePath, cmd)
}

// buildOpenCmd composes the pane-shell line of a named interactive session that takes no prompt.
// A fresh session is named with --session-id; when resume is true the line takes over the session sessionID names with --resume instead.
// It names the session with --name as a reference to LYX_STRAND_NAME, as buildLaunchCmd does, and appends --model and --effort only when non-empty.
// It appends --dangerously-skip-permissions when skipPermissions is true, on the fresh and the resume form alike.
// Reed replays the resume line verbatim, so a resume without the flag would bring a bypass session back prompting.
// It passes no prompt, no --settings (the session takes the worktree's own settings) and no environment assignment, so the line is one pipeline that OrElse may join.
func buildOpenCmd(sh shell.Shell, bin, sessionID, model, effort string, skipPermissions, resume bool) string {
	sessionFlag := " --session-id "
	if resume {
		sessionFlag = " --resume "
	}
	cmd := sh.Invoke(bin) + sessionFlag + sh.Quote(sessionID) + " --name " + sh.EnvRef(agentname.StrandNameEnv)
	if model != "" {
		cmd += " --model " + sh.Quote(model)
	}
	if effort != "" {
		cmd += " --effort " + sh.Quote(effort)
	}
	if skipPermissions {
		cmd += " --dangerously-skip-permissions"
	}
	return cmd
}

// OpenLaunch returns the launch of a named interactive session with no prompt.
// It loads claude.yaml, validates the effort and the permission mode and resolves the model and version into a model id before anything else, mints a session id, and returns the fresh line as Cmd.
// Bypass is opt-in per call through spec.PermissionMode;
// the session still loads Claude Code's own settings layers, whose deny rules and hooks Claude Code applies in bypass too, and carries no shuttle --settings or guard.
// "prompt" adds nothing, so it cannot override a default mode the settings set.
// The session takes no turn until text reaches its pane through the operator, `lyx shuttle send` or creel, each bounded by its own rules.
// ResumeCmd resumes the session and falls back to a fresh start under the same id, because Claude Code keeps no transcript for a session that never took a turn, so --resume fails there.
// Bound: the fresh line runs whenever the resume line exits nonzero, including a resumed session that ends nonzero after a working life;
// a fresh start under a session id whose transcript exists fails at once, so the fallback never starts a second live session over a recorded one.
func (c *Claude) OpenLaunch(spec shuttleengine.OpenSpec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	if err := validateEffort(spec.Effort); err != nil {
		return shuttleengine.Launch{}, err
	}
	resolvedModel, err := resolveModelID(spec.Model, spec.Version)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	skipPermissions, err := validatePermissionMode(spec.PermissionMode, true)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	sessionID, err := newSessionID()
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("mint session id: %w", err)
	}
	sh := shell.ForGOOS()
	bin := claudeBinary(claude)
	fresh := buildOpenCmd(sh, bin, sessionID, resolvedModel, spec.Effort, skipPermissions, false)
	resumeLine := buildOpenCmd(sh, bin, sessionID, resolvedModel, spec.Effort, skipPermissions, true)
	return shuttleengine.Launch{Cmd: fresh, ResumeCmd: sh.OrElse(resumeLine, fresh), SessionID: sessionID}, nil
}

// Compile-time proof that Claude can open a session with no prompt.
var _ shuttleengine.SessionOpener = (*Claude)(nil)

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
// It carries the promptCacheTTLEnvKey assignment too: the resumed claude inherits nothing from the launch, so without it the session would fall back to Claude Code's default TTL.
// It carries the bashMaxTimeoutEnvKey assignment too, for the same reason.
// It carries the envFileKey assignment naming envFilePath, leading the line, so a resumed session keeps the default stdin of its Bash tool.
func buildResumeCmd(sh shell.Shell, bin, settingsPath, sessionID, model, effort, notice, envFilePath, promptCacheTTL string, skipPermissions, forkSubagents bool) string {
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
	cmd = sh.WithEnv(promptCacheTTLEnvKey, promptCacheTTL, cmd)
	cmd = sh.WithEnv(bashMaxTimeoutEnvKey, bashMaxTimeoutMS, cmd)
	return sh.WithEnv(envFileKey, envFilePath, cmd)
}
