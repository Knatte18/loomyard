# Batch: reference-contracts

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
batch: reference-contracts
number: 3
cards: 2
verify: null
depends-on: [2]
```

## Batch Scope

This batch retires `docs/reference/builder-contract.md` and `docs/reference/plan-format.md` and puts webster's own cross-module contract in place of the first.
Webster is currently the only live module with no reference doc of its own — its entire cross-module surface is described inside a retired competitor's contract — so `webster-contract.md` closes a real gap rather than merely relocating text.
The batch also re-points every inbound deep link that this task permanently breaks, before tasks B and D run against them.

One batch because the new file and the link repairs are the same change seen from two ends: card 8 must land before card 9 deletes the old file, or the repaired links point at nothing.

Batch-local decision: the link-repair rule is scoped to **permanently-deleted** files, and `builder-contract.md` is the only one.
`plan-format.md`'s inbound links are left to dangle on purpose for the A→B window — task B re-creates the file under the same name, so repairing them now would retarget them at `plan-format-v3.md` only for B to rename that back, churning every link twice to land where it started.

## Cards

### Card 8: Create webster-contract.md

- **Context:**
  - `docs/reference/builder-contract.md`
  - `docs/reference/model-spec.md`
  - `docs/reference/plan-format-v3.md`
  - `docs/reference/status-schema.md`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/outcome.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/summary.go`
  - `manifest/designs/finalize.md`
- **Edits:** none
- **Creates:**
  - `docs/reference/webster-contract.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `docs/reference/webster-contract.md` as webster's consumer-facing cross-module contract — what **other modules may rely on**, nothing more.
  Open with a `> **Status: Contract — pinned.**` banner in the same shape `docs/reference/status-schema.md` and `docs/reference/model-spec.md` use, stating that this is a durable reference doc (kept, not deleted on landing) and that webster's own internals live in `internal/websterengine`'s package documentation, not here.
  The document must cover exactly four things:
  1. **Plan input.** webster consumes the pinned flat card-list [plan-format v3](plan-format-v3.md) via `internal/planparser`, the sole parser of `_lyx/plan/`.
     Describe batching **generically** — "webster groups a plan's cards into execution batches via a config-selected batcher" — and do **not** name `webster.yaml`'s `batcher:` key or the location of the batcher registry.
     A later task extracts that registry into its own module, and naming today's wiring would make this brand-new file a second owner of that change.
  2. **`_lyx/webster/` as an ownership boundary.** webster owns that directory and everything in it — `state.json`, the reports directory, `outcome.yaml`, `summary.md` — resolved via `websterengine`'s own `Dir`/`ReportsDir` helpers, which are the sole declarers of the path segment.
     Its never-tracked siblings (the pause flag, the rendered fork prompts, every `*.lock`) live at the mirrored subpath under `.lyx/webster/` via `ScratchDir`/`PromptsDir` and are deliberately outside the fabric-committed pathspec.
     No other module writes into either.
  3. **`outcome.yaml`.** Master's final-action file: `outcome` is one of `done`, `paused`, `stuck`;
     `stuck_reason` is required non-empty when `outcome: stuck` and empty otherwise;
     `batches_done` is the count of batches that reached `status: done` this run.
     It is strictly decoded (unknown fields rejected) and follows archive-never-refuse — a stale file is timestamp-renamed, never overwritten or refused.
  4. **`_lyx/webster/summary.md`.** The prose summary artifact: first line `# <title>`, then free-form prose narrating what was actually built, including deviations from the original task.
     It is required and fail-loud — presence, non-empty, and a `# <title>` first non-blank line with a non-empty title — **only** when `outcome: done`, and follows the same archive-never-refuse discipline as every other stale artifact.
     It is Finalize's PR-text source, because a long-lived Master session is the only party with full oversight of what actually shipped.
     State explicitly that a `summary.md` may additionally carry an appended `## Integration suite failed` section naming the bisect-localized offending card and its commit SHA — `websterengine`'s `AppendIntegrationFailure` writes it as the document half of an integration-failure escalation, and because Finalize dumps `summary.md` **verbatim** into the PR body, that section reaches the consumer.
     The bisect mechanism that produces it stays webster-internal and is not described here.
  Give the summary-artifact material its own `##` heading so `manifest/designs/finalize.md` and `docs/reference/plan-format-v3.md` can deep-link to a live anchor;
  card 9 re-points those links at it, so pick the heading text first and use it consistently.
  The document must **not** restate anything `internal/websterengine/doc.go` already covers: the fork-based loop shape, the bracket verbs, the `status: OK|FAILED` + `head_sha` + `deviations` fork-return contract, the fork-audit policy, crash/resume, the integration fork + bisect, or the per-batch model assertion.
  Point at the package documentation for all of it — the same split `builder-contract.md` drew against builder's own package docs.
  Do not carry over the old file's chain-rollback or recovery-ladder material: webster has no chain equivalent, and card 9's commit message records where it can be recovered from.
- **Commit:** `docs(webster): add webster-contract.md, webster's consumer-facing contract`

### Card 9: Delete the retired reference docs and re-point their deep links

- **Context:**
  - `docs/reference/webster-contract.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
  - `docs/reference/plan-format-v3.md`
  - `docs/reference/status-schema.md`
  - `manifest/designs/finalize.md`
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:**
  - `docs/reference/builder-contract.md`
  - `docs/reference/plan-format.md`
- **Moves:** none
- **Requirements:** Delete `docs/reference/builder-contract.md` and `docs/reference/plan-format.md`.
  Re-point every inbound deep link to `builder-contract.md` at `docs/reference/webster-contract.md`'s summary-artifact section from card 8:
  the two links in `manifest/designs/finalize.md` (the `require_pr_to_base` paragraph and the See-also bullet), the See-also link in `docs/reference/plan-format-v3.md`, and the link in `docs/reference/status-schema.md`'s Status banner.
  In `manifest/designs/loom.md`, rewrite the three claimed lines in full rather than patching their links: the "Naming note" paragraph asserting that the two deleted builder packages and `builder-contract.md` are "a real, separate, already-shipped sibling implementer loop", the following sentence quoting `websterengine`'s package doc calling itself "a sibling of builderengine's batch-implementation loop", the sentence claiming `builder-contract.md` "documents the older Builder/plan-v2 pairing and does not yet have a Webster/plan-v3 equivalent", and the module-decomposition table row for `webster` that repeats both claims.
  All four now read as false, and the package-name zero-hit criterion reaches them regardless of ownership.
  Replace the naming note with a short statement that webster is the stack's implementer module and that its cross-module contract is `webster-contract.md`, and narrow the table row to name `internal/websterengine`/`internal/webstercli` and `webster-contract.md` with no sibling-loop clause.
  Do **not** touch `manifest/designs/loom.md`'s Plan-producer paragraph about the target format changing — its only builder-era link targets `plan-format.md` and it belongs to another task.
  In `docs/reference/status-schema.md`, repair only the `builder-contract.md` link in the Status banner and leave the adjacent `plan-format.md` link dangling;
  the surrounding "the loom analogue of …" prose is rewritten in batch 4.
  In `docs/reference/discussion-format.md`, make the minimal edits that stop the file asserting things this task falsifies: the two-file-split rationale currently grounds itself in "Builder's 'distilled digest, never raw prose' rule (see `builder-contract.md`'s digest contract)" — restate the rule on its own terms without the builder attribution or the link, keeping the edit small because a later task rewrites this line's attribution in producer-model terms;
  and the `approved:` paragraph's "because `lyx builder run` can be invoked standalone, outside loom" is false once builder is gone — restate it for `lyx webster run`.
  Leave that paragraph's `plan-format.md` reference dangling.
  The commit message for this card must state explicitly that the deleted `builder-contract.md`'s chain-rollback and recovery-ladder material — the one piece of forward-valuable design with no webster equivalent — is recoverable via `git show <parent-sha>:docs/reference/builder-contract.md`.
- **Commit:** `docs: delete builder-contract.md and plan-format.md, re-point deep links`

## Batch Tests

`verify: null` — this batch is pure documentation with no runnable surface.
Nothing in the repo parses `docs/reference/*.md`;
the only test that reads Markdown is `cmd/lyx/sandbox_coverage_test.go`, which scans `tools/sandbox/*SUITE.md` and is untouched here.
The batch is verified by the module-wide `go build ./...` at the batch boundary and, substantively, by the acceptance greps in batch 5 — in particular the `builder-contract` zero-hit pattern, which is the only gate that can see a link to a file that no longer exists.
