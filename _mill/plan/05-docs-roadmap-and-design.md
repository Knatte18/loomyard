# Batch: docs-roadmap-and-design

```yaml
task: 'landing: parent-fabric resolution chain'
batch: docs-roadmap-and-design
number: 5
cards: 2
verify: go test ./internal/lyxcwd/... -run TestEnforcement_MarkdownLinks
depends-on: [1, 2, 3, 4]
```

## Batch Scope

This batch closes the Documentation Lifecycle obligation for the whole task: it moves the roadmap item to Done, correcting its two stale forward-references, and corrects the one stale forward-reference in `manifest/designs/loom.md`.
It depends on every code batch because both edits describe the task as complete — writing them before batches 1-4 land would make the docs false the moment they were committed.
No package doc changes remain outstanding here: `internal/fabricengine`'s and `internal/loomengine`'s doc comments were already updated in batch 1 (card 5) and batch 2 (card 9) respectively, alongside the code they document, per the Documentation Lifecycle's own same-commit rule.
`docs/overview.md` is not touched: this task adds no new module and changes no module's entry in that file's module table.
`CONSTRAINTS.md` is not touched: this task introduces no new cross-cutting invariant.

No card in this batch has a non-empty `Moves:`.

## Cards

### Card 20: Move the roadmap item to Done, correct its stale claims

- **Context:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three edits to `manifest/roadmap.md`, all in this one commit:

  1. Delete the `### landing: parent-fabric resolution chain` subsection under `## Planned` (roadmap.md:12-21) — its intro paragraph and its single numbered item — in its entirety.

  2. Add a new item under `## Done` (after `1. **preflight: split into two Shed rows...**`, the current last entry in that section, matching the section's own numbering convention where every entry is written literally as `1.`): `` 1. **landing: parent-fabric resolution chain** — ... `` summarizing what shipped: `fabricengine.OpenParent` as the four-step resolution chain (list, match, resolve, open) inside `internal/fabricengine`, a `Prunable` field on `WorktreeEntry` so a stale worktree entry is skipped rather than matched, the two vocabulary-neutral `Fabric.OriginURL`/`Fabric.PushBranch` methods, `loomengine.LoomScratchDir`, and `internal/loomcli/drive.go` filling `shedrecipe.Env.Landing` in full immediately before `loomrecipe.New` — closing the gap that made `lyx loom drive` fail construction on every invocation.
     Correct the stale claim the deleted Planned item's own text carried (`` "no worktree-listing helper exists yet in internal/gitrepo/internal/fabricengine, and one must be added to do the matching" ``) by stating plainly in the new Done entry that the worktree-listing helper (`fabricengine.List`) already existed before this task — what this task added was the matcher, the resolver, and the opener on top of it, per this file's own `## Maintenance` guidance that a Done entry should be short (a name plus one or two sentences), pointing at the `internal/fabricengine` package documentation for the rest.
     Link the entry at `[internal/fabricengine](../internal/fabricengine/doc.go)` and, if a second pointer is useful, `[designs/loom.md](designs/loom.md)`, following this file's own existing citation style (see the `internal/loomrecipe` link on the `loom: convert to a Shed recipe` entry for the pattern).

  3. In the existing `` 1. **loom: convert to a Shed recipe** `` Done entry (roadmap.md:179-184), locate the sentence `` "`Env.Landing` is deliberately left unfilled by `internal/loomcli`, preserving the pre-existing gap the new `landing: parent-fabric resolution chain` Planned item above closes." `` — this is now false, since that item is Done, not Planned, and the gap it names is closed.
     Reword it to state, in the past tense, that `Env.Landing` was left unfilled at the time this entry shipped, and that the `landing: parent-fabric resolution chain` Done entry above closed the gap — keeping both entries internally consistent about which one references which, without deleting the historical fact that `Env.Landing` genuinely was unfilled when this earlier item shipped.

  Apply the semantic-line-break markdown convention (CLAUDE.md's "Markdown: semantic line breaks" rule) to every line this card adds or rewrites: one sentence per line, breaking a long sentence at an internal comma-plus-coordinating-conjunction boundary where one exists.
- **Commit:** `roadmap: move landing: parent-fabric resolution chain to Done`

### Card 21: Correct the stale forward-reference in `designs/loom.md`

- **Context:** none
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Locate the sentence in `designs/loom.md` (in the "`## The phase machine — a flat producer list, no predefined slots`" section): `` "`loom: phase-machine scaffolding` stubs both and swaps in the real, shared-by-reference producers once `landing: Publish + Finalize producers` lands, on its own schedule (see [internal/landingshed](../../internal/landingshed/doc.go))." `` — `landing: Publish + Finalize producers` has itself already shipped (it is in `## Done` in `manifest/roadmap.md`), but the sentence's forward-looking framing ("once ... lands") is stale in the same shape the roadmap's own stale forward-references were: the producers exist, and as of this task, `Env.Landing`'s construction chain is complete too, so `Publish`/`Finalize` are now genuinely constructible in a real `lyx loom drive` run, not merely implemented.
  Reword the sentence to state, in the present/past tense rather than "once ... lands," that `loom: phase-machine scaffolding` swapped in the real `Publish`/`Finalize` producers (linking `internal/landingshed` as before), and that the `landing: parent-fabric resolution chain` item completed their construction chain by filling `Env.Landing`, citing that Done entry by name — the same observable-behavior-change framing this card's own plan calls for.
  Apply the semantic-line-break markdown convention to the rewritten sentence, matching the rest of this file's own paragraph style.
- **Commit:** `design: correct stale landing-construction forward-reference in loom.md`

## Batch Tests

`verify: go test ./internal/lyxcwd/... -run TestEnforcement_MarkdownLinks` runs the Markdown Link Integrity guard (`internal/lyxcwd/docslink_test.go`) scoped to just that test, over both files this batch edits — both live under `manifest/`, one of the two scanned roots, and each keeps or adds `[text](target)` links (to `internal/fabricengine/doc.go`, `designs/loom.md`, and `internal/landingshed/doc.go`) that must resolve.
No Go source changes in this batch, so no package test run is needed.
