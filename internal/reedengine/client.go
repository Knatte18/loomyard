// client.go answers the three tmux facts lyx loom start's handover decision needs, so loom never spells
// a tmux argv itself: whether the caller's $TMUX names this engine's server, which session the
// caller's own pane belongs to, and the argv that switches a client onto this engine's session.

package reedengine

import (
	"fmt"
	"strings"
)

// OwnsTmuxEnv reports whether tmuxEnv, a $TMUX value, names this engine's tmux server.
// $TMUX is "<socket path>,<pid>,<index>" and reed's -L socket key is the socket file's base name, so
// the base name of the first comma field is compared against Socket(). Both path separators are
// honoured so a Windows-style path resolves the same way on any host. An empty or malformed value
// is false.
func (e *Engine) OwnsTmuxEnv(tmuxEnv string) bool {
	fields := strings.Split(tmuxEnv, ",")
	if len(fields) < 3 {
		return false
	}
	path := fields[0]
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return path != "" && path == e.Socket()
}

// ClientSession returns the name of the session the client owning tmuxPane (the caller's $TMUX_PANE)
// is attached to, asked of reed's own socket. The spawn is logged at Debug by the tmux overlay.
func (e *Engine) ClientSession(tmuxPane string) (string, error) {
	if tmuxPane == "" {
		return "", fmt.Errorf("reed: no tmux pane given")
	}
	out, err := e.tmux.output("display-message", "-p", "-t", tmuxPane, "#{session_name}")
	if err != nil {
		return "", fmt.Errorf("reed: read client session for pane %s: %w", tmuxPane, err)
	}
	return strings.TrimSpace(out), nil
}

// SwitchClientArgv returns the tmux argv that switches the calling client onto this engine's session,
// on reed's socket, with the same exact-target form the attach argv uses. The caller runs it.
func (e *Engine) SwitchClientArgv() []string {
	return []string{"-L", e.Socket(), "switch-client", "-t", exactSessionTarget(e.SessionName())}
}
