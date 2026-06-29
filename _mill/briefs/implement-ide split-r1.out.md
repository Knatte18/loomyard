{"status":"success","commit_sha":"0d78080254199eaf5e7dec57289fabec322b7b71","session_id":"902200cf-c455-44e2-80a4-d8e578f0c3d7"}

All three cards implemented and verified. Relevant files:

- `internal/ideengine/spawn.go` — exported `CodeLauncher`, package `ideengine`
- `internal/ideengine/menu.go` — package `ideengine`, calls `Spawn` and `boardengine` unchanged
- `internal/ideengine/spawn_test.go` — swaps `CodeLauncher` in-package
- `internal/ideengine/menu_test.go` — retains `//go:build integration`, swaps `CodeLauncher` in-package
- `internal/idecli/cli.go` — package `idecli`, calls `ideengine.Spawn`/`ideengine.Menu`
- `internal/idecli/cli_test.go` — retains `//go:build integration`, swaps `ideengine.CodeLauncher` cross-package
- `cmd/lyx/main.go` — imports `idecli`, calls `idecli.Command()`

{"status":"success","commit_sha":"0d78080254199eaf5e7dec57289fabec322b7b71","session_id":"902200cf-c455-44e2-80a4-d8e578f0c3d7"}