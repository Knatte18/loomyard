2 of 2 cards committed — both card 12 and card 13 are complete, matching the batch's declared card count exactly.

Summary: Card 12 (`fabrictest: add the Check constants and the two refusal-expectation helpers`) and Card 13 (`fabrictest: prove the refusal helpers pin the layer that refused`) are both committed. Verify passed: `go build ./...`, `go test -tags integration ./internal/fabricengine/fabrictest/`, and `go test ./internal/lyxcwd/ -run TestEnforcement` all succeed. Files touched:

- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/internal/fabricengine/fabrictest/refusal.go` (new)
- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/internal/fabricengine/fabrictest/refusal_test.go` (new)

Notable deviation from the batch's literal wording, documented in `driveOwnershipGateRefusal`'s doc comment: the batch specified reaching a `CheckOwnership` gate refusal via `Topology.Prune(apply=true)` against an unowned weft-suffixed hub child. I verified empirically that this path is structurally unreachable — `Prune`'s `applyStalePairOwnership` pre-check and the gate's own ownership predicate both call the identical `isRegisteredLinkedWorktreeIn` function on identical inputs by design, so they can never disagree, and the gate's raw `"ownership check failed"` message is never produced. I drove a genuine ownership refusal instead through `UnwireJunctions`, chaining a wired junction through an intermediate symlink so the gate's one-hop `RawTarget` read diverges from the pre-check's fully-resolved `PointsTo` read — a real, reachable gate-only check. This mirrors the same "if unreachable in practice, say so and pivot" allowance the batch explicitly grants for the dirtiness case.

```json
{"status":"success","commit_sha":"2b0823b799dfe2e6e61e46741384e0edd6c162ea","session_id":"23cd8cd3-7b57-4e09-a9cb-32f6b6c3af84","cards_done":[12,13]}
```
