# quarry-backed plan symbol verification — checking a card's symbol claims against the code

> **Status: Speculative, not scoped.** [loom-plan-spec.md](../../contracts/specs/loom-plan-spec.md) v0 named this gap as three deferred fields, `creates-symbols`/`edits-symbols`/`reads-symbols`, "deliberately omitted in v0, not just left optional... they depend on a working, planner-side-verified `quarry`, which is deprioritized."
> **That framing is superseded and the quoted passage is gone from the spec:** format 4's shape classifier admits a symbol anywhere a path may go, so a card already declares symbols in its ordinary target and `Uses:` lists, and no separate fields are needed or wanted.
> What survives is the other half — *verification*. A path-shaped entry gets the `path-missing` existence check; a symbol-shaped entry is checked against nothing at all, which is precisely the gap `quarry` would close.
> Both original blockers are gone — `quarry` shipped (V1, Go-only, since ported out of this repo into its own standalone module) and the loom Planner producer (`internal/loomengine/plan.go` + `contracts/stencils/loom/loom-template-plan.md`) also shipped, with no review logic of its own blocking a prompt-level change. This doc names the idea and lays out the design space; it does not commit to an approach. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever picked up the durable parts fold into the owning doc (`loom-plan-spec.md` and/or `internal/loomengine`'s package doc) when it lands; if abandoned, this file is simply deleted.
> Renamed from `scout-plan-symbol-fields.md` by the 2026-08-29 designs audit, in two steps: `fields` became `verification` because format 4 already ships the fields, and `scout` is no longer a thing that exists in this repo — the tool was extracted into the standalone `quarry` module — so only the historical `scout-vs-grep benchmark` keeps the old name below, as the proper name of a benchmark run when the tool was still `lyx scout`.

## The problem this responds to

Today, `contracts/stencils/loom/loom-template-plan.md`'s Step 2 ("Explore the codebase") tells the Planner agent to read the relevant parts of the codebase before writing a card's target groups and `**Uses:**` list — in practice this means grep-and-read exploration, paid for in tokens and wall-clock, for every card that touches existing code.
Two failure modes follow: grepping a symbol's name returns false positives when an unrelated symbol elsewhere in the repo happens to share the name,
and it structurally cannot prove a call reached only through an interface — the exact case `quarry`'s own CLI help text calls out ("including calls reached only through an interface, which no amount of grepping can prove").
A card whose `Edits:` silently omits a real caller is a plan defect no existing plan-format validator check can catch, because every current check (`all-files-touched-mismatch` included) only verifies internal consistency between the overview and the cards — never consistency between a card's claims and the actual code.

## Two integration points — benchmarked, not just theorized

**(a) Prompt-level guidance only — no schema change.**
Add an instruction to `contracts/stencils/loom/loom-template-plan.md`'s Step 2 telling the Planner to run `quarry refs <symbol>` (or `refs <file>:<line>:<col>` for an ambiguous name) instead of grepping, for any card that renames, deletes, or edits call sites of an *existing* Go symbol — never for a symbol the plan itself creates, which does not exist yet for quarry to find.
Optionally also point a `Deletes:`/`Moves:` card's own `verify:` field at `quarry assert-no-callers <old-symbol> --except <new/location.go>` (see below) as a mechanical post-hoc check that no stale reference survived.
This is the small, buildable slice: it touches only `contracts/stencils/loom/loom-template-plan.md`'s prose, nothing in `internal/planparser` or `internal/loomengine/plan.go`.

**(a) is empirically weakened, not just unproven.** The [scout-vs-grep benchmark](https://github.com/Knatte18/quarry) — run before this tool was quarry's own external module, when it still lived in this repo as `lyx scout` — benchmarked exactly this shape of decision — an agent free to choose whether and how to use the tool versus an agent restricted to grep — across three hard symbol-resolution tasks in this repo.
The result was no dramatic, universal win, and it got weaker, not stronger, as the tasks got harder: one clean win for the tool (Task 2), one wash (Task 1, grep-only was actually faster and cheaper), and one clear loss (Task 3, grep won on all three metrics).
Combined across all three tasks, the tool's wall-clock advantage was ≈0% and it used *more* tool calls in aggregate than grep, not fewer; only the token count still favored it (-16%).
Worse than the win/loss tally itself: Task 3 exposed that `refs`' `"resolution":"complete"` trust marker can be present on a response that is majority cross-package noise (gopls resolves interface methods structurally, workspace-wide, not scoped to the interface actually queried) — an agent choosing to trust that marker at face value gets a wrong answer, exactly the re-verification the marker exists to make unnecessary.
An LLM given the *option* to call the tool still has to exercise judgment about when the tool's output can be trusted as-is versus needs cross-checking — the benchmark shows that judgment call itself doesn't reliably pay for itself.
This does not fully rule out (a) (single run, n=1 per cell, three hand-picked tasks — see that benchmark's own caveats), but it removes any presumption that giving the Planner tool access would obviously help, and no measurement has been done of (a) specifically wired into `contracts/stencils/loom/loom-template-plan.md` rather than a bare subagent.

**(b) Mechanical verification of the declarations already there.** A card already declares symbols; what is missing is something (`internal/planparser`'s `Validate`, most likely) cross-checking those declarations against `quarry`, turning the "planner missed a caller" failure mode into a hard validation error instead of a silent gap.
The original v0 framing put this behind three new schema fields, but format 4 removed that prerequisite — so this is now validator work in `internal/planparser`, not schema work.
This is the half `internal/websterengine`'s dead DAG scheduler seam is waiting on (see Relationship table below).

**(b) is the recommended direction if this is ever picked up, precisely because of what the benchmark found.** The failure modes in the scout-vs-grep benchmark — imprecise workspace-wide interface resolution, a trust marker that overpromises — are LLM-facing problems: they matter only when an agent has to decide whether to believe the tool's output.
Deterministic Go code calling `quarry`'s in-process API to fill or validate a schema field never faces that decision — it can apply the same scoping/filtering logic (e.g. the `--within <dir>` flag added after this benchmark) correctly and identically every time, and a validator either matches or hard-fails, with no judgment call to get wrong.
(a) is not recommended for further investment on the current evidence;
picking this idea up at all should start from (b).

## What already exists to build on

- `quarry` — the LSP-backed engine (`References`, `Definition`, `Symbol`), Go-only in V1, exposed both as an in-process Go API and as `quarry refs|definition|symbol` (a standalone module, `github.com/Knatte18/quarry`, not part of this repo).
- `quarry assert-no-callers <symbol|file:line:col> [--except <path>]...` — a CI-shaped gate built on the same engine calls: fails if any reference to a symbol survives outside its declaration and an allowed list. Built for exactly the "does this Deletes:/Moves: card's target symbol still have callers" question a card's `verify:` field would want to ask mechanically.

Neither of these required any change to ship — they exist independently of this idea and were available before this doc was written.

## Relationship to `quarry` and `webster`'s DAG scheduler — dependent, not overlapping

| Item | Answers | Depends on |
|---|---|---|
| `quarry` | "What exactly references/defines this symbol, right now?" | Nothing further (shipped) |
| symbol verification (this) | "Does this card's declared target list match what actually references the symbol?" | `quarry` (shipped) |
| `webster: worktree-per-card parallel execution`'s DAG scheduler | "Which cards can run concurrently without a real dependency edge between them?" | Symbol fields (this) — `internal/websterengine`'s scheduler runs strictly in declared order today specifically because the symbol-derived edges it would need don't exist yet |

Picking up this idea is a prerequisite for the parallel-execution item ever becoming buildable, not just a nice-to-have alongside it — see [webster-parallel-execution.md](webster-parallel-execution.md)'s own "Relationship to quarry" section, which already names structured impact lookup as the retired `websterv2.md` draft's Part B.

## The free-text half — `quarry literals`, folded in here rather than standalone

`quarry refs`' symbol resolution (LSP/`gopls`-backed) structurally cannot find a bare string literal that happens to share a constant's value — a literal has no binding to the constant to the compiler, so `refs` correctly returns nothing for it, not incompletely.
A companion capability, `quarry literals <value>` (walk `go/parser`'s AST for `*ast.BasicLit` string nodes matching a value — no LSP server, stdlib-only), was proposed independently to close that half.
It is folded into this doc rather than tracked as its own item: it answers the same kind of question (does this card's declared file-op list match what the code actually contains) for values instead of symbols, and the same (a)-vs-(b) conclusion above applies to it directly — it is only worth building as a mechanical field-filler/validator invoked by Go code, not as a tool a Planner LLM chooses to call, and it should not be scoped ahead of (b) itself being picked up.
`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` is a narrow, hardcoded existing instance of the same idea (a fixed-token ban-check, not a general query, and it excludes `*_test.go`) — evidence this pattern is already load-bearing in this repo, just not generalized.
Go-only, same as `quarry` V1; a lexer/AST approach cannot generalize by swapping servers the way the LSP path can, so multi-language support would mean a new parser per language (e.g. via tree-sitter), which is out of scope for both this doc and the original proposal.

## Open questions (genuinely unscoped)

- **(a) vs. (b), or both, and in what order.**
  The scout-vs-grep benchmark weakens the case for (a) specifically — see above — but has not measured (a) wired into `contracts/stencils/loom/loom-template-plan.md` itself, only bare subagents with/without tool access.
- **Advisory vs. hard-fail.**
  If symbol-derived checking ever becomes a validator check (whether via prompt convention or schema), does a mismatch halt plan approval outright, or just surface as a warning the human review gate weighs? `loom-plan-spec.md`'s existing 17 checks are all hard-fail;
  this would be the first check whose ground truth is "what the code actually does" rather than "is the plan internally consistent."
- **Symbol granularity.**
  A renamed function is a clean case;
  a renamed type used as an embedded field,
  or a package-level constant referenced only via a generated file, are messier — not explored here.
- **Language scope.** `quarry` V1 is Go-only;
  this idea inherits that limit until `quarry` grows the other planned languages.

## Related

- [loom-plan-spec.md](../../contracts/specs/loom-plan-spec.md) — the format-4 contract whose shape classifier already admits symbol targets, and whose `path-missing` check has no symbol-side counterpart;
  option (b) would add that counterpart.
- [webster-parallel-execution.md](webster-parallel-execution.md) — the item this one is a prerequisite for.
- [quarry](https://github.com/Knatte18/quarry) — the standalone module carrying the scout-vs-grep benchmark this doc's (a)-vs-(b) recommendation is grounded in.
- [review-finding-classification.md](review-finding-classification.md) — the discussion-review proposal that raised the free-text/`quarry literals` question this doc folds in above.
- [quarry](https://github.com/Knatte18/quarry) — the external module carrying the engine both integration options build on.
- `internal/loomengine`'s package documentation — the Planner producer (`plan.go` + `contracts/stencils/loom/loom-template-plan.md`) option (a) would edit.
