All 6 cards (15-20) committed successfully, verify passes clean, and no uncommitted tracked changes remain. Full completion: 6 of 6 cards committed.

{"status":"success","commit_sha":"a2bfff66d34463d4e136fbe899cdf78506954704","session_id":"ed5429b7-4755-49b8-8fd3-0d42f9074c47","cards_done":[15,16,17,18,19,20]}

Summary of work (all file paths absolute under `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename`):

- Card 15 (commit `51be45ab`): ran `tools/wordswap` over `docs/shared-libs/lyxcwd.md` and the five `.claude/agents/crucible-reviewer-*.md` files (dry-run then real run, 6 changes, 0 mismatches, 0 unresolved ambiguous).
- Card 16 (commit `b22d9a1e`): hand-edited `manifest/designs/fabric-unified-view.md` lines 86 and 162 only (verified `internal/fabriccli/clone.go`'s actual `warpURL` variable name before editing); left lines 88, 122, 203 untouched.
- Card 17 (commit `df94c04a`): reworded `docs/overview.md` and `README.md` per the Fabric-vs-warp vocabulary rule; left line 80 and the reed verb-sense lines untouched.
- Card 18 (commit `92d6273f`): reworded `docs/sandbox-hub.md`, `docs/sandbox-howto.md`, `docs/shared-libs/configengine.md`; also corrected a stale `DeriveHostName()` citation in `sandbox-hub.md` to `DeriveWarpName()` to match the actual Go sweep; left `configengine.md` line 66's `${env:HOST:-localhost}` untouched.
- Card 19 (commit `c0d60f59`): reworded `manifest/roadmap.md`, `manifest/designs/loom.md`, `manifest/designs/warp-visibility.md` (including its heading), leaving reed's verb-sense "hosts" occurrences untouched.
- Card 20 (commit `a2bfff66`): reworded all eight `tools/sandbox/SANDBOX-*-SUITE.md` prompt templates to consumer-facing Fabric/warp usage; every `**Covers:**` line and the machine-sense `conhost.exe` mentions in `SANDBOX-REED-SUITE.md` were left untouched.

Verify command `go test ./cmd/lyx/... ./internal/lyxcwd/...` passes. `go build ./...` also passes. `git status --porcelain --untracked-files=no` is clean (only the untracked `_mill/briefs/implement-docs-sweep-r1.md` orchestrator file remains, out of scope).

{"status":"success","commit_sha":"a2bfff66d34463d4e136fbe899cdf78506954704","session_id":"ed5429b7-4755-49b8-8fd3-0d42f9074c47","cards_done":[15,16,17,18,19,20]}
