40 chars + newline, correct.

{"status":"success","commit_sha":"50eda5d3f32aa11d73d532dfae554fba855880ef","session_id":"c2cc5283-9aca-4b51-942a-f5080a6e2e7d","cards_done":[1,2,3,4]}

Summary: All 4 of 4 cards in batch `01-buildinfo-and-mode-mapping.md` were committed this turn:

1. `feat(buildinfo): add zero-import build-channel leaf` (fcc81c7a) — created `/home/knatte/Code/loomyard/wts/orchestrator-preflight/internal/buildinfo/buildinfo.go` and `internal/buildinfo/doc.go`.
2. `test(buildinfo): pin IsDev exact-match semantics` (b9c64dc1) — created `internal/buildinfo/buildinfo_test.go`.
3. `test(buildinfo): enforce zero-import leaf invariant` (79b24d9b) — created `internal/buildinfo/leaf_enforcement_test.go`.
4. `feat(stencilstore): add ModeFor, the single dev-to-Mode mapping site` (50eda5d3) — edited `internal/stencilstore/stencilstore.go`, created `internal/stencilstore/modefor_test.go`.

Verify (`go test ./internal/buildinfo/... ./internal/stencilstore/... ./internal/lyxcwd/...`) passed. Working tree is clean (pre-report self-check confirmed no dirty tracked files). All commits pushed to `origin/orchestrator-preflight`.

{"status":"success","commit_sha":"50eda5d3f32aa11d73d532dfae554fba855880ef","session_id":"c2cc5283-9aca-4b51-942a-f5080a6e2e7d","cards_done":[1,2,3,4]}
