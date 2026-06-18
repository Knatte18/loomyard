Perfect! All work is complete. I have successfully implemented the config-path-migration batch with the following changes:

**Summary of Changes:**

1. **Card 6** - Updated `internal/config/config.go` to read from `_lyx/config/<module>.yaml` instead of `_lyx/<module>.yaml`
2. **Card 7** - Updated `internal/board/init.go` to create `_lyx/config/` directory and write board.yaml and worktree.yaml files there
3. **Card 8** - Updated all test fixtures in `internal/config/config_test.go` and added a regression test to ensure the old flat path is not picked up
4. **Card 9** - Updated assertions in `internal/board/init_test.go` to check for files in the new `_lyx/config/` location
5. **Card 10** - Updated fixtures in `internal/board/config_test.go` and `internal/board/cli_test.go` to use the new path
6. **Card 11** - Updated the `seedWiki` helper in `internal/board/boardtest/bench_test.go` to create and use the new path structure
7. **Card 12** - Updated test fixtures in `internal/worktree/config_test.go`, `cmd/lyx/main_test.go`, and the comment in `cmd/lyx/main.go` to reference the new path

All tests pass successfully.

{"status":"success","commit_sha":"ee0adff568839d9faa4e540c8d57e1188ec9e06e","session_id":"a81ccdc1-f5f1-4427-9029-8511e17c0125"}