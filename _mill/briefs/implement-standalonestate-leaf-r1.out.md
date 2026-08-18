All 4 cards (5, 6, 7, 8) are committed, matching all 4 `Commit:` messages declared in the batch file. Verify passes for both the untagged and integration-tagged runs. Working tree is clean.

{"status":"success","commit_sha":"7ef3b358fc4a95abcf0100ec06422f9ac2845600","session_id":"a851199e-5f44-487f-b5e7-642dac3312fe","cards_done":[5,6,7,8]}

Summary: 4 of 4 cards committed for batch `02-standalonestate-leaf.md`.

Files touched (all under `/home/knatte/Code/loomyard/wts/orchestrator-preflight/internal/standalonestate/`):
- `doc.go` — package doc comment (Card 5)
- `standalonestate.go` — `Derive`/`derive` implementation, stdlib-only (Card 5)
- `standalonestate_test.go` — table-style tests driving both platform rows through `derive` (Card 6)
- `symlink_integration_test.go` — `//go:build integration` test pinning symlink normalization (Card 7)
- `leaf_enforcement_test.go` — stdlib-only leaf invariant enforcement, copied from the tokenvocab idiom (Card 8)

Verify command `go test ./internal/standalonestate/... ./internal/lyxcwd/... && go test -tags integration ./internal/standalonestate/...` passes. Working tree is clean (only the untracked, out-of-scope brief file `_mill/briefs/implement-standalonestate-leaf-r1.md` remains untracked, which is expected and excluded from the porcelain check via `--untracked-files=no`).

{"status":"success","commit_sha":"7ef3b358fc4a95abcf0100ec06422f9ac2845600","session_id":"a851199e-5f44-487f-b5e7-642dac3312fe","cards_done":[5,6,7,8]}
