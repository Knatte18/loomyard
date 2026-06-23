MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-23
```

## Findings

### [BLOCKING] Unaccounted file: `internal/configcli/integration_test.go`

**Location:** `C:\Code\loomyard\wts\weft-producers\internal\configcli\integration_test.go`
**Issue:** This file is not listed in any batch's Creates/Edits and does not appear in the overview's "All Files Touched" manifest; Batch 4 Card 14 names exactly two files to create (`configcli_test.go` and `configcli_integration_test.go`), yet a third integration test file exists with a substantially overlapping e2e test (`TestIntegrationEditAndSync` vs `TestE2ESyncIntegration`).
**Fix:** Either add `internal/configcli/integration_test.go` to the plan's "All Files Touched" and to Card 14's Creates list (with rationale), or remove the file and consolidate the test into `configcli_integration_test.go`.

### [NIT] `os.Chdir` used directly instead of `t.Chdir` in integration test

**Location:** `C:\Code\loomyard\wts\weft-producers\internal\configcli\configcli_integration_test.go:53-56`
**Issue:** The plan (Card 14) explicitly requires `t.Chdir` for the integration test; the implementation uses `os.Chdir` with a manual `defer os.Chdir(oldCwd)`, which diverges from the stated plan and codebase convention.
**Fix:** Replace `os.Getwd` / `os.Chdir` / `defer os.Chdir(oldCwd)` with `t.Chdir(hostWorktreePath)`.

### [NIT] Overview `docs/overview.md` code block does not show `config` dispatch case

**Location:** `C:\Code\loomyard\wts\weft-producers\docs\overview.md:162-177`
**Issue:** The illustrative `switch module` snippet in the "Module dispatch" section still omits the `case "config"` line that Card 13 required adding to the doc comment; the actual `cmd/lyx/main.go` is correct, but the doc snippet is stale.
**Fix:** Add `case "config": return configcli.RunCLI(out, moduleArgs)` to the overview's switch block example.

## Verdict

REQUEST_CHANGES
One blocking out-of-plan file; two minor nits.
MILL_REVIEW_END