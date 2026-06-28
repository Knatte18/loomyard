Working tree is clean. All cards are committed and verify passes.

{"status":"success","commit_sha":"fdbd24383981d20dff407c22f2f453b2d82a0aee","session_id":"de0e97bc-71a4-4771-ad09-fe19c667a88c"}

---

Here is a summary of what was implemented:

**Cards 5-9 (group RunE + PreRunE guards):**

- `/internal/warp/warp.go`: Added `RunE: clihelp.GroupRunE` to the warp parent command (no guard needed — warp has no `PersistentPreRunE`).
- `/internal/weft/cli.go`: Added `RunE: clihelp.GroupRunE` and the guard `if cmd.Name() == "weft" { return nil }` at the top of `PersistentPreRunE`.
- `/internal/board/cli.go`: Added `cmd.RunE = clihelp.GroupRunE` and the guard `if cmd.Name() == "board" { return nil }` at the top of `PersistentPreRunE`.
- `/internal/ide/cli.go`: Added `RunE: clihelp.GroupRunE` and the guard `if cmd.Name() == "ide" { return nil }` at the top of `PersistentPreRunE`.
- `/internal/muxpoc/cli.go`: Added `RunE: clihelp.GroupRunE` and the guard `if c.Name() == "muxpoc" { return nil }` at the top of `PersistentPreRunE` (using the existing parameter name `c`).

**Card 10 (per-module test updates):**

Updated all five `*_test.go` files to: (a) parse the output as JSON and assert `ok:false`, (b) change the old `"unknown command"` substring check to `"unknown"`.

**Card 11 (mounted tests):**

Created `/cmd/lyx/unknown_subcommand_test.go` with three test functions.

{"status":"success","commit_sha":"fdbd24383981d20dff407c22f2f453b2d82a0aee","session_id":"de0e97bc-71a4-4771-ad09-fe19c667a88c"}
