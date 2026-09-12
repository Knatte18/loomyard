# Batch: design-doc-and-roadmap

```yaml
task: "lyx loom step + external supervisor skill"
batch: "design-doc-and-roadmap"
number: 7
cards: 1
verify: go test ./internal/lyxcwd/...
depends-on: [6]
```

## Batch Scope

This batch closes the task's own design doc and removes its roadmap entry.
It is one batch and one card because both edits are completion markers for the whole task rather than for any single code change, and neither is true until every prior batch has landed.
It delivers no code.

Batch-local decision: this is the only batch where a doc edit is deliberately deferred rather than landing with the change that made it true. `CLAUDE.md`'s task-completion rule binds the module doc, `docs/overview.md`, and `CONSTRAINTS.md` to the same commit as the change — those all moved in batches 4, 5, and 6. `manifest/roadmap.md` is the documented exception: it moves only on completing a planned item, which is now. `manifest/designs/loom-step.md`'s status line and open questions are in the same position: the questions are answered by the shipped code, not by any one commit of it.

## Cards

### Card 19: flip loom-step.md's status and remove the roadmap's Planned item

- **Context:**
  - `internal/loomcli/step.go`
  - `internal/loomcli/status.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/shedengine/run.go`
  - `plugins/ly/skills/ly-supervise/SKILL.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/self-report-tier2.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/designs/loom-step.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/loom-step.md`, change the status blockquote from "Status: Planned, design settled with the operator 2026-09-12" to a shipped status naming what landed. Keep the two existing sentences recording that it supersedes `designs/llm-driven-loom-alternative.md` and is independent of the two self-report tasks — both remain true.

  Replace the `## Open questions` section entirely. The doc lists two, and both are now settled, so the section becomes a settled-contract section rather than staying a question list:

  The first question — `lyx loom step`'s exact return contract — is answered by the ten-key envelope. Record it in full: `producer`, `outcome`, `output`, `next`, `state`, `reason`, `continue`, `history_length`, `next_interrupt_policy`, and `status_file`, with the one-line meaning of each; these are envelope key names, signature inlined, no file read needed. Record that the first six derive from `shedengine.StepResult` and the last four are computed by `internal/loomcli`, that `continue` is derived as `state == "running"` so the skill never carries a copy of the state vocabulary, and that a hard producer error is an error envelope with a non-zero exit rather than an `ok` envelope carrying a failed state. Record the five-value `kind` vocabulary error envelopes carry and that the skill's one-retry rule applies to the producer kind alone.

  The second question — whether the supervisor skill may advance past a stuck or blocked gate — is answered in the direction the doc already leaned: never. Record the operator-hand-back rule: on any non-running state and on any error envelope the skill stops and hands back, never clearing state, editing the status file, re-seeding, or pushing. Record that this matches crucible's own "the push/merge decision is the operator's" rule.

  Update the `## What needs to happen` section so it reads as done rather than as pending work, and point step 2's skill reference at the shipped `plugins/ly/skills/ly-supervise/SKILL.md` and the `/ly:ly-supervise` invocation.

  Check `manifest/designs/self-report-tier2.md`'s cross-reference to this skill — it names the skill as the supervised-run substitute for its own machinery. If it refers to the skill as unbuilt or unnamed, leave the file alone anyway: it is not in this card's `Edits:`, and changing it is out of this task's scope. Record any stale wording found as a note in the commit message so a later task can pick it up.

  In `manifest/roadmap.md`, remove the `lyx loom step + external supervisor skill` Planned item in full, both its numbered paragraph and its `See [designs/loom-step.md](designs/loom-step.md).` continuation line. Do not add a Done entry — the Done section is deliberately kept empty, cleared 2026-08-25. The two self-report items each state they are independent of this item; leave both, and leave their "Independent of the `lyx loom step` item above" wording intact, since it still describes a real relationship to a shipped thing.

  Write both edits in semantic line breaks per `CLAUDE.md`: one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, using plain newlines and never trailing double-spaces or a backslash. Every inline link must resolve, file part and `#anchor` alike — `internal/lyxcwd`'s `docslink_test.go` enforces it and is this batch's verify scope. Removing the roadmap item removes a link to `designs/loom-step.md`, which is safe; the file itself stays, and `manifest/designs/loom.md` still links to it.
- **Commit:** `docs(loom-step): record the settled step contract and clear the roadmap item`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `docslink_test.go`, the Markdown Link Integrity guard over `manifest/` and `docs/`. That is the only mechanical check a docs-only batch is subject to, and it is the one that matters here: this card both removes a link (the roadmap's pointer to the design doc) and may add links inside the rewritten settled-contract section.

No Go production code changes, so no other package needs running. The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) runs before the task is marked done and is what confirms nothing in the full tree regressed across all seven batches.
