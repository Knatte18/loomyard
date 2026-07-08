# stencil

> **Status: Built.** A shared infrastructure leaf. Per the
> [documentation lifecycle](../overview.md#documentation-lifecycle), once built its mechanics may
> fold into the package header (like the other implementation-only libs) — this doc pins the contract
> agreed during the review-engine design.

`internal/stencil` fills **marker fields** in a markdown template and returns the rendered markdown.
It is the one mechanical thing every prompt-building call-site needs: *template + values → prompt*.

It is a **leaf, not a module** — no CLI, no engine, no domain knowledge — the same category as
`yamlengine` / `output` / `state`. It never learns "review", "phase", or "cluster"; it substitutes
markers. Callers own the templates and the values; stencil just fills them.

## The name

A **stencil** is a template with cut-out fields you fill to reproduce a pattern — exactly "markdown
with marker fields that get filled." The name deliberately avoids two collisions in this codebase:

- **`render`** — already `mux`'s layout sub-package (`internal/muxengine/render`, strands → layout).
- **`template`** — already means the config default in `configreg.ConfigTemplate()`.

`stencil` is "template" said with its own word, so neither meaning is overloaded.

## The contract

```go
// Fill renders a markdown template by substituting marker fields from values.
// It returns an error if any marker in the template has no value — an unfilled
// marker is never silently left blank.
func Fill(template []byte, values map[string]string) ([]byte, error)
```

- **Input:** a markdown template (bytes / an asset file's contents) and a set of named values.
- **Output:** the filled markdown, ready to hand to `shuttle.Run` as a prompt.
- **Marker syntax:** the pinned grammar is Go stdlib `text/template` (`text/`, never
  `html/template` — output must not be HTML-escaped): `{{.X}}` substitution, plus
  `{{if eq .Type "…"}}` equality conditionals for bulk-vs-tool-use / cluster-present /
  seeded-context-vs-safety-pass sections. Variadic `eq`, and the `and`/`or`/`not`/comparison
  operators, come free with `text/template`. A leading `<!-- … -->` comment on the template
  asset is stripped before parsing.

## The one load-bearing guarantee — fail on an unfilled marker

This is the reason the leaf exists beyond DRY. **An unfilled marker is a hard error, never a silent
blank.** A template whose `fasit` marker rendered empty would quietly neuter a review — and *fasit is
the load-bearing field of a review profile* (`{fasit, target} → verdict`, not `target → verdict`). A
shared renderer that refuses to emit a prompt with a hole in it turns that whole class of bug into a
loud, early failure instead of a silently-degraded review. Centralizing this guard is worth more than
the substitution itself, which is trivial.

The built scoping: every **top-level** absent-or-empty marker is collected and reported together,
sorted, in one error, and the template is never executed. A **branch-internal** reached-but-absent
marker is instead caught incrementally, one per call, via `missingkey=error` — this is not "every hole
in one error" for branch-internal markers, only for top-level ones. A caller-required marker (like
`fasit`/`target`) must therefore live at the template's top level, never inside a conditional branch.

## Consumers

- **`burler`** — the handler prompt and each cluster-reviewer prompt (the pre-assembled bulk blob is
  passed *as a value*, not read via tools — see the `internal/burlerengine` package documentation).
- **`perch`** — the progress-judge prompt.
- **`loom`** — the discussion / plan producer prompts (producers are prompts + profiles, not modules).
- **`hardener`** (DRAFT) — the round-agent prompt (`review-prompt-template.md`).

All four go through the same leaf; the templates live as `.md` assets, the profiles supply the values.

## Tests

Pure and table-driven, no substrate: fill cases, the **missing-marker → error** guarantee, conditional
sections present/absent, and idempotence (same template + values → same output). Own deep tests, like
every shared lib.
