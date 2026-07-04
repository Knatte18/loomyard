// types.go defines the closed display vocabulary render exposes to its
// caller: the Anchor kinds a strand may declare, the per-strand Display
// settings, and the plain Strand/Box/Params value types. This file carries
// no logic — it is the vocabulary the policy layer (policy.go, height.go,
// focus.go) and the mechanics layer (layout.go, checksum.go) are built from.

// Package render owns the closed display vocabulary and the deterministic
// Rules(strands, box, params) -> (layout, focus) function that turns a set
// of strands into a tmux window_layout string. It is a pure leaf: no I/O, no
// psmux, no engine import.
//
// The package is deliberately split into two layers that must never merge.
// Layout policy (policy.go, height.go, focus.go) decides which strand lands
// where and how tall — this is where the closed Anchor vocabulary is
// interpreted and where the height math lives. Layout mechanics
// (layout.go, checksum.go) turns an already-decided list of (pane, height)
// placements into the tmux window_layout string and its checksum, with no
// opinion about placement or sizing. Adding a new anchor is a localized
// change to the policy layer (a policy.go case plus its test); it must never
// require touching the mechanics layer.
package render

// Anchor is the closed set of placement strategies a strand's Display may
// declare. Render recognizes exactly these four values.
type Anchor string

const (
	// AnchorTop reserves a fixed-height band for a strand at the top of the
	// window, above the below-parent stack. The band's GEOMETRY is always
	// honored; its POSITION is only physically top when the strand's pane
	// precedes the stack panes in the window's creation order — psmux
	// applies layout cells positionally and cannot reorder panes (see
	// Rules' paneOrder contract), so a top strand added after stack strands
	// keeps its compact band height at its actual position.
	AnchorTop Anchor = "top"
	// AnchorBelowParent places a strand in the vertically stacked region
	// below the top bands, ordered by parent-chain depth.
	AnchorBelowParent Anchor = "below-parent"
	// AnchorOwnWindow is declared in the vocabulary but deferred in v1:
	// Rules rejects any strand carrying it with an error. It is reserved
	// for a future release that gives a strand its own psmux window
	// instead of sharing a pane in the stacked layout.
	AnchorOwnWindow Anchor = "own-window"
	// AnchorHidden excludes a strand from the layout entirely. A hidden
	// strand never owns a pane and is dropped before placement.
	AnchorHidden Anchor = "hidden"
)

// Display carries the per-strand layout settings render acts on. Its JSON
// tags are load-bearing: the engine persists Display verbatim inside the
// mux.json strand record, so these lowerCamel keys are the on-disk contract
// callers (shuttle) will read and write.
type Display struct {
	// Anchor selects which placement strategy governs this strand.
	Anchor Anchor `json:"anchor"`
	// Focus marks this strand as the one that should receive psmux input
	// focus. At most one strand is expected to set Focus; if several do,
	// render breaks the tie by picking the bottom-most.
	Focus bool `json:"focus"`
	// ShrinkWhenWaitingOnChild, when true, lets this strand collapse to a
	// compact strip once one of its descendants is present in the layout
	// — the ancestor is blocked waiting on that child, so it need not
	// stay full height. When false the strand stays a co-equal full pane
	// even while a descendant is present.
	ShrinkWhenWaitingOnChild bool `json:"shrinkWhenWaitingOnChild"`
}

// Strand is the layout-facing projection of an engine strand: only the
// fields Rules needs to place a pane. The engine's opaque cmd, resumeCmd,
// sessionId, and worktree fields are not part of this type — the engine
// maps its full persisted record down to a Strand before calling Rules.
type Strand struct {
	// GUID is the strand's durable identity.
	GUID string
	// Parent is the parent strand's GUID, or "" for a root strand.
	Parent string
	// Display carries this strand's placement, focus, and shrink settings.
	Display Display
	// PaneID is the psmux pane id (e.g. "%5") this strand currently owns.
	// A strand with an empty PaneID owns no pane and is excluded from the
	// layout.
	PaneID string
	// Live reports whether the strand's pane is currently alive. A
	// non-live strand is excluded from the layout.
	Live bool
}

// Box is a rectangular region of the window, in tmux's row/column
// coordinate space.
type Box struct {
	X, Y, W, H int
}

// Params carries the tunable height-policy knobs the engine loads from
// mux.yaml, keeping render itself config-agnostic.
type Params struct {
	// TopBandRows is the fixed height reserved for each AnchorTop strand.
	TopBandRows int
	// CollapsedStripRows is the height a shrink:true ancestor collapses to
	// once it has a descendant present in the layout.
	CollapsedStripRows int
	// MinFullRows is the floor height the clamp rule tries to preserve for
	// a full (non-collapsed) pane when the window is too short to satisfy
	// every strand's natural height.
	MinFullRows int
}
