// scrub.go declares the environment keys a process spawned beside a Codex session must not inherit.

package codexengine

// ScrubbedEnv returns the environment keys Codex reads or injects: the CODEX_ prefix.
// A server spawned from a shell that set one, such as CODEX_HOME, would hand it to every session,
// so each Codex launch sets its own on its line.
func ScrubbedEnv() (names []string, prefixes []string) {
	return nil, []string{"CODEX_"}
}
