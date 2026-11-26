// pluginhook.go reads a plugin's SessionStart command hooks, runs them and takes their context.
// It gives another provider's session the context Claude Code adds from the plugins its scope turns on.

package claudeengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// pluginHooksDirName is the directory of a plugin that holds its hooks file.
	pluginHooksDirName = "hooks"
	// pluginHooksFileName is the plugin's hooks file inside pluginHooksDirName.
	pluginHooksFileName = "hooks.json"
	// sessionStartEvent is the hook event whose command hooks add context to a session.
	sessionStartEvent = "SessionStart"
	// startupSourceName is the source Claude Code reports for a session's first start.
	startupSourceName = "startup"
	// commandHookType is the hook type that runs a shell command.
	commandHookType = "command"
	// matchAllHooks is the matcher that admits every source.
	matchAllHooks = "*"
	// hookWaitDelay bounds the wait for a hook's output pipes once its context has ended.
	hookWaitDelay = 2 * time.Second
	// hookNotRunReason is the reason an omitted hook gives when the request carried no runner.
	hookNotRunReason = "not run"
)

// pluginHook is one command hook of a plugin's SessionStart event.
type pluginHook struct {
	// command is the shell command the hook runs.
	command string
	// timeout is the hook's own bound; zero when the hook sets none.
	timeout time.Duration
}

// sessionStartHooks returns the plugin's `SessionStart` command hooks whose matcher admits a session's startup.
// The plugin is the one installed at root.
// A matcher is admitted when it is empty, `*`, or a pattern that matches `startup` whole.
// A plugin with no hooks file has none.
// An unreadable or malformed hooks file, or a matcher that is no pattern, is an error naming the file.
func sessionStartHooks(pluginID, root string) ([]pluginHook, error) {
	path := filepath.Join(root, pluginHooksDirName, pluginHooksFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claudeengine: read the hooks file %s of plugin %s: %w", path, pluginID, err)
	}
	var file struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("claudeengine: parse the hooks file %s of plugin %s: %w", path, pluginID, err)
	}
	var hooks []pluginHook
	for _, entry := range file.Hooks[sessionStartEvent] {
		admitted, err := matcherAdmitsStartup(entry.Matcher)
		if err != nil {
			return nil, fmt.Errorf("claudeengine: the hooks file %s of plugin %s holds a matcher that is no pattern: %w", path, pluginID, err)
		}
		if !admitted {
			continue
		}
		for _, hook := range entry.Hooks {
			if hook.Type != commandHookType {
				continue
			}
			hooks = append(hooks, pluginHook{command: hook.Command, timeout: time.Duration(hook.Timeout) * time.Second})
		}
	}
	return hooks, nil
}

// matcherAdmitsStartup reports whether a hook entry's matcher admits a session's startup.
func matcherAdmitsStartup(matcher string) (bool, error) {
	if matcher == "" || matcher == matchAllHooks {
		return true, nil
	}
	pattern, err := regexp.Compile("^(?:" + matcher + ")$")
	if err != nil {
		return false, err
	}
	return pattern.MatchString(startupSourceName), nil
}

// pluginHookContext returns the context a hook's standard output adds to a session.
// A JSON object gives its `hookSpecificOutput.additionalContext`, and any other output is the context as plain text.
// The context is trimmed, and is empty when the hook adds none.
func pluginHookContext(stdout []byte) string {
	var object struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &object); err == nil {
		return strings.TrimSpace(object.HookSpecificOutput.AdditionalContext)
	}
	return strings.TrimSpace(string(stdout))
}

// pluginSessionStartContexts runs the SessionStart command hooks of each plugin in ids that the scope turns on.
// ids are the plugin ids in the order they run, and plugins maps each to its install directory.
// It returns the non-empty contexts as items, and each hook or hooks file it left out as an omitted hook.
// A nil req.RunHook runs nothing and omits every hook as not run.
func pluginSessionStartContexts(req shuttleengine.UserSetupRequest, ids []string, plugins map[string]string, scope sessionScope) (items []shuttleengine.InstructionItem, omitted []shuttleengine.OmittedHook) {
	// A map of strings always marshals.
	stdin, _ := json.Marshal(map[string]string{
		"session_id":      req.SessionID,
		"transcript_path": "",
		"cwd":             req.WorkDir,
		"hook_event_name": sessionStartEvent,
		"source":          startupSourceName,
	})
	for _, id := range ids {
		if on, listed := scope.EnabledPlugins[id]; listed && !on {
			continue
		}
		root := plugins[id]
		hooks, err := sessionStartHooks(id, root)
		if err != nil {
			omitted = append(omitted, shuttleengine.OmittedHook{
				Name:   filepath.Join(root, pluginHooksDirName, pluginHooksFileName),
				Reason: err.Error(),
			})
			continue
		}
		for _, hook := range hooks {
			name := fmt.Sprintf("plugin %s %s hook", id, sessionStartEvent)
			if req.RunHook == nil {
				omitted = append(omitted, shuttleengine.OmittedHook{Name: name + " " + hook.command, Reason: hookNotRunReason, Plugin: id, Hook: hook.command})
				continue
			}
			timeout := hook.timeout
			if timeout == 0 {
				timeout = req.HookTimeout
			}
			stdout, err := req.RunHook(shuttleengine.HookRun{
				Command: hook.command,
				Dir:     req.WorkDir,
				Env:     []string{"CLAUDE_PLUGIN_ROOT=" + root, "CLAUDE_PROJECT_DIR=" + req.WorkDir},
				Stdin:   stdin,
				Timeout: timeout,
			})
			if err != nil {
				omitted = append(omitted, shuttleengine.OmittedHook{Name: name + " " + hook.command, Reason: err.Error(), Plugin: id, Hook: hook.command})
				continue
			}
			hookContext := pluginHookContext(stdout)
			if hookContext == "" {
				continue
			}
			items = append(items, shuttleengine.InstructionItem{
				Name:   name,
				Plugin: id,
				Hook:   hook.command,
				Text:   hookContext,
				WayForward: "turn plugin " + id + " off in the session scope: drop its skills from the role's skills in agents.yaml (`lym config agents`) or from the spec, " +
					"or, while " + keyName(sessionScopeKey+".enabled") + " is off, turn it on (`lym config claude`), since then every installed plugin is on",
			})
		}
	}
	return items, omitted
}

// RunPluginHook runs one plugin hook as `sh -c <command>` in run.Dir and returns its standard output.
// The command's environment is the process environment plus run.Env, and it reads run.Stdin.
// When run.Timeout passes, it ends the command and the processes it forked.
// It then returns an error naming the timeout.
// A non-zero exit returns an error carrying the exit status and the trimmed standard error.
func RunPluginHook(run shuttleengine.HookRun) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), run.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", run.Command)
	cmd.Dir = run.Dir
	cmd.Env = append(os.Environ(), run.Env...)
	cmd.Stdin = bytes.NewReader(run.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	setHookProcessGroup(cmd)
	cmd.WaitDelay = hookWaitDelay

	logger.Info("claudeengine: running a plugin hook", "command", run.Command, "dir", run.Dir, "timeout", run.Timeout)
	err := cmd.Run()
	logger.Info("claudeengine: a plugin hook ended", "command", run.Command, "err", err)
	if err == nil {
		return stdout.Bytes(), nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("claudeengine: plugin hook %q timed out after %s", run.Command, run.Timeout)
	}
	return nil, fmt.Errorf("claudeengine: plugin hook %q failed: %w: %s", run.Command, err, strings.TrimSpace(stderr.String()))
}
