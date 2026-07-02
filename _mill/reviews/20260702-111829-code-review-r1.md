No geometry-token violations in the new production code. This confirms the Hub Geometry Invariant holds. My review is complete.

MILL_REVIEW_BEGIN
# Review: Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-02
```

## Findings

(none)

## Verdict

APPROVE
All three batches implement the plan faithfully; contracts, tests, docs, and constraints all check out.
MILL_REVIEW_END

Summary of what I verified (all files read directly, no inference):

- `internal/yamlengine/set.go` implements root-key-granularity orphan preservation exactly as the Shared Decision specifies: `preserveOrphanRootKeys` walks the root mapping (via `rootMappingNode`, mirroring `collectLeafPathsHelper`'s `DocumentNode` unwrap in `internal/yamlengine/reconcile.go`), grafts whole subtrees in sorted order, sets `preservedKeyComment` via direct assignment (not concatenation), and returns the preserved list before the unknown-pairs validation block — matching plan `01-yamlengine-preserve.md` Card 1 precisely.
- `internal/yamlengine/set_test.go` covers all five required new tests (unknown-orphan preservation, multi-key sorting, no-preserved regression, idempotency, non-flat orphan) without touching any pre-existing test — matches Card 2.
- `docs/shared-libs/yamlengine.md` adds the `SetValues` subsection in the right place with the orphan-preservation behavior documented — matches Card 3.
- `internal/configengine/set.go` threads `Preserved` through `Set`'s new `([]string, error)` signature on every return path (nil on all error paths, `result.Preserved` on success) — matches `02-configengine-set-signature.md` Card 4.
- `internal/configengine/set_test.go` updates all four existing call sites and adds `TestSet_PreservesUnrecognizedExistingKeyEndToEnd` — matches Card 5.
- `docs/shared-libs/configengine.md` documents `Set`'s new return value and cross-references `yamlengine` — matches Card 6.
- `internal/configcli/configcli.go`: `setModule` and `editOne` both now emit `output.Ok` JSON envelopes; `setModule` conditionally adds `"preserved"` only when non-empty; `--set` help text extended — matches `03-configcli-json-envelope.md` Cards 7–9.
- `internal/configcli/configcli_test.go` adds `assertJSONOkContains` right after `assertJSONErrContains`, updates the required existing tests, and adds the four new tests including the `--set` → `reconcile` end-to-end drift-detection test (`TestDispatchSet_PreservedKeyDetectedByReconcile`) — matches Card 10.
- `internal/configcli/configcli_integration_test.go` strengthens `TestE2ESyncIntegration`'s final assertion with the JSON envelope check while keeping the substring assertion — matches Card 11.
- Cross-batch contract (`SetResult.Preserved` → `configengine.Set`'s second return → `configcli`'s `"preserved"` field) is consistent end-to-end; no duplicated helpers; no out-of-plan files; no Hub Geometry / CLI-Cobra invariant violations found (grepped for raw `os.Getwd`/geometry tokens in the changed files — none).
