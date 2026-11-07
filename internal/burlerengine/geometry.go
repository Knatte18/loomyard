// geometry.go declares Geometry, the set of paths burler is told its coordinates through.
// It declares the type only; it adds no constructor, no validator, and no default.

package burlerengine

// Geometry is the set of paths burler is told once, at construction, and never derives itself.
// No field is validated, so the caller must populate every field with a usable absolute path.
type Geometry struct {
	// WorktreeRoot is the root that relative paths in review and fix profiles resolve against.
	// In hub mode it is the anchor path; in standalone mode it is the reviewed target directory.
	WorktreeRoot string
	// AnchorPath is the base the per-round .lym/burler instruction directory joins onto.
	AnchorPath string
	// RepoRoot is the repository root, the directory holding PATTERN.md and go.mod.
	// It is set apart from the anchor path.
	// The anchor path is not the repository root when a hub is anchored to a subpath.
	// An empty RepoRoot yields no PATTERN directive.
	RepoRoot string
	// ParentName is the parent agent name that the round prompt renders its parent directive from.
	// An empty name renders the directive's no-parent variant.
	ParentName string
	// Shortname and Slug are the hub shortname and the task slug the round's agent names are formed from.
	// An empty Slug gives the two-segment name form.
	Shortname string
	Slug      string
	// PatternRecordPath is the decision record whose declared PATTERN the review round's directive renders.
	// An empty path renders none.
	PatternRecordPath string
	// SpecsDir is the deployed-specs directory a fanned half's prompt points at for the lens report format.
	SpecsDir string
	// DefaultEngine names the engine a half with no model choice runs on, and its files are rendered for that engine.
	DefaultEngine string
}
