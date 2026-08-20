MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:consistency] "Exact used surface" table's call-site counts don't match source
**Section:** Technical context, "The exact used surface of the shared packages" (and the parallel "8 call sites" figure in `exact-replacement-shapes`).
**Issue:** The table is labelled "Measured, not assumed," and package-level LOC totals throughout the doc (7011, 1962, 1552, 1939, 923, 3028, 2192, 59, 87, etc.) all verify exactly against current source, but several per-symbol counts in this specific table do not: `clihelp.SetExit` is 21 real call sites, not 22; `clihelp.Execute`/`ExecuteIn` are 1 each, not 2; `output.Err`/`Ok` are 14/7, not 17/9; `logger.Warn` is 6 real calls (7 total with `Info`), not the 7 claimed here (or "8 call sites" claimed in `exact-replacement-shapes`); `lyxcwd.CwdFrom`/`Resolve` are 4/1, not 8/2; `lyxcwd.WithCwd` has no direct call site in scout at all (only a comment); `proc.KillPID`/`DetachBreakaway` have 1 real call site each, not 6/2.
**Fix:** Not blocking — the port is mechanical file-copy plus hand-read-and-port-exactly per `exact-replacement-shapes` ("port their semantics exactly, not guess them"), and `go build`/`go test` will surface any real gap regardless of the stated counts. Re-measure this table with a stated methodology (or drop the specific numbers) before a batch treats any of them as a literal checklist target, e.g. "SetExit has 22 call sites."

## Verdict

APPROVE
Decisions are complete, mutually consistent, and source-grounded; only a non-blocking measurement nit remains.
MILL_REVIEW_END
