// attach.go — attach subcommand: pop session into maximized terminal.
//
// cmdAttach launches the psmux session in a maximized terminal (Windows Terminal
// on Windows, plain psmux attach on other platforms).

package muxpoccli

import (
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/output"
)

// cmdAttach pops the session into a maximized terminal.
// Returns exit code (0 on success, 1 on error).
func cmdAttach(out io.Writer, cfg Config) int {
	cwd := cfg.WorktreeRoot

	state, err := LoadState(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("load state: %v", err))
	}
	if state == nil {
		return output.Err(out, "no active session")
	}

	mux := NewPsmuxCmd(cfg)
	up, err := mux.hasSession(state.Session)
	if err != nil {
		return output.Err(out, fmt.Sprintf("check session: %v", err))
	}
	if !up {
		return output.Err(out, "session not running")
	}

	if err := spawnAttach(cfg.PsmuxPath, state.Socket, state.Session); err != nil {
		return output.Err(out, fmt.Sprintf("attach: %v", err))
	}

	return output.Ok(out, map[string]any{
		"session": state.Session,
		"socket":  state.Socket,
		"message": "attach launched",
	})
}
