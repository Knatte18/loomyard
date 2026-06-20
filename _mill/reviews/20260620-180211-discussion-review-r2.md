MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-20
```

## Findings

### [GAP] Template bare remote: upstream tracking unspecified
**Section:** Decisions › fixture-amortisation; Testing (Push/Pull scenarios)
**Issue:** The copy strategy rewrites `[remote "origin"] url` as a text edit, but the discussion never says the template establishes upstream tracking; today `addWeftRemote` (`internal/weft/sync_test.go:71`) runs `git push -u origin main`, and `TestPull_FastForward`/`Push` plus `hasUnpushed`/`Pull --ff-only` depend on that `@{u}`.
**Fix:** State that the template build does the one-time `push -u` so the copied repo carries `branch.main.remote/merge` + `refs/remotes/origin/main`, and that url-rewrite alone preserves it.

### [GAP] weft_integration_test.go missing from file inventory
**Section:** Technical context (weft fixtures); Decisions › build-tag gating
**Issue:** `internal/weft/weft_integration_test.go` exists, spawns real git against a bare remote via `addWeftRemote`, and is currently NOT behind `//go:build integration`; the weft file list (sync_test.go, cli_test.go) omits it, so the plan may leave it untagged.
**Fix:** Add it to the weft inventory and explicitly mark it for the `integration` tag.

### [NOTE] paths_test.go tagging only implied
**Section:** Technical context (paths fixtures); Decisions › build-tag gating
**Issue:** `paths.Resolve` spawns `git rev-parse --show-toplevel` (`internal/paths/paths.go:61`), so `paths_test.go` cases must be `integration`-tagged, but the discussion names only weft_test.go and the guard tests as staying untagged without stating paths_test.go is tagged.
**Fix:** Explicitly classify `paths_test.go` (Resolve/newTestRepo cases) as integration-gated.

### [NOTE] sync-case detached path interaction with new cli.go env read
**Section:** Decisions › parallelism via layered env→param
**Issue:** cli.go `sync` (line 129/133) calls `Commit` then `spawnPush`; `spawnPush` already early-returns on the env vars, so a NEW env→option read added at the `push`/`commit` call sites and the spawn-time check together are correct but redundant — worth a one-line note so the plan writer doesn't treat them as conflicting.
**Fix:** Note that spawn-time env check and the cli.go env→option read are complementary (spawn decides whether to fork; the forked child's Push reads via cli.go).

## Verdict

GAPS_FOUND
Two gaps: unstated template upstream-tracking and an omitted integration test file in the inventory.
MILL_REVIEW_END
