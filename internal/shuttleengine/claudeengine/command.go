// command.go composes the opaque pane-shell command lines Prepare (settings.go) hands back as a
// Launch: the launch line that starts a fresh session and the resume line that reattaches an
// existing one.
// It also holds OpenLaunch, which writes an opened session's creel settings and composes its lines.
// Both are single-line strings typed verbatim into a pane via tmux send-keys (see
// reedengine/spawn.go's launchStrandLocked) — no newline may appear in either, since send-keys
// submits a line at a time.
// Argument quoting and the call operator are pane-shell mechanics
// owned entirely by internal/shell (the Shell Mechanics Seam invariant);
// this file only ever calls into that seam and never emits raw pwsh/posix syntax of its own.

package claudeengine

import (
	"fmt"
	"os"
	"path/filepath"
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
// Empty and "bypass" skip in either run mode, because every session lym launches into reed needs bypass to work unattended;
// "prompt" adds nothing and is the per-call opt-out.
// "prompt" on an autonomous run is an error, since a run that prompts with nobody to answer stalls;
// any other value is an error naming the three legal values.
func validatePermissionMode(mode string, interactive bool) (skipPermissions bool, err error) {
	switch mode {
	case "", permissionModeBypass:
		return true, nil
	case permissionModePrompt:
		if !interactive {
			return false, fmt.Errorf("claudeengine: permission mode %q on an autonomous run would stall at its first permission dialog with no operator to answer; use it only on an interactive run", mode)
		}
		return false, nil
	}
	return false, fmt.Errorf("claudeengine: invalid permission mode %q; valid values are \"\" (bypass), %q, %q (case-sensitive)", mode, permissionModeBypass, permissionModePrompt)
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

// bashMaxTimeout is the Bash tool timeout ceiling every lym-spawned session runs under.
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
	return fmt.Errorf("claudeengine: invalid %s %q; valid values are 5m, 1h (case-sensitive); fix it in claude.yaml with lym config claude", key, value)
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

// longKeepWarmCap is the long cap KeepWarmCaps answers and the ceiling of a configured keep-warm cap: the Bash tool timeout ceiling less five minutes, so a long wait ends before the tool kills it.
const longKeepWarmCap = bashMaxTimeout - 5*time.Minute

// minKeepWarmCap is the floor of a configured keep-warm cap.
const minKeepWarmCap = 60 * time.Second

// checkKeepWarmCap reports an error unless value parses as a Go duration within [minKeepWarmCap, longKeepWarmCap] and below lifetime, and returns the parsed cap.
// key names the claude.yaml entry the value came from, so the error points at what to fix.
func checkKeepWarmCap(key, value string, lifetime time.Duration) (time.Duration, error) {
	capWait, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("claudeengine: invalid %s %q; it must parse as a Go duration; fix it in claude.yaml with lym config claude", key, value)
	}
	switch {
	case capWait < minKeepWarmCap:
		return 0, fmt.Errorf("claudeengine: invalid %s %q; it must be at least %s; fix it in claude.yaml with lym config claude", key, value, minKeepWarmCap)
	case capWait >= lifetime:
		return 0, fmt.Errorf("claudeengine: invalid %s %q; it must be below the role's prompt-cache lifetime %s; fix it in claude.yaml with lym config claude", key, value, lifetime)
	case capWait > longKeepWarmCap:
		return 0, fmt.Errorf("claudeengine: invalid %s %q; it must be at most %s; fix it in claude.yaml with lym config claude", key, value, longKeepWarmCap)
	}
	return capWait, nil
}

// keepWarmCapFor returns the cap string claude holds for role: its keep_warm_cap_roles entry when one exists, else keep_warm_cap.
func keepWarmCapFor(role string, claude Config) string {
	if value, ok := claude.KeepWarmCapRoles[role]; ok {
		return value
	}
	return claude.KeepWarmCap
}

// ValidateKeepWarm reports an error unless every keep-warm cap in claude is usable.
// It first resolves the prompt-cache TTLs, so a bad TTL is reported before a bad cap.
// It then checks the default cap against the default prompt-cache lifetime and, for every role named in prompt_cache_ttl_roles or keep_warm_cap_roles in sorted order, that role's cap against that role's lifetime.
// A cap must parse as a Go duration, be at least 60s, be below its lifetime and be at most 55m.
func ValidateKeepWarm(claude Config) error {
	defaultTTL, err := resolvePromptCacheTTL("", claude)
	if err != nil {
		return err
	}
	defaultLifetime, err := time.ParseDuration(defaultTTL)
	if err != nil {
		return err
	}
	if _, err := checkKeepWarmCap("keep_warm_cap", claude.KeepWarmCap, defaultLifetime); err != nil {
		return err
	}
	named := map[string]bool{}
	for role := range claude.PromptCacheTTLRoles {
		named[role] = true
	}
	for role := range claude.KeepWarmCapRoles {
		named[role] = true
	}
	roles := make([]string, 0, len(named))
	for role := range named {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	for _, role := range roles {
		ttl, err := resolvePromptCacheTTL(role, claude)
		if err != nil {
			return err
		}
		lifetime, err := time.ParseDuration(ttl)
		if err != nil {
			return err
		}
		key := "keep_warm_cap"
		if _, ok := claude.KeepWarmCapRoles[role]; ok {
			key = fmt.Sprintf("keep_warm_cap_roles[%s]", role)
		}
		if _, err := checkKeepWarmCap(key, keepWarmCapFor(role, claude), lifetime); err != nil {
			return err
		}
	}
	return nil
}

// KeepWarmCaps returns the longest one keep-warm wait of a session of role blocks and the long cap a long gate wait uses.
// The cap is role's keep_warm_cap_roles entry, else keep_warm_cap, from claude.yaml.
// It loads and validates claude.yaml first; a load or validation error is returned with its claude.yaml advice.
func (c *Claude) KeepWarmCaps(role string) (time.Duration, time.Duration, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return 0, 0, err
	}
	if err := ValidateKeepWarm(claude); err != nil {
		return 0, 0, err
	}
	capWait, err := time.ParseDuration(keepWarmCapFor(role, claude))
	if err != nil {
		return 0, 0, err
	}
	return capWait, longKeepWarmCap, nil
}

// envFileKey is the variable Claude Code reads to find a file whose content it runs in the Bash tool's shell before each command.
// It rides the pane line for the same reason forkSubagentEnvKey does.
const envFileKey = "CLAUDE_ENV_FILE"

// envFileName is the file Prepare writes beside settings.json for envFileKey to name.
const envFileName = "bash-env.sh"

// envFileContentPosix is the env file's content on a host whose Bash tool reads native paths.
// Its first statement makes /dev/null the default stdin of the Bash tool's shell, so an interpreter left reading the tool's open stdin ends at once instead of hanging the session.
// Its second prepends the parent directory of the spawning binary's path, which reed's pane prelude exports in LYM_BIN, to PATH,
// so the tool's shell resolves `lym` to that binary after the operator's rc has reordered PATH.
// It adds nothing when LYM_BIN is unset or empty.
// The file runs those two statements and nothing else, and every expansion is double-quoted so a path with spaces holds.
// It changes only the default stdin and PATH of the tool's own shell: a heredoc, pipe or redirect attached to a command still supplies that command's stdin,
// a child `bash` the agent starts does not re-run the file, and it grants or denies nothing, so the agents' permission rules are untouched.
const envFileContentPosix = "exec </dev/null\n" +
	"if [ -n \"${" + agentname.LymBinEnv + ":-}\" ]; then PATH=\"${" + agentname.LymBinEnv + "%/*}:$PATH\"; fi\n"

// envFileContentWindows is the env file's content on a Windows host, where LYM_BIN holds a native path the Bash tool's shell must convert first.
// It behaves as the POSIX content, but converts LYM_BIN with `cygpath -u` into the scratch variable __lym_bin_dir before taking the parent directory, and unsets the variable afterwards.
// It changes nothing about PATH when `cygpath` is absent or LYM_BIN is unset or empty.
const envFileContentWindows = "exec </dev/null\n" +
	"if [ -n \"${" + agentname.LymBinEnv + ":-}\" ] && command -v cygpath >/dev/null 2>&1; then " +
	"__lym_bin_dir=\"$(cygpath -u \"$" + agentname.LymBinEnv + "\")\"; " +
	"PATH=\"${__lym_bin_dir%/*}:$PATH\"; unset __lym_bin_dir; fi\n"

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
// When appendPromptPath is non-empty it rides the line as --append-system-prompt-file,
// so the session is told which tools are denied and how to trust and answer creel mail;
// the text rides a file because a quote in it would be dropped by an argument passed through pwsh's Legacy mode.
// An empty path leaves the line unchanged.
// It names the session with --name, passed as a reference to LYM_STRAND_NAME rather than a value:
// shuttle builds this line before reed forms the strand's name,
// so the pane shell expands the variable from the export reed's launch script makes.
// It adds --dangerously-skip-permissions when skipPermissions is true, the mode validatePermissionMode resolved.
// When forkSubagents is true, it wraps the line via sh.WithEnv to enable fork subagent type.
// It assigns promptCacheTTL, the non-empty value Prepare validated, to promptCacheTTLEnvKey.
// It assigns bashMaxTimeoutMS to bashMaxTimeoutEnvKey, so the line carries the Bash timeout ceiling.
// It then wraps the finished line in the envFileKey assignment naming envFilePath, so that assignment leads the line.
func buildLaunchCmd(sh shell.Shell, bin, pointer, settingsPath, sessionID, model, effort, appendPromptPath, envFilePath, promptCacheTTL string, resume, skipPermissions, forkSubagents bool) string {
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
	if appendPromptPath != "" {
		cmd += " --append-system-prompt-file " + sh.Quote(appendPromptPath)
	}
	if forkSubagents {
		cmd = sh.WithEnv(forkSubagentEnvKey, "1", cmd)
	}
	cmd = sh.WithEnv(promptCacheTTLEnvKey, promptCacheTTL, cmd)
	cmd = sh.WithEnv(bashMaxTimeoutEnvKey, bashMaxTimeoutMS, cmd)
	return sh.WithEnv(envFileKey, envFilePath, cmd)
}

// buildOpenCmd composes the pane-shell line of a named interactive session that takes no task prompt.
// A non-empty firstTurn goes after the binary as the session's first turn.
// A fresh session is named with --session-id; when resume is true the line takes over the session sessionID names with --resume instead.
// It names the session with --name as a reference to LYM_STRAND_NAME, as buildLaunchCmd does, and appends --model and --effort only when non-empty.
// It appends --dangerously-skip-permissions when skipPermissions is true, on the fresh and the resume form alike.
// Reed replays the resume line verbatim, so a resume without the flag would bring a bypass session back prompting.
// A non-empty settingsPath adds --settings naming it, which Claude Code stacks over the worktree's own settings, and a non-empty appendPromptPath adds --append-system-prompt-file naming it.
// It passes no environment assignment, so the line is one pipeline that OrElse may join.
func buildOpenCmd(sh shell.Shell, bin, firstTurn, sessionID, model, effort, settingsPath, appendPromptPath string, skipPermissions, resume bool) string {
	sessionFlag := " --session-id "
	if resume {
		sessionFlag = " --resume "
	}
	cmd := sh.Invoke(bin)
	if firstTurn != "" {
		cmd += " " + sh.Quote(firstTurn)
	}
	cmd += sessionFlag + sh.Quote(sessionID) + " --name " + sh.EnvRef(agentname.StrandNameEnv)
	if model != "" {
		cmd += " --model " + sh.Quote(model)
	}
	if effort != "" {
		cmd += " --effort " + sh.Quote(effort)
	}
	if skipPermissions {
		cmd += " --dangerously-skip-permissions"
	}
	if settingsPath != "" {
		cmd += " --settings " + sh.Quote(settingsPath)
	}
	if appendPromptPath != "" {
		cmd += " --append-system-prompt-file " + sh.Quote(appendPromptPath)
	}
	return cmd
}

// OpenLaunch returns the launch of a named interactive session with no prompt.
// It loads claude.yaml, validates the effort and the permission mode and resolves the model and version into a model id before anything else, mints a session id, and returns the fresh line as Cmd.
// An engine told a hub board directory then creates dir and writes into it settings.json, holding creel's hook delivery and the empty attribution and nothing else, and the append-system-prompt file, holding the creel trust sentence alone;
// both lines name the two files, and the launch is marked CreelHooks, so the session gets its mail through its hooks as a run session does.
// A failure after it creates dir removes dir.
// An engine told no board directory writes nothing, leaves dir uncreated and marks nothing.
// Bypass is the default, and spec.PermissionMode "prompt" opts out per call;
// the session still loads Claude Code's own settings layers, whose deny rules and hooks Claude Code applies in bypass too, and carries no shuttle guard.
// "prompt" adds nothing, so it cannot override a default mode the settings set.
// A non-empty spec.Skills puts their load message on the fresh line as the session's first turn, and a message past the launch limit refuses the open before anything is written; the resume line carries none.
// The session takes no other turn until text reaches its pane through the operator or creel, each bounded by its own rules.
// ResumeCmd resumes the session and falls back to a fresh start under the same id, because Claude Code keeps no transcript for a session that never took a turn, so --resume fails there.
// Bound: the fresh line runs whenever the resume line exits nonzero, including a resumed session that ends nonzero after a working life;
// a fresh start under a session id whose transcript exists fails at once, so the fallback never starts a second live session over a recorded one.
func (c *Claude) OpenLaunch(dir string, spec shuttleengine.OpenSpec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
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
	skillLoadMessage := ""
	if len(spec.Skills) > 0 {
		skillLoadMessage = c.SkillLoadMessage(spec.Skills)
		if len(skillLoadMessage) > maxLaunchPromptBytes {
			return shuttleengine.Launch{}, fmt.Errorf(
				"skill-load message is %d bytes, over the %d-byte launch limit: the pane launch carries it as one command-line argument and Windows caps a process command line at 32,767 characters — name fewer or shorter skills than %v",
				len(skillLoadMessage), maxLaunchPromptBytes, spec.Skills,
			)
		}
	}
	creelFlagDirForHook, err := c.creelFlagDirForHook()
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	sessionID, err := newSessionID()
	if err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("mint session id: %w", err)
	}
	settingsPath, appendPromptPath := "", ""
	if creelFlagDirForHook != "" {
		settingsPath, appendPromptPath, err = writeOpenFiles(dir, creelFlagDirForHook)
		if err != nil {
			return shuttleengine.Launch{}, err
		}
	}
	sh := shell.ForGOOS()
	bin := claudeBinary(claude)
	fresh := buildOpenCmd(sh, bin, skillLoadMessage, sessionID, resolvedModel, spec.Effort, settingsPath, appendPromptPath, skipPermissions, false)
	resumeLine := buildOpenCmd(sh, bin, "", sessionID, resolvedModel, spec.Effort, settingsPath, appendPromptPath, skipPermissions, true)
	return shuttleengine.Launch{Cmd: fresh, ResumeCmd: sh.OrElse(resumeLine, fresh), SessionID: sessionID, CreelHooks: creelFlagDirForHook != ""}, nil
}

// writeOpenFiles creates dir and writes an opened session's settings.json, holding creel's hook delivery over the flag directory creelFlagDirPosix and the empty attribution, and its append-system-prompt file, holding the creel trust sentence.
// It returns the two files' absolute paths.
// A failure after the directory is created removes it, so a failed launch leaves nothing behind.
func writeOpenFiles(dir, creelFlagDirPosix string) (settingsPath, appendPromptPath string, err error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolve the open session directory: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create the open session directory: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(absDir)
		}
	}()
	settingsJSON, err := buildOpenSettings(creelFlagDirPosix)
	if err != nil {
		return "", "", fmt.Errorf("build open settings: %w", err)
	}
	settingsPath = filepath.Join(absDir, settingsFileName)
	if err := os.WriteFile(settingsPath, settingsJSON, 0o644); err != nil {
		return "", "", fmt.Errorf("write open settings: %w", err)
	}
	appendPromptPath = filepath.Join(absDir, appendPromptFileName)
	if err := os.WriteFile(appendPromptPath, []byte(appendPromptText("", nil)), 0o644); err != nil {
		return "", "", fmt.Errorf("write open append system prompt: %w", err)
	}
	return settingsPath, appendPromptPath, nil
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
// It carries the append-system-prompt file too, when its path is non-empty: Claude Code re-renders the system prompt from the current flags after a compaction and where recording is not enabled,
// so a resume line without --append-system-prompt-file would lose the deny notice and the creel trust sentence.
// It carries --name as a reference to LYM_STRAND_NAME, not a value, for the same reason buildLaunchCmd does,
// and because reed replays this line on every resume, a line without it would bring Claude back unnamed.
// When forkSubagents is true, the line is wrapped to keep the fork-subagent capability.
// It carries the promptCacheTTLEnvKey assignment too: the resumed claude inherits nothing from the launch, so without it the session would fall back to Claude Code's default TTL.
// It carries the bashMaxTimeoutEnvKey assignment too, for the same reason.
// It carries the envFileKey assignment naming envFilePath, leading the line, so a resumed session keeps the default stdin of its Bash tool.
func buildResumeCmd(sh shell.Shell, bin, settingsPath, sessionID, model, effort, appendPromptPath, envFilePath, promptCacheTTL string, skipPermissions, forkSubagents bool) string {
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
	if appendPromptPath != "" {
		cmd += " --append-system-prompt-file " + sh.Quote(appendPromptPath)
	}
	if forkSubagents {
		cmd = sh.WithEnv(forkSubagentEnvKey, "1", cmd)
	}
	cmd = sh.WithEnv(promptCacheTTLEnvKey, promptCacheTTL, cmd)
	cmd = sh.WithEnv(bashMaxTimeoutEnvKey, bashMaxTimeoutMS, cmd)
	return sh.WithEnv(envFileKey, envFilePath, cmd)
}
