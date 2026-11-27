// Package planparser is the sole parser and sole writer of the on-disk plan format under `_lym/plan/`.
// contracts/specs/loom-plan-spec.md pins that format.
// No other package reads or writes `_lym/plan/` directly.
// Every consumer goes through ParsePlan and the Plan/Card model it returns.
// Only the package's write paths rewrite a `_lym/plan/` byte.
// SetApproved writes the approval flag, and RewriteRefs substitutes refs across the plan (rewrite.go).
// AppendAmendment appends to the amendment log (amendment.go).
// InsertCard and RemoveCard add or drop one card and renumber the cards after it (renumber.go).
// RestoreRenumber undoes an interrupted renumber (restore.go).
// JoinParts joins the plans of a split task's parts into one plan numbered in part order (join.go).
// ExportMet (join.go) reports whether a part's plan delivers one of its exports, for the join and the part check.
// So the on-disk grammar has one reader and a fixed set of writers, and no other code re-derives it.
// Overlay (overlay.go) is a read path that writes no `_lym/plan/` byte:
// it lays proposed contents of the overview and the cards the Card Index lists over a plan directory,
// and a plan parsed through it reads, rewrites (Plan.RewriteRefs) and re-parses (Plan.Reparse) in memory.
//
// # The Glyph Conversion Chokepoint Invariant
//
// loomyard performs no glyph-to-path or path-to-glyph conversion of its own.
// The external-test unit mappings below are the only exception.
// glyph.Self (glyphref.go) is the only path-to-glyph call, and Glyph.UnitPath is the only glyph-to-path call.
// glyph.Parse and Glyph.String are the only glyph grammar.
// No code in this package or its callers may apply `strings.TrimSuffix(s, "#")` to a glyph-typed value.
// None may read Glyph.Unit as a disk path or run a local regex over a glyph string.
// cmd/lym/constraintchokepoint_test.go guards this mechanically.
// goPackageDir and GoPackageCounterpart (targetdirs.go) are the sanctioned adjustments beside Glyph.UnitPath.
// They exist for the external-test pseudo-unit only.
// quarry names an external test package's unit `<dir>_test`, a pseudo-path with no directory.
// Glyph.UnitPath returns that pseudo-path unchanged.
// goPackageDir maps that pseudo-unit to its package directory.
// GoPackageCounterpart maps a member glyph between `<dir>` and `<dir>_test`.
// So a done-check or handle binding finds a test written into the package its plan did not name.
// ParseRef (glyphref.go) is the one alphabet choice for a plan ref and for a `plan:` handle's body.
// It reads the alphabet from the plan's `language:`, so no other package passes a glyph.Language to a parse.
// An id quarry minted for a C# member is no plan ref, and ParseMintedCSharpID is the one parse of it.
// PathEntryPath is the one Go-alphabet reading of a canonicalized path entry back to its path.
// Quarry's Enclose is a location-to-glyph conversion, and internal/planglyph's EncloseAt is its one sanctioned call.
//
// # Path ownership
//
// planparser owns not only the plan format but where the plan directory lives.
// PlanDirName and PlanDirRel declare the worktree-relative token `_lym/plan`, forward-slash, used for Card.SourcePath.
// PlanDir and PlanOverview declare the absolute form.
// PlanScratchDir declares the plan directory's never-tracked mirror, joined from the same segment as PlanDir.
// RenumberJournalPath declares the journal under it.
// InsertCard and RemoveCard write that journal before they touch the plan directory and remove it when they finish.
// So a journal found on disk is an interrupted renumber.
// RestoreRenumber is the journal's only reader: it undoes that renumber from the journal.
// That discards any edit made to the journaled files since.
// All file operations of a renumber go through RenumberIO, which OSRenumberIO fills from the standard library.
// The package never resolves cwd.
// Its loomyard imports are internal/lymdirs and internal/pattern, the latter for the entry-line cap alone.
// The caller supplies the anchor path, which in a lym worktree is lymcwd.Location.AnchorPath() and never WorktreePath().
//
// # Type model
//
// A parsed plan is a Plan.
// It holds the overview's frontmatter (Format, Approved, Root) and its task-framing paragraph (Framing).
// It holds the flat ordered list of Cards.
// It holds the three optional plan-level body sections (SharedDecisions, RenameMechanic, Verify).
// A Card carries one or more type labels.
// Each label's own occurrence on the card is a TargetGroup (TargetGroups).
// A TargetGroup holds that label's own Refs and, for a Rename group, its own Pairs and malformed RenameRaw bullets.
// The flat Targets, Pairs, and RenameRaw fields are the union across the card's TargetGroups, in body order.
// They serve consumers that read the card-level shape rather than the per-label groups.
// Type (the first label seen) and TypeLabelCount are not validation state;
// a check must key on TargetGroups, never on Type, or it reintroduces first-label-wins.
// A Card also carries its Card Index fields (Number, Slug, Summary) and its own file's Title.
// Its Uses list holds refs read or depended on, not targeted.
// Its Intent prose states what and why.
// Its inline ImpactSummary is a hard-capped one-line blast-radius conclusion.
// ImpactSummaryTrailing captures any lines that follow it, which the validator reports as a defect.
// Its per-label HasX presence bits are HasType, HasUses, HasIntent, HasImpactSummary and HasVerify.
// RetiredLabels holds one entry per format-3 label the card body carries.
// The optional Commit and Verify fields complete the card.
//
// An Edit group's bullet may carry the re-sign arrow, `<glyph>` -> `<new declaration head>`.
// The arrow marks a signature change and states the new head.
// Each arrow bullet is a CardResign (Target, Decl) in the group's Resigns and the card's flat Resigns.
// Its left token also stays in Refs, so the member remains an ordinary Edit target.
//
// An Extends group holds existing refs the card only adds to, and changes nothing already there.
// Its refs join the card's flat Targets as an Edit group's do.
// The readers that treat Edit targets read an Extends group alike:
// card-missing-field requires ImpactSummary and path-missing checks the refs.
// CheckEditTestFile refuses a test file, and the glyph resolve pass sees them unchanged.
// An arrow bullet in an Extends group is in the group's own Resigns only, never the card's flat Resigns.
// So no re-sign check reads it.
// The extends-overlap check refuses a ref also under another type label of its card;
// extends-resign refuses an arrow bullet in an Extends group.
// CardRefKind says what a card does with a ref (RefChange, RefExtends, RefUse, or 0 when it does not name it).
// DependencyEdge decides, for a pair of overlapping refs its caller found, whether they make a dependency edge.
// Two Extends on one ref make no edge unless both cards fence a verbatim PATTERN.md line for one entry ID;
// every other pair of named refs makes an edge unless both are uses.
//
// Only path-shaped Targets/Uses/Pairs entries are normalized (see below).
// A path-shaped entry that carries a file extension or no "/" at all is canonicalized into its glyph string.
// That happens at parse time, and canonicalizablePath in normalize.go decides which entries qualify.
// A glyph-, handle-, or symbol-shaped entry is stored verbatim.
// Classification is by shape alone, through the pure classifier classifyRef (classify.go).
// Every dispatch site consults the ledger gates of the ref-shape registry (shape.go) by lookup.
// Classification never runs `go doc` or spawns a process.
// planparser spawns no process and stays tier1-pure per the Test Tier Purity Invariant.
// CardTargetDirs and RefScope call goPackageDir.
//
// # The root:/// resolution rule
//
// The overview's optional `root:` frontmatter key names a worktree-relative directory.
// Every path-shaped card ref resolves against it, unless the ref is `//`-prefixed.
// A `//`-prefixed ref is worktree-root-relative regardless of root:.
// ParsePlan applies this resolution once, at parse time (see normalizeCardPath in normalize.go).
// A malformed result (a still-absolute path, a `..` escape) is left in place; catching it is the validator's job.
//
// # Field presence and omission
//
// Format 4 omits a field with no content rather than writing a `none` sentinel:
// a card does not carry a label it has nothing to say under.
// ParsePlan records a HasX presence bit per recognized label.
// It also preserves the nil-versus-empty distinction on the label's own slice-typed field.
// A label written with no bullets under it (non-nil empty slice, HasX true) becomes a card-field-empty finding,
// and an omitted required label (nil slice, HasX false) becomes a card-missing-field finding.
// These are different card defects, and only the validator, not this package, turns either into a finding.
//
// The format-3 labels stay recognized and route into RetiredLabels, each producing a card-retired-label finding.
// They are `**What:**`, `**Context:**`, `**Edits:**` and `**Creates:**`;
// `**Deletes:**`, `**Moves:**`, `**Depends-on:**` and the lowercase `**verify:**` complete the set.
// A label removed from the recognized set would be swallowed into Intent:'s prose.
// That silent misparse is what this format's fail-loud discipline exists to prevent.
//
// # Validation lives in validate.go
//
// This package's parse step is lenient at the card level.
// A malformed bullet or an absent field is recorded in the model rather than failing the parse:
// the HasX bits, RenameRaw and a nil slice carry it.
// So Validate can enumerate every defect in one pass instead of stopping at the first one.
// ParsePlan fails loud only on document-structure errors.
// Those are a missing or undecodable overview file, an unparseable Card Index line, and a missing card file.
// They also include an unparseable card heading and an inline value where a field admits only a bullet list.
// Validate in validate.go implements the plan format's validation checks; ParsePlan implements none of them.
//
// # Reads beside ParsePlan
//
// OverviewWithoutCardIndex returns the overview's bytes with the Card Index section cut out.
// The cut is bounded exactly as ParsePlan reads the index lines.
// A caller that compares overviews across plan edits hashes that form.
// So a change confined to the Card Index is told apart from a change anywhere else without parsing the overview.
//
// IdentityText returns the overview's bytes without the frontmatter's `approved:` line, the one line SetApproved writes.
// A caller that decides whether a plan overview changed since review hashes that form.
// So an approval alone never counts as a change.
//
// CardCSharpSeeds returns one CSharpSeed per target ref of a C# plan's cards.
// Each seed holds the file the ref names and the type names its owner chain seeds.
// A test scoper that starts from types consumes them.
// CardCSharpSeeds reads no disk.
// A ref that is a directory or a namespace-only self glyph yields a seed with neither file nor types.
// Such a glyph is the slash-free spelling the bound below describes.
// The scoper reads that seed as a fallback to whole.
//
// # Test-file patterns
//
// IsTestFile reports whether a slash-separated, repository-relative path is a test file of a language.
// It reads a per-language table declared beside the source-extension table.
// A language the table lacks, `none` included, reads every path as a test file.
// So a cap built on it over-covers rather than lets a test edit through.
// IsAnyLanguageTestFile is the union over every declared language, for a caller with no plan.
// PlanLanguage returns a plan's `language:` value, reading an absent key as `go`.
// A caller uses it to tell which language its caps match against.
//
// # Checks outside both entry points
//
// These checks sit outside ValidateFormat and Validate:
// CheckFirstCard, CheckCardFabricReference, CheckFabricVocabularyName and CheckEditTestFile.
// CheckFirstCard and CheckCardFabricReference need told input:
// the rework round's first card number, or a matcher for the fabric-reference rule.
// CheckFabricVocabularyName needs no told input, but the plan index appends it beside CheckCardFabricReference.
// The plan index appends CheckEditTestFile at format and rework validation.
// It takes the done set, so format validation skips a completed card.
// It also takes a test naming the files that declare no member glyph, which it admits.
// Only the index can read the tree.
// CheckCardFabricReference scans the raw text the parse step records in Plan.OverviewText and Card.Text.
// It reads no file, so planparser stays the sole parser.
// It also runs the path rule over each card id.
// An id ending in the fabric suffix would make every command naming the card a fabric reference.
// The implementer audit would then flag each such command.
// A `lym gate` invocation is the exception.
//
// # Part scope
//
// Plan.Part, a *PartScope, marks a plan that is one part of a split plan.
// A caller sets it after parsing a part directory.
// It is nil for an ordinary plan, whose findings it never changes.
// Under it handle-dangling counts a handle in Part.Handles as claimed, which never counts toward handle-collision.
// Under it path-missing counts a path in Part.Paths as satisfied, as a Create target of the plan is.
// The part-spot-target check runs only under a part scope, right after path-missing.
// It refuses a ref of any target group, never a Uses entry, whose disk path is one of Part.SpotPaths.
// The chair writes the shared spots' cards.
// Plan.Reparse keeps the scope.
//
// # Entry-line cap
//
// pattern-entry-line-cap fails a card whose fenced code block holds an over-long entry line.
// An entry line opens like a PATTERN.md entry line, "- `PATTERN-".
// It is over-long when its rule text, as pattern.EntryRuleText cuts it, exceeds pattern.MaxEntryTextChars runes.
// It reads Card.Text alone and scans fenced blocks only, never inline spans or prose.
//
// # Verify chains
//
// verify-nested-module, verify-module-wide and verify-package-missing read a shell chain through one splitter.
// It joins a backslash-newline continuation into one line, then splits at `&&`, `||`, `;`, `|` and a newline.
// It reads `||` as one separator and a separator inside a single- or double-quoted span as text.
// A segment carries the separator that preceded it.
// So verify-nested-module follows a `cd` only after a sequencing separator,
// and it ends its scan at a `cd` in a pipeline stage or after `||`.
// VerifyCommands exports the same split for the plan gate's verify-raw-command and verify-gate-no-wait context checks.
// Loomshed appends those checks outside this package's entry points.
// The three read card Verify values only.
// The overview's `## verify:` section is parsed but no check reads it.
// The plan gate's plan-verify-retired finding reports it.
//
// # The language: key and the glyph alphabet
//
// The overview's optional `language:` key names the alphabet of a plan's symbol-shaped Targets/Uses/Pairs entries.
// "go", the default when the key is absent, enables glyph.Go's alphabet and "csharp" enables glyph.CSharp's.
// "none" opts a plan out of glyphs entirely, keeping every pre-glyph symbol-shaped behavior unchanged.
// Under "go" and "csharp", ParsePlan canonicalizes every eligible path-shaped entry into its glyph string.
// It does so right after root:/// resolution; canonicalizablePath decides eligibility.
// Plan.SurfaceRefs records every pre-canonicalization surface lexeme of a canonicalized entry.
// It is keyed by the owning card's identity and the resulting canonical string.
// Canonicalization goes through the Go unit alphabet under both glyph languages.
// Quarry spells a path in that alphabet whatever the plan's language.
//
// ParseRef parses a ref against the alphabet the plan's language selects, and follows three rules under csharp:
// a ref the C# alphabet accepts is a C# glyph;
// a ref only the Go alphabet accepts is returned only when it is a self glyph, as a canonicalized path entry reads;
// any other ref returns the C# alphabet's parse error.
// A ref both alphabets parse is therefore C#.
// Under none it returns an error and parses nothing.
// A slash-free self glyph such as `Glyphchain.Sharp#` reads as a namespace under csharp.
// So diskPathForRef gives it no disk path, and the path checks that key on it skip it.
// The bound: under csharp, a repository-root file or directory spelled without a slash is not checked against the disk.
//
// Plan.GlyphLanguage answers only whether the plan is glyph-enabled.
// Plan.GoToolchain is true under go or an absent language.
// It is the one test for the checks that read Go source or Go toolchain semantics:
// the verify-chain checks verify-nested-module, verify-module-wide and verify-package-missing;
// rename-leaves-test;
// the Go package-directory mapping of a unit in CardTargetDirs and RefScope;
// the external-test pseudo-unit reading in CheckFabricVocabularyName;
// and the delete-order and caller-coverage checks in internal/planglyph.
// Each of them leaves a csharp or none plan alone.
//
// plan-language-mismatch refuses a plan whose Targets name only the other language's files.
// Those are a `.cs` file under go or a `.go` file under csharp.
// A member glyph, a `plan:` handle or an extensionless self glyph lifts the refusal.
// Its resolve-backed half, in internal/planglyph, judges a go plan whose targets resolve only into `.cs` files.
// Under csharp, rename-namespace-pair refuses a Rename pair of two self glyphs.
// It refuses the pair when the old side is slash-free and names nothing on disk.
// Such a pair would run `git mv` as a file pair, and a namespace is no path.
// rename-leaves-test refuses a symbol Rename pair while a card lists a test named after the symbol under Edit.
// It refuses only when that test has no Rename pair of its own.
// An implementer renames the test along with its symbol.
package planparser
