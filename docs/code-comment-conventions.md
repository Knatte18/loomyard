# Code comment conventions — Go only for now

> **Status: implemented; a durable convention doc, not a module-design draft.**
> It lived under `manifest/designs/` until the 2026-08-29 designs audit, which found it misfiled: `manifest/` holds planned, not-yet-built work, and the [documentation lifecycle](overview.md#documentation-lifecycle) would read an implemented doc there as a deletion candidate.
> This doc is not that class — it is the standing rationale for a cross-cutting rule that live producer rubrics still cite (`contracts/stencils/loom/loom-rubric-webster-review.md`, guarded by `contracts/stencils/rubric_test.go`), so it moved here rather than being deleted.
> The operative rule lives in the `scribe` plugin's `code-quality` skill (Comments section) and `golang-comments` skill (`plugins/scribe/skills/`), installable via loomyard's own marketplace — that skill is the single source an agent actually loads and follows.
> This document is design rationale — the "why" behind the rule, not the rule text itself, which is restated once below only as context for a reader of this file.
> No producer-stencil wiring yet: "Load these skills" lines in loom's own producer prompts are planned as a separate, later roadmap item ("loom: Discussion-Write producer"), not this one.
> C#/Python versions deferred until Go is proven.

## The rule

See `plugins/scribe/skills/code-quality/SKILL.md`'s Comments section for the operative rule, its two exceptions, and the information-triage list — that skill is what an agent actually loads, so it is the canonical text.
Restated here only for a reader of this document: a doc comment states only what the symbol it documents does and why it exists, as a standalone contract — never in terms of, in comparison to, or dependent on any other named symbol.

## Why this is stronger than a staleness rule

The deeper problem is not that a cross-reference can go stale.
It is that describing a symbol's behavior in terms of what it uses, or how it differs from something else, is implementation detail — and implementation detail does not belong in a doc comment at all, regardless of whether the reference stays accurate.

A doc comment needing to look outside itself to be readable is already a defect independent of staleness — the symbol underneath it is not adequately self-contained.

This is the rationale the skill itself doesn't carry — a skill states the rule, not why it's the right rule — kept here as this task's design record.

## What replaces the lost context: query, not prose

The relationship this rule refuses to hardcode into text is answered on demand instead.
`quarry impact <symbol>` (and `refs`) must return, for every caller, not just its file:line but the caller's full enclosing function/method **and that caller's own doc comment**.
"Who depends on X, and why" is answered by running the query and reading the caller's own self-contained comment — never by X's comment naming the caller, and never by the caller's comment naming X.

Quarry doesn't exist yet — this section documents the intended replacement mechanism, not a shipped one.

## Enforcement

Two tiers, same shape as every other mechanical gate in this initiative:

1. **Now — review discipline**, carried by the `code-quality`/`golang-comments` skills above.
   No mechanical check exists yet.
2. **Once Quarry exists — a lint pass, advisory not a hard gate.**
   Because the rule has no "except when load-bearing" carve-out, the check is a clean mechanical target: flag any identifier-shaped token in a doc comment that resolves to a real symbol outside the current type/package (excluding the two exceptions named in `code-quality`'s Comments section — self-reference and stdlib/language contracts).
   No judgment about necessity is required — the rule is absolute, so a hit is always a violation.

Wiring into producer prompts is planned, not yet done: once loom's own producers write code, their stencils are meant to compose in a "Load these skills: ..." section (see `manifest/designs/plan-card-format.md`) naming `code-quality`/`golang-comments`, not leave invocation to model discretion.
That wiring is the separate, later roadmap item named above.
