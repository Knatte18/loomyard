MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
duration_s: 178.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [NIT:decision] `prosa-symbol-target` disposition unstated
**Demoted-from:** BLOCKING
**Section:** `package-spelling`, `directory-classifier-rule`
**Issue:** `checkProsaSymbolTarget` (`internal/planparser/validate.go:491`) flags any Prosa-group ref where `isPathRef` is false, yet `package-spelling` explicitly wants a Prosa card targeting a unit self glyph `internal/foo#`, and after canonicalization even file refs are glyphs — so the check either fires on every Prosa card or must be redefined in glyph terms.
**Fix:** state the check's new rule (which glyph kinds a Prosa group may target) or its retirement, as a named decision.

### [NIT:consistency] Scope claims new producer rows; `drift-boundaries` denies them
**Demoted-from:** BLOCKING
**Section:** Scope "In" vs `drift-boundaries`
**Issue:** Scope lists "New producer rows plus their parity CLI verbs, per the Gate Self-Check Parity Invariant" and Testing says "the existing parity test extends to every new producer row / verb pair", while `drift-boundaries` concludes "No new row, no new verb, and nothing to invent" — verified: `internal/shedrecipe/registry.go` holds exactly the fourteen names cited.
**Fix:** delete or restate the Scope and Testing bullets so the plan writer is not sent looking for registry entries that must not exist.

### [BLOCKING:design] `root:` shorthand × glyph canonicalization unordered
**Section:** `parse-time-canonicalization`, `file-spelling`
**Issue:** `normalizeCard` rewrites refs at parse time but is classifier-gated (`normalizeRefIfPath`, `internal/planparser/normalize.go:88`), so once a surface-spelled ref is a glyph it is no longer `refKindPath` and `root:`/`//` resolution silently stops applying; the discussion never fixes the order between root-joining and `glyph.Self`.
**Fix:** state whether `root:` applies before canonicalization and whether a surface glyph (`focus.go#`) under a non-`.` `root:` is legal, resolved, or a finding.

### [BLOCKING:design] `RewriteRefs` keyspace vs. on-disk surface spelling
**Section:** `rewrite-write-path`
**Issue:** the `Plan` model holds canonicalized, root-resolved glyphs while `_lyx/plan/` bytes hold whatever the planner wrote (plain path, `root:`-relative, file self glyph), so a `map[string]string` keyed on model strings will silently miss the entries it must rewrite during binding and drift repair.
**Fix:** state whether `RewriteRefs` keys are model strings or on-disk lexemes, and how the surface↔model gap is bridged for the non-canonical spellings `file-spelling` deliberately accepts.

### [NIT:scope] Contract-doc inventory omits two affected stencils
**Demoted-from:** BLOCKING
**Section:** Technical context, "Docs that must land in the same commits"
**Issue:** `contracts/stencils/webster/webster-body-implementer.md:62` tells the implementer the deviation union is "the files holding every symbol-shaped target entry, which you resolve yourself … resolving a package-qualified symbol to its file is one read" — invalidated by glyph refs and overlapping the glyph scope guard; `contracts/stencils/loom/loom-rubric-plan-review.md:44-51` names `prosa-symbol-target` and per-symbol card guidance. Neither appears in the docs list.
**Fix:** add both files to the docs inventory with their required change, or state why each is unaffected.

### [NIT:consistency] `DeltaGit` token catches only direct textual call sites
**Section:** `quarry-spawn-guard-visibility`
**Issue:** `bannedTokens` is a raw-substring scan of untagged `*_test.go` files (`cmd/lyx/tierpurity_test.go:60`), so a test calling a `planglyph` wrapper that reaches `DeltaGit` transitively contains no such token and still passes — "anything reaching `DeltaGit` is integration-tagged" stays partly asserted.
**Fix:** note the limit explicitly (as the existing tokens accept it) rather than claiming the guard closes the gap fully.

## Verdict

REQUEST_CHANGES
Five unresolved items: one stale check, one self-contradiction, two format gaps, one doc omission.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
