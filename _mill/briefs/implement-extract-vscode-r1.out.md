Perfect! The structure is exactly as required:
- **internal/vscode/** now contains: config.go, color.go, launch_windows.go, launch_other.go, config_test.go, color_test.go
- **internal/ide/** now contains only: cli.go, spawn.go, menu.go, cli_test.go, menu_test.go, spawn_test.go

All three cards have been successfully completed:

1. **Card 1**: Created the `internal/vscode` package with exported functions WriteConfig, PickColor, Launch, and ErrUnsupported
2. **Card 2**: Migrated white-box tests from internal/ide to internal/vscode (color_test.go and config_test.go)
3. **Card 3**: Rewired internal/ide/spawn.go to use the new vscode package, updated cli.go package doc comment, and deleted the moved files

All tests pass (18 packages tested, all OK), and the module compiles successfully.

{"status":"success","commit_sha":"d713f0066b06e033dd7894acd57d108418262fb1","session_id":"88326b88-4d8d-4d87-a832-020f17b1ded0"}
