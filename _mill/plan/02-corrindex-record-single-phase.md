# Batch: corrindex-record-single-phase

```yaml
task: 'fabric: close the corrindex two-phase read-modify-write race (slice 15)'
batch: 'corrindex-record-single-phase'
number: 2
cards: 1
verify: go test ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

This batch is the fix itself: `corrIndex.record` stops composing its payload from the stale in-memory `ix.recs` snapshot and applies its upsert to the freshly-read on-disk base under one exclusive lock, via batch 1's `state.UpdateJSON`.
It is one card because the reproducing test and the rewrite are a single TDD unit — the test's whole value is that its pre-fix failure is *observed* rather than assumed, which only holds if the same session writes it, watches it fail, and then fixes it.
No signature, call site, or existing test changes: `record` keeps `func (ix *corrIndex) record(e corrEntry) error`, and `exact`, `nearestAtOrBefore` and `entries` are untouched.

Batch-local decision beyond `## Shared Decisions`: the test drives `record()` against an **external write**, never `record()` against `record()`.
Every production path reaching `record()` runs under the weft write lock — its sole production caller is `Fabric.RecordCorrespondence`, called from `commitEmptySnapshot` and `commitWeftLocked`, both under that lock — so two `record()` calls against one index file cannot overlap in production.
A `record()`-versus-`record()` test would fail before the fix and pass after while proving nothing.
The interleaving that can occur is `record()` versus `RebuildIndex`, whose four callers hold no weft lock, and an external `state.WriteJSON` stands in for the rebuild's write deterministically, with no goroutines.

## Cards

### Card 4: rewrite `record` onto `state.UpdateJSON`, driven by a reproducing test

- **Context:**
  - `internal/state/state.go`
  - `internal/state/doc.go`
  - `internal/fabricengine/index.go`
- **Edits:**
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/corrindex_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Work in this order, so the pre-fix failure is observed rather than assumed.

  **First**, add the reproducing test to `internal/fabricengine/corrindex_test.go`, named `TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite`.
  It loads an empty index via `loadCorrIndex(path)` on a `t.TempDir()` path; then writes a *different* entry directly with `state.WriteJSON(path, path+".lock", []corrEntry{other})`, standing in for `RebuildIndex`'s write while the handle's snapshot is already stale; then calls `ix.record(mine)`; then asserts that a fresh `loadCorrIndex(path)` observes **both** entries.
  It is sequential, needs no goroutines and no barrier.
  Adding the `internal/state` import to `corrindex_test.go` is expected and does not breach the Test Tier Purity Invariant — keep the file untagged, keep every path a `t.TempDir()` path, and spawn no git.
  Run `go test ./internal/fabricengine/ -run TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite` and confirm it **fails** before touching `corrindex.go`.

  **Then** rewrite `record`'s body in `internal/fabricengine/corrindex.go`.
  Keep the signature `func (ix *corrIndex) record(e corrEntry) error`.
  Replace the `ix.recs`-based composition plus `state.WriteJSON` with a single `state.UpdateJSON[[]corrEntry](ix.path, ix.path+".lock", ...)` call whose `mutate` callback receives the freshly-read on-disk base and applies the existing logic to it verbatim: skip any entry whose `WarpSHA` equals `e.WarpSHA`, append `e`, and `sort.SliceStable` by `WarpSeq`.
  Discard the `found` flag, as `loadCorrIndex` already does.
  Assign the written result back to `ix.recs` only after `UpdateJSON` returns nil, preserving the persist-before-in-memory-update ordering the current doc comment promises; capture the mutated slice from inside the callback into a local so it can be assigned after the successful return.
  Update `record`'s doc comment: it must still state that persistence precedes the in-memory update so a write failure leaves the index unchanged, and must now also state that the upsert base is the freshly-read on-disk file rather than `ix.recs`, and that after a successful `record()` `ix.recs` reflects on-disk truth and may therefore contain entries another process recorded.
  Name the limit explicitly in that comment: this serialises `record()` against every other *write* to the file, but not against `RebuildIndex`'s scan-to-write span, so the reverse interleaving still loses an entry.

  Do not change `loadCorrIndex`, `exact`, `nearestAtOrBefore`, `entries`, the `corrEntry` struct, the on-disk JSON format, or any call site of `record`, `RecordCorrespondence`, `WeftSHAForWarpSHA` or `resolveRevertTarget`.
  Do not change `RebuildIndex`'s or `refreshCorrIndexAfterSwitch`'s locking in `internal/fabricengine/index.go` — that file is Context here, not an edit target.
  Do not assert the residual `RebuildIndex` scan-then-write window as fixed; it is not.

  Re-run the new test and confirm it passes, then run the full batch verify.
- **Commit:** `fix(fabric): apply corrindex record upserts to the on-disk base under one lock`

## Batch Tests

`verify: go test ./internal/fabricengine/...` runs the untagged Tier-1 suite for the package the fix lives in.
The scope is the package rather than the single file because `record()`'s behaviour is depended on beyond `corrindex_test.go`: `index_integration_test.go`'s and `checkout_index_refresh_test.go`'s refresh cover, and the `diff`/`revert`/`pull` suites that exercise `RebuildIndex` and the stale-hit self-correction path, all must keep passing with no behavioural change.
The new `TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite` is the batch's own gate, and it sabotage-proves cleanly: reverting `record()` to composing from `ix.recs` makes it fail on demand.
`corrindex_test.go`'s existing round-trip, upsert-by-`WarpSHA`, sort-order, `nearestAtOrBefore` and atomicity tests are the regression cover and must pass unedited.
Integration-tagged cover is out of this batch's verify scope and is picked up by the repo-wide done gate.
