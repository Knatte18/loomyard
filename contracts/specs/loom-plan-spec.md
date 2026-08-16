# Loom plan-spec — flat card list

> **Status: Contract — pinned.** This doc pins **plan-format**: the flat card-list plan schema `Plan-Write` produces, which webster (`internal/websterengine`, via its sole parser `internal/planparser`) consumes. This is `internal/planparser`'s own as-built contract — the fourteen checks below are already implemented, not a future spec — kept as a durable Go-to-Go reference doc under `contracts/specs/`, not deleted on landing. The LLM-facing subset of this format — what `Plan-Write` itself must write — is pinned separately in the producer's own stencil, `contracts/stencils/loom/loom-template-plan.md`, so the agent's prompt never duplicates this file and the two cannot drift from being the same doc.

## Producer and contract

This format is produced by `Plan-Write` (stencil: `contracts/stencils/loom/loom-template-plan.md`).
It is validated by `Plan-Validate`/`internal/planparser` — see [Validation checks](#validation-checks-as-implemented-by-internalplanparser) below, this file's own validation-checks section.
It is reviewed by `Plan-Review`.

- **Output** — the shape below, per [Card fields and order](#card-fields-and-order).
  Consumed as Input by `Batchifier` and `Webster` (via `internal/planparser`, webster's sole parser).

## What a card is

The smallest change that:

1. **Compiles/builds on its own** — `go build ./...` succeeds immediately after the card's commit.
   No broken syntax, no reference to a symbol that doesn't yet exist.
2. **Is independently committable** — a meaningful, revertible git commit on its own.
3. **Bundles its own test, when it introduces new behavior** (implementation + `_test.go` in the same card).
   Pure refactors/renames may rely on existing tests instead.

**Key insight,
and the reason the schema is shaped this way:** criterion 1 is not just a post-hoc check — it's *why the dependency graph exists at all*.
A card that references a symbol which doesn't exist yet cannot compile, so it structurally cannot be valid until whatever card creates that symbol has already landed.
The DAG is a **consequence** of the compile-validity requirement, not a separate constraint bolted on top.

## Batch is gone / the card is the unit

**Batching is a step outside the plan schema, not a plan-schema concept.**
The plan's unit is always the individual **card** — the smallest, most precise, independently verifiable unit.
Any later grouping of cards (e.g. by webster, for read-cost reasons — same file/module, per the cards' declared file-op fields) is a later, measured decision made outside the plan format, not something the plan format needs to express or `Plan-Write` needs to decide.

There is no batch-level "declared ownership" `## Scope` concept.
A card's own typed file-op fields (`Context:`/`Edits:`/`Creates:`/ `Deletes:`/`Moves:`) *are* its declared footprint;
there is no wider unit left to declare a footprint for.

## Plan vs. schedule

The flat card list is the **plan** (a DAG of intent: what depends on what).
It is not itself an execution order.
Whoever executes the plan (webster today, or a hypothetical future parallel executor — see the roadmap's Someday list) decides *how* to turn the DAG into an actual run — sequential-in-declared-order today, potentially wave-based parallel execution for some future version.
**The plan format should not need to change if that execution-policy decision changes later.**

## On-disk layout

```
_lyx/plan/
  00-overview.md       # frontmatter + Card Index + task framing + optional plan-level sections
  NN-<card-slug>.md    # one file per card (NN = zero-padded card order)
  ...
```

`00-overview.md` frontmatter carries **scalar-only** keys:

```yaml
format: 3
approved: true
root: <optional worktree-relative dir>   # optional; see Card path resolution below
```

The body carries a short task-framing paragraph, an ordered **Card Index** whose entries read `N — <card-slug> — <one-line intent>`,
and the optional plan-level body sections `## Shared Decisions`, `## Rename mechanic`, and `## verify:`.

`NN-<card-slug>.md` — one file per card (`NN` = zero-padded card order);
the file *is* the card.
Card Index ↔ card files are cross-checked mechanically (the `index-file-mismatch` check, below): numbering, slugs, no gaps, no orphaned file on disk.

## Card fields and order

Each card lives in its own file,
and the file's content is, in this order:

1. **Title heading** — `# Card N — <name>`, where `N` is the card's own flat number (see [Numbering and commit subject](#numbering-and-commit-subject) below).
2. **`**What:**`** — the instruction: the change to make, concretely (prose, may span multiple lines until the next field label). plan-format keeps lyx's own established `What:` name.
3. **The five typed file-op fields, all required, in this order:** `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`.
   Never omitted — a field with nothing to declare carries the literal `none` on its own label line:

   ```markdown
   **Context:** none
   ```

   A non-`none` field's value is one or more indented sub-bullets below the label line, each a single backtick-wrapped path, no commentary, no line-range suffix, no comma-separated inline list:

   ```markdown
   **Edits:**
   - `internal/boardcli/list.go`
   ```

   A `Moves:` sub-bullet instead carries a two-path pair, ASCII ` -> ` arrow, both sides backtick-wrapped:

   ```markdown
   **Moves:**
   - `internal/boardengine/rows.go` -> `internal/boardengine/rowsjson.go`
   ```

   The `card-missing-field` check flags a card missing any of the five (or `What:`/ `Depends-on:`) — `none` sentinels are silent-degradation-proof exactly because an omitted field is mechanically indistinguishable from a forgotten one otherwise;
   a forgotten `Moves:` in particular would silently degrade into an unstructured create+delete pair, the exact failure this format exists to prevent.
4. **`**Depends-on:**`** — new required field, placed immediately after `Moves:`.
   See [Depends-on](#depends-on) below.
5. Optionally, **`**Commit:**`** and **`**verify:**`** — see [Numbering and commit subject](#numbering-and-commit-subject) and [verify model](#verify-model) below.

**`Context:` is advisory, not a strict allowlist.** `Context:` names files the implementer is expected to *read but not change* — the implementer may read beyond it when the plan under-specifies something.
Files in `Edits:` are implicitly read and are never repeated in `Context:` for the same card — that repetition is itself a `card-field-overlap` finding.

**Fields are mutually exclusive within one card.**
The same path appearing in two of a single card's five fields (or as a `Moves:` endpoint alongside another field) is a contradiction — is the file being edited, or moved, or deleted? — flagged by the `card-field-overlap` check.
This is strictly **per-card**: across two cards of the same plan, `Creates:` in an earlier card followed by `Edits:` of the same path in a later card is legitimate sequencing,
and the same path repeated across multiple cards' `Edits:` is entirely normal.

## Depends-on

`**Depends-on:**` is a **new required field**, placed after `Moves:`.
Its value is a list of card ids (plain card numbers `N`) or the literal `none`.
It references only other cards in the **same plan** — never a claim about external code.

**Why it is safe to include in v0, unlike the symbol fields:** it carries no hallucination risk — it only references other cards within the same plan, written by the same planner in the same session, never a claim about external code that could turn out to be wrong.
Three reasons to include it now:

1. Human-readable context at escalation time (if card 5 fails, is card 6 known to depend on it?).
2. Forward-compatible input for a future DAG mechanism (a cross-check layer once scout-derived edges exist, analogous to how `SHAExists` cross-checks a stored git reference — see [`internal/fabricengine`](../../internal/fabricengine/doc.go)).
3. **A cheap, mechanical, pre-review order-validation gate:** it powers the `depends-on-order` check — a card whose `Depends-on:` names a *later* card in the declared order, names itself, or names an id referencing no existing card is flagged before any LLM-based review runs, at zero cost.

## Card path resolution: `root:` and `//`

`00-overview.md`'s frontmatter may carry an optional **plan-level** `root: <worktree-relative-dir>`.
When set, every card file-op path in the plan — all five typed fields, both sides of every `Moves:` pair — resolves as `<root>/<path>` **unless** the path starts with `//`, which is *always* worktree-root-relative (root set or not — one rule, no special cases): that is how a card names a file outside the shared root, e.g. `//cmd/lyx/main.go`.
This is purely a token-economy shorthand for a plan whose cards repeat the same directory prefix over and over.
The degenerate `root: "."` case (the worktree root itself) resolves a card path to the raw path unchanged, rather than the unclean `"./<raw>"` a literal string join would produce.

The parser normalizes every card path to a plain worktree-relative, forward-slash path exactly once, at parse time — the validator and any future consumer never see `root:` or `//` again, only normalized paths.
A single-`/` prefix or a `..` segment in a card path is malformed and is flagged by the `card-path-malformed` check.

## Moves and the Rename mechanic

A `Moves:` sub-bullet declares a rename: `` `old/path` -> `new/path` `` (backtick-wrapped paths on both sides, ASCII ` -> ` arrow, exactly the same grammar as any other field's path bullets, extended to a pair).
A path appearing as a `Moves:` endpoint must not also appear in the same plan's `Creates:`/`Deletes:` anywhere — that would be two contradictory instructions for the same file, flagged by `move-redundant`.

**Rename-plus-extraction is one `Moves:` pair plus a separate `Creates:` entry**: when a rename also splits new content out of the relocated file, the relocation itself is still exactly one `Moves:` pair (the file that moved),
and the newly-split-out file is a plain `Creates:` entry in that same card or another — never folded into the `Moves:` pair itself.

**`## Rename mechanic` is a plan-level section**, one section in `00-overview.md`, **required when any card in the plan has a non-empty `Moves:`** — the `move-mechanic-missing` check (plan-level) flags a plan that declares a rename but omits it.
The section's text is CANONICAL — reproduce it verbatim (adjusted only for the specific paths involved):

```markdown
## Rename mechanic

1. Run `git mv <old> <new>` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. Use `Creates:` only for genuinely new files, never for the relocated file itself.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.
```

This is the repo's own `git mv` + surgical-edits convention made declarable in a plan and mechanically checkable, rather than an unstated expectation an implementer might miss.

## Numbering and commit subject

Cards are numbered flat **`N` (1..N)** across the whole plan — no batch-scoped restart, no `NN.C` compound numbering.
The per-card file prefix `NN` (zero-padded) must equal the heading `N`.

The **default commit subject is `N: <name>`** — the card heading's `<name>`;
there is no separate `<short what>` seed.
An explicit `**Commit:**` overrides the default but must start with the card's own `N: ` prefix — the `commit-subject-mismatch` check enforces this, because a pinned message that breaks the `N:` shape would corrupt the git-log resume trail the numbering scheme exists to give.

Commit-per-card is the **resume mechanism**: a fresh session sees from `git log` exactly which card the previous session reached,
and a half-done card is resumed by discarding uncommitted changes and restarting that card.

## verify model

`verify:` is **optional per-card** — a cheap, targeted check where it is useful — plus an **optional plan-level integration verify** that lives as a **`## verify:` body section in `00-overview.md`** (the `00-overview.md` frontmatter itself stays scalar-only: `format`/`approved`/`root`).

There is **no mandatory per-batch/per-card verify gate** — batch is gone, and per-card `verify:` is no longer required either.
The build+unit-test gate is implicit in the card definition itself (criterion 1 of [What a card is](#what-a-card-is): every card compiles/builds on its own) and is run by the consumer (webster) after each card.
The plan-level `## verify:` is the single integration suite run once at the end of the plan, not per card.

## Deferred / forward-compat

The symbol fields — `creates-symbols`/`edits-symbols`/`reads-symbols` — are **deliberately omitted in v0**, not just left optional.
They depend on a working, planner-side-verified `scout`, which is deprioritized (see the roadmap's Someday list).
Adding them now as unused optional fields would create confusion later;
better to add them explicitly once `scout` is actually ready.
See [`internal/scoutengine`](../../internal/scoutengine/doc.go) for what they'd depend on.

**The derived `changes-files` union** — the union of the typed file-op fields (`Edits:` ∪ `Creates:` ∪ `Deletes:` ∪ both `Moves:` endpoints) — is the artifact webster's future contract-verification compares actual changed files against (a fork reports `OK, SHA <x>` or a deviation note;
a file-list mismatch against `changes-files` is always informational, never blocking on its own).
See `internal/websterengine`'s package documentation for the verification semantics.

The detailed continuous-DAG-update / symbol-matching / SCC-merging **scheduling** design is summarized in `internal/websterengine`'s package documentation ("Declared order now, a dead DAG seam for later") — v0 runs strictly in declared order;
the eventual DAG scheduler waits on scout-backed symbol fields.

A parked, more aggressive parallel-execution idea also exists — see [../../manifest/designs/webster-parallel-execution.md](../../manifest/designs/webster-parallel-execution.md).

## Validation checks (as implemented by `internal/planparser`)

Machine checks this format is designed to support, in this fixed order:

1. `format-unrecognized` / `plan-unapproved` — `format:` recognized, `approved: true`;
   else refuse to run.
2. `index-file-mismatch` — Card Index ↔ card files consistent (numbering, slugs, no gaps, no orphaned file on disk).
   This check covers the card count because there is no separate `(C cards)` segment to cross-check;
   the index itself IS the card list.
3. `card-path-malformed` — every card path, once normalized (both `Moves:` sides included, `root:`/`//` resolution applied), is non-empty, relative, clean, and free of `..` escapes.
4. `move-format` — every non-`none` `Moves:` sub-bullet matches the `` `src` -> `dst` `` grammar.
5. `move-redundant` — a path is both a `Moves:` endpoint and in `Creates:`/`Deletes:` of the same plan.
6. `move-source-missing` — a `Moves:` source neither exists on disk nor is a `Creates:` target or `Moves:` destination of an earlier or later card.
7. `move-target-collision` — a `Moves:` target already exists on disk, is targeted by more than one card, or collides with a different card's `Creates:` entry (same-card overlap is `card-field-overlap`'s job).
8. `move-mechanic-missing` — the plan has at least one `Moves:` pair somewhere but `00-overview.md` has no `## Rename mechanic` section (plan-level).
9. `card-missing-field` — a card lacks one of `What:`/`Context:`/`Edits:`/`Creates:`/`Deletes:`/ `Moves:`/`Depends-on:` (now including the new `Depends-on:` field).
10. `card-field-overlap` — the same path appears in more than one of a single card's `Context:`/`Edits:`/`Creates:`/`Deletes:` fields or `Moves:` endpoints (per-card mutual exclusivity only — the legitimate cross-card `Creates:`-then-`Edits:` sequencing is never flagged).
11. `card-numbering` — flat `N` runs 1..M across the whole plan, no gaps or duplicates;
    the per-card file prefix `NN` must equal the heading `N`.
12. `path-missing` — an `Edits:`/`Deletes:`/`Context:` path (a `Moves:` source is check 6's job) that does not exist on disk and is not a `Creates:` target or `Moves:` destination of any card.
13. `commit-subject-mismatch` — a present `Commit:` value that does not start with the card's own `N: ` prefix.
14. `depends-on-order` (**new**) — a card's `Depends-on:` names a card at or after its own position (a later card or itself), or names an id that references no existing card.

## Worked example

A complete minimal plan for a fictional task ("add a `--json` flag to `lyx board list`"), byte-consistent across Card Index ↔ per-card filenames ↔ card headings/numbering.
Across its four card files this example demonstrates every plan-format feature: all five typed file-op fields (with `none` sentinels), flat `N` card headings, a `## Shared Decisions` overview entry, a plan-level `root:` with a `//`-escaped path, a `Depends-on:` field with a real dependency, a pinned `Commit:` field, and a `Moves:` card with its plan-level `## Rename mechanic` section.

`_lyx/plan/00-overview.md`:

```markdown
---
format: 3
approved: true
root: internal/boardcli
---

# Plan: add --json to `lyx board list`

Add a `--json` output mode to `lyx board list`, emitting one JSON object per row via the
`internal/output` envelope, with tests and help text updated, and the row mapper relocated ahead
of a later extraction.

## Card Index

1 — json-flag — add the `--json` bool flag and RowJSON struct
2 — json-emission — marshal each row through output.Ok when --json is set
3 — json-tests — cover --json in boardcli list tests
4 — helptree-rename — update help-tree pins and rename the row mapper

## Shared Decisions

### Decision: json-envelope-reuse

- **Decision:** `--json` marshals each row through the existing `internal/output.Ok` envelope —
  no new envelope type is introduced.
- **Rationale:** one JSON emission path for the whole CLI; a second envelope shape would fork
  behavior for no gain.
- **Applies to:** all cards

## Rename mechanic

1. Run `git mv <old> <new>` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. Use `Creates:` only for genuinely new files, never for the relocated file itself.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.

## verify:

go test ./internal/boardcli/... ./internal/boardengine/... ./cmd/lyx/...
```

`_lyx/plan/01-json-flag.md`:

```markdown
# Card 1 — json-flag

**What:** Add a `--json` bool flag to the list command; define `RowJSON` with the existing
table's columns as fields.
**Context:** none
**Edits:**
- `list.go`
- `//internal/boardengine/rows.go`
**Creates:** none
**Deletes:** none
**Moves:** none
**Depends-on:** none
**Commit:** `1: json-flag`
**verify:** go build ./...
```

`_lyx/plan/02-json-emission.md`:

```markdown
# Card 2 — json-emission

**What:** When `--json` is set, marshal each row through `output.Ok` instead of the table
writer; keep the table path unchanged.
**Context:**
- `//internal/output/envelope.go`
**Edits:**
- `list.go`
**Creates:** none
**Deletes:** none
**Moves:** none
**Depends-on:** 1
```

`_lyx/plan/03-json-tests.md`:

```markdown
# Card 3 — json-tests

**What:** Add table-driven tests asserting one `output.Ok` envelope per row for `list --json`,
and that the table path is unchanged without the flag.
**Context:** none
**Edits:**
- `list_test.go`
**Creates:** none
**Deletes:** none
**Moves:** none
**Depends-on:** 2
```

`_lyx/plan/04-helptree-rename.md`:

```markdown
# Card 4 — helptree-rename

**What:** Update the pinned help-tree set with the new `--json` flag help text, and relocate the
row mapper via `git mv` per the Rename mechanic above (no behavior change in this card).
**Context:** none
**Edits:**
- `//cmd/lyx/helptree_test.go`
**Creates:** none
**Deletes:** none
**Moves:**
- `//internal/boardengine/rows.go` -> `//internal/boardengine/rowsjson.go`
**Depends-on:** 1
```

`list.go`/`list_test.go` above resolve (per the plan's `root: internal/boardcli`) to `internal/boardcli/list.go`/`internal/boardcli/list_test.go`;
the `//`-prefixed entries (`rows.go`, `envelope.go`, `helptree_test.go`) stay worktree-root-relative regardless of `root:`, escaping it for the files each card needs outside the shared prefix.

## Related

- [webster-spec.md](webster-spec.md#the-summary-artifact--_lyxwebstersummarymd) and `internal/websterengine`'s package documentation — the module that consumes this format.
- `contracts/stencils/loom/loom-template-plan.md` — the LLM-facing compact spec `Plan-Write` actually reads; this doc is the Go-parser's own fuller contract, not the agent's prompt.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — `ChangedFilesSince`/`SHAExists` used for contract verification.
- [`internal/scoutengine`](../../internal/scoutengine/doc.go) — the module the symbol fields depend on.
