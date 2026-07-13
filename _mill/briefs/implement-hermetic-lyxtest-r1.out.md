All 3 of 3 cards in batch `02-hermetic-lyxtest.md` were committed and verified. Card-count check: range start `8746f0fcfd5c221ee991df3117a8c12ee6d628a3` (the `mill-go: start batch` commit), `git log <range-start>..HEAD --oneline` shows exactly 3 commits matching all 3 `## Cards` `Commit:` messages — Card 2 (`63f080f`), Card 3 (`1753c51`), Card 4 (`96f8e13`). 3 of 3 cards committed, all done.

Summary of work:
- `internal/lyxtest/lyxtest.go` — `initRepo` and `initBareRemote` now set `core.fsmonitor=false`, `maintenance.auto=false`, `gc.auto=0` on every template repo (Layer A), with extended godoc.
- `internal/lyxtest/hermetic.go` (new) — exported `HermeticGitEnv()`, `sync.Once`-guarded, writes a neutral global git config to a temp file and sets `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM=1` (Layer B), with full godoc covering purpose, call site, accepted-leak lifecycle, and the guard-token rename caveat.
- `internal/lyxtest/lyxtest_test.go` — added `TestMain` (calls `HermeticGitEnv()` then `os.Exit(m.Run())`), `TestHermeticGitEnv_QuietAndPinned`, and `TestTemplateQuietConfig`, both `t.Parallel()`.

`verify: go test -tags integration -count=1 ./internal/lyxtest` passed (ok, ~4.6s). `git status --porcelain --untracked-files=no` is clean (no in-scope dirty tracked files; only the untracked brief file remains, which is out of scope). All commits pushed to `origin/faster-git-fixture-tests`.

{"status":"success","commit_sha":"96f8e1341d276586cd0a8ec564d4f2ee72fe868b","session_id":"3ccce4bd-3fab-4ecc-a9fb-3bc4dc86a600"}
