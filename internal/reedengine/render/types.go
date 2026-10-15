// types.go defines the closed display vocabulary render exposes to its caller: the Anchor kinds a
// strand may declare, the SizeKind height rules, the per-strand Display settings (anchor, size and focus),
// and the plain Strand/Box/Params value types.
// This file carries no logic — it is the vocabulary the policy layer (policy.go, height.go,
// focus.go) and the mechanics layer (layout.go, checksum.go) are built from.

// Package render owns the closed display vocabulary and the deterministic Rules(strands, box,
// params) -> (layout, focus) function that turns a set of strands into a tmux window_layout string.
// It is a pure leaf: no I/O, no tmux, no engine import.
//
// The package is deliberately split into two layers that must never merge.
// Layout policy (policy.go, height.go, focus.go) decides which strand lands where and how tall —
// this is where the closed Anchor vocabulary is interpreted and where the height math lives.
// The vocabulary is top, below-parent, own-window, overview and hidden; own-window, overview and hidden strands are never placed in the window this package lays out.
// Layout mechanics (layout.go, checksum.go) turns an already-decided list of (pane, height)
// placements into the tmux window_layout string and its checksum, with no opinion about placement
// or sizing.
// Adding a new anchor is a localized change to the policy layer (a policy.go case plus its test);
// it must never require touching the mechanics layer.
package render

// Anchor is the closed set of placement strategies a strand's Display may declare.
// Render recognizes exactly these five values.
type Anchor string

const (
	// AnchorTop places a strand in the stacked region above every below-parent strand, ordered by insertion among the top strands.
	// Only a content-sized strand may be top-anchored.
	AnchorTop Anchor = "top"
	// AnchorBelowParent places a strand in the vertically stacked region, ordered by insertion.
	AnchorBelowParent Anchor = "below-parent"
	// AnchorOwnWindow gives a strand a tmux window of its own, or its group's shared window, outside window 0.
	// Window 0's layout never places it.
	AnchorOwnWindow Anchor = "own-window"
	// AnchorOverview gives the session's one overview strand a window of its own that is always the session's first window.
	// Window 0's layout never places it.
	AnchorOverview Anchor = "overview"
	// AnchorHidden excludes a strand from the layout entirely.
	// A hidden strand never owns a pane and is dropped before placement.
	AnchorHidden Anchor = "hidden"
)

// SizeKind is the closed set of height rules a strand's Size may declare.
type SizeKind string

const (
	// SizePositional is the zero kind: the strand's height follows its position in the stack.
	// The bottom-most positional strand expands and every other one collapses.
	SizePositional SizeKind = ""
	// SizeContent sizes a strand to its reported content line count, within a share of the free height and above a minimum.
	SizeContent SizeKind = "content"
	// SizeSharing makes a strand share the expanding rows evenly with every other live sharing strand.
	// While one is live, every positional strand collapses.
	SizeSharing SizeKind = "sharing"
)

// Size is a strand's size declaration.
// The share is an integer fraction so floor(share × free height) is exact: one third is 1/3, never 0.33.
type Size struct {
	// Kind selects the height rule; the fields below are read for SizeContent only.
	Kind SizeKind `json:"kind,omitempty"`
	// ShareNum and ShareDen are the numerator and denominator of the strand's preferred share of the free height.
	ShareNum int `json:"shareNum,omitempty"`
	ShareDen int `json:"shareDen,omitempty"`
	// MinRows is the fewest rows the strand takes, and its height while it has reported no line count.
	MinRows int `json:"minRows,omitempty"`
}

// Display carries the per-strand layout settings render acts on.
// Its JSON tags are load-bearing: the engine persists Display verbatim inside the reed.json strand
// record, so these lowerCamel keys are the on-disk contract callers (shuttle) will read and write.
type Display struct {
	// Anchor selects which placement strategy governs this strand.
	Anchor Anchor `json:"anchor"`
	// Focus marks this strand as the one that should receive tmux input
	// focus. At most one strand is expected to set Focus; if several do,
	// render breaks the tie by picking the bottom-most.
	Focus bool `json:"focus"`
	// KeepViewer marks an own-window strand whose new window never takes the viewer, where an unset one may follow the run onto it.
	// Focus wins when both are set.
	KeepViewer bool `json:"keepViewer,omitempty"`
	// Size declares the strand's height rule.
	// A strand that declares nothing is SizePositional and marshals no size key.
	Size Size `json:"size,omitzero"`
	// Window names the window group the spawner puts the strand in; empty for a strand that wants a window to itself.
	// It is read only for an own-window strand, and reed decides from it and WindowSize whether the group shares one window.
	Window string `json:"window,omitempty"`
	// WindowSize is the number of strands the spawner puts in the Window group; 0 reads as 1.
	// It is read only for an own-window strand.
	WindowSize int `json:"windowSize,omitempty"`
}

// Strand is the layout-facing projection of an engine strand: only the fields Rules needs to place
// a pane.
// The engine's opaque cmd, resumeCmd, sessionId, and worktree fields are not part of this type —
// the engine maps its full persisted record down to a Strand before calling Rules.
type Strand struct {
	// GUID is the strand's durable identity.
	GUID string
	// Parent is the parent strand's GUID, or "" for a root strand.
	Parent string
	// Display carries this strand's placement and focus settings.
	Display Display
	// PaneID is the tmux pane id (e.g. "%5") this strand currently owns.
	// A strand with an empty PaneID owns no pane and is excluded from the
	// layout.
	PaneID string
	// Live reports whether the strand's pane is currently alive. A
	// non-live strand is excluded from the layout.
	Live bool
	// ContentRows is the strand's reported visible content line count;
	// 0 means not reported yet, read as the strand's Size.MinRows.
	ContentRows int
}

// Box is a rectangular region of the window, in tmux's row/column coordinate space.
type Box struct {
	X, Y, W, H int
}

// Selvage carries the always-on operator console pane's placement: PaneID names the tmux pane Rules
// renders as a fixed-height bottom band below the below-parent stack,
// and HeightRows is its requested row count (before clampBandHeight's floor-preserving
// adjustment).
// A zero-value Selvage (empty PaneID) means "no band" — Rules then lays out exactly as it did
// before the band existed, so every pre-band caller is unaffected.
// The Selvage is never a Strand: it is injected at this Params seam instead of being modelled in the
// strand slice (Shared Decision header-is-not-a-strand).
type Selvage struct {
	PaneID     string
	HeightRows int
}

// Params carries the tunable height-policy knobs the engine loads from reed.yaml, keeping render
// itself config-agnostic.
type Params struct {
	// CollapsedRows is the height every below-parent strand except the
	// bottom-most takes; the bottom-most takes every remaining row.
	CollapsedRows int
	// MinFullRows is the floor clampBandHeight preserves for the
	// strand-stack region when the Selvage band is present and the window
	// is too short to fit both.
	MinFullRows int
	// Selvage carries the always-on operator console pane's placement, if
	// any. A zero-value Selvage (empty PaneID) means no band is rendered.
	Selvage Selvage
	// TitleRow reports that the window draws a pane-border title row at window row 0,
	// so the first cell of the stack adds one row to show its content lines.
	TitleRow bool
}
