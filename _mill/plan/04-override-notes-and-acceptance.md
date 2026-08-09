# Batch: override-notes-and-acceptance

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: override-notes-and-acceptance
number: 4
cards: 2
verify: go build ./... && go test ./...
depends-on: [2, 3]
```

## Batch Scope

This batch writes the override notes that record, in the repo rather than in torn-down task state, every instruction of `manifest/designs/shed-followups.md` this task departed from — then runs the full acceptance gate end to end.
It is one batch because the notes can only be written once the departures are facts on disk, and because the final gate is what certifies them.
It depends on batches 2 and 3 together because the notes assert what both of them landed, and because the acceptance grep is only meaningful once every hand edit is in.

Batch-local decisions beyond `## Shared Decisions`:

- `shed-followups-override-notes` — the notes are **hand-written, outside the scripted sweep**. The file is excluded from the sweep in full, so the sweeper never sees it, and the notes must be typed rather than generated.
- The file keeps its stale citations of `docs/reference/plan-format-v3.md` at four places. This is accepted, not overlooked: the file is a historical record of what each task was told at scoping time, and those citations were accurate at that moment. Rewriting them would make the record claim the scoping task knew the post-rename name. The override notes are where a reader learns the file moved. Do not "fix" them.

## Cards

### Card 13: Write the three override-note blocks into shed-followups.md

- **Context:**
  - `CONSTRAINTS.md`
  - `docs/reference/plan-format.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/shed-followups.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three blocks, each opening with the bolded line `**Override recorded 2026-08-09 (task B, as landed).**` — the file already carries four `**Override recorded 2026-08-09 (task A, as landed).**` blocks as the precedent for exactly this shape, so match their formatting and indentation.
  Every block uses semantic line breaks (`markdown-semantic-line-breaks`).
  Locate each insertion point by its quoted text, not by line number.

  **Block 1 — in section `## B — plan-format-drop-v3-suffix`**, in the `### Scope` subsection, immediately after the sentence "This task changes paths and names only, never prose."
  Record every one of B's own instructions this task departed from:

  1. The stated **five**-pattern set became six. The five missed the doc title's space variant `# Plan format v3`, so the stated zero-hit criterion would have passed with the renamed doc still titled v3.
  2. The **unqualified "repo grep"** became a grep scoped to an exclusion set — this file, line 18 of `manifest/roadmap.md`, and `_mill/` — because the `### Acceptance` sentence naming the pattern set is itself a pattern-bearing line, and a blind sweep destroys the criterion it defines. Name the accepted consequence: four citations of the old path survive in this file, by design.
  3. **"This task changes paths and names only, never prose"** is superseded. The repo-wide v2 erasure rewrote prose in `docs/reference/plan-format.md`, `manifest/designs/loom.md`, `manifest/roadmap.md`, and Go comments across three packages.
  4. The `### Why` subsection's **rejected alternative** — "renaming the file but keeping in-text `v3` as a historical label" — was honoured rather than overridden, and extended: the four bare-`v3` labels in `internal/planparser` comments were rewritten too. Record it explicitly, because it is a *rejection* rather than an instruction and a reader could otherwise conclude task B left the class alone.
  5. The `### Sequencing` claim that this task "deliberately leaves `loom.md:29` self-contradicting" no longer holds — task B rewrote that line in full.
  6. The starting inventory's claim that **`CONSTRAINTS.md`'s Planparser Sole-Parser Invariant** needs rewording is stale. Read the invariant in `CONSTRAINTS.md` and confirm before writing this: it carries no version reference and no link to the doc, and the file's only `v3` occurrences are its two `gopkg.in/yaml.v3` import-allowlist entries, which are the hard exclusion. Task B edited nothing there.
  7. The `#### Hard exclusion` subsection's claim that the `gopkg.in/yaml.v3` import "appears in ten Go files" is wrong — the verified count is **32**. Correct the figure so the next reader is not misled about the blast radius of a broad `v3` replace.
  8. The same subsection's claim that "This task's script names the exclusion explicitly" is wrong — the script names no exclusion. All six patterns require a `plan` prefix, so the import string is unmatchable by construction; the exclusion is verified by a post-sweep count rather than implemented.

  **Block 2 — in section `## C — format-docs-name-producers`**, on the numbered item beginning "Rewrite `plan-format.md:5`'s "Coexistence, not replacement" section".
  Record that task A already rewrote that section away and task B then deleted the surviving retired-v2 blockquote, so C's obligation there is discharged;
  C's remaining work on the file is the producer-model rewrite only.

  **Block 3 — in section `## E — shed-model-contradiction-sweep`**, two notes:

  - On the `#### Part three` bullet for `:29`, which says task B "deliberately leaves this self-contradicting for this task to repair": record that task B rewrote the line in full instead, so E should verify it rather than repair it. E remains `loom.md`'s final owner for everything else on that list.
  - In `#### Part four`'s `manifest/roadmap.md` list: record that task B deleted the "v3 is the live plan format now that its predecessor is retired." sentence from the Done item, since B's own sweep of the item's heading is what made it incoherent.

  Do **not** edit anything else in this file, and do not repair its four stale citations of the old path.
- **Commit:** `docs(shed-followups): record task B's overrides for tasks C and E`

### Card 14: Run the full acceptance gate

- **Context:**
  - `docs/reference/plan-format.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/shed-followups.md`
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/designs/webster-parallel-execution.md`
  - `internal/state/state_test.go`
  - `internal/gitrepo/reset_test.go`
  - `internal/yamlengine/reconcile_test.go`
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/burlerengine/doc.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff card. Run the whole acceptance gate in order and report each result.
  A failure here is a real failure — fix the cause in the owning batch's territory, never by relaxing a gate.

  1. `git log --follow --name-status` over `docs/reference/plan-format.md` shows the move recorded with an `R` status.
  2. The six-pattern grep returns **zero** lines:

     ```
     grep -rniE 'plan-format-v3|plan_format_v3|plan-format v3|plan format v3|plan-v3' . \
       --exclude-dir=.git --exclude-dir=_mill --exclude-dir=.scratch \
       --exclude=shed-followups.md \
       | grep -v '^\./manifest/roadmap\.md:18:'
     ```

  3. Zero plan-format-v2 references remain. Run `grep -rniE '\bv2\b' --include='*.md' --include='*.go' . --exclude-dir=.git --exclude-dir=_mill --exclude-dir=.scratch` and review every hit against the deliberately-untouched list: `internal/state/state_test.go`, `internal/gitrepo/reset_test.go`, `internal/yamlengine/reconcile_test.go`, `internal/shuttleengine/claudeengine/command.go`, `internal/burlerengine/doc.go`, `manifest/designs/fabric-unified-view.md`, `manifest/designs/webster-parallel-execution.md`, and everything under `docs/research/`. None of those is a plan-format reference. Any hit outside that list is a miss.
  4. `grep -ni 'v3' docs/reference/plan-format.md` returns **nothing**.
  5. `grep -rni 'v3' internal/planparser/` returns only `gopkg.in/yaml.v3` import lines.
  6. `grep -rl 'gopkg.in/yaml.v3' --include='*.go' . | wc -l` returns **32**.
  7. `go build ./...` clean and `go test ./...` green (the batch `verify:` covers both).
  8. `git status --porcelain` lists nothing under `.scratch/` — no sweeper file staged or committed.
  9. Every relative markdown link and anchor **touched by the sweep** resolves. `docs/reference/plan-format.md` now exists, closing the links that dangled from `manifest/designs/loom.md` and from the deleted blockquote. This gate explicitly exempts `manifest/designs/shed-followups.md`: its four citations of the old path are excluded by design and are not a regression.

  One thing no gate can check: whether the six hand-written prose edits and the three override notes read correctly.
  That is review's job.
  Flag it at handoff, together with one open item from the discussion — the operator answered "out of scope" to bare-`v3` prose residue early on, and that answer was reversed twice during discussion review without being re-confirmed. The reversal is what put the four `internal/planparser` labels and the roadmap sentence in scope.
- **Commit:** none

## Batch Tests

`verify:` runs `go build ./...` plus the **unbounded** `go test ./...`.
This is the one place the plan justifies leaving per-batch scoping: this is the terminal batch, the task's own acceptance criterion is a repo-wide property rather than a per-package one, and the sweep touched six packages across three batches whose interactions no single scoped run covers.
The full suite is cheap in this repo — the scoped run of the six sweep-affected packages completes in about a second, so the whole suite is not a multi-minute cost here.

Card 14's grep gates are the substantive verification;
the suite is the behaviour-preservation backstop behind them.
`pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) runs once more from the git root before mill-go marks the task done, adding the integration-tagged packages this batch's `verify:` does not build.

No new test (`no-new-tests`).
