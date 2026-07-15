// state.go defines the persisted strand record and MuxState container, plus
// the .lyx/mux.json load/save wrappers and the mapper that projects the
// persisted record down to render.Strand. This is the module's dumb-carrier
// contract in concrete form: Strand stores every field a caller writes
// (cmd, resumeCmd, sessionId, worktree, name) and muxengine itself reads
// none of them semantically — only Display feeds the layout decision, via
// toRenderStrands.

package muxengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
	"github.com/Knatte18/loomyard/internal/state"
)

// Strand is the persisted record for one tmux pane muxengine owns. It
// reuses render.Display/render.Anchor for the display vocabulary (a single
// source of that vocabulary) and adds the opaque carrier fields muxengine
// stores but never interprets: Cmd/ResumeCmd (opaque launch/resume command
// strings), SessionID (opaque metadata, never identity — GUID is), Worktree
// (the owning worktree root, so a strand self-describes without external
// lookup), and Parent (the parent strand's GUID, or "" for a root strand).
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

// MuxState is the persisted record for one hub's tmux server: the socket
// name (which doubles as the server name — one identity, stored once), the
// session this state file belongs to, the env keys stripped at server-spawn
// time (stamped when this worktree's op booted the server, for diagnosis),
// and every strand across the session as a flat, GUID-keyed list. The flat
// list is the v2 union seam — each strand self-describes its own Worktree
// rather than being nested under a per-worktree map, so a strand can be
// looked up or iterated without first knowing which worktree owns it.
type MuxState struct {
	Socket      string   `json:"socket"`
	Session     string   `json:"session"`
	StrippedEnv []string `json:"strippedEnv"`
	Strands     []Strand `json:"strands"`
}

// muxStateFileName is the mux.json file name inside a Layout's ephemeral
// .lyx directory (hubgeometry.(*Layout).DotLyxDir()). It is a package-local
// constant rather than a hardcoded literal at each call site.
const muxStateFileName = "mux.json"

// LoadState reads the MuxState persisted at dotLyxDir/mux.json under a
// shared read lock. The caller supplies dotLyxDir as layout.DotLyxDir() so
// this package never hardcodes the ".lyx" literal itself (the Hub Geometry
// Invariant). Returns (nil, nil) if the file is absent — a fresh worktree
// with no mux state yet is not an error. Returns (nil, err) if the file is
// corrupt/unparseable or on other read errors.
func LoadState(dotLyxDir string) (*MuxState, error) {
	path := filepath.Join(dotLyxDir, muxStateFileName)
	lockPath := path + ".lock"

	v, found, err := state.ReadJSON[MuxState](path, lockPath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &v, nil
}

// SaveState writes s to dotLyxDir/mux.json atomically under an exclusive
// write lock, creating dotLyxDir if needed. The caller supplies dotLyxDir as
// layout.DotLyxDir(), matching LoadState.
func SaveState(dotLyxDir string, s *MuxState) error {
	path := filepath.Join(dotLyxDir, muxStateFileName)
	lockPath := path + ".lock"
	return state.WriteJSON(path, lockPath, s)
}

// toRenderStrands maps the persisted strands down to the render-facing
// projection Rules needs: GUID, Parent, Display, and PaneID carry straight
// through, and Live is set from liveIDs[PaneID]. liveIDs is the set of pane
// ids currently present in the tmux window per list-panes — a
// dead-but-remain-on-exit pane still counts as present until something
// explicitly kills it, so Live means "this strand owns a present window
// pane", not "this strand's command is still running". toRenderStrands maps
// every strand unconditionally; it is render.partitionByAnchor's job, not
// this mapper's, to drop not-live or empty-PaneID strands from the layout.
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
