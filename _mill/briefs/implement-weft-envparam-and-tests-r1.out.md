Batch weft-envparam-and-tests — completed (Builder took over manually; prior haiku session had left the batch with two dropped tests).

State assessment: cards 5 (sync.go SyncOptions), 6 (cli.go env mapping), 7 (sync/status migration), 8 (cli/integration migration) were already committed by the prior session as ff311bf. Both verify tiers were green, but the equivalence guardrail was violated — two tests with unique assertions had been dropped during the table-drive migration:
- TestCommit_ScopedPathspec (only coverage of scopedPathspec)
- TestStatus_JunctionOk_Windows (SKIP_MKLINK_TEST junction case; card 7 required keeping it)

Both restored, migrated to lyxtest fixtures, in commit 5d7e5f2.

Verify: `go test ./internal/weft/...` (ok, cached) && `go test -tags integration ./internal/weft/...` (ok, 11.1s) — both pass. `-race` unavailable in this environment (no C compiler / CGO); parallel tests use isolated CopyWeft temp-dir fixtures with no shared mutable state.

{"status":"success","commit_sha":"5d7e5f2","session_id":"e42df79a-fdf2-4e74-8c3e-31426297af46"}
