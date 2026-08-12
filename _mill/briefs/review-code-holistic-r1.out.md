MILL_REVIEW_BEGIN
# Review: fabric: close the corrindex two-phase read-modify-write race (slice 15) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-12
```

## Findings

No findings.

Verified end-to-end against all three batches:

- `internal/state/state.go` extracts `readJSONUnlocked`/`writeJSONUnlocked` exactly per card 1 (verbatim bodies), and `ReadJSON`/`WriteJSON` are re-expressed on top of them with no behaviour change. `UpdateJSON` (card 2) matches the required signature, ordering (MkdirAll → one `AcquireWriteLock` → `readJSONUnlocked` → `mutate` → `writeJSONUnlocked`), and never composes via `ReadJSON`/`WriteJSON`.
- `internal/state/update_test.go` covers all five required dispositions (missing file, existing file, mutate error on both existing/missing file, corrupt file, goroutine concurrency driven through `UpdateJSON` itself with no barrier) and correctly reuses `sample` from `state_test.go` without redeclaring it.
- `internal/state/doc.go` (card 3) sits immediately above `package state` with no blank line, states the read-modify-write rule, the reason `UpdateJSON` can't compose from `ReadJSON`+`WriteJSON`, the one-consumer adoption fact, and the one-direction guarantee — all consumer-agnostic, no `internal/fabricengine` internals named.
- `internal/fabricengine/corrindex.go`'s `record()` (card 4) now routes through `state.UpdateJSON`, upserting against the freshly-read on-disk base, discards `found`, assigns `ix.recs` only after a successful write (persist-before-in-memory-update preserved), and its doc comment states the on-disk-base change, the on-disk-truth convergence consequence, and the named residual against `RebuildIndex`'s scan-to-write span. Signature, `loadCorrIndex`, `exact`, `nearestAtOrBefore`, `entries`, and the on-disk JSON format are all untouched, matching the plan's "no call-site changes" constraint.
- `internal/fabricengine/corrindex_test.go`'s new `TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite` drives `record()` against an external `state.WriteJSON` (not `record()` vs `record()`, correctly honoring the batch-local decision that a self-race test proves nothing given the weft-lock call-graph), is sequential, untagged, and `t.TempDir()`-only.
- `internal/fabricengine/index.go` (`RebuildIndex`, `refreshCorrIndexAfterSwitch`) is unchanged, matching the "do not give them the weft lock" shared decision.
- `internal/fabricengine/doc.go`'s new "The correspondence index's write path" section matches slice 12's rationale-only house voice (no evidence table, no round numbers, no process history), covers exactly the five required points, and names the residual as accepted/LOW/self-healing without ever implying the race is closed in both directions.
- All four non-roadmap design docs were repointed/rewritten per the card-6 verb table: `lyxtest-real-hubs.md`'s "slice 13" sentence and blockquote build-order line, `fabric-windows-verification.md`'s "slice 13"/green-suite sentence and its stale "four Planned slices" bullet, `fabric-unified-view.md`'s "Related" bullet (tense corrected to "now landed"), and `gitexec-error-shape.md`'s "Related" bullet — every repointed target is the bare `../../internal/fabricengine/doc.go` with the section named in prose, never a `#anchor` fragment.
- `manifest/roadmap.md` (card 7): the "slices 14-15" Planned item is fully deleted (no residual entry for the `RebuildIndex` window), a correctly-worded slice-15 Done entry is present (states `record()`-side-only, names `UpdateJSON`, names the residual, records why no `CONSTRAINTS.md` invariant was added, no `designs/` link), slice 12's stale "Slices 14-15 remain" sentence is replaced, and both remaining "Full task body lives at …" sentences (slice 13/14 Done entries) are gone.
- `manifest/designs/fabric-crucible-followups.md` is confirmed deleted from disk. A repo-wide grep for `fabric-crucible-followups` returns hits only under `_mill/` (plan, discussion, review artifacts) — zero dangling references outside the excluded path, matching the card's closing check.
- `CONSTRAINTS.md` was correctly left unedited, consistent with the "no new invariant" shared decision, and no other out-of-plan files were touched.
- No global-utility duplication introduced; no language pitfalls found (generics used consistently with existing `ReadJSON`/`WriteJSON` conventions; lock/mkdir ordering follows established precedent).

## Verdict

APPROVE
Implementation matches the plan, shared decisions, and CONSTRAINTS.md precisely across all three batches with no findings.
MILL_REVIEW_END
