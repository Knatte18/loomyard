All 3 of 3 cards committed this turn (cards 12, 13, 14). Verify passed both halves. No uncommitted tracked changes remain (only the untracked brief file, which is out of scope).

{"status":"success","commit_sha":"9de72b86533a0577430a22c3bde8844b2a844c79","session_id":"ea56e12d-1c2b-47a0-aa7d-5ab84d37f03f","cards_done":[12,13,14]}

Summary of work:
- `internal/reedengine/doc.go` — extended the package godoc with the live-geometry rule, three new load-bearing bullets (silent layout rescale, the chained attach, the two geometry option pins), a `requiredSubcommands` note, and an extension to the Session targeting paragraph.
- `internal/reedengine/template_posix.yaml`, `internal/reedengine/template_windows.yaml` — restated the `width`/`height` comments as the detached-boot size and fallback render box, with no key/value change.
- `internal/reedengine/attachgeometry_integration_test.go` (new) — `//go:build integration && linux` real-pty/real-tmux proof with three `TestAttachGeometry*` functions (exact-layout + row budgets, degraded bare-argv path, stale-layout race safety), built on `golang.org/x/sys/unix` pty ioctls and the package's existing `newIntegrationEngine`/`waitUntil` harness.

Verify: `go test ./internal/reedengine/...` and `go test -tags integration -run TestAttachGeometry ./internal/reedengine/...` both pass. Card-count self-check: 3 of 3 cards committed, all declared cards complete.

{"status":"success","commit_sha":"9de72b86533a0577430a22c3bde8844b2a844c79","session_id":"ea56e12d-1c2b-47a0-aa7d-5ab84d37f03f","cards_done":[12,13,14]}