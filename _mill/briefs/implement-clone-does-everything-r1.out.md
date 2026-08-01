Summary of work completed for batch `clone-does-everything` (4 of 4 in the plan's numbering, all 6 declared cards):

- `internal/fabricengine/clone.go` — `CloneHub` now takes a `subpath` param and returns `CloneResult{HubPath, Anchor, BoardDir, WeftBase, PrimeCwd}`; resolves the lyx-anchor adopt-or-create, validates the subpath, writes `.fabric-anchor` to the board worktree on disk.
- `internal/fabriccli/clone.go` — `runCloneWithReset` now drives `configsync.ReconcileFabricAt` → `CommitWeftAt`/`PushWeftAt` → `WireJunctions` → `gitignore.Ensure` → `configsync.ReconcileAll`, leaving the clone intact on any orchestration failure.
- `internal/fabriccli/fabric.go`, `internal/fabriccli/weft_verbs.go` — migrated all nine CLI config-read sites to `LoadConfig(hubgeometry.BoardDir(l.Hub))`; added clone's `--subpath` flag and updated help prose.
- `internal/fabricengine/clone_adopt_test.go` — threaded the new signature through 4 existing tests; added 4 new tests covering create/typo/root-default/adopt anchor paths.
- `internal/fabriccli/cli_test.go` — added end-to-end CLI clone tests (`TestRunCLI_CloneEndToEnd`, `TestRunCLI_CloneDefaultSubpathAnchorsAtRoot`) plus fixture helpers; fixed `setupCLIRepo`/`TestRunCLI_EnvMapToOption` to seed fabric config at the real `hubgeometry.BoardDir` (a plan-text nuance where the fixture's `Hub` field is the worktree root, not the actual hubgeometry Hub container one level up — matched the existing `internal/boardcli` idiom).

Verify (`go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...`) passes. Working tree is clean, all commits pushed.

{"status":"success","commit_sha":"e8a46b36c0035766c10d86593cb8e7cd9d0b97b4","session_id":"ff407fb2-9f1a-4c5b-995a-2d031eb78e5f","cards_done":[14,15,16,17,18,19]}
