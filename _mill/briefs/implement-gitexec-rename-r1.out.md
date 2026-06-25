Perfect! The implementation is complete. I have successfully:

1. **Card 1**: Renamed the `internal/git` package to `internal/gitexec`, moving both `git.go` and `git_test.go` to the new directory and updating their package declarations.

2. **Card 2**: Updated all surviving production importers in `paths`, `board`, and `weft` packages to use the new `gitexec` package name and updated all `git.RunGit` calls to `gitexec.RunGit`.

3. **Card 3**: Updated all remaining importers in `worktree`, `gitclone`, and test files to use the new `gitexec` package and qualifier.

All changes maintain the exact behavior of the `RunGit` function with its 4-tuple signature, and the build passes with all tests passing.

{"status":"success","commit_sha":"298da5b31a9d0adc65dc117478430421dca490ec","session_id":"7d8d6357-0da8-419a-8c13-a293c659bd76"}
