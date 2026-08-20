{"status":"success","commit_sha":"2514be9e6f87f412570ee4e8dd14895d655438c5","session_id":"01172a27-2b19-445e-b28b-46eef74a1664"}

Summary: Fixed the single NIT finding from the holistic review — stencil header comments in `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/contracts/stencils/bouncer/bouncer-template-seed.md` and `/home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/contracts/stencils/bouncer/bouncer-template-judge.md` named nonexistent Go identifiers (`runSeed`, `runJudge`); updated them to reference the actual call sites in `internal/shedadapters/bouncer.go` (`seedCall`/`runSeedSpawn`, `judgeCall`). Committed as `2514be9e` and pushed. All verify commands (`go test ./internal/shedadapters/...`, `go test ./internal/lyxcwd/...`, `go test ./contracts/stencils/...`) pass.

{"status":"success","commit_sha":"2514be9e6f87f412570ee4e8dd14895d655438c5","session_id":"01172a27-2b19-445e-b28b-46eef74a1664"}
