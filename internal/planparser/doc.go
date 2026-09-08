// Package planparser is the SOLE parser AND SOLE writer of the on-disk plan format written
// under `_lyx/plan/` (see contracts/specs/loom-plan-spec.md, the pinned spec this package
// implements). No other package may read or write `_lyx/plan/` directly — every consumer
// (the batcher, webster's master, and fork prompt rendering) goes through
// planparser.ParsePlan and the Plan/Card model it returns, and the package's three write
// paths -- SetApproved (the approval flag), RewriteRefs (ref substitution across the plan,
// rewrite.go), and AppendAmendment (the append-only amendment log, amendment.go) -- are the
// only places any `_lyx/plan/` byte is ever rewritten — so the on-disk grammar has exactly
// one reader and three named writers, and the rest of webster never re-derives it.
//
// # The Glyph Conversion Chokepoint Invariant
//
// This package is also bound by CONSTRAINTS.md's Glyph Conversion Chokepoint Invariant:
// glyph.Self (glyphref.go) is the only path->glyph call, Glyph.UnitPath is the only
// glyph->path call, and glyph.Parse plus Glyph.String are the only glyph grammar this
// package uses. No `strings.TrimSuffix(s, "#")`, no reading Glyph.Unit as a disk path, and
// no local regex over a glyph string anywhere in this package or its callers --
// cmd/lyx/constraintchokepoint_test.go guards this mechanically.
//
// # Path ownership
//
// planparser owns not only the plan format but where the plan directory lives.
// PlanDirName and PlanDirRel declare the worktree-relative token (`_lyx/plan`,
// forward-slash, a document token used for Card.SourcePath); PlanDir and
// PlanOverview declare the absolute form. The package never resolves cwd and never
// imports internal/lyxcwd — the caller supplies the anchor path, which in a lyx
// worktree is lyxcwd.Location.AnchorPath() and never WorktreePath().
//
// # Type model
//
// A parsed plan is a Plan: the overview's frontmatter (Format, Approved, Root), its
// task-framing paragraph (Framing), the flat ordered list of Cards, and the three
// optional plan-level body sections (SharedDecisions, RenameMechanic, Verify). A Card
// carries one or more type labels: each label's own occurrence on the card is a
// TargetGroup (TargetGroups), holding that label's own Refs and, for a Rename group,
// its own Pairs and malformed RenameRaw bullets. The flat Targets, Pairs, and
// RenameRaw fields are retained as the union across every one of the card's own
// TargetGroups, in body order, for downstream consumers that read the card-level
// shape rather than its per-label groups. Type (the first label seen) and
// TypeLabelCount are retained for exported-field compatibility only — neither is
// validation state, and a new check must key on TargetGroups, never on Type, or it
// silently reintroduces first-label-wins. A Card also carries its Card Index fields
// (Number, Slug, Summary), its own file's Title, its Uses list (refs read or depended
// on, not targeted), its Intent prose (what, and why) and inline ImpactSummary (a
// hard-capped one-line blast-radius conclusion, with ImpactSummaryTrailing capturing
// any lines that follow it — a multiline ImpactSummary is itself a defect the
// validator reports), its per-label HasX presence bits (HasType, HasUses, HasIntent,
// HasImpactSummary, HasVerify), its RetiredLabels (one entry per format-3 label the
// card body still carried), and the optional Commit and Verify fields.
//
// Only path-shaped Targets/Uses/Pairs entries are normalized (see below), and
// only an extension-carrying path-shaped entry is further canonicalized into
// its glyph string at parse time; a glyph-, handle-, or symbol-shaped entry is
// stored verbatim. Classification is by shape alone, via this package's own
// pure classifier (classify.go's classifyRef/isPathRef/isGlyphRef/isHandleRef)
// — never `go doc`, never a process spawn, so the package stays a tier1-pure
// leaf per the Test Tier Purity Invariant. glyphref.go holds the sole
// path->glyph call (glyph.Self) and the sole glyph->path call
// (Glyph.UnitPath, used only by validate.go's disk-existence checks), per the
// glyph-conversion-chokepoint Shared Decision.
//
// # The root:/// resolution rule
//
// The overview's optional `root:` frontmatter key names a worktree-relative directory
// every path-shaped card ref resolves against, unless the ref is `//`-prefixed, in
// which case it is always worktree-root-relative regardless of root:. ParsePlan
// applies this resolution exactly once, at parse time (see normalizeCardPath in
// normalize.go) — a malformed result (a still-absolute path, a `..` escape) is left
// in place rather than rejected here; catching it is the validator's job.
//
// # Field presence and omission
//
// Format 4 omits a field with no content rather than writing a `none` sentinel: a
// card simply does not carry a label it has nothing to say under. ParsePlan records a
// HasX presence bit per recognized label and preserves the nil-versus-empty
// distinction on the label's own slice-typed field, so a label written with no
// bullets under it (non-nil empty slice, HasX true) becomes a card-field-empty
// finding rather than a silent nil, and an omitted required label (nil slice, HasX
// false) becomes a card-missing-field finding. These are mechanically different card
// defects, and only the validator (not this package) turns either into an enumerable
// finding.
//
// The eight format-3 labels (`**What:**`, `**Context:**`, `**Edits:**`,
// `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Depends-on:**`, and the lowercase
// `**verify:**`) stay recognized and route into RetiredLabels, each producing a
// card-retired-label finding — a label removed from the recognized set would instead
// be silently swallowed into Intent:'s prose, the exact silent misparse this format's
// fail-loud discipline exists to prevent.
//
// # Validation lives in validate.go
//
// This package's parse step is lenient at the card level: a malformed bullet or an
// absent field is recorded into the model (via the HasX bits, RenameRaw, or a nil
// slice) rather than failing the parse, so Validate can enumerate every defect in one
// pass instead of stopping at the first one. ParsePlan fails loud only on
// document-structure errors — a missing or undecodable overview file, an unparseable
// Card Index line, a missing card file, an unparseable card heading, or an inline
// value where a field admits only a bullet list. The plan format's 27 validation
// checks (card type presence, card-custom-not-alone, path malformation, the bare
// package-qualified symbol and directory-target hard rules, the Rename pair
// grammar, the plan: handle consistency checks, on-disk existence, and so on)
// are implemented by Validate in validate.go, not by ParsePlan itself.
//
// # The language: key and the glyph alphabet
//
// The overview's optional `language:` frontmatter key names the alphabet a
// plan's Targets/Uses/Pairs symbol-shaped entries are spelled in: "go" (the
// default when the key is absent) enables glyph.Go's alphabet, and "none"
// opts a plan out of it entirely, keeping every pre-glyph symbol-shaped
// behavior unchanged. Under "go", ParsePlan canonicalizes every
// extension-carrying path-shaped entry into its glyph string immediately
// after root:/// resolution, and Plan.SurfaceRefs records EVERY one of a
// canonicalized entry's pre-canonicalization surface lexemes, keyed by the
// owning card's identity and the resulting canonical string.
package planparser
