All four review findings addressed and committed, and every batch `verify:` command passes.

Summary of fixes:

1. **BLOCKING:scope** -- `/home/knatte/Code/loomyard/wts/scout-extract-standalone-repo/_mill/plan/05-quarry-live-and-equivalence.md` Card 38 now declares `paths_test.go` under `Edits:` (was `Context:`-only) and documents the de-parallelization of all three seam-mutating tests as the correct fix, explaining why isolating the temp dir alone doesn't stop the race.
2. **BLOCKING:consistency** -- `/home/knatte/Code/loomyard/wts/scout-extract-standalone-repo/_mill/plan/06-lyx-removal.md` batch 6 cards renumbered 39-47 (shift by one), eliminating the duplicate "Card 38" identifier with batch 5.
3. **BLOCKING:design** -- `/home/knatte/Code/quarry/wts/quarry/quarry/lspclient.go`: all 5 bare `slog.Warn` sites now call `defaultLogHandler.Warn` explicitly (matching `ensureserver.go`'s 2 sites), dropped the now-unused `log/slog` import, fixed the stale doc comment.
4. **NIT:consistency** -- `/home/knatte/Code/quarry/wts/quarry/docs/port-equivalence.md`: corrected "All 26 comparisons" to "All 27" in both prose locations to match the 27-row verdict table.

Commits: `e1bb4b4e` (plan fixes, this worktree, pushed), `80c2ccc` and `21835aa` (quarry worktree, pushed).

Verify commands run and passing: `go -C .../quarry test ./internal/...`, `go -C .../quarry test ./...`, `go -C .../quarry test -tags lsp ./... -count=1` (with `gopls` on PATH; also re-ran `internal/cli` 5x with `-race` to confirm the card-38 race fix is stable), and `go test ./...` in this worktree.

{"status":"success","commit_sha":"e1bb4b4e3fee300623cb18edbd79a75819a298cc","session_id":"14466974-c367-469b-9912-5c3916641130"}
