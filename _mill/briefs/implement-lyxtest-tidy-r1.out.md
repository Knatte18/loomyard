{"status":"success","commit_sha":"99e1241b0bbff831dcdf3870e5a692e002b51406","session_id":"e631c253-7e6c-4b12-971c-1c262ffc1cb9"}

All work is complete. Summary:

**Card 11** (`internal/lyxtest/lyxtest.go`): Extracted helpers `initRepo`, `commitAll`, `initBareRemote`, shared `mustGit`. Refactored `buildHostHub`, `buildWeftPrime`, `buildWeftOnly` to delegate. Dead-helper audit done.

**Card 12** (`internal/weft/weft_integration_test.go`): Added `paths` import, replaced literal `_lyx` paths with `paths.LyxDirName`.

{"status":"success","commit_sha":"99e1241b0bbff831dcdf3870e5a692e002b51406","session_id":"e631c253-7e6c-4b12-971c-1c262ffc1cb9"}
