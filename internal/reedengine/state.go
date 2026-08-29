// state.go defines the persisted strand record and ReedState container, plus the .lyx/reed.json
// load/save wrappers and the mapper that projects the persisted record down to render.Strand.
// This is the module's dumb-carrier contract in concrete form: Strand stores every field a caller
// writes (cmd, resumeCmd, sessionId, worktree, name) and reedengine itself reads none of them
// semantically — only Display feeds the layout decision, via toRenderStrands.

package reedengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/state"
)

// Strand is the persisted record for one tmux pane reedengine owns, reusing render.Display for the
// display vocabulary.
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
}

// ReedState is the persisted record for one hub's tmux server: the socket name, the session,
// stripped env keys, and every strand as a flat list.
type ReedState struct {
	Socket      string   `json:"socket"`
	Session     string   `json:"session"`
	StrippedEnv []string `json:"strippedEnv"`
	Strands     []Strand `json:"strands"`
	// HeaderPaneID is the tmux pane id of the always-present header pane —
	// deliberately outside Strands, since the header is a first-class but
	// separate construct and never itself a strand (Shared Decision
	// header-is-not-a-strand): it is excluded from strand accounting, from
	// being the preferred split target, and from both halves of reconcile's
	// kill schedule. Empty means the header pane has not yet been created (a
	// fresh worktree, or a server rebirth that cleared every binding) and
	// must be (re)created at the next up/resume boot.
	HeaderPaneID string `json:"headerPaneId,omitempty"`
	// PaneGeneration identifies the tmux session incarnation every PaneID
	// above — the strands' and HeaderPaneID alike — was bound against. It is
	// the one field in this struct reed reads back semantically rather than
	// carrying for its caller: without it a persisted pane id cannot be told
	// apart from a live pane belonging to something else, because tmux pane
	// ids are server-global and restart at %0 on every server rebirth. See
	// generation.go for the two guards built on it. A zero value means the
	// state predates this field (or has no bindings yet) and is adopted
	// rather than treated as a mismatch.
	PaneGeneration PaneGeneration `json:"paneGeneration"`
}

// PaneGeneration identifies one tmux session incarnation, so a persisted pane id can be told apart
// from a live pane that merely reuses its number.
// The zero value means "no generation recorded" and is what Recorded reports on.
type PaneGeneration struct {
	// SessionName is the tmux session name the bindings were minted under. It is the lookup key for
	// the still-alive-orphan check, and the only field that can change without the session becoming
	// a different one (tmux rename-session), which is why SameIncarnation ignores it.
	SessionName string `json:"sessionName,omitempty"`
	// TmuxSessionID is tmux's own session id ("$0", "$1", …), unique per session within one server.
	TmuxSessionID string `json:"tmuxSessionId,omitempty"`
	// ServerPID is the tmux SERVER process's pid, which distinguishes two servers on one socket.
	ServerPID string `json:"serverPid,omitempty"`
	// Created is the session's creation time in epoch seconds, which distinguishes a session id
	// reused by a later server on the same socket.
	Created string `json:"created,omitempty"`
}

// Recorded reports whether this generation carries an identity at all, as opposed to being the zero
// value a pre-field or freshly initialized state carries.
func (g PaneGeneration) Recorded() bool {
	return g != PaneGeneration{}
}

// SameIncarnation reports whether g and other name the same live tmux session, ignoring SessionName.
// The name is excluded deliberately: tmux rename-session changes a session's name in place without
// making it a different session, and treating that as a new generation would discard a healthy
// worktree's whole binding table.
func (g PaneGeneration) SameIncarnation(other PaneGeneration) bool {
	return g.TmuxSessionID == other.TmuxSessionID &&
		g.ServerPID == other.ServerPID &&
		g.Created == other.Created
}

// reedStateFileName is the reed.json file name inside .lyx directory.
const reedStateFileName = "reed.json"

// UnmarshalJSON decodes a ReedState, refusing a file whose entire content is the JSON document
// "null".
//
// encoding/json accepts "null" into any type and leaves the value untouched, so without this a
// four-byte corrupt file decoded to a valid EMPTY ReedState and reed answered `status` with
// ok:true and zero strands — discarding the whole persisted table silently (R5 review finding
// R5-F1). Silence is the worst possible outcome for a state file: every other corruption shape
// (truncation, a partial write, garbage bytes) already fails loud, and this one shape has to join
// them rather than being papered over with a plausible-looking empty answer.
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

// LoadState reads the ReedState persisted at dotLyxDir/reed.json.
// Returns (nil, nil) if absent, (nil, err) on corrupt or read errors.
// A corrupt-file error is self-describing — it names the file and both remedies — so callers pass
// it through bare rather than prefixing it (see unreadableStateError).
func LoadState(dotLyxDir string) (*ReedState, error) {
	path := filepath.Join(dotLyxDir, reedStateFileName)
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

// unreadableStateError describes a reed.json that exists but cannot be decoded, naming the file and
// the two ways out.
//
// It exists because the bare decode error was not actionable and the situation it describes is not
// rare: a crash, a `kill -9`, a full disk, or a power loss during a write leaves a partial or
// invalid file, and reed then refuses EVERY verb that loads state — up, resume, status, add,
// remove, even attach — with `load state: unmarshal state: unexpected end of JSON input`, naming
// neither the file nor a remedy, while the tmux session and every strand process it describes stay
// alive and healthy (R5 review finding R5-F1, reproduced live). The operator is left holding
// running work they can no longer see, resume, or attach to, and the only verb that still functions
// (`down`) is the one that destroys it.
//
// Both remedies are named because they are genuinely different trades, and the operator — not
// reed — has to pick. Deleting the file keeps the session: the panes and their processes keep
// running and can be attached to, but only until the next mutating verb (up, resume, add, or
// remove) reaps them, since an alive header now authorizes reaping every other pane the moment
// one of those verbs reconciles.
//
// Repairing the file automatically is deliberately not offered: every repair reed could perform
// amounts to discarding the strand table, which is exactly the silent loss the "null" refusal above
// exists to prevent.
func unreadableStateError(path string, err error) error {
	return fmt.Errorf(
		"reed state file %s is unreadable: %w — the tmux session it describes may still be running, "+
			`so reed will not guess at its contents. Either run "lyx reed down" to tear that session down and clear the file, `+
			"or delete %s by hand to keep the session for now (its panes and their processes keep running, untracked, "+
			"but are reaped by the next up/resume/add/remove — attach is the way back to that work) and lose only reed's strand tracking",
		path, err, path)
}

// SaveState writes s to dotLyxDir/reed.json atomically.
func SaveState(dotLyxDir string, s *ReedState) error {
	path := filepath.Join(dotLyxDir, reedStateFileName)
	lockPath := path + ".lock"
	return state.WriteJSON(path, lockPath, s)
}

// toRenderStrands maps persisted strands to the render projection, setting
// Live from liveIDs[PaneID].
func toRenderStrands(strands []Strand, liveIDs map[string]bool) []render.Strand {
	out := make([]render.Strand, len(strands))
	for i, s := range strands {
		out[i] = render.Strand{
			GUID:    s.GUID,
			Parent:  s.Parent,
			Display: s.Display,
			PaneID:  s.PaneID,
			Live:    liveIDs[s.PaneID],
		}
	}
	return out
}
