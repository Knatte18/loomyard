# Batch: docs-and-full-verification

```yaml
task: 'reed: extract Selvage-pane lifecycle'
batch: docs-and-full-verification
number: 2
cards: 5
verify: go test ./internal/reedengine/
depends-on: [1]
```

## Batch Scope

This batch delivers the documentation half of the task and the one verification tier no automatic gate covers.
It repoints `internal/reedengine/doc.go`'s Selvage section at the new owning file, records the post-extraction audit in the design doc alongside the correction of that doc's stale `pinGeometryOptionsLocked` claim, fixes a one-line stale file attribution in the shipped `reed-header-selvage.md`, moves the roadmap item to Done, and runs the full build plus the `integration` and `smoke` tiers.

It depends on batch 1 because every doc statement it writes describes batch 1's finished layout, and because the audit counts can only be taken once the extraction has landed.
It is a separate batch because none of it changes compiled behavior and all of it needs the extraction's final shape to be accurate.

No batch-local decision differs from the overview's Shared Decisions.

## Cards

### Card 10: repoint doc.go's Selvage section at the new owner

- **Context:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/selvagepane_enforcement_test.go`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update two passages in the package doc comment, changing what they attribute and leaving everything else as it stands.

  The second package-level-invariant paragraph currently cites the three exclusion seams by their old homes — `ensureSelvagePaneLocked` in `lifecycle.go`, `planPaneTarget` in `spawn.go`, and `planReconcile`'s `exemptPaneIDs` in `reconcile.go`.
  Repoint that citation at `selvagepane.go` and name the seams as they now stand: the create/heal path, `planPaneTarget`, and the `reapPolicy` value `planReconcile` is handed.

  The "Three module-local Selvage rules" block currently states those rules are kept in the package rather than in `CONSTRAINTS.md` because they describe this package's own design.
  That decision is unchanged and the three rules are unchanged.
  Add that `selvagepane.go` is now the single file those rules are implemented in, and that `selvagepane_enforcement_test.go` is the mechanical check keeping them there.

  Add no new Selvage detail beyond those two attributions, and leave the live-geometry rule and every later section untouched.
  This file is comments only, so it carries no AST identifier the enforcement test can see.
- **Commit:** `docs(reedengine): name selvagepane.go as the owner in the package doc's Selvage section`

### Card 11: design-doc Status section and the stale-complication correction

- **Context:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/windowsize.go`
  - `manifest/designs/reed-header-selvage.md`
- **Edits:**
  - `manifest/designs/reed-selvage-pane-extraction.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `## Status: Implemented` section and correct the stale section already in the file.

  The Status section records the post-extraction audit.
  Re-run the count at implementation time rather than copying figures from anywhere: match lines case-sensitively for the string `Selvage` across `internal/reedengine`'s non-test `.go` files, and state that method alongside the numbers, because a case-insensitive count is a different measurement and reports different figures.
  State the before figures this worktree's HEAD actually carried — `lifecycle.go` 58, `doc.go` 34, `reconcile.go` 20, `spawn.go` 17, `apply.go` 8, `state.go` 6, `config.go` 4, `generation.go` 2, and one comment-only line each in `windowsize.go`, `attach.go` and `overlay.go` — and say plainly that these are the re-count, not the figures recorded further up this file against commit `d39b30648`, which differ by one on `lifecycle.go`.
  Then state the after figures.
  The four host files are expected to show comment-only hits.
  `doc.go`'s figure is reported as changed-by-design rather than as evidence, since card 10 rewrote its Selvage section.
  Name the enforcement test as what keeps the count from regressing, so the audit is not re-run by hand next time.

  Correct the `## The one complication` section.
  Its claim that `pinGeometryOptionsLocked` recombines all three concerns is stale read against the current code: that function issues the status-line options, `window-size latest`, and the unset half of the watchdog hook, while the whole install half — the pins plus the watchdog signal entry — lives in `installResizePinsLocked`, and the "Selvage pin at index 0" property is a consequence of `render.FixedHeightPins`' ordering surfaced by `resizePinHookArgvs`.
  `windowsize.go`'s only Selvage hit in the whole file is a comment.
  Record that there is no three-way merge left to split, and that the function is deliberately left untouched by this task.

  Update the `## What needs to happen` section so it no longer reads as open questions for whoever picks this up — each of its three bullets is now answered by the Status section above it.
  Leave the `## The audit finding` and `## Related` sections as they stand: the first is the historical record the Status section is measured against.
- **Commit:** `docs(manifest): record the Selvage extraction's Status and correct the stale pin-geometry claim`

### Card 12: fix the stale file attribution in the shipped header-selvage doc

- **Context:**
  - `internal/reedengine/selvagepane.go`
- **Edits:**
  - `manifest/designs/reed-header-selvage.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  One targeted edit.
  The sentence attributing `ensureSelvagePaneLocked` and `splitSelvagePaneAtBottomLocked` to the package's lifecycle file asserts as shipped fact something that is now false, since both functions moved.
  Repoint that attribution at `internal/reedengine/selvagepane.go` and leave the rest of the sentence — the behavior it describes and the `ReedState.SelvagePaneID` and `reed.yaml` details that follow — exactly as written.

  The earlier sentence describing "~230 lines of pane-lifecycle machinery" across the four files is historical framing for why the header-pane item existed, not a claim about the current layout, so it stays as it is.
  Change nothing else in this file.
- **Commit:** `docs(manifest): repoint reed-header-selvage's Selvage lifecycle attribution at selvagepane.go`

### Card 13: move the roadmap item to Done

- **Context:**
  - `manifest/designs/reed-selvage-pane-extraction.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move the Planned item whose bold name begins "reed: extract Selvage-pane lifecycle" out of `## Planned` and into `## Done`, at the top of the Done list.
  Write it literally as `1.` per that file's Maintenance section — numbering renders sequentially and never needs editing.

  Rewrite the entry body to Done shape: a name plus one or two sentences of what shipped, not the pre-work framing it carries now.
  Per the Maintenance rule that a Done entry points at the module's own package documentation, point it at `internal/reedengine`'s package documentation.
  Write that reference as plain prose with no markdown link, matching how every existing Done entry already references a package doc.
  Add one real markdown link alongside it, to the design doc, which survives this task carrying the before-and-after audit record — see the overview's `design-doc-is-kept-with-a-status-section` Shared Decision.
  Write it as a bare file link with no `#anchor` fragment, in the same `See [designs/<name>.md](designs/<name>.md)` shape the entry already uses while Planned.

  Change no other entry, and add no entry anywhere else.
  That one link must resolve, per the repository's Markdown Link Integrity invariant.
- **Commit:** `docs(manifest): move the Selvage-pane extraction item to Done`

### Card 14: full-run verification across every tier

- **Context:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Run four commands from the repository root and confirm each passes, changing nothing.
  This card produces no diff — it is the gate that proves the pure-refactor claim held.

  First `go build ./...`, then `go test ./...`, then `go test -tags integration ./internal/reedengine/ ./internal/reedcli/`, then `go test -tags smoke ./internal/reedcli/`.
  All four need `CGO_ENABLED=1` and a C compiler on `PATH`, which is the native default here.

  The `smoke` tier is the one this card exists for: it is the only tier that drives the CLI end-to-end against a live tmux server, it is where Selvage's split path is exercised for real, and the configured `pipeline.done_gate` does not reach it.
  Five of its files carry Selvage coverage — `smoke_selvage_keepalive_test.go`, `smoke_lifecycle_test.go`, `smoke_staterecovery_test.go`, `smoke_panecwd_test.go`, `smoke_dotfill_test.go` — and all five are expected to pass unedited, because they drive the CLI rather than the moved unexported functions.

  If any smoke test appears to need an edit, stop rather than editing it.
  A required smoke edit means observable behavior changed and the pure-refactor claim is false, which is a finding to surface, not a test to adjust.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/reedengine/` re-runs the package's untagged tier, which is the right gate for a batch whose only compiled edit is `doc.go`.
That file is comments only, so the run's real job is confirming the package still builds and that batch 1's suite — the enforcement test included — still passes after the doc edits land.

The three markdown files this batch edits have no runnable surface of their own.
`manifest/roadmap.md`'s and the two design docs' link integrity is covered by the repository's existing Markdown Link Integrity test, which `pipeline.done_gate`'s repo-wide `go test ./...` reaches.

Card 14 carries the tiers `verify:` deliberately excludes: the repo-wide build and untagged run, the `integration` tier, and the `smoke` tier against a live tmux server.
It runs once, at the end, rather than after every implementer and fixer round, because the smoke tier is minutes of real tmux work per run.
