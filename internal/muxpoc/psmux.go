// psmux.go — thin exec wrapper around the psmux CLI for the muxpoc proof-of-concept.
//
// muxpoc is a deliberately minimal, parallel-to-the-future-`internal/mux` spike. Its
// purpose is to prove the HARD part of the mux design — a daemon that keeps a Claude
// session alive across a psmux crash and recovers it with `claude --resume` — using a
// single in-place column (no worktrees). It is a reference, not the real module.
//
// The single most important thing encoded here is env hygiene: psmux panes inherit the
// environment of the process that STARTED the psmux server, and if that environment
// carries the Claude Code parent-session markers (CLAUDECODE, CLAUDE_CODE_CHILD_SESSION,
// …) the claude launched in a pane treats itself as a nested child and silently stops
// persisting its transcript — which breaks `--resume`. Since mhgo is normally invoked
// from inside a Claude Code session (claude spawning reviewers/implementers), every psmux
// invocation here runs with a sanitized env. See docs/modules/mux-exploration.md.
package muxpoc

import (
	"os"
	"os/exec"
	"strings"
)

// Runner executes psmux commands against one isolated server socket (`psmux -L <socket>`).
type Runner struct {
	Bin    string // psmux binary (PATH name or absolute path)
	Socket string // -L socket label, keeps the PoC off the operator's real psmux server
}

// sanitizeEnv returns os.Environ() with the inherited Claude Code parent-session
// variables removed. These are what make a pane's claude believe it is a nested child
// and suppress transcript persistence (the root cause found during exploration):
//
//	CLAUDECODE, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_SESSION_ID,
//	CLAUDE_CODE_ENTRYPOINT, CLAUDE_CODE_SSE_PORT, and any other CLAUDE_CODE_* marker.
//
// Stripping them on the env of the process that starts the psmux server means the server
// — and therefore every pane and every claude launched in a pane — inherits a clean,
// top-level environment.
func sanitizeEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if key == "CLAUDECODE" || strings.HasPrefix(key, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// strippedEnvKeys reports which inherited Claude Code vars sanitizeEnv would drop, for
// diagnostics (`muxpoc status` / logging) so the env hygiene is observable.
func strippedEnvKeys() []string {
	var keys []string
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if key == "CLAUDECODE" || strings.HasPrefix(key, "CLAUDE_CODE_") {
			keys = append(keys, key)
		}
	}
	return keys
}

// command builds an *exec.Cmd for `psmux -L <socket> <args...>` with the sanitized env
// and no inherited console window (CREATE_NO_WINDOW), matching board's spawn discipline.
func (r *Runner) command(args ...string) *exec.Cmd {
	full := append([]string{"-L", r.Socket}, args...)
	cmd := exec.Command(r.Bin, full...)
	cmd.Env = sanitizeEnv()
	applyHidden(cmd) // platform helper: CREATE_NO_WINDOW on Windows, no-op elsewhere
	return cmd
}

// run executes a psmux command and returns trimmed combined output. A non-nil error
// carries the output as its message so callers can surface psmux's own diagnostics.
func (r *Runner) run(args ...string) (string, error) {
	out, err := r.command(args...).CombinedOutput()
	s := strings.TrimRight(string(out), "\r\n")
	if err != nil {
		if s != "" {
			return s, &psmuxError{args: args, msg: s, err: err}
		}
		return s, &psmuxError{args: args, msg: err.Error(), err: err}
	}
	return s, nil
}

type psmuxError struct {
	args []string
	msg  string
	err  error
}

func (e *psmuxError) Error() string {
	return "psmux " + strings.Join(e.args, " ") + ": " + e.msg
}
func (e *psmuxError) Unwrap() error { return e.err }
