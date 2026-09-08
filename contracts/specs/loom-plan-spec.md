# Loom plan-spec — flat card list

> **Status: Contract — pinned.** This doc pins **plan-format**: the flat card-list plan schema `Plan-Write` produces, which webster (`internal/websterengine`, via its sole parser `internal/planparser`) consumes. This is `internal/planparser`'s own as-built contract — the twenty-eight checks below are already implemented, not a future spec — kept as a durable Go-to-Go reference doc under `contracts/specs/`, not deleted on landing. The LLM-facing subset of this format — what `Plan-Write` itself must write — is pinned separately in the producer's own stencil, `contracts/stencils/loom/loom-template-plan.md`, so the agent's prompt never duplicates this file and the two cannot drift from being the same doc.

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
  amendments.md        # optional; append-only handle-substitution log, see Plan: handles below
  ...
```

`00-overview.md` frontmatter carries **scalar-only** keys:

```yaml
format: 5
approved: true
root: <optional worktree-relative dir>   # optional; see Card path resolution below
language: go                             # optional; "go" (default) or "none" — see The shape classifier below
```

The body carries a short task-framing paragraph, an ordered **Card Index** whose entries read `N — <card-slug> — <one-line intent>`,
and the optional plan-level body sections `## Shared Decisions`, `## Rename mechanic`, and `## verify:`.

`NN-<card-slug>.md` — one file per card (`NN` = zero-padded card order);
the file *is* the card.
Card Index ↔ card files are cross-checked mechanically (the `index-file-mismatch` check, below): numbering, slugs, no gaps, no orphaned file on disk.

## Card fields and order

Each card lives in its own file, and the file's content is:

1. **Title heading** — `# Card N — <name>`, where `N` is the card's own flat number (see [Numbering and commit subject](#numbering-and-commit-subject) below).
2. **One or more bold type labels** from the set `**Create:**`, `**Edit:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**` — the type name is the key, and there is no separate `Type:` field.
   Each label's own indented, backtick-wrapped sub-bullets are that label's own targets:

   ```markdown
   **Edit:**
   - `internal/boardcli#newListCmd`
   - `list.go`
   ```

   A card whose targets span two labels carries both groups, one after the other, each with its own sub-bullets:

   ```markdown
   **Edit:**
   - `list.go`

   **Create:**
   - `list_json_test.go`
   ```

   A card carrying a `**Custom:**` group may carry no group of a different type — `Custom` groups may repeat, but never combine with any of the other six.
   A `Rename` label's sub-bullets instead carry `` `old` -> `new` `` pairs — see [Rename and Move](#rename-and-move) below.
3. Optionally, **`**Uses:**`** — what the card reads or depends on but does not change, in the same backtick-wrapped-bullet shape as a target list.
4. A required, multi-line **`**Intent:**`** — what, and why (prose, may span multiple lines until the next field label).
5. **`**ImpactSummary:**`**, required for `Edit` and `Delete` cards only, taking its value inline on the label line — a hard-capped one-line blast-radius conclusion, never the card's main content.
6. Optionally, **`**Commit:**`** and **`**Verify:**`** — see [Numbering and commit subject](#numbering-and-commit-subject) and [verify model](#verify-model) below.

**A field with no content is omitted entirely** — this format admits no `none` sentinel on any field.
An optional field a card has nothing to say under simply does not appear in that card's file;
a required field a card omits — every type label (a card must carry at least one), `Intent:`, or `ImpactSummary:` when any of the card's own groups is `Edit` or `Delete` — is a `card-missing-field` finding, and a label that is present but carries no bullets/prose under it is the distinct `card-field-empty` finding.

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

A multi-label card composes this table's four columns as follows, one rule per column:

- **Target list holds** — per group, no composition: each label's own sub-bullets hold only that label's own kind of target, exactly as the table row states for that label alone.
- **Mechanical check** — the union across the card's own groups: every group's own mechanical check runs, each against that group's own targets, never against another group's targets.
  `Create`'s "none — check nothing equivalent exists first" cell is a real obligation that joins this union like any other, not a no-op that a multi-label card can skip past.
- **`ImpactSummary`** — required whenever any of the card's own groups is `Edit` or `Delete`, and stays exactly one per card even when several of its groups require it: it states the blast radius across every `Edit`/`Delete` group's targets together, never a separate summary per group.
- **Batchable?** — least permissive wins across the card's own groups: a card is batchable only when every one of its groups says `Yes`, and a single `No` group makes the whole card `No`.
  `Prosa`/`Custom`'s "—" is never a vote in this computation — it neither forces `No` nor grants `Yes`, so a `Prosa`/`Custom` group's presence is transparent to the other groups' own answer.

## The shape classifier

A card's own target/`Uses`/`Rename`-pair entries mix **glyphs**, `plan:` handles, file paths, and bare symbols in one flat list, distinguished by shape alone, in this fixed five-rule order:

1. A ref beginning with the literal prefix `plan:` is a **handle**.
2. Otherwise, a ref containing `#` is a **glyph** — the unit-then-member string `internal/foo#Bar` names, per quarry's `glyph` package (`docs/glyph.md`). This rule must precede the path rules below, because every Go glyph's unit half contains a `/`, so a slash-based path rule reached first would misclassify every glyph as a path.
3. Otherwise, a separator (`/`) present anywhere in the entry makes it a **path** — this also covers the `//` worktree-root escape, since it always contains a slash — and so does an all-lowercase-ASCII-alphanumeric final dot-segment (a bare filename with a lowercase extension, e.g. `list.go`).
4. Otherwise, an entry with no `.` and no `/` at all is a **path** too: an extensionless repository-root filename such as `Makefile`, `LICENSE`, or `Dockerfile`. Without this rule such a filename would have no legal spelling left under the `bare-symbol-target` hard rule below, since the `//` worktree-root escape does not rescue it.
   Canonicalization carries the rule the rest of the way: a slash-free path-shaped entry is canonicalized to its self glyph even without a file extension (see the "Card path resolution" section below), so the spelling this rule admits is one every glyph-backed layer downstream can act on rather than a bare token `quarry` rejects before resolution.
5. Otherwise, it is a **bare symbol** — the explicit default for an entry whose final dot-segment is not all-lowercase-alphanumeric (`shedrecipe.Lookup`).

The overview frontmatter's optional `language:` key gates this alphabet: `"go"` (the default when the key is absent) enables `glyph.Go`'s grammar for rules 1-2 and the two hard rules below; `"none"` opts a plan out of the glyph alphabet entirely, keeping a bare-symbol entry legal exactly as it was before this alphabet existed.

**Two hard rules, both skipped under `language: none`:**

- `bare-symbol-target` — any bare-symbol-shaped entry is a hard finding, under any card type. A bare package-qualified symbol is the one spelling that cannot have come verbatim from a quarry answer — not because the form is uglier, but because quarry never emits one. The fix is always to spell it as a glyph.
- `directory-target` — a path-shaped entry containing a `/` with no file extension names a directory rather than a file; spell it as a unit glyph (e.g. `internal/foo#`) if it is a package, or list the files instead if it is not code. The `/` condition keeps rule 4's extensionless repository-root filenames out of this finding; a slash-free extensionless entry at the repository root is not caught here — it is canonicalized to its own self glyph instead, which is the legal spelling for both a root file (`LICENSE#`) and a whole root directory (`docs#`), so there is nothing left for this check to say about it.

This is a deliberate, partial deviation from the design doc's own classification clause.
The design doc resolves ambiguity "against ground truth (`go doc` for a symbol, file existence for a path)";
this spec takes that clause in its **shape half only** — a process spawn (`go doc`) is barred from tier1 by the Test Tier Purity Invariant and would stop the parser being a leaf (the Planparser Sole-Parser Invariant).
The **file-existence half survives**, but at validation time rather than classification time, as the `path-missing` check below — existence never decides an entry's shape, only whether a path- or self-glyph-shaped entry's target is satisfied. `internal/planparser` itself never calls `quarry.Resolve` or any other `quarry.Repo` method — that belongs to `internal/planglyph`, the resolve-backed layer above it — and it performs exactly one glyph conversion in each direction: `glyph.Self` (path -> glyph, at canonicalization time) and `Glyph.UnitPath` (glyph -> path, used only by the two disk-existence checks below on a **self** glyph; a member glyph is skipped by both, since member existence is `internal/planglyph`'s resolve-backed business, not this package's).

**Known limitation:** an unexported symbol reference whose final dot-segment happens to be all-lowercase (e.g. `shedrecipe.lookup`) misclassifies as a path under rule 3, and surfaces as a loud `path-missing` finding rather than a silent misparse.
The author resolves it by writing the exported name (as a glyph), or by `//`-escaping the entry so it is unambiguously a path.

## Card path resolution: `root:` and `//`

`00-overview.md`'s frontmatter may carry an optional **plan-level** `root: <worktree-relative-dir>`.
When set, every path-shaped card entry in the plan resolves as `<root>/<path>` **unless** the path starts with `//`, which is *always* worktree-root-relative (root set or not — one rule, no special cases): that is how a card names a file outside the shared root, e.g. `//cmd/lyx/main.go`.
This is purely a token-economy shorthand for a plan whose cards repeat the same directory prefix over and over.
The degenerate `root: "."` case (the worktree root itself) resolves a card path to the raw path unchanged, rather than the unclean `"./<raw>"` a literal string join would produce.

Normalization applies to **path-shaped entries only**: the parser normalizes every card path to a plain worktree-relative, forward-slash path exactly once, at parse time.
A **glyph- or bare-symbol-shaped entry passes through verbatim, regardless of `root:`** — the shape classifier gates normalization, so `root:` never gets prepended onto one.

**Ordering is load-bearing: `root:`/`//` resolution runs first, while a ref is still path-shaped; canonicalization to a glyph runs strictly after, on the same ref.** Immediately after normalization, and only under a glyph-enabled `language:`, the parser canonicalizes a path-shaped entry into its glyph string via `glyph.Self` — the one path-to-glyph call this package makes — whenever that entry carries a file extension **or** carries no `/` at all.
A *slashed* extensionless entry (a bare directory such as `internal/foo`) is the one case left as a plain path, so the `directory-target` check above still has something to classify.
The slash-free half of the rule is what keeps rule 4's repository-root filenames usable end to end: left as the bare token `LICENSE`, such an entry validates clean through all twenty-eight checks and is then handed verbatim to `quarry`, which rejects it *before* resolution — so `internal/planglyph`'s own `DoneChecks` read the rejection as "did not resolve" and blocked the creating card with a permanent `create-not-done`, while `prosa-symbol-target` reported it as not-a-self-glyph.
Canonicalized to `LICENSE#` it resolves `found`, which is the spelling `quarry`'s own rejection message recommends. **A surface glyph is always repository-root-relative and is never `root:`-joined** — canonicalization only ever touches a ref classified as a path in the first place, and a glyph is never that shape.
The parser records each canonicalized entry's pre-canonicalization surface lexeme in `Plan.SurfaceRefs`, keyed by the owning card's own identity and the resulting canonical string, so a later rewrite of the underlying files can restore the exact byte-form the card's own file carried.

The validator and any future consumer never see `root:` or `//` again, only normalized-then-canonicalized paths (or verbatim glyphs/symbols).
A single-`/` prefix or a `..` segment in a card path is malformed and is flagged by the `card-path-malformed` check — now applied, per the shape classifier's disk-mapping rule above, to a path- or self-glyph-shaped entry alike.

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

Step 3 names a separate `Create` card rather than the removed `Creates:` field — this format has no typed file-op fields, so "genuinely new content" is always its own card under the `Create` label, never a bullet folded into another card's field.

This is the repo's own `git mv` + surgical-edits convention made declarable in a plan and mechanically checkable, rather than an unstated expectation an implementer might miss.

**Accepted false positive:** because a `Move` card's destination lives in `Intent` prose rather than in its own target list, there is no third union (parallel to the `Create`/`Rename` target unions below) for `path-missing` to check a `Move` destination against.
A later card naming a file an earlier `Move` card relocated into place therefore produces a false-positive `path-missing` finding — the earlier `Move`'s destination is real on disk by the time the later card runs, but nothing in the plan model records that.
The author resolves it by reordering the cards, or by naming the file's pre-move path in the later card instead.

## Plan: handles

A `plan:` handle (shape rule 1 of [The shape classifier](#the-shape-classifier) above) is a draft, plan-local placeholder spelling for a symbol or file that does not exist yet, so no quarry answer could ever have named it.
Letting the planner invent a handle's *draft* spelling is safe in a way that letting it invent a real glyph is not, because a handle carries no reality to point at — it only has to be internally consistent within the plan, which is fully mechanically checkable, and `quarry` never sees one.

A `Create:` label's sub-bullet may take either the plain single-ref form every other type label admits, or the two-field declaration grammar:

```markdown
**Create:**
- `plan:<draft-handle>` -> `<declaration head>`
```

The left-hand token is the handle itself — `plan:<unit>#<member>`-shaped, the unit half being an ordinary repository-relative path — and the right-hand token is the declaration head: the symbol's own spelling and kind (e.g. a function signature), the text `internal/planglyph`'s resolve-backed layer hands to `quarry.Name` as a `{Unit, Decl}` pair once the handle is bound to a real glyph. Reusing the `` `old` -> `new` `` arrow grammar `Rename` already carries means a handle declaration needs no new punctuation, only a `plan:`-prefixed left-hand token.

**The declaration head is parsed as source, not read as prose, and must declare exactly one symbol.**
`quarry.Name` wraps it in a synthetic file and parses it with the same grammar the walk uses, so a placeholder body (`type RowJSON struct{...}` — `...` is not Go) fails to parse and is the blocking finding `handle-name-failed`, and a head declaring several symbols (`const A, B = 1, 2`, an interface written out with its methods) is rejected for that instead.
A bare head is complete and is the form to write: `type RowJSON struct`, `func newRowJSON(r Row) RowJSON`, `type Reader interface`, and — for a method, receiver included — `func (c *Cache) Get(k string) (Row, bool)`.

Once bound, a `Create` declaration bullet collapses to the plain `` - `<glyph>` `` ref form every other `Create` target uses: the declaration head existed only to compute the glyph that has now been computed, and keeping the arrow beside a non-handle left token would fail `handle-malformed`.

A `Rename` group's two sides carry different obligations under this alphabet: the `Old` side must be a glyph — it names something real that will be resolved — and the `New` side must be a `plan:` handle, whose content a later binding step computes and overwrites at the validation boundary rather than trusting the planner's draft spelling. A `Rename` card therefore needs no declaration head of its own, unlike a `Create` card, because the declaration is derived from the resolved old side. A file-rename pair — both sides self glyphs — is exempt from this rule, since it has no declaration head to name and belongs in the same group as a plain path pair.

**A `Rename`'s `Old` side can never be a symbol the same plan creates.** `Plan-Validate`/`Plan-Revalidate` resolve the whole plan against the tree as it stands before any card has run, so a `Create` card's own not-yet-real symbol has nothing to resolve against yet — the `Old` side of a later card renaming it reads `not_found`, and the declaration-derivation step described above has no resolved signature to derive from (`rename-old-unresolved`), regardless of how the plan sequences its cards. This is a limit on what one plan can express, not a defect: a symbol renamed in the very plan that creates it gains nothing a correctly-named `Create` does not already give it, so the fix is to spell the `Create` target with its final name to begin with.

Six checks keep this mechanism internally consistent: `handle-dangling`, `handle-collision`, `handle-unreferenced`, and `handle-malformed` (all keeping a declared handle consistent with where it is referenced), and `rename-to-not-handle`/`rename-from-not-glyph` (keeping a `Rename` pair's two sides on their correct side of the handle/glyph line) — see [Validation checks](#validation-checks-as-implemented-by-internalplanparser) below for each check's own row.
The four handle checks do not all read the same source set, and the split is deliberate: `handle-dangling` and `handle-collision` read **both** declaring sources (a `Create` sub-bullet and a `Rename` to-side), `handle-unreferenced` reads `Create` declarations alone — a rename destination that no *other* card references is the ordinary case, not a defect — and `handle-malformed`'s file-unit rule reads `Create` declarations alone too, since a `Rename` to-side's unit is derived from its resolved old side rather than from the draft handle's own unit half. All six run under every `language:`, including `"none"`, since a handle is loomyard's own grammar, not glyph grammar, and its consistency is checkable without any alphabet.

A plan carrying at least one `plan:` handle also gains a companion write path once that handle is bound to a real glyph: `internal/planparser.RewriteRefs` substitutes the handle for its resolved glyph string, in place, across every card file that referenced it, and `internal/planparser.AppendAmendment` appends a record of that substitution to the plan directory's own append-only `amendments.md` log (a known non-card file, exempt from `index-file-mismatch` the same way `00-overview.md` is). Neither write path is part of the plan-format grammar itself — a card author never writes to either — but both exist because this format admits a handle in the first place, so a reader of a plan carrying handles should expect an `amendments.md` file to appear beside it once review resolves them.

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

The **`changes-files`/deviation union** — the artifact webster's fork-return contract compares actual changed files against (a fork reports `OK, SHA <x>` or a deviation note; a file-list mismatch against this union is always informational, never blocking on its own) — is, under format 5, restated over glyphs rather than paths-plus-symbol-files: a mechanical guard compares the delta's changed files against the batch's own target glyphs' `UnitPath`-mapped files directly, rather than this spec defining a files-union to compare against. The mismatch stays exactly as informational, never blocking on its own, and `Uses:` stays excluded from the comparison because it names what a card reads, not what it changes.
See `internal/websterengine`'s package documentation for the verification semantics.
This union is defined over each card's flat target set (the union across all of that card's own `TargetGroups`), so it is unchanged by multi-label: a card carrying two groups contributes both groups' targets exactly as it always contributed one group's.

Symbol/path matching and SCC condensation into a deterministic topological order have shipped — see `internal/websterengine`'s package documentation under its "Execution order is derived, not declared" section.
What remains deferred is continuous DAG update across waves and any parallel execution, both of which belong to the roadmap's Someday `webster: worktree-per-card parallel execution` item.

A parked, more aggressive parallel-execution idea also exists — see [../../manifest/designs/webster-parallel-execution.md](../../manifest/designs/webster-parallel-execution.md).

## Validation checks (as implemented by `internal/planparser`)

Machine checks this format is designed to support, in this fixed order, one row per distinct `Check:` ID — twenty-eight rows, twenty-eight IDs.
This figure counts distinct IDs rather than presentation rows, which resolves the row-count-versus-ID-count divergence the repo's former "14" carried (a 14-row list whose row 1 bundled two distinct IDs).
The twenty-eight IDs are split across two entry points, `ValidateFormat` and `Validate`: twenty-seven of them are the format-only set `ValidateFormat` runs, and `plan-unapproved` (row 3 below) is additionally checked by `Validate`, the full entry point.
The rows below stay in one fixed order regardless of which entry point runs them, and `plan-unapproved` keeps its position-three slot in that order even though it alone belongs to the wider entry point:

1. `format-unrecognized` — `format:` is a recognized version (currently only `5`); else refuse to run.
2. `plan-language-unrecognized` — `language:` (when present) is `"go"` or `"none"`; else flagged, naming the offending value and the two legal ones. Absent defaults to `"go"` and is never flagged.
3. `plan-unapproved` — `approved: true`; else refuse to run.
   This is a consumer guard, and its "else refuse to run" is deliberately not enforced by every caller: `Plan-Revalidate` (the post-segment mechanical row) and every standalone plan consumer (`internal/websterengine`, `internal/webstercli`, `internal/batcher`) enforce it, while the pre-review gate, `Plan-Validate`, deliberately does not — the plan writer is forbidden from setting the flag, and the review segment (`Plan-Bouncer`'s approved settle) is what writes it, so a pre-review caller demanding it would be demanding something only review itself can produce.
4. `index-file-mismatch` — Card Index ↔ card files consistent (numbering, slugs, no gaps, no orphaned file on disk).
   This check covers the card count because there is no separate `(C cards)` segment to cross-check;
   the index itself IS the card list.
5. `card-type-missing` — every card carries at least one recognized type label; zero is flagged.
6. `card-custom-not-alone` — a card carrying a `Custom` group alongside a group whose `Type` differs from `Custom` is flagged, once per offending card regardless of how many differently-typed groups it carries.
   Two `Custom` groups on one card, with nothing else, stays legal.
7. `card-retired-label` — a card body carries a format-3 label (`**What:**`, `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Depends-on:**`, or the lowercase `**verify:**`); each occurrence is its own finding.
8. `card-path-malformed` — this check is card-generic, not group-scoped.
   Every path- or self-glyph-shaped entry in a card's own flat `Targets`/`Uses`, once normalized (`root:`/`//` resolution applied) and mapped to its disk path (a self glyph via `Glyph.UnitPath`), is non-empty, relative, clean, and free of `..` escapes, regardless of which group contributed it. A member glyph is skipped.
9. `bare-symbol-target` — see [The shape classifier](#the-shape-classifier) above. Skipped entirely under `language: none`.
10. `directory-target` — see [The shape classifier](#the-shape-classifier) above. Skipped entirely under `language: none`.
11. `glyph-malformed` — a `#`-containing entry (shape rule 2, [The shape classifier](#the-shape-classifier) above) that fails `glyph.Parse` — a doubled `#`, an empty unit, a member carrying a paren, a keyword, or any other grammar violation — regardless of whether the entry's own group is otherwise exempt from existence/resolve checks. Card-generic over `Targets`/`Uses`, including a `Prosa` group's own targets, which `prosa-symbol-target` (row 25 below) also separately flags for the same entry as "not a self glyph" — the two checks answering the same defect from two angles is the same shape `bare-symbol-target` and `prosa-symbol-target` already share for a `Prosa` group's bare-symbol target. Skipped entirely under `language: none`, for the same reason `bare-symbol-target` is: without a glyph-enabled `language:` no ref is ever glyph-shaped by grammar, only by the shape classifier's own literal `#` rule, and validating a grammar this alphabet has not opted into would be meaningless. Without this check, an entry classified `refKindGlyph` by shape alone but rejected by `glyph.Parse` is invisible to every other check outside a `Prosa` group: `bare-symbol-target`/`directory-target` skip it (wrong shape), `card-path-malformed`/`path-missing` skip it (their own disk-path mapping fails closed rather than flagging), `containment-unit-overlap` skips it the same way, and it never even enters the batched glyph-resolve pass that would otherwise report `glyph-not-found`/`glyph-ambiguous`/`glyph-rejected` — so a plan carrying one validates clean while carrying a target no execution engine can ever act on.
12. `rename-format` — every non-well-formed `Rename:` sub-bullet fails the `` `old` -> `new` `` grammar, checked per `Rename` group.
13. `handle-dangling` — a `plan:` handle appearing in some card's `Targets` or `Uses` with no matching `Create`-group declaration on any card and no matching `Rename`-group to-side. See [Plan: handles](#plan-handles) below.
14. `handle-collision` — the same `plan:` handle claimed more than once across the plan: by two `Create` sub-bullets, by two `Rename` to-sides, or by one of each; one finding per colliding handle, not per claiming card.
    The format has **two** sources that bring a handle into existence and `handle-dangling` (row 13) already accepts either, so counting only `Create` declarations let a handle claimed by both pass validation clean and then be resolved by silent overwrite during canonicalization — a `Create` declaration's unit comes from the handle itself while a `Rename` to-side's comes from the resolved old side, so the two produce two different canonical glyphs for one draft spelling and the later one rewrites the `Create` card's own declaration bullet.
15. `handle-unreferenced` — a declared `plan:` handle that no card other than its own declaring card(s) references.
16. `handle-malformed` — a `Create:` sub-bullet whose payload carries the `` -> `` arrow but fails the two-field `` `plan:<draft-handle>` -> `<declaration head>` `` grammar, or a handle-shaped entry whose text after the `plan:` prefix carries no `#` and therefore names no unit, or — under a glyph-enabled `language:` only — a handle whose unit half names a `.go` file rather than a package directory (quarry's `Name` echoes a file-unit member ID that its `Resolve` then never answers, so the spelling validates clean and wedges the creating card's own record-batch done-check instead; the unit is always the package directory).
    The file-unit rule binds a handle whose unit half is actually read — a `Create` declaration's — and is skipped for a handle claimed **only** as a `Rename` pair's to-side, whose derived declaration takes its unit from the resolved `Old` side rather than from the draft spelling, so the rule's own stated consequence cannot arise for it.
17. `rename-to-not-handle` — a symbol `Rename` pair's `New` side classifies as anything other than a `plan:` handle; the finding names the shape it actually is (a glyph, a bare symbol, or a file path). A file-rename pair (both sides self glyphs) is exempt.
18. `rename-from-not-glyph` — a symbol `Rename` pair's `Old` side classifies as anything other than a glyph, named the same way. A file-rename pair is exempt.
    Both rows are the negation of the one admitted shape rather than an enumeration of the forbidden ones, so no shape the classifier can return escapes them.
19. `rename-mechanic-missing` — the plan has at least one card carrying a `Rename` group but `00-overview.md` has no `## Rename mechanic` section (plan-level);
   a `Rename` group on an otherwise multi-label card still counts.
20. `card-missing-field` — a card lacks `Intent:` (card-generic), or lacks `ImpactSummary:` when any of its own groups is `Edit` or `Delete` (group-triggered, but the missing field itself is still one card-level field).
21. `card-field-empty` — a label present with no content under it: an empty target list is checked per group, so a card carrying a populated group alongside an empty one is still flagged for the empty group, while an empty `Uses:`, blank `Intent:` prose, or a blank `ImpactSummary:` value are each card-generic.
22. `card-field-overlap` — the same entry appears in both a card's own flat target list and its own `Uses:` field; card-generic, per-card mutual exclusivity only — the legitimate cross-card `Create`-then-`Edit` sequencing is never flagged.
23. `containment-unit-overlap` — a member glyph on one card's own target list physically overlaps another card's own self glyph naming the same unit — the syntactic half of the cross-granularity containment check; the member→file half is `internal/planglyph`'s resolve-backed business, not this package's. Skipped entirely under `language: none`. See [Plan: handles](#plan-handles) below for the write paths this batch adds alongside it.
24. `impact-summary-multiline` — an `ImpactSummary:` field followed by trailing non-label lines; `ImpactSummary` must stay a single line.
    Card-generic, since `ImpactSummary` is one field per card regardless of how many groups require it.
25. `prosa-symbol-target` — under a glyph-enabled `language:`, a `Prosa` group's own target list holds an entry that is not a self glyph (a member glyph, or anything that fails to parse as a glyph at all — a plain path or a bare symbol); under `language: none`, a symbol-shaped entry, exactly as before this alphabet. Group-scoped, so an offending entry in the same card's non-`Prosa` group is never flagged by this rule.
26. `card-numbering` — a card file's heading number must equal the Card Index number assigned to it.
    Card-generic.
27. `path-missing` — a path- or self-glyph-shaped entry that does not exist on disk (mapped via `Glyph.UnitPath` for a self glyph) and is not satisfied by any card's `Create`-group target or `Rename`-group destination (mapped the same way) in the same plan. A member glyph is skipped, not resolved — member existence is `internal/planglyph`'s resolve-backed business.
    A card's `Uses:` entries are checked card-generically, and within a card, its own groups are then walked one at a time, and a group's own path-/self-glyph-shaped targets are checked only when that group's own `Type` is `Edit`, `Delete`, `Move`, or `Prosa`.
    A `Rename` group's own `Pairs.Old` entries are checked instead of its `Refs`, and its `Pairs.New` side is never checked.
    `Custom` stays exempt on its own targets — and from the `prosa-symbol-target` rule above, restated in group terms: a `Custom` group's own targets are exempt from both rules — and from nothing else, since every other group and every card-generic check still binds it.
28. `commit-subject-mismatch` — a present `Commit:` value that does not start with the card's own `N: ` prefix. Card-generic.

## Worked example

A complete plan for a fictional task ("add a `--json` flag to `lyx board list`"), byte-consistent with the golden fixture `internal/planparser`'s own tests parse.
Across its seven card files this example demonstrates every plan-format feature: all seven type labels are exercised across the suite of cards below (`Create`, `Edit`, `Custom`, `Delete`, `Rename`, `Move`, `Prosa`), flat `N` card headings, a `## Shared Decisions` overview entry, a plan-level `root:` with `//`-escaped entries, a pinned `Commit:`/`Verify:` pair, and a `Rename` card with its plan-level `## Rename mechanic` section.
Card 2 is additionally the multi-label example: it carries an `**Edit:**` group followed by a `**Create:**` group, for the implementation-plus-its-own-new-test-file shape.

`_lyx/plan/00-overview.md`:

```markdown
---
format: 5
approved: true
root: internal/boardcli
language: go
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
- `internal/boardcli#RowJSON`

**Intent:** Define the `RowJSON` struct carrying the list command's existing table columns as JSON-taggable fields.

**Commit:** `1: json-row-type`
**Verify:** go build ./...
```

`_lyx/plan/02-json-flag.md`:

```markdown
# Card 2 — json-flag

**Edit:**
- `internal/boardcli#newListCmd`
- `list.go`

**Create:**
- `list_json_test.go`

**Uses:**
- `//internal/output/envelope.go`

**Intent:** Add the `--json` bool flag to `newListCmd` and branch its row output between the table writer and the JSON path.

**ImpactSummary:** Adds a --json flag to the list command and branches its row-emission path on it.
```

`_lyx/plan/03-json-emission.md`:

```markdown
# Card 3 — json-emission

**Custom:**
- `internal/output#emitJSON`
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
- `internal/boardengine#MapRow` -> `plan:internal/boardengine#MapRowJSON`
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

`list.go`/`doc.go`/`list_json_test.go` above resolve (per the plan's `root: internal/boardcli`) to `internal/boardcli/list.go`/`internal/boardcli/doc.go`/`internal/boardcli/list_json_test.go`, then canonicalize (per the plan's `language: go`) to the file self glyphs `internal/boardcli/list.go#`/`internal/boardcli/doc.go#`/`internal/boardcli/list_json_test.go#`;
the `//`-prefixed entries (`envelope.go`, `emit.go`, `legacyrows.go`, `rows.go`, `rowsjson.go`, `helppins.go`, `boardcli-json.md`) stay worktree-root-relative regardless of `root:`, escaping it for the files each card needs outside the shared prefix, and canonicalize to their own file self glyphs the same way.
`internal/boardcli#newListCmd`, `internal/boardcli#RowJSON`, `internal/output#emitJSON`, and `internal/boardengine#MapRow` are glyph-shaped entries already, copied verbatim from a quarry answer, and pass through every one of these resolution rules byte-identical — a glyph is never `root:`-joined and never re-canonicalized.
Card 5's `New` side, `plan:internal/boardengine#MapRowJSON`, is a handle rather than a glyph, because a symbol `Rename`'s `New` side must be one (`rename-to-not-handle`, and [Plan: handles](#plan-handles) above): the renamed symbol does not exist until the rename lands, so its spelling is computed at the validation boundary rather than written here.

## Related

- [webster-spec.md](webster-spec.md#the-summary-artifact--_lyxwebstersummarymd) and `internal/websterengine`'s package documentation — the module that consumes this format.
- `contracts/stencils/loom/loom-template-plan.md` — the LLM-facing compact spec `Plan-Write` actually reads; this doc is the Go-parser's own fuller contract, not the agent's prompt.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — `ChangedFilesSince`/`SHAExists` used for contract verification.
- [manifest/designs/plan-card-format.md](../../manifest/designs/plan-card-format.md) — the design doc this spec's format-4 rewrite implements. Format 5's glyph alphabet is a later, additive rewrite on top of it.
