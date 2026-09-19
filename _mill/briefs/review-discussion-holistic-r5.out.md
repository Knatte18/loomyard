MILL_REVIEW_BEGIN
# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514-class model (self-assessment, uncertain)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] `lyx loom`'s own verbs get no addressing decision
**Section:** run-id-positional-replaces-recipe-flag / Technical context (`entry.Args` paragraph)
**Issue:** The arity decision is stated for `battencli` only; `internal/loomcli`'s `run`/`step`/`status`/`pause` declare no `Args` validator at all (only `validate.go:45,110` set `NoArgs`), so after this change a run-id typed at `lyx loom status <run-id>` is silently ignored and addresses `self` — and `shedcli/table.go:45`'s `cobra.NoArgs` for loom plus `TestShed_LoomRunRefusesExtraArgByNoArgs` (parity_test.go:278) both contradict the new universal positional.
**Fix:** Decide whether loom's own subtree takes `[<run-id>]` with `MaximumNArgs(1)` or is self-only, and say what becomes of the pinned `NoArgs` assertion.

### [BLOCKING:design] Run-id listing refusal has no owner on the `lyx batten` path
**Section:** shedcli-resolves-the-location… / seed-verb-plus-auto-seed… / Technical context
**Issue:** The seed read and its listing refusal are placed in `shedcli`'s pre-run alone, and `battencli`'s own pre-run is explicitly "unchanged" — yet three separate passages assert that `lyx batten status|pause <unseeded-slug>` and argument-less `lyx batten run` land on the run-id listing refusal. Nothing on that path is specified to read a seed (batten's arm knows its recipe without one), so the natural behaviour is batten's `AbsentDisposition{Refuse:false}` `found: false`, not a listing.
**Fix:** State which code performs the seed-presence check on the `lyx batten` path and where it sits relative to the non-prime refusal and the auto-seed gate.

### [BLOCKING:scope] Board `type` has no write path
**Section:** board-type-field-defaults-to-loom-and-is-validated-late
**Issue:** `internal/boardengine/store.go`'s `upsertAllowedKeys` (ending line 223) is a closed allowlist enforced by `validateUpsertFields` for both `UpsertTask` and `UpsertTasksBatch`, so `type` can never be set once added to the structs; the discussion names `Task`, the store record, `template.yaml`, `render.go` and `layer.go` but not the allowlist, and names no CLI flag or daemon field for setting it.
**Fix:** State the write path — allowlist entry plus whichever `lyx board` flag/daemon field sets it — or say explicitly that `type` is write-less this task and that `Seed-Child` therefore always reads empty (`loom`).

### [NIT:decision] `child_driver`'s value at auto-seed time is unsourced
**Section:** seed-verb-plus-auto-seed-on-the-batten-entry-path
**Issue:** Prime's auto-written seed is specified as carrying `params.child_driver`, but no flag or default is named for it on the `lyx batten run|step <slug>` path (`lyx shed seed` gets `--driver`, batten's auto-seed gets nothing).
**Fix:** Say whether batten's auto-seed hardcodes `go` this task or takes a flag.

### [NIT:consistency] `persist`'s doc comment contradicts the no-op skip
**Section:** run-shed-re-entrancy-via-self-pointing-on-stuck
**Issue:** `internal/shedengine/run.go:452-454` states the seam "is called on every persist invocation, never conditionally on nextCurrentProducer having changed: state, history, and error can all change without it" — batten's `(producer, state)`-keyed skip is exactly that conditional, one layer out.
**Fix:** Note that the comment must be qualified in the same commit, and that the skip key is deliberately blind to `history`/`error` changes.

## Verdict

REQUEST_CHANGES
Loom-side addressing, batten's listing refusal owner, and Board `type`'s write path are unresolved.
MILL_REVIEW_END
