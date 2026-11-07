// command.go composes the pane-shell command lines that start, resume and open a claude session.
// They are typed into a pane with tmux send-keys, which submits a line at a time, so none may hold a newline.
// Quoting and the call operator belong to internal/shell, and this file emits no raw pwsh or posix syntax of its own.

package claudeengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// maxLaunchPromptBytes is the largest launch argument, in bytes, that a launch accepts without failing.
// The Windows command-line limit is 32,767 UTF-16 characters, and this bound keeps the argument under it.
const maxLaunchPromptBytes = 30000

// launchPointer returns the one argument a fresh session starts with: a pointer to the prompt file at promptPath.
func launchPointer(promptPath string) string {
	return "Read " + promptPath + " in full first; it is your complete, authoritative instructions."
}

// validateEffort reports an error unless effort is empty or one of the exact-lowercase values claude accepts.
// claude ignores invalid efforts rather than failing, so shuttle must reject them here to prevent silent drops.
func validateEffort(effort string) error {
	if effort == "" || slices.Contains(agentconfig.Efforts(), effort) {
		return nil
	}
	return fmt.Errorf("claudeengine: invalid effort %q; valid values are %s (case-sensitive, exact-lowercase)", effort, strings.Join(agentconfig.Efforts(), ", "))
}

// The permission modes a spec may name besides the empty default, which means bypass.
const (
	permissionModeBypass = "bypass"
	permissionModePrompt = "prompt"
)

// validatePermissionMode resolves a permission mode into whether launch lines carry --dangerously-skip-permissions.
// It serves both runs and opened sessions, and an opened session is always interactive.
// Empty and "bypass" skip in either mode, because every session lym launches into reed needs bypass to work unattended;
// "prompt" adds nothing and is the per-call opt-out.
// "prompt" on an autonomous run is an error, since a run that prompts with nobody to answer stalls;
// any other value is an error that names the legal values.
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
// An empty version returns model unchanged.
// A version with no model is an error.
// A dashed model already pins its own version, so a version with it is an error too.
// Otherwise the id is "claude-<model>-<version>", with the version's dots as dashes.
// For example, "sonnet" and "4.5" give "claude-sonnet-4-5".
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
// It rides the pane line because the reed server env is scrubbed of CLAUDE_CODE_* at boot,
// and it outranks every Claude Code control for the TTL but FORCE_PROMPT_CACHING_5M.
const promptCacheTTLEnvKey = "CLAUDE_CODE_PROMPT_CACHE_TTL"

// bashMaxTimeoutEnvKey is the variable Claude Code reads as the ceiling on the Bash tool's timeout parameter.
// It rides the pane line because the reed server env is scrubbed of CLAUDE_CODE_* at boot.
// BASH_DEFAULT_TIMEOUT_MS is left at Claude Code's default.
// Only a call that sets an explicit timeout runs past that default.
const bashMaxTimeoutEnvKey = "BASH_MAX_TIMEOUT_MS"

// bashMaxTimeout is the Bash tool timeout ceiling every lym-spawned session runs under.
const bashMaxTimeout = time.Hour

// bashMaxTimeoutMS is the Bash tool timeout ceiling in milliseconds, as the decimal string the pane line carries.
var bashMaxTimeoutMS = strconv.FormatInt(bashMaxTimeout.Milliseconds(), 10)

// validPromptCacheTTLs is the set of exact, case-sensitive prompt-cache TTL values Claude Code accepts.
var validPromptCacheTTLs = map[string]bool{
	"5m": true,
	"1h": true,
}

// validatePromptCacheTTL reports an error unless value is one of the TTLs Claude Code accepts.
// key names the claude.yaml entry the value came from, so the error points at what to fix.
func validatePromptCacheTTL(key, value string) error {
	if validPromptCacheTTLs[value] {
		return nil
	}
	return fmt.Errorf("claudeengine: invalid %s %q; valid values are 5m, 1h (case-sensitive); fix it in claude.yaml with lym config claude", key, value)
}

// resolvePromptCacheTTL returns the prompt-cache TTL for a strand's role.
// It first validates claude.PromptCacheTTL and every value in claude.PromptCacheTTLRoles.
// The roles are checked in sorted key order, so the reported entry is deterministic.
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

// PromptCacheLifetime returns the prompt-cache TTL of a session of spec's role, parsed as a duration.
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

// longKeepWarmCap is the cap a long gate wait uses, and the ceiling of a configured keep-warm cap.
// It sits five minutes under the Bash tool timeout ceiling, so a long wait ends before the tool kills it.
const longKeepWarmCap = bashMaxTimeout - 5*time.Minute

// minKeepWarmCap is the floor of a configured keep-warm cap.
const minKeepWarmCap = 60 * time.Second

// checkKeepWarmCap checks value as a keep-warm cap and returns the parsed cap, or an error.
// A cap must parse as a Go duration, lie in [minKeepWarmCap, longKeepWarmCap] and be below lifetime.
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

// keepWarmCapFor returns the cap string for role: its keep_warm_cap_roles entry, else keep_warm_cap.
func keepWarmCapFor(role string, claude Config) string {
	if value, ok := claude.KeepWarmCapRoles[role]; ok {
		return value
	}
	return claude.KeepWarmCap
}

// ValidateKeepWarm reports an error unless every keep-warm cap in claude is usable.
// A cap must parse as a Go duration that is at least minKeepWarmCap and at most longKeepWarmCap.
// It must also be below its prompt-cache lifetime.
// The prompt-cache TTLs are resolved first, so a bad TTL is reported before a bad cap.
// The default cap is checked against the default lifetime first.
// Each role named in prompt_cache_ttl_roles or keep_warm_cap_roles is then checked against its own lifetime.
// Roles are visited in sorted order, so the reported error is deterministic.
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

// KeepWarmCaps returns the cap on one keep-warm wait of a session of role, and the long cap a long gate wait uses.
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
// It rides the pane line because the reed server env is scrubbed of CLAUDE_CODE_* at boot.
const envFileKey = "CLAUDE_ENV_FILE"

// envFileName is the name of the Bash env file in a run's directory, which the pane line points at.
const envFileName = "bash-env.sh"

// envFileContentPosix is the env file's content on a host whose Bash tool reads native paths.
// Its first statement makes /dev/null the default stdin of the Bash tool's shell,
// so an interpreter left reading the tool's open stdin ends at once instead of hanging the session.
// Its second statement prepends the parent directory of LYM_BIN to PATH,
// so the tool's shell resolves `lym` to the spawning binary after the operator's rc has reordered PATH.
// reed's pane prelude exports LYM_BIN as that binary's absolute path.
// It adds nothing when LYM_BIN is unset or empty.
// The file runs those two statements and nothing else.
// Every expansion is double-quoted, so a path with spaces holds.
// It changes only the default stdin and PATH of the tool's own shell.
// A heredoc, pipe or redirect attached to a command still supplies that command's stdin.
// A child `bash` the agent starts does not re-run the file.
// It grants or denies nothing, so the agents' permission rules are untouched.
const envFileContentPosix = "exec </dev/null\n" +
	"if [ -n \"${" + agentname.LymBinEnv + ":-}\" ]; then PATH=\"${" + agentname.LymBinEnv + "%/*}:$PATH\"; fi\n"

// envFileContentWindows is the env file's content on a Windows host, where LYM_BIN holds a native path.
// The Bash tool's shell must convert that path first, so the content runs `cygpath -u` on LYM_BIN.
// The result goes into the scratch variable __lym_bin_dir, which the content unsets afterwards.
// Apart from the conversion, it sets the default stdin to /dev/null.
// It also prepends the parent directory of that path to PATH.
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
// It passes pointer as the first message when non-empty, and none otherwise.
// A launch that loads skills passes the skill-load message there and sends the prompt pointer afterwards.
// It quotes every interpolated value and appends --model and --effort only when non-empty.
// When appendPromptPath is non-empty it rides the line as --append-system-prompt-file,
// so the session is told which tools are denied and how to trust and answer creel mail;
// the text rides a file because a quote in it would be dropped by an argument passed through pwsh's Legacy mode.
// It names the session with --name as a reference to LYM_STRAND_NAME, not a value.
// Shuttle builds this line before reed forms the strand's name,
// so the pane shell expands the reference from the export that reed's launch script makes.
// It wraps the finished line in the envFileKey assignment naming envFilePath, so that assignment leads the line.
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
// A fresh session is named with --session-id;
// when resume is true the line takes over the session sessionID names with --resume instead.
// It names the session with --name as a reference to LYM_STRAND_NAME.
// The pane shell expands that reference from the export that reed's launch script makes.
// It appends --model and --effort only when non-empty.
// It appends --dangerously-skip-permissions when skipPermissions is true, on the fresh and the resume form alike.
// Reed replays the resume line verbatim, so a resume without the flag would bring a bypass session back prompting.
// A non-empty settingsPath adds --settings naming it, which Claude Code stacks over the worktree's own settings.
// A non-empty appendPromptPath adds --append-system-prompt-file naming it.
// It passes no environment assignment, so the line is one pipeline that OrElse may join;
// the caller exports the environment once in front of the joined lines.
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
// A spec naming no effort takes the agents.yaml effort for its model.
// A full Claude model id resolves to its alias, and a model without an entry gets the default.
// It validates the effort, the permission mode and the model before it writes anything,
// and it resolves the prompt-cache TTL before then too.
// The TTL is spec.Role's from claude.yaml, as for a run, and an invalid TTL anywhere in claude.yaml refuses the open.
// Both lines open with a statement that exports it as CLAUDE_CODE_PROMPT_CACHE_TTL,
// so the resume line and its fresh fallback both run under it, in either shell dialect.
// With a hub board directory, it creates dir and writes two files into it:
// settings.json holds creel's hook delivery, the empty attribution and the session's scope for spec.Skills alone.
// The append-system-prompt file holds the creel trust sentence.
// The Cmd and ResumeCmd lines both name these files, and the launch is marked CreelHooks,
// so the session gets its mail through its hooks, as a run session does.
// Without a hub board directory, it writes nothing, creates no dir and marks nothing.
// A failure after dir is created removes it.
// Bypass is the default, and spec.PermissionMode "prompt" opts out per call.
// "prompt" adds nothing, so it cannot override a default mode the settings set.
// The session still loads Claude Code's own settings layers, whose deny rules and hooks apply in bypass too,
// and it carries no shuttle guard.
// A non-empty spec.Skills puts their load message on the fresh line as the session's first turn.
// A message past the launch limit refuses the open before anything is written, and the resume line carries none.
// The session takes no other turn until the operator or creel sends text to its pane, each bounded by its own rules.
// The ResumeCmd line resumes the session and falls back to a fresh start under the same id:
// Claude Code keeps no transcript for a session that never took a turn, so resuming it fails.
// The fresh line runs whenever the resume line exits nonzero, even after a working life.
// A fresh start under a session id whose transcript exists fails at once,
// so the fallback never starts a second live session over a recorded one.
func (c *Claude) OpenLaunch(dir string, spec shuttleengine.OpenSpec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	claude, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	// An opened session runs as no agents.yaml role, so a spec with no effort takes the one its model gets.
	spec.Effort = effortFor(cfg.Agents, "", spec.Effort, spec.Model)
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
	promptCacheTTL, err := resolvePromptCacheTTL(spec.Role, claude)
	if err != nil {
		return shuttleengine.Launch{}, err
	}
	logger.Info("claudeengine: resolved the prompt-cache TTL of an opened session", "role", spec.Role, "ttl", promptCacheTTL)
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
		scope := c.sessionScopeFor(claude.SessionScope, spec.Skills, spec.Role)
		settingsPath, appendPromptPath, err = writeOpenFiles(dir, creelFlagDirForHook, scope)
		if err != nil {
			return shuttleengine.Launch{}, err
		}
	}
	sh := shell.ForGOOS()
	bin := claudeBinary(claude)
	fresh := buildOpenCmd(sh, bin, skillLoadMessage, sessionID, resolvedModel, spec.Effort, settingsPath, appendPromptPath, skipPermissions, false)
	resumeLine := buildOpenCmd(sh, bin, "", sessionID, resolvedModel, spec.Effort, settingsPath, appendPromptPath, skipPermissions, true)
	exportTTL := sh.ExportEnv(promptCacheTTLEnvKey, promptCacheTTL)
	return shuttleengine.Launch{
		Cmd:        sh.Chain(exportTTL, fresh),
		ResumeCmd:  sh.Chain(exportTTL, sh.OrElse(resumeLine, fresh)),
		SessionID:  sessionID,
		CreelHooks: creelFlagDirForHook != "",
	}, nil
}

// writeOpenFiles creates dir and writes an opened session's two files into it.
// settings.json holds creel's hook delivery over the flag directory creelFlagDirPosix;
// its attribution is empty, and its scope is the one passed in.
// The append-system-prompt file holds the creel trust sentence.
// It returns the two files' absolute paths.
// A failure after the directory is created removes it, so a failed launch leaves nothing behind.
func writeOpenFiles(dir, creelFlagDirPosix string, scope sessionScope) (settingsPath, appendPromptPath string, err error) {
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
	settingsJSON, err := buildOpenSettings(creelFlagDirPosix, scope)
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
// The resumed claude is a fresh process that inherits nothing from the launch,
// so the line carries the same model, effort and permission mode as the launch line.
// Without them a resumed run comes back on the provider's default model,
// and an autonomous run stalls at a tool dialog with no operator present, which shuttle can only classify as a timeout.
// It carries the append-system-prompt file too, when its path is non-empty:
// Claude Code re-renders the system prompt from the current flags after a compaction;
// where recording is not enabled, a resume line without the file loses the deny notice and the creel trust sentence.
// It carries --name as a reference to agentname.StrandNameEnv, not a value:
// the strand's name is formed after the line is built, so the pane shell expands the variable at run time.
// A session rebuild replays this string verbatim, so a line without --name would bring Claude back unnamed.
// When forkSubagents is true, the line is wrapped to keep the fork-subagent capability.
// It carries the promptCacheTTLEnvKey assignment too, since the resumed claude inherits nothing from the launch:
// without it the session falls back to Claude Code's default TTL.
// It carries the bashMaxTimeoutEnvKey assignment too, for the same reason.
// It leads with the envFileKey assignment naming envFilePath:
// a resumed session keeps the default stdin of its Bash tool.
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
