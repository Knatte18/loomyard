# Adopt quarry's glyph alphabet as the plan alphabet

> **Status: Planned, design settled with the operator 2026-09-05.**
> Full text lives in [GitHub issue #226](https://github.com/Knatte18/loomyard/issues/226) — this doc is the short pointer `manifest/roadmap.md`'s Maintenance section asks for, not a restatement.
> Unblocked since quarry's T5b merged (the `glyph` package is importable, cgo-free).
> All work is in this repo; quarry only supplies plan-unaware primitives (`toc`, batched `resolve`, `expand`, and three planned ones — the `glyphs` flat index, the glyph-maker, and diff-to-symbols).
> **Supersedes the former Someday `quarry-backed plan symbol verification` item**, retired 2026-09-05: that item's option (b) — mechanically cross-checking a card's symbol declarations against the code — is exactly what glyph-based `Resolve` does, and its "no unique key for a bare symbol name" gap (GitHub issue #225, judged outdated by the operator against this issue) is closed by the glyph alphabet itself rather than a `package+receiver+name` key scheme. Both GitHub issues (#225, #226) are closed as folded into this item.

## What changes

- `planparser` imports `glyph`; the plan format's spelling for existing symbols changes to glyphs (`internal/shedrecipe#Lookup`-shaped).
- The validator calls quarry's batched `Resolve` once per plan draft, getting per-target verdicts (`found` / `not_found` with unit status / `ambiguous` with candidates).
- Plain file paths in `Uses`/`Creates` keep validating and packing exactly as today — mixing glyphs and paths is per-target, never worse than the current format.
- **Hard rule: the LLM never spells a glyph.** Every glyph in a plan is copied verbatim from a quarry answer; the validator's report echoes kind + signature per resolved glyph so a valid-but-wrong-referent guess is visible in plan review.

## Create/Rename cards: placeholder handles

A symbol a plan creates doesn't exist yet, so its glyph is unknowable up front. Handles are spelled `plan:<expected-glyph>` (the `plan:` prefix is unambiguous against both the glyph grammar and file paths, and survives shell/YAML/Markdown). The planner drafts a self-chosen `plan:` spelling; mechanical validation batch-calls quarry's glyph-maker to canonicalize every handle across the plan, and the DAG runs on handles until the creator card completes, at which point the handle binds to the real glyph via diff-to-symbols (exact-tier match binds deterministically; a miss degrades to a visible review decision).

Rename cards reuse the same handle machinery for the to-side, with quarry's diff-to-symbols exact/evidence tiering deciding whether a rename auto-binds or needs review; the same tiering also sharpens Delete's done-check against a symbol that was actually renamed rather than removed.

## Drift detection

Plans re-resolve at every boundary (pack dispatch, done-checks, one batched `Resolve` after each card merge). The drift signal is deterministic: diff-deleted symbols intersected with the remaining plan's references, gated so a Rename card's own expected outcome is never mistaken for drift. An exact-tier rename auto-repairs the plan (rewrite + revalidate, logged as an amendment); an evidence-tier candidate goes to review.

## Mechanical uses, no LLM involved

Execution DAG at symbol granularity, done-checks (`Create` unresolved / `Delete` still resolves), scope-guard (diff mapped to glyphs vs. declared targets), and an optional kick-start pack (resolved spans injected into the implementer prompt, re-resolved, never cached — gated on quarry's own M4 measurement).

## Deliberately out of scope

No LSP-shaped tools in an agent's own hands (measured flat-to-negative in quarry's ladders); semantics enter only the mechanical layer. The Planner's toolset stays `toc` + the validator.

## Related

- [GitHub issue #226](https://github.com/Knatte18/loomyard/issues/226) — full proposal text.
- [GitHub issue #225](https://github.com/Knatte18/loomyard/issues/225) — the symbol-key ambiguity problem this supersedes.
- [webster-parallel-execution.md](webster-parallel-execution.md) — the DAG-scheduler consumer waiting on symbol-derived edges.
