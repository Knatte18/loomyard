# PATTERN-verified-tree

Every plan-verify site runs the plan's verify command through `verifytree.Verify`.

- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- Records are per command, each entry with its tree and its commit; a skip requires an entry of the same command naming HEAD's tree.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler gate, Publish, Finalize and `lyx webster verify`.
- The Webster-Burler gate runs the derived round command through `Verify`, with `BaseCommand` set to the plan's verify command so its pass keeps the plan-verify entry the next round diffs from.
- `internal/websterengine/cardverify.go` reruns a card's own `**Verify:**` command through `verifyrun` directly and is no plan-verify site.

## Enforcement

`cmd/lyx/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify`, no other production file calls `verifyrun.Run`, and none names the retired `verify-pending` marker.
