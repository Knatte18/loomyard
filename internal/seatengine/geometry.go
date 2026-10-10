// geometry.go declares Geometry, the struct the seat engine is told its coordinates through.
// It declares the type only: no constructor, no validator and no default.

package seatengine

// Geometry is the set of paths and names the seat engine is told, once, and never derives itself.
// Every field is an absolute path or a plain name a caller supplies; the engine validates none of them.
type Geometry struct {
	// WorktreeRoot is the root the seats' shuttle runs resolve relative paths against.
	WorktreeRoot string
	// AnchorPath is the base the seats' run directories join onto.
	AnchorPath string
	// StencilsDir is the directory the seats' stencils and their include blocks are read from.
	StencilsDir string
	// ParentName is the told parent agent name the seat prompts render their parent directive from.
	// An empty name renders the directive's no-parent variant.
	ParentName string
	// Shortname and Slug are the hub shortname and the task slug the seats' agent names are formed from.
	// An empty Slug gives the two-segment name form.
	Shortname string
	Slug      string
}
