MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-02
```

## Findings

### [BLOCKING] SourcePath token construction violates Planparser Sole-Parser Invariant
**Location:** Batch 1, Card 1 (`01-planparser-card-source-identity.md`); the same construction is baked into the "card pointer is a bare worktree-relative token owned by planparser" Shared Decision in `00-overview.md`.
**Issue:** The Requirements direct `path.Join(hubgeometry.LyxDirName, "plan", cardFileName(entry.Number, entry.Slug))` inside `parse.go`, hardcoding the literal `"plan"` path segment. `hubgeometry.PlanDir`'s own doc comment states verbatim "Per the Hub Geometry Invariant, no other package may construct this path," and CONSTRAINTS.md's Planparser Sole-Parser Invariant separately pins "Resolves `_lyx/plan/` via hubgeometry, never string literals." `hubgeometry.go` never appears in any card's `Edits:` list across the whole plan, so no new accessor is ever added for planparser to call instead of the raw literal.
**Fix:** Add an exported hubgeometry accessor in the same batch (e.g. a relative-token constant/function for the "plan" segment, or a `CardSourcePath(number, slug) string`-style builder that returns the whole `_lyx/plan/NN-slug.md` token), put `hubgeometry.go` into Card 1's `Edits:`, and have `parse.go` call that accessor instead of hardcoding `"plan"`.

### [NIT] Stale doc comments left outside the plan's edited-file set
**Location:** `internal/websterengine/beginbatch.go` (`BeginDeps.Layout` field comment, Context-only, never Edited by any card); `internal/websterengine/recoverbatch.go` (`RecoverDeps.Plan` field comment — Card 3's sweep only names the package header and `recoverSpawn`'s own doc comment, not this struct-field comment).
**Issue:** `beginbatch.go`'s comment claims `RenderForkPrompt` uses `Layout` for "the PATTERN active check," which goes false once Card 3 drops `pattern_directive` from the thin fork prompt entirely — and no card edits `beginbatch.go` to fix it. `recoverbatch.go`'s `RecoverDeps.Plan` comment ("the already-parsed plan RenderForkPrompt reads its plan-level context from") names the wrong function post-split and a "plan-level context" claim that's no longer true (RenderRecoveryPrompt doesn't read `plan` either).
**Fix:** Widen Card 3's stale-comment sweep to cover every stale `RenderForkPrompt`/PATTERN-check reference in `recoverbatch.go`, and add a one-line doc fix to `beginbatch.go`'s `BeginDeps.Layout` comment (or add `beginbatch.go` to a card's `Edits:`).

### [NIT] Ambiguous test-merge instruction
**Location:** Batch 2, Card 3, `template_test.go` bullet ("Flip `TestRenderForkPrompt_InjectsSharedDecisionsAlways` and `TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch`").
**Issue:** The bullet describes one combined assertion set (no Shared Decisions/rename-mechanic body, plus a SourcePath pointer check) for both named tests without stating whether the two test functions stay separate (each keeping its own original focus plus the new pointer check) or get merged into one.
**Fix:** State explicitly whether the two test functions remain distinct or are merged, and which assertions belong to each.

## Verdict

REQUEST_CHANGES
Card-pointer construction hardcodes a path segment hubgeometry's own doc comment reserves to itself; fix before proceeding.
MILL_REVIEW_END
