MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewer_self_id: claude-sonnet-4.5 (self-assessed; exact point version not introspectable)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [BLOCKING] Batch 4 introduces a fabricengine↔configsync import cycle
**Location:** Batch 4 (clone-does-everything), cards 15 and 16, `internal/fabricengine/clone.go`.
**Issue:** Card 15 has `CloneHub` (package `fabricengine`) call `configsync.ReconcileFabricAt(boardDir, true)`, and card 16 has it call `configsync.ReconcileAll(weftBase, true)` — both require `fabricengine` to import `internal/configsync`. But `internal/configsync/configsync.go` already imports `internal/configreg` (confirmed, line 13), and `internal/configreg/configreg.go` already imports `internal/fabricengine` (confirmed, line 12, for `fabricengine.ConfigTemplate` module registration). This closes the cycle `fabricengine → configsync → configreg → fabricengine`, which Go's compiler rejects unconditionally — batch 4 will not build. Card 12's own mitigation ("configsync must not import fabricengine directly, route through `configreg.Template("fabric")` instead") only prevents the *direct* edge and does not see the *indirect* one through `configreg`, so it does not actually close the gap. Confirming context: the pre-existing `internal/initengine/init.go` safely imports both `configsync` and `fabricengine` together (lines 17-18) precisely because `initengine` sits *above* both packages in the import graph — the plan moves that same orchestration verbatim into `fabricengine.CloneHub` itself, which sits *below* `configreg`/`configsync`, losing that safety margin.
**Fix:** Do not call `configsync.ReconcileFabricAt`/`ReconcileAll` from inside `internal/fabricengine`. Either (a) keep `CloneHub` in `fabricengine` narrowly focused on git/wiring and move the config-materialization orchestration up into `internal/fabriccli`'s clone handler (which already imports both `fabricengine` and could import `configsync` without cycling), having `CloneHub` return `boardDir`/`weftBase` for the CLI layer to drive reconciliation, or (b) break the `configreg → fabricengine` edge (e.g. registering `fabric`'s template via a different indirection) so `fabricengine` no longer transitively depends on `configreg`/`configsync`. This must be resolved before batch 4 is implemented — it affects the card 15/16 design, not just a line of code.

### [NIT] Card 23's ported snippet silently drops an error the source checks
**Location:** Batch 6, card 23 (`internal/fabricengine/unwire.go`).
**Issue:** The card's illustrative snippet writes `f, _ := New(l.WorktreeRoot, l.WeftWorktree())`, discarding the error, whereas the source it says to port "verbatim in behavior" (`initengine/undo.go:145-148`) checks that error and returns it. Read literally, an implementer following the snippet would swallow a real `fabricengine.New` failure.
**Fix:** Correct the snippet (or add a note) to preserve `undo.go`'s `if err != nil { return ..., err }` handling around `New(...)`, matching "verbatim in behavior."

## Verdict

REQUEST_CHANGES
Batch 4's clone→configsync calls close a fabricengine→configsync→configreg→fabricengine import cycle; the design must route around it.
MILL_REVIEW_END
