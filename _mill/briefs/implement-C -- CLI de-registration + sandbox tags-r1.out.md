{"status":"success","commit_sha":"98ffd3c396ed2af2c84d1ed52e12f56986a5e474","session_id":"ff5af2e0-8796-4f98-a7e4-e159eab96204","cards_done":[13,14]}

Both cards in batch C are committed: 2 of 2 declared cards complete.

Summary of work:

**Card 13** (`refactor(cmd/lyx): de-register warp/weft CLI and update pinned test sets`, commit `d1274ffa`):
- `/home/knatte/Code/loomyard/wts/fabric-cutover/cmd/lyx/main.go`: removed `warpcli`/`weftcli` imports and `root.AddCommand` registrations; dropped `weft`/`warp` from `root.Long`'s module list and the `PersistentPreRunE` comment.
- `/home/knatte/Code/loomyard/wts/fabric-cutover/cmd/lyx/helptree_test.go`, `main_test.go`, `unknown_subcommand_test.go`, `jsonhelp_test.go`, `exitcode_test.go`: removed all warp/weft assertions, table rows, and behavioural subtests per the batch's requirements.
- `/home/knatte/Code/loomyard/wts/fabric-cutover/tools/sandbox/SANDBOX-CORE-SUITE.md`: deleted scenarios S7 (weft) and S8 (warp).

**Plan gap discovered and fixed** (commits `9cd8ee31` plan edit + `98ffd3c3` code fix): `cmd/lyx/registration_test.go` was listed as Context-only, but its "exists ⇒ registered" invariant discovers `Command()`-exposing packages from disk, not the live cobra tree — since `warpcli`/`weftcli` aren't deleted until batch D1, de-registering them broke this guard. Per protocol I extended `/home/knatte/Code/loomyard/wts/fabric-cutover/_mill/plan/03-cli-dereg.md`'s card-13 Edits: list first, committed that, then added `warpcli`/`weftcli` to the test's `allowlist` map with a documenting comment (the allowlist mechanism's exact intended use for a documented mid-deletion exception).

**Card 14** (`refactor(sandbox): clone shared hub via fabric clone`, commit `e136f7c5`):
- `/home/knatte/Code/loomyard/wts/fabric-cutover/tools/sandbox/main.go`: flipped `cloneRun`'s shelled command from `lyx warp clone` to `lyx fabric clone` only; left the parallel-build prose comments untouched per the card's explicit instruction (that cleanup is card 24 in batch D3).

Verify (`go test ./cmd/lyx/... ./tools/sandbox/...`) passes, `go build ./...` succeeds, and `git status --porcelain --untracked-files=no` is clean.
