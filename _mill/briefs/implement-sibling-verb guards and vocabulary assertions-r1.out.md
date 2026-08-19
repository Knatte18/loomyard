All 3 cards in batch 05 (sibling-verb guards and vocabulary assertions) are committed, plus one necessary fix commit discovered during verify. Card-count self-check: 3 of 3 declared cards committed (cards 13, 14, 15), matched by their exact `Commit:` messages in the log range `53b2033c..HEAD`. Verify passed both the untagged suite and the widened integration `-run` pattern.

{"status":"success","commit_sha":"358f7b106a8bc616da88c9f140354e8d9775f0e9","session_id":"327d2276-b14d-4f0c-9a09-1dd2ae8fcee5","cards_done":[13,14,15]}

Summary of work, with all commits on branch `fabric-merge-conflict-primitive`:

- Card 13 (`33a47729`): added `mergeBlocksMutation` to `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive/internal/fabricengine/mergestate.go`, and wired `*ErrMergeInProgress` refusals into `commit.go`, `pull.go`, `checkout.go`, and `remove.go`.
- Fix (`8e4a33b0`): during verify, two pre-existing `Remove` integration tests failed because `mergeBlocksMutation` propagated `newPaired`'s `ErrMissingPath` as a hard error instead of tolerating it; fixed in `mergestate.go` to report `false` for any unopenable pair, matching the existing unreadable-gitdir tolerance.
- Card 14 (`82df1367`): new `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive/internal/fabricengine/mergesiblings_integration_test.go` covering the full sibling-verb disposition table on a live conflicted merge (non-prime pair, since `Remove` refuses the prime slug unconditionally), plus Commit's foreign-git-state disposition on the prime pair.
- Card 15 (`358f7b10`): new `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive/internal/fabricengine/mergevocab_test.go` — the explicit side-free vocabulary assertion for `MergeResult`/`MergeOptions` fields, every named merge error's message, and the closed guard-reason set.

Verify command (`go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run 'Merge|Commit|Pull|Checkout|Remove|Cleanup' ./internal/fabricengine/`) passes in full.
