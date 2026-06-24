All three [NIT] findings from round 2 applied (verdict was APPROVE, no blocking findings).

1. Corrected nil-error wrapping in cloneRepo IsDir branch — internal/gitclone/clone.go.
2. Added clarifying comment on Windows parent+basename path handling — internal/gitclone/clone.go.
3. Extended cloneHub to return the resolved board URL; cli.go now uses it instead of re-deriving — internal/gitclone/clone.go, cli.go, clone_integration_test.go.

All tests pass: go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/

{"status":"success","commit_sha":"8ab5ddc8377bd88bf9e54e27bfdbb842ac14ddf5","session_id":"c688a1a7-6339-46f4-9a25-f845f7ff4fdc"}
