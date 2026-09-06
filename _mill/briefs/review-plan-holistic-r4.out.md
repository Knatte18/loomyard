MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude (Anthropic), Opus-class; exact release self-reported as Opus 5
reviewed_file: plan/
date: 2026-09-06
```

## Findings

### [BLOCKING:design] Batch 5 existing fixtures use non-repo worktree roots
**Location:** batch 5 / cards 23, 24, 25
**Issue:** `internal/loomcli/parity_test.go` builds every fixture with `worktreeRoot := t.TempDir()` (lines 272–273) and `internal/loomshed/planvalidate_test.go` does the same (lines 125–126, 161–162); once card 23/24 route those roots into `planglyph.Validate`, card 21's `openRepo` wraps `ErrQuarryUnavailable` on a non-repository directory, so `CleanApproved`/`Unapproved`/`FormatInvalid` all become `verdictError` — which contradicts card 25's "keep every existing cell's expectation" and makes card 25's seventh non-repository case indistinguishable from the four already there.
**Fix:** state in cards 23/24/25 how the pre-existing fixtures are made to pass — either seeding a real quarry-openable repository at `worktreeRoot`, or writing `language: none` into those fixture overviews — and restate which cells change.

### [BLOCKING:design] Create arrow bullet is unreachable at the named seam
**Location:** batch 3 / card 9
**Issue:** `parseTypeLabelCase` never sees raw sub-bullet text — it delegates to `parseRefField`, which applies `stripBackticks(...)` to each payload (parse.go line 567), so `` - `plan:X` -> `Decl` `` arrives as `plan:X` -> `Decl` with the outer pair already stripped and can never match the `` `x` -> `y` `` shape `moveLineRe` pins (parse.go line 401); card 9 also never names the field where a malformed Create arrow bullet is captured, which card 10's `checkHandleMalformed` must read.
**Fix:** point card 9 at the `parseRefField` payload site (or a create-specific parse path) that still holds the raw bullet, and name the `Card`/`TargetGroup` field the malformed bullet lands in, the way `RenameRaw` is named.

### [BLOCKING:scope] Card 5 never names the TargetGroups fields to canonicalize
**Location:** batch 2 / card 5
**Issue:** `normalizeCard` normalizes card-level `Targets`/`Uses`/`Pairs` and `TargetGroups[i].Refs`/`Pairs` independently (normalize.go lines 56–74), and `checkProsaSymbolTarget`, `checkPathMissing`, `checkCardFieldEmpty` and `createTargetsUnion` all read group `Refs` (validate.go lines 491–516, 572–587, 640–660); card 5 says only "each ref", so canonicalizing card-level fields alone leaves `createTargetsUnion` holding pre-canonical strings while `checkPathMissing` compares canonical ones, producing false `path-missing` findings.
**Fix:** enumerate the same field set `normalizeCard` walks in card 5's `canonicalizeCard` requirement, explicitly including `TargetGroups[*].Refs` and `TargetGroups[*].Pairs`.

### [BLOCKING:design] Extensionless bare filenames become unfixable hard findings
**Location:** batch 2 / cards 4, 5
**Issue:** classify.go's own doc comment (lines 29–33) records that an entry with no "." and no "/" — its example is `Makefile` — falls to `refKindSymbol`; card 4 makes every `refKindSymbol` target a hard `bare-symbol-target` finding, card 5 skips canonicalization for extensionless refs, and `//Makefile` re-normalizes to `Makefile`, so a plan targeting `Makefile`/`LICENSE`/`Dockerfile` has no legal spelling left.
**Fix:** give card 4's rule order an explicit disposition for an extensionless, slash-free non-code filename (e.g. exempt it from `bare-symbol-target`, or state the required spelling) and cover it in that card's test list.

### [BLOCKING:scope] Batch 6 verify does not run the tests card 26 adds
**Location:** batch 6 / card 26, `00-overview.md` batch index
**Issue:** card 26 edits `internal/planglyph/repo.go` and requires "Cover the four wrappers in `internal/planglyph/repo_test.go`", but batch 6's `verify:` is `go test ./internal/quarrycli/ ./cmd/lyx/ ./internal/lyxcwd/` — `./internal/planglyph/` is absent, so the tests this batch adds never execute in the batch that adds them (batch 4's verify already ran, batch 7's runs later).
**Fix:** add `./internal/planglyph/` to batch 6's `verify:` in both the batch file and the overview's `batches:` block, and drop the Batch Tests paragraph that justifies the exclusion.

### [NIT:consistency] README prerequisite bullet is in the wrong section
**Location:** batch 1 / card 2
**Issue:** the card says to extend "the `## Building` section's prerequisite list (which currently reads `- Go 1.26+`)", but `## Building` (README.md line 193) is a fenced bash block with no bullet list; `- Go 1.26+` is in `## Requirements` at line 226.
**Fix:** retarget the card's README edit at the `## Requirements` list, keeping the wording it already specifies.

### [NIT:consistency] Batch Tests prose contradicts the declared verify commands
**Location:** batches 4, 5, 6, 7 / `## Batch Tests`
**Issue:** batches 4 and 6 open by quoting a `verify:` string that omits `./internal/lyxcwd/` although their yaml carries it; batch 5 says "All three packages' untagged tests" over four packages; batch 7 lists `beginbatch_test.go`/`recordbatch_test.go` as untagged-half coverage although both files carry `//go:build integration` (their line 1) and only run in the tagged half.
**Fix:** restate each `## Batch Tests` opening to quote its own yaml `verify:` verbatim and move the two webster test files to the tagged-half sentence.

### [NIT:scope] Card 27 omits two hand-maintained registration lists
**Location:** batch 6 / card 27
**Issue:** `longlist_test.go` fails unless `quarry` is added to `newRoot()`'s `root.Long` "Available modules:" line (main.go line 74) and `helptree_test.go`'s `requiredModules` slice (line 27–29) is hand-maintained; card 27 names neither, and describes the existing `root.AddCommand(...)` order as "alphabetical-by-module" when it is board, config, ide, reed, fabric, selfreport, shuttle, burler, stencil, webster, loom.
**Fix:** name both edits explicitly in card 27's Requirements and drop the alphabetical claim.

## Verdict

REQUEST_CHANGES
Five blocking gaps: batch 5 fixtures, card 9 seam, card 5 field set, extensionless names, batch 6 verify.
MILL_REVIEW_END
