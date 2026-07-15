// env.go implements the module's single env-hygiene chokepoint:
// CleanClaudeEnv strips every Claude-Code-injected environment variable
// before a tmux server or pane spawns a child process, so that the child
// never inherits a stale session identity from its own launcher.

package muxengine

import "strings"

// CleanClaudeEnv returns environ with every entry whose key (the part
// before "=") is exactly CLAUDECODE or has the prefix CLAUDE_CODE_ removed,
// plus the list of stripped keys in environ order. This is the single
// documented chokepoint for the env-hygiene decision: any caller that spawns
// a tmux server or a child process (mux's own server-spawn in batch 5, or
// shuttle reusing this helper) must route its environ through here first, so
// a spawned Claude session never inherits CLAUDECODE/CLAUDE_CODE_* from the
// process that launched it.
func CleanClaudeEnv(environ []string) (clean []string, strippedKeys []string) {
	clean = []string{}
	strippedKeys = []string{}
	for _, entry := range environ {
		key := strings.SplitN(entry, "=", 2)[0]
		if key == "CLAUDECODE" || strings.HasPrefix(key, "CLAUDE_CODE_") {
			strippedKeys = append(strippedKeys, key)
			continue
		}
		clean = append(clean, entry)
	}
	return clean, strippedKeys
}
