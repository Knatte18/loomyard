MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude (Opus-class, harness-reported "Opus 5"); exact build not self-verifiable
reviewed_file: plan/
date: 2026-09-06
```

## Findings

### [BLOCKING:scope] format-5 bump's fixture blast radius is unenumerated
**Location:** batch 2 card 7 / card 8, batch 5 cards 23–25
**Issue:** Card 7 sets `recognizedFormat` to 5, but only card 8 rewrites fixtures, and only `internal/planparser/testdata/goodplan`; `format: 4` overviews also live in `internal/loomcli/validate_test.go` (`planFixture`, line 178), `internal/loomshed/gatefindings_test.go` (line 79), `internal/loomshed/planvalidate_test.go`, `internal/webstercli/cli_test.go`, `internal/websterengine/runlevel_test.go`, `internal/loomrecipe/fixture_test.go` and `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` — every one of those now gains a `format-unrecognized` finding, and the break lands in batch 2 whose `verify: go test ./internal/planparser/` cannot see it (`go build ./...` runs no tests).
**Fix:** Name the format-5 fixture sweep as its own card in batch 2, listing each of those files in `Edits:`, so the bump and its blast radius land in the batch that causes them.

### [BLOCKING:scope] parity fixtures' `language: none` cannot be added where card 25 says
**Location:** batch 5 card 25 (and card 23's batch-local decision)
**Issue:** Card 25 says to add `language: none` "to each of those four fixtures' overviews" in `internal/loomcli/parity_test.go`, but two of the four (`CleanApproved`, `Unapproved`, parity_test.go:218/226) are built by `planFixture`, which lives in `internal/loomcli/validate_test.go` — a file card 25 does not list in `Edits:` and card 24 edits without any `language:` instruction.
**Fix:** Move the `planFixture` overview edit into card 24's `Requirements:`, or add `internal/loomcli/validate_test.go` to card 25's `Edits:`.

### [BLOCKING:scope] loomrecipe's Plan-Validate fixture is neither edited nor verified
**Location:** batch 5 cards 23–25
**Issue:** `internal/loomrecipe/fixture_test.go`'s `buildSequenceFixture` drives the real `Plan-Validate` row (its own doc comment, lines 500–513) over a `t.TempDir()` anchor with a `format: 4`, `language:`-absent plan; after card 23 that row's `openRepo` wraps `ErrQuarryUnavailable` and returns an error instead of `Done`, breaking the sequencing tests. The file appears in no card's `Edits:`, in `## All Files Touched`, or in any batch `verify:`, so the failure surfaces only at the hub done gate.
**Fix:** Add `internal/loomrecipe/fixture_test.go` to card 23's `Edits:` (format 5 + `language: none`) and `./internal/loomrecipe/` to batch 5's `verify:`.

### [BLOCKING:consistency] card 6 freezes the two union helpers it must change
**Location:** batch 2 card 6
**Issue:** Card 6 states `createTargetsUnion` and `renameTargetsUnion` "keep their current bodies and are simply fed the mapped path instead of the raw ref", but neither takes a ref — both walk the plan themselves and filter on `isPathRef` (`internal/planparser/validate.go:572–600`), so after card 5's canonicalization every glyph `Create` target drops out of the union while `checkPathMissing`'s `satisfied()` looks up a `UnitPath`-mapped path, manufacturing `path-missing` on exactly the Create targets card 5's group-level canonicalization exists to protect.
**Fix:** State in card 6 that both union builders gain the `isGlyphRef` branch and key the union on the same mapped value `satisfied()` looks up.

### [BLOCKING:scope] loom.md's Plan-Validate detail and check count go stale
**Location:** batch 5 cards 23–25; batch 2 cards 3/4/7
**Issue:** `manifest/designs/loom.md` line 146 states the verb and the row call `planparser.ValidateFormat`/`planparser.Validate` in each mode, and line 163 pins "seventeen check IDs … sixteen of the seventeen upstream by `Plan-Validate`"; cards 23/24 falsify the first and cards 3/4/7/10/11/14 add at least seven IDs. Neither `manifest/designs/loom.md` nor `manifest/designs/plan-card-format.md` (line 104, same seventeen-ID claim) appears in any card's `Edits:` or in `## All Files Touched`, against CLAUDE.md's docs-land-in-the-same-commit rule.
**Fix:** Add `manifest/designs/loom.md` to card 25's `Edits:` with a requirement to restate the Plan-Validate detail over `planglyph` and drop the pinned count.

### [NIT:consistency] classifier rule 4 silently root-joins bare filenames
**Location:** batch 2 card 4 / card 5
**Issue:** Making a dot-free, slash-free token `refKindPath` also routes it through `normalizeRefIfPath` (`internal/planparser/normalize.go:88`), so under a non-empty `root:` a `Makefile` entry becomes `<root>/Makefile` where today it passes through verbatim — a behaviour change card 4 calls a "repository-root filename" and no listed test covers.
**Fix:** State the intended `root:` behaviour for rule-4 tokens in card 4 and add it to card 5's test list.

## Verdict

REQUEST_CHANGES
Format-bump blast radius, two union helpers, and stale loom docs are unaccounted for.
MILL_REVIEW_END
