// status.go — status subcommand: report session and pane status.
//
// cmdStatus reports comprehensive status: whether state exists, whether the
// server is running, live pane information from the server, and saved pane metadata.

package muxpoc

import (
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/output"
)

// cmdStatus reports session and pane status.
// Returns exit code (0 on success, 1 on error).
func cmdStatus(out io.Writer, cfg Config) int {
	cwd := cfg.WorktreeRoot

	haveState := false
	var state *MuxpocState

	state, _, err := LoadState(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("load state: %v", err))
	}
	if state != nil {
		haveState = true
	}

	mux := NewPsmuxCmd(cfg)
	serverUp := false
	if state != nil {
		up, err := mux.hasSession(state.Session)
		if err == nil {
			serverUp = up
		}
		// Ignore errors — server may be down
	}

	// Initialize result fields
	session := ""
	socket := ""
	strippedEnv := []string(nil)
	statePanes := []Pane(nil)

	if state != nil {
		session = state.Session
		socket = state.Socket
		strippedEnv = state.StrippedEnv
		statePanes = state.Panes
	}

	livePanes := []LivePane(nil)
	if serverUp {
		panes, err := mux.listPanes(state.Session)
		if err == nil {
			livePanes = panes
		}
		// Ignore errors — server may have issues
	}

	return output.Ok(out, map[string]any{
		"have_state":   haveState,
		"server_up":    serverUp,
		"session":      session,
		"socket":       socket,
		"stripped_env": strippedEnv,
		"state_panes":  statePanes,
		"live_panes":   livePanes,
	})
}
