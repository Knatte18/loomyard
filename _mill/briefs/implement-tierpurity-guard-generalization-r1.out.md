All 5 cards complete (Cards 1 and 2 folded into a single commit since Card 2's doc fixes were made as part of the same file edit as Card 1; Cards 3, 4, 5 each got their own commit). Verify command passed all three `-run` filters. Working tree is clean.

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/cmd/lyx/tierpurity_test.go` — generalized `isTierTagged` to `knownTierTags` list, added `TestIsTierTagged_RecognizesKnownTagsList`, fixed stale doc comments, renamed `spawnerAllowed`→`pathAllowlisted`, wired in the Sleep guard.
- `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/cmd/lyx/tiersleep_test.go` (new) — `findLongLiteralSleep`/`sleepDurationAtLeastOneSecond` AST-based detector, `allowedLongSleepers` allowlist, `TestFindLongLiteralSleep_DetectsAllArgumentForms`.
- `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/CONSTRAINTS.md` — updated Test Tier Purity Invariant statement and added the Real-time-wait guard bullet.

4 of 4 expected commits made (Card 2 had no separate commit since its changes were folded into Card 1's; all 5 cards' requirements verified satisfied in the final diff).

{"status":"success","commit_sha":"b1b8e6bbcc808863b51cf3f65c760a15f0903d3d","session_id":"4f25dffd-76eb-4c73-9535-59f372e5dfc0","cards_done":[1,2,3,4,5]}