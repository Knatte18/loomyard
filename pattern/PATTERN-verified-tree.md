# PATTERN-verified-tree

Every verify site runs the call rendered from a gate of the hub's `gate.yaml` through `verifytree.Verify`: the `final` gate's at the webster gate and `lyx webster verify`, the `round` gate's at the Webster-Burler-Fix round gate, and the `landing` gate's at Merge-In, Finalize, Code-Burler-Fix and Darn-Write.

- `Verify` takes no gate slot and applies no timeout: its command is `lyx gate test --wait` calls, which take their own slots and bound themselves.
- No site verifies a dirty tree: `Verify` refuses one with its paths before running anything.
- Records are per command, each entry with its tree and its commit; a skip requires an entry of the same command naming HEAD's tree.
- The record is written only by `Verify`, and only after a pass; no prompt names its path.
- The sites are the webster gate, the Webster-Burler-Fix gate, the Code-Burler-Fix and Darn-Write gates, Merge-In, Finalize and `lyx webster verify`.
- The `round` gate's sites are Webster's two verify-gate fixers, the fixer fork and the cold fixer `lyx webster fix-verify` spawns, which share one prompt body and each run the hub's `round` call, rendered into the prompt's `fix_gate` marker, before they report.
- Code-Burler-Fix and Darn-Write each run their own gate key's call through `Verify`, the `code-burler-fix` and `darn-write` keys, and share no pass with Webster's end gate.
  Each lints from its own latest recorded pass or the merge base, and Darn-Write, and Code-Burler-Fix in a darn run, lint from the merge base on every attempt and run their own key's call over the touched trees' targets for a diff wholly under `darn.yaml`'s `tree_scope`, else their own key's call alone.
- Merge-In and Finalize run the `landing` call through `Verify` with `BaseCommand` set to the base command, which is the `final` call in a loom run and the `landing` call in a darn run, so a landing-list pass keeps the entry the next round diffs from.
  A darn Finalize runs its `finalize` key's call over the touched trees' targets in its place for a tree-scoped diff, resolved at each call; it is one `lyx gate test --wait` call, and an empty one, rendered when no touched tree contributes a target, runs nothing.
- Each run that spawns its command writes its own `verify-<n>.log` in the verify directory and names it on its result, so no later verify overwrites an earlier run's log.
  After each such run, `Verify` keeps the newest logs up to its retention bound plus the one the failure record names, and removes the rest.
- The failure record in the verify directory is the landing failure record: Merge-In and Finalize write it when their `landing` call fails with a non-zero exit, with the failing gate as kind, the failing tier, the failing tests, the failing run's own log, HEAD and the merge commit.
  A pass or a skip of the gate removes it, and the other sites neither write nor remove it.
- Every site tells `Site.Hold`, built by `verifytree.NewPauseHold` from its wiring's pause geometry.
  Finalize's site hold reads hub entries only while its run holds the parent lock; Merge-In's verify runs without the lock.
  `cardverify` and battencli's `verifyrun` rows tell none, being no verify site.
- `internal/cardverify/runner.go` reruns a card's own `**Verify:**` command through `verifyrun` directly and is no plan-verify site.
- `internal/battencli/land.go` runs the landing config's `deploy` command through `verifyrun` directly and is no plan-verify site: it verifies no tree.

## Enforcement

`cmd/lyx/verifiedtree_test.go`, a tripwire rather than a completeness proof.
Each site names `verifytree.Verify` and also names a `Hold` it passes, no other production file calls `verifyrun.Run`, and none names the retired `verify-pending` marker.
