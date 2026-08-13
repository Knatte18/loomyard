MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
duration_s: 200.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [NIT:scope] scoutcli rebase names 1 of 4 defaulting points
**Demoted-from:** BLOCKING
**Section:** `the-injected-cwd-contract` ("rebase at the flag's **defaulting point** … `cli.go:142-145`")
**Issue:** The `dir := targetDir` / `dir = cwd` defaulting block occurs four times in `internal/scoutcli/cli.go` — at `:142-145`, `:272-274`, `:377-379`, `:569-571`, one per `RunE` matching the four enumerated `Getwd()` sites (`:136`, `:266`, `:371`, `:563`) — but only the first is named, and a missed one silently returns a wrong answer rather than failing to build.
**Fix:** State the rebase as applying at all four defaulting points (list the line ranges), and require the named `--target-dir` scenario to cover more than one subcommand.

### [BLOCKING:design] parseQuery/inFileQuery rebase has no stated mechanism
**Section:** `the-injected-cwd-contract` ("the same treatment applies to … `parseQuery`'s `file:line:col`, and `--in-file`")
**Issue:** `parseQuery(arg)` (`cli.go:774`) and `inFileQuery(inFilePath, name)` (`:794`) are package-level functions with no base parameter, called from six sites via the `buildQuery` closure; the doc gives the semantics but not the signature change — the same "not mechanical at the registration site" gap that forced `WrapRunCtx`.
**Fix:** Decide and record the mechanism (base parameter vs. closure over the seam cwd) for both functions, as was done for `WrapRunCtx`.

### [NIT:consistency] "scoutcli/cli_test.go untouched" collides with commit 2
**Demoted-from:** BLOCKING
**Section:** `guard-bans-both-chdir-spellings` (fourteen files "this task deliberately does not touch")
**Issue:** `internal/scoutcli/cli_test.go` is untagged and pins the current process-cwd behaviour — `TestInFileQuery_ResolvesRelativePathToAbsolute` (`:622-635`) does `t.Chdir(cwd)` and asserts `filepath.Join(cwd, "relative/bar.go")` — so any base-parameter change to `inFileQuery`/`parseQuery` in commit 2 must edit that file, contradicting its "not touched" disposition and the "each commit green on its own" property.
**Fix:** Give `scoutcli/cli_test.go` an explicit disposition under commit 2 (which tests change, whether it joins the guard's subject set).

### [NIT:consistency] Smoke-tier inventory understates the tree
**Section:** Scope/Out ("eleven `//go:build smoke` files (30 `.Chdir`)") and ("seventeen further `.Chdir`-using test files")
**Issue:** A repo-wide count gives 12 smoke files / 33 occurrences (reedcli 6 files/20, shuttlecli 3/6, burlerengine 2/4, treadleengine 1/3) and 12 further chdir files outside the 20 named, not 17 — 33 files / 111 occurrences total.
**Fix:** Correct both counts; they are out-of-scope populations, so no in-scope work changes.

### [NIT:consistency] Preflight caller line numbers disagree with themselves
**Section:** per-call-site notes (`:188` and `:225`) vs `three-commits-each-self-contained` (`:192` and `:229`)
**Issue:** The `loomengine.Preflight()` calls are at `preflight_integration_test.go:192` and `:229`; the earlier bullet cites `:188`/`:225`.
**Fix:** Use one pair of line numbers.

## Verdict

REQUEST_CHANGES
scoutcli rebase under-enumerated, its mechanism unstated, and one "untouched" file is touched.
MILL_REVIEW_END
