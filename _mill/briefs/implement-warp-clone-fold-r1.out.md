Perfect. The batch has been completed successfully. All three cards have been implemented:

- **Card 4**: Created `internal/warp/warp.go` with the `RunCLI` dispatcher and `internal/warp/clone.go` with all clone logic moved from gitclone.
- **Card 5**: Moved all clone tests to `internal/warp` and deleted the `internal/gitclone` directory.
- **Card 6**: Wired warp into the main dispatch, removed git-clone case, and updated tests.

The verify command passed, confirming that all code builds correctly and tests pass.

{"status":"success","commit_sha":"f4396c567a65672bcc4961564ab9ed9460598bc0","session_id":"59341e77-1ec8-4ee1-9912-4e7272dca98d"}
