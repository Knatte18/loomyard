MILL_REVIEW_BEGIN
# Review: config-layer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\config-layer\_mill\discussion.md
date: 2026-06-08
```

## Findings

### [GAP] --board-path vs cwd-authoritative _mhgo/ check
**Section:** Decisions → spawn-sync-path / config-location
**Issue:** config-location says an absent `<cwd>/_mhgo/` is a hard error ("run `mhgo init`"), but spawn-sync-path runs the detached child as `mhgo board --board-path <abs> sync` with an arbitrary inherited cwd that likely has no `_mhgo/` — the child would spuriously error before doing any work.
**Fix:** State explicitly that presence of `--board-path` bypasses `LoadConfig` and the `_mhgo/` existence check entirely (path-injected, no resolution), and that output names for the sync child are irrelevant (sync touches only git/tasks.json).

### [NOTE] boardtest CLI benchmarks unaddressed by new cwd model
**Section:** Testing → cli_test.go / Rename churn
**Issue:** `wikitest/bench_test.go` drives `RunCLI` via `--wiki-path <tempdir>` (lines 97/117/137) against dirs with no `_mhgo/`; under the new model that flag is gone and the CLI requires `<cwd>/_mhgo/`, so these benches break beyond simple rename, and CLI-bench numbers will now include `os.Getwd()`+config-load overhead.
**Fix:** Note that these CLI benches must be re-architected (temp cwd with `_mhgo/board.yaml`, or moved to the facade constructor) and acknowledge the added config-load cost in the CLI path.

### [NOTE] Proposal prefix in rendered link text, not just filenames
**Section:** Technical context → render.go
**Issue:** The configured prefix must reach three sites — filename generation (`renderProposals`), the orphan glob, AND the in-content links hardcoded as `proposal-%s.md` in `renderHome` (line 97) and `renderSidebar` (line 146); the brief calls out filenames and the glob but not these cross-reference links.
**Fix:** Add that the configured prefix also drives the proposal links emitted inside Home/Sidebar content, or links will point at filenames that no longer exist under a custom prefix.

## Verdict

GAPS_FOUND
One unresolved interaction (detached sync child vs cwd-required _mhgo/ check) must be settled before planning.
MILL_REVIEW_END
