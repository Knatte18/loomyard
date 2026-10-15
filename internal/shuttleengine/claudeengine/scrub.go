// scrub.go declares the environment keys a process spawned beside a Claude session must not inherit.

package claudeengine

// ScrubbedEnv returns the environment keys Claude Code injects into its own process: the exact names and the key prefixes.
// A server spawned from inside a Claude session that kept them would hand every child a stale session identity.
func ScrubbedEnv() (names []string, prefixes []string) {
	return []string{"CLAUDECODE"}, []string{"CLAUDE_CODE_"}
}
