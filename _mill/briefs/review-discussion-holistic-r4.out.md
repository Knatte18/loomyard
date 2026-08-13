MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model; exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:design] scoutcli relative-base list stops at the package edge
**Section:** `the-injected-cwd-contract` (Consequence bullet) / Scope
**Issue:** The four named bases are `filepath.Abs` call sites *inside* `scoutcli`, but the raw `--target-dir` value leaves the package unresolved — `cli.go:142-147,173` passes `dir` into `buildOptions` → `scoutengine.Options.TargetDir` (`refs.go:50`), where `rootURIFor` does `filepath.Abs(targetDir)` (`ensureserver.go:120`, reached from `:182` and `:308`) and `DetectLanguage(opts.TargetDir, …)` reads the tree, both against the *process* cwd. `cli.go:446` is only the out-of-hub fallback branch, not the main path.
**Fix:** Decide the rebase at the flag's defaulting point — make `dir` absolute against the seam cwd before `lookupContext`/`buildOptions` — so every downstream consumer inside and outside `scoutcli` inherits it, and state that the enumeration is by value-entry point, not by `filepath.Abs` occurrence.

### [BLOCKING:consistency] Commit 2 cannot build green as stated
**Section:** `three-commits-each-self-contained`
**Issue:** Commit 2 changes `loomengine.Preflight()` → `Preflight(cwd string)`, but its only two callers are `preflight_integration_test.go:192` and `:229`, which commit 3 migrates — so commit 2 does not compile under `-tags integration`, contradicting "each building and testing green on its own". (`RunCLIIn` and `--into` are additive and do not have this problem; `Preflight` is the one breaking signature change.)
**Fix:** State that the two `Preflight()` call-site updates (and the `export_test.go` comment rewrite) land in commit 2 with the signature change, leaving only `t.Parallel()`/chdir removal for commit 3.

### [BLOCKING:design] "Injected cwd MUST be absolute" has no stated mechanism
**Section:** `the-injected-cwd-contract` (Value contract)
**Issue:** `WithCwd(ctx, dir) context.Context` returns no error, so "a relative value is illegal" and "`WithCwd(ctx, "")` is not legal" name a precondition with no detection: panic, silent `filepath.Abs` normalisation, and an error from `CwdFrom` are three materially different behaviours, and the section's own argument is that the bad case fails confusingly far from its cause.
**Fix:** Pick one and say so — e.g. `WithCwd` panics on a non-absolute or empty dir, or `CwdFrom` returns a named error — and add it to the TDD list beside the existing `CwdFrom` unit test.

### [NIT:consistency] fabriccli `:659`/`:716` carry two conflicting dispositions
**Section:** Technical context → per-call-site notes
**Issue:** Bullet 1 says `:659→680` and `:716→738` "need cwd as a per-call value, which `RunCLIIn` provides"; bullet 4 says `:493,579,609,659,716` "move onto `--into`, **not** onto `RunCLIIn`'s cwd". Verified in source: `:659`/`:716` are clone destinations (`--into`) and `:680`/`:738` are lookups for `reconcile` (`RunCLIIn`) — both, not either.
**Fix:** Reword bullet 4 to name only the destination chdirs and say those two tests use both seams.

### [NIT:design] `-race` is credited with catching a wrong-cwd migration
**Section:** Testing → Verification protocol
**Issue:** "`-race` is what catches a cwd dependence removed incorrectly" is false — process cwd is not race-detectable memory; a wrongly-rebased path fails an assertion, not the race detector. `-race`'s real value here is the newly parallel tests sharing state.
**Fix:** Restate `-race`'s purpose as parallel-safety of the newly parallelized files, and credit assertion preservation to the per-site notes and the named scenarios.

## Verdict

REQUEST_CHANGES
Scout rebase under-scoped, commit 2 will not compile, absolute-cwd precondition unenforced.
MILL_REVIEW_END
