That's 40 chars + newline = 41, correct SHA-1 length. All good.

{"status":"success","commit_sha":"c96cb4071db9768782a0cc567c5852f484d31262","session_id":"3829c3cb-a9f9-4bda-ae1e-efe4b8433f47"}

Summary of work completed for batch `01-mux-extensions` (all 4 cards committed and pushed to `internal-shuttle`):

- `internal/muxengine/strand.go`, `internal/muxengine/strand_test.go` — added `AddSpec.SessionID` (opaque caller metadata), stamped into the appended `Strand`, with a round-trip test through `SaveState`/`LoadState`. Commit `90f15c6`.
- `internal/muxengine/template.yaml`, `internal/muxengine/config.go`, `internal/muxengine/config_test.go`, `internal/configsync/configsync_test.go` — removed the dead `claude:` config key and `Config.Claude` field; added `TestReconcileAll_DropsStaleMuxClaudeKey` covering reconcile of a stale user `mux.yaml`. Commit `4d5f2da`.
- `internal/muxengine/io.go`, `internal/muxengine/io_test.go` (new files) — added `resolveLivePaneID`, `(*Engine).SendText`, `(*Engine).SendKey` as pure pane-transport ops under the op lock, with hermetic tests for the resolver's error/happy paths. Commit `85c017f`.
- `internal/muxengine/io.go`, `internal/muxengine/io_test.go` — added `(*Engine).CapturePane` as a read-only op sharing the same resolver, with doc updates making explicit the shared-resolver coverage. Commit `c96cb40`.

Verify command `go test ./internal/muxengine/...` passes. `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes). Final HEAD: `c96cb4071db9768782a0cc567c5852f484d31262`.

{"status":"success","commit_sha":"c96cb4071db9768782a0cc567c5852f484d31262","session_id":"3829c3cb-a9f9-4bda-ae1e-efe4b8433f47"}
