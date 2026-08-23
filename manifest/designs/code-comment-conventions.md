# Code comment conventions — Go only for now

> **Status: designed, not implemented.** No stencil or CONSTRAINTS.md entry wires this in yet. C#/Python versions deferred until Go is proven.

## The rule

A doc comment states only what the symbol it documents does and why it exists, as a standalone contract — never in terms of, in comparison to, or dependent on any other named symbol.

Test: can a reader trust the comment as fully correct with zero knowledge of any other named symbol in the repo? If no, the comment violates the rule, regardless of how well-justified the reference feels.

No exception for "well-justified" comparative rationale ("unlike `X`'s `Y`, this does..."). If a comparison to another symbol feels necessary to explain this symbol, that is bigger-picture/architectural information — it belongs in the package doc or a `manifest/designs/*.md` doc, never in the individual symbol's own comment.

## Two narrow exceptions

- **Self-reference.** Go convention requires a doc comment to open with its own symbol's name ("`GetUser` retrieves..."). This is required, not forbidden — it is not a cross-reference.
- **Stdlib/language contracts.** Naming `io.Writer`, `error`, `context.Context`, or an interface being satisfied (`"Write implements io.Writer by forwarding..."`) is permitted — these never rename or drift.

Everything else — another internal package, another internal symbol, a comparison or contrast to how something else behaves — is forbidden, full stop.

## Why this is stronger than a staleness rule

The deeper problem is not that a cross-reference can go stale. It is that describing a symbol's behavior in terms of what it uses, or how it differs from something else, is implementation detail — and implementation detail does not belong in a doc comment at all, regardless of whether the reference stays accurate.

Triage for where information belongs:
- **What this symbol does, and why, on its own terms** — the doc comment.
- **How it does it internally, including what it calls** — the code itself, readable if someone looks; never restated in the comment.
- **How this symbol relates to others, or fits a wider pattern** — a package doc or a `manifest/designs/*.md` design doc, never an individual symbol's comment.

A doc comment needing to look outside itself to be readable that is already a defect independent of staleness — the symbol underneath it is not adequately self-contained.

## What replaces the lost context: query, not prose

The relationship this rule refuses to hardcode into text is answered on demand instead. `quarry impact <symbol>` (and `refs`) must return, for every caller, not just its file:line but the caller's full enclosing function/method **and that caller's own doc comment**. "Who depends on X, and why" is answered by running the query and reading the caller's own self-contained comment — never by X's comment naming the caller, and never by the caller's comment naming X.

## Enforcement

Two tiers, same shape as every other mechanical gate in this initiative:

1. **Now — review discipline.** No mechanical check exists yet.
2. **Once Quarry exists — a lint pass, advisory not a hard gate.** Because the rule has no "except when load-bearing" carve-out, the check is a clean mechanical target: flag any identifier-shaped token in a doc comment that resolves to a real symbol outside the current type/package (excluding the two exceptions above). No judgment about necessity is required — the rule is absolute, so a hit is always a violation.

Wiring into producer prompts: composed into every code-writing producer's stencil via a "Load these skills: ..." section (see `plan-card-format.md`), not left to model discretion to invoke.
