// state.go defines the persisted strand record and ReedState container, plus
// the .lyx/reed.json load/save wrappers and the mapper that projects the
// persisted record down to render.Strand. This is the module's dumb-carrier
// contract in concrete form: Strand stores every field a caller writes
// (cmd, resumeCmd, sessionId, worktree, name) and reedengine itself reads
// none of them semantically — only Display feeds the layout decision, via
// toRenderStrands.

package reedengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/state"
)

// Strand is the persisted record for one tmux pane reedengine owns,
// reusing render.Display for the display vocabulary.
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

// ReedState is the persisted record for one hub's tmux server: the socket
// name, the session, stripped env keys, and every strand as a flat list.
type ReedState struct {
	Socket      string   `json:"socket"`
	Session     string   `json:"session"`
	StrippedEnv []string `json:"strippedEnv"`
	Strands     []Strand `json:"strands"`
	// HeaderPaneID is the tmux pane id of the always-present header pane —
	// deliberately outside Strands, since the header is a first-class but
	// separate construct and never itself a strand (Shared Decision
	// header-is-not-a-strand): it is excluded from every strand accounting,
	// adoption, reconcile, and layout path a Strand would otherwise be
	// subject to. Empty means the header pane has not yet been created (a
	// fresh worktree, or a server rebirth that cleared every binding) and
	// must be (re)created at the next up/resume boot.
	HeaderPaneID string `json:"headerPaneId,omitempty"`
}

// reedStateFileName is the reed.json file name inside .lyx directory.
const reedStateFileName = "reed.json"

// LoadState reads the ReedState persisted at dotLyxDir/reed.json.
// Returns (nil, nil) if absent, (nil, err) on corrupt or read errors.
func LoadState(dotLyxDir string) (*ReedState, error) {
	path := filepath.Join(dotLyxDir, reedStateFileName)
	lockPath := path + ".lock"

	v, found, err := state.ReadJSON[ReedState](path, lockPath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &v, nil
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
