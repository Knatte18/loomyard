// stopserver.go stops the whole tmux server of a hub by its path, whether or not the hub's directory still exists.

package reedengine

import (
	"errors"
	"fmt"
	"os/exec"
)

// StopServer kills the tmux server whose socket key ServerName(hubPath) derives, and removes its socket file.
// It works from the path alone, so it reaches the server of a hub whose directory is already gone.
// It reports whether a live server answered and was stopped, and false when none was live.
// A server that outlives its kill past the bounded wait leaves its socket file, with no error.
// A tmux that cannot be run is an error.
func StopServer(tmuxPath, hubPath string) (bool, error) {
	key := ServerName(hubPath)
	socketDir := socketDirFromEnv()
	cmd := NewTmuxCmd(tmuxPath, key)

	// Any command that needs a server tells a live one from none: tmux exits non-zero when no server answers.
	if _, err := cmd.output("display-message", "-p", "#{pid}"); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return false, fmt.Errorf("probe the tmux server %s: %w", key, err)
		}
		return false, removeStaleSocket(socketDir, key)
	}
	if err := cmd.run("kill-server"); err != nil {
		return false, fmt.Errorf("kill the tmux server %s: %w", key, err)
	}
	if err := removeSocketFileOnceGone(socketDir, key, socketGoneWait); err != nil {
		return true, fmt.Errorf("remove the socket file of the stopped tmux server %s: %w", key, err)
	}
	return true, nil
}
