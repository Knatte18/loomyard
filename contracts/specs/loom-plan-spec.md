# Loom plan-spec — flat card list

> **Status: Contract — pinned.** This doc pins **plan-format**: the flat card-list plan schema `Plan-Write` produces, which webster (`internal/websterengine`, via its sole parser `internal/planparser`) consumes. This is `internal/planparser`'s own as-built contract — the sixteen checks below are already implemented, not a future spec — kept as a durable Go-to-Go reference doc under `contracts/specs/`, not deleted on landing. The LLM-facing subset of this format — what `Plan-Write` itself must write — is pinned separately in the producer's own stencil, `contracts/stencils/loom/loom-template-plan.md`, so the agent's prompt never duplicates this file and the two cannot drift from being the same doc.

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
Any later grouping of cards (e.g. by webster, for read-cost reasons — same file/module, per the cards' declared targets) is a later, measured decision made outside the plan format, not something the plan format needs to express or `Plan-Write` needs to decide.

There is no batch-level "declared ownership" `## Scope` concept.
A card's own type-label target list *is* its declared footprint;
there is no wider unit left to declare a footprint for.

## Plan vs. schedule

The flat card list is the **plan** (a DAG of intent: what depends on what).
It is not itself an execution order.
Whoever executes the plan (webster today, or a hypothetical future parallel executor — see the roadmap's Someday list) decides *how* to turn the DAG into an actual run — webster today derives a topological order from the cards' own `Targets`/`Uses` refs and runs it strictly sequentially, one fork at a time, potentially wave-based parallel execution for some future version.
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
format: 4
approved: true
root: <optional worktree-relative dir>   # optional; see Card path resolution below
```

The body carries a short task-framing paragraph, an ordered **Card Index** whose entries read `N — <card-slug> — <one-line intent>`,
and the optional plan-level body sections `## Shared Decisions`, `## Rename mechanic`, and `## verify:`.

`NN-<card-slug>.md` — one file per card (`NN` = zero-padded card order);
the file *is* the card.
Card Index ↔ card files are cross-checked mechanically (the `index-file-mismatch` check, below): numbering, slugs, no gaps, no orphaned file on disk.

## Card fields and order

Each card lives in its own file, and the file's content is:

1. **Title heading** — `# Card N — <name>`, where `N` is the card's own flat number (see [Numbering and commit subject](#numbering-and-commit-subject) below).
2. **Exactly one bold type label** from the set `**Create:**`, `**Edit:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**` — the type name is the key, and there is no separate `Type:` field.
   The label's own indented, backtick-wrapped sub-bullets are the card's own targets:

   ```markdown
   **Edit:**
   - `boardcli.newListCmd`
   - `list.go`
   ```

   A `Rename` card's sub-bullets instead carry `` `old` -> `new` `` pairs — see [Rename and Move](#rename-and-move) below.
3. Optionally, **`**Uses:**`** — what the card reads or depends on but does not change, in the same backtick-wrapped-bullet shape as a target list.
4. A required, multi-line **`**Intent:**`** — what, and why (prose, may span multiple lines until the next field label).
5. **`**ImpactSummary:**`**, required for `Edit` and `Delete` cards only, taking its value inline on the label line — a hard-capped one-line blast-radius conclusion, never the card's main content.
6. Optionally, **`**Commit:**`** and **`**Verify:**`** — see [Numbering and commit subject](#numbering-and-commit-subject) and [verify model](#verify-model) below.

**A field with no content is omitted entirely** — format 4 admits no `none` sentinel on any field.
An optional field a card has nothing to say under simply does not appear in that card's file;
a required field a card omits (its type label, `Intent:`, or `ImpactSummary:` on an `Edit`/`Delete` card) is a `card-missing-field` finding, and a label that is present but carries no bullets/prose under it is the distinct `card-field-empty` finding.

**Uses:** names what the card reads but does not change — never a target.
An entry appearing in both a card's own target list and its own `Uses:` is a contradiction: is it being changed, or only read? — flagged by the `card-field-overlap` check.
This is strictly **per-card**: across two cards of the same plan, one card's `Create` target followed by a later card's `Edit` of the same target is legitimate sequencing.

## Card types

| Type | Target list holds | Mechanical check | `ImpactSummary` | Batchable? |
|---|---|---|---|---|
| Create | new symbol(s)/file(s) | none — check nothing equivalent exists first | not required | No — one judgment unit per card |
| Edit | existing symbol(s) | impact/blast-radius on the symbol being changed | required | No |
| Delete | existing symbol(s) OR whole file(s) | assert-no-callers (necessary, not sufficient) | required | Yes — independent targets only |
| Rename | existing symbol(s), `old -> new` pairs | AST-aware script + grep verify, never text/regex replace | not required | Yes — independent symbols only |
| Move | existing symbol relocated to a file, OR a whole file relocated | `git mv` + import fixup; destination stated in `Intent`, not the target list | not required | Yes |
| Prosa | file(s), no symbol target | none | not required | — |
| Custom | either | none — explicit escape hatch | as applicable | — |

`ImpactSummary` is required for `Edit` and `Delete` only — a `Create` card has no existing callers to have a blast radius over, which is why this spec resolves the design doc's table in favour of the design doc's own prose rather than its table row, a drafting slip the doc's prose does not carry.

## The shape classifier

A card's own target/`Uses`/`Rename`-pair entries mix symbols and file paths in one flat list, distinguished by shape alone, in this fixed three-rule order:

1. A separator (`/`) present anywhere in the entry makes it a path — this also covers the `//` worktree-root escape, since it always contains a slash.
2. Otherwise, an all-lowercase-ASCII-alphanumeric final dot-segment makes it a path (a bare filename with a lowercase extension, e.g. `list.go`).
3. Otherwise, it is a symbol — the explicit default for an entry with no `.` at all (`Lookup`, `Makefile`) and for an entry whose final dot-segment is not all-lowercase-alphanumeric (`shedrecipe.Lookup`).

This is a deliberate, partial deviation from the design doc's own classification clause.
The design doc resolves ambiguity "against ground truth (`go doc` for a symbol, file existence for a path)";
this spec takes that clause in its **shape half only** — a process spawn (`go doc`) is barred from tier1 by the Test Tier Purity Invariant and would stop the parser being a leaf (the Planparser Sole-Parser Invariant).
The **file-existence half survives**, but at validation time rather than classification time, as the `path-missing` check below — existence never decides an entry's shape, only whether a path-shaped entry's target is satisfied.

**Known limitation:** an unexported symbol reference whose final dot-segment happens to be all-lowercase (e.g. `shedrecipe.lookup`) misclassifies as a path under rule 2, and surfaces as a loud `path-missing` finding rather than a silent misparse.
The author resolves it by writing the exported name, or by `//`-escaping the entry so it is unambiguously a path.

## Card path resolution: `root:` and `//`

`00-overview.md`'s frontmatter may carry an optional **plan-level** `root: <worktree-relative-dir>`.
When set, every path-shaped card entry in the plan resolves as `<root>/<path>` **unless** the path starts with `//`, which is *always* worktree-root-relative (root set or not — one rule, no special cases): that is how a card names a file outside the shared root, e.g. `//cmd/lyx/main.go`.
This is purely a token-economy shorthand for a plan whose cards repeat the same directory prefix over and over.
The degenerate `root: "."` case (the worktree root itself) resolves a card path to the raw path unchanged, rather than the unclean `"./<raw>"` a literal string join would produce.

Normalization applies to **path-shaped entries only**: the parser normalizes every card path to a plain worktree-relative, forward-slash path exactly once, at parse time.
A **symbol-shaped entry passes through verbatim, regardless of `root:`** — the shape classifier gates normalization, so `root:` never gets prepended onto a symbol reference.
The validator and any future consumer never see `root:` or `//` again, only normalized paths (or verbatim symbols).
A single-`/` prefix or a `..` segment in a card path is malformed and is flagged by the `card-path-malformed` check.

## Rename and Move

A `Rename` card's sub-bullets are `` `old` -> `new` `` pairs on the ASCII arrow grammar (backtick-wrapped on both sides, exactly one arrow), and both endpoints of every pair project into the card's own target list, in pair order (`Old` then `New`).

A `Move` card states its destination in `**Intent:**` prose rather than in its target list — the target list holds only the symbol or file being relocated, never its new location.

**`## Rename mechanic` is a plan-level section**, one section in `00-overview.md`, **required when any card in the plan is type `Rename`** — the `rename-mechanic-missing` check (plan-level) flags a plan that declares a `Rename` card but omits it.
The section's text is CANONICAL — reproduce it verbatim (adjusted only for the specific paths involved):

```markdown
## Rename mechanic

1. Run `git mv <old> <new>` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. A genuinely new file with no predecessor belongs in a separate `Create` card, never folded
   into the `Rename` pair.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.
```

Step 3 names a separate `Create` card rather than the removed `Creates:` field — format 4 has no typed file-op fields, so "genuinely new content" is always its own card under the `Create` label, never a bullet folded into another card's field.

This is the repo's own `git mv` + surgical-edits convention made declarable in a plan and mechanically checkable, rather than an unstated expectation an implementer might miss.

**Accepted false positive:** because a `Move` card's destination lives in `Intent` prose rather than in its own target list, there is no third union (parallel to the `Create`/`Rename` target unions below) for `path-missing` to check a `Move` destination against.
A later card naming a file an earlier `Move` card relocated into place therefore produces a false-positive `path-missing` finding — the earlier `Move`'s destination is real on disk by the time the later card runs, but nothing in the plan model records that.
The author resolves it by reordering the cards, or by naming the file's pre-move path in the later card instead.

## Numbering and commit subject

Cards are numbered flat **`N` (1..N)** across the whole plan — no batch-scoped restart, no `NN.C` compound numbering.
The per-card file prefix `NN` (zero-padded) must equal the heading `N`.

The **default commit subject is `N: <name>`** — the card heading's `<name>`;
there is no separate `<short what>` seed.
An explicit `**Commit:**` overrides the default but must start with the card's own `N: ` prefix — the `commit-subject-mismatch` check enforces this, because a pinned message that breaks the `N:` shape would corrupt the git-log resume trail the numbering scheme exists to give.

Commit-per-card is the **resume mechanism**: a fresh session sees from `git log` exactly which card the previous session reached,
and a half-done card is resumed by discarding uncommitted changes and restarting that card.

## verify model

The three-tier verify model (tier1 automatic package-scoped, tier2 plan-level integration, tier3 rare and explicit-only) is **designed, not implemented** — see `manifest/designs/plan-card-format.md`'s own Verify model section for the full design.
This spec pins only what exists today: the per-card **`**Verify:**`** field stays the optional, verbatim, rare escape hatch it already was under format 3 — a cheap, targeted check where it is useful, never a required field, never a long hand-maintained list.
Tier1's automatic package-scoped run is specified only, not implemented, by this task;
there is no mandatory per-card or per-batch verify gate in the code today.
The plan-level `## verify:` body section in `00-overview.md` (unchanged in shape from format 3) is the single integration suite run once at the end of the plan.

## Deferred / forward-compat

The **`changes-files`/deviation union** — the artifact webster's fork-return contract compares actual changed files against (a fork reports `OK, SHA <x>` or a deviation note; a file-list mismatch against this union is always informational, never blocking on its own) — is, under format 4: every path-shaped target entry across the batch's cards, plus the files holding every symbol-shaped target entry.
`Uses:` is excluded from this union because it names what a card reads, not what it changes.
See `internal/websterengine`'s package documentation for the verification semantics.

Symbol/path matching and SCC condensation into a deterministic topological order have shipped — see `internal/websterengine`'s package documentation under its "Execution order is derived, not declared" section.
What remains deferred is continuous DAG update across waves and any parallel execution, both of which belong to the roadmap's Someday `webster: worktree-per-card parallel execution` item.

A parked, more aggressive parallel-execution idea also exists — see [../../manifest/designs/webster-parallel-execution.md](../../manifest/designs/webster-parallel-execution.md).

## Validation checks (as implemented by `internal/planparser`)

Machine checks this format is designed to support, in this fixed order, one row per distinct `Check:` ID — sixteen rows, sixteen IDs.
This figure counts distinct IDs rather than presentation rows, which resolves the row-count-versus-ID-count divergence the repo's former "14" carried (a 14-row list whose row 1 bundled two distinct IDs).
The sixteen IDs are split across two entry points, `ValidateFormat` and `Validate`: fifteen of them are the format-only set `ValidateFormat` runs, and `plan-unapproved` (row 2 below) is additionally checked by `Validate`, the full entry point.
The rows below stay in one fixed order regardless of which entry point runs them, and `plan-unapproved` keeps its position-two slot in that order even though it alone belongs to the wider entry point:

1. `format-unrecognized` — `format:` is a recognized version (currently only `4`); else refuse to run.
2. `plan-unapproved` — `approved: true`; else refuse to run.
   This is a consumer guard, and its "else refuse to run" is deliberately not enforced by every caller: `Plan-Revalidate` (the post-segment mechanical row) and every standalone plan consumer (`internal/websterengine`, `internal/webstercli`, `internal/batcher`) enforce it, while the pre-review gate, `Plan-Validate`, deliberately does not — the plan writer is forbidden from setting the flag, and the review segment (`Plan-Bouncer`'s approved settle) is what writes it, so a pre-review caller demanding it would be demanding something only review itself can produce.
3. `index-file-mismatch` — Card Index ↔ card files consistent (numbering, slugs, no gaps, no orphaned file on disk).
   This check covers the card count because there is no separate `(C cards)` segment to cross-check;
   the index itself IS the card list.
4. `card-type-missing` — every card carries exactly one recognized type label; zero or more than one is flagged.
5. `card-retired-label` — a card body carries a format-3 label (`**What:**`, `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Depends-on:**`, or the lowercase `**verify:**`); each occurrence is its own finding.
6. `card-path-malformed` — every path-shaped target/`Uses` entry, once normalized (`root:`/`//` resolution applied), is non-empty, relative, clean, and free of `..` escapes.
7. `rename-format` — every non-well-formed `Rename:` sub-bullet fails the `` `old` -> `new` `` grammar.
8. `rename-mechanic-missing` — the plan has at least one `Rename` card but `00-overview.md` has no `## Rename mechanic` section (plan-level).
9. `card-missing-field` — a card lacks its required type label, `Intent:`, or (on an `Edit`/`Delete` card) `ImpactSummary:`.
10. `card-field-empty` — a label present on a card with no content under it (an empty target list, an empty `Uses:`, blank `Intent:` prose, or a blank `ImpactSummary:` value).
11. `card-field-overlap` — the same entry appears in both a single card's own target list and its own `Uses:` field (per-card mutual exclusivity only — the legitimate cross-card `Create`-then-`Edit` sequencing is never flagged).
12. `impact-summary-multiline` — an `ImpactSummary:` field followed by trailing non-label lines; `ImpactSummary` must stay a single line.
13. `prosa-symbol-target` — a `Prosa` card's target list holds a symbol-shaped entry; `Prosa` cards may target only files.
14. `card-numbering` — a card file's heading number must equal the Card Index number assigned to it.
15. `path-missing` — a path-shaped entry that does not exist on disk and is not satisfied by any card's `Create` target or `Rename` destination in the same plan.
    `Custom` is exempt from this check on its own targets — and from the `prosa-symbol-target` rule above, and from nothing else: it remains bound by every other card-generic check.
16. `commit-subject-mismatch` — a present `Commit:` value that does not start with the card's own `N: ` prefix.

## Worked example

A complete plan for a fictional task ("add a `--json` flag to `lyx board list`"), byte-consistent with the golden fixture `internal/planparser`'s own tests parse.
Across its seven card files this example demonstrates every plan-format feature: all seven type labels are exercised across the suite of cards below (`Create`, `Edit`, `Custom`, `Delete`, `Rename`, `Move`, `Prosa`), flat `N` card headings, a `## Shared Decisions` overview entry, a plan-level `root:` with `//`-escaped entries, a pinned `Commit:`/`Verify:` pair, and a `Rename` card with its plan-level `## Rename mechanic` section.

`_lyx/plan/00-overview.md`:

```markdown
---
format: 4
approved: true
root: internal/boardcli
---

# Plan: add --json to `lyx board list`

Add a `--json` output mode to `lyx board list`, emitting one JSON object per row via the
`internal/output` envelope, with tests and help text updated, and the row mapper relocated ahead
of a later extraction.

## Card Index

1 — json-row-type — define the RowJSON struct
2 — json-flag — add the --json bool flag and wire list.go
3 — json-emission — marshal each row through output.Ok when --json is set
4 — legacy-rows-delete — remove the superseded legacy row-conversion file
5 — rowmapper-rename — rename the row mapper ahead of a later extraction
6 — helppins-move — relocate the pinned help-tree fixture
7 — json-docs — update the package doc comment and the standalone docs page

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
3. A genuinely new file with no predecessor belongs in a separate `Create` card, never folded
   into the `Rename` pair.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.

## verify:

go test ./internal/boardcli/... ./internal/boardengine/... ./cmd/lyx/...
```

`_lyx/plan/01-json-row-type.md`:

```markdown
# Card 1 — json-row-type

**Create:**
- `boardcli.RowJSON`

**Intent:** Define the `RowJSON` struct carrying the list command's existing table columns as JSON-taggable fields.

**Commit:** `1: json-row-type`
**Verify:** go build ./...
```

`_lyx/plan/02-json-flag.md`:

```markdown
# Card 2 — json-flag

**Edit:**
- `boardcli.newListCmd`
- `list.go`

**Uses:**
- `//internal/output/envelope.go`

**Intent:** Add the `--json` bool flag to `newListCmd` and branch its row output between the table writer and the JSON path.

**ImpactSummary:** Adds a --json flag to the list command and branches its row-emission path on it.
```

`_lyx/plan/03-json-emission.md`:

```markdown
# Card 3 — json-emission

**Custom:**
- `boardcli.emitJSON`
- `//internal/output/emit.go`

**Uses:**
- `list.go`

**Intent:** Introduce `emitJSON`, a new helper in a new file, marshaling each row through `output.Ok` when `--json` is set.
```

`_lyx/plan/04-legacy-rows-delete.md`:

```markdown
# Card 4 — legacy-rows-delete

**Delete:**
- `//internal/boardengine/legacyrows.go`

**Intent:** Remove the legacy per-row conversion helper now that `boardengine.MapRowJSON` (card 5) supersedes it.

**ImpactSummary:** Deletes the legacy row-conversion file; no remaining callers reference it.
```

`_lyx/plan/05-rowmapper-rename.md`:

```markdown
# Card 5 — rowmapper-rename

**Rename:**
- `boardengine.MapRow` -> `boardengine.MapRowJSON`
- `//internal/boardengine/rows.go` -> `//internal/boardengine/rowsjson.go`

**Intent:** Rename the row mapper and its file to make the JSON-oriented behavior explicit ahead of a later extraction.
```

`_lyx/plan/06-helppins-move.md`:

```markdown
# Card 6 — helppins-move

**Move:**
- `//cmd/lyx/helppins.go`

**Intent:** Relocate the pinned help-tree fixture to `//cmd/lyx/helptree/helppins.go` ahead of the CLI help-tree split, with no behavior change in this card.
```

`_lyx/plan/07-json-docs.md`:

```markdown
# Card 7 — json-docs

**Prosa:**
- `doc.go`
- `//docs/boardcli-json.md`

**Intent:** Update the package doc comment and the standalone docs page describing `--json` output.
```

`list.go`/`doc.go` above resolve (per the plan's `root: internal/boardcli`) to `internal/boardcli/list.go`/`internal/boardcli/doc.go`;
the `//`-prefixed entries (`envelope.go`, `emit.go`, `legacyrows.go`, `rows.go`, `rowsjson.go`, `helppins.go`, `boardcli-json.md`) stay worktree-root-relative regardless of `root:`, escaping it for the files each card needs outside the shared prefix.
`boardcli.newListCmd`, `boardcli.RowJSON`, `boardcli.emitJSON`, and `boardengine.MapRow`/`boardengine.MapRowJSON` are symbol-shaped entries and pass through every one of these resolution rules verbatim.

## Related

- [webster-spec.md](webster-spec.md#the-summary-artifact--_lyxwebstersummarymd) and `internal/websterengine`'s package documentation — the module that consumes this format.
- `contracts/stencils/loom/loom-template-plan.md` — the LLM-facing compact spec `Plan-Write` actually reads; this doc is the Go-parser's own fuller contract, not the agent's prompt.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — `ChangedFilesSince`/`SHAExists` used for contract verification.
- [manifest/designs/plan-card-format.md](../../manifest/designs/plan-card-format.md) — the design doc this spec's format-4 rewrite implements.
