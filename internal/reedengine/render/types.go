// types.go defines the closed display vocabulary render exposes to its caller: the Anchor kinds a
// strand may declare and the per-strand Display settings (anchor, focus, viewer and window group).
// This file carries no logic.

// Package render owns the closed display vocabulary a spawner names when it places a strand.
// It is a pure leaf: no I/O, no tmux, no engine import.
//
// The vocabulary is top, below-parent, own-window, overview and hidden.
// ValidateDisplay is the one check a display passes before reed stores it.
package render

// Anchor is the closed set of placement strategies a strand's Display may declare.
// Render recognizes exactly these five values.
type Anchor string

const (
	// AnchorTop is a pane-era placement that reed no longer lays out.
	AnchorTop Anchor = "top"
	// AnchorBelowParent is a pane-era placement that reed no longer lays out.
	AnchorBelowParent Anchor = "below-parent"
	// AnchorOwnWindow gives a strand a tmux window of its own, or its group's shared window, outside window 0.
	// Window 0's layout never places it.
	AnchorOwnWindow Anchor = "own-window"
	// AnchorOverview gives the session's one overview strand a window of its own that is always the session's first window.
	// Window 0's layout never places it.
	AnchorOverview Anchor = "overview"
	// AnchorHidden excludes a strand from the layout entirely.
	// A hidden strand never owns a pane.
	AnchorHidden Anchor = "hidden"
)

// Display carries the per-strand settings render validates.
// Its JSON tags are load-bearing: the engine persists Display verbatim inside the reed.json strand
// record, so these lowerCamel keys are the on-disk contract callers (shuttle) will read and write.
type Display struct {
	// Anchor selects which placement strategy governs this strand.
	Anchor Anchor `json:"anchor"`
	// Focus marks this strand as the one that should receive tmux input focus.
	Focus bool `json:"focus"`
	// KeepViewer marks an own-window strand whose new window never takes the viewer, where an unset one may follow the run onto it.
	// Focus wins when both are set.
	KeepViewer bool `json:"keepViewer,omitempty"`
	// Window names the window group the spawner puts the strand in; empty for a strand that wants a window to itself.
	// It is read only for an own-window strand, and reed decides from it and WindowSize whether the group shares one window.
	Window string `json:"window,omitempty"`
	// WindowSize is the number of strands the spawner puts in the Window group; 0 reads as 1.
	// It is read only for an own-window strand.
	WindowSize int `json:"windowSize,omitempty"`
}
