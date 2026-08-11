MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-opus-5 (runtime-reported; best-effort self-assessment)
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] `verbs_test.go`'s `vc.Run` call site is nowhere in the plan
**Location:** batch 7 / cards 30, 31; overview `## All Files Touched`
**Issue:** `internal/fabricengine/fabrictest/verbs_test.go:78` calls `err := vc.Run(t, h, fixture)` (integration-tagged, `package fabrictest`); card 30's arity change to `(fabricengine.Mutations, error)` breaks it, but that file appears in no card's `Edits:`/`Context:` and not in `## All Files Touched` — card 30 names only `verbs.go`'s closures, card 31 only `matrix_test.go`'s `runCell` and `TestCloneHubReset`.
**Fix:** Add `internal/fabricengine/fabrictest/verbs_test.go` to card 30's `Edits:` and to `## All Files Touched`, stating that `TestVerbCases_CleanState`'s call site discards the record with `_`.

### [BLOCKING:scope] `reconcile.go`'s own `writeWarpBinding` has no recording site
**Location:** batch 5 / card 21
**Issue:** Card 21 claims to cover "the hub-visible constructive writes cards 17-20 do not reach" and records `writeWarpBinding` only at its `CloneHub` call site, but `internal/fabricengine/reconcile.go:309` calls it too, inside `(*Topology).Reconcile`'s backfill branch — a `<boardDir>/.lyx-warp` write inside the hub that `CaptureManifest` sees and no record entry covers.
**Fix:** Add the `reconcile.go:309` `writeWarpBinding` success site to card 21's list as a `KindFileWritten`, gated on the `WarpBindingOutcomeRecorded` branch actually reaching it (`reconcile.go` is already in card 21's `Edits:`).

### [BLOCKING:consistency] `rollbackAdd`'s recorder parameter has two claimed owners and an incomplete requirement
**Location:** batch 4 / cards 13 and 15
**Issue:** Four of card 13's helpers are reached from inside `rollbackAdd` — `removeWeftWorktree` (add.go:227), `removeWarpJunction` (:242), `removePortal` (:249), `removeLaunchers` (:256) — yet card 13 never names `rollbackAdd`, while card 15 claims ownership of its `rec` parameter and lists only its `removeGitWorktree` and `deleteBranch` calls as recording through it. An implementer who passes `nil` at card 13's four sites ships a silently dropped rollback record that no batch-5 or batch-7 assertion catches (Add's mint-then-rollback nets to zero in the manifest diff).
**Fix:** State in card 13 that `rollbackAdd` gains its `rec *Mutations` parameter there (or in card 15) and name all six of its gate-reaching calls in whichever card owns it.

### [NIT:consistency] Card 13 says "three" test-caller helpers and names two
**Location:** batch 4 / card 13
**Issue:** "Three of these helpers have in-package test callers" is followed by exactly two: `removeLaunchers` in `portallauncher_test.go` and `removeJunctionRecords` in `weftwiring_test.go` — which matches the tree (those are the only two test callers among card 13's helpers).
**Fix:** Change "Three" to "Two", keeping the grep-is-the-authority sentence that follows.

### [NIT:consistency] Cards 25/26 hand a `*Mutations` to helpers declared over a `Mutations` value
**Location:** batch 6 / cards 23, 25, 26
**Issue:** Card 23 declares `okWithRecord(w, rec fabricengine.Mutations, …)` / `errWithRecord(w, rec fabricengine.Mutations, err)`, while cards 25 and 26 build a local `rec := fabricengine.NewMutations(...)` (a `*Mutations`) and say only "emit through the card-23 helpers"; Go will not auto-dereference, and the required `rec.Snapshot()` conversion is never stated.
**Fix:** Say in cards 25 and 26 that the CLI-layer recorder is passed as `rec.Snapshot()`, matching card 24's `r.Mutated()` value form.

## Verdict

REQUEST_CHANGES
Two unlisted work items and one split-ownership gap that can silently drop Add's rollback record.
MILL_REVIEW_END
