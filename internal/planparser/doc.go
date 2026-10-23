// Package planparser is the SOLE parser AND SOLE writer of the on-disk plan format written
// under `_lyx/plan/` (see contracts/specs/loom-plan-spec.md, the pinned spec this package
// implements). No other package may read or write `_lyx/plan/` directly — every consumer
// (the batcher, webster's master, and fork prompt rendering) goes through
// planparser.ParsePlan and the Plan/Card model it returns.
// The package's write paths are the only places any `_lyx/plan/` byte is ever rewritten:
// SetApproved (the approval flag), RewriteRefs (ref substitution across the plan, rewrite.go), AppendAmendment (the append-only amendment log, amendment.go), InsertCard and RemoveCard (one card added or dropped with the cards after it renumbered, renumber.go), and RestoreRenumber (the undo of an interrupted renumber, restore.go).
// So the on-disk grammar has exactly one reader and named writers, and the rest of webster never re-derives it.
// Overlay (overlay.go) is a read path beside them and writes no `_lyx/plan/` byte:
// it lays proposed contents of the overview and the cards the Card Index lists over a plan directory, and a plan parsed through it reads, rewrites (Plan.RewriteRefs) and re-parses (Plan.Reparse) in memory.
//
// # The Glyph Conversion Chokepoint Invariant
//
// This package is also bound by PATTERN-glyph-conversion-chokepoint:
// glyph.Self (glyphref.go) is the only path->glyph call, Glyph.UnitPath is the only
// glyph->path call, and glyph.Parse plus Glyph.String are the only glyph grammar this
// package uses. No `strings.TrimSuffix(s, "#")`, no reading Glyph.Unit as a disk path, and
// no local regex over a glyph string anywhere in this package or its callers --
// cmd/lyx/constraintchokepoint_test.go guards this mechanically.
// goPackageDir (targetdirs.go) is the one sanctioned adjustment beside Glyph.UnitPath:
// it maps quarry's external-test pseudo-unit `<dir>_test` to its package directory.
// ParseRef (glyphref.go) is the one alphabet choice for a plan ref and for a `plan:` handle's body, so nothing outside this package passes a glyph.Language to a parse;
// PATTERN-glyph-conversion-chokepoint states the rule.
//
// # Path ownership
//
// planparser owns not only the plan format but where the plan directory lives.
// PlanDirName and PlanDirRel declare the worktree-relative token (`_lyx/plan`,
// forward-slash, a document token used for Card.SourcePath); PlanDir and
// PlanOverview declare the absolute form.
// PlanScratchDir declares the plan directory's never-tracked mirror, joined from the same segment as PlanDir, and RenumberJournalPath the journal under it.
// InsertCard and RemoveCard write that journal before they touch the plan directory and remove it when they finish, so a journal found on disk is an interrupted renumber.
// RestoreRenumber is the journal's only reader: it undoes that renumber from the journal, which discards any edit made to the journaled files since.
// All file operations of a renumber go through RenumberIO, the raw I/O seam OSRenumberIO fills from the standard library.
// The package itself never resolves cwd: its loomyard imports are internal/lyxdirs and internal/pattern, the latter for the entry-line cap alone.
// The caller supplies the anchor path, which in a lyx worktree is lyxcwd.Location.AnchorPath() and never WorktreePath().
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
// An Edit group's bullet may carry the re-sign arrow, `<glyph>` -> `<new declaration head>`, which marks a signature change and states the new head.
// Each arrow bullet is a CardResign (Target, Decl) in the group's Resigns and the card's flat Resigns, and its left token also stays in Refs, so the member remains an ordinary Edit target.
//
// An Extends group holds existing refs the card only adds to, and changes nothing already there.
// Its refs join the card's flat Targets as an Edit group's do, and the readers that treat Edit targets read an Extends group alike:
// card-missing-field requires ImpactSummary, path-missing checks the refs, CheckEditTestFile refuses a test file, and the glyph resolve pass sees them unchanged.
// An arrow bullet in an Extends group is in the group's own Resigns only, never the card's flat Resigns, so no re-sign check reads it.
// The extends-overlap check refuses a ref also under another type label of its card, and extends-resign refuses an arrow bullet in an Extends group.
//
// Only path-shaped Targets/Uses/Pairs entries are normalized (see below), and a
// path-shaped entry is further canonicalized into its glyph string at parse time
// when it carries a file extension or carries no "/" at all (canonicalizablePath,
// normalize.go); a glyph-, handle-, or symbol-shaped entry is
// stored verbatim. Classification is by shape alone, via this package's own
// pure classifier (classify.go's classifyRef) and the shape.go ref-shape
// registry's ledger gates, which every dispatch site consults via lookup
// rather than comparing classifyRef's return value itself
// — never `go doc`, never a process spawn.
// planparser itself spawns no process and stays tier1-pure per the Test Tier Purity Invariant.
// glyphref.go holds the sole path->glyph call (glyph.Self), and Glyph.UnitPath is the sole glyph->path call, per the glyph-conversion-chokepoint Shared Decision.
// goPackageDir (targetdirs.go) stands beside Glyph.UnitPath as the external-test unit mapping.
// CardTargetDirs and RefScope call it.
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
// This package's parse step is lenient at the card level: a malformed bullet or an absent field is recorded into the model (via the HasX bits, RenameRaw, or a nil slice) rather than failing the parse,
// so Validate can enumerate every defect in one pass instead of stopping at the first one.
// ParsePlan fails loud only on document-structure errors — a missing or undecodable overview file, an unparseable Card Index line, a missing card file, an unparseable card heading, or an inline value where a field admits only a bullet list.
// The plan format's validation checks (card type presence, card-custom-not-alone, path malformation, the bare package-qualified symbol, directory-target and glyph-malformed hard rules, the Rename pair grammar, the plan: handle consistency checks, on-disk existence, and so on) are implemented by Validate in validate.go, not by ParsePlan itself.
//
// # Reads beside ParsePlan
//
// OverviewWithoutCardIndex returns the overview's bytes with the Card Index section cut out, bounded exactly as ParsePlan reads the index lines.
// A caller that compares overviews across plan edits hashes that form, so a change confined to the Card Index is told apart from a change anywhere else without the caller parsing the overview.
//
// IdentityText returns the overview's bytes without the frontmatter's `approved:` line, the one line SetApproved writes.
// A caller that decides whether a plan overview changed since it was reviewed hashes that form, so an approval alone never counts as a change.
//
// # Checks outside both entry points
//
// CheckFirstCard, CheckCardFabricReference, CheckFabricVocabularyName and CheckEditTestFile sit outside ValidateFormat and Validate.
// Only a caller holding the rework round's first card number, or a matcher for the fabric-reference rule, can run the first two.
// CheckFabricVocabularyName needs no told input, but the plan index appends it beside CheckCardFabricReference.
// CheckEditTestFile is appended by the plan index at format and rework validation, and takes the done set so format validation skips a completed card.
// CheckCardFabricReference scans the raw text the parse step records in Plan.OverviewText and Card.Text, so it reads no file and planparser stays the sole parser.
//
// # Entry-line cap
//
// pattern-entry-line-cap fails a card whose fenced code block holds a line opening like a PATTERN.md entry line, "- `PATTERN-", longer than pattern.MaxEntryLineChars runes.
// It reads Card.Text alone and scans fenced blocks only, never inline spans or prose.
//
// # Verify chains
//
// verify-nested-module, verify-module-wide and verify-package-missing read a shell chain through one splitter.
// It joins a backslash-newline continuation into one line, then splits at `&&`, `||`, `;`, `|` and a newline, reading `||` as one separator and a separator inside a single- or double-quoted span as text.
// A segment carries the separator that preceded it, so verify-nested-module follows a `cd` only after a sequencing separator and ends its scan at a `cd` in a pipeline stage or after `||`.
// VerifyCommands exports the same split for the plan gate's verify-raw-command and verify-gate-no-wait context checks, which loomshed appends outside this package's entry points.
// The three read card Verify values only; the overview's `## verify:` section is still parsed but no check reads it, and the plan gate's plan-verify-retired finding reports it.
//
// # The language: key and the glyph alphabet
//
// The overview's optional `language:` frontmatter key names the alphabet a plan's Targets/Uses/Pairs symbol-shaped entries are spelled in.
// "go", the default when the key is absent, enables glyph.Go's alphabet and "csharp" enables glyph.CSharp's.
// "none" opts a plan out of glyphs entirely, keeping every pre-glyph symbol-shaped behavior unchanged.
// Under "go" and "csharp", ParsePlan canonicalizes every canonicalizablePath-eligible path-shaped entry into its glyph string immediately after root:/// resolution.
// Plan.SurfaceRefs records EVERY one of a canonicalized entry's pre-canonicalization surface lexemes, keyed by the owning card's identity and the resulting canonical string.
// That canonicalization goes through the Go unit alphabet under both glyph languages, since quarry spells a path in that alphabet whatever the plan's language;
// PathEntryPath is the one Go-alphabet reading of a canonicalized path entry back to its path, and its callers are the language-mismatch check and the rename-namespace-pair check.
//
// ParseRef parses a ref against the alphabet the plan's language selects, and follows three rules under csharp:
// a ref the C# alphabet accepts is a C# glyph;
// a ref only the Go alphabet accepts is returned only when it is a self glyph, which is how a canonicalized path entry reads;
// any other ref returns the C# alphabet's parse error.
// A ref both alphabets parse is therefore C#.
// Under none it returns an error and parses nothing.
// A slash-free self glyph such as `Glyphchain.Sharp#` reads as a namespace under csharp, so diskPathForRef gives it no disk path and the path checks that key on it skip it:
// the bound is that a repository-root file or directory spelled without a slash is not checked against the disk under csharp.
//
// Plan.GlyphLanguage answers only whether the plan is glyph-enabled.
// Plan.GoToolchain is true under go or an absent language and is the one test for the checks that read Go source or Go toolchain semantics:
// the verify-chain checks verify-nested-module, verify-module-wide and verify-package-missing, the Go package-directory mapping of a unit in CardTargetDirs and RefScope, the external-test pseudo-unit reading in CheckFabricVocabularyName, and the delete-order and caller-coverage checks in internal/planglyph.
// Each of them leaves a csharp or none plan alone.
//
// plan-language-mismatch refuses a plan whose Targets name only the other language's files, a `.cs` file under go or a `.go` file under csharp, unless a member glyph, a `plan:` handle or an extensionless self glyph shows the plan may name code of its own language.
// Its resolve-backed half, in internal/planglyph, judges a go plan whose targets resolve only into `.cs` files.
// rename-namespace-pair refuses, under csharp, a Rename pair of two self glyphs whose old side is slash-free and names nothing on disk, since such a pair is a file pair that runs `git mv` and a namespace is no path.
package planparser
