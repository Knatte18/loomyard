// Package planparser is the sole parser and sole writer of the on-disk plan format under `_lym/plan/`.
// contracts/specs/loom-plan-spec.md pins that format.
// No other package reads or writes `_lym/plan/` directly.
// Every consumer goes through ParsePlan and the Plan/Card model it returns.
// Overlay is a read path that writes no `_lym/plan/` byte: it lays proposed contents over a plan directory in memory.
//
// # The Glyph Conversion Chokepoint Invariant
//
// loomyard performs no glyph-to-path or path-to-glyph conversion of its own.
// glyph.Self is the only path-to-glyph call, and Glyph.UnitPath is the only glyph-to-path call.
// glyph.Parse and Glyph.String are the only glyph grammar.
// No code in this package or its callers may apply `strings.TrimSuffix(s, "#")` to a glyph-typed value.
// None may read Glyph.Unit as a disk path or run a local regex over a glyph string.
// cmd/lym/constraintchokepoint_test.go guards this mechanically.
// The one exception is the external-test pseudo-unit `<dir>_test`, which Glyph.UnitPath returns unchanged;
// GoPackageDir and GoPackageCounterpart (targetdirs.go) map it to its package directory and back.
// ParseRef is the one alphabet choice for a plan ref and for a `plan:` handle's body;
// it reads the alphabet from the plan's `language:`.
// Quarry's Enclose is a location-to-glyph conversion, and internal/planglyph's EncloseAt is its one sanctioned call.
//
// # Path ownership
//
// planparser owns where the plan directory lives.
// PlanDirName, PlanDirRel, PlanDir, PlanOverview, PlanScratchDir and RenumberJournalPath declare it.
// InsertCard and RemoveCard write the renumber journal before they touch the plan directory.
// They remove it when they finish, so a journal found on disk is an interrupted renumber.
// RestoreRenumber undoes it, and discards any edit made to the journaled files since.
// The package never resolves cwd; the caller supplies the anchor path.
// In a lym worktree that is lymcwd.Location.AnchorPath() and never WorktreePath().
//
// # Type model
//
// A Card carries one or more type labels.
// Each label's own occurrence is a TargetGroup (TargetGroups).
// The flat Targets, Pairs, and RenameRaw fields are the union across the card's TargetGroups, in body order.
// A check must key on TargetGroups, never on a first label seen, or it reintroduces first-label-wins.
// An Edit group's bullet may carry the re-sign arrow, `<glyph>` -> `<new declaration head>`, marking a signature change.
// Its left token also stays in Refs, so the member remains an ordinary Edit target.
//
// An Extends group holds existing refs the card only adds to.
// Its refs join the card's flat Targets as an Edit group's do.
// An arrow bullet in an Extends group is in the group's own Resigns only, never the card's flat Resigns,
// and the extends-resign check refuses it.
//
// Only path-shaped Targets/Uses/Pairs entries are normalized, at parse time (canonicalizablePath in normalize.go).
// A glyph-, handle-, or symbol-shaped entry is stored verbatim.
// Classification is by shape alone and never runs `go doc` or spawns a process.
// planparser spawns no process and stays tier1-pure per the Test Tier Purity Invariant.
//
// The overview's optional `root:` frontmatter key names a worktree-relative directory.
// Every path-shaped card ref resolves against it at parse time, unless the ref is `//`-prefixed.
// A `//`-prefixed ref is worktree-root-relative.
// A malformed result is left in place; catching it is the validator's job.
//
// # Field presence and validation
//
// Format 4 omits a field with no content rather than writing a `none` sentinel.
// ParsePlan records a HasX presence bit per recognized label and keeps the nil-versus-empty distinction on its slice.
// A label written with no bullets is a card-field-empty defect; an omitted required label is card-missing-field.
// The format-3 labels stay recognized and route into RetiredLabels, so none is swallowed into Intent:'s prose.
//
// The parse step is lenient at the card level.
// A malformed bullet or an absent field is recorded in the model, so Validate can enumerate every defect in one pass.
// ParsePlan fails loud only on document-structure errors.
// Validate (validate.go) implements the plan format's validation checks; ParsePlan implements none of them.
//
// # Reads beside ParsePlan
//
// OverviewWithoutCardIndex returns the overview's bytes with the Card Index section cut out.
// IdentityText returns them without the frontmatter's `approved:` line, so an approval alone never counts as a change.
// CardCSharpSeeds reads no disk.
// IsTestFile reports whether a repository-relative path is a test file of a language;
// a language its table lacks, `none` included, reads every path as a test file, so a cap over-covers.
//
// # Checks outside both entry points
//
// These checks sit outside ValidateFormat and Validate:
// CheckFirstCard, CheckCardFabricReference, CheckFabricVocabularyName and CheckEditTestFile.
// Each needs told input such as the done set or a matcher.
// CheckCardFabricReference scans the raw text the parse step records in Plan.OverviewText and Card.Text.
// It reads no file, so planparser stays the sole parser.
//
// # Part scope
//
// Plan.Part, a *PartScope, marks a plan that is one part of a split plan; a caller sets it after parsing.
// It is nil for an ordinary plan, whose findings it never changes.
// Plan.Reparse keeps the scope.
//
// # Entry-line cap
//
// pattern-entry-line-cap fails a card whose fenced code block holds a PATTERN.md-style entry line:
// a line starting "- `PATTERN-" whose rule text exceeds pattern.MaxEntryTextChars runes.
// It reads Card.Text alone and scans fenced blocks only.
//
// # Verify chains
//
// verify-nested-module, verify-module-wide and verify-package-missing read a shell chain through one splitter.
// The splitter is exported as VerifyCommands.
// They read card Verify values only; the overview's `## verify:` section is parsed but no check reads it.
//
// # The language: key and the glyph alphabet
//
// The overview's optional `language:` key names the alphabet of a plan's symbol-shaped Targets/Uses/Pairs entries.
// "go", the default, enables glyph.Go's alphabet and "csharp" enables glyph.CSharp's.
// "none" opts a plan out of glyphs entirely.
// Under "go" and "csharp", ParsePlan canonicalizes every eligible path-shaped entry into its glyph string.
// It does so through the Go unit alphabet under both languages.
//
// Under csharp, ParseRef returns a ref the C# alphabet accepts as a C# glyph.
// It returns a ref only the Go alphabet accepts only when it is a self glyph.
// It returns any other ref as the C# alphabet's parse error.
// A slash-free self glyph such as `Glyphchain.Sharp#` reads as a namespace under csharp and has no disk path.
// So under csharp a repository-root file or directory spelled without a slash is not checked against the disk.
//
// Plan.GoToolchain is true under go or an absent language.
// It is the one test for the checks that read Go source or Go toolchain semantics.
// Those checks leave csharp and none plans alone.
//
// plan-language-mismatch refuses a plan whose Targets name only the other language's files;
// a member glyph, a `plan:` handle or an extensionless self glyph lifts the refusal.
// Under csharp, rename-namespace-pair refuses a Rename pair of two self glyphs.
// It does so when the slash-free old side names nothing on disk.
// rename-leaves-test refuses a symbol Rename pair while a card lists a test named after the symbol under Edit;
// that card has no Rename pair of its own.
package planparser
