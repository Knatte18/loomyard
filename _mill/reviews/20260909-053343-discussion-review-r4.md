MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration

```yaml
duration_s: 159.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:design] containment fix conflates absent answer with bad status
**Section:** Decision: status-family-disposition (1); Testing (TDD candidates)
**Issue:** `containment.go:112` guards two conditions — `!resolved` (target absent from `index`) and an out-of-vocabulary `Status` — and the discussion treats both as one fail-open hole, prescribing `doneCheckVerdicts`' shape, which hard-errors on absence; but absence is structurally legitimate here: `resolvePass` resolves `collectGlyphTargets(pending, …)` (`planglyph.go:169`, handles excluded) and then calls `resolveContainment(current, results)` (`planglyph.go:241`) where `current` may be the plan *reloaded after* `CanonicalizeHandles` rewrote handles into glyph refs that were never in the batch.
**Fix:** State explicitly that only the unreadable-`Status` branch gets the vocabulary guard and that an index miss keeps its silent `continue` (with the reason), or name the alternative disposition and why it does not fire on canonicalization-introduced targets.

### [NIT:consistency] Third rename arm is a kind gate, contra the discussion
**Demoted-from:** BLOCKING
**Section:** Decision: kind-policy-ledger (worked examples); Q&A "Disposition vocabulary and ledger granularity"
**Issue:** The discussion says `checkRenamePairShape`'s third arm "is not a kind-gate at all — it is a glyph-grammar refinement (`IsSelf`)", but `validate.go:800` is `else if classifyRef(p.New) == refKindHandle` — a live `refKind` comparison in `validate.go`, which the boundary grep (`{classify.go, shape.go}` only) would flag, and which no policy in the two declared gates covers.
**Fix:** Give that comparison a stated disposition — reuse of the to-side policy's `dispKeep` lookup, a third registered policy, or a named site exemption — instead of asserting the arm carries no kind gate.

### [BLOCKING:design] Enforcement mechanism (raw text vs AST) left open
**Section:** Decision: kind-policy-ledger (3); Testing (Meta-tests to add)
**Issue:** The tests are called "grep-shaped" over source text, yet the cited precedent states the opposite (`internal/cliwire/bannedecl_enforcement_test.go:71`: "The match is on the AST, never on raw text, so a doc comment naming a function cannot trip it") — and existing production comments outside the boundary already carry the banned tokens: `donecheck.go:24` and `planglyph/handle.go:326` spell `"plan:"`, `normalize.go:168`, `parse.go:160`, `validate.go:408/444/488/756-763` name `refKind*`/`isPathRef`.
**Fix:** Decide and state whether the scans are AST-based or text-based, and if text-based, say how comments and string-literal occurrences are excluded (or which existing comments must change).

### [NIT:decision] `resolveKeyFor`'s disposition left as an either/or
**Section:** Technical context → Inventory B
**Issue:** "becomes a thin wrapper over `HandleBody` or is replaced by it" is two options with no pick, and the signatures differ — `resolveKeyFor(ref) string` falls through to `ref` unchanged (`donecheck.go:35-40`), `HandleBody` returns `(string, bool)`.
**Fix:** Name one, since "replaced by it" forces all 7 call sites to handle the `ok` return.

## Verdict

REQUEST_CHANGES
Three source-verified gaps: containment fix scope, an unaccounted kind gate, and undecided enforcement mechanism.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
