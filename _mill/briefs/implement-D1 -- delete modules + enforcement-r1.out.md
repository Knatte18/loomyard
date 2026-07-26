{"status":"stuck","stuck_type":"logic","reason":"Card 15's coverage-confirmation step failed: lifecycle_differential_test.go/reconcile_differential_test.go contain multiple explicitly-labeled fabric-only regression subtests (F1/F2/F3/R5/R6/R10 fixes) with no standalone equivalent in the Context list files","commit_sha":"5b60801f900e1116400d9db8dd2a08ff40338a3a","session_id":"5a32ca4d-1964-43f8-9d40-25342e769209"}

Card-count self-check: 0 of 4 cards committed this turn (range start `5b60801f900e1116400d9db8dd2a08ff40338a3a`, the batch-start commit; `git log 5b60801f..HEAD --oneline` is empty). No commits were made. This is not "all complete" — it is fully blocked at card 15.

## Summary

Card 15 requires reading each of the four `*_differential_test.go` files and confirming every fabric-specific assertion is already exercised by a standalone test in the card's `Context:` list before deleting them. I did this read carefully and found the confirmation fails for two of the four files:

- `/home/knatte/Code/loomyard/wts/fabric-cutover/internal/fabricengine/reconcile_differential_test.go` contains several subtests explicitly documented in their own doc comments as regression guards for real, previously-shipped bugs, with **no** standalone equivalent among the Context-list files (`reconcile_stale_registration_test.go`, `corrindex_test.go`, `index_integration_test.go`):
  - `TestPrune_DifferentialEquivalence/ApplyRemovesPortalAndLaunchers` — "fabric-only (the R6 fix)"
  - `TestPrune_StaleRegistrationReportedOnce` — "the F2/F3 regression guard (fabric-only, no warp comparison...)"
  - `TestCleanup_DifferentialEquivalence/PrimaryBranchSurvivesForceWhenNotCheckedOut` — "the F1 regression guard"
  - `TestCleanup_DifferentialEquivalence/NonSuffixedBranchNeverDeleted` — "fabric-only"
  - `TestCleanup_DifferentialEquivalence/DetachedHostHeadProtectsCheckedOutWeftBranch` — "fabric-only (the R5 fix)"
  - `TestPairInSyncAndHostClean_DifferentialEquivalence/PairInSync_RealDirNotAJunction` — "fabric-only (the R10 fix)"

- `/home/knatte/Code/loomyard/wts/fabric-cutover/internal/fabricengine/clone_differential_test.go` also has a narrower gap: `TestCloneHub_DifferentialEquivalence` is the only test exercising `CloneHub`'s "fresh" (non-adopt) weft-primary-branch-creation path end to end, and `TestCloneHub_DifferentialStrictAbort` is the only test exercising `teardownHub`'s cleanup-on-failure behavior; `clone_test.go` covers only helper functions (`DeriveHostName`, `cloneRepo` error formatting, `deriveBoardURL`) and `clone_adopt_test.go` covers only the adopt-existing-branch scenario.

Per the brief's explicit instruction ("If a genuinely unique fabric-behaviour assertion is found with no standalone equivalent, STOP and report it as a stuck (`stuck_type: logic`) rather than silently dropping coverage — do not invent a test blind"), I stopped without deleting any of the four files and made no edits or commits. The working tree is clean except for the brief file itself (untracked, out of scope).

This blocks cards 16-18 as well, since the batch narrative requires the differential tests removed first to keep every commit green. Resolving this likely requires the plan to add new standalone regression tests preserving these fabric-only assertions (or explicitly deciding that some are acceptable to retire) before card 15 can proceed.
