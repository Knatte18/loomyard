// up.go — up subcommand: cold-start and cold-recover.
//
// coldStart initializes a new psmux session with a primary pane. coldRecover
// reconnects to an existing session and restarts its panes from saved state.

package muxpoc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/output"
)

// cmdUp is the entry point for the 'mhgo muxpoc up' subcommand.
// Returns exit code (0 on success, 1 on error).
func cmdUp(out io.Writer, cfg Config) int {
	cwd := cfg.WorktreeRoot

	state, warn, err := LoadState(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("load state: %v", err))
	}
	if warn != "" {
		fmt.Fprintln(os.Stderr, warn)
	}

	mux := NewPsmuxCmd(cfg)

	if state != nil {
		// State exists — check if session is still up
		up, err := mux.hasSession(state.Session)
		if err != nil {
			return output.Err(out, fmt.Sprintf("check session: %v", err))
		}
		if up {
			// Session is running — report already up
			if len(state.Panes) == 0 {
				return output.Err(out, "state has no panes")
			}
			return output.Ok(out, map[string]any{
				"session_id":   state.Panes[0].SessionID,
				"socket":       state.Socket,
				"stripped_env": state.StrippedEnv,
				"message":      "already up",
			})
		}
		// Session is down but state exists — recover
		return coldRecover(out, cfg, cwd, state, mux)
	}

	// No state — cold start
	return coldStart(out, cfg, cwd, mux)
}

// coldStart initializes a new psmux session and launches the primary claude instance.
func coldStart(out io.Writer, cfg Config, cwd string, mux PsmuxCmd) int {
	sock := socketName(cwd)
	sessionName := sock
	// Socket name and session name are the same — stable per repo.

	sid, err := newSessionID()
	if err != nil {
		return output.Err(out, fmt.Sprintf("generate session id: %v", err))
	}

	// Build sanitised environment
	clean := sanitizeEnv(os.Environ())
	stripped := strippedEnvKeys(os.Environ())

	// Resolve claude path
	claudePath := cfg.ClaudePath
	if claudePath == "" {
		var err error
		claudePath, err = exec.LookPath("claude")
		if err != nil {
			return output.Err(out, fmt.Sprintf("claude not found on PATH: %v", err))
		}
	}

	// Build psmux new-session command
	cmd := exec.Command(
		cfg.PsmuxPath,
		"-L", sock,
		"new-session", "-d", "-s", sessionName,
		"-x", fmt.Sprintf("%d", cfg.Width),
		"-y", fmt.Sprintf("%d", cfg.Height),
		cfg.PwshPath,
	)
	cmd.Env = clean
	spawnServer(cmd)

	if err := cmd.Start(); err != nil {
		return output.Err(out, fmt.Sprintf("start psmux: %v", err))
	}

	// Wait for session to come up
	time.Sleep(500 * time.Millisecond)
	hasSession := false
	for i := 0; i < 3; i++ {
		up, err := mux.hasSession(sessionName)
		if err != nil {
			return output.Err(out, fmt.Sprintf("check session: %v", err))
		}
		if up {
			hasSession = true
			break
		}
		if i < 2 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Verify session started before proceeding
	if !hasSession {
		return output.Err(out, "psmux session did not start in time")
	}

	// Build launch command from template
	launchCmd := expandTpl(cfg.LaunchTpl, sid, "")
	launchCmd = strings.ReplaceAll(launchCmd, "%CLAUDE%", claudePath)

	// Send launch command to the session
	if err := mux.run("send-keys", "-t", sessionName, launchCmd, "Enter"); err != nil {
		return output.Err(out, fmt.Sprintf("send launch: %v", err))
	}

	// Capture the main pane's id so state records what runs where.
	mainPaneID, err := mux.activePaneID(sessionName)
	if err != nil {
		return output.Err(out, fmt.Sprintf("get pane id: %v", err))
	}

	// Save state
	state := &MuxpocState{
		Session:     sessionName,
		Socket:      sock,
		StrippedEnv: stripped,
		Panes: []Pane{{
			ID:        mainPaneID,
			SessionID: sid,
			Kind:      "main",
		}},
	}
	if err := SaveState(cwd, state); err != nil {
		return output.Err(out, fmt.Sprintf("save state: %v", err))
	}

	return output.Ok(out, map[string]any{
		"session_id":   sid,
		"socket":       sock,
		"stripped_env": stripped,
		"message":      "started",
	})
}

// coldRecover reconnects to an existing session and restarts its panes.
func coldRecover(out io.Writer, cfg Config, cwd string, state *MuxpocState, mux PsmuxCmd) int {
	// Build sanitised environment
	clean := sanitizeEnv(os.Environ())

	// Resolve claude path
	claudePath := cfg.ClaudePath
	if claudePath == "" {
		var err error
		claudePath, err = exec.LookPath("claude")
		if err != nil {
			return output.Err(out, fmt.Sprintf("claude not found on PATH: %v", err))
		}
	}

	// Build new-session command targeting the same socket and session
	cmd := exec.Command(
		cfg.PsmuxPath,
		"-L", state.Socket,
		"new-session", "-d", "-s", state.Session,
		"-x", fmt.Sprintf("%d", cfg.Width),
		"-y", fmt.Sprintf("%d", cfg.Height),
		cfg.PwshPath,
	)
	cmd.Env = clean
	spawnServer(cmd)

	if err := cmd.Start(); err != nil {
		return output.Err(out, fmt.Sprintf("start psmux: %v", err))
	}

	// Wait for session to come up
	time.Sleep(500 * time.Millisecond)
	hasSession := false
	for i := 0; i < 3; i++ {
		up, err := mux.hasSession(state.Session)
		if err != nil {
			return output.Err(out, fmt.Sprintf("check session: %v", err))
		}
		if up {
			hasSession = true
			break
		}
		if i < 2 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Verify session started before proceeding
	if !hasSession {
		return output.Err(out, "psmux session did not start in time")
	}

	// Restart each pane
	for i := range state.Panes {
		pane := state.Panes[i]
		// Record the pane id — psmux assigns fresh ids across a server restart.
		// Review panes split a new pane (capture its id from split-window -P -F);
		// the main pane already exists from new-session (query the active pane).
		var paneID string
		if pane.Kind == "review" {
			paneOut, err := mux.output("split-window", "-t", state.Session, "-v", "-p", "30", "-P", "-F", "#{pane_id}", cfg.PwshPath)
			if err != nil {
				return output.Err(out, fmt.Sprintf("split window: %v", err))
			}
			paneID = strings.TrimSpace(paneOut)
		} else {
			id, err := mux.activePaneID(state.Session)
			if err != nil {
				return output.Err(out, fmt.Sprintf("get pane id: %v", err))
			}
			paneID = id
		}
		state.Panes[i].ID = paneID

		// Build resume command from template
		resumeCmd := expandTpl(cfg.ResumeTpl, pane.SessionID, "")
		resumeCmd = strings.ReplaceAll(resumeCmd, "%CLAUDE%", claudePath)

		// Send resume command to the session
		if err := mux.run("send-keys", "-t", state.Session, resumeCmd, "Enter"); err != nil {
			return output.Err(out, fmt.Sprintf("send resume: %v", err))
		}
	}

	// Re-tile so the bottom (active) pane dominates, matching the live layout.
	if err := mux.applyColumnLayout(state.Session); err != nil {
		return output.Err(out, fmt.Sprintf("apply layout: %v", err))
	}

	// Persist refreshed pane ids (psmux reassigns them across a server restart).
	if err := SaveState(cwd, state); err != nil {
		return output.Err(out, fmt.Sprintf("save state: %v", err))
	}

	return output.Ok(out, map[string]any{
		"session":         state.Session,
		"socket":          state.Socket,
		"stripped_env":    state.StrippedEnv,
		"recovered_panes": len(state.Panes),
		"message":         "cold-recovered",
	})
}
