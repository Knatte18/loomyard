# Loom plan-spec — flat card list

> **Status: Contract — pinned.** This doc pins **plan-format**: the flat card-list plan `Plan-Write` writes and webster reads through its sole parser, `internal/planparser`. It is the parser's as-built contract, kept under `contracts/specs/`. The plan writer's prompt carries the LLM-facing subset in the plan-format blocks `contracts/stencils/loom/loom-block-plan-*.md`, which `contracts/stencils/loom/loom-template-plan.md` includes.

## Files of this contract

The contract is split by need; read the file that covers what you are doing.

- `loom-plan-spec.md`, this file — the card format: layout, fields, types, refs, handles, numbering, the verify model and granularity.
- `loom-plan-checks-spec.md` — the catalogue of the plan gate's validation checks.
  `lym loom validate-plan` names each failing check and its way forward, so a check's entry is read only when the finding does not settle it.
- `loom-plan-csharp-spec.md` — what a plan under `language: csharp` adds.
- `loom-parts-spec.md` — a split task: its parts, the part scope a part's plan is checked under, and the join of the parts' plans.

## Producer and contract

`Plan-Write` produces the plan, the gates of `Plan-Write` and `Plan-Burler-Fix` validate it, `Plan-Review` reviews it, and `Webster` executes it.

## What a card is

The smallest change that:

1. **Builds on its own** — the whole module builds immediately after the card's commit, so no card references a symbol no earlier card creates.
2. **Is independently committable** — a meaningful, revertible git commit on its own.
3. **Carries the coverage for any new behavior it introduces**; the card's own targets include the test that covers it.
   Pure refactors and renames may rely on existing tests.
   A stencil's static prose is not behavior a test covers; a stencil card carries a test only when it changes a marker, a value Go fills, reads or names, a fragment Go chooses, or a rule a scan across stencils checks.

The card is the plan's unit.
How webster groups cards into batches is no card property; `internal/batcher`'s package doc describes it.

## Plan vs. schedule

A valid plan is executable strictly in card-number order: a card never depends on anything a later card does.
Webster runs the cards in that order and never sorts them, so a card that needs a symbol, file or fixture a later card creates is a defect of the plan.
`uses-later-target` and `delete-before-reference` refuse the cases a card's own refs show; every other dependency on a later card is the writer's to avoid and the plan reviewer's to flag.

## On-disk layout

```
_lym/plan/
  00-overview.md       # frontmatter + Card Index + task framing + optional plan-level sections
  NN-<card-slug>.md    # one file per card (NN = zero-padded card order)
  amendments.md        # optional; append-only handle-substitution log, see Plan: handles below
```

`00-overview.md` frontmatter carries scalar keys only:

```yaml
format: 5
approved: true
root: <optional worktree-relative dir>   # see Card path resolution below
language: go                             # optional; "go" (default), "csharp" or "none"
first_card: 1                            # optional positive integer, default 1; see Numbering below
```

The body carries a short task-framing paragraph, an ordered **Card Index** whose entries read `N — <card-slug> — <one-line intent>`,
and the optional sections `## Shared Decisions`, `## Rename mechanic` and `## PATTERN changes`; a `## verify:` section is accepted and ignored.

`## PATTERN changes` carries the decision record's `## PATTERN changes` entries, one bullet each, in the bullet shape `loom-discussion-spec.md` defines, without the record's added-after-Discussion suffix.

Each `NN-<card-slug>.md` is one card.
The Card Index and the card files must agree on numbering and slugs, with no gap and no orphaned file (`index-file-mismatch`).

## Card fields and order

A card file holds, in this order:

1. **Title heading** — `# Card N — <name>`, `N` the card's flat number.
2. **One or more bold type labels** from `**Create:**`, `**Edit:**`, `**Extends:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**`.
   Each label's indented, backtick-wrapped sub-bullets are that label's targets:

   ```markdown
   **Edit:**
   - `internal/boardcli#newListCmd`
   - `internal/boardcli#TestList`

   **Create:**
   - `list_json_test.go`
   ```

3. Optionally **`**Uses:**`** — what the card reads but does not change, in the same bullet shape.
4. A required, multi-line **`**Intent:**`** — what, and why.
5. **`**ImpactSummary:**`**, required when any group is `Edit`, `Extends` or `Delete`, its value inline on the label line: a one-line blast-radius conclusion, such as "3 callers, all local to the billing package, no cross-module effects", never a restatement of `Intent`.
6. Optionally **`**Commit:**`** and **`**Verify:**`** — see [Numbering and commit subject](#numbering-and-commit-subject) and [verify model](#verify-model).

A field with no content is omitted; the format has no `none` sentinel.
A missing required field is `card-missing-field`, and a present label with nothing under it is `card-field-empty`.

Rules on targets:

- A new test file is a `**Create:**` file target, and the card's Intent states its tier, such as `tier0`, the tier its test functions name in their opening `tiergate.Require` call, on a line that names no other test file (`test-tier-unstated`).
- A card that touches a struct a `//lym:seam` test marks lists that test by its member glyph, under `Edit` when it changes it and under `Uses` when it stays as is (`seam-test-unlisted`).
- A card that changes a package's non-test Go code covers the package's `doc.go`: under `Edit` when it changes what the doc says, under `Extends` when it only adds to it, under `Uses` when it stays as is (`doc-go-unlisted`).
- An `Edit` bullet may carry the re-sign arrow, `` `<glyph>` -> `<new declaration head>` ``, which marks a signature change and states the new head, written like a `Create` declaration head.
  The left token is an ordinary `Edit` target and must be a member glyph (`resign-not-member`).
- A `Custom` group combines with no group of another type; `Custom` groups may repeat.
- An `Extends` group holds existing refs the card only adds to, changing nothing already there; everywhere else its refs read as `Edit` targets.
  The same ref under `Extends` and another label of one card is `extends-overlap`, and an arrow under `Extends` is `extends-resign`.
- A `Rename` label's sub-bullets carry `` `old` -> `new` `` pairs — see [Rename and Move](#rename-and-move).

**Uses:** names what the card reads, never a target.
One entry in both a card's targets and its `Uses:` is `card-field-overlap`.
The rule is per card: one card's `Create` of a target and a later card's `Edit` of it is ordinary sequencing.

There is no `DependsOn` or `Produces` field.
Dependency edges are derived: a card's `Uses:` against every other card's targets.
Two `Extends` on one ref make no edge unless both cards fence a verbatim `PATTERN.md` line for one entry ID; an `Extends` against any other label, and a `Uses` against an `Extends` target, make one.

## Card types

| Type | Target list holds | Mechanical check | `ImpactSummary` |
|---|---|---|---|
| Create | new symbol(s)/file(s) | none — check nothing equivalent exists first | not required |
| Edit | existing symbol(s) | impact on the symbol changed; `caller-uncovered` for a re-signed member | required |
| Extends | existing symbol(s) or file(s) the card only adds to | `extends-overlap`, `extends-resign` | required |
| Delete | existing symbol(s) or whole file(s) | assert-no-callers, mechanically `caller-uncovered` for a member | required |
| Rename | existing symbol(s), `old -> new` pairs | the hub's structure-aware rename tool (`language.yaml` `rename_tools`) + grep verify, never text or regex replace | not required |
| Move | existing symbol or whole file relocated | `git mv` + import fixup; destination stated in `Intent`, not the target list | not required |
| Prosa | file(s), no symbol target | none | not required |
| Custom | either | none — explicit escape hatch | as applicable |

A card with several labels runs every group's mechanical check against that group's own targets, `Create`'s obligation included, and carries one `ImpactSummary` across all its `Edit`, `Extends` and `Delete` groups.

Assert-no-callers is necessary, not sufficient: interface satisfaction, dispatch-table registration, reflection and prose references survive a delete or a rename without failing the build.
A rename also rewrites the symbol's own doc-comment opening to the new name, and the same grep pass verifies it.

A `Create` target under the repository root's git-ignored `.scratch/` is a transient draft: the card may remove it once read, and Webster's `create-not-done` check skips it.
A target no glyph spells, such as a path whose directory holds a space, stays a plain path, and Webster's done-checks judge it by whether the path exists.
A card that renames or deletes what an earlier card targets never shares that card's batch.

## The shape classifier

A card's target, `Uses` and `Rename` entries are classified by shape alone, in this order:

1. A ref beginning `plan:` is a **handle**.
2. Otherwise a ref containing `#` is a **glyph**, the `unit#member` string quarry's `glyph` package defines (`docs/glyph.md`).
3. Otherwise a ref holding `/`, or whose final dot-segment is all-lowercase ASCII alphanumeric (`list.go`), is a **path**.
4. Otherwise a ref with no `.` and no `/` is a **path**: an extensionless repository-root file such as `Makefile`.
5. Otherwise it is a **bare symbol** (`shedrecipe.Lookup`).

`language:` sets the glyph alphabet: `go`, the default, reads refs in Go's glyph grammar; `csharp` in C#'s, per `loom-plan-csharp-spec.md`; `none` opts out of the alphabet and skips every glyph check.
The writer sets `csharp` when the code the cards change is C#.

Two hard rules hold under `go` and `csharp`:

- `bare-symbol-target` — a bare symbol is refused under every card type; spell it as a glyph, copied from a quarry answer.
- `directory-target` — a slashed path with no extension names a directory; spell a package as its unit glyph (`internal/foo#`), and list a non-code directory's files.
  An extensionless file under a `root:` other than `.` is spelled as its repository-root-relative file self glyph (`internal/foo/Makefile#`).

`internal/planparser` never resolves a ref: existence of a path or self glyph is checked on disk at validation (`path-missing`), and a member glyph's existence is `internal/planglyph`'s resolve pass.

**Known limitation:** an unexported symbol whose final dot-segment is all-lowercase (`shedrecipe.lookup`) classifies as a path and surfaces as `path-missing`.
Write it as a glyph, or `//`-escape it when it is a path.

## Card path resolution: `root:` and `//`

The overview's optional `root: <worktree-relative-dir>` prefixes every path-shaped entry as `<root>/<path>`, unless the path starts with `//`, which is always worktree-root-relative: `//cmd/lym/main.go`.
`root: "."` leaves a path unchanged.
Glyphs, handles and bare symbols are never `root:`-joined.

The parser normalizes each path once at parse time to a clean, worktree-relative, forward-slash path.
Then, under `go` or `csharp`, it canonicalizes a path that carries an extension or holds no `/` into its self glyph (`list.go` → `internal/boardcli/list.go#`, `LICENSE` → `LICENSE#`); a slashed extensionless path stays a path, for `directory-target` to judge.
The parser records each canonicalized entry's surface spelling in `Plan.SurfaceRefs`, so a rewrite of the card files restores the bytes the card carried.
A single `/` prefix or a `..` segment is `card-path-malformed`.

## Rename and Move

A `Rename` card's sub-bullets are `` `old` -> `new` `` pairs, backtick-wrapped on both sides with one arrow; both sides join the card's target list, `Old` then `New`.
A symbol pair's `Old` side is a glyph and its `New` side a `plan:` handle; a file pair has a self glyph or path on both sides.

A `Move` card states its destination in `**Intent:**`; its target list holds only what is relocated.
Since the plan records no `Move` destination, a later card naming the moved file at its new path gets a false `path-missing`: reorder the cards, or name the pre-move path.

A plan with any `Rename` group carries `## Rename mechanic` in `00-overview.md` (`rename-mechanic-missing`), reproduced verbatim:

```markdown
## Rename mechanic

1. For a file pair (both sides file self glyphs or paths), run `git mv <old> <new>` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. A genuinely new file with no predecessor belongs in a separate `Create` card, never folded
   into the `Rename` pair.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.
5. For a symbol pair, run the hub's structure-aware rename tool the implementer prompt names (`language.yaml` `rename_tools`), never a text or regex replace, then grep for zero remaining old-name references and confirm the symbol's doc comment opening names the new name.
```

A hub with an empty `rename_tools` list cannot run a symbol pair (`rename-tool-missing`).

## Plan: handles

A `plan:` handle is a plan-local draft spelling for a symbol that does not exist yet, so no quarry answer can name it.
It only has to be consistent within the plan, and quarry never sees it.

A `Create:` sub-bullet takes either a plain ref or the declaration form:

```markdown
**Create:**
- `plan:internal/boardcli#RowJSON` -> `type RowJSON struct`
- `plan:internal/boardcli#newRowJSON` -> `func newRowJSON(r Row) RowJSON`
```

The left token is the handle, `plan:<unit>#<member>`, its unit the package directory, never a `.go` file (`handle-malformed`).
The right token is the declaration head, parsed as source and declaring exactly one symbol: `type RowJSON struct`, `type Reader interface`, `func (c *Cache) Get(k string) (Row, bool)` with its receiver.
A placeholder body such as `struct{...}`, or a head declaring several symbols, is `handle-name-failed`.
Once bound, the bullet collapses to the plain `` - `<glyph>` `` form.

A symbol `Rename` pair's `New` side is a handle whose declaration is derived from the resolved `Old` side, so it needs no head.
The `Old` side must resolve against the tree before any card runs, so a plan cannot rename a symbol it creates: give the `Create` target its final name (`rename-old-unresolved`).

Every handle a card references is declared once, by a `Create` bullet or a `Rename` `New` side (`handle-dangling`, `handle-collision`); a declared handle no card references is valid.
The handle checks run under every `language:`, `none` included.

Once a handle binds to a real glyph, `planparser.RewriteRefs` rewrites it across every card file, and `planparser.AppendAmendment` logs the substitution in `amendments.md`, which `index-file-mismatch` exempts like `00-overview.md`.
No card author writes either.

## Numbering and commit subject

Cards are numbered flat across the plan, from the overview's `first_card:` (default `1`) upward with no gap, so a rework generation can continue the numbering of the one it retires.
The file prefix `NN` (zero-padded) equals the heading `N`.

A card is inserted or removed with `lym loom insert-card` or `lym loom remove-card`, which move every later card by one in file prefix, heading, explicit `Commit:` prefix and Card Index number.
They rewrite every card id naming a moved card, `<N>-<slug>` or `<NN>-<slug>`, so a card refers to another card by its card id.
They report, and never rewrite, lines that name a moved number bare in prose.

The default commit subject is `N: <name>`, from the heading.
An explicit `**Commit:**` must start with the card's `N: ` (`commit-subject-mismatch`), since the `N:` subjects are the resume trail: a fresh session reads from `git log` which card the last one reached, and a half-done card restarts from a clean tree.

## Pattern-Departure trailer

A Webster fork, the recovery strand or a verify-gate fixer that departs from its card because a PATTERN entry forbids the card's way records each departure as one trailer line in the departing commit's body, `Pattern-Departure: <PATTERN-entry-id> — <what differs from the card and why>`, in the commit message's last paragraph, where git reads trailers.
A departure from an entry the task declares changed in the overview's `## PATTERN changes` cites the declared new text, never the old.
`record-batch` and `recover-batch` read the trailers of the batch's own non-merge first-parent commits into the digest's `departures`, and the verify gate reads the verify-gate fixers'; Code-Review reads them in the commit messages of its range.

## verify model

The hub's tests run through `lym gate test`, which runs the numbered tiers `gate.yaml` declares over a scope lym computes.
A gate key names a site's tiers and scope, `card`, `diff` or `all`.
A Go site runs `lym gate test --wait --gate <key>`; an agent's card, batch and `webster-fix` calls carry no `--wait`.
The `card` key runs per card and the `batch` key per batch, both over the cards' packages; `webster-fix` in Webster's verify fixers; `webster` once for the whole plan; `parent-align` and `finalize` at Parent-Align and Finalize.
The plan writer and reviewer get the hub's tiers and gate keys in their prompts, so a plan names no gate, tier list or target list.

- **Per card.** The fork runs `lym gate test --gate card --card <card id>`, which computes the card's packages from the plan, runs the `//lym:guard` tests and the lints the key lists.
  It does not test the rest of the module or the tiers the key leaves out, and a passing call names the declared tiers it did not run.
- **Per batch.** The fork runs `lym gate test --gate batch` with one `--card <card id>` per card of the batch, after its last card.
- **Per plan.** The `webster` gate runs once, a must-pass gate on Merriam that sends a failure back to Merriam.
- Tier 3 tests drive a real LLM; no gate runs them.

Parent-Align and Finalize run their own gate keys whenever the tree differs from the last verified tree, which `internal/verifytree` records, and run the merge fixer on a failure before anything lands.
Both halt Stuck on a dirty tree.

The per-card **`**Verify:**`** field is the rare escape hatch for a check no gate expresses, such as a CLI smoke test or a targeted higher-tier scenario.
A `**Verify:**` line that tests is one `lym gate test --wait` call with a gate key or a tier list and a scope form, such as `lym gate test --wait --tier 1 --card <card id>`, never a raw test command or a target list.
The overview's `## verify:` section is accepted and ignored (`plan-verify-retired`).

## Card granularity

One card per independently reviewable unit, never one card per symbol.
A private supporting type or a constructor inseparable from its type goes on the card of the symbol it serves, and so does a helper serving one surface.
A symbol of a module's public surface, or one reused across packages, gets its own card.

Removing a symbol's last caller and deleting the symbol are separate cards: each caller's `Edit` card, then one `Delete`.

A docstring is never a card of its own: the `Create`, `Edit` or `Rename` card that touches a symbol writes its doc comment, per `docs/code-comment-conventions.md`.
`Prosa` is for documentation with no single-symbol owner: design docs, `docs/overview.md`, a `doc.go` header no code change triggers, a comment sweep.

## Worked example

A plan for a fictional task, "add a `--json` flag to `lym board list`", modelled on the fixture `internal/planparser/testdata/goodplan`.
It uses seven of the eight type labels, a multi-label card (card 2), `root:` with `//`-escaped entries, a pinned `Commit:` and `Verify:`, and a `Rename` card with its `## Rename mechanic`.

`_lym/plan/00-overview.md`:

```markdown
---
format: 5
approved: true
root: internal/boardcli
language: go
---

# Plan: add --json to `lym board list`

Add a `--json` output mode to `lym board list`, emitting one JSON object per row via the
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

<the canonical section above, verbatim>
```

`_lym/plan/01-json-row-type.md`:

```markdown
# Card 1 — json-row-type

**Create:**
- `plan:internal/boardcli#RowJSON` -> `type RowJSON struct`

**Intent:** Define the `RowJSON` struct carrying the list command's existing table columns as JSON-taggable fields.

**Commit:** `1: json-row-type`
**Verify:** lym gate test --wait --tier 0 --card 1-json-row-type
```

`_lym/plan/02-json-flag.md`:

```markdown
# Card 2 — json-flag

**Edit:**
- `internal/boardcli#newListCmd`

**Create:**
- `list_json_test.go`

**Uses:**
- `//internal/output/envelope.go`

**Intent:** Add the `--json` bool flag to `newListCmd` and branch its row output between the table writer and the JSON path.
`list_json_test.go` is tier0.

**ImpactSummary:** Adds a --json flag to the list command and branches its row-emission path on it.
```

`_lym/plan/03-json-emission.md`:

```markdown
# Card 3 — json-emission

**Custom:**
- `internal/output#emitJSON`
- `//internal/output/emit.go`

**Uses:**
- `list.go`

**Intent:** Introduce `emitJSON`, a new helper in a new file, marshaling each row through `output.Ok` when `--json` is set.
```

`_lym/plan/04-legacy-rows-delete.md`:

```markdown
# Card 4 — legacy-rows-delete

**Delete:**
- `//internal/boardengine/legacyrows.go`

**Intent:** Remove the legacy per-row conversion helper, which the JSON path supersedes.

**ImpactSummary:** Deletes the legacy row-conversion file; no remaining callers reference it.
```

`_lym/plan/05-rowmapper-rename.md`:

```markdown
# Card 5 — rowmapper-rename

**Rename:**
- `internal/boardengine#MapRow` -> `plan:internal/boardengine#MapRowJSON`
- `//internal/boardengine/rows.go` -> `//internal/boardengine/rowsjson.go`

**Intent:** Rename the row mapper and its file to make the JSON-oriented behavior explicit ahead of a later extraction.
```

`_lym/plan/06-helppins-move.md`:

```markdown
# Card 6 — helppins-move

**Move:**
- `//cmd/lym/helppins.go`

**Intent:** Relocate the pinned help-tree fixture to `//cmd/lym/helptree/helppins.go` ahead of the CLI help-tree split, with no behavior change in this card.
```

`_lym/plan/07-json-docs.md`:

```markdown
# Card 7 — json-docs

**Prosa:**
- `doc.go`
- `//docs/boardcli-json.md`

**Intent:** Update the package doc comment and the standalone docs page describing `--json` output.
```

Under `root: internal/boardcli`, `list.go`, `doc.go` and `list_json_test.go` resolve to `internal/boardcli/…` and canonicalize to their file self glyphs; the `//` entries stay worktree-root-relative.
The glyphs are copied from quarry answers and pass through unchanged.
Card 5's `New` side is a handle, because the renamed symbol does not exist until the rename lands.

## Related

- [webster-spec.md](webster-spec.md) and `internal/websterengine`'s package documentation — the module that executes this format.
- `contracts/stencils/loom/loom-template-plan.md` and the plan-format blocks it includes — what `Plan-Write` reads.
