MILL_REVIEW_BEGIN
# Review: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-13
```

## Findings

### [BLOCKING:design] `WrapRun` gives the handler no `cmd`, only `(out, args)`
**Section:** Decisions › plain-handlers-take-the-context
**Issue:** The rationale "clihelp.WrapRun already has cmd in scope, so passing the context through is mechanical" is false at the registration site: `clihelp.WrapRun(fn func(out io.Writer, args []string) int)` (`internal/clihelp/exec.go:123`) keeps `cmd` inside the wrapper, and every site passes a closure over `(out, args)` only (`configcli.go:325,343`; `fabriccli/fabric.go:125,159,171,196,227,243,264,303,346,372`).
**Fix:** Decide the mechanism — a new ctx-aware `clihelp` wrapper, or converting those `RunE`s to raw `func(*cobra.Command, []string) error` — and add it to Scope, whose `clihelp` bullet names only `ExecuteIn`.

### [BLOCKING:scope] Enumeration counts `Getwd()` sites, not ctx-threading sites
**Section:** Technical context › Production `lyxcwd.Getwd()` call sites to migrate
**Issue:** The work inventory is derived from `Getwd()` occurrences (15 + 2), but the actual surface is the transitive set of handlers that must carry a context: `resolveWarpLocation()` alone has twelve callers (`fabric.go:427,459,485,531,563,666,691,715`, `unwire.go:18`, `weft_verbs.go:76`), each a `WrapRun`-wrapped handler with no ctx today, yet `fabriccli` is scoped as two touch points.
**Fix:** State the enumeration method as "every function on the path from a cobra `RunE` to a cwd resolution", and re-derive the per-module touch-point counts from that.

### [BLOCKING:consistency] "nine files / 41 occurrences" contradicts the coalesce exclusion
**Section:** Scope › In (bullet 8) vs Scope › Out vs guard-bans-both-chdir-spellings
**Issue:** Scope claims migration of "every `.Chdir` call site in the nine `//go:build integration` target files (41 occurrences)", while Out declares `fabricengine/coalesce_integration_test.go` (2) untouched and the guard's subject set names exactly eight migrated files.
**Fix:** Restate as eight files / 39 occurrences, with coalesce present in the guard subject set only as the allowlisted exemption.

### [BLOCKING:design] A `t.Parallel()` target mutates a production package-level var
**Section:** Decisions › parallel-is-written-where-it-applies; Technical context › What is already safe
**Issue:** `idecli/cli_test.go:27-29` swaps `ideengine.CodeLauncher` (`ideengine/spawn.go:17`, package-level) and restores it in a `defer`; the file is one of the four slated for `t.Parallel()`, and the safety audit covers only infra packages and per-invocation closure locals, so "chdir only" as that file's blocker list is unverified.
**Fix:** Give the injectable-seam swap a stated disposition (keep the test serial, or convert the seam to a per-invocation injection) and extend the safety audit to test-owned global stubs in each parallelized file.

### [NIT:consistency] Q&A says 11 modules, Scope says 10
**Section:** Q&A log (first entry) vs Scope › In
**Issue:** "Build the explicit-cwd seam across all 11 `RunCLI` modules" contradicts the decided exclusion of `internal/selfreportcli`, which resolves no cwd.
**Fix:** Reword the Q&A answer to "10 of the 11 `RunCLI` modules".

### [NIT:scope] `ExecuteIn` needs a `RunRoot` variant, unnamed in Scope
**Section:** Scope › In (bullet 2)
**Issue:** `clihelp.RunRoot` hardcodes `context.Background()` (`exec.go:150`) and is the shared implementation for both `Execute` and `cmd/lyx`, so seeding a cwd requires changing or duplicating `RunRoot`, not only adding `ExecuteIn`.
**Fix:** Name the `RunRoot` change (parent-context parameter or sibling) in Scope and in commit 1.

## Verdict

REQUEST_CHANGES
Seam mechanism rests on a false `WrapRun` premise; enumeration and file counts need restating.
MILL_REVIEW_END
