Confirmed: `RunCLI` calls `LoadConfig` before the subcommand switch. So `UnknownSubcommand` (`bogus`) only reaches the `default` case if `LoadConfig` succeeds — which requires the config to be at the path the loader reads. This validates the root-cause analysis: before the fix, `LoadConfig` fails (config written to stale `_lyx/worktree.yaml`), so `bogus` returns an error envelope via the LoadConfig path, but the test still expects exit 1 / ok:false, which it gets — so why does `UnknownSubcommand` "keep passing"? The card notes it keeps passing. The `List` and `RemoveWithForceFlag` subtests are the ones that genuinely fail pre-fix (they expect ok:true). The plan's claim is consistent.

I have verified all cards. The plan is accurate and complete. One thing to double check: the overview's `verify: null` at the top-level yaml, while each batch has its own verify. That's fine — batch-level verify is what's scheduled. Let me confirm there are no forward dependencies or missing Context entries.

All cards list `internal/paths/paths.go` in Context plus the edited file. Requirements reference `paths.ConfigDir`, `paths.ConfigFile`, `paths.LyxDirName` — all defined in paths.go (in Context). The `setupCLIRepo`, `LoadConfig`, etc. identifiers are in the Edits file or are stable. Context completeness satisfied.

MILL_REVIEW_BEGIN
# Review: Fix failing TestRunCLI in internal/worktree — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [NIT] Card 1 cites cli.go default case but omits it from Context
**Location:** Batch 1 / Card 1
**Issue:** Requirements explains `bogus` reaching the `default` case in `internal/worktree/cli.go`, a file not in Context/Edits; strictly this trips the context-completeness rule.
**Fix:** It is explanatory background, not a symbol to edit (the change is a pure RHS path substitution in cli_test.go), so no Context entry is actually needed — optionally drop the cli.go reference to stay clean.

### [NIT] Top-level overview verify is null
**Location:** 00-overview.md (`verify: null`)
**Issue:** The plan-level `verify` is null; only batch-level verifies are populated.
**Fix:** Harmless since mill-go schedules per-batch verifies; leave as-is or set it to the union for documentation.

## Verdict

APPROVE
Recipe, orphan-import rules, counts, sequencing, and DAG all verified against source; safe behaviour-preserving sweep.
MILL_REVIEW_END