# Code comment conventions — Go only for now

> **Status: implemented; a durable convention doc, not a module-design draft.**
> It is the standing rationale for a cross-cutting rule this repository's own code follows,
> so the [documentation lifecycle](overview.md#documentation-lifecycle) does not treat it as a deletion candidate.
> The operative rule lives in the shared `scribe` plugin's `code-quality` and `golang-comments` skills, which an agent loads and follows.
> The plugin is [`Knatte18/scribe`](https://github.com/Knatte18/scribe), under `plugins/scribe/skills/`.
> This document is the "why" behind the rule, not the rule text.
> C#/Python versions are deferred until Go is proven.

## The rule

The canonical text is the Comments section of the `scribe:code-quality` skill in `Knatte18/scribe`.
It holds the two exceptions and the information-triage list.
In short: a doc comment states only what its symbol does and why it exists, as a standalone contract;
it never refers to, compares itself with, or depends on any other named symbol.

## Why this is stronger than a staleness rule

The deeper problem is not that a cross-reference can go stale.
The problem is that describing a symbol by what it uses, or by how it differs from another, is implementation detail.
Implementation detail does not belong in a doc comment at all, whether or not the reference stays accurate.

A doc comment that needs outside context to be readable is a defect, whether or not it goes stale:
the symbol underneath it is not self-contained.

The skill states the rule; this document carries the rationale the skill does not.

## What replaces the lost context: query, not prose

The relationship this rule refuses to hardcode into text is answered on demand instead.
`quarry impact <symbol>` (and `refs`) must return a result for every caller.
Each result carries the file:line, the caller's full enclosing function or method, **and its own doc comment**.
"Who depends on X, and why" is answered by running the query and reading the caller's own self-contained comment;
X's comment never names the caller, and the caller's comment never names X.

The `impact` and `refs` verbs are not implemented; this section documents the intended mechanism.

## Enforcement

Enforcement has two tiers:

1. **Review discipline**, carried by the `code-quality`/`golang-comments` skills above.
   No mechanical check of the cross-reference rule exists.
2. **Once `quarry impact` and `refs` exist — a lint pass, advisory not a hard gate.**
   Because the rule has no "except when load-bearing" carve-out, the check is a clean mechanical target:
   flag any identifier-shaped token in a doc comment that resolves to a symbol outside the current type or package.
   The check excludes the two exceptions in `code-quality`'s Comments section: self-reference and stdlib/language contracts.
   No judgment about necessity is required; the rule is absolute, so a hit is always a violation.

Loading the skills does not depend on model discretion, and no stencil asks an agent to load a skill.
The spawning module names the skills on the launch spec, and lym delivers them.
