# PATTERN-verified-tree

Every verify site runs the call rendered from a gate of the hub's `gate.yaml` through `verifytree.Verify`: the `final` gate's at the webster gate and `lyx webster verify`, the `round` gate's at the Webster-Burler-Fix and Darn round gates, the `landing` gate's at Merge-In, Finalize and Publish, and the `publish` gate's at Publish.

- `Verify` takes no gate slot and applies no timeout: its command is `lyx gate test --wait` calls, which take their own slots and bound themselves.
- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- Records are per command, each entry with its tree and its commit; a skip requires an entry of the same command naming HEAD's tree.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler-Fix gate, the Darn-Write and Darn-Burler-Fix gates, Publish, Finalize and `lyx webster verify`.
- The Darn-Write and Darn-Burler-Fix gates run the tree-scoped command for a diff wholly under `darn.yaml`'s `tree_scope`, else the `round` gate's call, through `Verify` with `BaseCommand` set to the `landing` call, since darn has no webster gate.
- Merge-In, Finalize and Publish run the `landing` call through `Verify` with `BaseCommand` set to the base command, which is the `final` call in a loom run and the `landing` call in a darn run, so a landing-list pass keeps the entry the next round diffs from.
  A darn run's three sites run the tree-scoped command in its place for a tree-scoped diff, resolved at each call; it is one `lyx gate test --wait` call, and an empty one, rendered when no touched tree contributes a target, runs nothing.
- Publish then runs the `publish` call through `Verify`, with the same `BaseCommand`; an empty `publish` list runs nothing, and Finalize never runs the `publish` gate.
  A darn Publish skips the `publish` gate for a diff tree-scoped at that call.
- The Webster-Burler-Fix gate runs the `round` call through `Verify`, with `BaseCommand` set to the told base command so its pass keeps the entry the next round diffs from.
- Each run that spawns its command writes its own `verify-<n>.log` in the verify directory and names it on its result, so no later verify overwrites an earlier run's log.
  After each such run, `Verify` keeps the newest logs up to its retention bound plus the one the failure record names, and removes the rest.
- Merge-In and Publish alone leave a failure record in the verify directory when their `landing` or `publish` call fails with a non-zero exit: the failing gate as kind, the failing tier, the failing tests, the failing run's own log, HEAD and the merge-in commit.
  A Publish that passes both calls removes it before its push, and Finalize and the other sites neither write nor remove it.
- Every site tells `Site.Hold`, built by `verifytree.NewPauseHold` from its wiring's pause geometry.
  The Merge-In, Finalize and Publish site's hold reads hub entries only while its run holds the parent lock.
  `cardverify` and battencli's `verifyrun` rows tell none, being no verify site.
- `internal/cardverify/runner.go` reruns a card's own `**Verify:**` command through `verifyrun` directly and is no plan-verify site.
- `internal/battencli/land.go` runs the landing config's `deploy` command through `verifyrun` directly and is no plan-verify site: it verifies no tree.

## Enforcement

`cmd/lyx/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify` and also names a `Hold` it passes, no other production file calls `verifyrun.Run`, and none names the retired `verify-pending` marker.
