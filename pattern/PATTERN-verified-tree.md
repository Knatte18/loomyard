# PATTERN-verified-tree

Every plan-verify site runs the plan's verify command through `verifytree.Verify`.

- `Verify` takes a gate slot from the pool it is handed after the dirty and skip checks and before it spawns, so no site gains a second runner; a nil pool runs unslotted.
  The running marker reads `waiting` until the slot is held and `running` after, and the verify timeout counts only from the moment the slot is held.
- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- Records are per command, each entry with its tree and its commit; a skip requires an entry of the same command naming HEAD's tree.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler gate, Publish, Finalize and `lyx webster verify`.
- Publish also runs landing config's `publish_verify`, when set, through `Verify` after the plan verify, with `BaseCommand` set to the plan's verify command; Finalize never runs it.
- The Webster-Burler gate runs the derived round command through `Verify`, with `BaseCommand` set to the plan's verify command so its pass keeps the plan-verify entry the next round diffs from.
- Publish alone leaves a failure record in the verify directory when its plan verify or `publish_verify` fails with a non-zero exit: the failing kind, the failing tests, a copy of the log, HEAD and the merge-in commit.
  A Publish that passes both verifies removes it before its push, and Finalize and the other sites neither write nor remove it.
- `internal/websterengine/cardverify.go` reruns a card's own `**Verify:**` command through `verifyrun` directly and is no plan-verify site.

## Enforcement

`cmd/lyx/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify`, no other production file calls `verifyrun.Run`, and none names the retired `verify-pending` marker.
