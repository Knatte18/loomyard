# Batch: docs-sandbox

```yaml
task: 'fabric: warp-rebase / remote-reconcile recovery'
batch: docs-sandbox
number: 4
cards: 2
verify: null
depends-on: [2, 3]
```

## Batch Scope

This batch lands the documentation-lifecycle updates the slice's observable-CLI-behaviour change and campaign-slice completion require: the fabricengine package doc, the fabric campaign design doc, the module overview, the roadmap, and a new sandbox scenario. It is a pure-docs/markdown batch with no runnable surface (`verify: null`). It depends on batches 2-3 so the docs describe the shipped `Fabric.Pull`/CLI behaviour accurately.

## Cards

### Card 12: Doc-lifecycle updates for Fabric.Pull

- **Context:**
  - `internal/fabricengine/pull.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/gitrepo/doc.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `manifest/designs/fabric-unified-view.md`
  - `docs/overview.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  (1) `internal/fabricengine/doc.go`: add `Pull` to the cross-repo operations the `Fabric` handle exposes — the sentence currently naming "(`SyncWeft`, `RevertWithWeft`)" becomes "(`SyncWeft`, `RevertWithWeft`, `Pull`)" — and add a short paragraph describing `Fabric.Pull` as the unified read path: weft ff-pull first, then warp fetch + fast-forward-vs-rewrite classification, safe auto-reconcile (warp-only reset + weft correspondence re-anchor to the nearest surviving `Warp-SHA`, via the empty-commit mechanism) when local warp is clean, the double-conflict and no-surviving-anchor loud aborts, and the `PullResult` PATTERN-residue report — all detection/reachability via ancestry, never `SHAExists`.
  (2) `manifest/designs/fabric-unified-view.md`: in the "Build order — slices" section's slice-6 entry, correct the stale "Detection (`SHAExists`)" phrasing to reachability-based detection (`merge-base --is-ancestor` via `gitrepo.IsAncestor`), since `SHAExists` cannot detect a rewrite after fetch. Mark the fabric-layer half of slice 6 as landed (detection + safe re-anchor + PATTERN document via `Fabric.Pull`), and in the "still-open question" about exact rebase/remote-reconcile orchestration, record that the fabric-layer half is now resolved by `Fabric.Pull` while the orchestration-layer half (who consumes the PATTERN document and drives raddle-regen) stays open until `loom`/`Shed` exist. Do not overclaim: raddle regeneration and the LLM conflict-resolver remain out of scope/unbuilt.
  (3) `docs/overview.md`: update the `fabric` module bullet in the module list so its weft-content-sync description notes that `pull` is now unified across warp+weft with rebased-warp detection and safe re-anchor reconciliation (not weft-only); leave the CLI-surface enumeration (`clone|add|...|pull|sync`) unchanged since no verb is added.
  (4) `manifest/roadmap.md`: in the Done `fabric` entry, add a short clause noting warp-rebase / remote-reconcile recovery landed via `Fabric.Pull` (fabric-layer detection + safe re-anchor + PATTERN residue document); and in the `native clients` entry, update the stale lowercase `hasUnpushed` identifier mention to `HasUnpushed` following batch 1's promotion.
- **Commit:** `docs: describe unified Fabric.Pull rebase-reconcile`

### Card 13: Sandbox scenario for rebased-warp pull

- **Context:**
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new scenario `### F6 -- Rebased-warp pull recovery` to `tools/sandbox/SANDBOX-FABRIC-SUITE.md`, placed after the existing F5 scenario and before the closing material, in the exact bold-label format the other scenarios use (`**Covers:** fabric`, `**Goal:**`, `**Watch:**`, `**Verdict:**`, trailing `---`). The `**Covers:** fabric` line is required (Sandbox Suite Coverage). The Goal names the task without prescribing commands (F0 ethos): drive `lyx fabric pull` against a warp remote whose history was rebased/force-pushed underneath the local clone, and confirm fabric detects the drift and re-aligns rather than silently fast-forwarding or erroring. The Watch guidance should call out: that `pull` now touches BOTH warp and weft (not weft-only); that a clean local warp auto-reconciles (warp resets to the new remote history, weft's correspondence re-anchors) while a local warp with unpushed commits aborts loudly with no changes; and that the JSON output reports which `_pattern/`-touching weft commits need review. Keep the black-box framing (discover the surface via `lyx fabric pull --help`).
- **Commit:** `docs(sandbox): add F6 rebased-warp pull scenario`

## Batch Tests

`verify: null` — this batch edits only markdown docs and Go package-doc comments (no executable behaviour). The new `SANDBOX-FABRIC-SUITE.md` F6 scenario keeps `fabric` covered under the Sandbox Suite Coverage guard (fabric is already covered by F1/F4/F5, so coverage does not regress), and the package-doc edit to `doc.go` compiles as part of the whole-repo `go test ./...` done-gate. No runnable surface is introduced, so a scoped test command would be vacuous.
