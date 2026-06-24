Perfect! All work is complete. Both cards have been implemented:

1. **Card 1**: Fixed `setupCLIRepo` in `internal/worktree/cli_test.go` to write config via `paths.ConfigFile` and `paths.ConfigDir` instead of hardcoded paths. Added the `paths` import and removed the unused `lyxDir` local variable.

2. **Card 2**: Updated `internal/worktree/config_test.go` to resolve `_lyx` and `config` paths through `internal/paths` helpers in all three test setup blocks (`TestLoadConfig_HappyPath`, `TestLoadConfig_EmptyBranchPrefix`, `TestLoadConfig_EnvResolution`).

The verify command confirms all tests pass, and there are no uncommitted changes.

{"status":"success","commit_sha":"3e1bffff051788a3c7b79e34266327af2ef7284d","session_id":"9ed07ad7-ad8b-47f3-9150-205be9ac9f07"}
