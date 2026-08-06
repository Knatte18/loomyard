Summary: all 4 cards (37, 38, 39, 40) of the board-junction batch are committed, and the batch's full verify command passed.

Files touched:
- `internal/fabricengine/junction.go` (added `wireBoardLink`)
- `internal/fabricengine/clone.go` (wires `_board` at end of `CloneHub`)
- `internal/fabricengine/add.go` (wires `_board` alongside pathspec junctions in `Add`)
- `internal/fabricengine/reconcile.go` (unconditional `wireBoardLink` call in the `weftWorktreeExists` branch, next to `applyStaleRemoval`; extracted `appendPrDetail` helper)
- `internal/fabricengine/unwire.go` (added `unwireBoardLink` and `UnwireVerbResult.BoardJunctionRemoved`)
- `internal/fabricengine/boardjunction_integration_test.go` (new integration test file, 6 test functions)

Commits (HEAD b3fecb85):
- 74093804 feat(fabric): wire the operator-convenience _board junction at clone and add
- 2bb45b6a feat(fabric): re-wire the _board junction on every reconciled pair
- e6e471e8 feat(fabric): remove the _board junction on unwire
- b3fecb85 test(fabric): cover the _board junction's wiring, exclusion and non-monitoring

Card-count check: 4 of 4 cards committed -- all complete.

{"status":"success","commit_sha":"b3fecb851e7bd8d83bc322ed3dc3f97af83ae4b3","session_id":"4789b2c7-c7a7-453c-8f2a-13c3dd9612aa","cards_done":[37,38,39,40]}