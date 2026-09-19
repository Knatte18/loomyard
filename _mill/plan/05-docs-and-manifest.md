# Batch: docs-and-manifest

```yaml
task: "Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise"
batch: "docs-and-manifest"
number: 5
cards: 6
verify: go test ./internal/lyxcwd/ ./contracts/... ./tools/...
depends-on: [4]
```

## Per-file sweep rule

Identical to batch 1's rule of the same name, and it governs this batch too: a file in a card's `Edits:` is swept **whole**, and the sites named in `Requirements:` are landmarks pinning the hard judgment calls and the sentences that must be rewritten rather than substituted — never the boundary.
Sweep for verb names (`lyx loom run`/`lyx loom drive`/`lyx run` and bare backticked `` `run` ``/`` `drive` `` naming a verb), pre-move filename citations (`run.go` -> `start.go`, `drive.go` -> `run.go`), and `ly-supervise` -> `ly-drive`, including in link text and document titles.

Leave the noun sense alone — "run" meaning "an execution", and "drives"/"driver" as ordinary English — and leave the other modules' own verbs (`lyx webster run`, `lyx burler run`, `lyx shuttle run`) untouched.

## Batch Scope

This batch is the whole prose surface: the top-level operator docs, the invariant text, every design doc naming a renamed verb or the skill, the roadmap's Planned-to-Done move, the loom status spec, and the sandbox suite doc.
It is one batch because the Markdown Link Integrity invariant couples its pieces — deleting `manifest/designs/loom-cli-rename.md` requires both inbound links fixed in the same verify window — and because the whole set is a single classification pass over one kind of content.

Card order inside the batch is load-bearing for the link invariant: card 16 fixes the `shed-generic-watchdog.md` inbound link and card 17 fixes the `roadmap.md` one, so that card 19's deletion lands only after nothing points at the doc any more.

Batch-local decision: retrospective prose is rewritten outright, per the shared `historical-prose-rewritten-not-glossed` decision, including the literal path `plugins/ly/skills/ly-supervise/SKILL.md` in `manifest/designs/loom-step.md`, which becomes the path batch 4 created.

## Cards

### Card 14: top-level operator docs and the CLI/Cobra invariant

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
  - `internal/loomcli/cli.go`
- **Edits:**
  - `README.md`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `README.md`, rewrite the four sentences naming a renamed verb: the spine sentence saying `lyx run` in a worktree bootstraps a task, the `loom` module bullet naming `lyx loom run`, the convenience-alias line `**`lyx run` → `lyx loom run`**`, and the getting-started sentence ending "and `lyx run` inside it".
  The alias line becomes the `lyx start` to `lyx loom start` mapping;
  the other three name `lyx start` or `lyx loom start` according to which form the surrounding sentence uses.
  In `docs/overview.md`, rewrite the same alias sentence, the `loom` module bullet, the selfreport bullet naming `lyx loom drive` as the call Tier 1 fires after, the loom module-table entry listing `lyx loom run|drive|step|status|pause|validate-discussion|validate-plan` plus the bare root alias, the two sentences describing what `run` and `drive` each do, the `step` sentence saying it bootstraps exactly as `run` does, the interactive-handoff sentence naming `lyx loom status --watch` and `lyx loom run` (alias `lyx run`), and the bootstrap bullet naming `lyx loom run` (alias `lyx run`).
  The verb list becomes `lyx loom start|run|step|status|pause|validate-discussion|validate-plan` with the alias named as `lyx start`, and the two role sentences must be rewritten as sentences so `start` is described as the four-step session bootstrap and `run` as the no-tmux foreground escape hatch — a token swap would make each describe the other.
  Leave the `webster` bullets' own `run` and `recover-batch` prose untouched: those name `lyx webster run`, a different module's verb this task does not audit.
  In `CONSTRAINTS.md`, change the CLI/Cobra Invariant's interactive-handoff exception list from `` `lyx loom status --watch`, `lyx loom run`/`lyx run` `` to name `` `lyx loom start`/`lyx start` ``, leaving the `reedengine` entries in that same list unchanged.
  Use semantic line breaks in every rewritten paragraph — one sentence per line, no fixed-column hard-wrap.
- **Commit:** `docs: rename the loom verbs in README, overview, and CONSTRAINTS`

### Card 15: the loom and shed design docs

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
  - `docs/overview.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/loom.md` carries roughly twenty verb references and is the densest file in the batch;
  read each one and classify it before editing.
  The control-flow, orchestrator-module, phase-machine, and module-table references to `lyx loom run` name the bootstrap and become `lyx loom start`, as do the bare-alias references `lyx run` in the resume, human-boundaries, separation-of-state, and crash-recovery sections, and the `lyx run --auto` form, which becomes `lyx start --auto`.
  The convenience-alias sentence becomes the `lyx start` to `lyx loom start` mapping.
  The sentence about a genuinely constructible `lyx loom drive` run and the one about `Shed.Run`'s envelope naming `lyx loom drive` both name the foreground verb and become `lyx loom run`.
  The `VerifySeedOwnership` bullet names both verbs in contrast — "the ownership check both `lyx loom run` and `lyx loom drive` run" — and must be rewritten as a sentence naming `lyx loom start` and `lyx loom run`, keeping the two sides distinct.
  The adjacent `loomshed.Seed` bullet, which says only `lyx loom run` calls it to seed the file, names the bootstrap and becomes `lyx loom start`.
  The `/ly-*` skills table row naming `ly-supervise` as the first shipped skill becomes `ly-drive`.
  The session-bootstrap section's `lyx loom run:` heading line and the launcher sentence about invoking the explicit two-word verb both become `lyx loom start`.
  In `manifest/designs/shed.md`, rewrite the six references: the `loom` bullet's `lyx loom run`, the pause sentence's "until the next `lyx run`", the state-persistence sentence's "right after typing `lyx loom run` to resume", the lock bullet's "two concurrent `lyx loom run` invocations", the terminal-condition paragraph's "a restarted `lyx run`", and the out-of-scope bullet's "the product's CLI entry point (`lyx loom run`)".
  All six name the bootstrap and become the `start` form, matching whichever of the two spellings the sentence already uses.
  Do not edit the `internal/treadleengine/run.go` mention or the `run.go:119–128` citation in the lock bullet — that is a different package's filename, not a loom verb.
  Use semantic line breaks in every rewritten paragraph.
- **Commit:** `docs(manifest): rename the loom verbs in the loom and shed design docs`

### Card 16: the remaining design docs and the watchdog's Related link

- **Context:**
  - `plugins/ly/skills/ly-drive/SKILL.md`
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/designs/loom-step.md`
  - `manifest/designs/self-report-tier1.md`
  - `manifest/designs/self-report-tier2.md`
  - `manifest/designs/reed-born-as-strand.md`
  - `manifest/designs/reed-mailbox.md`
  - `manifest/designs/reed-header-selvage.md`
  - `manifest/designs/logger-coverage.md`
  - `manifest/designs/shed-recipe.md`
  - `manifest/designs/shed-generic-watchdog.md`
  - `manifest/designs/worktree-lifecycle-shed-producers.md`
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/loom-step.md`, rewrite the Shipped-status header naming the `/ly:ly-supervise` skill, the sentence saying `step` reads state exactly as `lyx loom run`'s internal loop does, the Done entry naming the literal path `plugins/ly/skills/ly-supervise/SKILL.md`, and the Tier-1 exemption sentence naming both `lyx loom drive` and `lyx loom run`.
  The path becomes `plugins/ly/skills/ly-drive/SKILL.md`, matching what batch 4 created;
  the exemption sentence is contrast prose whose "(and therefore to `lyx loom run`, which spawns it)" clause turns on the two verbs being different, so it must be rewritten as a sentence naming `lyx loom run` and `lyx loom start` in the right positions rather than substituted.
  In `manifest/designs/self-report-tier1.md`, the opening sentence naming `lyx loom drive` as the detector becomes `lyx loom run`;
  the "Tier 1 fires on the `drive` path only" heading sentence and its `ly-supervise` reference become the `run` path and `ly-drive`;
  and the clarifying paragraph stating that `lyx loom run` spawns `drive` as its detached driver is the exemption whose whole meaning turns on the two verbs being distinct — rewrite it as a sentence so it reads that `lyx loom start` spawns `run` as its detached driver.
  In `manifest/designs/self-report-tier2.md`, rewrite the four references: the scoping sentence naming plain `lyx loom run`, the driver-handshake sentence, the mutual-exclusion sentence about a second `lyx loom run` seeing a free lock, and the bootstrap-lifecycle bullet naming `lyx loom run` and `lyx loom step` — all four name the bootstrap and become `lyx loom start`, while `lyx loom step` is unchanged.
  In `manifest/designs/reed-born-as-strand.md`, rewrite the document title, the `loom run` terminal-handoff sentence, the escape-hatch sentence mirroring `loom drive`'s relationship to `loom run`, and the `ly-supervise + orchestrator` bullet.
  The escape-hatch sentence is contrast prose and must end up naming `loom run`'s relationship to `loom start`;
  the title and the other two become the `loom start` and `ly-drive` forms.
  In `manifest/designs/reed-mailbox.md`, the two `ly-supervise` references — the addressable-Strand list and the dependency sentence — become `ly-drive`.
  In `manifest/designs/reed-header-selvage.md`, the Related bullet naming the `ly-supervise` skill and linking the born-as-strand item by its `loom run` title becomes `ly-drive` with the retitled link text.
  In `manifest/designs/logger-coverage.md`, the two references to the loomcli bootstrap source file's `loom drive` spawn describe a file batch 1 renamed to start.go and a verb that is now `loom run`;
  rewrite both accordingly, since this is a retrospective survey describing the tree as it stands.
  In `manifest/designs/shed-recipe.md`, the geometry sentence naming `lyx loom run` for hub mode becomes `lyx loom start`.
  In `manifest/designs/shed-generic-watchdog.md`, rewrite the title and the body sentence naming `ly-supervise` and loom's three verbs, and rewrite the Related bullet so it no longer links `loom-cli-rename.md` — card 19 deletes that file, and the Markdown Link Integrity invariant fails the build on a link to a deleted file.
  Replace that bullet with prose recording that the naming half shipped, carrying no markdown link to the deleted doc.
  Add a new `## The shape it would take` section to that same file, between `## The idea` and `## Why not Planned`, recording what this task settled about the generalisation — the doc currently states only that the skill and the three verbs "could, in principle, become a watchdog over any `Shed`", with no account of what that would look like.
  The section must state four things.
  First, that the three verbs generalise rather than collapse: the end state is a verb set on a generic `shed`, armed with an FSM recipe supplied from outside — from the task description or an equivalent carrier — rather than a single merged watchdog command.
  Second, that under that set the verb/engine symmetry this task established becomes literal, since the generic `run` calls `Shed.Run` and the generic `step` calls `Shed.Step`;
  record that landing the rename first is what makes this true, because the pre-rename names would have carried the asymmetry into the abstraction, where it is harder to unpick than in `loomcli` alone.
  Third, that `start` is the one verb with no engine counterpart — there is no `Shed.Start`, because it seeds, commits, spawns the detached driver and hands the terminal over, all of which sit above the engine — so whether it belongs on a generic `shed` at all, or stays loom-specific, is itself open.
  Fourth, that the skill's role in that end state is to loop the generic `step` until the recipe's own producer list is exhausted or the run reaches a non-`running` state, which is what it already does over `lyx loom step` today.
  Do not change the `## Why not Planned` section's verdict: a second `shedrecipe` consumer beyond loom is still what the generalisation needs before it can be validated, and this new section describes a shape, not a commitment.
  Use semantic line breaks throughout the new section.
  In `manifest/designs/worktree-lifecycle-shed-producers.md`, the status line naming `lyx loom run` as one of three manually-sequenced steps and the bootstrap bullet naming a `loom run` producer both name the bootstrap and become the `start` form.
  In `manifest/designs/reed-fabric-standalone-api.md`, the per-importer shape bullet describing `loomcli` as the sole consumer retaining a concrete `*reedengine.Engine` field cites the pre-move filenames and line ranges for the six `Up`/`Status`/`AddStrand`/`RemoveStrand`/`TmuxPath`/`AttachArgv` call sites (`internal/loomcli/run.go:145-320`, `drive.go:64`).
  After the rename those calls live in `internal/loomcli/sharedbootstrap.go` (`Up`, `Status`, `RemoveStrand`, `AddStrand`), `internal/loomcli/start.go` (`Status`, `TmuxPath`, `AttachArgv`), and `internal/loomcli/run.go` (`Up`);
  retarget the citation to `internal/loomcli/sharedbootstrap.go, start.go, run.go` naming the call sites without the now-false line-range claim, since the six calls are no longer contiguous in one file.
  Use semantic line breaks in every rewritten paragraph.
- **Commit:** `docs(manifest): rename the verbs and skill across the remaining design docs`

### Card 17: move the roadmap item from Planned to Done

- **Context:**
  - `manifest/designs/shed-generic-watchdog.md`
  - `manifest/designs/reed-born-as-strand.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the Planned entry `**loom CLI: rename `run`/`drive`/`step` for verb/engine symmetry, plus rename `ly-supervise`**` together with its `See [designs/loom-cli-rename.md](designs/loom-cli-rename.md).` continuation line, and add a Done entry for the same item at the top of the `## Done` section's numbered list.
  The Done entry's one-line what/why must record the decided names — `drive` became `run`, the old `run` became `start`, `step` unchanged, the root alias became `lyx start`, and `ly-supervise` became `ly-drive` — and must carry no `See [designs/...]` link, since card 19 deletes that design doc.
  Removing the Planned entry's `See` line is what fixes the first of the two inbound links the deletion requires;
  card 16 fixes the second.
  Also rewrite the three other entries in this file that name a renamed verb or the skill: the Next Up `reed: born-as-strand for the operator's `loom run` attach` item's title and body, whose title must match the retitled design doc card 16 produced;
  the Next Up `generalize `ly-supervise` and loom's `run`/`drive`/`step` CLI verbs into a Shed-generic watchdog` item's title, which must match the retitled `shed-generic-watchdog.md`;
  and the two Done entries naming `ly-supervise` — the `launch via lyx reed add` entry and the `lyx loom step` + external supervisor entry — both of which become `ly-drive`, rewritten outright rather than glossed because they describe the tree as it stands.
  Keep the file's `1.` numbering convention, per the Maintenance section, and use semantic line breaks.
- **Commit:** `docs(manifest): move the loom CLI rename item to Done`

### Card 18: the loom status spec and the sandbox core suite

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/pause.go`
  - `internal/loomcli/status.go`
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `contracts/specs/loom-status-spec.md`, rewrite the seven verb references.
  The opening sentence is contrast prose — it says `lyx loom drive` (via `Shed.Run`) rewrites the shell on every step, and that the loop belongs to the driver verb `lyx loom run` spawns detached rather than to `lyx loom run` itself — so it must be rewritten as a sentence naming `lyx loom run` as the rewriting verb and `lyx loom start` as the spawner, keeping both sides distinct.
  The remaining six references — the seed's t=0 sentence, the "It is written by **`lyx loom run`**, the session bootstrap" sentence, the pinned-binding sentence, the "but `lyx loom run` is always the writer" sentence, the single-writer bullet naming the seeder alongside `lyx loom pause`, and the realistic-seed sentence — all name the bootstrap and become `lyx loom start`.
  Editing this file is safe and expected: an operator whose hub already seeded the old spec keeps their copy until they delete the stale file and let `stencilstore` re-seed, which is `stencilstore`'s existing deliberate behaviour for any spec edit.
  Add no force-sync path — the Stencil Ownership Invariant bans that carve-out.
  In `tools/sandbox/SANDBOX-CORE-SUITE.md`, update the two scenarios that assert on refusal text: the fixture note explaining that no shipped verb seeds without going through `lyx loom run`'s tmux bootstrap handover, and the question asserting each verb refuses by naming its own remedy `"no status file ... run `lyx loom run`"`.
  Both become `lyx loom start`, matching the refusal strings cards 2 and 3 now emit.
  Add no `**Covers:**` tag change — no tag names a renamed verb, so the Sandbox Suite Coverage invariant needs nothing here beyond card 5's allowlist key.
  Use semantic line breaks in every rewritten paragraph.
- **Commit:** `docs(contracts): name lyx loom start in the status spec and sandbox suite`

### Card 19: delete the design doc

- **Context:**
  - `manifest/roadmap.md`
  - `manifest/designs/shed-generic-watchdog.md`
  - `docs/overview.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `manifest/designs/loom-cli-rename.md`
- **Moves:** none
- **Requirements:** Delete `manifest/designs/loom-cli-rename.md` with `git rm`.
  The Documentation Lifecycle deletes a module-design doc when its work lands, and this doc's entire content — the naming question, recorded as not yet decided — is obsolete once this task ships.
  Both inbound links are already gone by this point: card 17 removed the roadmap's `See [designs/loom-cli-rename.md](designs/loom-cli-rename.md)` line with the Planned entry, and card 16 rewrote `shed-generic-watchdog.md`'s Related bullet.
  Before deleting, confirm no other markdown link anywhere under `manifest/` or `docs/` resolves to this file;
  the Markdown Link Integrity invariant's test fails the build on a dangling link, and this batch's `verify:` runs it.
  Do not edit the file's contents before deleting it.
- **Commit:** `docs(manifest): delete the loom-cli-rename design doc on landing`

## Batch Tests

`verify:` runs `go test ./internal/lyxcwd/ ./contracts/... ./tools/...`, which completes in roughly a second on this tree.

`./internal/lyxcwd/` is the batch's real gate: it hosts `docslink_test.go`, the Markdown Link Integrity invariant's test, which walks every inline markdown link under `manifest/` and `docs/` and fails on any unresolvable file part or `#anchor`.
That is what proves card 19's deletion is safe and that cards 16 and 17 fixed both inbound links, and it is also what catches an anchor broken by a retitled heading in `reed-born-as-strand.md` or `shed-generic-watchdog.md`.

`./contracts/...` covers `contracts/specs`' own registry tests, confirming card 18's spec edit left the file readable and the registry's stamp round-trip intact.
`./tools/...` covers `tools/sandbox` and `tools/mdreflow`, the latter being the semantic-line-break tooling this batch's prose must stay consistent with.

The package scope is per-batch as the default expects — no unbounded whole-suite run is used or needed, since the repo-wide surface is covered by `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) at task completion.
