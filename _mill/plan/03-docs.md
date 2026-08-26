# Batch: docs

```yaml
task: "Fix Bouncer anchor-path and run-dir clearing"
batch: "docs"
number: 3
cards: 2
verify: null
depends-on: [2]
```

## Batch Scope

This batch lands the two documents neither code change falsifies but the project's task-completion rule owes: an addition to the gate's design home in `manifest/designs/loom.md`, and the roadmap item's move from Planned to Done.
Every doc comment and yaml comment a code change *does* falsify already landed with that change, in batch 1 or batch 2 — this batch is only what is left.
It depends on batch 2 because the roadmap Done entry states what actually shipped, which is not knowable until both defects are in.

`docs/overview.md` is deliberately not touched: the module table and the execution stack are both unchanged by this task, and the project's task-completion rule scopes that file to changes that move one of them.
No new `CONSTRAINTS.md` invariant is recorded, per the `no-new-constraints-invariant` Decision in `_mill/discussion.md` — the segment re-entry rule is one adapter's internal round-artifact contract, whose durable home is `internal/shedadapters/doc.go` (batch 2, card 11), and the anchor fix is an application of the existing Cwd Resolution Invariant rather than a new rule.

Batch-local decision, on top of `## Shared Decisions`: both files are prose, so this batch's `verify:` is `null` — it has no runnable surface of its own.

## Cards

### Card 13: state the segment re-entry rule in the gate's design home

- **Context:**
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/burler.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two or three sentences to the "The gate" section of `manifest/designs/loom.md`.
  This is an addition, not a correction: nothing currently in that section is falsified by this task, because it delegates to the package documentation and names neither a root nor the four-mode branch.
  It is owed because segment re-entry is a design-level property of the gate — a gate that has already approved and is entered again re-judges rather than replays.
  State that rule, and state the accepted budget asymmetry beside it: a re-entered segment's `Bouncer` row gets a fresh bounce budget while its `Burler` row does not, so a second generation runs on the `Burler`'s leftover budget and can halt the run by exhausting it, which fails safe to a human.
  Leave the mechanism — the trigger, the archive-aside, the recreate — to `internal/shedadapters`, which the section already delegates to;
  do not restate it here.
  Place the addition where it reads naturally within the existing section, after the paragraph describing the black box's two exits and its `Bouncer`/`Burler` pairing, and before the "Which segment fixes what, and commits how." paragraph.
  Change no heading text and no anchor, because `manifest/roadmap.md` links to `designs/loom.md#the-gate` and card 14 relies on that link resolving.
  Write it with semantic line breaks, one sentence per line.
- **Commit:** `docs(loom): record the gate's segment re-entry rule and its budget asymmetry`

### Card 14: move the roadmap item to Done, without its refuted route claim

- **Context:**
  - `internal/shedadapters/doc.go`
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move the Planned item "loom: review segments resolve `_lyx` paths against the wrong root and don't clear their Bouncer run directory on re-entry" out of the "### loom: real LLM producers" subsection of `## Planned` and into `## Done`, keeping its `See [designs/loom.md](designs/loom.md#the-gate).` continuation line.
  Write it literally as `1.` like every other entry — numbering renders automatically and restarts per section, so no other item's number changes.
  The Done entry must not carry the Planned item's route claim forward.
  The Planned text calls defect 2 "confirmed present in the shipped `Discussion-Validate` → `Discussion-Write` → `Discussion-Bouncer` path", which `_mill/discussion.md` refutes: nothing downstream of `Discussion-Bouncer` (whose `on_done` is `Plan-Write`) routes back to `Discussion-Write`, so that segment's exposure is cross-invocation and crash-window, not an in-run bounce-back.
  Write instead what shipped — both defects fixed across all three review segments, the segments' `_lyx` paths now resolving against the anchor path their commit seam already anchored at, and a `Bouncer` re-entered after approving now archiving its run directory and re-judging rather than replaying a settled verdict.
  Keep it to a name plus one or two sentences, per the file's own Maintenance rules — the durable detail lives in `internal/shedadapters`' package documentation and in `manifest/designs/loom.md`, which the continuation line points at.
  The "### loom: real LLM producers" subsection's own lead-in paragraph ends with "the item below is unblocked", which names the item this card removes;
  if removing the item leaves that subsection with no items, remove the now-empty subsection heading and its lead-in paragraph along with it rather than leaving a heading pointing at nothing.
  Change no other entry in any section, and add no `designs/<name>.md` file — a Done entry points at the module's own package documentation instead.
  Write it with semantic line breaks, one sentence per line.
- **Commit:** `docs(roadmap): move the Bouncer anchor-root and run-dir item to Done`

## Batch Tests

`verify:` is `null`: both cards edit prose only, and neither `manifest/designs/loom.md` nor `manifest/roadmap.md` has a test that reads it — there is no runnable surface for this batch to check.

The two properties worth confirming by hand while implementing are that `manifest/roadmap.md`'s `designs/loom.md#the-gate` link still resolves after card 13 (which is why card 13 is barred from changing that heading), and that removing the Planned item leaves no dangling "the item below is unblocked" lead-in behind it.

Repo-wide coverage still applies at the task boundary: `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) runs from the git root before mill-go marks the task done, so a stray non-doc edit in this batch could not pass unnoticed.
