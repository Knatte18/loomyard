MILL_REVIEW_BEGIN
# Review: Extend codeintel lookup to non-Go languages via LSP

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] output belongs to CLI layer, not engine leaf
**Section:** Technical context (leaf note) vs Constraints (CLI/Cobra) vs Decisions (language-detection, server-provisioning)
**Issue:** The leaf allowlist is stated as "stdlib + hubgeometry + yaml + `output`", yet `output.Err/Ok` take an `io.Writer` and return an exit code (verified in `internal/output/output.go`), and the CLI/Cobra invariant + this task's own Constraints say `codeintelengine` returns `(T, error)` with no `io.Writer`/exit codes; the modelspec leaf it mirrors excludes `output` entirely.
**Fix:** Decide the layer split explicitly — engine returns typed errors, `codeintelcli` maps them to `output.Err`/`output.Ok`; drop `output` from the engine leaf allowlist and reword the "loud `output.Err`" decisions as engine-error-returned, CLI-emitted.

### [NOTE] Bare-symbol verb behaviour on multi-match unspecified
**Section:** Decisions → cli-verb / lsp-client-surface
**Issue:** `lyx codeintel refs <symbol|file:line:col>` accepts a bare name resolved via `workspace/symbol`, but the behaviour when a name resolves to zero or multiple candidates is not defined (the benchmark sidesteps this by hand-picking positions).
**Fix:** State the verb's contract for ambiguous/absent name resolution (e.g. loud error listing candidates) even though precise name resolution is out of scope.

### [NOTE] Detection precedence order named but not given
**Section:** Decisions → language-detection
**Issue:** "Deterministic precedence order" for polyglot repos is asserted but the actual ordering is left unstated, and AND-markers (`package.json` + `tsconfig.json`) vs OR-markers (`.sln`/`.csproj`) interaction with precedence is not spelled out.
**Fix:** Pin the precedence list and the AND/OR marker-match semantics, or explicitly delegate both to mill-plan.

## Verdict

GAPS_FOUND
Resolve the engine/CLI `output` layering contradiction before plan writing.
MILL_REVIEW_END
