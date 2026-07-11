{"status":"success","commit_sha":"70e734ec9a05b00a4a5bb3549b8e9e3c530117b2","session_id":"baff33d6-1804-40bd-ba66-1f63edbb4e55"}

Card-count self-check: 3 of 3 cards committed (Card 8, Card 9, Card 10), matching the batch file's declared `cards: 3`. Verify command (`go test ./internal/configreg/... ./internal/configsync/... ./internal/configcli/... ./internal/initengine/... ./cmd/lyx/...`) passed, and I additionally ran `go vet ./...` and `go test ./...` across the whole repo as a sanity check — all green.

Summary of changes:
- `C:\Code\loomyard\wts\modelspec\internal\configreg\configreg.go` — added `SeedOnly bool` to `Module`, converted `Modules()` literals to named fields, inserted `{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true}`.
- `C:\Code\loomyard\wts\modelspec\internal\configreg\configreg_test.go` — updated `TestNames` to the 7-module list, added `TestModules_SeedOnly`.
- `C:\Code\loomyard\wts\modelspec\internal\configsync\configsync.go` — `ReconcileAll` branches on `m.SeedOnly`: present file reported untouched, absent file materialized verbatim via `fsx.AtomicWriteBytes` with `Added` from `yamlengine.MissingKeys`.
- `C:\Code\loomyard\wts\modelspec\internal\configsync\configsync_test.go` — added `TestReconcileAll_SeedOnly` with 4 subtests (absent-materialize, present-untouched-with-alias, present-no-resurrection, non-seed-only-still-pruned).
- `C:\Code\loomyard\wts\modelspec\docs\overview.md` — amended the `config` module bullet to document seed-only reconcile semantics.

Commits: `46fd071`, `edb37a2`, `70e734e` (all pushed to `modelspec` branch).

{"status":"success","commit_sha":"70e734ec9a05b00a4a5bb3549b8e9e3c530117b2","session_id":"baff33d6-1804-40bd-ba66-1f63edbb4e55"}
