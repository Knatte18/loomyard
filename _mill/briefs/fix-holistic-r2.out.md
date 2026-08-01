All findings from the review were fixed and committed, and every verify command from all 7 batch plans passed. HEAD now differs from the baseline housekeeping commit, and there are no uncommitted tracked changes.

Summary of work:

1. **BLOCKING** — `internal/fabricengine/commit.go`: `Fabric.Commit` was reading the wired name-set from the stale per-pair `f.weftPath` base (`WiredNames(f.weftPath)`), which fails with "not initialized here" for any clone-does-everything pair since batch 3 stopped materializing a per-worktree `fabric.yaml`. Fixed to resolve `hubgeometry.ResolveWorktree(f.warpPath)` then `RepoWiredNames(l)`, matching the six sites card 7 already migrated. Updated `internal/fabricengine/commit_integration_test.go`'s `seedFabricConfig` helper and its three call sites to seed the repo-wide `fabric.yaml` at `hubgeometry.BoardDir(Hub)` instead.

2. **BLOCKING** — `docs/overview.md:172`: fabric module's CLI-surface bullet was missing the `unwire` verb registered in batch 6. Added it in its actual command-tree position.

3. **NIT** — `internal/gitignore/gitignore.go:105`: `Remove`'s doc comment named the deleted `lyx init --undo`; reworded to `lyx fabric unwire` (its only caller today, `internal/fabricengine/unwire.go`).

4. **NIT** — `internal/fabricengine/reconcile_stale_removal_test.go`: `TestReconcile_AddsMissingRemovesStaleNoOpsCorrect`'s "Add itself wires nothing" premise was false post batch-5's eager wiring, so its add-missing assertion passed without exercising Reconcile's add-missing branch. Fixed by narrowing the repo-wide pathspec to `_lyx` before `Add` and widening it back to `_lyx _pattern` before `Reconcile`, restoring genuine coverage. Swept and fixed the identical false claim in `TestReconcile_ConvergesAllWorktreesToRepoWidePathspec`'s comment too (same file, same violation class).

All 7 batch verify commands (`go test [-tags integration] ./internal/hubgeometry/...`, `./internal/fabricengine/...`, `./internal/configsync/...`, `./internal/fabricengine/... ./internal/fabriccli/...`, `./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... ./internal/loomengine/...`, `./cmd/lyx/...`) passed with exit code 0.

Files touched: `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/commit.go`, `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/commit_integration_test.go`, `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/docs/overview.md`, `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/gitignore/gitignore.go`, `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/reconcile_stale_removal_test.go`.

{"status":"success","commit_sha":"34c3d37bdee48db4e5267ebbb1508083145b2684","session_id":"b1c729c2-584d-4793-813b-b637dcbf4d4c"}
