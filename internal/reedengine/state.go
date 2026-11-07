// state.go defines the persisted strand record and ReedState container, plus the .lym/reed.json load/save wrappers.
// Strand carries the fields a caller writes as opaque data.
// Reed stores them and reads one only where its field comment says so.
// Retiring is lifecycle bookkeeping that a reed verb sets (MarkRetiring); no caller of AddStrand writes it.

package reedengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/state"
)

// Strand is the persisted record for one tmux pane reedengine owns.
// Its Display field uses the display vocabulary of render.Display.
type Strand struct {
	GUID      string         `json:"guid"`
	Name      string         `json:"name"`
	Worktree  string         `json:"worktree"`
	Parent    string         `json:"parent,omitempty"`
	Cmd       string         `json:"cmd"`
	ResumeCmd string         `json:"resumeCmd,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
	PaneID    string         `json:"paneId"`
	Display   render.Display `json:"display"`
	// Retiring records that a caller of `reed remove --detach` asked for this strand's removal.
	// It grants nothing by itself: a caller that finds it set only completes the request.
	Retiring bool `json:"retiring,omitempty"`
	// DoneWhen lists paths whose joint existence means the strand's work finished.
	// The spawner sets it at add time.
	// Reed only stats them.
	// Resume drops a non-live strand whose list is non-empty and fully present, instead of relaunching it.
	DoneWhen []string `json:"doneWhen,omitempty"`
	// Segment is the loom segment its spawner named.
	// It is an opaque display field: reed reads it only to look up the segment's color.
	// A strand recorded before the field existed decodes with none.
	Segment string `json:"segment,omitempty"`
	// Provider is the opaque tag its spawner set.
	// Reed stores it and keys the session namers by it, never reading what it means.
	// A strand recorded before the field existed, or added outside shuttle, decodes with none.
	Provider string `json:"provider,omitempty"`
	// CreelHooks is the opaque mark its spawner sets when the strand's session runs creel's hook delivery.
	// Reed stores it and never reads what it means.
	// A strand recorded before the field existed, or added outside shuttle, decodes with none.
	CreelHooks bool `json:"creelHooks,omitempty"`
	// LymBin is the absolute `lym` path the strand's pane prelude exported as LYM_BIN at its latest launch.
	// It is empty when no prelude was composed.
	// A strand recorded before the field existed decodes with none.
	LymBin string `json:"lymBin,omitempty"`
	// Color is the palette color Segment resolved to, filled on the strand an add or replace returns.
	// It is never persisted.
	Color segmentcolor.Color `json:"-"`
}

// ReedState is the persisted record for one hub's tmux server.
// It holds the socket name, the session, the stripped env keys, and every strand as a flat list.
type ReedState struct {
	Socket      string   `json:"socket"`
	Session     string   `json:"session"`
	StrippedEnv []string `json:"strippedEnv"`
	Strands     []Strand `json:"strands"`
	// SelvagePaneID is the tmux pane id of the always-present Selvage pane.
	// It sits outside Strands, since Selvage is a separate construct and never itself a strand:
	// strand accounting, the preferred split target, and both halves of reconcile's kill schedule all skip it.
	// Empty means Selvage has not yet been created: a fresh worktree, or a server rebirth that cleared every binding.
	// It must then be (re)created at the next up/resume boot.
	SelvagePaneID string `json:"selvagePaneId,omitempty"`
	// PaneGeneration identifies the tmux session incarnation that every pane id in this state was bound against:
	// the strands' ids and SelvagePaneID alike.
	// It is the one field in this struct that reed reads back semantically rather than carrying for its caller.
	// A persisted pane id cannot be told apart from a live pane belonging to something else without it.
	// Tmux pane ids are server-global and restart at %0 on every server rebirth.
	// See generation.go for the two guards built on it.
	// A zero value means the state predates this field or has no bindings yet.
	// Such a state is adopted rather than treated as a mismatch.
	PaneGeneration PaneGeneration `json:"paneGeneration"`
	// WindowGroups records the own-window groups that have a strand, keyed by group name.
	// An older file decodes with none.
	WindowGroups map[string]WindowGroup `json:"windowGroups,omitempty"`
}

// PaneGeneration identifies one tmux session incarnation.
// It lets a persisted pane id be told apart from a live pane that merely reuses its number.
// The zero value means no generation was recorded.
type PaneGeneration struct {
	// SessionName is the tmux session name the bindings were minted under.
	// It is the lookup key for the still-alive-orphan check.
	// It is the only field that a tmux rename-session can change without the session becoming a different one.
	// SameIncarnation therefore ignores it.
	SessionName string `json:"sessionName,omitempty"`
	// TmuxSessionID is tmux's own session id ("$0", "$1", …), unique per session within one server.
	TmuxSessionID string `json:"tmuxSessionId,omitempty"`
	// ServerPID is the tmux SERVER process's pid, which distinguishes two servers on one socket.
	ServerPID string `json:"serverPid,omitempty"`
	// Created is the session's creation time in epoch seconds.
	// It distinguishes a session id reused by a later server on the same socket.
	Created string `json:"created,omitempty"`
}

// Recorded reports whether this generation carries an identity.
// The zero value is what a pre-field or freshly initialized state carries.
func (g PaneGeneration) Recorded() bool {
	return g != PaneGeneration{}
}

// SameIncarnation reports whether g and other name the same live tmux session, ignoring SessionName.
// The name is excluded: tmux rename-session changes it in place without making the session a different one.
// Treating that as a new generation would discard a healthy worktree's whole binding table.
func (g PaneGeneration) SameIncarnation(other PaneGeneration) bool {
	return g.TmuxSessionID == other.TmuxSessionID &&
		g.ServerPID == other.ServerPID &&
		g.Created == other.Created
}

// reedStateFileName is the reed.json file name inside the .lym directory.
const reedStateFileName = "reed.json"

// UnmarshalJSON decodes a ReedState and refuses a file whose entire content is the JSON document "null".
//
// encoding/json accepts "null" into any type and leaves the value untouched.
// Without this check such a file would decode to an empty ReedState.
// Reed would then report zero strands, and the persisted table would go unseen.
// Truncated, partial, and garbage files already fail loud, and this shape has to fail loud too.
func (s *ReedState) UnmarshalJSON(data []byte) error {
	if string(bytes.TrimSpace(data)) == "null" {
		return errors.New(`the file's entire content is the JSON document "null", not a state object`)
	}
	// A local defined type with the same fields but no method set, so this call cannot recurse
	// back into UnmarshalJSON.
	type reedStateFields ReedState
	var fields reedStateFields
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*s = ReedState(fields)
	return nil
}

// LoadState reads the ReedState persisted at dotLymDir/reed.json.
// It returns (nil, nil) if the file is absent, and (nil, err) on a corrupt file or a read error.
// The error for a corrupt file names the file and both remedies, so callers return it unwrapped.
func LoadState(dotLymDir string) (*ReedState, error) {
	path := filepath.Join(dotLymDir, reedStateFileName)
	lockPath := path + ".lock"

	v, found, err := state.ReadJSON[ReedState](path, lockPath)
	if err != nil {
		return nil, unreadableStateError(path, err)
	}
	if !found {
		return nil, nil
	}
	return &v, nil
}

// unreadableStateError describes a reed.json that exists but cannot be decoded.
// It names the file and the two ways out, because a bare decode error names neither.
// A crash, a partial write, or a full disk can leave such a file while the tmux session it describes keeps running.
// Every verb that loads state then refuses to run.
// The one verb that still works, `down`, destroys the running work.
//
// Both remedies are named because they trade differently, and the operator, not reed, has to pick.
// Deleting the file keeps the session: its panes and processes keep running and can be attached to.
// They stay that way only until the next mutating verb (up, resume, add, or remove) reaps them.
// An alive Selvage then authorizes reaping every other pane when that verb reconciles.
//
// Reed does not repair the file automatically, since every repair it could perform discards the strand table.
func unreadableStateError(path string, err error) error {
	return fmt.Errorf(
		"reed state file %s is unreadable: %w — the tmux session it describes may still be running, "+
			`so reed will not guess at its contents. Either run "lym reed down" to tear that session down and clear the file, `+
			"or delete %s by hand to keep the session for now (its panes and their processes keep running, untracked, "+
			"but are reaped by the next up/resume/add/remove — attach is the way back to that work) and lose only reed's strand tracking",
		path, err, path)
}

// SaveState writes s to dotLymDir/reed.json atomically.
func SaveState(dotLymDir string, s *ReedState) error {
	path := filepath.Join(dotLymDir, reedStateFileName)
	lockPath := path + ".lock"
	return state.WriteJSON(path, lockPath, s)
}
