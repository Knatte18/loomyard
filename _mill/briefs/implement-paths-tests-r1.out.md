Batch paths-tests — card 13 complete (Sonnet implementer, Builder-reviewed). No production change.

Migrated internal/paths/paths_test.go and worktreelist_test.go onto lyxtest (CopyHostHub/MustRun), tagged //go:build integration, parallelised. Deleted drained helpers_test.go.

Builder review: top-level test set IDENTICAL to baseline (12); subtests 44/44, zero dropped. Untagged tier runs only the pure/guard tests (TestWeftGeometryMethods, TestHostLyxLinkHereDivergesFromLyxDir, TestWeftGeometryAtMainWorktree, TestCodeguideGuard, TestEnforcement) with zero git spawns — drain confirmed. Only the 2 test files + helper deletion changed.

Verify: go test ./internal/paths/... (ok) && go test -tags integration ./internal/paths/... (ok). -race not run (no CGO; not part of verify).

{"status":"success","commit_sha":"76b8beea529f9468c25545d3c3b6f197e8051160","session_id":"6417c0a0-4fdd-4c9b-a67c-76e515c1b0ba"}
