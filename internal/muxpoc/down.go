// down.go — down subcommand: intentional session teardown.
//
// cmdDown stops the psmux server and deletes the state file. This marks an
// intentional shutdown, distinguishing from a crash (which would leave state
// for recovery via 'up').

package muxpoc

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/mhgo/internal/output"
)

// cmdDown stops the session and deletes state.
// Returns exit code (0 on success, 1 on error).
func cmdDown(out io.Writer, cfg Config) int {
	cwd, _ := os.Getwd()

	state, _, err := LoadState(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("load state: %v", err))
	}
	if state == nil {
		return output.Ok(out, map[string]any{
			"message": "no active session",
		})
	}

	mux := NewPsmuxCmd(cfg)
	// Kill the server — ignore errors (server may already be dead; down is idempotent)
	_ = mux.run("kill-server")

	// Delete state — ignore errors (file may already be gone)
	_ = DeleteState(cwd)

	return output.Ok(out, map[string]any{
		"session": state.Session,
		"message": "session stopped and state deleted",
	})
}
