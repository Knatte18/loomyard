# Batch: shed-md-and-constraints

```yaml
task: 'shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions'
batch: 'shed-md-and-constraints'
number: 1
cards: 5
verify: go test ./internal/lyxcwd
depends-on: []
```

## Batch Scope

This batch writes the **authoritative** half of the task: `manifest/designs/shed.md` gains the producer-typology carve-out, the thin-Input carve-out, the two-case thin-Output resolution, and the two-axes cross-reference sentence, and sheds its four residue sites;
`CONSTRAINTS.md` gains the short `## Producer Pointer-Rule Invariant` that is the formal twin of `shed.md`'s prose pointer rule.
It is one batch because every card lands in one of two small files that must agree with each other line by line — the invariant statement and the prose rule it formalizes cannot be split across batches without one of them shipping unverified against the other.

The external interface batch 2 consumes is the anchor `shed.md#producer-contract-vs-producer-definition` and the exact vocabulary written into that section: `loom.md` will point at this anchor from two places and restate nothing.
Card 2's typology bullets are the text batch 2's `Kind` column values (`simple`/`bespoke`) are read against, so they must land first.

Batch-local decision beyond `## Shared Decisions`: **`shed.md`'s section headings are frozen.**
The new content in cards 2 and 3 goes in as prose and bullets under the existing `### Producer contract vs. producer definition` heading — no new sub-heading — because `loom.md` and any future doc address that section by its current anchor.

## Cards

### Card 1: retire shed.md's four residue sites

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/finalize.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four surgical fixes in `manifest/designs/shed.md`, none of which adds or removes a heading.
  (a) Line 7 opens "**Revised model (2026-08-08), superseding "two swappable slots" below:**" — the referenced pre-revision text was deleted in commit `256b8262`, so "below" dangles.
  Reword so the supersession is stated without a positional back-reference (the earlier "two swappable slots" description is superseded, full stop);
  do not delete the supersession claim itself, which is still true and is echoed on the roadmap's Planned `Shed` item.
  (b) Line 19 ends "(resolves the open question the pre-revision text **below** left open)" — same dangling back-reference, same fix: state that it resolves the earlier draft's open question without pointing "below".
  (c) Line 18 reads "it is an ordinary producer both `loom` and `Hardener` happen to reference (by *value* — the same producer definition named in both lists)".
  Change **by value** to **by reference**, per the `finalize-shared-by-reference` decision, and adjust the parenthetical so it explains sharing by reference rather than by value.
  Note the phrase is italicised as `by *value*`, so a plain `by value` grep does not find it — locate it by reading line 18.
  (d) Line 63 reads "Wiki task `shed-producer-model-scoping` is the dedicated pass that reconciles any remaining detail mismatch between the two docs."
  That task completed on 2026-08-09 and this task is the last owner, so the present-tense claim is stale.
  Delete that one sentence.
  **The enclosing `## Why this doc doesn't rewrite loom.md's full detail` section stays** — its actual argument (that `loom.md` keeps its own crash-recovery, pause, session-bootstrap and module-decomposition detail rather than having it duplicated into `shed.md`) is unchanged by anything in this task.
  Do not rewrite or delete the section, and do not touch the heading.
- **Commit:** `docs(shed): repair dangling back-references, by-reference sharing, and the stale scoping-task claim`

### Card 2: qualify shed.md's atomicity claim and land the producer typology

- **Context:**
  - `_mill/discussion.md`
  - `manifest/roadmap.md`
  - `manifest/designs/finalize.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two connected edits in `manifest/designs/shed.md`.
  **(a) Line 8** currently reads "It is a generic engine that walks one ordered, flat list of **producers**, each an atomic mechanical action or LLM session, honoring resume/crash-recovery/pause uniformly across every entry."
  This unqualified atomicity claim sits in the `## What it is` summary, fourteen lines above the contract section, so a reader meets it first and may never reach the carve-out.
  Qualify the clause so atomicity binds **simple** producers only, and point the reader down at the `### Producer contract vs. producer definition` section for the carve-out.
  Do **not** restate the carve-out at line 8 — one qualifying clause plus a pointer.
  **(b) The `### Producer contract vs. producer definition` section** (currently lines 22-29) gains the full typology, as prose and bullets under the existing heading, with no new sub-heading.
  It must state, using `manifest/roadmap.md` lines 57-61's own vocabulary rather than any paraphrase, that producers split into two kinds and that the atomicity rule is scoped to the first:
  a **simple, single-agent-spawn producer** — one mechanical action or one LLM session — whose current LLM examples are `Discussion-Write` and `Plan-Write` and whose mechanical examples are the five gate/step producers `Preflight`, `Discussion-Validate`, `Plan-Sweep`, `Plan-Validate` and `Batchifier`;
  and a **bespoke, multi-spawn producer** that owns its own internal loop (many LLM spawns, or an agent orchestrating sub-agents), whose current examples are `Webster` (its per-batch fork loop) and the three `perch`-gated review producers `Discussion-Review`, `Plan-Review` and `Webster-Review` (perch's own round loop, now internal/treadleengine, spawning a fresh burler round per iteration plus ephemeral judge/triage calls).
  **`Finalize` must not appear in the simple list** — it is classified bespoke, per card 2's closing bullet below, and the omission has to be right here rather than corrected downstream.
  The simple kind's LLM examples are named as **candidates** for a shared `Shed`-level "LLM-Producer" type (input file(s) plus optional input-format pointer, internal instruction files, output file(s) plus optional output-format pointer, plus a log) — candidate, never decided — and that kind does not typically need its own crash-recovery, since re-running one spawn from scratch is cheap.
  Bespoke producers are **exempt from the atomicity rule by design, not in violation of it**.
  The section must also record that `Shed`'s own contract stays exactly two parts, Input and Output pointers;
  that its resume/crash-recovery/pause guarantee operates at **producer granularity only**, re-driving a crashed producer from its last recorded pointer and never mid-producer;
  and that a bespoke multi-spawn producer which would lose expensive internal progress on a crash needs its **own** internal crash-recovery, a capability `Shed` does not provide.
  Note that both current bespoke examples already ship it — internal/websterengine re-drives the first unreported batch from its recorded state (see its package documentation's crash/resume section), and internal/treadleengine's round loop keeps its own resumable run-dir state under an OS advisory lock released automatically if the holding process dies.
  Finally the section must classify **`Finalize` as bespoke** and argue it: `manifest/designs/finalize.md` puts raddle-regeneration's parallel leaf forks plus a serial `Overview.md` step inside the merge's critical section, requires the merge lock to span that whole section as one atomic unit never released and re-acquired partway through, and spawns a fresh higher-capability LLM in a clean session on merge conflict — an internal multi-spawn process with an internal-atomicity obligation `Shed` does not provide at sub-producer granularity.
  State that the happy path is genuinely pure Go with zero LLM spawns and that the classification is made on the worst case regardless, because the axis exists to say who owns crash-recovery.
  Record — as an observation for the `Shed` build task, **not** as a design — that unlike `Webster` and the `perch`-gated reviews, `Finalize` does not ship its own internal crash-recovery today, so a crash inside its locked critical section is unrecovered, and that `manifest/designs/finalize.md` already records an alternative giving Raddle its own `Shed` producer with merge-in and locking lifted into `Shed` itself as a live candidate for a future task.
  Do not design that recovery here.
  Do not re-derive or re-open the typology decision itself, and do not re-litigate the rejected alternative of decomposing `Webster` into flat producers.
- **Commit:** `docs(shed): scope atomicity to simple producers and land the simple/bespoke typology`

### Card 3: land the thin-Input carve-out and the two-case thin-Output resolution

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/finalize.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add both carve-outs to `manifest/designs/shed.md`'s `### Producer contract vs. producer definition` section, immediately beside each other and attached to the two-part Input/Output contract statement that currently sits at line 24.
  No new heading.
  **Thin-Input carve-out:** the Input contract permits **no Input at all** for a chain-head producer, because its input is human intent expressed in an interactive session rather than an artifact with a format contract. `Discussion-Write` is the current and only example.
  State the resume consequence honestly, mirroring the thin-Output carve-out's own reasoning: a producer with no Input has nothing to re-read on resume, so a crashed `Discussion-Write` re-runs from its own partial output plus fresh human input — correct, since the human is present at that boundary by definition.
  **Explicitly reject** the alternative framing that the task record is the Input, making the pointer target a task record rather than a format-contract file: that is a mill-ism which does not transfer, since `lyx` has no wiki and no task record, and `manifest/designs/loom.md` already describes Discussion input as an inherently interactive human boundary `lyx run` yields at.
  Admitting a second kind of pointer target would weaken the pointer rule for a target that does not exist in `lyx`.
  **Thin-Output carve-out, stated as two cases, never one:** first, the three gate producers `Preflight`, `Discussion-Validate` and `Plan-Validate` genuinely emit nothing at all — the Output contract permits a bare pass/fail gate signal with no artifact, and the resume-on-output-files rule degrades gracefully, because a producer with no artifact simply re-runs on resume, which is correct for all three since each is a cheap idempotent re-check.
  Second, **`Finalize` is a different case and must not be folded into that sentence**: it plainly has effects (a real `SyncWeft` commit plus an optional PR), and what it has no instance of is a **contract-level output artifact** — nothing downstream consumes its output through a format pointer, because it is the terminal producer in the list.
  Its thin Output is therefore "no *pointer target*", not "no effect", and its resume story is not the graceful degradation above, since a partially-completed merge is not a cheap idempotent re-run;
  that recovery is `Finalize`'s own obligation and is explicitly not designed here.
  The original single-sentence framing — that all four producers "genuinely have no output artifact" — is false for `Finalize` and must not be written.
- **Commit:** `docs(shed): add the thin-Input carve-out and resolve thin-Output as two cases`

### Card 4: add the two-axes cross-reference to the engine-adapter section

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/finalize.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/shed.md`'s `### Engine adapters — a thin, shared seam, not one per producer` section (currently lines 31-46) keeps its four-way split — mechanical Go-function producers, single-LLM-spawn producers, `perch`, `Webster` — **unchanged**, and keeps its "two new adapters, not eleven" conclusion at line 46 **unchanged**.
  Do not restructure the section around the typology;
  its whole purpose is that one argument.
  Add a short cross-reference (one sentence, plus the `Finalize` clarification below) noting that this section cuts on **engine type** — which code drives the producer, and therefore how many adapters must be built — whereas the simple/bespoke typology in the contract section above cuts on **atomicity and crash-recovery ownership**.
  State that the two axes align on `Webster` and `perch` but not elsewhere: `Discussion-Validate` is mechanical *and* simple, while one `perch` adapter serves three separate bespoke producers.
  The cross-reference **must name `Finalize` explicitly** as the sharpest non-alignment case: it is bespoke on the typology axis and still adapter-free on the engine axis, so line 41 correctly keeps listing it among the mechanical Go-function producers that need no translation adapter.
  Explain why both hold at once — `Shed` drives `Finalize` by calling a plain Go function that already satisfies the `ProducerRunner` seam, and the conflict-path LLM spawn and raddle's leaf forks happen *inside* that function through the existing `shuttle` layer, invisible to `Shed`, which is exactly what "bespoke" means (the producer owns its own internals, including recovery) rather than something that changes who drives it.
  State that `Finalize`'s reclassification adds no third adapter, so the count is unchanged.
- **Commit:** `docs(shed): cross-reference the engine-type and typology axes, naming Finalize`

### Card 5: add the Producer Pointer-Rule Invariant to CONSTRAINTS.md

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Producer Pointer-Rule Invariant` section to `CONSTRAINTS.md`, placed **immediately after** the `## Batcher Registry+Config Invariant` section (which currently ends at line 353) and **before** `## GitHub Auth Invariant` (currently at line 355).
  It must be **short** and match the shape of the surrounding seam invariants (Treadle Runner-Seam, Scout Engine-Seam, Shuttle Provider-Seam, Batcher Registry+Config): one invariant statement, then a bulleted clarification, then the enforcement line.
  The invariant statement: an instruction file — a producer's own prompt or skill — must never duplicate or paraphrase another producer's format-contract content, only point at it, so that editing that one format-contract file alone is sufficient to change what both its producer and its consumers do.
  The clarifying bullet must name the invariant's subject precisely: it binds **instruction files** (agent prompts and skills) and format-contract docs, **not** Go source, and not design docs restating the rule for a human reader.
  Close with a `- **Enforced by** review obligation.` line, matching the `## Batcher Registry+Config Invariant` entry's own closing line.
  This invariant is the short formal twin of the prose pointer rule at `manifest/designs/shed.md` line 25 — read that line and make sure the two do not disagree;
  keep both, and do not edit `shed.md` in this card.
  Do not add a machine-checked guard: `manifest/designs/shed-followups.md` scopes the pointer rule as review-enforced.
  `CONSTRAINTS.md` is a link target for `TestEnforcement_MarkdownLinks` but not a scan source, so a new heading here is safe;
  still, do not rename any existing heading in the file, since `manifest/` and `docs/` files link to them by anchor.
- **Commit:** `docs(constraints): add the Producer Pointer-Rule Invariant`

## Batch Tests

`verify: go test ./internal/lyxcwd` runs the three enforcement tests that a docs edit can trip — `TestEnforcement_MarkdownLinks` (`internal/lyxcwd/docslink_test.go`, resolving every inline link and `#anchor` under `manifest/` and `docs/`), plus `TestEnforcement_GeometryLiterals` and `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) — alongside the package's own tests.
`TestEnforcement_MarkdownLinks` is the one machine check that directly covers this batch's output, because card 1 touches back-reference prose adjacent to existing links and cards 2-4 add cross-references inside a section other docs address by anchor.
The batch is scoped to `internal/lyxcwd` rather than the whole tree because that package holds every enforcement test reachable from a Markdown edit;
the repo-wide `go test ./...` regression backstop runs at the done gate.

No new test file is written: this batch is pure docs, and the pointer-rule invariant is deliberately review-enforced rather than machine-checked.
The substantive verification for cards 1-5 is the acceptance grep set in batch 3, card 18, which proves zero surviving instances of the retired phrasings (`by \*value\*`, the present-tense `shed-producer-model-scoping` owner claim) across `manifest/`, `docs/` and `README.md`.
