MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
duration_s: 206.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [NIT:scope] Guard's package set and allowlist size undefined
**Demoted-from:** BLOCKING
**Section:** Scope / `guard-bans-both-chdir-spellings`
**Issue:** The guard bans `t.Chdir(`/`os.Chdir(` in "the migrated packages' test files", but that set is never named, and the eight packages that gain a seam change carry ~14 further chdir-using test files the Out section defers (`boardcli/cli_test.go`+`cli_unit_test.go`, `burlercli/cli_test.go`, `shuttlecli/cli_test.go`, `scoutcli/cli_test.go` (7), `perchcli/cli_test.go`+`run_test.go`, `webstercli/cli_test.go`, `reedcli/cli_test.go`, `configcli/reconcile_test.go`+`reconcile_integration_test.go`) plus the 11 smoke files in those same packages — so the allowlist would exceed the guarded set.
**Fix:** State the guard's subject explicitly (per-file set of the nine migrated files, or per-package with the deferred files enumerated) and say what reason string the deferred entries carry.

### [BLOCKING:design] "Drift test gains a second pinned shape" is false
**Section:** `runcli-gains-a-sibling-rather-than-changing`; Constraints (CLI/Cobra)
**Issue:** `cmd/lyx/drift_test.go` only walks the cobra tree asserting non-empty `Short`; no test in `cmd/lyx` references `RunCLI` at all (only a comment in `main_test.go`), so no machine check pins the seam signature and there is nothing to "amend in the same commit".
**Fix:** Drop or restate the rationale — either say the seam shape is unpinned today (so `RunCLIIn` needs only the CONSTRAINTS.md wording), or decide to add a real signature-pinning test and name it.

### [BLOCKING:design] LYX_TRACE=1 exclusion rests on a false premise
**Section:** Out ("`internal/logger`'s durable-sink `sync.Once`")
**Issue:** The exclusion is justified by "no test sets `LYX_TRACE=1`", but CONSTRAINTS.md's Live-Substrate Spawn Observability documents `LYX_TRACE=1` as the supported under-`go test` gate; verified at `internal/logger/sink.go:79-98`, the sink then does `Getwd()`+`Resolve()` once per process, so removing the chdirs makes trace output resolve against the loomyard worktree instead of the fixture hub.
**Fix:** State the disposition for the `LYX_TRACE=1` path (accepted regression, `sinkDirOverride` in fixtures, or explicitly unsupported under parallel tests) rather than asserting the mode is unused.

### [NIT:design] `RunCLIIn` on selfreportcli accepts a cwd nothing reads
**Section:** Scope / `runcli-gains-a-sibling-rather-than-changing`
**Issue:** `internal/selfreportcli` resolves no cwd anywhere (`cli.go` has no `lyxcwd` reference), so its `RunCLIIn` would silently ignore its first argument.
**Fix:** Say whether such modules get the sibling anyway for uniformity (with a doc comment naming the no-op) or are omitted from the 11.

### [NIT:design] `--into` relative-path base unspecified
**Section:** `clone-gets-an-explicit-destination`
**Issue:** `runCloneWithReset` passes cwd straight into `CloneAndWire(cwd, …)` (`clone.go:119,133`); the discussion does not say whether a relative `--into` resolves against the process cwd or the context-carried cwd — the difference is exactly what parallel tests will hit.
**Fix:** Require `--into` to be resolved against the seam cwd (or mandate absolute) and state it.

### [NIT:scope] Doc inventory omits `docs/overview.md`
**Section:** Scope (doc updates)
**Issue:** `docs/overview.md:253` states the seam verbatim as `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`, and becomes incomplete once `RunCLIIn`/`ExecuteIn` exist; the doc list names only CONSTRAINTS.md, package docs, module docs and the timing row.
**Fix:** Add `docs/overview.md` to commit 1 or 2's doc set.

## Verdict

REQUEST_CHANGES
Three premises need correcting: guard scope, drift-test claim, and the LYX_TRACE sink exclusion.
MILL_REVIEW_END
