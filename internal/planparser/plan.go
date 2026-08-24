// plan.go defines planparser's public struct model: Plan (the whole parsed `_lyx/plan/` directory)
// and Card (one flat, format-4 plan-format card), plus CardType (a card's own type label) and
// MovePair (the normalized-path pair a Rename card's Pairs field carries).
// No parsing logic lives here — see parse.go, normalize.go, and sections.go for how these types are
// filled.

package planparser

// CardType is the type label a card's own body declares — the key of the card's target list
// (`**Create:**`, `**Edit:**`, and so on) per manifest/designs/plan-card-format.md.
type CardType string

// The recognized format-4 card type labels.
const (
	// CardTypeUnknown is the zero value: no recognized type label was seen.
	CardTypeUnknown CardType = ""
	// CardTypeCreate marks a card whose targets are new symbol(s) or file(s).
	CardTypeCreate CardType = "Create"
	// CardTypeEdit marks a card whose targets are existing symbol(s) being changed.
	CardTypeEdit CardType = "Edit"
	// CardTypeDelete marks a card whose targets are existing symbol(s) or whole file(s) being removed.
	CardTypeDelete CardType = "Delete"
	// CardTypeRename marks a card whose targets are old -> new symbol pairs.
	CardTypeRename CardType = "Rename"
	// CardTypeMove marks a card relocating a symbol or file to a new file.
	CardTypeMove CardType = "Move"
	// CardTypeProsa marks a card touching file(s) with no single-symbol owner (docs, design notes).
	CardTypeProsa CardType = "Prosa"
	// CardTypeCustom marks a card that is an explicit escape hatch from the other six types.
	CardTypeCustom CardType = "Custom"
)

// Plan is the in-memory form of a fully parsed plan-format plan directory.
type Plan struct {
	// Dir is the plan directory ParsePlan was given.
	Dir string

	// Format is the plan-format version the plan is written against from the frontmatter.
	// The only version Validate currently recognizes is 4.
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

// Card is one flat, format-4 plan-format card: the Card Index entry's fields plus everything
// parsed from the card's own file — its type label and target list, its Uses list, its Intent
// prose and ImpactSummary, and its optional Commit/Verify fields.
type Card struct {
	// Number is the card's own flat number (1..N), taken from the Card Index entry.
	Number int

	// Slug is the card's segment, taken from the Card Index entry.
	Slug string

	// Title is the card file's "# Card N — <name>" heading's trailing name text.
	Title string

	// Summary is the Card Index entry's one-line summary.
	Summary string

	// SourcePath is the card's bare worktree-relative source-identity token `_lyx/plan/NN-<slug>.md`.
	SourcePath string

	// Type is the first recognized type label the card body carried, or CardTypeUnknown when none.
	Type CardType

	// TypeLabelCount is how many recognized type labels the card body carried, so a two-label card
	// (a card-type-missing defect) is expressible.
	TypeLabelCount int

	// HasType reports whether TypeLabelCount is greater than zero.
	HasType bool

	// Targets is the card's own flat target ref list — symbols and paths mixed, in body order.
	// For a Rename card, both endpoints of every Pairs entry are projected here too, Old then New.
	Targets []string

	// Pairs is every well-formed "- `old` -> `new`" sub-bullet under a Rename card's "**Rename:**"
	// field. Populated for a Rename card only.
	Pairs []MovePair

	// RenameRaw is every "**Rename:**" sub-bullet that failed the two-symbol pair grammar.
	RenameRaw []string

	// Uses is the card's "**Uses:**" field: refs the card reads or depends on without targeting.
	Uses []string

	// HasUses reports whether the card carried a "**Uses:**" label at all.
	HasUses bool

	// Intent is the card body's multi-line "**Intent:**" prose — what, and why.
	Intent string

	// HasIntent reports whether the card carried an "**Intent:**" label at all.
	HasIntent bool

	// ImpactSummary is the card body's inline one-line "**ImpactSummary:**" value.
	ImpactSummary string

	// HasImpactSummary reports whether the card carried an "**ImpactSummary:**" label at all.
	HasImpactSummary bool

	// ImpactSummaryTrailing is every non-label line following the "**ImpactSummary:**" label line,
	// captured rather than discarded so the impact-summary-multiline check has something to report.
	ImpactSummaryTrailing []string

	// RetiredLabels is one entry per format-3 label occurrence the card body carried, holding the
	// label's own literal text (e.g. "**Context:**").
	RetiredLabels []string

	// Commit is the card's optional "**Commit:**" field value with surrounding backticks stripped.
	Commit string

	// Verify is the card's optional "**Verify:**" field's value, taken verbatim.
	Verify string

	// HasVerify reports whether the card carried a "**Verify:**" label at all.
	HasVerify bool
}

// MovePair is one well-formed "old -> new" sub-bullet: a Rename card declaring that Old is
// renamed to New.
type MovePair struct {
	Old, New string
}
