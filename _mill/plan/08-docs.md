# Batch: docs

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'docs'
number: 8
cards: 2
verify: go test ./internal/lyxcwd/...
depends-on: [7]
```

## Batch Scope

This batch closes the documentation half of the task: the two new packages enter `docs/overview.md`'s module tree, the roadmap item moves from Planned to Done, and `manifest/designs/self-report-tier2.md`'s three Open questions are closed in place.

It depends on batch 7 so the docs describe a feature that is actually wired, and it edits no Go production file.

`CONSTRAINTS.md`'s new Friction Leaf Invariant is deliberately **not** here — it landed in batch 1, beside the package it constrains and the `leaf_enforcement_test.go` that pins it.

Batch-local decision: the design doc is updated rather than deleted, against `docs/overview.md#documentation-lifecycle`'s default, because two sibling design docs link to it and the Markdown Link Integrity invariant requires those links to resolve.
The full rationale is in the overview's Shared Decisions.

## Cards

### Card 27: `docs/overview.md` module tree and shared-infrastructure paragraph

- **Context:**
  - `internal/friction/doc.go`
  - `internal/frictionengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two rows to the module tree in `docs/overview.md`'s `## Modules` section, in the style and column alignment the surrounding rows already use:

  - `internal/friction/` beside the existing `internal/pattern/` row at `docs/overview.md:261`, described as the Tier 2 friction-note directive leaf, consumed by webster, burler, and loom.
  - `internal/frictionengine/` beside the existing `internal/mergeresolve/` row at `docs/overview.md:243`, described as the aggregation-and-reflection step `internal/loomcli`'s drive verb calls once per run.

  The tree's last row uses a `└──` connector rather than `├──`;
  keep exactly one such row after the insertions.

  Extend the shared-infrastructure paragraph at `docs/overview.md:362`, which today lists `internal/pattern` among the leaves and then gives it its own explanatory sentence, to name `internal/friction` in the same list and give it the equivalent one-sentence description: the leaf that returns the role-appropriate friction-note directive injected into all seven agent prompts when Tier 2 is enabled, with the note path composed by its own non-clobbering `NotePath`.

  Do not add a `## Modules` bullet for either package: neither registers a `lyx` CLI module, so neither belongs in the user-facing-module list, and neither adds a Sandbox Suite Coverage obligation.
  Confirm that against the sandbox suite's own registry rather than assuming it.

  Every inline markdown link added or touched must resolve, file part and `#anchor` alike, per the Markdown Link Integrity invariant.
- **Commit:** `docs(overview): add internal/friction and internal/frictionengine to the module map`

### Card 28: roadmap Done entry and the design doc's closed Open questions

- **Context:**
  - `manifest/designs/self-report-tier1.md`
  - `manifest/designs/loom-step.md`
  - `docs/overview.md`
  - `internal/friction/doc.go`
  - `internal/frictionengine/doc.go`
  - `internal/loomengine/template.yaml`
- **Edits:**
  - `manifest/roadmap.md`
  - `manifest/designs/self-report-tier2.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`manifest/roadmap.md`.**
  Move the `self-report Tier 2: per-agent friction notes for unsupervised runs` item from `## Planned` to `## Done`, keeping its literal `1.` prefix — the file's own Maintenance section states that numbering is automatic and restarts per section, so no renumbering is needed anywhere.
  Rewrite the entry to past tense in the Done section's own style: a name plus one or two sentences of what shipped, never a design writeup.
  Per the Maintenance section, a Done entry points at the module's own package documentation, so point it at `internal/friction` and `internal/frictionengine`'s package docs.
  Keep the `See [designs/self-report-tier2.md](designs/self-report-tier2.md)` line, since the doc is retained rather than deleted.

  Update the two sibling Planned entries that describe this item as pending: the `lyx loom step` entry at `manifest/roadmap.md:12` and the `self-report Tier 1` entry at `:15` each say this item is independent and buildable in parallel, which is now true in the past tense for one of the three.
  Make the minimum edit that keeps both entries accurate;
  do not rewrite them.

  **`manifest/designs/self-report-tier2.md`.**
  Change the `> **Status: Planned, design settled with the operator 2026-09-12.**` line to record that it shipped, and replace the `## Open questions` section's three bullets with the answers the implementation settled:

  - Where notes physically live: `.lyx/loom/friction/`, via `loomengine.LoomFrictionDir` built on `LoomScratchDir`, because the Durable-vs-Ephemeral State Invariant puts never-tracked files under `.lyx` and `_lyx` would drag in the Fabric Git Invariant's commit-seam machinery for a file deleted minutes later.
  - Default-on versus opt-in per producer or profile: default-on, with one global `loom.yaml` key (`friction`) that is both the model spec and the kill switch, because the feature's value is breadth of coverage and per-row opt-in would mean a Tier 2 key on five different recipe engines whose row names are durable on-disk identities.
  - Cross-phase semantic friction: still explicitly deferred, and now recorded as a stated limitation of the shipped design rather than an open question — a Tier 2 note can only ever describe friction inside its own narrow task.

  Rename the section heading from `## Open questions` to something the shipped state reads honestly, and keep the `## Related` section's three links intact — `manifest/designs/self-report-tier1.md:31` and `manifest/designs/loom-step.md:32` both link back to this file, so it must stay on disk and its own links must keep resolving.

  Every inline markdown link under `manifest/` and `docs/` must resolve, file part and `#anchor` alike.
- **Commit:** `docs(manifest): move self-report Tier 2 to Done and close its open questions`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `internal/lyxcwd/docslink_test.go`, the Markdown Link Integrity check over `manifest/` and `docs/`.
That is the only runnable surface either card in this batch has: both cards edit markdown only, and link resolution — including the `#documentation-lifecycle` anchor and the two inbound links into `manifest/designs/self-report-tier2.md` that card 28 is careful to keep valid — is exactly what a docs batch can regress.

The scope is deliberately this one package rather than a wider sweep: no Go file changes in this batch, so nothing else can regress from it.
Whole-repo confirmation at task end remains `pipeline.done_gate`'s job (`go test ./... && go test -tags integration ./...`).
