# PATTERN-verified-tree

Every plan-verify site runs the plan's verify command through `verifytree.Verify`.

- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- A skip requires the record to name HEAD's tree and the same command.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler gate, Publish, Finalize and `lyx webster verify`.
- `internal/websterengine/cardverify.go` reruns a card's own `**Verify:**` command through `verifyrun` directly and is no plan-verify site.

## Enforcement

`cmd/lyx/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify`, no other production file calls `verifyrun.Run`, and none names the retired `verify-pending` marker.
