// review.go — review subcommand: add reviewer pane.
//
// cmdReview adds a new review pane to an active session via split-window.

package muxpoc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Knatte18/mhgo/internal/output"
)

// cmdReview adds a reviewer pane to the active session.
// Returns exit code (0 on success, 1 on error).
func cmdReview(out io.Writer, cfg Config) int {
	cwd, _ := os.Getwd()

	state, _, err := LoadState(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("load state: %v", err))
	}
	if state == nil {
		return output.Err(out, "no active session: run 'mhgo muxpoc up' first")
	}

	mux := NewPsmuxCmd(cfg)
	up, err := mux.hasSession(state.Session)
	if err != nil {
		return output.Err(out, fmt.Sprintf("check session: %v", err))
	}
	if !up {
		return output.Err(out, "session not running: run 'mhgo muxpoc up' first")
	}

	sid, err := newSessionID()
	if err != nil {
		return output.Err(out, fmt.Sprintf("generate session id: %v", err))
	}

	// Resolve claude path
	claudePath := cfg.ClaudePath
	if claudePath == "" {
		var err error
		claudePath, err = exec.LookPath("claude")
		if err != nil {
			return output.Err(out, fmt.Sprintf("claude not found on PATH: %v", err))
		}
	}

	// Split window for the new review pane
	if err := mux.run("split-window", "-t", state.Session, "-v", "-p", "30", cfg.PwshPath); err != nil {
		return output.Err(out, fmt.Sprintf("split window: %v", err))
	}

	// Build launch command from template
	launchCmd := expandTpl(cfg.LaunchTpl, sid, "")
	launchCmd = strings.ReplaceAll(launchCmd, "%CLAUDE%", claudePath)

	// Send launch command to the session
	if err := mux.run("send-keys", "-t", state.Session, launchCmd, "Enter"); err != nil {
		return output.Err(out, fmt.Sprintf("send launch: %v", err))
	}

	// Append new pane to state and save
	state.Panes = append(state.Panes, Pane{
		ID:        "",
		SessionID: sid,
		Kind:      "review",
	})
	if err := SaveState(cwd, state); err != nil {
		return output.Err(out, fmt.Sprintf("save state: %v", err))
	}

	return output.Ok(out, map[string]any{
		"session_id": sid,
		"socket":     state.Socket,
		"message":    "review pane added",
	})
}
