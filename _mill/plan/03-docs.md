# Batch: docs

```yaml
task: 'webster: DAG-derived card sequencing'
batch: 'docs'
number: 3
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [2]
```

## Batch Scope

This batch corrects every repo doc outside `internal/websterengine` that now makes a false claim about webster's execution order, and moves the roadmap item to Done.
It is its own batch because it consumes batch 2's shipped behavior and touches no Go source at all — three `.md` files, no compile surface.
`internal/websterengine/doc.go` is deliberately NOT here: it is a Go doc comment describing the code that changed, so batch 2's card 12 lands it alongside that code.

Batch-local decision beyond `## Shared Decisions`: `contracts/specs/webster-spec.md` is checked and left alone unless the check turns up a false claim.
Its line 7 — Master "forks one implementer per execution batch in-session, sequentially, until the plan is built" — stays true after this task, since sequencing changes the order, not the sequentiality.

## Cards

### Card 13: correct the plan spec's scheduling claims

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/sequence.go`
  - `manifest/designs/plan-card-format.md`
- **Edits:**
  - `contracts/specs/loom-plan-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Correct the two now-false claims in `contracts/specs/loom-plan-spec.md`.

  In the `## Plan vs. schedule` section, the sentence beginning "Whoever executes the plan (webster today, or a hypothetical future parallel executor …) decides *how* to turn the DAG into an actual run" currently ends "— sequential-in-declared-order today, potentially wave-based parallel execution for some future version."
  Replace the "sequential-in-declared-order today" clause with an accurate one: webster today derives a topological order from the cards' own `Targets`/`Uses` refs and runs it strictly sequentially, one fork at a time.
  Keep the surrounding sentence and the section's closing claim — "The plan format should not need to change if that execution-policy decision changes later" — untouched;
  this task is itself the evidence for that claim, since it changed the execution policy with no format change.

  In the `## Deferred / forward-compat` section, the paragraph reading "The detailed continuous-DAG-update / symbol-matching / SCC-merging **scheduling** design is summarized in `internal/websterengine`'s package documentation ("Declared order now, a dead DAG seam for later") — v0 runs strictly in declared order;
  the eventual DAG scheduler is the roadmap's Wave 3 `webster: DAG-derived card sequencing` item." must be rewritten: symbol/path matching and SCC condensation have shipped, so point at `internal/websterengine`'s package documentation under its new section title (see batch 2 card 12's retitle) and state what is still deferred — continuous DAG update across waves and any parallel execution, both of which belong to the roadmap's Someday `webster: worktree-per-card parallel execution` item.
  Do not quote the old section title, which no longer exists.

  Leave the paragraph about the `changes-files`/deviation union unchanged, and leave the parked `webster-parallel-execution.md` pointer beside it unchanged — that design doc is explicitly owned by another roadmap item.
  Change no validation-check row and no count in the `## Validation checks` section: this task adds no check to `internal/planparser`.
  Use semantic line breaks.
- **Commit:** `docs(specs): correct loom-plan-spec's scheduling claims for derived sequencing`

### Card 14: correct the module overview's webster bullet

- **Context:**
  - `_mill/discussion.md`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/sequence.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update the **webster** module bullet in `docs/overview.md`'s module table — the entry describing plan consumption and batching, which currently states webster "groups cards into execution batches via `internal/batcher`, a separate config-selected registry module webster consumes (identity batcher — one card, one batch — by default)".
  Add, in one clause, that webster then sequences those batches into a topological execution order derived from the cards' own `Targets`/`Uses` refs, condensing and reporting dependency cycles rather than refusing the run.
  Keep the bullet's existing structure, its `✅ Implemented.` marker, and its trailing `See [webster-spec.md](...)` link line exactly as they are.

  Leave the **batcher** module bullet untouched — grouping is still entirely `internal/batcher`'s, which is precisely the distinction the **Batcher Registry+Config Invariant** draws.
  Every inline markdown link in this file is checked by `TestEnforcement_MarkdownLinks`, so introduce no new link whose target or `#anchor` does not resolve;
  the safest edit adds prose only.
  Use semantic line breaks.
- **Commit:** `docs(overview): note webster's DAG-derived batch sequencing`

### Card 15: move the roadmap item to Done and correct the Someday pointer

- **Context:**
  - `_mill/discussion.md`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/doc.go`
  - `contracts/specs/loom-plan-spec.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move the Wave 3 item **webster: DAG-derived card sequencing** out of the Wave 3 list and into the `## Done` section of `manifest/roadmap.md`, rewriting its body to describe what actually shipped rather than what was planned.
  The rewritten Done entry must record, at minimum: sequencing lives in `internal/websterengine/sequence.go` over `[]batcher.Batch`, not in a new package and not as a batchifier;
  edges derive from `Uses` ∩ `Targets` across cards plus a lower-number-wins edge between two cards sharing a target, with path-shaped and symbol-shaped refs treated alike;
  cycles are condensed via strongly-connected components, reported, and never fatal;
  ordering is Kahn's algorithm with a lowest-member-batch-number tie-break, so an already dependency-correct plan sequences to exactly its declared order;
  sequencing is unconditional, with no config key;
  batch identity, numbering, report filenames, and `state.json` keys are unchanged;
  the previous-digest lookup in `begin-batch`/`recover-batch` was corrected from `batchNumber-1` arithmetic to a true execution-predecessor lookup, which is the one existing site the reordering would otherwise have silently broken;
  and `internal/planparser` and `internal/batcher` were not modified.
  Correct the field names while rewriting: the shipped `planparser.Card` field is `Targets`, not the `Edits` the Planned entry's wording still used.

  In the `## Someday` section, update the **webster: worktree-per-card parallel execution** item's dependency pointer, which currently reads "Depends on `webster: DAG-derived card sequencing` (Planned, above)" — that is no longer accurate.
  Point it at the Done entry instead, and keep the rest of that item, including its `webster-parallel-execution.md` staleness note and reconciliation assignment, unchanged.

  Leave the Wave 3 **loom: Plan-Write producer** item and the Wave 3 preamble untouched.
  Every inline markdown link in this file is checked by `TestEnforcement_MarkdownLinks`, so any link carried into the Done entry must still resolve — the moved item's own `See [designs/plan-card-format.md](designs/plan-card-format.md).` line keeps the same relative target and stays valid.
  Use semantic line breaks.
- **Commit:** `docs(roadmap): move webster DAG-derived card sequencing to Done`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `TestEnforcement_MarkdownLinks` (`docslink_test.go`), the machine check that every inline markdown link under `manifest/` and `docs/` resolves — both its file part and any `#anchor`.
That covers two of this batch's three edited files directly (`docs/overview.md`, `manifest/roadmap.md`);
`contracts/specs/loom-plan-spec.md` is outside the scan-source roots, so its own outgoing links are a review obligation, which card 13 handles by adding no new link.
The same package's `TestEnforcement_GeometryLiterals` and the Fabric Vocabulary walk also live here and cover the `.md` half of the vocabulary rule, so a doc edit that reintroduced a policed `host`-sense phrase would fail this same command.

No Go source changes in this batch, so no compile or behavior surface needs a test beyond the overview's module-wide `go build ./...`.
