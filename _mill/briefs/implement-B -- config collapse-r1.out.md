All 4 cards for this batch are committed:
- Card 9 → `fe1bf03a` (`refactor(configreg): drop warp/weft modules, keep fabric`), extended with plan edits `341b1dcf`/`64a46e98` and a discovered-fallout fix `370cbcc8` (`internal/configsync/configsync_test.go`, added to card 9's scope via a plan edit since `ReconcileAll` iterates `configreg.Modules()`)
- Card 10 → `fa19420b` (`refactor(configcli): dispatch weft sync via fabriccli.RunCLI`)
- Card 11 → `7b5b6790` (`test(configreg): pin fabric-only module list`)
- Card 12 → `aa67bcdf` (`test(configcli): rewrite integration fixture onto fabric`), extended via a plan edit to also cover `internal/configcli/configcli_test.go` and `internal/configcli/reconcile_integration_test.go`, both discovered as same-package fallout not listed in the original card scope.

Verify command (`go test -tags integration ./internal/configreg/... ./internal/configcli/...`) passes, and I additionally confirmed `go build ./...` and the full `go test -tags integration ./...` are green (no other package was left broken by the registry collapse). Working tree is clean.

Card-count self-check: 4 of 4 cards declared in this batch (`## Cards`: 9, 10, 11, 12) are committed. This is a full completion, not partial — verified by matching commit subjects against the batch file's `Commit:` messages in `git log 5aaca973..HEAD --oneline`.

Relevant files (all absolute, under `/home/knatte/Code/loomyard/wts/fabric-cutover`):
- `internal/configreg/configreg.go`, `internal/configreg/configreg_test.go`
- `internal/configcli/configcli.go`, `internal/configcli/configcli_test.go`, `internal/configcli/configcli_integration_test.go`, `internal/configcli/reconcile_integration_test.go`
- `internal/configsync/configsync_test.go`
- `_mill/plan/02-config-collapse.md` (extended twice via the STOP-and-extend-plan protocol for files discovered mid-implementation)

{"status":"success","commit_sha":"aa67bcdf79673b804565a18ab7a22a0e03f4be6d","session_id":"e6973662-8f32-4475-a8cc-2855b82cce44","cards_done":[9,10,11,12]}
