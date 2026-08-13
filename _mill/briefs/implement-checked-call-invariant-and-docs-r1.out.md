All work for this batch is complete and verified.

**Card-count summary:** 6 of 6 cards committed/completed this turn — cards 33, 34, 35, 36, 37 each landed a commit (`2ceedf56`, `dc426799`, `9d18e795`, `bfe9f8e9`, `9e6e3246`), and card 38 (Commit: none, verification-only) was re-performed this turn: all four of its checks pass (exactly 5 `//gitexec:raw` markers total across `internal/gitrepo`/`internal/fabricengine`, the pinned per-package counts in `cmd/lyx/checkedcall_test.go` match the tree, `TestNoDestructiveBypass_FabricengineProductionSource`/`TestMutationRecord_FabricengineProductionSource` pass, and no `//nolint:errcheck` sits beside a migrated `gitexec` call). This closes the whole task (batch 8 of 8). `go test ./...` and `go test -tags integration ./...` both pass in full, and the worktree is clean.

Relevant files touched:
- cmd/lyx/checkedcall_test.go (new)
- cmd/lyx/tierpurity_test.go
- cmd/lyx/hermeticenv_test.go
- cmd/lyx/rawgitmutation_test.go
- CONSTRAINTS.md
- docs/shared-libs/README.md
- manifest/roadmap.md
- manifest/designs/gitexec-error-shape.md (deleted)

{"status":"success","commit_sha":"9e6e3246856dc55c2dc9610d4332882849146b88","session_id":"c4757495-2763-4479-82e9-541926fa9bcd","cards_done":[33,34,35,36,37,38]}
