# Batch: docs

```yaml
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
batch: docs
number: 4
cards: 4
verify: go test ./internal/shedadapters/... ./internal/lyxcwd/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch carries the whole doc set the task owes: the package doc for `internal/shedadapters` and the three doc files whose existing claims become false the moment the package ships.
It depends on all three adapter batches because `doc.go` documents the as-built contract of all three, and because four of the five `manifest/designs/shed.md` corrections describe behaviour those batches decide.
No batch after this one exists, so nothing consumes its output.

Batch-local decision, additional to `## Shared Decisions` in the overview: every claim corrected here is named individually rather than summarised as "update the docs", because a partial edit would leave a doc self-contradicting — `manifest/designs/shed.md` states the reattach claim twice, in different words, and `manifest/roadmap.md`'s Done entry for the Shed skeleton forward-references the very Planned item this batch deletes.

## Cards

### Card 10: package doc for `internal/shedadapters`

- **Context:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/perch.go`
  - `internal/shedadapters/webster.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedengine/doc.go`
  - `internal/perchengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** write the package doc comment for `shedadapters` as the as-built contract, following the sectioned godoc style `internal/shedengine/doc.go` already uses.
  It must state: the three adapters and which engine each wraps;
  the three outcome-mapping tables, including that a gate producer reports an empty output pointer and that `SingleLLMProducer` reports the first entry of its `Spec.OutputFiles`;
  the told-not-derived discipline, naming the told producer name (log fields and error text only) and the injected clock (nil selecting the real one);
  the perch run-id scheme `<prefix>-<hash8>-<N>`, including that it advances only past a terminal block and that the hash segment exists to keep an edited profile from wedging the producer;
  and the shared cancellation rule, including that a genuine success verdict survives cancellation and is returned as done so a finished artifact and a paid-for session are never discarded.
  It must also state the three limitations plainly, not by implication: `SingleLLMProducer` never reattaches to a live session — it archives stale outputs and respawns, because no reattach entry point exists to call;
  neither `SingleLLMProducer` nor `WebsterProducer` installs a mid-run bridge, so a cancel is observed only once the run reaches a terminal outcome or its own configured deadline elapses, bounded by the shuttle spec's timeout and by webster's own whole-run timeout respectively;
  and the standalone perch pause verb is a silent no-op against an adapter-driven run dir, because the pause callback the adapter installs is the context bridge rather than the CLI's own flag-file closure, so the verb writes a flag nothing reads and reports success — the remedy being the product's own pause path, cancelling the context handed to Shed's run loop, never that verb.
  Keep the file to a doc comment plus the package clause, matching the sibling packages' own `doc.go` shape.
- **Commit:** `docs(shedadapters): add package doc for the three Shed engine adapters`

### Card 11: correct the five stale claims in the Shed design doc

- **Context:**
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/singlellm.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** apply exactly five corrections, leaving every other line of the file alone.
  (1) The status banner in the blockquote on line 3: the three engine adapters are no longer Planned — they ship as `internal/shedadapters` — so the banner names the package, and the doc's Documentation-Lifecycle survival rationale is restated on its own footing (this doc remains the authoritative narrative of Shed's own generic mechanism), since the current rationale rests on a Planned item this task deletes.
  (2) Line 38's "This is written down rather than assumed because three of the four planned adapters — `perch`, `Webster`, and a bespoke multi-spawn engine — own their own error taxonomies and are not designed yet", now false for two of the three: reword so the sentence states the obligation's stakes without asserting the adapters are undesigned.
  (3) Line 255's parenthetical "`SingleLLMProducer` wraps `shuttle`+`reed` and does this internally", which claims the full live-session/fresh-output/respawn three-case discipline: correct it to the as-built behaviour — the adapter archives stale outputs and respawns, and does not reattach.
  (4) Line 261's bullet under "What `Shed` does not provide", "Crash-recovery of live-session state (reattach vs. respawn) — inside `SingleLLMProducer`/`perch`/`Webster`'s own `Call()`", which restates the same reattach claim in different words: correct it the same way, so the two lines agree.
  (5) Line 278's description of `SingleLLMProducer` as "parameterized by an Input-format pointer, an Output-format pointer, and one instruction file": reword to say the parameterization lives in the caller's own Spec source, which the adapter evaluates once per call and never templates.
  Keep every existing inline link's file part and anchor resolving, and keep the repo's one-sentence-per-line markdown convention.
- **Commit:** `docs(shed): correct five adapter claims made stale by internal/shedadapters`

### Card 12: record the package in the overview

- **Context:**
  - `internal/shedadapters/doc.go`
  - `manifest/designs/shed.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** apply exactly three edits.
  (1) Add a repo-tree line for the new package immediately after the `internal/shedengine/` line (line 228), matching the surrounding column alignment and one-line description style.
  (2) Extend the `shed` module bullet (line 292) with a sentence naming the three shipped adapters and the package that holds them, in the same voice as the rest of the bullet.
  (3) Correct line 294's "the three engine adapters (`SingleLLMProducer`, the `perch` adapter, the `Webster` adapter) remain Planned" — they are implemented — while leaving the skeleton's own implemented marker intact.
  Keep every existing inline link's file part and anchor resolving, and keep the repo's one-sentence-per-line markdown convention.
- **Commit:** `docs(overview): record internal/shedadapters and mark the adapters implemented`

### Card 13: move the adapters item to Done on the roadmap

- **Context:**
  - `internal/shedadapters/doc.go`
  - `manifest/designs/shed.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** apply exactly three edits.
  (1) Remove Planned item 1 (lines 12-14, the three-adapters item) and add a corresponding entry at the end of the Done section — after the Shed skeleton entry, which is the section's current last item — stating what shipped: three reusable producer implementations in one new package, each a thin wrapper over an already-shipped engine, with the same design-doc link the Planned item carried.
  (2) Reword line 16's "`Discussion-Review` (wired via the `perch` adapter above)" in the loom Discussion-phase producers item, whose "above" dangles once Planned item 1 leaves the Planned section: point it at the shipped package instead.
  (3) Update the Shed skeleton's Done entry (lines 196-199), whose two claims both become false here — that the three adapters "remain their own Planned item above", and that the design doc survives its landing *because* of that Planned item.
  Restate the survival rationale on its own footing: the doc remains the authoritative narrative of Shed's generic mechanism, independent of any Planned item.
  Write every list item literally as `1.` per the file's own Maintenance section, keep every inline link's file part and anchor resolving, and keep the repo's one-sentence-per-line markdown convention.
- **Commit:** `docs(roadmap): move Shed's engine adapters from Planned to Done`

## Batch Tests

`verify: go test ./internal/shedadapters/... ./internal/lyxcwd/...` covers both halves of what this batch touches.
The `shedadapters` package is re-run because `doc.go` is production Go in it and must still compile.
`internal/lyxcwd` is the package that hosts the three enforcement tests these doc edits can break: the markdown-link integrity walk over `manifest/` and `docs/`, the fabric-vocabulary walk over production Go under `internal/` plus `internal/**/*.md`, and the geometry-literal walk — the first two of which read exactly the files edited here, and the third of which reads the new `doc.go`.
That is a scoped two-package run, not the full suite;
the repo-wide regression sweep is `pipeline.done_gate`'s job at Handoff, not this batch's.
