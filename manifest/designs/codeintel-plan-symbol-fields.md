# codeintel-backed plan symbol fields — making the Planner's file-op enumeration deterministic

> **Status: Speculative, not scoped.** [plan-format-v3.md](../../docs/reference/plan-format-v3.md) already named this gap explicitly: the symbol fields (`creates-symbols`/`edits-symbols`/`reads-symbols`) were "deliberately omitted in v0, not just left optional... they depend on a working, planner-side-verified `codeintel`, which is deprioritized." Both blockers are now gone — `codeintel` shipped (V1, Go-only) and the loom Planner producer (`internal/loomengine/plan.go` + `plan-template.md`) also shipped, with no review logic of its own standing in the way of a prompt-level change. This doc names the idea and lays out the design space; it does not commit to an approach. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever picked up the durable parts fold into the owning doc (`plan-format-v3.md` and/or `internal/loomengine`'s package doc) when it lands; if abandoned, this file is simply deleted.

## The problem this responds to

Today, `plan-template.md`'s Step 2 ("Explore the codebase") tells the Planner agent to read the relevant parts of the codebase before writing a card's `Edits:`/`Context:`/`Creates:`/`Deletes:`/`Moves:` fields — in practice this means grep-and-read exploration, paid for in tokens and wall-clock, for every card that touches existing code. Two failure modes follow directly from that: grepping a symbol's name returns false positives when an unrelated symbol elsewhere in the repo happens to share the name, and it structurally cannot prove a call reached only through an interface — the exact case `internal/codeintelengine`'s own CLI help text calls out ("including calls reached only through an interface, which no amount of grepping can prove"). A card whose `Edits:` silently omits a real caller is a plan defect no existing plan-format-v3 validator check can catch, because every current check (`all-files-touched-mismatch` included) only verifies internal consistency between the overview and the cards — never consistency between a card's claims and the actual code.

## Two integration points, not yet chosen between

**(a) Prompt-level guidance only — no schema change.** Add an instruction to `plan-template.md`'s Step 2 telling the Planner to run `lyx codeintel refs <symbol>` (or `refs <file>:<line>:<col>` for an ambiguous name) instead of grepping, for any card that renames, deletes, or edits call sites of an *existing* Go symbol — never for a symbol the plan itself creates, which does not exist yet for codeintel to find. Optionally also point a `Deletes:`/`Moves:` card's own `verify:` field at `lyx codeintel assert-no-callers <old-symbol> --except <new/location.go>` (shipped in `internal/codeintelcli`, see below) as a mechanical post-hoc check that no stale reference survived. This is the small, already-buildable slice: it touches only `plan-template.md`'s prose, nothing in `internal/planparser` or `internal/loomengine/plan.go`, and ships the moment someone is willing to go through the same adversarial prompt-wording review `plan-template.md`'s other instructions already went through (see its module's crucible review history — ambiguous wording in this specific template has been a real, live bug more than once).

**(b) The originally-envisioned schema fields.** `plan-format-v3.md` names `creates-symbols`/`edits-symbols`/`reads-symbols` as the deferred fields themselves — a card would declare symbols, not just files, and something (`internal/planparser`'s `Validate`, most likely) would cross-check those declarations against `codeintel` mechanically, turning the "planner missed a caller" failure mode into a hard validation error instead of a silent gap. This is the fuller original vision and the one `internal/websterengine`'s dead DAG scheduler seam is waiting on (see Relationship table below) — but it means real schema, parser, and validator work in `internal/planparser`, not just prompt wording.

These are not mutually exclusive — (a) could ship first as a cheap, low-risk instruction change, with (b) picked up later only if (a)'s measured effect on plan quality/token cost justifies the schema investment. No decision is made here about which to build, or whether to build both.

## What already exists to build on

- `internal/codeintelengine` — the LSP-backed engine (`References`, `Definition`, `Symbol`), Go-only in V1, exposed both as an in-process Go API and as `lyx codeintel refs|definition|symbol`.
- `lyx codeintel assert-no-callers <symbol|file:line:col> [--except <path>]...` (`internal/codeintelcli`) — a CI-shaped gate built on the same engine calls: fails if any reference to a symbol survives outside its declaration and an allowed list. Built for exactly the "does this Deletes:/Moves: card's target symbol still have callers" question a card's `verify:` field would want to ask mechanically.

Neither of these required any change to ship — they exist independently of this idea and were available before this doc was written.

## Relationship to `codeintel` and `webster`'s DAG scheduler — dependent, not overlapping

| Item | Answers | Depends on |
|---|---|---|
| `codeintel` | "What exactly references/defines this symbol, right now?" | Nothing further (shipped) |
| symbol fields (this) | "Does this card's declared file-op list match what actually references the symbol?" | `codeintel` (shipped) |
| `webster: parallel card execution`'s DAG scheduler | "Which cards can run concurrently without a real dependency edge between them?" | Symbol fields (this) — `internal/websterengine`'s scheduler runs strictly in declared order today specifically because the symbol-derived edges it would need don't exist yet |

Picking up this idea is a prerequisite for the parallel-execution item ever becoming buildable, not just a nice-to-have alongside it — see [webster-parallel-execution.md](webster-parallel-execution.md)'s own "Relationship to codeintel" section, which already names structured impact lookup as the retired `websterv2.md` draft's Part B.

## Open questions (genuinely unscoped)

- **(a) vs. (b), or both, and in what order.** No measurement exists yet of how much (a) alone actually helps — token/time savings, or defect-catch rate — before deciding whether (b)'s schema work is justified.
- **Advisory vs. hard-fail.** If symbol-derived checking ever becomes a validator check (whether via prompt convention or schema), does a mismatch halt plan approval outright, or just surface as a warning the human review gate weighs? `plan-format-v3.md`'s existing 14 checks are all hard-fail; this would be the first check whose ground truth is "what the code actually does" rather than "is the plan internally consistent."
- **Symbol granularity.** A renamed function is a clean case; a renamed type used as an embedded field, or a package-level constant referenced only via a generated file, are messier — not explored here.
- **Language scope.** `codeintel` V1 is Go-only; this idea inherits that limit until `codeintel` grows the other planned languages.

## Related

- [plan-format-v3.md](../../docs/reference/plan-format-v3.md) — names the deferred symbol fields directly; the schema option (b) would extend it.
- [webster-parallel-execution.md](webster-parallel-execution.md) — the item this one is a prerequisite for.
- `internal/codeintelengine`'s package documentation — the engine both integration options build on.
- `internal/loomengine`'s package documentation — the Planner producer (`plan.go` + `plan-template.md`) option (a) would edit.
