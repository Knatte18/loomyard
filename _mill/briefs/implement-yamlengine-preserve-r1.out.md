No tracked modifications remain. All three cards are committed, formatted, and verify passes.

{"status":"success","commit_sha":"8ae3efb72abe5069d3175fd87545daa90e3bb00c","session_id":"ab0778be-522c-48a4-85a8-57161a25c22e"}

Summary of work done in this batch (yamlengine-preserve, round 1):

- `internal/yamlengine/set.go` — Added `SetResult.Preserved []string`, a `preservedKeyComment` constant, and a new `preserveOrphanRootKeys`/`rootMappingNode` helper pair. `SetValues` now grafts any existing top-level key absent from the template onto `templateNode`'s root mapping (whole subtree, sorted by key name, marked with `# preserved (not in current template)` via direct assignment) before the unknown-pairs validation step, and returns the grafted key names via `SetResult.Preserved`. Doc comments updated accordingly.
- `internal/yamlengine/set_test.go` — Added `TestSetValues_PreservesUnknownExistingKey`, `TestSetValues_PreservesMultipleUnknownKeysSorted`, `TestSetValues_NoPreservedWhenAllKeysKnown`, `TestSetValues_PreservedKeyIdempotent`, `TestSetValues_PreservesNonFlatOrphanWhole`. All pre-existing tests untouched and still passing.
- `docs/shared-libs/yamlengine.md` — Added a new `### SetValues(...)` subsection (Behavior, Orphan-key preservation, Key properties, Returns) documenting the function, `KV`, and `SetResult`'s four fields.

Commits (all pushed to `config-set-key-loss`):
- `4c40744` fix(yamlengine): preserve orphaned config keys instead of silently dropping them
- `016c011` test(yamlengine): cover SetValues orphan-key preservation and idempotency
- `8ae3efb` docs(yamlengine): document SetValues and its orphan-key preservation

Verify (`go test ./internal/yamlengine/...`) passes in full, including all pre-existing and new tests.
