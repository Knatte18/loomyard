// plan.go defines planparser's public struct model: Plan (the whole parsed
// `_lyx/plan/` directory) and Card (one flat, plan-format-v3 card), plus MovePair,
// the normalized-path pair a card's Moves: field carries. No parsing logic lives
// here — see parse.go, normalize.go, and sections.go for how these types are filled.

package planparser

// Plan is the in-memory form of a fully parsed plan-format v3 plan directory: the
// overview's frontmatter and task framing, plus every card the Card Index lists,
// each parsed from its own NN-<card-slug>.md file (see Card). ParsePlan performs no
// validation itself — a Plan may carry an unrecognized Format, an unapproved plan, or
// cards with defects; Validate (validate.go) is what turns those into findings.
type Plan struct {
	// Dir is the plan directory ParsePlan was given: hubgeometry.PlanDir(...) in
	// production, a t.TempDir() fixture in tests. ParsePlan never constructs this
	// path itself — the caller resolves it, per the Hub Geometry Invariant.
	Dir string

	// Format is the plan-format version the plan is written against, taken as-is
	// from 00-overview.md's frontmatter. Zero when the frontmatter carries no
	// format: key at all — ParsePlan does not reject an unrecognized or absent
	// value; that is Validate's format-unrecognized check.
	Format int

	// Approved mirrors the overview frontmatter's approved: field. False when the
	// key is absent. ParsePlan does not refuse an unapproved (or unmarked) plan
	// itself — that is Validate's plan-unapproved check.
	Approved bool

	// Root mirrors the overview frontmatter's optional root: field: the
	// worktree-relative directory every card file-op path resolves against unless
	// `//`-prefixed. Empty when the key is absent, in which case card paths are
	// stored exactly as written (still worktree-relative). See normalizeCardPath
	// for the three-case resolution rule this field feeds.
	Root string

	// Framing is the short task-framing paragraph(s) between the overview's title
	// heading and its "## Card Index" heading.
	Framing string

	// Cards is every card the Card Index lists, in index order, each parsed from
	// its own NN-<card-slug>.md file.
	Cards []Card

	// SharedDecisions is the raw body text of the overview's optional
	// "## Shared Decisions" section, verbatim and trimmed. Empty when the section
	// is absent — the section is plan-wide and optional, unlike a card's own
	// fields, which are always present (possibly as "none").
	SharedDecisions string

	// RenameMechanic is the raw body text of the overview's optional
	// "## Rename mechanic" section, verbatim and trimmed. Empty when the section
	// is absent. Validate's move-mechanic-missing check flags a plan with at
	// least one card's Moves: pair but an empty RenameMechanic.
	RenameMechanic string

	// Verify is the single command line the overview's optional "## verify:"
	// section carries: the plan-level integration verify, run once at the end of
	// the whole plan (distinct from any per-card Verify). Empty when the section
	// is absent.
	Verify string
}

// Card is one flat, plan-format-v3 card: the Card Index entry's fields (Number,
// Slug, Intent) plus everything parsed from the card's own NN-<card-slug>.md file.
// Card-level defects — a missing field, a malformed Moves: bullet — are never parse
// errors: plan-format-v3.md's lenient-card-parse discipline records them here (via
// the HasX bits, MovesRaw, or a nil slice) so Validate can turn them into enumerable
// findings instead of a single fail-loud error.
type Card struct {
	// Number is the card's own flat number (1..N across the whole plan), taken
	// from the Card Index entry. The card file's own "# Card N — <name>" heading
	// carries this same number and is cross-checked against it while parsing the
	// card body (see parseCardBody); a mismatch is a card-numbering defect for
	// Validate, not a parse failure.
	Number int

	// Slug is the card's <card-slug> segment, taken from the Card Index entry —
	// also the segment that names the card's own file (NN-<Slug>.md).
	Slug string

	// Title is the card file's own "# Card N — <name>" heading's trailing name
	// text.
	Title string

	// Intent is the Card Index entry's one-line summary. It has exactly one
	// source: the index. The card file's own "**What:**" prose is for the
	// implementer and is never stored here (HasWhat records only its presence).
	Intent string

	// HasWhat reports whether the card carried a "**What:**" label at all. The
	// prose itself is never stored — Intent is the machine-read summary.
	HasWhat bool

	// ContextFiles, EditsFiles, CreatesFiles, and DeletesFiles are the card's four
	// non-Moves typed file-op fields, each a normalized worktree-relative path
	// list (see normalizeCardPath). nil means the field's bold label was never
	// seen at all (see HasContext etc.); a present field whose label line carries
	// the literal "none" parses to an empty non-nil slice — the two are
	// deliberately distinguishable.
	ContextFiles, EditsFiles, CreatesFiles, DeletesFiles []string

	// Moves is every well-formed "- `old` -> `new`" sub-bullet under the card's
	// "**Moves:**" field, both sides normalized. Present-but-none parses to an
	// empty non-nil slice, matching the other four fields.
	Moves []MovePair

	// MovesRaw is every "**Moves:**" sub-bullet that failed the two-path pair
	// grammar, retained verbatim (never normalized) so Validate's move-format
	// check can name the exact offending line.
	MovesRaw []string

	// HasContext, HasEdits, HasCreates, HasDeletes, HasMoves, and HasDependsOn
	// report whether the card's body carried that field's bold label at all —
	// distinct from the field being present-but-none. Validate's
	// card-missing-field check flags a card missing any of the six.
	HasContext, HasEdits, HasCreates, HasDeletes, HasMoves, HasDependsOn bool

	// DependsOn is the card's "**Depends-on:**" field: the plain card numbers it
	// names, or an empty non-nil slice for a present "none". nil when the label
	// was never seen at all (HasDependsOn false). Validate's depends-on-order
	// check flags an entry naming the card's own position, a later card, or an
	// id referencing no existing card.
	DependsOn []int

	// Commit is the card's optional "**Commit:**" field value, with its
	// surrounding backticks stripped. Empty when the field is absent; the
	// implementer then derives the commit subject from the "N: <name>"
	// convention itself. Validate's commit-subject-mismatch check flags a
	// present value that does not start with the card's own "N: " prefix.
	Commit string

	// Verify is the card's optional "**verify:**" field's value, taken verbatim.
	// Empty when absent. Distinct from Plan.Verify, the plan-level integration
	// verify.
	Verify string
}

// MovePair is one well-formed "Moves:" sub-bullet: a card declaring that Old is
// renamed to New, both already normalized worktree-relative paths (per
// normalizeCardPath). The repo's own rename convention — "git mv" first, then only
// surgical edits — is what a card's Moves: entry declares; see
// docs/reference/plan-format-v3.md's Rename mechanic text.
type MovePair struct {
	Old, New string
}
