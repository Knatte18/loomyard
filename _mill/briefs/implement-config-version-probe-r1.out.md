{"status":"success","commit_sha":"a5d2868eeddd3e339209b55eb5186dde0bf7ede7","session_id":"9262a5a1-f7e1-491c-a1f3-66b7aac33d9a"}

All five cards of batch `02-config-version-probe` are implemented and committed:

1. `feat(muxengine): GOOS-aware template defaults (tmux/bash on Linux)` (29d647b) — split template into `template_windows.go`+`.yaml` and `template_posix.go`+`.yaml`.
2. `feat(muxengine): per-binary min-version constants and -V parsers` (aada773) — new `version.go`/`version_test.go`. minPsmuxVersion `[3,3,3]`, minTmuxVersion `[3,3,0]`.
3. `feat(muxengine): capability probe for multiplexer version and command surface` (788e825) — new `probe.go`/`probe_test.go`.
4. `feat(muxengine): fail loud on unknown multiplexer surface at server-ensure` (cce10cf) — wired `probeCapabilityLocked()` into `lifecycle.go`.
5. `docs(muxcli): note capability probe in mux up help` (a5d2868) — `up.go` Long text updated.

Verify passed: `GOOS=linux go build ./internal/muxengine/... && go test ./internal/muxengine/... ./internal/muxcli/...`. Working tree clean.
