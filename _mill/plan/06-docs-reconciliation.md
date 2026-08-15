# Batch: docs-reconciliation

```yaml
task: 'Shed: outer phase-FSM skeleton'
batch: docs-reconciliation
number: 6
cards: 7
verify: go test -run 'TestEnforcement_MarkdownLinks|TestDocsLink' ./internal/lyxcwd/
depends-on: [2]
```

## Batch Scope

This batch reconciles `manifest/designs/shed.md` against every decision this task's discussion produced, then lands the two repo-wide doc updates the task-completion rule requires: `docs/overview.md`'s module map and `manifest/roadmap.md`'s Planned-to-Done move.
It is one batch because it is one editorial pass over one design's documentation, and splitting `shed.md`'s reconciliation from the overview and roadmap entries that describe the same shipped thing would invite the three to disagree.

It depends on batch 2 because it describes the loop as built.
It exposes no interface to a later batch.

Batch-local decisions, beyond `## Shared Decisions` in the overview:

- Cards 25 through 29 each edit `manifest/designs/shed.md`, running front to back through the document. Card 29 closes with the whole-document sweep that catches whatever the preceding four missed — the discussion's own list of known edits is explicitly non-exhaustive, so an implementer who treats cards 25 through 28 as a complete checklist will leave drift behind.
- Every `.md` file this batch touches follows the repo's semantic-line-break rule: one sentence per line, plus a break at internal independent-clause boundaries, no fixed-column hard wrap, and never a trailing double-space or backslash to force a line break.
- Every link added or moved must resolve, including `#anchor` fragments on `.md` targets, per the Markdown Link Integrity invariant this batch's verify enforces.

## Cards

### Card 25: shed.md — status banner and the producer contract

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/doc.go`
  - `internal/shedengine/producer.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite two things at the top of `manifest/designs/shed.md`.

  **The status banner.** It currently reads as a design sketch that is Planned.
  Replace that classification with the true one: the skeleton — the loop, the status file, the `ShedProducer` interface, and the producer-list validation — is **shipped** as `internal/shedengine`, while the three engine adapters remain Planned as their own roadmap item.
  Point the reader at the `internal/shedengine` package documentation for the as-built contract, and keep this doc positioned as the design's own narrative rather than a duplicate of it.
  Keep the naming paragraph and the existing pointer to the loom design's producer-list section.
  State explicitly why this doc survives its module landing, since the Documentation Lifecycle would otherwise imply deletion: it also describes the engine adapters, which have not shipped, and the roadmap's own Planned adapters item links here.

  **The four-value outcome prose.** The "Producer contract vs. producer definition" section describes the contract as returning one of *four* values — done, approved, stuck, blocked — while the section's own Go block declares exactly two.
  That prose predates the two-value contract and must be corrected, not preserved: a producer returns exactly `Done` or `Stuck`.
  In the same place, add the two contract obligations a producer must honour and `Shed` cannot enforce: return exactly one of those two values, and surface context cancellation as a non-nil `error` from `Call`, never as `Stuck`.
  Explain the second one's stakes — `Shed` cannot tell a `Stuck` return with a cancelled context from a genuine verdict, so a producer that reports cancellation as `Stuck` would silently consume bounce budget, or escalate to blocked, for what was an operator stop.
  Note that this is written down rather than assumed because three of the four planned adapters own their own error taxonomies and are not designed yet.
- **Commit:** `docs(shed): re-banner as shipped and correct the producer contract`

### Card 26: shed.md — the loop's exact mechanics

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the six-step loop in the "The `Shed` loop — exact mechanics" section of `manifest/designs/shed.md` so it describes the loop as built.
  The corrections, all of which change behaviour the current text gets wrong:

  - **Step 6 routes back to step 1, not step 2.** The status file is re-read at the top of every iteration, which is what lets step 3's pause check see a pause requested *during* the producer call that just returned. Say why the old routing was a bug: a pause requested during a twenty-minute producer call was both never observed and silently destroyed.
  - **Steps 5 and 6 are one atomic persist, not two writes.** Routing is computed first; the history append, the new `current_producer`, and the new state and error all land in a single locked read-modify-write. Say why: as two writes, a crash landing between them leaves `current_producer` still naming the producer that just finished, so the next run re-calls it and appends a duplicate history entry — defeating the exact crash-safety property step 5 exists to provide, which only holds if "after it" and "step 6 decided" are the same instant.
  - **The persist merges rather than rewrites**, carrying `pause_requested` and `product` forward from the on-disk copy, and it aborts without writing when the file is found missing — so a status file deleted mid-run is never silently re-created from a zero value.
  - **Add the already-done short-circuit** with its exact position: after step 1's read, before step 2's lookup. State its consequence as a decision — a done file whose `current_producer` is no longer in the list returns cleanly rather than hard-erroring — and its rationale, that a finished task must not become un-queryable because someone later edited the producer list. State that blocked and failed deliberately do not short-circuit, so a human can resume after fixing the cause.
  - **Add the read gate's own strictness rule:** a persisted state outside the five legal values, the empty string included, is a hard error at the gate.
  - **Step 6 gains a fourth branch**, for an `Outcome` that is neither `Done` nor `Stuck` returned with a nil error: state failed, an error naming both the offending value and the producer, a non-nil `Run` error, and a history entry still appended recording the literal value received.
  - **Step 6's error branch is cancellation-aware.** When `Call` returns a non-nil error and the context is cancelled, the iteration routes to the pause exit — paused, a nil error — rather than to failure. Pin the predicate as the context's own state, and say why an error-sentinel match would be wrong in the opposite direction: a producer whose own internal derived context times out is a genuine producer failure while the parent context is healthy. State that no history entry is appended on that path and `current_producer` is left unchanged.
  - **Pin the bounce budget's exact boundary:** `MaxBounces` bounces are permitted and the next `Stuck` that would otherwise route is the one refused, so a budget of three performs three bounce-backs and blocks on the fourth. State the default is ten, and that the budget is per-`Run`-call and held in memory, deliberately unpersisted — the status file carries no bounces-used field, so a crash-restart or a human-resumed blocked run starts again with the full budget.
  - **Both blocked causes carry an exact error string** — one for the exhausted budget, one for a stuck producer with no target — and `Result.Reason` carries the identical text, one string written to two places rather than two phrasings that could drift apart.

  Keep the section's existing voice and its unconditional-re-call subsection, which is still correct.
- **Commit:** `docs(shed): correct the loop's routing, persist, and exit mechanics`

### Card 27: shed.md — the Go type blocks

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Bring every Go block in `manifest/designs/shed.md` into agreement with the shipped declarations in `internal/shedengine`.

  **The `Shed` struct block** gains `StatusLockPath` beside `StatusPath` and `LockPath`, with the never-the-same-file rule stated in the surrounding prose: the persistence package acquires its lock with the blocking form, so one shared path would hang on the first persist rather than failing, and the pre-loop validation rejects the mistake so it fails loud.
  Note that the caller already has both paths on hand.
  Update `MaxBounces`'s comment with the concrete default and the per-call, in-memory scope.

  **Two types this doc references but never declares** get real Go blocks with their JSON tags, alongside the blocks that already exist: `Status`, the persisted status file, currently pinned only as a JSON example;
  and `HistoryEntry`, which `Result.History` is typed on with no declaration anywhere.
  `Status`'s fields are `current_producer`, `state`, `error`, `pause_requested`, `activity`, `history`, and `product`.
  `HistoryEntry`'s are `producer`, `outcome`, `output`, and `at`.
  The `activity` sub-object needs a named type of its own with its three fields, and the `state` field needs its own named string type with its five legal values — the three clean-exit values being the literal same strings as the run-outcome constants, so mapping between them is identity rather than a lookup table, plus `running` and `failed`, which no run outcome ever carries.

  **The `Result` and `Run` prose** gains the two rules the doc currently leaves implicit: a caller must branch on the outcome before reading the reason, and `Result` is meaningless unless the returned error is nil, because the outcome's zero value is not one of the three legal constants and every hard-error path returns an unpopulated result alongside its error.

  **`history[].at`** is pinned as RFC3339 UTC written from a direct clock call, with no injectable clock — tests assert the format and the ordering, never a literal.
  **`Result.History`** is pinned as the full persisted history as it stands when `Run` returns, not only the entries that invocation appended.
- **Commit:** `docs(shed): declare the status types and pin the struct and Result rules`

### Card 28: shed.md — the status file section

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/activity.go`
  - `docs/reference/status-schema.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the status-file section of `manifest/designs/shed.md`.

  **The JSON example gains the `product` field**, which is currently absent from it entirely.

  **Replace the "`Shed` is the file's only writer" sentence** with the explicit three-way ownership split, because that sentence is false and licenses exactly the whole-file clobber the merging persist exists to prevent.
  Shed-owned and rewritten on every persist: `current_producer`, `state`, `error`, `activity`, `history`.
  Shared and write-to-clear: `pause_requested`, which an outside actor sets true and `Shed` only ever writes false, exactly once, in the same persist that records the paused state.
  External-writer-owned and only ever carried through: `product`.
  Note that the seed itself is written by a spawn-time command, not by `Shed`, and that `pause_requested` living in-status rather than in a separate flag file is a deliberate divergence the status schema reference already pins.

  **Add the external-writer lock contract**, and qualify the merge-safety claim rather than stating it unconditionally: any actor other than `Shed` that writes this file must go through the persistence package using the same status lock path `Shed` was told, because that lock is advisory and keyed on the caller-supplied path.
  A writer that ignores it can still lose its write and still clobber Shed's.
  Say plainly that this cannot be enforced from Shed's side and is therefore written down, alongside the two producer-side obligations that already are.

  **State that `pause_requested` is a request `Shed` consumes, not a latch** — cleared in the same persist that records the paused state, so no window exists in which a stale true flag sits on disk, and the durable record of "this run is paused" is the state field.
  Say why: without it, the next run re-reads a still-true flag and pauses again immediately, forever.

  **Pin `activity.last`'s exact composed format** — the producer name, a spaced right-arrow, and the outcome — rather than "formatted for a human", and say why: a test asserts this field, and an unpinned format cannot be asserted, only approximated.
  Keep `wait` scoped to the blocked and failed states only.

  **Add the strictness scoping note.** Strict decoding is the contract of the top-of-iteration read gate, not of the persist's internal merge base, which re-reads leniently.
  Malformed JSON still fails loud on both paths.
  The one behaviour leniency permits is an unknown top-level key written by an external actor *after* the read gate passed, and its fate must be stated honestly: it is silently destroyed by the full-struct marshal, not surfaced, and the next strict read then sees a clean file and has nothing to reject.
  Say why that is acceptable — `product` is the sanctioned channel for what an external writer owns, so a key outside it is a mistake nothing here promises to preserve — and do not claim the key would be caught later, because it would not.
  A key present *before* the read does hard-error at the gate.

  **State that `product` carries no compatibility claim for loom's shipped schema.**
  The status schema reference mandates `phase`, `stage`, and `narration` as top-level fields and pins a different history shape, none of which a `product` sub-object satisfies;
  reconciling the two is loom's own later rewiring task.
- **Commit:** `docs(shed): rewrite the status-file section for ownership, locking, and strictness`

### Card 29: shed.md — the pre-loop section and the whole-document sweep

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/run.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/errors.go`
  - `internal/shedengine/doc.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/activity.go`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** First, rewrite the section of `manifest/designs/shed.md` describing what `Run` does before step 1.

  **The validation list grows** from three rules to the full set: an empty producer list, a duplicate name, an `OnStuck` naming an absent name, an **empty** name, a **nil** producer, a negative bounce budget, the two lock paths naming the same file, and any of the three paths being empty.
  Give the two non-obvious ones their reasons.
  An empty name is rejected because the empty string is already load-bearing twice — it is the escalate-to-human sentinel, so a producer named with it would make that sentinel ambiguous, and it is the zero value a partial seed leaves in `current_producer`, which the lookup would then resolve successfully and *run*, turning a corrupt status file into silent execution.
  A nil producer is rejected because it panics at the call step rather than failing loud, and a panic inside a long unattended run is strictly worse than a validation error at second zero.

  **`Run` creates both lock parents** before acquiring, because the locking package opens with create but never creates parents — which is why the loom and treadle engines both do this already.
  State that this is not path derivation: the paths are still told, `Shed` only ensures the told path is usable.

  **Correct the "neither touches the status file" wording.** Acquiring a lock is not a no-op on disk — the lock file itself is created.
  The honest claim is that `Shed` never creates or modifies the **status file** outside the persist.
  Find and fix every phrasing in the document that says otherwise.

  Then run the **whole-document sweep**, which is this card's real work and not an afterthought.
  Re-read `manifest/designs/shed.md` end to end against the whole Decisions section of `_mill/discussion.md`, and against the shipped source in `internal/shedengine`, and fix anything that has drifted — the preceding cards' lists are known edits, deliberately not a complete checklist, and treating them as exhaustive is how the rest of the document silently rots.
  Check in particular: every cross-reference and anchor still resolves after the edits;
  no sentence still describes a behaviour an earlier card changed;
  no Go block disagrees with its shipped counterpart;
  and the doc nowhere claims a property the implementation does not have.
- **Commit:** `docs(shed): reconcile the pre-loop section and sweep the whole document`

### Card 30: docs/overview.md — the module map

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/doc.go`
  - `manifest/designs/shed.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make three edits to `docs/overview.md`.

  **The repository tree.** Add a line for `internal/shedengine/` immediately after the `internal/treadleengine/` line, matching the surrounding alignment and one-line description style: the generic outer phase-FSM that walks one flat producer list, honoring resume, crash-recovery, and pause at producer granularity.

  **The Modules section.** Add a bullet for **shed** immediately after the **loom** bullet, since loom is the consumer that gives it meaning.
  State what it is, that it lives in `internal/shedengine`, and that it has no `lyx shed` verb of its own by design — a product's own CLI constructs one with its producer list and calls `Run`, and a bare verb would be a command with no list to walk.
  Mark the skeleton implemented and the three engine adapters Planned.
  Point at the `internal/shedengine` package documentation and at `manifest/designs/shed.md`, and make sure both links resolve from this file's location.

  **The execution-stack section.** Add `shed` to the layered stack block above `loom`, and update loom's own annotation there so it reads as building on shed and perch rather than on perch alone.
  The sentence immediately after that block currently claims the cross-OS spawn primitive is the one remaining internal, non-CLI layer of the stack;
  adding shed makes that false, so reword it minimally to name both rather than leaving a claim the block itself contradicts.

  Leave the existing parenthetical that distinguishes the abandoned earlier reed model/view draft named `shed` from this `Shed` — it becomes more load-bearing now that a real `internal/shedengine` exists, not less.
  Do not restructure any section, and change nothing else in the file.
- **Commit:** `docs(overview): add shedengine to the module map and the execution stack`

### Card 31: manifest/roadmap.md — Planned to Done

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/shedengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move the **Shed: shared outer phase-FSM, no predefined slots** item in `manifest/roadmap.md` out of the Planned section and into the Done section, appended as the last entry there, immediately before the `## Maintenance` heading.

  Rewrite it for the Done section's own conventions, which the Maintenance section states: an entry is a name plus one or two sentences of what and why, never a design writeup, and a Done entry points at the module's own package documentation.
  So: name it the same way, state in one sentence what shipped, state in a second that this is the skeleton only and that the three engine adapters remain their own Planned item above, note that it landed the Shed Producer-Seam Invariant in `CONSTRAINTS.md` the way the neighbouring entries note theirs, and point at the `internal/shedengine` package documentation.

  Keep the pointer to `manifest/designs/shed.md` as well, and say in the entry that the design doc survives because it also covers the still-Planned adapters — otherwise a future reader applying the Documentation Lifecycle's delete-on-landing rule will delete a doc the Planned adapters item still links to.

  Do not renumber anything: every item is written literally as `1.` and renders sequentially within its own section, so removing the first Planned item and appending a Done item needs no number edits anywhere.
  Leave the three remaining Planned items in their existing order, and check that the adapters item's own link into `manifest/designs/shed.md` still resolves after that document's card-25 through card-29 edits — if a heading it anchors to was renamed, fix the anchor here.
- **Commit:** `docs(roadmap): move the Shed skeleton item from Planned to Done`

## Batch Tests

`verify: go test -run 'TestEnforcement_MarkdownLinks|TestDocsLink' ./internal/lyxcwd/`.

This batch changes only Markdown, so the runnable surface it affects is the Markdown Link Integrity invariant, enforced from `internal/lyxcwd/docslink_test.go`.
That matters concretely here rather than as a formality: this batch adds links in three files, moves a roadmap entry whose own link points into `manifest/designs/shed.md`, and rewrites headings in that same document — so an anchor renamed by card 26 or 28 and still referenced by card 31 is exactly the failure this run catches.
The `-run` pattern selects the enforcement test plus the three link-parsing unit tests it is built on, so a failure is localised to either the parser or the corpus.

No Go source changes in this batch, so `go test ./internal/shedengine/...` would prove nothing new;
the task-wide done gate runs the full suite regardless.
