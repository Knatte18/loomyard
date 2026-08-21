# stencil

> **Status: Built.** A shared infrastructure leaf. Per the [documentation lifecycle](../overview.md#documentation-lifecycle), once built its mechanics may fold into the package header (like the other implementation-only libs) — this doc pins the contract agreed during the review-engine design.

`internal/stencil` fills **marker fields** in a markdown template and returns the rendered markdown.
It is the one mechanical thing every prompt-building call-site needs: *template + values → prompt*.

It is a **leaf, not a module** — no CLI, no engine, no domain knowledge — the same category as `yamlengine` / `output` / `state`.
It never learns "review", "phase", or "cluster";
it substitutes markers.
Callers own the templates and the values;
stencil just fills them.

## The name

A **stencil** is a template with cut-out fields you fill to reproduce a pattern — exactly "markdown with marker fields that get filled."
The name deliberately avoids two collisions in this codebase:

- **`render`** — already `reed`'s layout sub-package (`internal/reedengine/render`, strands → layout).
- **`template`** — already means the config default in `configreg.ConfigTemplate()`.

`stencil` is "template" said with its own word, so neither meaning is overloaded.

## The contract

```go
// Fill renders a markdown template by substituting marker fields from values.
// It returns an error if any marker in the template has no value — an unfilled
// marker is never silently left blank.
func Fill(template []byte, values map[string]string) ([]byte, error)

// FillOptional renders a markdown template exactly like Fill, except every name
// listed in optional is exempt from Fill's unfilled-top-level-marker guarantee.
func FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)
```

- **Input:** a markdown template (bytes / an asset file's contents), a set of named values, and (for `FillOptional`) a list of marker names exempt from the unfilled-marker guarantee.
- **Output:** the filled markdown, ready to hand to `shuttle.Run` as a prompt.
- **Marker syntax:** the pinned grammar is Go stdlib `text/template` (`text/`, never `html/template` — output must not be HTML-escaped): `{{.X}}` substitution, plus `{{if eq .Type "…"}}` equality conditionals for bulk-vs-tool-use / cluster-present / seeded-context-vs-safety-pass sections.
  Variadic `eq`,
  and the `and`/`or`/`not`/comparison operators, come free with `text/template`.
  A leading `<!-- … -->` comment on the template asset is stripped before parsing.
- **`Fill` is defined as `FillOptional(template, values, nil)`.**
  There is one code path, not two parallel implementations that could drift apart;
  `Fill` simply supplies no optional names.

## The one load-bearing guarantee — fail on an unfilled marker

This is the reason the leaf exists beyond DRY.
**An unfilled marker is a hard error, never a silent blank.**
A template whose `fasit` marker rendered empty would quietly neuter a review — and *fasit is the load-bearing field of a review profile* (`{fasit, target} → verdict`, not `target → verdict`).
A shared renderer that refuses to emit a prompt with a hole in it turns that whole class of bug into a loud, early failure instead of a silently-degraded review.
Centralizing this guard is worth more than the substitution itself, which is trivial.

The built scoping: every **top-level** absent-or-empty marker is collected and reported together, sorted, in one error,
and the template is never executed.
A **branch-internal** reached-but-*absent* marker is instead caught incrementally, one per call, via `missingkey=error` — this is not "every hole in one error" for branch-internal markers, only for top-level ones.

**A branch-internal marker *present* as `""` is a third case, closed 2026-08-15.** Neither guard above catches it on its own: it isn't top-level (so the batch check never sees it), and it isn't absent (so `missingkey=error` never fires — a present empty-string value is a valid map entry, not a missing key). Left alone, it would render silently blank — exactly the risk every producer template's own header comment warns "`{{if}}`/`{{range}}` conditionals" carry.
`FillOptional` closes this specific case with a third, additive check: for a simple `{{if .X}}` or `{{if eq .X "literal"}}` condition confidently evaluated true against `values` (an absent or unresolvable condition is left entirely to the existing `missingkey=error` path, never newly flagged), any non-optional marker inside that branch present as `""`/whitespace-only is folded into the same batch error as top-level offenders.
It does not descend into `{{range}}`/`{{with}}`/`{{template}}`, and it does not resolve an `{{else}}` branch — both remain exactly as unguarded as before this check existed.
A caller-required marker (like `fasit`/`target`) should still live at the template's top level when practical — the branch-internal guarantee only covers the one specific shape above, not every conditional a template could write.

## The optional-marker exemption — `FillOptional`

A marker name passed in `FillOptional`'s `optional` argument is exempt from **both** of the guards above: the top-level batch check no longer reports it as unfilled, and `missingkey=error` no longer fires on it at execution time even when it is reached only via a branch.
Concretely, a listed name that is absent from `values`, present as `""`, or present as a whitespace-only string all render as nothing rather than tripping either guard or leaking literal whitespace into the output — one shared `strings.TrimSpace` definition of "empty" governs both guards and both call paths.
A listed name that is present with a genuine non-empty value renders that value exactly as before;
the exemption only changes behaviour for the absent-or-empty case.
A listed name that never appears in the template at all is a harmless no-op.

Optionality is a property of **the caller's argument list**, not of the template text: a marker is optional because a specific Go call site passed its name in `optional`, which is a testable fact about that call site, unlike wrapping the same field in a `{{if}}` conditional inside the markdown asset.
The same template can therefore be filled once with a marker required (via `Fill`, or `FillOptional` with that name omitted from `optional`) and once with it optional (via `FillOptional` with that name listed), depending entirely on which caller is filling it.

## Consumers

- **`burler`** — the handler prompt and each cluster-reviewer prompt: the thin round orchestrator is passed *as a value*, while the three per-step instruction files it names are written to a fresh `.lyx`-anchored directory and read by the agent via tools (see the `internal/burlerengine` package documentation).
- **`treadle`** — the progress-judge prompt.
- **`loom`** — the discussion / plan producer prompts (producers are prompts + profiles, not modules).
- **`hardener`** (DRAFT) — the round-agent prompt (`review-prompt-template.md`).

All four go through the same leaf;
the templates live as `.md` assets, the profiles supply the values.

## Tests

Pure and table-driven, no substrate: fill cases, the **missing-marker → error** guarantee, conditional sections present/absent, and idempotence (same template + values → same output).
Own deep tests, like every shared lib.
