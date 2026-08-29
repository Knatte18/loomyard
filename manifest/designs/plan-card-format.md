# Plan Card format — symbol-level, Quarry-ready

> **Status: implemented**, by the **planparser: Card-format migration to `Edits`/`Uses`** task (`manifest/roadmap.md`'s Done section is cleared regularly, not a durable record). `contracts/specs/loom-plan-spec.md` is the as-built format-4 contract this doc designed; see it, not this doc, for the pinned Card fields — both the spec and `contracts/stencils/loom/loom-template-plan.md` are rewritten to this design by that task. `manifest/designs/webster-parallel-execution.md` predates this doc and is stale; its reconciliation is the roadmap's job, not this doc's, owned by the Someday `webster: worktree-per-card parallel execution` item. `manifest/designs/quarry-plan-symbol-verification.md` was stale for the same reason and was reconciled against format 4 by the 2026-08-29 designs audit.
> Audited 2026-08-29 and kept rather than deleted, against the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle)'s default for an implemented design draft: this doc is a live reference, not a superseded one.
> `contracts/stencils/loom/loom-rubric-plan-review.md`, `contracts/stencils/loom/loom-rubric-webster-review.md`, `contracts/stencils/loom/loom-template-plan.md`, `contracts/recipes/loom-recipe.yaml`, `contracts/specs/loom-plan-spec.md`, and `internal/planparser`'s package comments all cite it by name for the Card model the spec's format only pins, and its own Verify model section is the sole home of the three-tier design `loom-plan-spec.md` records as designed-but-not-implemented.
> Delete it once that Verify model either ships into the spec or is abandoned, and the citing stencils are repointed.

## Card fields

```
Card:
  <TypeName>:                # Create | Edit | Delete | Rename | Move | Prosa | Custom
    - symbol-or-file, ...    # this card's own target(s)
  Uses:
    - symbol-or-file, ...    # read/depended on, not the target — omit if empty
  Intent: "..."               # what, and why — prose, can be multi-sentence
  ImpactSummary: "..."        # one line, required for Edit/Delete only
  Verify: "..."                # optional, rare — see Verify model
```

The type name is the key — no separate `Type:` field. A card's own list can hold symbols or file paths mixed together, distinguished by shape (a file path has an extension/separator, a symbol reference does not) and, where ambiguous, resolvable against ground truth (`go doc` for a symbol, file existence for a path). Omit any field with no content — no `Uses: []`, no `Uses: None`.

Symbol references are package-qualified short names (`shedrecipe.Lookup`), never file:line, never full import paths.

No `DependsOn`/`Produces` field. Dependency edges are derived, never authored: a card's `Uses` intersected against every other card's target list in the same plan is the dependency graph. A symbol no card in the plan touches needs no edge — its state never changes during the plan's execution.

## Card types

| Type | Target list holds | Mechanical check | `ImpactSummary` | Batchable? |
|---|---|---|---|---|
| Create | new symbol(s) | none — check nothing equivalent exists first | required | No — one judgment unit per card |
| Edit | existing symbol(s) | impact/blast-radius on the symbol being changed | required | No |
| Delete | existing symbol(s) OR whole file(s) | assert-no-callers (necessary, not sufficient) | required | Yes — independent targets only |
| Rename | existing symbol(s), `old -> new` pairs | AST-aware script + grep verify, never text/regex replace | not required — see below | Yes — independent symbols only |
| Move | existing symbol relocated to a file, OR a whole file relocated | `git mv` + import fixup; destination stated in `Intent`, not the list | not required | Yes |
| Prosa | file(s), no symbol target | none | not required | — |
| Custom | either | none — explicit escape hatch | as applicable | — |

A multi-label card composes this table's four columns as follows, one rule per column:

- **Target list holds** — per group, no composition: each label's own sub-bullets hold only that label's own kind of target, exactly as the table row states for that label alone.
- **Mechanical check** — the union across the card's own groups: every group's own mechanical check runs, each against that group's own targets, never against another group's targets.
  `Create`'s "none — check nothing equivalent exists first" cell is a real obligation that joins this union like any other, not a no-op that a multi-label card can skip past.
- **`ImpactSummary`** — required whenever any of the card's own groups is `Edit` or `Delete`, and stays exactly one per card even when several of its groups require it: it states the blast radius across every `Edit`/`Delete` group's targets together, never a separate summary per group.
- **Batchable?** — least permissive wins across the card's own groups: a card is batchable only when every one of its groups says `Yes`, and a single `No` group makes the whole card `No`.
  `Prosa`/`Custom`'s "—" is never a vote in this computation — it neither forces `No` nor grants `Yes`, so a `Prosa`/`Custom` group's presence is transparent to the other groups' own answer.

`Intent` vs. `ImpactSummary`: `Intent` is what/why, the card's main content. `ImpactSummary` is a separate, hard-capped one-line blast-radius conclusion ("3 callers, all local to billing package, no cross-module effects") — kept as its own field specifically so it stays terse; folding it into `Intent` lets it balloon into unbounded reasoning.

**Rename does not require `ImpactSummary`.** A correctly executed AST-aware rename is binary — every reference updates and the build/tests pass, or a leftover reference fails the build immediately. There is no graded blast-radius judgment to summarize. The same blind spot `assert-no-callers` has for Delete still applies: string/reflection-based references (a registry keyed by name, `getattr` in Python) survive a rename without failing the build. Rename must also rewrite the target symbol's own leading self-reference in its own doc comment (Go convention opens a doc comment with the symbol's name) — safe to do mechanically, since the card's own `old -> new` pair names exactly what changed; verify with the same grep pass.

## Verify model

Three tiers, matching this repo's own test-tier discipline (`internal/planparser`'s existing `Verify` fields are the V1 precedent this generalizes) — not three tiers invented for this format:

- **Tier1 (per card, automatic, no author action).** Tier1 tests are fast by construction (the Test Tier Purity Invariant's own discipline — no cwd resolution, no process spawn), so no "known-slow package" carve-out is needed. Scope: `go test`, restricted to the package(s) holding the card's own target symbol(s) plus every package holding a caller found via impact lookup (the same lookup that derives `Uses`-based dependency edges elsewhere in this doc). Fully mechanical — no author enumerates a file list, which is what made V1-style `verify:` lists grow long in practice.
- **Tier2 (plan-level, not per card).** Real git-against-remote tests (built via `internal/hubforge` — real repo creation, real clone) are genuinely slower; paying that cost once per plan, not once per card that happens to touch an affected package, is the point. Defaults to the existing plan-level `## verify:` integration suite (`contracts/specs/loom-plan-spec.md`'s model, unchanged) — the same gate the Concurrency section's post-merge `go build && go test` backstop already assumes. **Someday, not now:** a batch could eventually own its own tier2 verify instead of waiting for the whole plan — not designed here, deliberately deferred.
- **Tier3 (rare, explicit only, never automatic).** Tests that drive a real LLM — expensive in both wall-clock and tokens, and rare-to-nonexistent in this repo today. Never swept into an automatic per-card or per-plan gate under any circumstance. If a card genuinely needs one, that is exactly what the optional `Verify:` field is for — an explicit, deliberate, author-named exception, not something inferred.

The optional per-card `Verify:` field (V1's own mechanism, kept) exists for what tier1's automatic package-scoped run cannot catch on its own — a specific CLI smoke test, a targeted tier2 scenario, or (rare) a tier3 case. It stays genuinely optional and exceptional; the default, automatic tier1 run is what most cards rely on, which is what keeps `Verify:` from becoming the long, hand-maintained list it was in millhouse's own equivalent.

Granularity rule: one card per independently reviewable/testable unit, not one card per literal symbol. A symbol with no independent meaning or testability apart from another symbol in the same card (a private supporting type, a constructor inseparable from its type) is bundled into that other symbol's card. A symbol that is independently testable/reusable gets its own card even if one card happens to be its first consumer.

Delete vs. Edit: removing the last caller of a symbol and deleting the symbol are two cards, not one — a symbol can have N callers, each requiring its own Edit card, and only one final Delete card once all are gone.

Docstrings are not a separate card: a symbol's own doc comment is written/updated as part of whichever Create/Edit/Rename card touches that symbol, per `docs/code-comment-conventions.md`. `Prosa` is reserved for documentation with no single-symbol owner — design docs, `docs/overview.md`, module-level `doc.go` headers not triggered by a code change, repo-wide comment-convention sweeps.

## Docstring convention

See [code-comment-conventions.md](../../docs/code-comment-conventions.md) — the actual rule (Go only, for now). This doc only adds one Card-specific requirement: a Rename card's execution must also fix the target symbol's own self-referencing comment opening (see above), on top of whatever `docs/code-comment-conventions.md` requires generally.

Wiring: a "Load these skills: ..." section composed into every code-writing producer's stencil (same stencil-composition mechanism `bouncer.go` already uses) — not left to model discretion to invoke.

## Quarry integration — degraded mode is the default, not a fallback

Every mechanical check above must work without Quarry, because Quarry does not exist yet and the format must not be blocked on it:

- **Impact/refs**: `go doc <pkg> <Symbol>` for existence/definition, `grep -rn` scoped to the right package for call sites, manual read. Quarry later replaces this with `quarry impact <symbol>`, which must also return each caller's full enclosing function and its own doc comment, not just file:line (see `docs/code-comment-conventions.md`). The Card format and `ImpactSummary` output shape do not change either way.
- **Rename**: an AST-aware script (`go/ast`+`go/types` for Go, Roslyn `Renamer` for C#, `rope`/`libcst` for Python — model picks the library, but the script must not be text/regex-based), then a grep pass to confirm zero remaining old-name references and that the symbol's own doc comment was updated. Quarry later replaces the script-writing step with a mechanical rename primitive; the grep-verify step stays as the same backstop either way.
- **Delete**: manual grep for zero remaining callers, same caveat as today (`assert-no-callers`-equivalent is necessary, not sufficient — interface satisfaction, dispatch-table registration, and prose references are not caught).

No card type is defined in terms of Quarry's presence. When Quarry exists, it only makes an already-defined mechanical step faster and more reliable — it never changes what a card contains or how it's reviewed.

## Concurrency (worktree-per-card, not concurrent-forks-in-one-checkout)

The dependency graph derived from target lists vs. `Uses` determines what can execute in parallel: two cards with no shared symbol are independent. Each independently-executable card (or batch) runs in its own `git worktree`, spawned by Webster — not as concurrent forks sharing one checkout's git index, which `webster-parallel-execution.md` already rejected for the shared-index-lock race it produces. Separate worktrees each hold their own index; the race that doc describes does not apply to worktree-per-card.

Sequencing for cards that DO share a symbol: plan order settles it. If card 8 has `Uses: X` and card 5's target list includes `X`, and card 5 precedes card 8 in the plan, card 8 depends on X's state after card 5 lands — no further bookkeeping needed, since a sequentially-ordered plan has exactly one current state of any touched symbol at any point in its own sequence.

Correctness backstop: dependency-list completeness cannot be proven mechanically at plan time (same class of gap as `assert-no-callers` — catches what it can see, not what requires understanding intent). The real gate is post-merge: after any concurrently-executed batch's worktrees merge back, `go build ./... && go test ./...` (or the language's equivalent) against the merged result is what actually catches a missed dependency — a card whose plan-time `Uses` list was incomplete surfaces here as a build/test failure, not before.

## Open, not decided here

All three items below closed when the **planparser: Card-format migration to `Edits`/`Uses`** task landed:

- **Whether `Custom` needs its own mechanical check at all, or is purely an escape hatch by design.**
  Closed in the affirmative: `Custom` needs no type-specific mechanical check.
  It stays a principled, deliberate escape hatch — bound by every card-generic check (path well-formedness, field presence, and so on) but exempt from `path-missing` on its own targets and from the `Prosa` target-shape rule, exactly as documented and nothing more.
  A card that can name its targets under a typed group has, by definition, found a fit and is therefore not `Custom` — a card carrying a `Custom` group whose targets could instead be expressed as a multi-label combination is a defect, not a legitimate use of the escape hatch.
- **Whether `ImpactSummary` on Delete needs a structured shape beyond one line of prose.**
  Closed: it stays one line of prose, identical in shape to an `Edit` card's.
  A structured shape (e.g. an enumerated caller list) would need a caller enumeration the parser cannot produce without the symbol lookup this task deliberately excludes — see the shape classifier's own deviation from ground-truth resolution, above.
- **Reconciliation with `contracts/specs/loom-plan-spec.md`'s existing validator checks — which of those still apply, which are now redundant, which need rewriting.**
  Closed: the disposition table this task implemented lands on seventeen distinct check IDs, one row per ID, in `contracts/specs/loom-plan-spec.md`'s own Validation checks section — up from the spec's former 14-row list, which itself bundled two distinct IDs into its first row.
  Seventeen, not sixteen, because the multi-label grammar added its own new `card-custom-not-alone` row: a card carrying a `Custom` group alongside a differently-typed group is a defect the original one-label-per-card grammar had no need to check for.
