// state.go — muxpoc's durable local state: `_mhgo/muxpoc-state.json`.
//
// This file is the source of truth that survives a psmux crash or a machine reboot. It
// records the socket, the session geometry, and — per pane — the worktree-less cwd, the
// pane role, and the mux-assigned claude `--session-id`. `up` (cold start) and the daemon
// (hot recovery) both rebuild the layout and `claude --resume <session-id>` from it.
package muxpoc

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PaneState is one pane mux owns: its cwd, role, and the claude session id to resume.
type PaneState struct {
	Role      string `json:"role"`       // "main" | "reviewer"
	SessionID string `json:"session_id"` // mux-assigned claude --session-id
	CWD       string `json:"cwd"`        // in-place working dir (no worktree)
	PaneID    string `json:"pane_id"`    // last-known psmux pane id (%N); advisory, re-derived on rebuild
}

// State is the whole muxpoc layout for one repo (one in-place column for the PoC).
type State struct {
	Socket  string      `json:"socket"`  // psmux -L label
	Session string      `json:"session"` // psmux session name
	Width   int         `json:"width"`
	Height  int         `json:"height"`
	Panes   []PaneState `json:"panes"` // index 0 = main column; later = stacked reviewers
}

// statePath returns <cwd>/_mhgo/muxpoc-state.json.
func statePath(cwd string) string {
	return filepath.Join(cwd, "_mhgo", "muxpoc-state.json")
}

// loadState reads the state file; ok=false (no error) when it does not exist yet.
func loadState(cwd string) (State, bool, error) {
	var s State
	data, err := os.ReadFile(statePath(cwd))
	if os.IsNotExist(err) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, false, err
	}
	return s, true, nil
}

// saveState writes the state file atomically (temp + rename), creating _mhgo/ as needed.
func saveState(cwd string, s State) error {
	dir := filepath.Join(cwd, "_mhgo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath(cwd) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(cwd))
}

// clearState removes the state file (used by `down`).
func clearState(cwd string) error {
	err := os.Remove(statePath(cwd))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
