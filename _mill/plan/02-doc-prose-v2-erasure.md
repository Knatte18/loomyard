# Batch: doc-prose-v2-erasure

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: doc-prose-v2-erasure
number: 2
cards: 5
verify: null
depends-on: [1]
```

## Batch Scope

This batch erases every plan-format-**v2** reference and every surviving bare-`v3` label from the three markdown files the sweep could not finish on its own: the renamed `docs/reference/plan-format.md`, `manifest/designs/loom.md`, and `manifest/roadmap.md`.
It is one batch because all five cards are prose surgery on documents, judged by whether the sentences read correctly rather than by any assertion — the same reviewer reads them all, and splitting them would fragment that judgement across sessions.
It depends on batch 1 because every line it edits is either inside the renamed file or was itself rewritten by the sweep;
it runs in parallel with batch 3, which owns the Go-comment half of the same erasure and shares no file with this batch.

Batch-local decisions beyond `## Shared Decisions`:

- `plan-format-5-in-scope` — the retired-v2 blockquote and its bare `>` separator are deleted as a pair, not repaired. The sweep leaves the line untouched (it carries no pattern), and after the rename its link points at the file itself. This task creates that defect, so this task fixes it.
- `roadmap-203-in-scope` — `manifest/roadmap.md`'s "v3 is the live plan format now that its predecessor is retired." sentence is deleted rather than left for task E, because this task's own sweep of the heading above it is what makes the sentence incoherent. Same "owns every site whose claim it itself falsifies" rule the manifest applies to task A.
- `roadmap-18-is-rewritten-not-swept` — card 7 additionally rewrites line 18, the one line the sweeper deliberately skipped, so `manifest/roadmap.md` ends this batch with no plan-format-v3 reference and no standing grep exclusion. Added on operator instruction, 2026-08-09: every plan-format-v3 reference the task can reach is removed, and this one is reachable by rewriting the sentence rather than replacing a token.
- `loom-29-in-scope` — the `loom.md` producer-list line is rewritten in full rather than left self-contradicting for task E, overriding `shed-followups.md:209-210`. Repairing half a sentence is worse than repairing all of it.

Both of the last two are deliberate departures from the manifest, and both are recorded as override notes by batch 4 — do not silently widen or narrow them here.

## Cards

### Card 4: Delete the retired-v2 blockquote from the renamed plan-format doc

- **Context:** none
- **Edits:**
  - `docs/reference/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete **two** adjacent lines near the top of the file — the bare `>` separator line and the blockquote paragraph immediately after it that currently reads, post-sweep:

  ```
  > **v3 is the live plan format.** [plan-format.md v2](plan-format.md) is retired now that builder, its sole consumer, is gone. v3 — consumed by `webster` — is the sole plan format.
  ```

  Delete them as a pair.
  Deleting only the paragraph leaves the `>` behind as a trailing empty quote line;
  deleting both leaves the `> **Status: Contract — pinned.**` paragraph as the sole blockquote, which is the intended end state.
  Do not rewrite or relocate the claim — there is nothing left to say once v2 is gone, and the `Status` paragraph above already states that the doc is the pinned contract.
- **Commit:** `docs(plan-format): drop the retired-v2 blockquote`

### Card 5: Erase every remaining plan-format-v2 reference and bare-v3 label from the renamed doc

- **Context:** none
- **Edits:**
  - `docs/reference/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite each site below so it states the property directly, with no reference to a predecessor format and no version label of any kind.
  Every one of these is prose or a parenthetical;
  none carries a sweep pattern, which is why they survived batch 1.
  Locate them by their quoted text, not by line number — card 4 has already shifted the file by two lines.

  1. "v2's per-batch `## Scope` concept is **removed entirely** — there is no batch-level "declared ownership" list in v3." → state the property: there is no batch-level declared-ownership list and no `## Scope` section.
  2. "v3 keeps lyx's own established `What:` name, playing the same role it played in v2." → keep the first claim, drop the trailing comparison clause, and name the format rather than a version.
  3. "(unlike v2, where `root:` was per-batch)" → delete the parenthetical. The surrounding sentence already says `root:` is plan-level.
  4. The `card-path-malformed` bullet's "(v2's `scope-malformed`, renamed because Scope is gone)" → delete the parenthetical. It appears **twice** in the file, once in the prose paragraph and once in the numbered check list;
     delete both.
  5. "This check **absorbs** v2's `card-count-mismatch` — v3 has no `(C cards)` segment to cross-check separately" → restate as a property of the check itself: the check covers the card count because there is no separate `(C cards)` segment to cross-check.
  6. The `move-mechanic-missing` bullet's "(now plan-level, was per-batch in v2)" → "(plan-level)".
  7. The whole "**Dropped from v2, and why:**" block — its lead-in plus all five entries (`verify-missing`, `chain-end-dangling`, `batch-oversized`, `card-outside-scope`, `card-count-mismatch`) — is **deleted**. It exists solely to describe a delta against a format that no longer exists. Delete the surrounding blank line left behind so the numbered check list flows straight into the `## Worked example` heading.

  Do **not** touch the `format: 3` frontmatter key anywhere in the file, including inside the worked example — that is the schema's own version field, not the document's name (`schema-version-field-is-not-the-doc-name`).
  Do **not** rewrite the doc's framing beyond removing the v2 claims, do not add producer-model vocabulary, and do not touch the "Batch is gone / the card is the unit" section — those belong to downstream tasks C and F.
  When done, `grep -ni 'v3' docs/reference/plan-format.md` must return nothing;
  verify that rather than assuming it.
- **Commit:** `docs(plan-format): erase every plan-format-v2 reference and version label`

### Card 6: Rewrite the loom.md Plan-producer line

- **Context:**
  - `internal/websterengine/doc.go`
  - `docs/reference/plan-format.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the "Plan producer" bullet of the agent/producer list, replace the sentence that currently reads, post-sweep, "**The target format is changing:** today's pinned [plan-format.md v2](../../docs/reference/plan-format.md) (batch-based) is being replaced by [plan-format](../../docs/reference/plan-format.md) (a flat card list) — see that doc for the schema the Plan producer will write against, and `internal/websterengine`'s package documentation for the consumer that now implements it."

  The sweep collapsed both links onto the same target, so the sentence now claims a file is being replaced by itself.
  Rewrite it to state that the pinned plan format is `plan-format.md`, a flat card list, linked once at `../../docs/reference/plan-format.md`, and to point at `internal/websterengine`'s package documentation for the consumer that implements it.
  No reference to a predecessor format, no "is changing" or "will write against" framing — the format is live and the consumer already implements it.
  Read the package doc comment first so the pointer describes what is actually there.

  Change **only** this one bullet.
  Rows 2-7 of the producer table belong to task C, and everything else in the file belongs to task E;
  the sweep already made its own path-level edits elsewhere in the file in batch 1 and those stand as they are.
- **Commit:** `docs(loom): state the live plan format instead of a replacement in progress`

### Card 7: Repair both roadmap sites — the stale live-format sentence and the excluded breakdown line

- **Context:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two separate edits in this file.

  **Edit 1 — the Done item.** In the item whose heading the sweep rewrote to "**plan-format: flat card list**", delete the single sentence "v3 is the live plan format now that its predecessor is retired."

  Both of its claims are broken by this task's own edit: the item is no longer titled v3, so "v3 is the live plan format" names nothing, and "its predecessor is retired" is a v2 reference carrying no token for the sweep to catch.
  Delete the sentence outright rather than rewriting it — the heading and the following `See` link already say everything that remains true.
  Leave the item's other lines, including the `See [docs/reference/plan-format.md](../docs/reference/plan-format.md).` line the sweep already rewrote, exactly as they are.

  **Edit 2 — line 18, the six-task breakdown line.** The sweeper deliberately skipped this one line, so it still reads, in the middle of a longer sentence listing the six follow-up tasks:

  ```
`plan-format-drop-v3-suffix` (B — mechanical rename sweep, `plan-format-v3.md` → `plan-format.md`)
  ```

  Rewrite the parenthetical so it describes the change instead of spelling both filenames — for example, a mechanical rename sweep that drops the version suffix from the plan-format doc.
  Naming the old file was never load-bearing here: the parenthetical is a one-line summary of what task B does, not a citation of a path, and the surrounding sentence already links `designs/shed-followups.md` for the full task bodies.
  This is why the line is hand-rewritten rather than swept — a blind replacement would have collapsed the arrow to "`plan-format.md` → `plan-format.md`", destroying the record (`roadmap-18-is-rewritten-not-swept`).

  Keep the task slug `plan-format-drop-v3-suffix` on that line exactly as it is.
  It is a task name, not a format reference, it is the wiki and branch identifier for this very task, and it matches none of the six sweep patterns — `plan-format-drop-v3` breaks the `plan-format-v3` adjacency and the string `plan-v3` does not occur — so it passes the acceptance grep untouched.
  The same slug appears elsewhere in the repo for the same reason;
  leave every occurrence alone.

  After this card, `manifest/roadmap.md` carries no plan-format-v3 reference at all and needs no grep exclusion — batch 4's final gate checks it like any other file.
- **Commit:** `docs(roadmap): drop the v3 suffix from the plan-format item and the task breakdown`

### Card 8: Confirm the documentation gates

- **Context:**
  - `docs/reference/plan-format.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `manifest/designs/shed-followups.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff verification card for this batch's half of the erasure.

  1. Acceptance gate 4 — `grep -ni 'v3' docs/reference/plan-format.md` returns **nothing**. The renamed doc names no version anywhere.
  2. `grep -niE '\bv2\b' docs/reference/plan-format.md manifest/designs/loom.md manifest/roadmap.md` returns **nothing**.
  3. Acceptance gate 8, for this batch's share — every relative markdown link and anchor this batch touched resolves. In particular `../../docs/reference/plan-format.md` from `manifest/designs/loom.md` and `../docs/reference/plan-format.md` from `manifest/roadmap.md` both resolve to the file that now exists, closing the dangling links task A deliberately left open.
  4. Re-run acceptance gate 2 (the six-pattern grep from card 3) and confirm it still returns zero — a hand edit that reintroduces a pattern is the one way this batch could regress batch 1's work.
  5. Card 7's line-18 rewrite specifically — run the same grep **without** the `^\./manifest/roadmap\.md:18:` filter and confirm `manifest/roadmap.md` produces no line at all. This is the gate batch 4 runs repo-wide; proving it here localizes a failure to this batch rather than to the terminal one.
  6. `grep -n 'plan-format-drop-v3-suffix' manifest/roadmap.md` still finds the task slug on line 18. The slug is deliberately preserved (`roadmap-18-is-rewritten-not-swept`) — if it is gone, card 7 over-edited.

  Explicitly **not** a regression, and not to be filed as one: `manifest/designs/shed-followups.md` still cites the doc's pre-rename path in four places.
  That file is excluded by design;
  it is a historical record of what each task was told at scoping time, and those citations are accurate as of that moment.
  Batch 4's override notes are where a reader learns the file moved.
- **Commit:** none

## Batch Tests

`verify: null`.
This batch edits three markdown files and no Go source, so there is no runnable surface of its own to exercise.
Nothing in the repo reads any of these three files at test time — the plan-format worked example that `internal/planparser`'s golden fixture mirrors is materialized from hardcoded test source, not loaded from `docs/reference/`, so a doc edit cannot turn a test red.
The overview's module-wide `verify: go build ./...` still runs at the batch boundary as a cheap backstop.

Verification for this batch is card 8's grep gates, which check the property that actually matters here — zero surviving version labels and zero surviving v2 references in the three files — plus review, which is the only thing that can judge whether the seven rewritten sentences read correctly.
That last point is the task's known blind spot: `shed-followups.md:232-233` is explicit that the meaningful failure mode is incompleteness, checked by grep rather than by an assertion.
