# stencil (design)

> **Status: Design — not built.** A shared infrastructure leaf. Per the
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
func Fill(template []byte, values map[string]any) ([]byte, error)
```

- **Input:** a markdown template (bytes / an asset file's contents) and a set of named values.
- **Output:** the filled markdown, ready to hand to `shuttle.Run` as a prompt.
- **Marker syntax:** an implementation choice — Go stdlib `text/template` (`{{.X}}`, and it gives
  conditional sections for free: `{{if .Cluster}}…{{end}}` for bulk-vs-tool-use / cluster-present /
  seeded-context-vs-safety-pass) is the likely backing; simple `<PLACEHOLDER>` substitution (mill's
  convention) is the alternative. Either way, the load-bearing guarantee below holds.

## The one load-bearing guarantee — fail on an unfilled marker

This is the reason the leaf exists beyond DRY. **An unfilled marker is a hard error, never a silent
blank.** A template whose `fasit` marker rendered empty would quietly neuter a review — and *fasit is
the load-bearing field of a review profile* (`{fasit, target} → verdict`, not `target → verdict`). A
shared renderer that refuses to emit a prompt with a hole in it turns that whole class of bug into a
loud, early failure instead of a silently-degraded review. Centralizing this guard is worth more than
the substitution itself, which is trivial.

## Consumers

- **`burler`** — the handler prompt and each cluster-reviewer prompt (the pre-assembled bulk blob is
  passed *as a value*, not read via tools — see [burler.md](../modules/burler.md)).
- **`perch`** — the progress-judge prompt.
- **`loom`** — the discussion / plan producer prompts (producers are prompts + profiles, not modules).
- **`hardener`** (DRAFT) — the round-agent prompt (`review-prompt-template.md`).

All four go through the same leaf; the templates live as `.md` assets, the profiles supply the values.

## Tests

Pure and table-driven, no substrate: fill cases, the **missing-marker → error** guarantee, conditional
sections present/absent, and idempotence (same template + values → same output). Own deep tests, like
every shared lib.
