All fixes applied and verified.

**Finding: [BLOCKING:scope] Stale `run.go`/`drive.go` citation survives the rename**
- VERIFY: accurate - `manifest/designs/reed-fabric-standalone-api.md:143` still cited `internal/loomcli/run.go:145-320` and `drive.go:64`, both stale after batch 1's rename chain (old `run.go` -> `start.go`, `drive.go` -> `run.go`).
- HARM CHECK: none - fixing a doc citation to the correct current file locations doesn't break anything or conflict with any decision.
- Action: FIX. Added the file to batch 5 card 16's `Edits:` scope and the overview's file list (plan commit `d585e0e81`), then retargeted the citation to the three files where the six reed methods now live: `internal/loomcli/sharedbootstrap.go`, `start.go`, `run.go` (code commit `a586039da`).

All batch `verify:` commands ran clean from `/home/knatte/Code/loomyard/wts/loom-cli-rename`:
- `go test ./internal/loomcli/... ./cmd/lyx/... && go test -tags smoke -run XXX_NONE ./internal/loomcli/...` - pass
- `go test ./internal/fabricengine/` - pass
- `go test ./internal/loomengine/... ./internal/loomshed/... ./internal/shuttleengine/... ./internal/webstercli/... ./internal/websterengine/... ./internal/frictionengine/... ./internal/friction/... ./internal/shedadapters/...` - pass
- batch 4 verify: null (skipped)
- `go test ./internal/lyxcwd/ ./contracts/... ./tools/...` - pass

Working tree clean (`git status --porcelain --untracked-files=no` empty). HEAD is `a586039da4b09cef8662b3c7ad9439b1ba525239`, distinct from the recorded baseline `4a27db178341829258116ea625dee3a91ad32568`.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/_mill/plan/00-overview.md`
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/_mill/plan/05-docs-and-manifest.md`
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/manifest/designs/reed-fabric-standalone-api.md`

{"status":"success","commit_sha":"a586039da4b09cef8662b3c7ad9439b1ba525239","session_id":"6e6b5335-1938-469e-a2c7-acccf1e4f77d"}
