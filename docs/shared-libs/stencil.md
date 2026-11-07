# stencil

> **Status: Built.** A shared infrastructure leaf. Per the [documentation lifecycle](../overview.md#documentation-lifecycle), once built its mechanics may fold into the package header, like the other implementation-only libs; this doc pins the contract.

`internal/stencil` fills **marker fields** in a markdown template and returns the rendered markdown.
It is the one mechanical thing every prompt-building call-site needs: *template + values → prompt*.

It is a **leaf, not a module**: no CLI, no engine, no domain knowledge.
It is the same category as `yamlengine`, `output` and `state`.
It never learns "review", "phase", or "cluster";
it substitutes markers.
Callers own the templates and the values;
stencil fills them.

## The name

A **stencil** is a template with cut-out fields you fill to reproduce a pattern.
That is "markdown with marker fields that get filled."
The name avoids two collisions in this codebase:

- **`render`** — already `reed`'s layout sub-package (`internal/reedengine/render`, strands → layout).
- **`template`** — already means a module's config default, as in `ConfigTemplate()`.

`stencil` is "template" said with its own word, so neither meaning is overloaded.

## The contract

```go
// Fill renders a markdown template by substituting {{.X}} markers from values.
// Every top-level marker absent from values or blank is collected and reported in one error;
// the template is never executed with unfilled top-level markers.
func Fill(template []byte, values map[string]string) ([]byte, error)

// FillOptional renders a template in which the names in optional are exempt from the unfilled-marker guarantee.
// Such a name renders as nothing when it is absent or blank.
func FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)
```

- **Input:** a markdown template (bytes / an asset file's contents) and a set of named values.
  `FillOptional` also takes a list of marker names exempt from the unfilled-marker guarantee.
- **Output:** the filled markdown, ready to hand to `shuttle.Run` as a prompt.
- **Marker syntax:** Go stdlib `text/template`, never `html/template`, because output must not be HTML-escaped.
  `{{.X}}` substitutes a value.
  `{{if eq .Type "…"}}` equality conditionals select sections, such as bulk-vs-tool-use or cluster-present.
  Variadic `eq` and the `and`/`or`/`not`/comparison operators come free with `text/template`.
  A leading `<!-- … -->` comment on the template asset is stripped before parsing.
- **`Fill` is defined as `FillOptional(template, values, nil)`.**
  There is one code path, so two implementations cannot drift apart;
  `Fill` supplies no optional names.

## The one load-bearing guarantee — fail on an unfilled marker

This is the reason the leaf exists beyond DRY.
**An unfilled marker is a hard error, never a silent blank.**
A template whose `fasit` marker rendered empty would quietly neuter a review.
`fasit` is the load-bearing field of a review profile: the profile maps `{fasit, target}` to a verdict, not `target` alone.
A shared renderer that refuses to emit a prompt with a hole in it makes that class of bug a loud, early failure.
Centralizing this guard is worth more than the substitution itself, which is trivial.

Every **top-level** absent-or-empty marker is collected and reported together, sorted, in one error,
and the template is never executed.
A **branch-internal** marker that is reached but *absent* is caught one per call, via `missingkey=error`.

A branch-internal marker *present* as `""` is a third case.
The batch check never sees it because it is not top-level.
`missingkey=error` never fires on it because a present empty-string value is a valid map entry.
Left alone, it would render silently blank, which is the risk of `{{if}}`/`{{range}}` conditionals.
`FillOptional` closes this case with a third check.
A simple `{{if .X}}` or `{{if eq .X "literal"}}` condition counts when `values` confidently evaluates it true.
Every non-optional marker inside that branch that is present as `""` or whitespace-only is reported.
It is folded into the same batch error as top-level offenders.
An absent or unresolvable condition is left to the `missingkey=error` path and never flagged by this check.
The check does not descend into `{{range}}`/`{{with}}`/`{{template}}`, and it does not resolve an `{{else}}` branch.
A caller-required marker (like `fasit`/`target`) should still live at the template's top level when practical,
since the branch-internal guarantee covers only the one shape above.

## The optional-marker exemption — `FillOptional`

A marker name passed in `FillOptional`'s `optional` argument is exempt from **both** of the guards above.
The top-level batch check no longer reports it as unfilled.
`missingkey=error` no longer fires on it at execution time, even when it is reached only via a branch.
A listed name that is absent from `values`, or present as `""` or whitespace only, renders as nothing.
It trips neither guard and leaks no whitespace into the output.
One shared `strings.TrimSpace` definition of "empty" governs both guards and both call paths.
A listed name present with a non-empty value renders that value;
the exemption changes only the absent-or-empty case.
A listed name that never appears in the template is a harmless no-op.

Optionality is a property of **the caller's argument list**, not of the template text:
a marker is optional because a specific Go call site passed its name in `optional`.
That is testable at the call site, unlike wrapping the field in a `{{if}}` conditional inside the markdown asset.
The same template can therefore be filled with a marker required by one caller and optional by another.
The first caller uses `Fill`, or `FillOptional` with that name omitted from `optional`.
The second uses `FillOptional` with that name listed.

## Consumers

Every prompt-building call site fills its prompt through this leaf, such as the reviewer, fixer and judge prompts of the engines and the producer prompts of the planning modules.
The templates live as `.md` assets, and the profiles supply the values.

`burler` passes each half's thin orchestrator as a value.
The instruction files that orchestrator names are written to a fresh `.lym`-anchored directory and read by the agent via tools.
See the `internal/burlerengine` package documentation.

## Tests

Pure and table-driven, no substrate.
The cases cover fills, the **missing-marker → error** guarantee and conditional sections present or absent.
They also pin idempotence: the same template and values give the same output.
Own deep tests, like every shared lib.
