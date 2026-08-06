// plan.go defines planparser's public struct model: Plan (the whole parsed `_lyx/plan/` directory)
// and Card (one flat, plan-format-v3 card), plus MovePair, the normalized-path pair a card's Moves:
// field carries.
// No parsing logic lives here — see parse.go, normalize.go, and sections.go for how these types are
// filled.

package planparser

// Plan is the in-memory form of a fully parsed plan-format v3 plan directory.
type Plan struct {
	// Dir is the plan directory ParsePlan was given.
	Dir string

	// Format is the plan-format version the plan is written against from the frontmatter.
	Format int

	// Approved mirrors the overview frontmatter's approved: field.
	Approved bool

	// Root mirrors the overview frontmatter's optional root: field.
	Root string

	// Framing is the task-framing paragraph(s) between the overview's title heading and its "## Card Index" heading.
	Framing string

	// Cards is every card the Card Index lists, in index order.
	Cards []Card

	// SharedDecisions is the raw body text of the overview's optional "## Shared Decisions" section.
	SharedDecisions string

	// RenameMechanic is the raw body text of the overview's optional "## Rename mechanic" section.
	RenameMechanic string

	// Verify is the single command line the overview's optional "## verify:" section carries.
	Verify string
}

// Card is one flat, plan-format-v3 card: the Card Index entry's fields plus everything parsed from
// the card's own file.
type Card struct {
	// Number is the card's own flat number (1..N), taken from the Card Index entry.
	Number int

	// Slug is the card's segment, taken from the Card Index entry.
	Slug string

	// Title is the card file's "# Card N — <name>" heading's trailing name text.
	Title string

	// Intent is the Card Index entry's one-line summary.
	Intent string

	// What is the card file's "**What:**" prose — the concrete instruction the implementer works from.
	What string

	// SourcePath is the card's bare worktree-relative source-identity token `_lyx/plan/NN-<slug>.md`.
	SourcePath string

	// HasWhat reports whether the card carried a "**What:**" label at all.
	HasWhat bool

	// ContextFiles, EditsFiles, CreatesFiles, and DeletesFiles are the card's four non-Moves typed file-op fields.
	ContextFiles, EditsFiles, CreatesFiles, DeletesFiles []string

	// Moves is every well-formed "- `old` -> `new`" sub-bullet under the card's "**Moves:**" field.
	Moves []MovePair

	// MovesRaw is every "**Moves:**" sub-bullet that failed the two-path pair grammar.
	MovesRaw []string

	// HasContext, HasEdits, HasCreates, HasDeletes, HasMoves, and HasDependsOn report whether the card's body carried that field's label.
	HasContext, HasEdits, HasCreates, HasDeletes, HasMoves, HasDependsOn bool

	// DependsOn is the card's "**Depends-on:**" field: the plain card numbers it names.
	DependsOn []int

	// Commit is the card's optional "**Commit:**" field value with surrounding backticks stripped.
	Commit string

	// Verify is the card's optional "**verify:**" field's value, taken verbatim.
	Verify string
}

// MovePair is one well-formed "Moves:" sub-bullet: a card declaring that Old is renamed to New.
type MovePair struct {
	Old, New string
}
