# Batch: docs

```yaml
task: "loom's status file can conflict on the landing merge"
batch: "docs"
number: 3
cards: 5
verify: go test ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch edits every durable doc that makes a claim batch 1 falsifies: the status-file contract, loom's design doc, three sibling design docs, the module table in the overview, and the sandbox suite's loom scenario.
It also adds the one-time operator migration note, which the discussion settled as belonging in loom's design doc — that placement is decided and is not reopened here.

All five docs are kept, durable references, so every edit is in place; nothing is deleted.
The batch depends on batch 1 because it describes the post-move state, and no batch consumes anything from it.

## Cards

### Card 12: Update the status-file contract

- **Context:**
  - `internal/loomengine/config.go`
  - `manifest/designs/shed.md`
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update the four occurrences of the old path — the status banner's "This doc pins the ... schema", the "What it is" opening sentence, the "Format decision (defended)" paragraph's "machine-written, machine-read orchestration state" sentence, and "The seed / handover"'s definition of the seed — to name `.lyx/loom/status.json`.
  Rewrite the "What it is" durability sentence, currently "It is durable **fabric-overlay state**: it lives under `_lyx/` (git-synced via fabric, not `.lyx/`'s ephemeral machine-local state), which is what makes resume work across machines."
  Replace it with the opposite, stated plainly: the file is machine-local, never-tracked state living under the ephemeral tree beside its own lock, and loom's orchestration state does not travel between machines.
  Do not describe a replacement carrier, a future one, or a migration path for the dropped property — it is dropped, not relocated.
  In "The seed / handover", the sentence "That binding is now pinned: `lyx loom run` seeds the file itself when it is absent, tolerating a re-run's already-seeded case rather than re-seeding it, and commits the seed weft-side before it spawns the detached driver" keeps its seed half and loses its commit half — `lyx loom run` no longer commits the status file at all.
  Leave the second banner paragraph's product-scoping rationale, the schema block, the `internal/state` format decision, and the `Shed`-owns-the-shell prose unchanged.
  The sentence naming `internal/loomengine.LoomStatusFile` as the resolver stays correct as written and needs no edit.
- **Commit:** `docs(specs): re-point the loom status contract at the ephemeral tree`

### Card 13: Rewrite loom's design doc and add the operator migration note

- **Context:**
  - `internal/loomcli/run.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/fabriccli/weft_verbs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update every mention of the status file's location in this doc, in six places.
  First, the `Loom-Preflight` paragraph under the producer table, which says `CheckSeed` "validates that `_lyx/loom/status.json` exists and is a coherent fresh seed".
  Second, the `## loom — the autonomous driver` opening, "It reads loom's **status file** in `_lyx/`".
  Third, the `### State & contracts` first bullet, which names the path inline, and its later sub-bullet arguing the product scoping "not bare `_lyx/status.json`" — retarget both to the ephemeral tree while keeping the product-collision argument intact, since it is about the `loom/` subdirectory and not about which tree it sits in.
  Fourth, the `### Crash recovery` opening, "cold-starts from the `_lyx/` status file".
  Fifth, the closing paragraph after the bootstrap listing, "loom writes the `_lyx/` status file".
  Do not change the text of the `### Crash recovery — resume on output files, not live processes` heading itself: it is linked by anchor from `manifest/roadmap.md` and from elsewhere in this doc, and the Markdown Link Integrity invariant fails on a broken anchor.
  Rewrite the resume-across-machines paragraph, currently opening "The status file lives in the weft repo, but it is not continuously committed there, so **resume across machines does not work today**".
  State instead that loom's status is machine-local by construction: it is never tracked, so a second machine sees nothing of a run's state, and that is the intended contract rather than a gap awaiting a carrier.
  Keep the closing observation that making orchestration genuinely cross-machine would mean committing on every producer transition — a `Shed` persistence-policy decision with a real per-transition git cost — and state that no such carrier is designed or scaffolded by this change.
  Delete the whole `**Publish` and `Finalize` commit the status file before they merge...**` paragraph and its four following sentences, which describe the removed `CommitStatus` seam, the removed checkpoint, and the closure `internal/loomcli` used to fill.
  In its place, state why no checkpoint is needed: `fabricengine`'s merge guard refuses tracked modifications only, so a never-tracked status file cannot make either landing producer refuse on the run's own bookkeeping.
  In the `lyx loom run` bootstrap listing, step `0c` reads "commit that seed weft-side, before anything below" with a parenthetical about the first precondition row seeing an uncommitted status file.
  Rewrite it so it names the origin record as the committed path and keeps the before-the-driver-spawns ordering constraint, which still holds for that record.
  Leave step `0b` (the seed itself) as it stands.
  Add a short operator-migration note alongside the rewritten status-file section, stating the one-time step for a hub that already carries the tracked file, and carrying the sanctioned mechanism verbatim rather than leaving it to the reader.
  The note must say: finish or abandon any in-flight loom run before upgrading, because an in-flight run cannot be migrated — after the move `lyx loom run` finds no file at the new path and seeds a fresh one, discarding the old run's `history`, which is budget-bearing since per-producer bounce budgets are derived by counting `stuck` entries.
  Then, from each affected worktree including the parent's, delete the file through that worktree's own `_lyx` junction with an ordinary `rm _lyx/loom/status.json`, and run `lyx fabric commit`, whose staging pathspec already covers the durable structural directory code-injected rather than through fabric config.
  State that a raw `git rm` inside the sibling overlay worktree is not the sanctioned spelling — the Fabric Git Invariant's exemption for ordinary human git covers the ordinary repo worktree only — and note the invariant's own carve-out that makes this legitimate: its ban binds LYX's own code at a loop-owner boundary, not a human running the shipped CLI by hand, which is why this is an operator step rather than something loom does for itself.
  State that the leftover file is inert rather than a live bug once no code writes that path, so the step is about removing dead tracked junk from the parent, not about a conflict that would otherwise recur.
  Change no other section, and add no roadmap entry.
- **Commit:** `docs(loom): drop the status-file checkpoint and add the migration note`

### Card 14: Update the three sibling design docs

- **Context:**
  - `internal/loomengine/config.go`
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/designs/shed.md`
  - `manifest/designs/self-report.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/shed.md`, the "**The status file**" heading's parenthetical calls loom's own file "one instance of it, not a `loom`-specific shape" and names the old path; update the path only, leaving the generic-contract framing and the JSON example below it unchanged.
  In `manifest/designs/self-report.md`, the Tier-1 section's opening sentence names the old path inline with a link to loom's design doc; update the path and leave the link and the surrounding argument untouched.
  In `manifest/designs/fabric-unified-view.md`, the "Shipped correction (slice 7, updated slice 9)" paragraph's as-built anchoring table lists `LoomStatusFile` inside "the durable, weft-synced, git-tracked `_lyx` group" alongside `PlanDir`, `DiscussionDir`, `WebsterDir`, and `PatternDir`.
  Move `LoomStatusFile` out of that group and into the ephemeral, machine-bound, never-git-tracked group named in the following clause, alongside `logger.LogsDir`, `ScoutDaemonStateFile`, and `ScoutDaemonLock`.
  Both groups already join onto `Location.AnchorPath()`, so the anchoring claim itself is unchanged — only the group membership moves.
  Edit this doc in place despite its own status banner scheduling it for deletion once slice 6's open half lands; a scheduled deletion is not a reason to leave a false claim standing until then.
- **Commit:** `docs(designs): move LoomStatusFile into the ephemeral group`

### Card 15: Correct the module table's bootstrap description

- **Context:**
  - `internal/loomcli/run.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the `loom` module entry, the `run` sentence describes the session bootstrap's four steps and names the first as "resolve the recorded parent branch and seed+commit the status file weft-side when it is absent".
  The seed half stays; the commit half is false after batch 1, which reduced that commit's pathspec to the provenance record alone.
  Rewrite the clause so it names seeding the status file when it is absent and committing the recorded parent-branch provenance, keeping the sentence's four-steps-in-order shape and the other three steps unchanged.
  Change nothing else in the entry — the `drive`, `status`, `pause`, `validate-discussion`, and `validate-plan` sentences and the interactive-handoff exceptions paragraph are all still accurate.
  The module table itself gains and loses no row, so no other part of this file moves.
- **Commit:** `docs(overview): correct the loom bootstrap's commit step`

### Card 16: Fix the sandbox suite's S8 fixture path

- **Context:**
  - `contracts/specs/loom-status-spec.md`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In scenario S8, "Loom status and pause over a seeded fixture", the **Fixture note** tells the operator to hand-write the status file at the old durable path; retarget it to `.lyx/loom/status.json`.
  Keep the note's stated reason for hand-writing the fixture rather than reaching a seeded state through a shipped verb, and keep the second sentence pointing at the status contract's worked example for the fixture's shape.
  Leave the **Goal**, **Covers**, **Watch**, and **Verdict** lines exactly as they are: the `**Covers:** loom` tag is what `cmd/lyx/sandbox_coverage_test.go` checks for the Sandbox Suite Coverage invariant, and this scenario is `loom`'s only registered coverage.
  That test checks tagging rather than fixture correctness, so a stale path here fails the operator running the suite by hand, not the suite — which is why the correction is worth making even though nothing red points at it.
- **Commit:** `docs(sandbox): retarget S8's loom status fixture path`

## Batch Tests

`verify:` runs `./internal/lyxcwd/...` for `docslink_test.go`, the machine half of the Markdown Link Integrity invariant — the check that card 13's heavy edit to loom's design doc left every cross-doc link and every intra-doc anchor resolvable, including the `#crash-recovery--resume-on-output-files-not-live-processes` anchor that `manifest/roadmap.md` links to and that card 13 is explicitly forbidden from renaming.
It also runs `./cmd/lyx/...` for `sandbox_coverage_test.go`, which asserts every registered module is either exercised by a tagged sandbox scenario or explicitly excluded — the guard that card 16's edit left S8's `**Covers:** loom` tag intact.

No new test is added.
This is a documentation batch: every card edits prose whose only machine-checkable properties are link integrity and the sandbox coverage tag, and both already have guards.
The substantive correctness of these edits — that each doc now describes the shipped behavior — is what the plan reviewer and the code reviewer check against batch 1's diff.
