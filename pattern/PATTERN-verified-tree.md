# PATTERN-verified-tree

Every verify site runs the told verify command through `verifytree.Verify`: the plan's in a loom run, `darn.yaml`'s in a darn run.

- `Verify` takes a gate slot from the pool it is handed after the dirty and skip checks and before it spawns, so no site gains a second runner; a nil pool runs unslotted.
  The running marker reads `waiting` until the slot is held and `running` after, and the verify timeout counts only from the moment the slot is held.
- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- Records are per command, each entry with its tree and its commit; a skip requires an entry of the same command naming HEAD's tree.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler gate, the Darn gate, Publish, Finalize and `lyx webster verify`.
- Publish also runs landing config's `publish_verify`, when set, through `Verify` after the told verify, with `BaseCommand` set to the told verify command; Finalize never runs it.
- The Webster-Burler gate runs the derived round command through `Verify`, with `BaseCommand` set to the told verify command so its pass keeps the entry the next round diffs from.
- Each run that spawns its command writes its own `verify-<n>.log` in the verify directory and names it on its result, so no later verify overwrites an earlier run's log.
  After each such run, `Verify` keeps the newest logs up to its retention bound plus the one the failure record names, and removes the rest.
- Publish alone leaves a failure record in the verify directory when its plan verify or `publish_verify` fails with a non-zero exit: the failing kind, the failing tests, the failing run's own log, HEAD and the merge-in commit.
  A Publish that passes both verifies removes it before its push, and Finalize and the other sites neither write nor remove it.
- `internal/websterengine/cardverify.go` reruns a card's own `**Verify:**` command through `verifyrun` directly and is no plan-verify site.

## Enforcement

`cmd/lyx/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify`, no other production file calls `verifyrun.Run`, and none names the retired `verify-pending` marker.
