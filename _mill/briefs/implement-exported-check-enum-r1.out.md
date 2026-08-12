All 3 cards (4, 5, 6) committed. Card-count self-check: 3 of 3 cards committed, matching the batch's declared 3 cards. Verify passed.

{"status":"success","commit_sha":"6e5949bf4719a7e2e50ccc047764a19fffc6e9a2","session_id":"47e38dd8-9d9e-4da3-81e8-f723dc7ed7e1","cards_done":[4,5,6]}

Summary: All 3 cards in batch `exported-check-enum` are complete — 3 of 3 committed.

- Card 4: exported `Check` (string-backed, `CheckContainment`/`CheckOwnership`/`CheckDirtiness`), deleted `checkForce`/`String()`, added `Refusal`/`RefusalOf`, repointed `checkout.go`'s `String()` call. Commit `1d2c691c`.
- Card 5: repointed `internal/fabricengine/destroy_test.go` to the exported constants and `Check` type. Commit `e7aaacf6`.
- Card 6: added `internal/fabricengine/refusalof_test.go` covering `RefusalOf`'s `errors.As` traversal, the Check spellings, and the no-`CheckForce` rule. Commit `6e5949bf`.

`go test ./internal/fabricengine/` passes. Working tree is clean (no uncommitted tracked changes).

Files touched:
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/destroy.go`
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/checkout.go`
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/destroy_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/refusalof_test.go` (new)

{"status":"success","commit_sha":"6e5949bf4719a7e2e50ccc047764a19fffc6e9a2","session_id":"47e38dd8-9d9e-4da3-81e8-f723dc7ed7e1","cards_done":[4,5,6]}
