All 8 of 8 cards in batch A -- consumers are committed and verified. The batch's verify command (`go test -tags integration ./internal/initengine/... ./internal/loomengine/... ./internal/buildercli/... ./internal/webstercli/... ./internal/perchcli/...`) passes, and `go build ./...` is clean across the whole tree.

Summary of work:

- Card 1: `internal/initengine/init.go` — rewired `WireJunctions` onto `fabricengine`.
- Card 2: `internal/initengine/undo.go`, `internal/initengine/undo_test.go` — rewired `UnwireJunctions`, `EnvSyncOptions`, the `weftengine.Commit`→`fabricengine.New(...).CommitWeft(...)` construction, and `Push`→`PushWeftAt`; swept all deleted-module comments in both files.
- Card 3: `internal/initengine/init_test.go` — loop now `["board","fabric"]`, `fabricengine.LoadConfig(root)` one-arg form.
- Card 4: `internal/loomengine/preflight.go` — `HostClean`/`PairInSync` onto `fabricengine`.
- Card 5: `internal/loomengine/preflight_integration_test.go`, `internal/loomengine/testmain_test.go` — fixture wiring onto `fabricengine.WireJunctions`; also had to add a weft-branch fixup (checkout `fabricengine.WeftBranchName("main")`) discovered during verify, since fabric's `PairInSync` requires suffixed branch naming rather than warp's plain equality check (mirroring the same fixup `fabricengine`'s own `buildDiffPair` differential-test helper applies).
- Card 6/7/8: `internal/buildercli/weft.go`, `internal/webstercli/weft.go`, `internal/perchcli/run.go` — rewired the four weft-commit call sites onto `fabricengine.New(host, weft).CommitWeft(...)` / `PushWeftAt`.

One verify-driven fix (commit `cf13ab47`) was required beyond the cards' original shape: `fabricengine.New` unconditionally stat-validates both the host and weft paths, whereas the old `weftengine.Commit` short-circuited on `opts.SkipGit` before touching any path at all. Buildercli/webstercli tests exercise `WEFT_SKIP_GIT=1` against fixtures with no real weft worktree on disk, so the naive `New`-then-`CommitWeft` rewrite broke that CI/test bypass. Fixed by checking `opts.SkipGit` before constructing the `Fabric` handle in all three call sites (buildercli, webstercli, perchcli), restoring exact behavioral parity with `weftengine.Commit`.

Card-count self-check: 8 of 8 cards committed (commits `c59e7fd4`, `b5e127b9`, `91a9a401`, `6208a3ae`, `126f522a`, `703da287`, `ada43373`, `9a1edc9b`, plus follow-up fix `cf13ab47`), matching the batch's declared 8 cards. Working tree is clean.

{"status":"success","commit_sha":"cf13ab47a794f43483079752d430a2ff60ea869c","session_id":"5125010a-d6f6-4a34-8b0c-2ca81606d967","cards_done":[1,2,3,4,5,6,7,8]}
