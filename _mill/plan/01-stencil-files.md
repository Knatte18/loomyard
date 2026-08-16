# Batch: stencil-files

```yaml
task: 'PATTERN directives: move from Go constants to stencil files'
batch: stencil-files
number: 1
cards: 2
verify: go build ./... && go test ./...
depends-on: []
```

## Batch Scope

This batch adds the three new stencil files and registers them, and changes no Go behaviour at all.
After it lands, `stencils/pattern/` holds the three directive files, `stencils/stencils.go` embeds and registers them, and the repo-root `.gitattributes` pins all three to LF — but `internal/pattern.Directive` still returns its Go constants, untouched, so the tree is fully green with the new files simply unread by any production code path.
The external interface batch 2 consumes is the three registered stencil names — `pattern-directive-implementer`, `pattern-directive-review-fix`, `pattern-directive-orchestrator` — and the three exported byte vars `stencils.PatternDirectiveImplementer`, `stencils.PatternDirectiveReviewFix`, `stencils.PatternDirectiveOrchestrator` that batch 2's test fixtures seed from.
Batch-local decision that differs from nothing in the overview but is worth stating once: the three constants in `internal/pattern/pattern.go` are the **source** for the file bodies in this batch and are read but not edited here;
their deletion belongs to batch 2, so the prose exists in two places for exactly one batch.

## Cards

### Card 1: Create the three pattern-directive stencil files

- **Context:**
  - `internal/pattern/pattern.go`
  - `stencils/webster/webster-prefix-recovery.md`
  - `stencils/loom/loom-template-plan.md`
  - `internal/loomengine/plan.go`
  - `internal/websterengine/render.go`
  - `internal/burlerengine/engine.go`
- **Edits:** none
- **Creates:**
  - `stencils/pattern/pattern-directive-implementer.md`
  - `stencils/pattern/pattern-directive-review-fix.md`
  - `stencils/pattern/pattern-directive-orchestrator.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the new `stencils/pattern/` family directory with three files.
  Each file is a leading `<!-- … -->` banner, then one blank line, then the directive body.

  The body of `stencils/pattern/pattern-directive-implementer.md` is the raw-string-literal content of the `implementerDirective` constant in `internal/pattern/pattern.go`, copied byte-for-byte including its trailing newline.
  The body of `stencils/pattern/pattern-directive-review-fix.md` is the content of `reviewFixDirective` the same way.
  The body of `stencils/pattern/pattern-directive-orchestrator.md` is the content of `orchestratorDirective` the same way.
  Do not reword, re-wrap, re-punctuate, or re-order a single character of any of the three bodies — every body begins at its own `## Constraints` heading and ends with exactly one trailing newline, and `internal/pattern`'s own tests assert both properties.
  Do not add a heading, title, or any other content of your own above or below the body;
  the banner and the body are the whole file.

  Each banner follows the house style of `stencils/webster/webster-prefix-recovery.md` and `stencils/loom/loom-template-plan.md`: an opening `<!--` on the first line, continuation lines indented to align under the opening text, and the closing `-->` on the last banner line.
  Each banner must state four things: which `pattern.Role` the file serves (`RoleImplementer`, `RoleReviewFix`, `RoleOrchestrator` respectively);
  that `internal/pattern.Directive` reads it through `stencilstore.Read` and strips this banner with `stencil.StripLeadingComment`;
  that the stripped result is injected as a producer template's optional pattern_directive marker **value** and therefore never itself passes through `stencil.Fill`;
  and that the file declares no markers of its own and must stay marker-free because `stencilstore.Validate` parses it regardless.
  Name the consuming call sites per role: `internal/loomengine/plan.go` and `internal/websterengine/render.go` for the implementer file, `internal/burlerengine/engine.go` for the review-fix file, `internal/websterengine/render.go` for the orchestrator file.

  Two hard constraints on the banner text, both mechanically enforced elsewhere.
  Do not write a `{{` sequence anywhere in any of the three files, banner or body — write the marker's name as bare `pattern_directive` with no braces, because `stencilstore.Validate` parses every registered file with `stencil.TopLevelMarkers`.
  Do not write the substrings `weft` or `warp`, or a fabric-sense `host <noun>` phrase, anywhere in any of the three files — every stencil markdown file is inside the Fabric Vocabulary Invariant's enforcement walk in internal/lyxcwd.

  Write all three files with LF line endings.
- **Commit:** `feat(stencils): add the three pattern-directive stencil files`

### Card 2: Register the three files and pin them to LF

- **Context:**
  - `stencils/registry_test.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `stencils/stencils.go`
  - `.gitattributes`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `stencils/stencils.go`, add three `//go:embed` byte vars after the existing `WebsterBodyImplementer` var, each with a doc comment in the same one-line shape the existing fifteen use: `PatternDirectiveImplementer` embedding `pattern/pattern-directive-implementer.md`, `PatternDirectiveReviewFix` embedding `pattern/pattern-directive-review-fix.md`, and `PatternDirectiveOrchestrator` embedding `pattern/pattern-directive-orchestrator.md`.

  In the same file, append three rows to the `entries` slice as a trailing `pattern` family block, after the last `webster` row and in this order: `{"pattern-directive-implementer", &PatternDirectiveImplementer}`, `{"pattern-directive-review-fix", &PatternDirectiveReviewFix}`, `{"pattern-directive-orchestrator", &PatternDirectiveOrchestrator}`.
  Do not insert the block anywhere but last — `entries` order is `lyx stencil list`'s print order.
  The registered names must be exactly the filenames minus `.md`, because `stencilstore.RelPath` derives the family subdirectory from the substring before the first `-`, which is what makes `pattern-directive-implementer` resolve to `pattern/pattern-directive-implementer.md`.

  In the repo-root `.gitattributes`, add three lines in the same `<path> text eol=lf` shape, immediately after the existing webster-body-implementer line and before the internal/boardengine template.yaml line, one per new file.
  This file enumerates every embed target individually and carries no `stencils/**` glob, and nothing machine-checks an omission, so all three lines are required.
- **Commit:** `feat(stencils): register the three pattern-directive stencils`

## Batch Tests

`verify:` is the full `go build ./... && go test ./...`, per the overview's Shared Decision on verify scope.
The load-bearing test for this batch is `stencils/registry_test.go`'s `TestRegistry_MatchesOnDiskTree`, which asserts a bijection between `*.md` files in family subfolders and `entries` rows in both directions — a new file without a row, or a row without a file, fails it.
`TestRegistry_DefaultsAndRelPathAreConsistent` in the same file additionally pins that each registered name's `stencilstore.RelPath` resolves to a file that really exists on disk, which is what catches a name/filename mismatch in the new `pattern` family.
Confirm both actually ran rather than assuming they did — a missing `entries` row is exactly the silent failure they exist to catch.
`internal/lyxcwd/enforcement_test.go`'s Fabric Vocabulary walk covers the three new `.md` files automatically and is the guard on the banner's wording.
No new test is added in this batch: the registry bijection is already the complete check for what this batch delivers, and the byte-exactness of the bodies is asserted in batch 2, where a consumer exists to assert it against.
