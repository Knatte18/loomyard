{"status":"success","commit_sha":"e0f8744da6820fb917bd01e07ba5ae3724fb6005","session_id":"3e2fd3df-1c0d-413a-abfa-e3c01e2ee5f4","cards_done":[7,8,9,10,11,12]}

All 6 of 6 cards in this batch (7 through 12) are committed and verified. Summary of the work:

- **Card 7** (`c2148b21`): Moved `internal/fabricengine/fabrictest/hub.go` → `internal/hubforge/hub.go` via `git mv`, changed the package to `hubforge`, dropped the `//go:build integration` tag, and moved `GitStatusPorcelain` into `internal/gitkit/gitkit.go`.
- **Card 8** (`b062d9b3`): Moved `hub_test.go` alongside it, keeping its `integration` tag.
- **Card 9** (`cd0c1556`): Added `internal/hubforge/doc.go` and `internal/hubforge/testmain_test.go`.
- **Card 10** (`70c6e16e`, corrected by `2dbe5878`): Relocated all twelve live-state files into `internal/fabricengine/` with the `livestate_` prefix, `package fabricengine_test`, and `//go:build integration`; deleted the old `testmain_test.go`; retargeted `NewHub`/`AddPair`/`*Hub`/`GitStatusPorcelain` onto `hubforge`/`gitkit`; added `livestate_helpers_test.go` for `mustGit`; renamed `currentSHA` to `liveStateCurrentSHA`. Note: the original card-10 commit's `git add` included a stale pathspec that caused a fatal error and silently aborted staging most of the content edits (landing as pure renames). Caught during verify and fixed with a corrective commit (`2dbe5878`).
- **Card 11** (`2a2ffef0`): Retargeted the two external consumers (`dotlyxjunction_integration_test.go`, `weftgit_exclude_test.go`) onto `gitkit.GitStatusPorcelain`, and updated `commitweftat_test.go`'s comment.
- **Card 12** (`e0f8744d`): Removed the dead `fabrictest` exclusion from `cmd/lyx/destructiveguard_test.go`, swapped `internal/lyxcwd/enforcement_test.go`'s allowlist entries for `internal/hubforge`, and retargeted the remaining comment references in `fabriccli/clone.go`, `fabricengine/mutation.go`, `destroy_test.go`, `refusalof_test.go`, and `mutation_record_integration_test.go`.

Final verify (`go vet -tags integration ./...`, `go vet -tags smoke ./...`, `go test -tags integration ./internal/hubforge/... ./internal/fabricengine/... ./internal/lyxcwd/... ./cmd/lyx/...`) all passed, and `grep -rln '\bfabrictest\b' --include=*.go internal cmd` returns empty.

Key paths touched: `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/hubforge/{hub.go,hub_test.go,doc.go,testmain_test.go}`, `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/fabricengine/livestate_*_test.go`, `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/gitkit/gitkit.go`, `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/internal/lyxcwd/enforcement_test.go`, `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs/cmd/lyx/destructiveguard_test.go`.
