# Batch: docs-sweep

```yaml
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
batch: 'docs-sweep'
number: 4
cards: 6
verify: go build ./... && go test ./internal/lyxcwd/...
depends-on: [3]
```

## Batch Scope

This batch rewrites every documentation statement the preceding three batches falsified, and backs the rewrite with a grep sweep so the inventory is mechanical rather than hand-enumerated.
It is one batch because the doc edits share one argument that has to read consistently across five files: sequential `Done` routing is gone, the bounce budget is per-producer and episode-scoped, and no run-wide cap survives.
Splitting them would let two halves of the same argument land in different commits and drift.

Batch-local decision on tone, inherited from the discussion and worth restating because it is the easiest thing to get wrong: the new prose must state the **inversion explicitly**.
Naming the old per-`Run`-call reset, saying it was deliberate, and saying why it was overturned, rather than describing only the new behavior — a doc that describes only the new behavior leaves the missing reset looking like an accidental omission, which is exactly how a future reader reintroduces it as a bugfix.
The same applies to the run-wide cap: the old doc argued *against* a per-producer budget on the grounds that an A↔B cycle would then run twice the budget, and that argument must be answered in place, not silently deleted.

## Cards

### Card 15: Rewrite the routing and budget sections of the Shed design doc

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/validate.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite every falsified passage in `manifest/designs/shed.md`, in place, keeping the file's existing structure and its semantic-line-break style — one sentence per line, plain newlines, no fixed-column wrapping.
  The `Done` bullet under the loop's step 6 currently says `Done` advances to the next entry and that going past the last entry writes the done state; it becomes: `Done` routes to this producer's `OnDone`, an empty `OnDone` finishes the run from any list position, and `current_producer` still keeps the just-finished producer's own name rather than the empty string.
  The `Stuck` bullet in the same list keeps its shape but must name the budget as this producer's own episode budget rather than a shared one.
  The whole bounce-budget paragraph that argues for a single total cap must be replaced by the new position, and must answer its own former argument rather than dropping it: the aggregate is still bounded, by a **sum** rather than a single number, so within one set of episodes the total is at most the sum of the participating producers' effective budgets, the A↔B cycle really does cost twice the budget, and that is now a deliberate price.
  Across episodes the lifetime total is unbounded in principle, because a `Done` resets one producer's episode — but a reset is only ever earned by a producer succeeding, or granted once after a hard failure a human had to resolve, so what grows without bound is progress, not wasted spend.
  Name the residual rather than hiding it: a cycle whose every member alternates stuck and done forever is bounded by nothing here, and its stop is pause or cancellation, checked at the top of every iteration.
  The exact-boundary paragraph keeps its arithmetic — a budget of three performs three bounce-backs and blocks on the fourth stuck — but restated per-producer, and its "per-`Run`-call and held in memory" sentence is replaced by the episode rule: the count is of this producer's own stuck entries since its own last done entry, read from the persisted history, so it spans invocations, crashes, and human resumes.
  State the failure-path terminator in the same breath as the episode rule, so the scan's "stop at the first done" is not read as "stop at the first success".
  The `ProducerDef` code block gains the three new fields with their inline comments, and the `Shed` block's `MaxBounces` comment becomes the inherited-default wording.
  The prose paragraph under that block, which explains `MaxBounces` as one field on `Shed` rather than per-`ProducerDef`, is rewritten to describe the two-level inheritance.
  The validation bullet list under what `Run` does before step 1 gains the four new rules, and states what is deliberately not validated: reachability, and multi-producer done cycles.
  Add the escape-hatch paragraph the discussion commits to: the supported remedy is fixing the underlying failure so the producer returns done, which resets its episode by itself; when that is impossible the remedy is raising that producer's budget strictly above its current episode stuck count, which is one more than the budget immediately after the first block and grows by one per re-block, so raising it by one is never enough; this is a source edit and rebuild today, not an operator action, since the budget reaches no CLI flag and no config key; and hand-editing the persisted history is not endorsed, because it contradicts the status spec's one-entry-per-producer-call rule and the append-only property the derivation depends on.
  Say in those words that a producer which structurally never returns done has a task-lifetime cap rather than a per-round one, and that such a row's budget must be sized accordingly.
  State the silent-terminal risk in the doc's own words, so a reader knows an omitted `OnDone` ends the run quietly.
  State that a done cycle is unbounded by design, that validation catches only the single-producer self-reference case, that pause and cancellation are the stop, and that the cost while it spins is not merely wasted iterations — every iteration appends a history entry and the persist rewrites the whole slice, so an unattended done cycle writes quadratically many status-file bytes into an unboundedly growing file.
  Finally, qualify the two sentences that frame a product as its producers "in what order" and review as "always the next, separate producer in the list": the first is about which producers distinguish one product from another and survives with a qualification that order is now display order, not routing; the second is falsified as written and must say review is a separate producer reached by explicit routing.
- **Commit:** `docs(shed): rewrite Done routing and the bounce budget for per-producer episodes`

### Card 16: Fix the sequential-routing sentences in the loom design doc

- **Context:**
  - `manifest/designs/shed.md`
  - `internal/shedengine/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Correct the three falsified sentences in `manifest/designs/loom.md`, keeping the file's semantic-line-break style.
  The sentence saying that on stuck the engine bounces back to an earlier producer in the list, with its surrounding sequential framing, must say the bounce target is the producer's own explicit stuck target, which may sit anywhere in the list, and that the same explicitness now governs the done direction too.
  The sentence framing loom's identity as which producers are in the list "in what order" takes the same qualification card 15 applies to its twin: order is display and enumeration order, not routing.
  The sentence saying review is always the next, separate producer in the list is falsified as written — review stays a separate producer, reached by explicit routing rather than by position.
  Do not restate the budget design here; that argument lives in the Shed design doc and this file should point at it rather than duplicate it.
- **Commit:** `docs(loom): drop the sequential-routing framing`

### Card 17: Update the shedengine package documentation

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-read `internal/shedengine/doc.go` whole and correct every statement this task falsified.
  The specific one already known: the opening paragraph says what makes a product a product is which producers are in its list and in what order, which the task's own "the producer list becomes pure storage with zero routing meaning" position contradicts.
  Both occurrences of that framing in the file take the same qualification — which producers, with the order being enumeration and display order rather than routing.
  Add a short section documenting the routing contract itself, since a reader arriving at this package should not have to open the design doc to learn that `Done` routes through `OnDone` with no positional fallback, that an empty `OnDone` finishes the run from any position, and that the bounce budget is per-producer and episode-scoped, counted from the persisted history rather than held in memory.
  Keep the existing sentence about a producer that reports cancellation as stuck silently consuming bounce budget — it stays accurate, and the budget it now consumes is that producer's own.
  Do not add any import and do not change the package clause.
- **Commit:** `docs(shedengine): document OnDone routing and the episode-scoped budget`

### Card 18: Note that the persisted history is budget-bearing

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one prose line to `contracts/specs/loom-status-spec.md` stating that the history array is budget-bearing after this task — it is no longer only a log, it is the sole storage of every producer's bounce budget, so it must never be truncated or compacted.
  Place it where the spec already describes the history array's one-entry-per-producer-call rule, so the warning sits next to the property it depends on.
  The schema itself is unchanged: add no field, and change no existing field's type or meaning.
  The fresh-start check's existing explanation, which already depends on a stuck entry being appended unconditionally on every stuck route including the escalation, stays exactly as it is — the same unconditional append is why a block-path entry counts toward the budget, and the new line may cross-reference it rather than restating it.
  Keep the file's semantic-line-break style.
- **Commit:** `docs(spec): note that history[] is budget-bearing and must not be truncated`

### Card 19: Move the roadmap item to Done and resnapshot the parallel-work map

- **Context:**
  - `manifest/designs/shed.md`
  - `internal/loomshed/loomshed.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `manifest/parallel-work.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, move the Planned item named "shedengine: per-producer bounce budget + explicit `OnDone` routing" — the first item under the Perch to Shed flattening group — into the Done section.
  Follow that file's own Maintenance rules: every item is written literally as a `1.` and numbering is automatic, so no renumbering is needed anywhere; the moved entry is shortened to a name plus one or two sentences of what and why, and points at the module's own package documentation rather than carrying a design writeup.
  Leave the remaining Planned items in that group untouched.
  Two of them refer back to this one and both must survive the move intact: the item immediately after it points back as "(previous item)", and a later item in the real-LLM-producers group points back by this item's full bold name.
  Neither wording is falsified by the move — a Done item is still a legitimate referent — so change neither.
  `manifest/parallel-work.md` is a point-in-time snapshot of which Planned items can be spawned in parallel, and its own header says to recompute it whenever tasks land, so the edit here is to bring it in line with this task having shipped rather than to correct a sentence in place.
  Remove this task's bullet from the no-caveats section, since a shipped task is no longer something to start.
  Update the running-now paragraph so it names this task as landed, in the same shape it already uses for the previously-landed group.
  Update the light-caveat paragraph below it: its whole caveat is that two downstream tasks are cleanest once this task lands, and that precondition is now satisfied, so the paragraph must say the dependency is met rather than continue to describe a wait.
  Do not correct the removed bullet's `internal/shedengine`-only scope claim in place — the bullet is deleted, so there is nothing left to correct, and the claim was false anyway once the loom producer-list migration became mandatory.
  Keep both files' semantic-line-break style.
- **Commit:** `docs(roadmap): ship the per-producer bounce budget item`

### Card 20: Run the doc-falsification sweep and disposition every hit

- **Context:**
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `manifest/parallel-work.md`
  - `contracts/specs/loom-status-spec.md`
  - `internal/shedengine/doc.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/loomshed/loomshed.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the grep sweep this task's doc inventory depends on, and commit a disposition for every hit rather than working from the hand-enumerated list alone — a hand-enumerated list is exactly what goes stale.
  Sweep the Go doc comments under the internal tree, every markdown file directly under the manifest directory, every design doc beneath it, and the contract specs, for each of these phrases: "in what order", "next entry", "next, separate producer", "next producer in the list", "bounce budget", "MaxBounces", "last entry", and "bounces back".
  For every hit, record one of two dispositions in the commit message: rewritten by a named card in this task, or still accurate with the reason why.
  Known survivors, to be confirmed rather than assumed: the two cancellation comments in the loom and landing context helpers say a producer reporting cancellation as stuck would silently consume bounce budget, which stays true and now refers to that producer's own budget; the "in what order" hit in the scout design doc is about an unrelated ordering question and is not this task's concern.
  Edit `internal/loomcli/wiring.go`'s comment on the line that leaves the budget field zero: its claim that the engine's own default applies is still true, but "default" now means the inherited per-producer default rather than a run-wide total, and the comment must say which.
  Do not change that file's behavior — this is a comment-only edit, and the field stays zero.
  If the sweep turns up a falsified statement in a file no card in this task lists, do not silently edit it: name it in the commit message and report it, because a file outside the plan's declared surface is a plan defect rather than a fix to fold in.
- **Commit:** `docs(loomcli): clarify what the inherited bounce-budget default now means`

## Batch Tests

`verify: go build ./... && go test ./internal/lyxcwd/...` is scoped to exactly what this batch can break.
The build covers the two Go files it edits — the package documentation file and the wiring comment — where the only realistic failure mode is a malformed comment block or an accidental edit outside the comment, both of which a compile catches immediately; neither file has behavior this batch changes, so running their packages' test suites would add minutes and prove nothing beyond what the build proves.
`internal/lyxcwd`'s own suite is the repository's markdown link-integrity check, and it is the one real gate here: this batch edits five markdown files, four of which carry cross-document links — only the parallel-work map has none — and a moved roadmap item or a rewritten design-doc section is precisely the edit that breaks a link target.
The whole-repository regression check is `pipeline.done_gate`, which runs both the untagged and integration-tagged suites before the task is marked done.
