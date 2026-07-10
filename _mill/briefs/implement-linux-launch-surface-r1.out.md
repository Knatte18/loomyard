{"status":"success","commit_sha":"cbf1862e1590e50bebc6187f500df3078e87449f","session_id":"811aa8e4-5272-4012-9018-99cf9a5a676f"}

Summary of work completed for batch `05-linux-launch-surface`:

- Card 16 — Created `internal/warpengine/launcher_content.go` (pure `launcherScript`/`launcherExt`) and its test. Commit `1e0a0c0`.
- Card 17 — Made `MenuLauncherPath()` GOOS-aware in `internal/hubgeometry/hubgeometry.go` via `menuLauncherName()`; updated tests. Commit `502fcb8`.
- Card 18 — Rewrote `internal/warpengine/launchers.go`'s `writeLaunchers` to route both Windows and non-Windows through `launcherScript`/`launcherExt`. Commit `3920447`.
- Card 19 — Implemented `internal/vscode/launch_linux.go`'s `Launch`. Commit `cbf1862`.

Verify passed: `GOOS=linux go build ./internal/warpengine/... ./internal/hubgeometry/... ./internal/vscode/... && go test ...` — all green.
